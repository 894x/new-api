package sim

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicPolicyAdaptsControllerAllocationsToSelections(t *testing.T) {
	policy, err := NewDynamicPolicy(dynamicrouting.Config{
		Enabled: true, MaxSamples: 20, MaxAge: time.Minute, MinSamples: 3, ProbeFraction: 0.25,
	}, 41, dynamicrouting.RouteKey{Group: "default", Model: "test-model"})
	require.NoError(t, err)

	candidates := []Candidate{
		{ID: "primary", ChannelID: 1, Priority: 100, Weight: 1},
		{ID: "fallback", ChannelID: 2, Priority: 50, Weight: 1},
	}
	var primary, fallback, probes int
	for i := 0; i < 200; i++ {
		decision := policy.Select(time.Duration(i)*time.Millisecond, candidates)
		switch decision.CandidateID {
		case "primary":
			primary++
		case "fallback":
			fallback++
		}
		if decision.Probe {
			probes++
		}
	}

	assert.Greater(t, primary, fallback)
	assert.Greater(t, fallback, 0)
	assert.Equal(t, fallback, probes)
}

func TestDynamicPolicyMapsControllerDiagnosticsToSimulatorLabels(t *testing.T) {
	policy, err := NewDynamicPolicy(dynamicrouting.Config{
		Enabled: true, MaxSamples: 20, MaxAge: time.Minute, MinSamples: 3, ProbeFraction: 0.05,
		DegradationThreshold: 1.4, CriticalThreshold: 2, CandidateAdvantage: 1.1, Aggressiveness: 0.8,
	}, 17, dynamicrouting.RouteKey{Group: "default", Model: "test-model"})
	require.NoError(t, err)
	candidates := []Candidate{
		{ID: "primary", ChannelID: 1, Priority: 100, Weight: 100},
		{ID: "fallback", ChannelID: 2, Priority: 50, Weight: 100},
	}
	policy.Select(0, candidates)
	for index := 0; index < 3; index++ {
		at := time.Duration(index+1) * time.Second
		policy.Observe(Sample{CandidateID: "primary", CompletedAt: at, TTFT: 100 * time.Millisecond, TPOT: 20 * time.Millisecond, Success: true})
		policy.Observe(Sample{CandidateID: "fallback", CompletedAt: at, TTFT: 120 * time.Millisecond, TPOT: 25 * time.Millisecond, Success: true})
	}
	for index := 0; index < 3; index++ {
		at := time.Duration(index+10) * time.Second
		policy.Observe(Sample{CandidateID: "primary", CompletedAt: at, TTFT: 240 * time.Millisecond, TPOT: 50 * time.Millisecond, Success: true})
		policy.Observe(Sample{CandidateID: "fallback", CompletedAt: at, TTFT: 120 * time.Millisecond, TPOT: 25 * time.Millisecond, Success: true})
	}

	decision := policy.Select(20*time.Second, candidates)
	assert.Equal(t, []string{"primary"}, decision.DegradedCandidates)
}

func TestDynamicPolicySmoothlyBudgetsFivePercentProbeTraffic(t *testing.T) {
	policy, err := NewDynamicPolicy(dynamicrouting.Config{
		Enabled: true, MaxSamples: 20, MaxAge: time.Minute, MinSamples: 3, ProbeFraction: 0.05,
	}, 99, dynamicrouting.RouteKey{Group: "default", Model: "test-model"})
	require.NoError(t, err)
	candidates := []Candidate{
		{ID: "primary", ChannelID: 1, Priority: 100, Weight: 100},
		{ID: "fallback", ChannelID: 2, Priority: 50, Weight: 100},
	}

	probeIndexes := make([]int, 0, 3)
	for index := 0; index < 60; index++ {
		decision := policy.Select(time.Duration(index)*time.Millisecond, candidates)
		if decision.Probe {
			probeIndexes = append(probeIndexes, index)
		}
	}

	assert.Len(t, probeIndexes, 3)
	for index := 1; index < len(probeIndexes); index++ {
		assert.LessOrEqual(t, probeIndexes[index]-probeIndexes[index-1], 20)
	}
}

