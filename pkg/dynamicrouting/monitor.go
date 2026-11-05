package dynamicrouting

import (
	"cmp"
	"math"
	"slices"
	"time"
)

const (
	monitorWindowSeconds = 60
	maxMonitorRoutes     = 512
	maxMonitorChannels   = 128
	maxMonitorEvents     = 20
)

// RouteSnapshot describes controller decisions, including retries and attempts
// later denied by capacity admission. It is not an upstream dispatch counter.
// Performance observations are shared by channel/model, while allocations and
// selection counts are isolated by group, model, policy and routing mode.
type RouteSnapshot struct {
	Group          string                 `json:"group"`
	Model          string                 `json:"model"`
	Policy         string                 `json:"policy"`
	Mode           string                 `json:"mode"`
	Dynamic        bool                   `json:"dynamic"`
	LastSeen       int64                  `json:"last_seen"`
	WindowSeconds  int                    `json:"window_seconds"`
	SelectionCount int64                  `json:"selection_count"`
	Truncated      bool                   `json:"truncated"`
	Channels       []RouteChannelSnapshot `json:"channels"`
	History        []RouteHistorySnapshot `json:"history"`
	Events         []RouteEventSnapshot   `json:"events"`
}

type RouteChannelSnapshot struct {
	ChannelID             int      `json:"channel_id"`
	Priority              int64    `json:"priority"`
	Weight                uint     `json:"weight"`
	EffectiveCostDiscount *float64 `json:"effective_cost_discount"`
	Active                bool     `json:"active"`
	TargetShare           float64  `json:"target_share"`
	SelectionCount        int64    `json:"selection_count"`
	SelectedShare         float64  `json:"selected_share"`
	Probe                 bool     `json:"probe"`
	Degraded              bool     `json:"degraded"`
	Unverified            bool     `json:"unverified"`
	EjectedUntil          int64    `json:"ejected_until,omitempty"`
	TTFTMS                *float64 `json:"ttft_ms"`
	TPOTMS                *float64 `json:"tpot_ms"`
	SampleCount           int      `json:"sample_count"`
}

type RouteHistoryChannel struct {
	ChannelID      int     `json:"channel_id"`
	SelectionCount int64   `json:"selection_count"`
	SelectedShare  float64 `json:"selected_share"`
}

type RouteHistorySnapshot struct {
	At             int64                 `json:"at"`
	SelectionCount int64                 `json:"selection_count"`
	Channels       []RouteHistoryChannel `json:"channels"`
}

type RouteEventAllocation struct {
	ChannelID int     `json:"channel_id"`
	Share     float64 `json:"share"`
	Probe     bool    `json:"probe"`
}

type RouteEventSnapshot struct {
	At          int64                  `json:"at"`
	Kind        string                 `json:"kind"`
	ChannelIDs  []int                  `json:"channel_ids"`
	Allocations []RouteEventAllocation `json:"allocations"`
}

type routeMonitor struct {
	lastSeen  time.Time
	dynamic   bool
	truncated bool
	channels  map[int]monitoredChannel
	buckets   [monitorWindowSeconds]monitorBucket
	events    []RouteEventSnapshot
}

type monitoredChannel struct {
	priority     int64
	weight       uint
	discount     float64
	active       bool
	share        float64
	probe        bool
	degraded     bool
	unverified   bool
	ejectedUntil time.Time
}

type monitorBucket struct {
	second    int64
	count     int64
	counts    map[int]int64
	truncated bool
}

// Recording reuses temporary lookup maps under Controller.mu. Stable routing
// decisions allocate no new candidate or allocation arrays for monitoring.
type monitorScratch struct {
	candidates  map[int]Candidate
	allocations map[int]Allocation
	degraded    map[int]bool
	unverified  map[int]bool
}

