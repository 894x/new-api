package dynamicrouting

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigRejectsImpossibleCountsAndNonFiniteFloats(t *testing.T) {
	base := Config{
		Enabled:              true,
		MaxSamples:           10,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	}

	tests := map[string]func(*Config){
		"min samples exceed max":   func(config *Config) { config.MinSamples = 11 },
		"hard failures exceed max": func(config *Config) { config.HardFailureThreshold = 11 },
		"probe NaN":                func(config *Config) { config.ProbeFraction = math.NaN() },
		"degradation infinity":     func(config *Config) { config.DegradationThreshold = math.Inf(1) },
		"recovery NaN":             func(config *Config) { config.RecoveryThreshold = math.NaN() },
		"critical infinity":        func(config *Config) { config.CriticalThreshold = math.Inf(1) },
		"advantage NaN":            func(config *Config) { config.CandidateAdvantage = math.NaN() },
		"aggressiveness infinity":  func(config *Config) { config.Aggressiveness = math.Inf(1) },
		"recovery step NaN":        func(config *Config) { config.RecoveryStep = math.NaN() },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			config := base
			mutate(&config)
			_, err := NewController(config)
			require.Error(t, err)
		})
	}
}

func TestEnabledControllerRejectsZeroProbeFraction(t *testing.T) {
	enabled := Config{
		Enabled:    true,
		MaxSamples: 10,
		MaxAge:     time.Hour,
	}
	_, err := NewController(enabled)
	require.Error(t, err)

	disabled := enabled
	disabled.Enabled = false
	controller, err := NewController(disabled)
	require.NoError(t, err)
	require.Error(t, controller.UpdateConfig(enabled))
	require.NoError(t, controller.UpdateConfig(disabled))
}

func TestObservationWindowHonorsCountAndAge(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    2,
		MaxAge:        10 * time.Minute,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	key := ObservationKey{ChannelID: 7, Model: "chat-model"}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for offset, ttft := range []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond} {
		controller.Observe(key, Sample{
			ObservedAt: base.Add(time.Duration(offset) * time.Minute),
			TTFT:       ttft,
			HasTTFT:    true,
			Success:    true,
		})
	}

	stats := controller.ObservationStats(key, base.Add(2*time.Minute))
	assert.Equal(t, 2, stats.SampleCount)
	assert.True(t, stats.HasTTFT)
	assert.Equal(t, 250*time.Millisecond, stats.TTFT)

	stats = controller.ObservationStats(key, base.Add(13*time.Minute))
	assert.Zero(t, stats.SampleCount)
	assert.False(t, stats.HasTTFT)
}

func TestObservationWindowUsesObservedAtWhenCompletionsArriveOutOfOrder(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    2,
		MaxAge:        10 * time.Minute,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	key := ObservationKey{ChannelID: 9, Model: "chat-model"}
	now := time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)
	controller.Observe(key, Sample{ObservedAt: now, TTFT: 300 * time.Millisecond, HasTTFT: true, Success: true})
	controller.Observe(key, Sample{ObservedAt: now.Add(-time.Hour), TTFT: time.Millisecond, HasTTFT: true, Success: true})
	controller.Observe(key, Sample{ObservedAt: now.Add(-time.Minute), TTFT: 200 * time.Millisecond, HasTTFT: true, Success: true})

	stats := controller.ObservationStats(key, now)
	assert.Equal(t, 2, stats.SampleCount)
	assert.Equal(t, 250*time.Millisecond, stats.TTFT)
}

func TestObservationStatsIgnoreMissingMetricsInsteadOfTreatingThemAsZero(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    10,
		MaxAge:        time.Hour,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	key := ObservationKey{ChannelID: 8, Model: "chat-model"}
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Observe(key, Sample{
		ObservedAt: now,
		TTFT:       800 * time.Millisecond,
		HasTTFT:    true,
		Success:    true,
	})
	controller.Observe(key, Sample{
		ObservedAt: now,
		TPOT:       40 * time.Millisecond,
		HasTPOT:    true,
		Success:    true,
	})

	stats := controller.ObservationStats(key, now)
	assert.Equal(t, 1, stats.TTFTSampleCount)
	assert.Equal(t, 1, stats.TPOTSampleCount)
	assert.Equal(t, 800*time.Millisecond, stats.TTFT)
	assert.Equal(t, 40*time.Millisecond, stats.TPOT)
}

func TestHealthOnlySuccessDoesNotEnterOrEvictPerformanceWindow(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    2,
		MaxAge:        time.Hour,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	key := ObservationKey{ChannelID: 8, Model: "chat-model"}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index, ttft := range []time.Duration{100 * time.Millisecond, 200 * time.Millisecond} {
		controller.Observe(key, Sample{
			ObservedAt: base.Add(time.Duration(index) * time.Second),
			TTFT:       ttft,
			HasTTFT:    true,
			Success:    true,
		})
	}
	for offset := 2; offset < 12; offset++ {
		controller.Observe(key, Sample{
			ObservedAt:        base.Add(time.Duration(offset) * time.Second),
			UpstreamStartedAt: base.Add(time.Duration(offset)*time.Second - time.Millisecond),
			Success:           true,
		})
	}

	stats := controller.ObservationStats(key, base.Add(12*time.Second))
	assert.Equal(t, 2, stats.SampleCount)
	assert.Equal(t, 2, stats.TTFTSampleCount)
	assert.Equal(t, 150*time.Millisecond, stats.TTFT)

	controller.Observe(key, Sample{
		ObservedAt: base.Add(13 * time.Second),
		TTFT:       300 * time.Millisecond,
		HasTTFT:    true,
		Success:    true,
	})
	stats = controller.ObservationStats(key, base.Add(13*time.Second))
	assert.Equal(t, 2, stats.SampleCount)
	assert.Equal(t, 2, stats.TTFTSampleCount)
	assert.Equal(t, 250*time.Millisecond, stats.TTFT)
}

func TestVerificationRequiresMinSamplesForAtLeastOneActualMetric(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    20,
		MaxAge:        time.Hour,
		MinSamples:    3,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt: base,
		TTFT:       100 * time.Millisecond,
		HasTTFT:    true,
		Success:    true,
	})
	for index := 1; index <= 3; index++ {
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: base.Add(time.Duration(index) * time.Second),
			Success:    true,
		})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{
			ObservedAt: base.Add(time.Duration(index) * time.Second),
			TPOT:       25 * time.Millisecond,
			HasTPOT:    true,
			Success:    true,
		})
	}

	decision := controller.Select(RouteKey{Group: "default", Model: model}, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}, base.Add(5*time.Second))

	assert.Equal(t, []int{1}, decision.UnverifiedCandidates)
}

func TestColdStartKeepsMainTrafficOnHighestPriorityAndProbesLowerPriority(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    50,
		MaxAge:        time.Hour,
		MinSamples:    3,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	decision := controller.Select(
		RouteKey{Group: "default", Model: "chat-model"},
		[]Candidate{
			{ChannelID: 1, Priority: 100, Weight: 100},
			{ChannelID: 2, Priority: 0, Weight: 100},
		},
		time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
	)

	require.Len(t, decision.Allocations, 2)
	assert.True(t, decision.Dynamic)
	assert.InDelta(t, 0.95, decision.Allocations[0].Share, 0.0001)
	assert.False(t, decision.Allocations[0].Probe)
	assert.InDelta(t, 0.05, decision.Allocations[1].Share, 0.0001)
	assert.True(t, decision.Allocations[1].Probe)
}

