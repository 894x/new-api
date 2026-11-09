package dynamicrouting

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorCountsSelectionsInWindowAndKeepsUserAndModeSeparate(t *testing.T) {
	controller, key, candidates, limits, now := discountRoutingFixture(t)
	counts := map[int]int64{}
	for range 12 {
		decision := controller.SelectDiscountAvoiding(key, candidates, map[int]struct{}{1: {}}, limits, now)
		require.True(t, decision.HasSelection)
		counts[decision.SelectedChannelID]++
	}
	controller.SelectAvoiding(key, candidates, nil, now)
	userKey := key
	userKey.Policy = "user:42"
	controller.SelectDiscountAvoiding(userKey, candidates, nil, limits, now)
	controller.SelectDiscountAvoiding(key, candidates, map[int]struct{}{1: {}, 2: {}, 3: {}}, limits, now)

	snapshots := controller.MonitorSnapshot(now)
	require.Len(t, snapshots, 3)
	base := snapshots[0]
	require.Equal(t, "discount", base.Mode)
	assert.Empty(t, base.Policy)
	assert.EqualValues(t, 12, base.SelectionCount, "excluded decisions without a selection are not requests")
	for _, channel := range base.Channels {
		assert.Equal(t, counts[channel.ChannelID], channel.SelectionCount)
		assert.InDelta(t, float64(counts[channel.ChannelID])/12, channel.SelectedShare, 1e-9)
		assert.True(t, channel.Unverified)
	}
	assert.InDelta(t, .985, base.Channels[0].TargetShare, 1e-9, "excluded attempts do not rewrite target allocation")
	assert.Zero(t, base.Channels[0].SelectedShare)
	assert.Equal(t, "smart", snapshots[1].Mode)
	assert.EqualValues(t, 1, snapshots[1].SelectionCount)
	assert.Equal(t, "user:42", snapshots[2].Policy)
	assert.EqualValues(t, 1, snapshots[2].SelectionCount)

	controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(59*time.Second))
	after := controller.MonitorSnapshot(now.Add(60 * time.Second))
	require.Len(t, after, 3)
	assert.EqualValues(t, 1, after[0].SelectionCount, "the first second fell out of the 60 second window")
	assert.Zero(t, after[1].SelectionCount)
	assert.Zero(t, after[2].SelectionCount)
	require.Len(t, after[0].History, 60)
	assert.Equal(t, now.Add(time.Second).UnixMilli(), after[0].History[0].At)
	assert.EqualValues(t, 1, after[0].History[58].SelectionCount)
}

func TestPriorityMonitorRecordsPhysicalTrafficAndLeavesAdaptiveRecoveryIndependent(t *testing.T) {
	controller, key, candidates, _, now := discountRoutingFixture(t)
	controller.RecordPriorityDispatch(key, candidates, 1, now)
	controller.RecordPriorityOutcome(key, 1, Sample{ObservedAt: now.Add(time.Second), UpstreamStartedAt: now, Success: true, HasTTFT: true, TTFT: 100 * time.Millisecond})
	controller.RecordPriorityDispatch(key, candidates, 2, now.Add(time.Second))
	controller.RecordPriorityOutcome(key, 2, Sample{ObservedAt: now.Add(3 * time.Second), UpstreamStartedAt: now.Add(time.Second), HardFailure: true})
	controller.RecordChannelStatus(2, 3, "upstream unavailable", now.Add(3*time.Second))
	snapshots := controller.MonitorSnapshot(now.Add(3 * time.Second))
	require.Len(t, snapshots, 1)
	priority := snapshots[0]
	assert.Equal(t, "priority", priority.Mode)
	assert.False(t, priority.Dynamic)
	assert.EqualValues(t, 2, priority.SelectionCount)
	require.Len(t, priority.Requests, 2)
	assert.EqualValues(t, 1, priority.Channels[0].CompletedCount)
	assert.EqualValues(t, 1, priority.Channels[0].SuccessCount)
	require.NotNil(t, priority.Channels[0].LatencyMS)
	assert.Equal(t, float64(1000), *priority.Channels[0].LatencyMS)
	assert.EqualValues(t, 1, priority.Channels[1].FailureCount)
	assert.Equal(t, 3, priority.Channels[1].Status)
	assert.Equal(t, "channel_disabled", priority.Events[0].Kind)
	assert.Zero(t, priority.Channels[0].TargetShare)
	// Passive status and failures do not eject a channel from adaptive routing.
	decision := controller.SelectAvoiding(key, candidates, nil, now.Add(3*time.Second))
	assert.True(t, decision.HasSelection)
	snapshots = controller.MonitorSnapshot(now.Add(3 * time.Second))
	require.Len(t, snapshots, 2)
	assert.Equal(t, "smart", snapshots[1].Mode)
	for _, channel := range snapshots[1].Channels {
		assert.Zero(t, channel.EjectedUntil)
	}
	controller.RecordChannelStatus(2, 1, "", now.Add(4*time.Second))
	assert.Equal(t, "channel_enabled", controller.MonitorSnapshot(now.Add(4 * time.Second))[0].Events[0].Kind)
	assert.Zero(t, controller.MonitorSnapshot(now.Add(64 * time.Second))[0].SelectionCount)
}

