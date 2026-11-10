package dynamicrouting

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discountRoutingFixture(t *testing.T) (*Controller, RouteKey, []Candidate, DiscountLimits, time.Time) {
	t.Helper()
	controller, err := NewController(Config{Enabled: true, MaxSamples: 60, MaxAge: 90 * time.Second, MinSamples: 3, ProbeFraction: 0.015, DegradationThreshold: 1.3, RecoveryThreshold: 1.1, CriticalThreshold: 1.9, Aggressiveness: 0.9, RecoveryStep: 0.02, Cooldown: 3 * time.Second, HardFailureCooldown: 30 * time.Second})
	require.NoError(t, err)
	cheap, backup, expensive := 0.5, 0.6, 0.9
	return controller, RouteKey{Group: "default", Model: "discount-model"}, []Candidate{
		{ChannelID: 1, CostDiscount: &cheap},
		{ChannelID: 2, Priority: 50, CostDiscount: &backup},
		{ChannelID: 3, Priority: 100, CostDiscount: &expensive},
	}, DiscountLimits{MaxTTFT: 5 * time.Second, MaxTPOT: 200 * time.Millisecond}, time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
}

func TestDiscountRoutingKeepsCheapTrafficWhileQualityMeetsLimits(t *testing.T) {
	controller, key, candidates, limits, now := discountRoutingFixture(t)
	for _, candidate := range candidates {
		for range 3 {
			ttft := time.Second
			if candidate.ChannelID == 1 {
				ttft = 5 * time.Second
			}
			controller.Observe(ObservationKey{ChannelID: candidate.ChannelID, Model: key.Model}, Sample{ObservedAt: now, Success: true, HasTTFT: true, TTFT: ttft, HasTPOT: true, TPOT: 100 * time.Millisecond})
		}
	}
	decision := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now)
	require.True(t, decision.HasSelection)
	assert.Equal(t, 1, decision.SelectedChannelID)
	assert.Empty(t, decision.DegradedCandidates)
	assert.InDelta(t, 0.985, decision.Allocations[0].Share, 1e-9)
	assert.InDelta(t, 0.0075, decision.Allocations[1].Share, 1e-9)
	assert.InDelta(t, 0.0075, decision.Allocations[2].Share, 1e-9)
	// The existing smart strategy still starts on the highest priority channel.
	stable := controller.SelectAvoiding(key, candidates, nil, now)
	assert.Equal(t, 3, stable.SelectedChannelID)
	avoiding := controller.SelectDiscountAvoiding(key, candidates, map[int]struct{}{1: {}}, limits, now)
	assert.NotEqual(t, 1, avoiding.SelectedChannelID)
	assert.Equal(t, decision.Allocations, avoiding.Allocations)
}

func TestDiscountRoutingMovesToNextCheapestAndRecoversWithProbes(t *testing.T) {
	for _, metric := range []string{"ttft", "tpot"} {
		t.Run(metric, func(t *testing.T) {
			controller, key, candidates, limits, now := discountRoutingFixture(t)
			for _, candidate := range candidates {
				for range 3 {
					ttft, tpot := time.Second, 100*time.Millisecond
					if candidate.ChannelID == 1 {
						if metric == "ttft" {
							ttft = 10 * time.Second
						} else {
							tpot = 400 * time.Millisecond
						}
					}
					controller.Observe(ObservationKey{ChannelID: candidate.ChannelID, Model: key.Model}, Sample{ObservedAt: now, Success: true, HasTTFT: true, TTFT: ttft, HasTPOT: true, TPOT: tpot})
				}
			}
			degraded := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now)
			assert.Contains(t, degraded.DegradedCandidates, 1)
			assert.Less(t, degraded.Allocations[0].Share, 0.1)
			assert.Greater(t, degraded.Allocations[0].Share, 0.0, "the cheapest channel keeps recovery samples")
			assert.Greater(t, degraded.Allocations[1].Share, 0.88)
			assert.InDelta(t, 0.0075, degraded.Allocations[2].Share, 1e-9)
			for range 3 {
				controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: now.Add(time.Second), Success: true, HasTTFT: true, TTFT: time.Second, HasTPOT: true, TPOT: 50 * time.Millisecond})
			}
			recovered := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(4*time.Second))
			assert.NotContains(t, recovered.DegradedCandidates, 1)
			assert.InDelta(t, degraded.Allocations[0].Share+0.02, recovered.Allocations[0].Share, 1e-9)
			assert.Less(t, recovered.Allocations[0].Share, 0.985, "recovery must be gradual")
		})
	}
}