func TestProbeDebtForcesLowerPriorityExplorationWithinFiniteRequests(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    50,
		MaxAge:        time.Hour,
		MinSamples:    3,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	key := RouteKey{Group: "default", Model: "chat-model"}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	selectedLower := 0
	for request := 0; request < 20; request++ {
		decision := controller.Select(key, candidates, base.Add(time.Duration(request)*time.Millisecond))
		if decision.SelectedChannelID == 2 {
			selectedLower++
		}
	}

	assert.GreaterOrEqual(t, selectedLower, 1)
}

func TestSelectAvoidingChargesWeightedFairDebtToTheChannelActuallySelected(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           1,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  time.Minute,
	})
	require.NoError(t, err)

	model := "retry-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 10, Weight: 100},
		{ChannelID: 3, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base,
		HardFailure: true,
	})

	first := controller.Select(key, candidates, base)
	require.True(t, first.HasSelection)
	assert.Equal(t, 2, first.SelectedChannelID)

	retry := controller.SelectAvoiding(key, candidates, map[int]struct{}{2: {}}, base.Add(time.Millisecond))
	require.True(t, retry.HasSelection)
	assert.Equal(t, 3, retry.SelectedChannelID)
	require.Len(t, retry.Allocations, 2)
	assert.Equal(t, 2, retry.Allocations[0].ChannelID)
	assert.Equal(t, 3, retry.Allocations[1].ChannelID)

	// Channel 3 already received the retry. Weighted-fair accounting must debit
	// that actual selection, so it is not selected again as if the retry had
	// gone to the excluded channel 2.
	for request := 0; request < 20; request++ {
		decision := controller.Select(key, candidates, base.Add(time.Duration(request+2)*time.Millisecond))
		require.True(t, decision.HasSelection)
		assert.Equal(t, 2, decision.SelectedChannelID, "request %d", request)
	}
}

func TestSelectAvoidingDoesNotClearForcedProbeDebtUntilProbeIsSelected(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           1,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "retry-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base,
		HardFailure: true,
	})
	controller.Select(key, candidates, base)

	for request := 0; request < 20; request++ {
		decision := controller.SelectAvoiding(
			key,
			candidates,
			map[int]struct{}{1: {}},
			base.Add(31*time.Second+time.Duration(request)*time.Millisecond),
		)
		require.True(t, decision.HasSelection)
		assert.Equal(t, 2, decision.SelectedChannelID)
	}

	ejection := controller.routes[key].ejections[1]
	assert.GreaterOrEqual(t, ejection.probeDebt, 1.0)

	probe := controller.Select(key, candidates, base.Add(32*time.Second))
	require.True(t, probe.HasSelection)
	assert.Equal(t, 1, probe.SelectedChannelID)
	assert.Zero(t, controller.routes[key].ejections[1].probeDebt)
}

func TestSelectAvoidingAllCandidatesDoesNotChangeWeightedFairSequence(t *testing.T) {
	config := Config{
		Enabled:       true,
		MaxSamples:    20,
		MaxAge:        time.Hour,
		MinSamples:    1,
		ProbeFraction: 0.2,
	}
	withNoOp, err := NewController(config)
	require.NoError(t, err)
	withoutNoOp, err := NewController(config)
	require.NoError(t, err)

	key := RouteKey{Group: "default", Model: "retry-model"}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	noOp := withNoOp.SelectAvoiding(
		key,
		candidates,
		map[int]struct{}{1: {}, 2: {}},
		base,
	)
	assert.False(t, noOp.HasSelection)
	require.Len(t, noOp.Allocations, 2)

	for request := 0; request < 10; request++ {
		now := base.Add(time.Duration(request+1) * time.Millisecond)
		withDecision := withNoOp.Select(key, candidates, now)
		withoutDecision := withoutNoOp.Select(key, candidates, now)
		require.True(t, withDecision.HasSelection)
		require.True(t, withoutDecision.HasSelection)
		assert.Equal(t, withoutDecision.SelectedChannelID, withDecision.SelectedChannelID, "request %d", request)
	}
}

func TestSelectAvoidingAllCandidatesDoesNotAdvanceForcedProbeDebt(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           1,
		ProbeFraction:        0.2,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "retry-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base,
		HardFailure: true,
	})
	controller.Select(key, candidates, base)

	noOp := controller.SelectAvoiding(
		key,
		candidates,
		map[int]struct{}{1: {}, 2: {}},
		base.Add(31*time.Second),
	)
	assert.False(t, noOp.HasSelection)
	assert.Zero(t, controller.routes[key].ejections[1].probeDebt)
}