func TestMonitorReadsAreDetachedAndNeverAdvanceRoutingOrExpireObservations(t *testing.T) {
	controller, key, candidates, limits, now := discountRoutingFixture(t)
	control, controlKey, controlCandidates, controlLimits, _ := discountRoutingFixture(t)
	for _, current := range []*Controller{controller, control} {
		current.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: now, Success: true, HasTTFT: true, TTFT: time.Second})
	}
	for range 8 {
		expected := control.SelectDiscountAvoiding(controlKey, controlCandidates, nil, controlLimits, now)
		actual := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now)
		require.Equal(t, expected, actual)
		snapshot := controller.MonitorSnapshot(now)
		require.Len(t, snapshot, 1)
		require.NotEmpty(t, snapshot[0].Events)
		snapshot[0].Channels[0].TargetShare = 99
		*snapshot[0].Channels[0].EffectiveCostDiscount = 99
		snapshot[0].Events[0].Allocations[0].Share = 99
		snapshot[0].History[len(snapshot[0].History)-1].Channels[0].SelectionCount = 99
		fresh := controller.MonitorSnapshot(now)
		assert.LessOrEqual(t, fresh[0].Channels[0].TargetShare, 1.0)
		assert.Equal(t, .5, *fresh[0].Channels[0].EffectiveCostDiscount)
		assert.LessOrEqual(t, fresh[0].Events[0].Allocations[0].Share, 1.0)
		assert.LessOrEqual(t, fresh[0].History[len(fresh[0].History)-1].Channels[0].SelectionCount, int64(8))
	}
	assert.Empty(t, controller.MonitorSnapshot(now.Add(10*time.Minute)))
	assert.Equal(t, control.routes, controller.routes)
	assert.Equal(t, control.selections, controller.selections)
	assert.Equal(t, control.observations, controller.observations, "monitoring must not prune live controller observations")
}

func TestMonitorRecordsActualQualityTransitionsWithoutRepeatingUnchangedEvents(t *testing.T) {
	controller, key, candidates, limits, now := discountRoutingFixture(t)
	for _, candidate := range candidates {
		for range 3 {
			controller.Observe(ObservationKey{ChannelID: candidate.ChannelID, Model: key.Model}, Sample{ObservedAt: now, Success: true, HasTTFT: true, TTFT: time.Second, HasTPOT: true, TPOT: 40 * time.Millisecond})
		}
	}
	controller.SelectDiscountAvoiding(key, candidates, nil, limits, now)
	initial := controller.MonitorSnapshot(now)
	require.Len(t, initial, 1)
	assert.Equal(t, "initialized", initial[0].Events[0].Kind)
	assert.Equal(t, float64(1000), *initial[0].Channels[0].TTFTMS)
	assert.Equal(t, float64(40), *initial[0].Channels[0].TPOTMS)
	assert.False(t, initial[0].Channels[0].Unverified)
	for range 3 {
		controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: now.Add(time.Second), Success: true, HasTTFT: true, TTFT: 10 * time.Second})
	}
	controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(4*time.Second))
	degraded := controller.MonitorSnapshot(now.Add(4 * time.Second))[0]
	assert.True(t, degraded.Channels[0].Degraded)
	assert.Equal(t, "degraded", degraded.Events[0].Kind)
	assert.Equal(t, []int{1}, degraded.Events[0].ChannelIDs)
	controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(4*time.Second))
	assert.Len(t, controller.MonitorSnapshot(now.Add(4 * time.Second))[0].Events, len(degraded.Events))
	for range 3 {
		controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: now.Add(5 * time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
	}
	controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(8*time.Second))
	recovered := controller.MonitorSnapshot(now.Add(8 * time.Second))[0]
	assert.False(t, recovered.Channels[0].Degraded)
	assert.Equal(t, "recovery", recovered.Events[0].Kind)
	assert.Greater(t, recovered.Channels[0].TargetShare, degraded.Channels[0].TargetShare)
	for range 3 {
		controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: now.Add(9 * time.Second), HardFailure: true})
	}
	controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(10*time.Second))
	failed := controller.MonitorSnapshot(now.Add(10 * time.Second))[0]
	assert.Equal(t, "failure", failed.Events[0].Kind)
	assert.Equal(t, []int{1}, failed.Events[0].ChannelIDs)
	assert.Equal(t, now.Add(39*time.Second).UnixMilli(), failed.Channels[0].EjectedUntil)
	assert.Zero(t, failed.Channels[0].TargetShare)
	assert.Nil(t, failed.Channels[0].TTFTMS, "ejected performance samples no longer describe current health")
}