func (c *Controller) recordMonitorLocked(key RouteKey, candidates []Candidate, decision Decision, now time.Time) {
	if len(candidates) == 0 {
		return
	}
	if c.monitor == nil {
		c.monitor = make(map[RouteKey]*routeMonitor)
	}
	state := c.monitor[key]
	initialized := state == nil || now.Sub(state.lastSeen) >= max(c.config.MaxAge, monitorWindowSeconds*time.Second)
	if initialized {
		delete(c.monitor, key)
		if len(c.monitor) >= maxMonitorRoutes {
			var oldest RouteKey
			var lastSeen time.Time
			for existing, monitor := range c.monitor {
				if lastSeen.IsZero() || monitor.lastSeen.Before(lastSeen) {
					oldest, lastSeen = existing, monitor.lastSeen
				}
			}
			delete(c.monitor, oldest)
		}
		state = &routeMonitor{channels: make(map[int]monitoredChannel)}
		c.monitor[key] = state
	}
	if now.Before(state.lastSeen) {
		return // Clock rollback must not overwrite a newer bucket or target.
	}
	state.lastSeen, state.dynamic = now, decision.Dynamic
	state.truncated = len(candidates) > maxMonitorChannels
	if c.monitorScratch == nil {
		c.monitorScratch = &monitorScratch{
			candidates: make(map[int]Candidate), allocations: make(map[int]Allocation),
			degraded: make(map[int]bool), unverified: make(map[int]bool),
		}
	}
	scratch := c.monitorScratch
	clear(scratch.candidates)
	clear(scratch.allocations)
	clear(scratch.degraded)
	clear(scratch.unverified)
	active := scratch.candidates
	for _, candidate := range candidates[:min(len(candidates), maxMonitorChannels)] {
		active[candidate.ChannelID] = candidate
	}
	allocations := scratch.allocations
	for _, allocation := range decision.Allocations {
		if _, tracked := active[allocation.ChannelID]; tracked {
			allocations[allocation.ChannelID] = allocation
		}
	}
	degraded := scratch.degraded
	for _, id := range decision.DegradedCandidates {
		if _, tracked := active[id]; tracked {
			degraded[id] = true
		}
	}
	unverified := scratch.unverified
	for _, id := range decision.UnverifiedCandidates {
		if _, tracked := active[id]; tracked {
			unverified[id] = true
		}
	}
	changed, candidatesChanged := false, false
	failed, worsened, recovered := []int{}, []int{}, []int{}
	for id, previous := range state.channels {
		if _, exists := active[id]; !exists {
			candidatesChanged = candidatesChanged || previous.active
			delete(state.channels, id)
		}
	}
	for id, candidate := range active {
		previous, exists := state.channels[id]
		allocation := allocations[id]
		current := monitoredChannel{priority: candidate.Priority, weight: candidate.Weight, discount: candidateDiscount(candidate), active: true, share: allocation.Share, probe: allocation.Probe, degraded: degraded[id], unverified: unverified[id]}
		if route := c.routes[key]; route != nil {
			if ejection, ejected := route.ejections[id]; ejected {
				current.ejectedUntil = ejection.ejectedUntil
			}
		}
		candidatesChanged = candidatesChanged || !exists || previous.priority != current.priority || previous.weight != current.weight || previous.discount != current.discount
		changed = changed || math.Abs(previous.share-current.share) > 1e-9 || previous.probe != current.probe
		if current.ejectedUntil.After(previous.ejectedUntil) {
			failed = append(failed, id)
		} else if !previous.degraded && current.degraded {
			worsened = append(worsened, id)
		} else if exists && ((previous.degraded && !current.degraded) || (!previous.ejectedUntil.IsZero() && current.ejectedUntil.IsZero())) {
			recovered = append(recovered, id)
		}
		state.channels[id] = current
	}
	if initialized || changed || candidatesChanged || len(failed)+len(worsened)+len(recovered) > 0 {
		kind := "allocation_changed"
		ids := []int{}
		switch {
		case initialized:
			kind = "initialized"
		case candidatesChanged:
			kind = "candidates_changed"
		case len(failed) > 0:
			kind, ids = "failure", failed
		case len(worsened) > 0:
			kind, ids = "degraded", worsened
		case len(recovered) > 0:
			kind, ids = "recovery", recovered
		}
		slices.Sort(ids)
		event := RouteEventSnapshot{At: now.UnixMilli(), Kind: kind, ChannelIDs: ids, Allocations: make([]RouteEventAllocation, 0, len(state.channels))}
		for id, channel := range state.channels {
			event.Allocations = append(event.Allocations, RouteEventAllocation{ChannelID: id, Share: channel.share, Probe: channel.probe})
		}
		slices.SortFunc(event.Allocations, func(a, b RouteEventAllocation) int { return cmp.Compare(a.ChannelID, b.ChannelID) })
		if len(state.events) == maxMonitorEvents {
			copy(state.events, state.events[1:])
			state.events[len(state.events)-1] = event
		} else {
			state.events = append(state.events, event)
		}
	}
	if !decision.HasSelection {
		return
	}
	second := now.Unix()
	index := int(second % monitorWindowSeconds)
	if index < 0 {
		index += monitorWindowSeconds
	}
	bucket := &state.buckets[index]
	if bucket.second != second || bucket.counts == nil {
		*bucket = monitorBucket{second: second, counts: make(map[int]int64)}
	}
	bucket.count++
	if _, tracked := state.channels[decision.SelectedChannelID]; tracked {
		if _, exists := bucket.counts[decision.SelectedChannelID]; exists || len(bucket.counts) < maxMonitorChannels {
			bucket.counts[decision.SelectedChannelID]++
		} else {
			bucket.truncated = true
		}
	} else {
		bucket.truncated = true
	}
}