func TestProbeFeedbackVerifiesLowerPriorityThenEscapesDegradedPrimary(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           50,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	lastDecision := Decision{}
	for request := 0; request < 60; request++ {
		now := base.Add(time.Duration(request) * time.Second)
		lastDecision = controller.Select(key, candidates, now)
		require.True(t, lastDecision.HasSelection)
		sample := Sample{ObservedAt: now, HasTTFT: true, HasTPOT: true, Success: true}
		if lastDecision.SelectedChannelID == 1 {
			sample.TTFT = 100 * time.Millisecond
			sample.TPOT = 20 * time.Millisecond
		} else {
			sample.TTFT = 120 * time.Millisecond
			sample.TPOT = 25 * time.Millisecond
		}
		controller.Observe(ObservationKey{ChannelID: lastDecision.SelectedChannelID, Model: model}, sample)
	}

	lowerStats := controller.ObservationStats(ObservationKey{ChannelID: 2, Model: model}, base.Add(60*time.Second))
	assert.GreaterOrEqual(t, lowerStats.SampleCount, 3)
	require.Len(t, lastDecision.Allocations, 2)
	assert.True(t, lastDecision.Allocations[1].Probe)

	for request := 0; request < 5; request++ {
		now := base.Add(time.Duration(60+request) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: now,
			TTFT:       400 * time.Millisecond,
			TPOT:       80 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
	}

	shifted := controller.Select(key, candidates, base.Add(66*time.Second))
	require.Len(t, shifted.Allocations, 2)
	assert.Less(t, shifted.Allocations[0].Share, 0.20)
	assert.Greater(t, shifted.Allocations[1].Share, 0.80)
	assert.False(t, shifted.Allocations[1].Probe)
	assert.Equal(t, 2, shifted.SelectedChannelID)
}

func TestSlowerFallbackKeepsProbeFloorUntilPrimaryDegrades(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           50,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	lastDecision := Decision{}
	for request := 0; request < 60; request++ {
		now := base.Add(time.Duration(request) * time.Second)
		lastDecision = controller.Select(key, candidates, now)
		sample := Sample{ObservedAt: now, HasTTFT: true, HasTPOT: true, Success: true}
		if lastDecision.SelectedChannelID == 1 {
			sample.TTFT = 250 * time.Millisecond
			sample.TPOT = 25 * time.Millisecond
		} else {
			sample.TTFT = 400 * time.Millisecond
			sample.TPOT = 35 * time.Millisecond
		}
		controller.Observe(ObservationKey{ChannelID: lastDecision.SelectedChannelID, Model: model}, sample)
	}

	require.Len(t, lastDecision.Allocations, 2)
	assert.GreaterOrEqual(t, lastDecision.Allocations[1].Share, 0.05-0.0001)
	assert.True(t, lastDecision.Allocations[1].Probe)
	assert.GreaterOrEqual(t, controller.ObservationStats(ObservationKey{ChannelID: 2, Model: model}, base.Add(60*time.Second)).SampleCount, 3)

	for request := 0; request < 5; request++ {
		now := base.Add(time.Duration(60+request) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: now,
			TTFT:       1800 * time.Millisecond,
			TPOT:       100 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
	}
	shifted := controller.Select(key, candidates, base.Add(66*time.Second))
	assert.Greater(t, shifted.Allocations[1].Share, 0.80)
	assert.False(t, shifted.Allocations[1].Probe)
	assert.Equal(t, 2, shifted.SelectedChannelID)
}

func TestDisabledControllerFallsBackToStaticPriorityAndWeight(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       false,
		MaxSamples:    50,
		MaxAge:        time.Hour,
		ProbeFraction: 0.10,
	})
	require.NoError(t, err)

	decision := controller.Select(
		RouteKey{Group: "default", Model: "chat-model"},
		[]Candidate{
			{ChannelID: 1, Priority: 100, Weight: 3},
			{ChannelID: 2, Priority: 100, Weight: 1},
			{ChannelID: 3, Priority: 0, Weight: 100},
		},
		time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
	)

	assert.False(t, decision.Dynamic)
	require.Len(t, decision.Allocations, 2)
	assert.Equal(t, 1, decision.Allocations[0].ChannelID)
	assert.InDelta(t, 13.0/24.0, decision.Allocations[0].Share, 0.0001)
	assert.Equal(t, 2, decision.Allocations[1].ChannelID)
	assert.InDelta(t, 11.0/24.0, decision.Allocations[1].Share, 0.0001)
}

func TestMaximumCandidateWeightDoesNotOverflowWeightPlusTen(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:    false,
		MaxSamples: 20,
		MaxAge:     time.Hour,
	})
	require.NoError(t, err)

	decision := controller.Select(RouteKey{Group: "default", Model: "chat-model"}, []Candidate{
		{ChannelID: 1, Priority: math.MaxInt64, Weight: ^uint(0)},
		{ChannelID: 2, Priority: math.MaxInt64, Weight: 0},
	}, time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))

	require.Len(t, decision.Allocations, 2)
	assert.False(t, math.IsNaN(decision.Allocations[0].Share))
	assert.False(t, math.IsInf(decision.Allocations[0].Share, 0))
	assert.Greater(t, decision.Allocations[0].Share, 0.99)
	assert.InDelta(t, 1, decision.Allocations[0].Share+decision.Allocations[1].Share, 0.0001)
}

func TestGradualDegradationMovesTrafficToVerifiedLowerPriorityCandidate(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		CriticalThreshold:    2.0,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		Cooldown:             10 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: observedAt,
			TTFT:       100 * time.Millisecond,
			TPOT:       20 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{
			ObservedAt: observedAt,
			TTFT:       120 * time.Millisecond,
			TPOT:       25 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
	}

	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	initial := controller.Select(key, candidates, base.Add(3*time.Second))
	assert.InDelta(t, 0.95, initial.Allocations[0].Share, 0.0001)

	for index := 0; index < 3; index++ {
		observedAt := base.Add(20*time.Second + time.Duration(index)*time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: observedAt,
			TTFT:       240 * time.Millisecond,
			TPOT:       50 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{
			ObservedAt: observedAt,
			TTFT:       120 * time.Millisecond,
			TPOT:       25 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
	}

	degraded := controller.Select(key, candidates, base.Add(30*time.Second))
	require.Len(t, degraded.Allocations, 2)
	assert.Less(t, degraded.Allocations[0].Share, 0.20)
	assert.Greater(t, degraded.Allocations[1].Share, 0.80)
	assert.False(t, degraded.Allocations[1].Probe)
}

func TestHardFailureBypassesCooldownAndImmediatelyEjectsChannel(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.3,
		RecoveryStep:         0.05,
		Cooldown:             time.Minute,
		HardFailureThreshold: 1,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		for _, channelID := range []int{1, 2} {
			controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
				ObservedAt: observedAt,
				TTFT:       100 * time.Millisecond,
				TPOT:       20 * time.Millisecond,
				HasTTFT:    true,
				HasTPOT:    true,
				Success:    true,
			})
		}
	}
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	controller.Select(key, candidates, base.Add(10*time.Second))

	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(11 * time.Second),
		HardFailure: true,
	})
	decision := controller.Select(key, candidates, base.Add(12*time.Second))

	require.Len(t, decision.Allocations, 1)
	assert.Equal(t, 2, decision.Allocations[0].ChannelID)
	assert.InDelta(t, 1, decision.Allocations[0].Share, 0.0001)
	assert.False(t, decision.Allocations[0].Probe)
}

func TestNeutralSamplesDoNotBreakHardFailureStreakButSuccessDoes(t *testing.T) {
	tests := []struct {
		name          string
		middle        Sample
		expectEjected bool
	}{
		{
			name:          "neutral sample is ignored",
			middle:        Sample{},
			expectEjected: true,
		},
		{
			name: "health-only successful sample resets streak",
			middle: Sample{
				Success: true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller, err := NewController(Config{
				Enabled:              true,
				MaxSamples:           10,
				MaxAge:               time.Hour,
				MinSamples:           1,
				ProbeFraction:        0.05,
				DegradationThreshold: 1.4,
				RecoveryThreshold:    1.15,
				CriticalThreshold:    2,
				CandidateAdvantage:   1.1,
				Aggressiveness:       0.8,
				RecoveryStep:         0.05,
				HardFailureThreshold: 2,
				HardFailureCooldown:  30 * time.Second,
			})
			require.NoError(t, err)

			model := "chat-model"
			base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
			for _, channelID := range []int{1, 2} {
				controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
					ObservedAt: base,
					TTFT:       100 * time.Millisecond,
					HasTTFT:    true,
					Success:    true,
				})
			}
			controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
				ObservedAt:  base.Add(10 * time.Second),
				HardFailure: true,
			})
			middle := test.middle
			middle.ObservedAt = base.Add(11 * time.Second)
			controller.Observe(ObservationKey{ChannelID: 1, Model: model}, middle)
			controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
				ObservedAt:  base.Add(12 * time.Second),
				HardFailure: true,
			})

			decision := controller.Select(RouteKey{Group: "default", Model: model}, []Candidate{
				{ChannelID: 1, Priority: 100, Weight: 100},
				{ChannelID: 2, Priority: 0, Weight: 100},
			}, base.Add(12*time.Second))
			if test.expectEjected {
				require.Len(t, decision.Allocations, 1)
				assert.Equal(t, 2, decision.Allocations[0].ChannelID)
				assert.Equal(t, []int{1}, decision.EmergencyCandidates)
				return
			}
			require.Len(t, decision.Allocations, 2)
			assert.Equal(t, 1, decision.Allocations[0].ChannelID)
			assert.InDelta(t, 0.95, decision.Allocations[0].Share, 0.0001)
			assert.Empty(t, decision.EmergencyCandidates)
		})
	}
}