func TestDiscountRoutingRecoveryIsNotBlockedByADegradedBackup(t *testing.T) {
	controller, key, candidates, limits, now := discountRoutingFixture(t)
	for _, candidate := range candidates {
		for range 3 {
			ttft := 10 * time.Second
			if candidate.ChannelID == 2 {
				ttft = time.Second
			}
			controller.Observe(ObservationKey{ChannelID: candidate.ChannelID, Model: key.Model}, Sample{ObservedAt: now, Success: true, HasTTFT: true, TTFT: ttft})
		}
	}
	before := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now)
	for range 3 {
		controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: now.Add(time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
	}
	after := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(4*time.Second))
	assert.Contains(t, after.DegradedCandidates, 3)
	assert.GreaterOrEqual(t, after.Allocations[0].Share, before.Allocations[0].Share+0.02)
	assert.LessOrEqual(t, after.Allocations[2].Share, before.Allocations[2].Share, "recovery must not refill a degraded probe")
	total := 0.0
	for _, allocation := range after.Allocations {
		total += allocation.Share
	}
	assert.InDelta(t, 1, total, 1e-9)
}

func TestDiscountRoutingUnknownZeroChangesAndFailureFallback(t *testing.T) {
	for _, discount := range []float64{0, 0.2, 1, math.NaN(), math.Inf(1), -1, 1.1} {
		t.Run(fmt.Sprint(discount), func(t *testing.T) {
			controller, key, _, limits, now := discountRoutingFixture(t)
			candidates := []Candidate{{ChannelID: 1, CostDiscount: &discount}, {ChannelID: 2, Priority: 100}}
			decision := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now)
			want := 1
			if math.IsNaN(discount) || math.IsInf(discount, 0) || discount < 0 || discount > 1 {
				want = 2
			}
			assert.Equal(t, want, decision.SelectedChannelID)
		})
	}
	controller, key, candidates, limits, now := discountRoutingFixture(t)
	controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: now, HardFailure: true})
	failed := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now)
	assert.Equal(t, 2, failed.SelectedChannelID, "hard failure uses the next cheapest even before verification")
	for _, allocation := range failed.Allocations {
		assert.NotEqual(t, 1, allocation.ChannelID, "ejected channels have no allocation")
	}
	updated := 0.1
	candidates[2].CostDiscount = &updated
	changed := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(time.Second))
	assert.Equal(t, 3, changed.SelectedChannelID, "cost edits must rebuild the target allocation")
	for _, allocation := range changed.Allocations {
		if allocation.ChannelID == 1 {
			assert.Zero(t, allocation.Share, "cost edits must preserve ejection")
		}
	}
	limits.MaxTTFT = 3 * time.Second
	retuned := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(2*time.Second))
	for _, allocation := range retuned.Allocations {
		assert.NotEqual(t, 1, allocation.ChannelID, "quality edits preserve failure cooldown")
	}
	probing := controller.SelectDiscountAvoiding(key, candidates, nil, limits, now.Add(31*time.Second))
	for _, allocation := range probing.Allocations {
		assert.NotEqual(t, 1, allocation.ChannelID, "cooldown expiry does not send customer traffic")
	}
	targets := controller.RecoveryTargets(now.Add(31 * time.Second))
	require.Len(t, targets, 1)
	assert.Equal(t, ObservationKey{ChannelID: 1, Model: key.Model}, targets[0].ObservationKey)
	assert.False(t, controller.SelectDiscountAvoiding(key, candidates, nil, DiscountLimits{}, now).HasSelection)
}

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
		"cooldown cap below base":  func(config *Config) { config.HardFailureMaxCooldown = time.Second },
		"negative observation":     func(config *Config) { config.RecoveryObservationWindow = -time.Second },
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