// MonitorSnapshot reads detached telemetry without running selection, advancing
// recovery or debt, expiring routing observations, or modifying controller maps.
func (c *Controller) MonitorSnapshot(now time.Time) []RouteSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]RouteSnapshot, 0, len(c.monitor))
	cutoff := now.Add(-max(c.config.MaxAge, monitorWindowSeconds*time.Second))
	firstSecond := now.Unix() - monitorWindowSeconds + 1
	for key, state := range c.monitor {
		if state.lastSeen.Before(cutoff) || state.lastSeen.After(now) {
			continue
		}
		snapshot := RouteSnapshot{Group: key.Group, Model: key.Model, Policy: key.Policy, Mode: "smart", Dynamic: state.dynamic, LastSeen: state.lastSeen.UnixMilli(), WindowSeconds: monitorWindowSeconds, Truncated: state.truncated, Channels: make([]RouteChannelSnapshot, 0, len(state.channels)), History: make([]RouteHistorySnapshot, 0, monitorWindowSeconds), Events: make([]RouteEventSnapshot, 0, len(state.events))}
		if key.discountFirst {
			snapshot.Mode = "discount"
		}
		counts := make(map[int]int64)
		for second := firstSecond; second <= now.Unix(); second++ {
			index := int(second % monitorWindowSeconds)
			if index < 0 {
				index += monitorWindowSeconds
			}
			bucket := state.buckets[index]
			point := RouteHistorySnapshot{At: second * 1000, Channels: []RouteHistoryChannel{}}
			if bucket.second == second {
				snapshot.Truncated = snapshot.Truncated || bucket.truncated
				point.SelectionCount = bucket.count
				for id, count := range bucket.counts {
					point.Channels = append(point.Channels, RouteHistoryChannel{ChannelID: id, SelectionCount: count, SelectedShare: float64(count) / float64(bucket.count)})
					counts[id] += count
				}
				slices.SortFunc(point.Channels, func(a, b RouteHistoryChannel) int { return cmp.Compare(a.ChannelID, b.ChannelID) })
			}
			snapshot.SelectionCount += point.SelectionCount
			snapshot.History = append(snapshot.History, point)
		}
		countIDs := make([]int, 0, len(counts))
		for id := range counts {
			countIDs = append(countIDs, id)
		}
		slices.Sort(countIDs)
		for _, id := range countIDs {
			if _, active := state.channels[id]; active {
				continue
			}
			// Retired candidates retain observed window counts, but no current target.
			if len(snapshot.Channels) >= maxMonitorChannels-len(state.channels) {
				snapshot.Truncated = true
				break
			}
			snapshot.Channels = append(snapshot.Channels, RouteChannelSnapshot{ChannelID: id, SelectionCount: counts[id]})
		}
		for id, channel := range state.channels {
			value := RouteChannelSnapshot{ChannelID: id, Priority: channel.priority, Weight: channel.weight, Active: true, TargetShare: channel.share, SelectionCount: counts[id], Probe: channel.probe, Degraded: channel.degraded, Unverified: channel.unverified}
			if channel.discount <= 1 {
				discount := channel.discount
				value.EffectiveCostDiscount = &discount
			}
			if !channel.ejectedUntil.IsZero() {
				value.EjectedUntil = channel.ejectedUntil.UnixMilli()
			}
			observationKey := ObservationKey{ChannelID: id, Model: key.Model}
			observationCutoff := now.Add(-c.config.MaxAge)
			reset := c.performanceAfter[observationKey]
			var ttftTotal, tpotTotal time.Duration
			ttftCount, tpotCount := 0, 0
			samples := c.observations[observationKey]
			for i := len(samples) - 1; i >= 0; i-- {
				sample := samples[i]
				if sample.ObservedAt.Before(observationCutoff) || !sample.ObservedAt.After(reset) || sample.ObservedAt.After(now) || !sample.Success || sample.HardFailure {
					continue
				}
				value.SampleCount++
				if sample.HasTTFT && sample.TTFT > 0 && ttftCount < c.config.MinSamples {
					ttftTotal += sample.TTFT
					ttftCount++
				}
				if sample.HasTPOT && sample.TPOT > 0 && tpotCount < c.config.MinSamples {
					tpotTotal += sample.TPOT
					tpotCount++
				}
			}
			if ttftCount > 0 {
				metric := float64(ttftTotal) / float64(ttftCount) / float64(time.Millisecond)
				value.TTFTMS = &metric
			}
			if tpotCount > 0 {
				metric := float64(tpotTotal) / float64(tpotCount) / float64(time.Millisecond)
				value.TPOTMS = &metric
			}
			value.Unverified = value.Unverified || max(ttftCount, tpotCount) < c.config.MinSamples
			snapshot.Channels = append(snapshot.Channels, value)
		}
		for i := range snapshot.Channels {
			if snapshot.SelectionCount > 0 {
				snapshot.Channels[i].SelectedShare = float64(snapshot.Channels[i].SelectionCount) / float64(snapshot.SelectionCount)
			}
		}
		slices.SortFunc(snapshot.Channels, func(a, b RouteChannelSnapshot) int { return cmp.Compare(a.ChannelID, b.ChannelID) })
		for i := len(state.events) - 1; i >= 0; i-- {
			event := state.events[i]
			event.ChannelIDs = slices.Clone(event.ChannelIDs)
			event.Allocations = slices.Clone(event.Allocations)
			snapshot.Events = append(snapshot.Events, event)
		}
		result = append(result, snapshot)
	}
	slices.SortFunc(result, func(a, b RouteSnapshot) int {
		if a.Group != b.Group {
			return cmp.Compare(a.Group, b.Group)
		}
		if a.Model != b.Model {
			return cmp.Compare(a.Model, b.Model)
		}
		if a.Policy != b.Policy {
			return cmp.Compare(a.Policy, b.Policy)
		}
		return cmp.Compare(a.Mode, b.Mode)
	})
	return result
}