func TestHardFailureEscapesToHighestStaticCandidateEvenWhenUnverified(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		Cooldown:             time.Minute,
		HardFailureThreshold: 1,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: base.Add(time.Duration(index) * time.Second),
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	controller.Select(key, candidates, base.Add(10*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(11 * time.Second),
		HardFailure: true,
	})

	decision := controller.Select(key, candidates, base.Add(12*time.Second))
	require.Len(t, decision.Allocations, 1)
	assert.Equal(t, 2, decision.Allocations[0].ChannelID)
	assert.InDelta(t, 1, decision.Allocations[0].Share, 0.0001)
	assert.True(t, decision.HasSelection)
	assert.Equal(t, 2, decision.SelectedChannelID)
}

func TestAllHardFailedCandidatesRetainAStaticFallbackSelection(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    20,
		MaxAge:        time.Hour,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for _, channelID := range []int{1, 2} {
		controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{ObservedAt: now, HardFailure: true})
	}
	decision := controller.Select(RouteKey{Group: "default", Model: model}, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}, now)

	assert.True(t, decision.HasSelection)
	assert.Equal(t, 1, decision.SelectedChannelID)
	require.Len(t, decision.Allocations, 2)
	assert.InDelta(t, 1, decision.Allocations[0].Share+decision.Allocations[1].Share, 0.0001)
}

func TestHardFailureDistributesTrafficAcrossMultipleHealthyCandidates(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		for _, channelID := range []int{1, 2, 3} {
			controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
				ObservedAt: observedAt,
				TTFT:       100 * time.Millisecond,
				HasTTFT:    true,
				Success:    true,
			})
		}
	}
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
		{ChannelID: 3, Priority: 0, Weight: 200},
	}
	controller.Select(key, candidates, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{ObservedAt: base.Add(6 * time.Second), HardFailure: true})
	decision := controller.Select(key, candidates, base.Add(7*time.Second))

	require.Len(t, decision.Allocations, 2)
	assert.Equal(t, 2, decision.Allocations[0].ChannelID)
	assert.Equal(t, 3, decision.Allocations[1].ChannelID)
	assert.Greater(t, decision.Allocations[0].Share, 0.30)
	assert.Greater(t, decision.Allocations[1].Share, 0.60)
	assert.InDelta(t, 1, decision.Allocations[0].Share+decision.Allocations[1].Share, 0.0001)
}

func TestHardFailureCooldownReprobesAndRecoversSlowlyAfterValidSuccess(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.10,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		for _, channelID := range []int{1, 2} {
			controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
				ObservedAt: observedAt,
				TTFT:       100 * time.Millisecond,
				TPOT:       20 * time.Millisecond,
				HasTTFT:    true,
				HasTPOT:    true,
				Success:    true,
			})
		}
	}
	controller.Select(key, candidates, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(10 * time.Second),
		HardFailure: true,
	})

	ejected := controller.Select(key, candidates, base.Add(10*time.Second))
	require.Len(t, ejected.Allocations, 1)
	assert.Equal(t, 2, ejected.SelectedChannelID)
	withinCooldown := controller.Select(key, candidates, base.Add(39*time.Second))
	require.Len(t, withinCooldown.Allocations, 1)
	assert.Equal(t, 2, withinCooldown.SelectedChannelID)

	probeSelectedAt := time.Time{}
	for request := 0; request < 20; request++ {
		now := base.Add(40*time.Second + time.Duration(request)*time.Millisecond)
		decision := controller.Select(key, candidates, now)
		require.Len(t, decision.Allocations, 2)
		assert.InDelta(t, 0.05, decision.Allocations[0].Share, 0.0001)
		assert.True(t, decision.Allocations[0].Probe)
		if decision.SelectedChannelID == 1 {
			probeSelectedAt = now
			break
		}
	}
	require.False(t, probeSelectedAt.IsZero())

	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt: probeSelectedAt.Add(time.Millisecond),
		Success:    true,
	})
	missingMetric := controller.Select(key, candidates, probeSelectedAt.Add(2*time.Millisecond))
	require.Len(t, missingMetric.Allocations, 2)
	assert.InDelta(t, 0.05, missingMetric.Allocations[0].Share, 0.0001)
	assert.True(t, missingMetric.Allocations[0].Probe)

	for offset := 3; offset <= 5; offset++ {
		observedAt := probeSelectedAt.Add(time.Duration(offset) * time.Millisecond)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt:        observedAt,
			UpstreamStartedAt: observedAt.Add(-time.Millisecond),
			TTFT:              100 * time.Millisecond,
			TPOT:              20 * time.Millisecond,
			HasTTFT:           true,
			HasTPOT:           true,
			Success:           true,
		})
	}
	recovered := controller.Select(key, candidates, probeSelectedAt.Add(6*time.Millisecond))
	require.Len(t, recovered.Allocations, 2)
	assert.Greater(t, recovered.Allocations[0].Share, 0.05)
	assert.LessOrEqual(t, recovered.Allocations[0].Share, 0.1501)
	assert.False(t, recovered.Allocations[0].Probe)
}

func TestHardFailureRecoveryIgnoresInFlightSuccessAndRequiresPostCooldownStreak(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.10,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		for _, channelID := range []int{1, 2} {
			controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
				ObservedAt: base.Add(time.Duration(index) * time.Second),
				TTFT:       100 * time.Millisecond,
				HasTTFT:    true,
				Success:    true,
			})
		}
	}
	controller.Select(key, candidates, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(10 * time.Second),
		HardFailure: true,
	})
	controller.Select(key, candidates, base.Add(10*time.Second))

	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt: base.Add(20 * time.Second),
		TTFT:       100 * time.Millisecond,
		HasTTFT:    true,
		Success:    true,
	})
	afterCooldown := controller.Select(key, candidates, base.Add(41*time.Second))
	require.Len(t, afterCooldown.Allocations, 2)
	assert.InDelta(t, 0.05, afterCooldown.Allocations[0].Share, 0.0001)
	assert.True(t, afterCooldown.Allocations[0].Probe)

	for _, offset := range []time.Duration{42, 43} {
		observedAt := base.Add(offset * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt:        observedAt,
			UpstreamStartedAt: observedAt.Add(-time.Second),
			TTFT:              100 * time.Millisecond,
			HasTTFT:           true,
			Success:           true,
		})
	}
	beforeStreak := controller.Select(key, candidates, base.Add(44*time.Second))
	require.Len(t, beforeStreak.Allocations, 2)
	assert.InDelta(t, 0.05, beforeStreak.Allocations[0].Share, 0.0001)
	assert.True(t, beforeStreak.Allocations[0].Probe)

	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:        base.Add(45 * time.Second),
		UpstreamStartedAt: base.Add(44 * time.Second),
		TTFT:              100 * time.Millisecond,
		HasTTFT:           true,
		Success:           true,
	})
	recovered := controller.Select(key, candidates, base.Add(46*time.Second))
	require.Len(t, recovered.Allocations, 2)
	assert.Greater(t, recovered.Allocations[0].Share, 0.05)
	assert.LessOrEqual(t, recovered.Allocations[0].Share, 0.1501)
	assert.False(t, recovered.Allocations[0].Probe)
}