func TestHardFailureDuringReprobeDoublesCooldown(t *testing.T) {
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
	require.Equal(t, []Allocation{{ChannelID: 2, Share: 1}}, firstProbe.Allocations)
	assert.Equal(t, base.Add(40*time.Second), controller.RecoveryTargets(base.Add(40 * time.Second))[0].CooldownUntil)
	controller.Observe(ObservationKey{ChannelID: 1, Model: model}, Sample{RecoveryProbe: true, ObservedAt: base.Add(41 * time.Second), HardFailure: true})
	failedAgain := controller.Select(key, candidates, base.Add(41*time.Second))
	require.Len(t, failedAgain.Allocations, 1)
	assert.Equal(t, 2, failedAgain.Allocations[0].ChannelID)

	stillCooling := controller.Select(key, candidates, base.Add(70*time.Second))
	require.Len(t, stillCooling.Allocations, 1)
	stillBackingOff := controller.Select(key, candidates, base.Add(71*time.Second))
	require.Len(t, stillBackingOff.Allocations, 1)
	afterResetCooldown := controller.Select(key, candidates, base.Add(101*time.Second))
	require.Equal(t, []Allocation{{ChannelID: 2, Share: 1}}, afterResetCooldown.Allocations)
	assert.Equal(t, base.Add(101*time.Second), controller.RecoveryTargets(base.Add(101 * time.Second))[0].CooldownUntil)
}

func hardFailureRecoveryFixture(t *testing.T) (*Controller, RouteKey, []Candidate, time.Time) {
	t.Helper()
	controller, err := NewController(Config{
		Enabled: true, MaxSamples: 20, MaxAge: 90 * time.Second,
		MinSamples: 1, ProbeFraction: 0.05, RecoveryStep: 0.1, Cooldown: 3 * time.Second,
	})
	require.NoError(t, err)
	key := RouteKey{Group: "default", Model: "recovery-model"}
	candidates := []Candidate{{ChannelID: 1, Priority: 100, Weight: 100}, {ChannelID: 2, Weight: 100}}
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	for _, candidate := range candidates {
		controller.Observe(ObservationKey{ChannelID: candidate.ChannelID, Model: key.Model}, Sample{
			ObservedAt: now, Success: true, HasTTFT: true, TTFT: time.Second,
		})
	}
	controller.Select(key, candidates, now)
	return controller, key, candidates, now
}

func TestActiveRecoveryRequiresFreshProbesAndObservationWindow(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		active, oldStart, metrics bool
		maxAge                    time.Duration
		wantRecovery              bool
	}{
		{name: "health only", active: true, wantRecovery: true},
		{name: "short performance window", active: true, maxAge: time.Second, wantRecovery: true},
		{name: "synthetic latency does not calibrate real requests", active: true, metrics: true, wantRecovery: true},
		{name: "real request cannot confirm recovery", metrics: true},
		{name: "probe started during cooldown", active: true, oldStart: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			maxAge := tc.maxAge
			if maxAge == 0 {
				maxAge = 90 * time.Second
			}
			controller, err := NewController(Config{Enabled: true, MaxSamples: 20, MaxAge: maxAge, MinSamples: 3, ProbeFraction: 0.05, RecoveryStep: 0.1, Cooldown: 3 * time.Second, RecoveryObservationWindow: 10 * time.Second})
			require.NoError(t, err)
			base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
			key := RouteKey{Group: "default", Model: "recovery-model"}
			candidates := []Candidate{{ChannelID: 1, Priority: 100}, {ChannelID: 2}}
			observation := ObservationKey{ChannelID: 1, Model: key.Model}
			controller.Observe(observation, Sample{ObservedAt: base.Add(time.Second), HardFailure: true})
			controller.Select(key, candidates, base.Add(2*time.Second))
			for _, seconds := range []int{31, 36, 41, 50, 51} {
				at := base.Add(time.Duration(seconds) * time.Second)
				started := at
				if tc.oldStart {
					started = base.Add(30 * time.Second)
				}
				controller.Observe(observation, Sample{RecoveryProbe: tc.active, ObservedAt: at, UpstreamStartedAt: started, Success: true, HasTTFT: tc.metrics, TTFT: time.Millisecond})
				controller.RecoveryTargets(at)
				decision := controller.Select(key, candidates, at)
				if seconds < 51 || !tc.wantRecovery {
					require.Equal(t, []Allocation{{ChannelID: 2, Share: 1}}, decision.Allocations)
					assert.Equal(t, 2, decision.SelectedChannelID)
				} else {
					require.Len(t, decision.Allocations, 2)
					assert.InDelta(t, 0.1, decision.Allocations[0].Share, 1e-9)
					assert.NotContains(t, decision.DegradedCandidates, 1)
				}
			}
			if tc.active {
				assert.Zero(t, controller.ObservationStats(observation, base.Add(51*time.Second)).SampleCount, "active checks must not enter customer performance samples")
			}
		})
	}
}