func TestDynamicPolicyFeedbackLoopVerifiesProbeThenUnloadsDegradedPrimary(t *testing.T) {
	scenario := Scenario{
		Name: "closed-loop", Arrivals: constantArrivals(10, 12*time.Second), OutputTokens: 2,
		Channels: []Channel{
			{ID: "primary", ChannelID: 1, Priority: 100, Weight: 100, Concurrency: 20, Timeline: []Phase{
				{TTFT: 50 * time.Millisecond, TPOT: 5 * time.Millisecond},
				{Start: 6 * time.Second, TTFT: 400 * time.Millisecond, TPOT: 40 * time.Millisecond},
			}},
			{ID: "fallback", ChannelID: 2, Priority: 50, Weight: 100, Concurrency: 20, Timeline: []Phase{{TTFT: 70 * time.Millisecond, TPOT: 8 * time.Millisecond}}},
		},
		SLO:   SLO{TTFT: 150 * time.Millisecond, TPOT: 20 * time.Millisecond},
		Fault: FaultSpec{At: 6 * time.Second, BadChannels: []string{"primary"}, MitigationWindow: 10, MitigatedBadShare: 0.2},
	}
	policy, err := NewDynamicPolicy(dynamicrouting.Config{
		Enabled: true, MaxSamples: 30, MaxAge: time.Minute, MinSamples: 3, ProbeFraction: 0.05,
		DegradationThreshold: 1.3, RecoveryThreshold: 1.1, CriticalThreshold: 2,
		CandidateAdvantage: 1.1, Aggressiveness: 0.9, RecoveryStep: 0.03,
	}, 5, dynamicrouting.RouteKey{Group: "default", Model: "test-model"})
	require.NoError(t, err)

	result, err := Run(scenario, policy)
	require.NoError(t, err)
	preFaultProbes := 0
	firstFallbackDominant := time.Duration(-1)
	postFaultPrimary := 0
	postFaultTotal := 0
	for _, request := range result.Requests {
		if request.ArrivedAt < scenario.Fault.At && request.ChannelID == "fallback" {
			preFaultProbes++
		}
		if request.ArrivedAt >= scenario.Fault.At {
			postFaultTotal++
			if request.ChannelID == "primary" {
				postFaultPrimary++
			}
			if firstFallbackDominant < 0 && request.DominantCandidate == "fallback" {
				firstFallbackDominant = request.ArrivedAt
			}
		}
	}

	assert.GreaterOrEqual(t, preFaultProbes, 3)
	assert.LessOrEqual(t, firstFallbackDominant-scenario.Fault.At, time.Second)
	assert.Less(t, postFaultPrimary, postFaultTotal/2)
}

func TestDynamicPolicyHardFailureEjectsThenReprobesRecoveredPrimary(t *testing.T) {
	scenario := Scenario{
		Name: "hard-recovery", Arrivals: constantArrivals(10, 12*time.Second), OutputTokens: 2,
		Channels: []Channel{
			{ID: "primary", ChannelID: 1, Priority: 100, Weight: 100, Concurrency: 20, Timeline: []Phase{
				{TTFT: 50 * time.Millisecond, TPOT: 5 * time.Millisecond},
				{Start: 2 * time.Second, TTFT: 100 * time.Millisecond, HardFailureRate: 1},
				{Start: 5 * time.Second, TTFT: 50 * time.Millisecond, TPOT: 5 * time.Millisecond},
			}},
			{ID: "fallback", ChannelID: 2, Priority: 50, Weight: 100, Concurrency: 20, Timeline: []Phase{{TTFT: 70 * time.Millisecond, TPOT: 8 * time.Millisecond}}},
		},
		SLO:   SLO{TTFT: 150 * time.Millisecond, TPOT: 20 * time.Millisecond},
		Fault: FaultSpec{At: 2 * time.Second, BadChannels: []string{"primary"}, MitigationWindow: 10, MitigatedBadShare: 0.2},
	}
	policy, err := NewDynamicPolicy(dynamicrouting.Config{
		Enabled: true, MaxSamples: 40, MaxAge: time.Minute, MinSamples: 2, ProbeFraction: 0.1,
		DegradationThreshold: 1.3, RecoveryThreshold: 1.1, CriticalThreshold: 2,
		CandidateAdvantage: 1.1, Aggressiveness: 0.9, RecoveryStep: 0.1,
		HardFailureThreshold: 1, HardFailureCooldown: time.Second,
	}, 7, dynamicrouting.RouteKey{Group: "default", Model: "test-model"})
	require.NoError(t, err)

	result, err := Run(scenario, policy)
	require.NoError(t, err)
	fallbackDominantDuringFailure := false
	primaryDominantAfterRecovery := false
	for _, request := range result.Requests {
		if request.ArrivedAt >= 2500*time.Millisecond && request.ArrivedAt < 5*time.Second && request.DominantCandidate == "fallback" {
			fallbackDominantDuringFailure = true
		}
		if request.ArrivedAt >= 8*time.Second && request.DominantCandidate == "primary" {
			primaryDominantAfterRecovery = true
		}
	}
	assert.True(t, fallbackDominantDuringFailure)
	assert.True(t, primaryDominantAfterRecovery)
	assert.LessOrEqual(t, result.Metrics.RouteReversals, 1)
}

func TestDynamicPolicyMapsUpstreamChannelErrorsToHardFailures(t *testing.T) {
	tests := []struct {
		name    string
		failure FailureKind
	}{
		{name: "hard failure", failure: FailureHard},
		{name: "HTTP 429", failure: FailureHTTP429},
		{name: "HTTP 503", failure: FailureHTTP503},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, err := NewDynamicPolicy(dynamicrouting.Config{
				Enabled: true, MaxSamples: 20, MaxAge: time.Minute, MinSamples: 2, ProbeFraction: 0.1,
				HardFailureThreshold: 1, HardFailureCooldown: time.Second,
			}, 13, dynamicrouting.RouteKey{Group: "default", Model: "test-model"})
			require.NoError(t, err)
			candidates := []Candidate{
				{ID: "primary", ChannelID: 1, Priority: 100, Weight: 100},
				{ID: "fallback", ChannelID: 2, Priority: 50, Weight: 100},
			}

			policy.Select(0, candidates)
			policy.Observe(Sample{
				CandidateID: "primary", ArrivedAt: time.Millisecond, CompletedAt: 10 * time.Millisecond,
				Failure: test.failure,
			})
			decision := policy.Select(20*time.Millisecond, candidates)

			assert.Equal(t, []string{"primary"}, decision.EmergencyCandidates)
			assert.Equal(t, "fallback", decision.CandidateID)
		})
	}
}