func TestHardFailureRecoveryRequiresRequestsStartedAfterCooldown(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           30,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.10,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		for _, channelID := range []int{1, 2} {
			controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
				ObservedAt: base.Add(time.Duration(index) * time.Second),
				TTFT:       100 * time.Millisecond,
				HasTTFT:    true,
				Success:    true,
			})
		}
	}
	controller.Select(key, candidates, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(10 * time.Second),
		HardFailure: true,
	})
	controller.Select(key, candidates, base.Add(10*time.Second))

	for offset := 41; offset <= 43; offset++ {
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt:        base.Add(time.Duration(offset) * time.Second),
			UpstreamStartedAt: base.Add(time.Duration(offset-25) * time.Second),
			TTFT:              100 * time.Millisecond,
			HasTTFT:           true,
			Success:           true,
		})
	}
	completedAfterButStartedBefore := controller.Select(key, candidates, base.Add(44*time.Second))
	require.Len(t, completedAfterButStartedBefore.Allocations, 2)
	assert.InDelta(t, 0.05, completedAfterButStartedBefore.Allocations[0].Share, 0.0001)
	assert.True(t, completedAfterButStartedBefore.Allocations[0].Probe)

	for offset := 45; offset <= 47; offset++ {
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: base.Add(time.Duration(offset) * time.Second),
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	missingStart := controller.Select(key, candidates, base.Add(48*time.Second))
	require.Len(t, missingStart.Allocations, 2)
	assert.InDelta(t, 0.05, missingStart.Allocations[0].Share, 0.0001)
	assert.True(t, missingStart.Allocations[0].Probe)

	for offset := 49; offset <= 51; offset++ {
		observedAt := base.Add(time.Duration(offset) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt:        observedAt,
			UpstreamStartedAt: observedAt.Add(-100 * time.Millisecond),
			TTFT:              100 * time.Millisecond,
			HasTTFT:           true,
			Success:           true,
		})
	}
	recovered := controller.Select(key, candidates, base.Add(52*time.Second))
	require.Len(t, recovered.Allocations, 2)
	assert.Greater(t, recovered.Allocations[0].Share, 0.05)
	assert.False(t, recovered.Allocations[0].Probe)
}

func TestEjectedChannelRecoversFromControllerSelectedHealthOnlyProbes(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           2,
		ProbeFraction:        0.10,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.10,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "tool-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 2; index++ {
		for _, channelID := range []int{1, 2} {
			controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
				ObservedAt: base.Add(time.Duration(index) * time.Second),
				TTFT:       100 * time.Millisecond,
				HasTTFT:    true,
				Success:    true,
			})
		}
	}
	controller.Select(key, candidates, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(10 * time.Second),
		HardFailure: true,
	})
	controller.Select(key, candidates, base.Add(10*time.Second))

	now := base.Add(40 * time.Second)
	for successfulProbe := 0; successfulProbe < 2; successfulProbe++ {
		selected := false
		for request := 0; request < 20; request++ {
			now = now.Add(time.Millisecond)
			decision := controller.Select(key, candidates, now)
			if decision.SelectedChannelID != 1 {
				continue
			}
			selected = true
			require.Len(t, decision.Allocations, 2)
			assert.InDelta(t, 0.10, decision.Allocations[0].Share, 0.0001)
			assert.True(t, decision.Allocations[0].Probe)
			controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
				ObservedAt:        now.Add(time.Millisecond),
				UpstreamStartedAt: now.Add(100 * time.Microsecond),
				Success:           true,
			})
			now = now.Add(time.Millisecond)
			break
		}
		require.True(t, selected)
	}

	recovered := controller.Select(key, candidates, now.Add(time.Millisecond))
	require.Len(t, recovered.Allocations, 2)
	assert.Greater(t, recovered.Allocations[0].Share, 0.10)
	assert.LessOrEqual(t, recovered.Allocations[0].Share, 0.2001)
	assert.False(t, recovered.Allocations[0].Probe)
	assert.Contains(t, recovered.UnverifiedCandidates, 1)

	stats := controller.ObservationStats(ObservationKey{ChannelID: 1, Model: model}, now.Add(time.Millisecond))
	assert.Equal(t, 2, stats.SampleCount)
	assert.Equal(t, 2, stats.TTFTSampleCount)
}

func TestHardFailureDuringReprobeRestartsCooldown(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           1,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.10,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for _, channelID := range []int{1, 2} {
		controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
			ObservedAt: base,
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	controller.Select(key, candidates, base.Add(time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{ObservedAt: base.Add(10 * time.Second), HardFailure: true})
	controller.Select(key, candidates, base.Add(10*time.Second))

	firstProbe := controller.Select(key, candidates, base.Add(40*time.Second))
	require.Len(t, firstProbe.Allocations, 2)
	assert.InDelta(t, 0.05, firstProbe.Allocations[0].Share, 0.0001)
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{ObservedAt: base.Add(41 * time.Second), HardFailure: true})
	failedAgain := controller.Select(key, candidates, base.Add(41*time.Second))
	require.Len(t, failedAgain.Allocations, 1)
	assert.Equal(t, 2, failedAgain.Allocations[0].ChannelID)

	stillCooling := controller.Select(key, candidates, base.Add(70*time.Second))
	require.Len(t, stillCooling.Allocations, 1)
	afterResetCooldown := controller.Select(key, candidates, base.Add(71*time.Second))
	require.Len(t, afterResetCooldown.Allocations, 2)
	assert.InDelta(t, 0.05, afterResetCooldown.Allocations[0].Share, 0.0001)
	assert.True(t, afterResetCooldown.Allocations[0].Probe)
}

func TestStaleCandidateOnlyReceivesProbeTraffic(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               10 * time.Minute,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: base.Add(-time.Minute + time.Duration(index)*time.Second),
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{
			ObservedAt: base.Add(-time.Hour + time.Duration(index)*time.Second),
			TTFT:       80 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}

	decision := controller.Select(
		RouteKey{Group: "default", Model: model},
		[]Candidate{
			{ChannelID: 1, Priority: 100, Weight: 100},
			{ChannelID: 2, Priority: 0, Weight: 100},
		},
		base,
	)

	assert.Equal(t, []int{2}, decision.UnverifiedCandidates)
	require.Len(t, decision.Allocations, 2)
	assert.InDelta(t, 0.05, decision.Allocations[1].Share, 0.0001)
	assert.True(t, decision.Allocations[1].Probe)
}

func TestVerifiedBetterCandidateCanPromoteWithoutHistoricalHealthyBaseline(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: observedAt,
			TTFT:       800 * time.Millisecond,
			TPOT:       80 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{
			ObservedAt: observedAt,
			TTFT:       150 * time.Millisecond,
			TPOT:       25 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
	}

	decision := controller.Select(
		RouteKey{Group: "default", Model: model},
		[]Candidate{
			{ChannelID: 1, Priority: 100, Weight: 100},
			{ChannelID: 2, Priority: 0, Weight: 100},
		},
		base.Add(5*time.Second),
	)

	assert.Equal(t, []int{1}, decision.DegradedCandidates)
	require.Len(t, decision.Allocations, 2)
	assert.Less(t, decision.Allocations[0].Share, 0.20)
	assert.Greater(t, decision.Allocations[1].Share, 0.80)
}

func TestCandidateChangesAddProbeRemoveOldShareAndKeepTotalNormalized(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    20,
		MaxAge:        time.Hour,
		MinSamples:    3,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	key := RouteKey{Group: "default", Model: "chat-model"}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}, base)

	decision := controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 3, Priority: 0, Weight: 100},
	}, base.Add(time.Second))

	require.Len(t, decision.Allocations, 2)
	assert.Equal(t, 1, decision.Allocations[0].ChannelID)
	assert.InDelta(t, 0.95, decision.Allocations[0].Share, 0.0001)
	assert.Equal(t, 3, decision.Allocations[1].ChannelID)
	assert.InDelta(t, 0.05, decision.Allocations[1].Share, 0.0001)
	assert.True(t, decision.Allocations[1].Probe)
	assert.InDelta(t, 1, decision.Allocations[0].Share+decision.Allocations[1].Share, 0.0001)
}