func TestAllFaultedChannelsWaitForIndependentRecovery(t *testing.T) {
	for _, count := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			controller, err := NewController(Config{Enabled: true, MaxSamples: 20, MaxAge: 90 * time.Second, MinSamples: 1, ProbeFraction: 0.05, RecoveryStep: 0.1, RecoveryObservationWindow: 10 * time.Second})
			require.NoError(t, err)
			base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
			key := RouteKey{Group: "default", Model: "all-faulted"}
			candidates := make([]Candidate, count)
			for i := range count {
				candidates[i] = Candidate{ChannelID: i + 1, Priority: int64(count - i)}
			}
			controller.Select(key, candidates, base)
			for i := range count {
				at := base.Add(time.Duration(i+1) * time.Second)
				controller.Observe(ObservationKey{ChannelID: i + 1, Model: key.Model}, Sample{ObservedAt: at, HardFailure: true})
				controller.Select(key, candidates, at)
			}
			for _, seconds := range []int{35, 40, 45} {
				at := base.Add(time.Duration(seconds) * time.Second)
				decision := controller.Select(key, candidates, at)
				assert.False(t, decision.HasSelection, "all-failed requests must fail closed")
				assert.Empty(t, decision.Allocations)
				require.Len(t, controller.RecoveryTargets(at), count)
			}
			// The scheduler can complete recovery without a single real request.
			observation := ObservationKey{ChannelID: 1, Model: key.Model}
			for _, seconds := range []int{46, 51, 56} {
				at := base.Add(time.Duration(seconds) * time.Second)
				controller.Observe(observation, Sample{RecoveryProbe: true, UpstreamStartedAt: at, ObservedAt: at, Success: true})
				controller.RecoveryTargets(at)
			}
			assert.Len(t, controller.RecoveryTargets(base.Add(56*time.Second)), count-1)
			recovered := controller.Select(key, candidates, base.Add(56*time.Second))
			require.True(t, recovered.HasSelection)
			assert.Equal(t, 1, recovered.SelectedChannelID)
			assert.Equal(t, []Allocation{{ChannelID: 1, Share: 1}}, recovered.Allocations)
		})
	}
}

func TestRetriesCannotSelectRecoveringChannels(t *testing.T) {
	controller, key, candidates, base := hardFailureRecoveryFixture(t)
	controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: base, HardFailure: true})
	controller.Select(key, candidates, base)
	for _, blocked := range []map[int]struct{}{nil, {1: {}}, {2: {}}, {1: {}, 2: {}}} {
		decision := controller.SelectAvoiding(key, candidates, blocked, base.Add(31*time.Second))
		_, healthyExcluded := blocked[2]
		assert.Equal(t, !healthyExcluded, decision.HasSelection)
		if decision.HasSelection {
			assert.Equal(t, 2, decision.SelectedChannelID)
		}
		assert.Equal(t, []Allocation{{ChannelID: 2, Share: 1}}, decision.Allocations)
	}
	assert.Len(t, controller.RecoveryTargets(base.Add(31*time.Second)), 1, "retry exclusions cannot consume or disable active recovery checks")
}

