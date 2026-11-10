package dynamicrouting

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

const RecoveryProbeInterval = 5 * time.Second

// Recovery feedback needs enough history for the active-check cadence even when
// a route uses a very short customer-performance sampling window.
func recoverySampleMaxAge(config Config) time.Duration {
	const maxDuration = time.Duration(1<<63 - 1)
	if uint64(config.MinSamples) > uint64(maxDuration/RecoveryProbeInterval) {
		return maxDuration
	}
	return max(config.MaxAge, config.RecoveryObservationWindow, time.Duration(config.MinSamples)*RecoveryProbeInterval)
}

type RecoveryTarget struct {
	ObservationKey
	CooldownUntil time.Time
}

// RecoveryTargets refreshes isolation even when no customer requests arrive.
// Observations are shared by channel/model while each route retains its cooldown.
func (c *Controller) RecoveryTargets(now time.Time) []RecoveryTarget {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Independent recovery owns an active isolation until verified success. An
	// idle customer-traffic TTL must not silently reopen the failing upstream.
	for _, state := range c.routes {
		for id := range state.candidates {
			if _, ejected := state.ejections[id]; ejected {
				state.lastAccess = now
				break
			}
		}
	}
	c.cleanupExpiredLocked(now)
	targets := make(map[ObservationKey]time.Time)
	for route, state := range c.routes {
		if state.expired(state.config, now) {
			delete(c.routes, route)
			delete(c.selections, route)
			continue
		}
		for id := range state.candidates {
			key := ObservationKey{ChannelID: id, Model: route.Model}
			_, ejected, _ := c.updateChannelRecoveryLocked(state, key, now)
			if !ejected {
				continue
			}
			deadline := state.ejections[id].ejectedUntil
			if previous, found := targets[key]; !found || deadline.Before(previous) {
				targets[key] = deadline
			}
		}
	}
	result := make([]RecoveryTarget, 0, len(targets))
	for key, deadline := range targets {
		result = append(result, RecoveryTarget{ObservationKey: key, CooldownUntil: deadline})
	}
	slices.SortFunc(result, func(a, b RecoveryTarget) int {
		if order := a.CooldownUntil.Compare(b.CooldownUntil); order != 0 {
			return order
		}
		if a.ChannelID != b.ChannelID {
			return cmp.Compare(a.ChannelID, b.ChannelID)
		}
		return strings.Compare(a.Model, b.Model)
	})
	return result
}

// IsRecovering also protects pins and session affinity that bypass selection.
func (c *Controller) IsRecovering(route RouteKey, channelID int, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.routes[route]
	if state == nil || state.expired(state.config, now) {
		return false
	}
	_, ejected, _ := c.updateChannelRecoveryLocked(state, ObservationKey{ChannelID: channelID, Model: route.Model}, now)
	return ejected
}