func TestDuplicateCandidateIDsAreDeduplicatedIntoASafeDecision(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    20,
		MaxAge:        time.Hour,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	decision := controller.Select(RouteKey{Group: "default", Model: "chat-model"}, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 10},
		{ChannelID: 1, Priority: 0, Weight: 1000},
	}, time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))

	require.Len(t, decision.Allocations, 1)
	assert.Equal(t, 1, decision.Allocations[0].ChannelID)
	assert.InDelta(t, 1, decision.Allocations[0].Share, 0.0001)
	assert.True(t, decision.HasSelection)
	assert.Equal(t, 1, decision.SelectedChannelID)
}

func TestCandidateFingerprintRebuildsWhenPriorityChanges(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    20,
		MaxAge:        time.Hour,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	key := RouteKey{Group: "default", Model: "chat-model"}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}, base)

	decision := controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 200, Weight: 100},
	}, base.Add(time.Second))

	require.Len(t, decision.Allocations, 2)
	assert.Equal(t, 2, decision.Allocations[0].ChannelID)
	assert.InDelta(t, 0.95, decision.Allocations[0].Share, 0.0001)
	assert.Equal(t, 1, decision.Allocations[1].ChannelID)
	assert.InDelta(t, 0.05, decision.Allocations[1].Share, 0.0001)
	assert.True(t, decision.Allocations[1].Probe)
}

func TestCandidateFingerprintRebuildsForWeightAndFullReplacement(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:       true,
		MaxSamples:    20,
		MaxAge:        time.Hour,
		ProbeFraction: 0.05,
	})
	require.NoError(t, err)

	key := RouteKey{Group: "default", Model: "chat-model"}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 0},
		{ChannelID: 2, Priority: 100, Weight: 90},
	}, base)
	weightChanged := controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 90},
		{ChannelID: 2, Priority: 100, Weight: 0},
	}, base.Add(time.Second))
	require.Len(t, weightChanged.Allocations, 2)
	assert.InDelta(t, 100.0/110.0, weightChanged.Allocations[0].Share, 0.0001)
	assert.InDelta(t, 10.0/110.0, weightChanged.Allocations[1].Share, 0.0001)

	replaced := controller.Select(key, []Candidate{
		{ChannelID: 3, Priority: 200, Weight: 100},
		{ChannelID: 4, Priority: 0, Weight: 100},
	}, base.Add(2*time.Second))
	require.Len(t, replaced.Allocations, 2)
	assert.Equal(t, 3, replaced.Allocations[0].ChannelID)
	assert.InDelta(t, 0.95, replaced.Allocations[0].Share, 0.0001)
	assert.Equal(t, 4, replaced.Allocations[1].ChannelID)
	assert.InDelta(t, 0.05, replaced.Allocations[1].Share, 0.0001)
}

func TestCandidateFingerprintRebuildReappliesExistingHealthInSameSelect(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{ObservedAt: observedAt, TTFT: 800 * time.Millisecond, HasTTFT: true, Success: true})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{ObservedAt: observedAt, TTFT: 100 * time.Millisecond, HasTTFT: true, Success: true})
	}
	key := RouteKey{Group: "default", Model: model}
	controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}, base.Add(5*time.Second))

	rebuilt := controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 200},
	}, base.Add(6*time.Second))
	require.Len(t, rebuilt.Allocations, 2)
	assert.Less(t, rebuilt.Allocations[0].Share, 0.20)
	assert.Greater(t, rebuilt.Allocations[1].Share, 0.80)
	assert.Equal(t, []int{1}, rebuilt.DegradedCandidates)
}

func TestHardFailureEjectionSurvivesCandidateFingerprintChanges(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           1,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for _, channelID := range []int{1, 2, 3} {
		controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
			ObservedAt: base,
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	initial := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	controller.Select(key, initial, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(10 * time.Second),
		HardFailure: true,
	})
	ejected := controller.Select(key, initial, base.Add(10*time.Second))
	require.Len(t, ejected.Allocations, 1)
	assert.Equal(t, 2, ejected.Allocations[0].ChannelID)
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt: base.Add(12 * time.Second),
		TTFT:       100 * time.Millisecond,
		HasTTFT:    true,
		Success:    true,
	})

	weightChanged := controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 200},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}, base.Add(15*time.Second))
	require.Len(t, weightChanged.Allocations, 1)
	assert.Equal(t, 2, weightChanged.Allocations[0].ChannelID)

	priorityChanged := controller.Select(key, []Candidate{
		{ChannelID: 1, Priority: 200, Weight: 200},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}, base.Add(20*time.Second))
	require.Len(t, priorityChanged.Allocations, 1)
	assert.Equal(t, 2, priorityChanged.Allocations[0].ChannelID)

	filtered := controller.Select(key, []Candidate{
		{ChannelID: 2, Priority: 0, Weight: 100},
	}, base.Add(25*time.Second))
	require.Len(t, filtered.Allocations, 1)
	assert.Equal(t, 2, filtered.Allocations[0].ChannelID)

	reintroduced := []Candidate{
		{ChannelID: 1, Priority: 200, Weight: 200},
		{ChannelID: 2, Priority: 0, Weight: 100},
		{ChannelID: 3, Priority: 0, Weight: 100},
	}
	stillCooling := controller.Select(key, reintroduced, base.Add(39*time.Second))
	for _, allocation := range stillCooling.Allocations {
		assert.NotEqual(t, 1, allocation.ChannelID)
	}

	afterOriginalDeadline := controller.Select(key, reintroduced, base.Add(40*time.Second))
	require.Len(t, afterOriginalDeadline.Allocations, 3)
	assert.Equal(t, 1, afterOriginalDeadline.Allocations[0].ChannelID)
	assert.InDelta(t, 0.05, afterOriginalDeadline.Allocations[0].Share, 0.0001)
	assert.True(t, afterOriginalDeadline.Allocations[0].Probe)
}