func TestActiveRecoveryDoesNotExpireWithIdleCustomerTraffic(t *testing.T) {
	controller, key, candidates, base := hardFailureRecoveryFixture(t)
	controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: base, HardFailure: true})
	controller.Select(key, candidates, base)
	at := base.Add(time.Hour)
	require.Len(t, controller.RecoveryTargets(at), 1, "recovery remains scheduled without customer traffic")
	assert.True(t, controller.IsRecovering(key, 1, at))
	decision := controller.Select(key, candidates, at)
	assert.Equal(t, 2, decision.SelectedChannelID)
	require.Len(t, decision.Allocations, 1)
}

func TestFrequentActiveChecksDoNotPostponeTheCurrentCooldown(t *testing.T) {
	controller, key, candidates, base := hardFailureRecoveryFixture(t)
	observation := ObservationKey{ChannelID: 1, Model: key.Model}
	controller.Observe(observation, Sample{ObservedAt: base.Add(10 * time.Second), HardFailure: true})
	controller.Select(key, candidates, base.Add(10*time.Second))
	for _, seconds := range []int{15, 20, 25, 30, 35} {
		at := base.Add(time.Duration(seconds) * time.Second)
		controller.Observe(observation, Sample{RecoveryProbe: true, UpstreamStartedAt: at, ObservedAt: at, HardFailure: true})
		assert.Equal(t, base.Add(40*time.Second), controller.RecoveryTargets(at)[0].CooldownUntil)
	}
	at := base.Add(41 * time.Second)
	controller.Observe(observation, Sample{RecoveryProbe: true, UpstreamStartedAt: at, ObservedAt: at, HardFailure: true})
	assert.Equal(t, base.Add(101*time.Second), controller.RecoveryTargets(at)[0].CooldownUntil, "a failed recovery attempt after cooldown still backs off")
}

func TestRepeatedHardFailuresBackOffToCapWithoutExpiringAtSampleAge(t *testing.T) {
	controller, key, candidates, base := hardFailureRecoveryFixture(t)
	failedAt := base.Add(10 * time.Second)
	for _, seconds := range []int{30, 60, 120, 240, 480, 900, 900} {
		controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: failedAt, HardFailure: true})
		failed := controller.Select(key, candidates, failedAt)
		require.Len(t, failed.Allocations, 1)
		assert.Equal(t, 2, failed.SelectedChannelID)
		deadline := failedAt.Add(time.Duration(seconds) * time.Second)
		cooling := controller.Select(key, candidates, deadline.Add(-time.Millisecond))
		require.Len(t, cooling.Allocations, 1, "cooldown must last %d seconds", seconds)
		probe := controller.Select(key, candidates, deadline)
		require.Len(t, probe.Allocations, 1)
		assert.Equal(t, deadline, controller.RecoveryTargets(deadline)[0].CooldownUntil)
		failedAt = deadline.Add(time.Second)
	}
}

func TestHardFailureBackoffHonorsConfiguredCapWithoutDurationOverflow(t *testing.T) {
	for _, test := range []struct {
		name string
		base time.Duration
		cap  time.Duration
	}{
		{name: "non-power-of-two cap", base: 30 * time.Second, cap: 45 * time.Second},
		{name: "maximum duration", base: time.Duration(1 << 62), cap: time.Duration(1<<63 - 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			controller, key, candidates, base := hardFailureRecoveryFixture(t)
			config := controller.config
			config.HardFailureCooldown, config.HardFailureMaxCooldown = test.base, test.cap
			require.NoError(t, controller.UpdateConfig(config))
			failedAt := base.Add(time.Second)
			for _, cooldown := range []time.Duration{test.base, test.cap, test.cap} {
				controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: failedAt, HardFailure: true})
				controller.Select(key, candidates, failedAt)
				deadline := failedAt.Add(cooldown)
				require.Len(t, controller.Select(key, candidates, deadline.Add(-time.Millisecond)).Allocations, 1)
				require.Len(t, controller.Select(key, candidates, deadline).Allocations, 1)
				assert.Equal(t, deadline, controller.RecoveryTargets(deadline)[0].CooldownUntil)
				failedAt = deadline.Add(time.Second)
			}
		})
	}
}