func TestMonitorCandidateChangesKeepPastCountsAndBoundRoutesAndChannels(t *testing.T) {
	controller, key, candidates, limits, now := discountRoutingFixture(t)
	controller.SelectDiscountAvoiding(key, candidates, nil, limits, now)
	controller.SelectDiscountAvoiding(key, candidates[1:], nil, limits, now.Add(time.Second))
	snapshot := controller.MonitorSnapshot(now.Add(time.Second))[0]
	require.Len(t, snapshot.Channels, 3)
	assert.False(t, snapshot.Channels[0].Active)
	assert.Zero(t, snapshot.Channels[0].TargetShare)
	assert.EqualValues(t, 1, snapshot.Channels[0].SelectionCount)
	assert.Equal(t, "candidates_changed", snapshot.Events[0].Kind)
	assert.EqualValues(t, 2, snapshot.SelectionCount)
	for i := range 22 {
		changed := []Candidate{{ChannelID: 1, Weight: uint(i + 1)}}
		controller.SelectDiscountAvoiding(key, changed, nil, limits, now.Add(time.Duration(i+2)*time.Second))
	}
	events := controller.MonitorSnapshot(now.Add(24 * time.Second))[0].Events
	assert.Len(t, events, 20, "only recent transition events are retained")
	assert.Equal(t, "candidates_changed", events[0].Kind)

	bounded, err := NewController(Config{Enabled: true, MaxSamples: 6, MaxAge: 90 * time.Second, ProbeFraction: .015})
	require.NoError(t, err)
	for i := range 513 {
		bounded.Select(RouteKey{Group: fmt.Sprintf("group-%03d", i), Model: "model"}, candidates, now.Add(time.Duration(i)*time.Millisecond))
	}
	routes := bounded.MonitorSnapshot(now.Add(time.Second))
	require.Len(t, routes, 512, "inactive telemetry is evicted instead of allowing unbounded route history")
	assert.Equal(t, "group-001", routes[0].Group)
	assert.Equal(t, "group-512", routes[len(routes)-1].Group)

	many := make([]Candidate, 129)
	for i := range many {
		many[i] = Candidate{ChannelID: i + 1}
	}
	bounded.Select(RouteKey{Group: "many", Model: "model"}, many, now.Add(2*time.Second))
	routes = bounded.MonitorSnapshot(now.Add(2 * time.Second))
	last := routes[len(routes)-1]
	assert.Equal(t, "many", last.Group)
	assert.True(t, last.Truncated)
	assert.Len(t, last.Channels, 128)
	assert.EqualValues(t, 1, last.SelectionCount)
	excluded := make(map[int]struct{}, 128)
	for id := range 128 {
		excluded[id+1] = struct{}{}
	}
	decision := bounded.SelectAvoiding(RouteKey{Group: "many", Model: "model"}, many, excluded, now.Add(2*time.Second))
	assert.Equal(t, 129, decision.SelectedChannelID)
	bounded.Select(RouteKey{Group: "many", Model: "model"}, many[:1], now.Add(3*time.Second))
	routes = bounded.MonitorSnapshot(now.Add(3 * time.Second))
	last = routes[len(routes)-1]
	assert.EqualValues(t, 3, last.SelectionCount, "the total includes selections beyond the row limit")
	assert.True(t, last.Truncated, "partial historical counts remain flagged after candidate count shrinks")
	assert.EqualValues(t, 2, last.Channels[0].SelectionCount)
	assert.InDelta(t, 2.0/3, last.Channels[0].SelectedShare, 1e-9)
}