func TestExpiredEjectionFirstReentryIsCappedAtProbeShare(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           1,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for _, channelID := range []int{1, 2} {
		controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
			ObservedAt: base,
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	controller.Select(key, candidates, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(10 * time.Second),
		HardFailure: true,
	})
	controller.Select(key, candidates, base.Add(10*time.Second))
	controller.Select(key, candidates[1:], base.Add(11*time.Second))

	firstReentry := controller.Select(key, candidates, base.Add(41*time.Second))
	require.Len(t, firstReentry.Allocations, 2)
	assert.Equal(t, 1, firstReentry.Allocations[0].ChannelID)
	assert.InDelta(t, 0.05, firstReentry.Allocations[0].Share, 0.0001)
	assert.True(t, firstReentry.Allocations[0].Probe)
	assert.Equal(t, 2, firstReentry.Allocations[1].ChannelID)
	assert.InDelta(t, 0.95, firstReentry.Allocations[1].Share, 0.0001)
}

func TestAlternatingEligibilityStillSelectsExpiredEjectionWithinFiniteRequests(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           1,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for _, channelID := range []int{1, 2} {
		controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
			ObservedAt: base,
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	controller.Select(key, candidates, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(10 * time.Second),
		HardFailure: true,
	})
	controller.Select(key, candidates, base.Add(10*time.Second))
	controller.Select(key, candidates[1:], base.Add(11*time.Second))

	eligibleRequests := 0
	selectedOnEligibleRequest := 0
	for request := 0; request < 40; request++ {
		now := base.Add(41*time.Second + time.Duration(request)*time.Millisecond)
		if request%2 == 1 {
			decision := controller.Select(key, candidates[1:], now)
			assert.Equal(t, 2, decision.SelectedChannelID)
			continue
		}

		eligibleRequests++
		decision := controller.Select(key, candidates, now)
		require.Len(t, decision.Allocations, 2)
		assert.InDelta(t, 0.05, decision.Allocations[0].Share, 0.0001)
		assert.True(t, decision.Allocations[0].Probe)
		if decision.SelectedChannelID == 1 {
			selectedOnEligibleRequest = eligibleRequests
			break
		}
	}
	assert.NotZero(t, selectedOnEligibleRequest)
	assert.LessOrEqual(t, selectedOnEligibleRequest, 20)
}

func TestRouteStateResetsAfterItsObservationWindowHasBeenIdle(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               10 * time.Minute,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
			ObservedAt: observedAt,
			TTFT:       800 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{
			ObservedAt: observedAt,
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	shifted := controller.Select(key, candidates, base.Add(5*time.Second))
	assert.Greater(t, shifted.Allocations[1].Share, 0.80)

	reset := controller.Select(key, candidates, base.Add(time.Hour))
	require.Len(t, reset.Allocations, 2)
	assert.InDelta(t, 0.95, reset.Allocations[0].Share, 0.0001)
	assert.InDelta(t, 0.05, reset.Allocations[1].Share, 0.0001)
	assert.True(t, reset.Allocations[1].Probe)
}

func TestRecoveredChannelReturnsSlowlyAndCooldownPreventsFlapping(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           30,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		Cooldown:             10 * time.Second,
	})
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{ObservedAt: observedAt, TTFT: 100 * time.Millisecond, HasTTFT: true, Success: true})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{ObservedAt: observedAt, TTFT: 120 * time.Millisecond, HasTTFT: true, Success: true})
	}
	controller.Select(key, candidates, base.Add(3*time.Second))

	for index := 0; index < 3; index++ {
		observedAt := base.Add(20*time.Second + time.Duration(index)*time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{ObservedAt: observedAt, TTFT: 240 * time.Millisecond, HasTTFT: true, Success: true})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{ObservedAt: observedAt, TTFT: 120 * time.Millisecond, HasTTFT: true, Success: true})
	}
	degraded := controller.Select(key, candidates, base.Add(30*time.Second))
	degradedShare := degraded.Allocations[0].Share
	require.Less(t, degradedShare, 0.20)

	for index := 0; index < 3; index++ {
		observedAt := base.Add(40*time.Second + time.Duration(index)*time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{ObservedAt: observedAt, TTFT: 100 * time.Millisecond, HasTTFT: true, Success: true})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{ObservedAt: observedAt, TTFT: 120 * time.Millisecond, HasTTFT: true, Success: true})
	}
	firstRecovery := controller.Select(key, candidates, base.Add(43*time.Second))
	firstRecoveryShare := firstRecovery.Allocations[0].Share
	assert.Empty(t, firstRecovery.DegradedCandidates)
	assert.Greater(t, firstRecoveryShare, degradedShare)
	assert.LessOrEqual(t, firstRecoveryShare, degradedShare+0.0501)

	withinCooldown := controller.Select(key, candidates, base.Add(44*time.Second))
	assert.InDelta(t, firstRecoveryShare, withinCooldown.Allocations[0].Share, 0.0001)

	secondRecovery := controller.Select(key, candidates, base.Add(55*time.Second))
	assert.Greater(t, secondRecovery.Allocations[0].Share, firstRecoveryShare)
	assert.LessOrEqual(t, secondRecovery.Allocations[0].Share, firstRecoveryShare+0.0501)
}

func TestUpdateConfigCanDisableAndReenableWithoutLosingObservations(t *testing.T) {
	config := Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		Cooldown:             time.Minute,
	}
	controller, err := NewController(config)
	require.NoError(t, err)

	model := "chat-model"
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		observedAt := base.Add(time.Duration(index) * time.Second)
		controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{ObservedAt: observedAt, TTFT: 800 * time.Millisecond, HasTTFT: true, Success: true})
		controller.Observe(ObservationKey{ChannelID: 2, Model: model}, Sample{ObservedAt: observedAt, TTFT: 100 * time.Millisecond, HasTTFT: true, Success: true})
	}
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	beforeDisable := controller.Select(key, candidates, base.Add(5*time.Second))
	assert.Greater(t, beforeDisable.Allocations[1].Share, 0.80)

	config.Enabled = false
	require.NoError(t, controller.UpdateConfig(config))
	disabled := controller.Select(key, candidates, base.Add(6*time.Second))
	assert.False(t, disabled.Dynamic)
	require.Len(t, disabled.Allocations, 1)
	assert.Equal(t, 1, disabled.Allocations[0].ChannelID)
	assert.InDelta(t, 1, disabled.Allocations[0].Share, 0.0001)

	config.Enabled = true
	require.NoError(t, controller.UpdateConfig(config))
	reenabled := controller.Select(key, candidates, base.Add(7*time.Second))
	assert.True(t, reenabled.Dynamic)
	require.Len(t, reenabled.Allocations, 2)
	assert.Greater(t, reenabled.Allocations[1].Share, 0.80)
	assert.Equal(t, 3, controller.ObservationStats(ObservationKey{ChannelID: 2, Model: model}, base.Add(7*time.Second)).SampleCount)
}