func TestConfiguredPoolsKeepIndependentWindowsAndCooldownsWithSharedObservations(t *testing.T) {
	controller, key, candidates, base := hardFailureRecoveryFixture(t)
	global := controller.config
	global.MaxSamples, global.MaxAge = 2, 10*time.Second
	require.NoError(t, controller.UpdateConfig(global))
	wide := global
	wide.MaxSamples, wide.MaxAge, wide.MinSamples, wide.HardFailureCooldown = 4, time.Minute, 3, 10*time.Second
	wideKey := key
	wideKey.Group = "wide-window"
	_, err := controller.SelectConfiguredAvoiding(wideKey, candidates, nil, base, wide, nil)
	require.NoError(t, err)
	for _, seconds := range []int{1, 2, 3} {
		for _, id := range []int{1, 2} {
			controller.Observe(ObservationKey{ChannelID: id, Model: key.Model}, Sample{ObservedAt: base.Add(time.Duration(seconds) * time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
		}
	}
	narrow := controller.Select(key, candidates, base.Add(20*time.Second))
	assert.ElementsMatch(t, []int{1, 2}, narrow.UnverifiedCandidates)
	verified, err := controller.SelectConfiguredAvoiding(wideKey, candidates, nil, base.Add(20*time.Second), wide, nil)
	require.NoError(t, err)
	assert.Empty(t, verified.UnverifiedCandidates, "reading a short window must not prune a wider pool's samples")
	failedAt := base.Add(21 * time.Second)
	controller.Observe(ObservationKey{ChannelID: 1, Model: key.Model}, Sample{ObservedAt: failedAt, HardFailure: true})
	controller.Select(key, candidates, failedAt)
	_, err = controller.SelectConfiguredAvoiding(wideKey, candidates, nil, failedAt, wide, nil)
	require.NoError(t, err)
	probe, err := controller.SelectConfiguredAvoiding(wideKey, candidates, nil, base.Add(32*time.Second), wide, nil)
	require.NoError(t, err)
	require.Len(t, probe.Allocations, 1)
	assert.Equal(t, failedAt.Add(10*time.Second), controller.RecoveryTargets(base.Add(32 * time.Second))[0].CooldownUntil)
	require.Len(t, controller.Select(key, candidates, base.Add(32*time.Second)).Allocations, 1, "pool tuning must not change the global cooldown")
}

func TestRecoveryObservesSuccessfulProbesBeforeRampingAndKeepsBackoff(t *testing.T) {
	for _, discount := range []bool{false, true} {
		t.Run(fmt.Sprintf("discount=%t", discount), func(t *testing.T) {
			controller, key, candidates, base := hardFailureRecoveryFixture(t)
			if discount {
				cheap, expensive := 0.5, 1.0
				candidates[0].CostDiscount, candidates[1].CostDiscount = &cheap, &expensive
				key.discountFirst = true
			}
			selectAt := func(seconds int) Decision {
				return controller.selectAvoiding(key, candidates, nil, base.Add(time.Duration(seconds)*time.Second), DiscountLimits{MaxTTFT: 5 * time.Second, MaxTPOT: time.Second})
			}
			observationKey := ObservationKey{ChannelID: 1, Model: key.Model}
			controller.Observe(observationKey, Sample{ObservedAt: base.Add(10 * time.Second), HardFailure: true})
			selectAt(10)
			controller.Observe(observationKey, Sample{
				ObservedAt: base.Add(41 * time.Second), UpstreamStartedAt: base.Add(40 * time.Second),
				RecoveryProbe: true, Success: true, HasTTFT: true, TTFT: time.Second,
			})
			for _, seconds := range []int{41, 100, 101} {
				observing := selectAt(seconds)
				require.Equal(t, []Allocation{{ChannelID: 2, Share: 1}}, observing.Allocations)
			}
			controller.Observe(observationKey, Sample{
				ObservedAt: base.Add(102 * time.Second), UpstreamStartedAt: base.Add(101 * time.Second),
				RecoveryProbe: true, Success: true, HasTTFT: true, TTFT: time.Second,
			})
			recovered := selectAt(102)
			require.Len(t, recovered.Allocations, 2)
			assert.InDelta(t, 0.10, recovered.Allocations[0].Share, 1e-9)
			assert.False(t, recovered.Allocations[0].Probe)
			assert.InDelta(t, 0.10, selectAt(103).Allocations[0].Share, 1e-9)
			assert.InDelta(t, 0.20, selectAt(105).Allocations[0].Share, 1e-9)
			controller.Observe(observationKey, Sample{ObservedAt: base.Add(106 * time.Second), HardFailure: true})
			selectAt(106)
			require.Len(t, selectAt(165).Allocations, 1)
			require.Len(t, selectAt(166).Allocations, 1)
			assert.Equal(t, base.Add(166*time.Second), controller.RecoveryTargets(base.Add(166 * time.Second))[0].CooldownUntil)
		})
	}
}

func TestBackoffResetsOnlyAfterRestoredTrafficAndFreshStableSuccess(t *testing.T) {
	controller, key, candidates, base := hardFailureRecoveryFixture(t)
	observationKey := ObservationKey{ChannelID: 1, Model: key.Model}
	controller.Observe(observationKey, Sample{ObservedAt: base.Add(10 * time.Second), HardFailure: true})
	controller.Select(key, candidates, base.Add(10*time.Second))
	for _, seconds := range []int{41, 102} {
		at := base.Add(time.Duration(seconds) * time.Second)
		controller.Observe(observationKey, Sample{RecoveryProbe: true, ObservedAt: at, UpstreamStartedAt: at.Add(-time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
		controller.Select(key, candidates, at)
	}
	for seconds := 105; seconds <= 129; seconds += 3 {
		controller.Select(key, candidates, base.Add(time.Duration(seconds)*time.Second))
	}
	at := base.Add(162 * time.Second)
	controller.Observe(observationKey, Sample{ObservedAt: at, UpstreamStartedAt: at.Add(-time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
	assert.InDelta(t, 0.95, controller.Select(key, candidates, at).Allocations[0].Share, 1e-9)
	controller.Observe(observationKey, Sample{ObservedAt: at.Add(time.Second), HardFailure: true})
	controller.Select(key, candidates, at.Add(time.Second))
	require.Len(t, controller.Select(key, candidates, at.Add(30*time.Second)).Allocations, 1)
	require.Len(t, controller.Select(key, candidates, at.Add(31*time.Second)).Allocations, 1)
	assert.Equal(t, at.Add(31*time.Second), controller.RecoveryTargets(at.Add(31 * time.Second))[0].CooldownUntil)
}

func TestFailedRecoveryRestartsBackoffEvenIfSuccessCompletesBeforeNextSelection(t *testing.T) {
	controller, key, candidates, base := hardFailureRecoveryFixture(t)
	observationKey := ObservationKey{ChannelID: 1, Model: key.Model}
	for _, seconds := range []int{10, 11} {
		at := base.Add(time.Duration(seconds) * time.Second)
		controller.Observe(observationKey, Sample{ObservedAt: at, HardFailure: true})
		controller.Select(key, candidates, at)
	}
	require.Len(t, controller.Select(key, candidates, base.Add(40*time.Second)).Allocations, 1)
	require.Len(t, controller.Select(key, candidates, base.Add(41*time.Second)).Allocations, 1, "in-flight failures extend the current cooldown without doubling it")
	controller.Observe(observationKey, Sample{RecoveryProbe: true, ObservedAt: base.Add(42 * time.Second), UpstreamStartedAt: base.Add(41 * time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
	controller.Select(key, candidates, base.Add(42*time.Second))
	controller.Observe(observationKey, Sample{ObservedAt: base.Add(50 * time.Second), HardFailure: true})
	controller.Observe(observationKey, Sample{ObservedAt: base.Add(51 * time.Second), UpstreamStartedAt: base.Add(50 * time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
	require.Len(t, controller.Select(key, candidates, base.Add(51*time.Second)).Allocations, 1)
	require.Len(t, controller.Select(key, candidates, base.Add(109*time.Second)).Allocations, 1)
	require.Len(t, controller.Select(key, candidates, base.Add(110*time.Second)).Allocations, 1)
	assert.Equal(t, base.Add(110*time.Second), controller.RecoveryTargets(base.Add(110 * time.Second))[0].CooldownUntil)
}

func TestRetuningPreservesObservationWindowAndBackoffWhileTrafficRecovers(t *testing.T) {
	controller, key, candidates, base := hardFailureRecoveryFixture(t)
	observationKey := ObservationKey{ChannelID: 1, Model: key.Model}
	controller.Observe(observationKey, Sample{ObservedAt: base.Add(10 * time.Second), HardFailure: true})
	controller.Select(key, candidates, base.Add(10*time.Second))
	controller.Observe(observationKey, Sample{RecoveryProbe: true, ObservedAt: base.Add(41 * time.Second), UpstreamStartedAt: base.Add(40 * time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
	controller.Select(key, candidates, base.Add(41*time.Second))
	config := controller.config
	config.RecoveryStep = 0.05
	require.NoError(t, controller.UpdateConfig(config))
	candidates[0].Weight = 200
	observing := controller.Select(key, candidates, base.Add(100*time.Second))
	require.Len(t, observing.Allocations, 1)
	assert.Equal(t, 2, observing.Allocations[0].ChannelID)
	assert.InDelta(t, 1, observing.Allocations[0].Share, 1e-9)
	controller.Observe(observationKey, Sample{RecoveryProbe: true, ObservedAt: base.Add(102 * time.Second), UpstreamStartedAt: base.Add(101 * time.Second), Success: true, HasTTFT: true, TTFT: time.Second})
	assert.InDelta(t, 0.05, controller.Select(key, candidates, base.Add(102*time.Second)).Allocations[0].Share, 1e-9)
	config.ProbeFraction = 0.04
	require.NoError(t, controller.UpdateConfig(config))
	candidates[0].Priority++
	assert.LessOrEqual(t, controller.Select(key, candidates, base.Add(103*time.Second)).Allocations[0].Share, 0.150000001)
	controller.Observe(observationKey, Sample{ObservedAt: base.Add(104 * time.Second), HardFailure: true})
	controller.Select(key, candidates, base.Add(104*time.Second))
	require.Len(t, controller.Select(key, candidates, base.Add(163*time.Second)).Allocations, 1)
	probe := controller.Select(key, candidates, base.Add(164*time.Second))
	require.Len(t, probe.Allocations, 1)
	assert.Equal(t, base.Add(164*time.Second), controller.RecoveryTargets(base.Add(164 * time.Second))[0].CooldownUntil)
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
	require.Len(t, afterOriginalDeadline.Allocations, 2)
	for _, allocation := range afterOriginalDeadline.Allocations {
		assert.NotEqual(t, 1, allocation.ChannelID)
	}
	assert.Equal(t, base.Add(40*time.Second), controller.RecoveryTargets(base.Add(40 * time.Second))[0].CooldownUntil)
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
	require.Equal(t, []Allocation{{ChannelID: 2, Share: 1}}, afterDeadline.Allocations)
	assert.Equal(t, base.Add(40*time.Second), controller.RecoveryTargets(base.Add(40 * time.Second))[0].CooldownUntil)
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
