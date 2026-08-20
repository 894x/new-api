package sim

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedPolicy struct {
	channel  string
	degraded []string
}

func (p *fixedPolicy) Select(_ time.Duration, _ []Candidate) Decision {
	return Decision{CandidateID: p.channel, DominantCandidate: p.channel, DegradedCandidates: p.degraded}
}

func (p *fixedPolicy) Observe(Sample) {}

func TestTuneRanksEveryCandidateAgainstStaticAcrossEverySeed(t *testing.T) {
	scenario := Scenario{
		Name: "clear-regression", Arrivals: constantArrivals(2, 10*time.Second), OutputTokens: 2,
		Channels: []Channel{
			{ID: "bad", ChannelID: 1, Priority: 100, Weight: 100, Concurrency: 20, Timeline: []Phase{{TTFT: 2 * time.Second, TPOT: 100 * time.Millisecond}}},
			{ID: "good", ChannelID: 2, Priority: 50, Weight: 100, Concurrency: 20, Timeline: []Phase{{TTFT: 100 * time.Millisecond, TPOT: 20 * time.Millisecond}}},
		},
		SLO:   SLO{TTFT: 500 * time.Millisecond, TPOT: 50 * time.Millisecond},
		Fault: FaultSpec{At: 0, BadChannels: []string{"bad"}, MitigationWindow: 2, MitigatedBadShare: 0.2},
	}
	parameters := []ParameterSet{{Name: "losing"}, {Name: "winning"}}
	factory := func(parameters ParameterSet, _ int64) (Policy, error) {
		if parameters.Name == "winning" {
			return &fixedPolicy{channel: "good"}, nil
		}
		return &fixedPolicy{channel: "bad"}, nil
	}
	config := TuneConfig{
		Scenarios: []Scenario{scenario}, Seeds: []int64{3, 7}, Parameters: parameters,
		DynamicFactory: factory, SignificantImprovementPercent: 20, MaxRegressionPercent: 5, MinScenarioWinRate: 1,
	}

	report, err := Tune(config)
	require.NoError(t, err)
	require.Len(t, report.Rankings, 2)
	require.Len(t, report.Runs, 4)
	assert.Equal(t, "winning", report.Rankings[0].Parameters.Name)
	assert.True(t, report.Rankings[0].Significant)
	assert.Equal(t, "losing", report.Rankings[1].Parameters.Name)
	assert.False(t, report.Rankings[1].Significant)
	assert.Equal(t, dynamicrouting.Config{}, report.Rankings[1].Parameters.Config)

	repeated, err := Tune(config)
	require.NoError(t, err)
	assert.Equal(t, report, repeated)
}

func TestDefaultParameterGridContainsValidDistinctAggressivenessChoices(t *testing.T) {
	parameters := DefaultParameterGrid()
	require.GreaterOrEqual(t, len(parameters), 6)
	names := make(map[string]struct{}, len(parameters))
	aggressiveness := make(map[float64]struct{})
	for _, candidate := range parameters {
		_, duplicate := names[candidate.Name]
		assert.False(t, duplicate)
		names[candidate.Name] = struct{}{}
		aggressiveness[candidate.Config.Aggressiveness] = struct{}{}
		_, err := dynamicrouting.NewController(candidate.Config)
		assert.NoError(t, err)
	}
	assert.GreaterOrEqual(t, len(aggressiveness), 3)
}

func TestTuneRequiresDedicatedUserExperienceAcceptanceGates(t *testing.T) {
	scenario := func(name string, fault bool) Scenario {
		result := Scenario{
			Name: name, Arrivals: constantArrivals(2, 8*time.Second), OutputTokens: 2,
			Channels: []Channel{
				{ID: "bad", ChannelID: 1, Priority: 100, Weight: 100, Concurrency: 20, Timeline: []Phase{{TTFT: 1200 * time.Millisecond, TPOT: 100 * time.Millisecond}}},
				{ID: "good", ChannelID: 2, Priority: 50, Weight: 100, Concurrency: 20, Timeline: []Phase{{TTFT: 100 * time.Millisecond, TPOT: 20 * time.Millisecond}}},
			},
			SLO: SLO{TTFT: 500 * time.Millisecond, TPOT: 50 * time.Millisecond},
		}
		if fault {
			result.Fault = FaultSpec{At: 0, BadChannels: []string{"bad"}, MitigationWindow: 2, MitigatedBadShare: 0.2}
		}
		if name == "healthy_steady_state" {
			result.Channels[0].Timeline[0] = result.Channels[1].Timeline[0]
		}
		return result
	}
	scenarios := []Scenario{
		scenario("gradual_degradation", true),
		scenario("sudden_outage", true),
		scenario("capacity_aggregation", true),
		scenario("transient_spike", true),
		scenario("recovery_no_flap", true),
		scenario("healthy_steady_state", false),
	}
	factory := func(_ ParameterSet, _ int64) (Policy, error) {
		return &fixedPolicy{channel: "good", degraded: []string{"bad"}}, nil
	}

	report, err := Tune(TuneConfig{
		Scenarios: scenarios, Seeds: []int64{3, 7}, Parameters: []ParameterSet{{Name: "candidate"}},
		DynamicFactory: factory, SignificantImprovementPercent: 15, MaxRegressionPercent: 5,
		AcceptanceThresholds: DefaultAcceptanceThresholds(),
	})
	require.NoError(t, err)
	require.Len(t, report.Rankings, 1)
	acceptance := report.Rankings[0].Acceptance
	assert.True(t, acceptance.AllPassed)
	assert.True(t, acceptance.DegradationTTFT.Passed)
	assert.True(t, acceptance.SLOViolationArea.Passed)
	assert.True(t, acceptance.CapacityThroughput.Passed)
	assert.True(t, acceptance.BadExposure.Passed)
	assert.True(t, acceptance.HealthyTTFT.Passed)
	assert.True(t, acceptance.HealthySuccess.Passed)
	assert.True(t, acceptance.StabilityReversals.Passed)
	assert.True(t, report.Rankings[0].Significant)
}

func TestHealthySuccessRegressionFailsDedicatedGate(t *testing.T) {
	scenario := Scenario{
		Name: "healthy_steady_state", Arrivals: constantArrivals(2, 5*time.Second), OutputTokens: 2,
		Channels: []Channel{
			{ID: "primary", ChannelID: 1, Priority: 100, Weight: 100, Concurrency: 10, Timeline: []Phase{{TTFT: 100 * time.Millisecond, TPOT: 20 * time.Millisecond}}},
			{ID: "failing", ChannelID: 2, Priority: 50, Weight: 100, Concurrency: 10, Timeline: []Phase{{TTFT: 100 * time.Millisecond, HTTP503Rate: 1}}},
		},
		SLO: SLO{TTFT: time.Second, TPOT: time.Second},
	}
	report, err := Tune(TuneConfig{
		Scenarios: []Scenario{scenario}, Seeds: []int64{1}, Parameters: []ParameterSet{{Name: "unsafe"}},
		DynamicFactory:                func(_ ParameterSet, _ int64) (Policy, error) { return &fixedPolicy{channel: "failing"}, nil },
		SignificantImprovementPercent: -100, MaxRegressionPercent: 100,
		AcceptanceThresholds: DefaultAcceptanceThresholds(),
	})
	require.NoError(t, err)
	assert.False(t, report.Rankings[0].Acceptance.HealthySuccess.Passed)
	assert.False(t, report.Rankings[0].Significant)
}