func TestUpdateConfigPreservesHardEjectionSafetyState(t *testing.T) {
	config := Config{
		Enabled:              true,
		MaxSamples:           20,
		MaxAge:               time.Hour,
		MinSamples:           1,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	}
	controller, err := NewController(config)
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for _, channelID := range []int{1, 2} {
		controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
			ObservedAt: base,
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	controller.Select(key, candidates, base.Add(5*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:  base.Add(10 * time.Second),
		HardFailure: true,
	})
	controller.Select(key, candidates, base.Add(10*time.Second))
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{
		ObservedAt:        base.Add(12 * time.Second),
		UpstreamStartedAt: base.Add(9 * time.Second),
		TTFT:              100 * time.Millisecond,
		HasTTFT:           true,
		Success:           true,
	})

	config.Aggressiveness = 0.7
	require.NoError(t, controller.UpdateConfig(config))
	withinCooldown := controller.Select(key, candidates, base.Add(15*time.Second))
	require.Len(t, withinCooldown.Allocations, 1)
	assert.Equal(t, 2, withinCooldown.Allocations[0].ChannelID)
	assert.Equal(t, 2, withinCooldown.SelectedChannelID)

	afterDeadline := controller.Select(key, candidates, base.Add(40*time.Second))
	require.Len(t, afterDeadline.Allocations, 2)
	assert.Equal(t, 1, afterDeadline.Allocations[0].ChannelID)
	assert.InDelta(t, 0.05, afterDeadline.Allocations[0].Share, 0.0001)
	assert.True(t, afterDeadline.Allocations[0].Probe)
}

func TestActiveRoutePrunesRemovedEjectionTombstonesAfterSafeTTL(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           5,
		MaxAge:               time.Minute,
		MinSamples:           1,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	const safeChannelID = 1000
	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	controller.Observe(ObservationKey{ChannelID: safeChannelID, Model: model}, Sample{
		ObservedAt: base,
		TTFT:       100 * time.Millisecond,
		HasTTFT:    true,
		Success:    true,
	})
	for channelID := 1; channelID <= 100; channelID++ {
		controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
			ObservedAt: base,
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
		controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
			ObservedAt:  base.Add(10 * time.Second),
			HardFailure: true,
		})
		controller.Select(key, []Candidate{
			{ChannelID: channelID, Priority: 100, Weight: 100},
			{ChannelID: safeChannelID, Priority: 0, Weight: 100},
		}, base.Add(10*time.Second))
	}
	require.Len(t, controller.routes[key].ejections, 100)

	safeOnly := []Candidate{{ChannelID: safeChannelID, Priority: 0, Weight: 100}}
	for _, offset := range []time.Duration{20, 50, 80, 99} {
		controller.Select(key, safeOnly, base.Add(offset*time.Second))
	}
	require.Len(t, controller.routes[key].ejections, 100)

	controller.Select(key, safeOnly, base.Add(101*time.Second))
	assert.Empty(t, controller.routes[key].ejections)
}

func TestControllerSupportsConcurrentObserveSelectAndConfigUpdates(t *testing.T) {
	config := Config{
		Enabled:              true,
		MaxSamples:           50,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	}
	controller, err := NewController(config)
	require.NoError(t, err)

	model := "chat-model"
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
	}
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	start := make(chan struct{})
	updateErrors := make(chan error, 20)
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		worker := worker
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			for request := 0; request < 100; request++ {
				now := base.Add(time.Duration(worker*100+request) * time.Millisecond)
				channelID := request%2 + 1
				controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
					ObservedAt: now,
					TTFT:       time.Duration(100+channelID*10) * time.Millisecond,
					HasTTFT:    true,
					Success:    true,
				})
				controller.Select(key, candidates, now)
			}
		}()
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		<-start
		for update := 0; update < 20; update++ {
			updated := config
			updated.Aggressiveness = 0.7 + float64(update%2)*0.1
			if err := controller.UpdateConfig(updated); err != nil {
				updateErrors <- err
			}
		}
	}()
	close(start)
	wait.Wait()
	close(updateErrors)
	for updateErr := range updateErrors {
		require.NoError(t, updateErr)
	}
	require.NoError(t, controller.UpdateConfig(config))

	decision := controller.Select(key, candidates, base.Add(time.Second))
	require.True(t, decision.HasSelection)
	total := 0.0
	for _, allocation := range decision.Allocations {
		total += allocation.Share
	}
	assert.InDelta(t, 1, total, 0.0001)
}

func TestObserveOnlyTrafficPeriodicallyCleansExpiredObservationKeys(t *testing.T) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           10,
		MaxAge:               time.Minute,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	require.NoError(t, err)

	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for channelID := 1; channelID <= 100; channelID++ {
		controller.Observe(ObservationKey{ChannelID: channelID, Model: "chat-model"}, Sample{
			ObservedAt: base,
			TTFT:       100 * time.Millisecond,
			HasTTFT:    true,
			Success:    true,
		})
	}
	require.Len(t, controller.observations, 100)

	controller.Observe(ObservationKey{ChannelID: 101, Model: "chat-model"}, Sample{
		ObservedAt: base.Add(2 * time.Minute),
		TTFT:       100 * time.Millisecond,
		HasTTFT:    true,
		Success:    true,
	})

	assert.Len(t, controller.observations, 1)
	assert.Contains(t, controller.observations, ObservationKey{ChannelID: 101, Model: "chat-model"})
}

func BenchmarkControllerSelect(b *testing.B) {
	controller, err := NewController(Config{
		Enabled:              true,
		MaxSamples:           50,
		MaxAge:               time.Hour,
		MinSamples:           3,
		ProbeFraction:        0.05,
		DegradationThreshold: 1.4,
		RecoveryThreshold:    1.15,
		CriticalThreshold:    2,
		CandidateAdvantage:   1.1,
		Aggressiveness:       0.8,
		RecoveryStep:         0.05,
		HardFailureThreshold: 1,
		HardFailureCooldown:  30 * time.Second,
	})
	if err != nil {
		b.Fatal(err)
	}
	model := "chat-model"
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		for _, channelID := range []int{1, 2, 3} {
			controller.Observe(ObservationKey{ChannelID: channelID, Model: model}, Sample{
				ObservedAt: now.Add(time.Duration(index) * time.Second),
				TTFT:       100 * time.Millisecond,
				TPOT:       20 * time.Millisecond,
				HasTTFT:    true,
				HasTPOT:    true,
				Success:    true,
			})
		}
	}
	key := RouteKey{Group: "default", Model: model}
	candidates := []Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 0, Weight: 100},
		{ChannelID: 3, Priority: 0, Weight: 100},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		controller.Select(key, candidates, now.Add(5*time.Second))
	}
}
