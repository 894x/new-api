package sim

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedPolicy struct {
	decisions []Decision
	next      int
}

func (p *scriptedPolicy) Select(_ time.Duration, _ []Candidate) Decision {
	decision := p.decisions[p.next]
	p.next++
	return decision
}

func (p *scriptedPolicy) Observe(Sample) {}

func TestRunReportsUserExperienceAndSwitchingMetrics(t *testing.T) {
	scenario := Scenario{
		Name: "measured-switch", Arrivals: []time.Duration{0, time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 5 * time.Second},
		OutputTokens: 2,
		Channels: []Channel{
			{ID: "bad", ChannelID: 1, Priority: 100, Weight: 1, Concurrency: 2, Timeline: []Phase{
				{TTFT: 50 * time.Millisecond, TPOT: 10 * time.Millisecond},
				{Start: 2 * time.Second, TTFT: 300 * time.Millisecond, TPOT: 50 * time.Millisecond},
			}},
			{ID: "good", ChannelID: 2, Priority: 50, Weight: 1, Concurrency: 2, Timeline: []Phase{{TTFT: 50 * time.Millisecond, TPOT: 10 * time.Millisecond}}},
		},
		SLO: SLO{TTFT: 100 * time.Millisecond, TPOT: 25 * time.Millisecond},
		Fault: FaultSpec{
			At: 2 * time.Second, BadChannels: []string{"bad"}, MitigationWindow: 2, MitigatedBadShare: 0.5,
		},
	}
	policy := &scriptedPolicy{decisions: []Decision{
		{CandidateID: "bad"},
		{CandidateID: "bad"},
		{CandidateID: "bad"},
		{CandidateID: "good", DegradedCandidates: []string{"bad"}},
		{CandidateID: "good", DegradedCandidates: []string{"bad"}},
		{CandidateID: "bad", Probe: true, DegradedCandidates: []string{"bad"}},
	}}

	result, err := Run(scenario, policy)
	require.NoError(t, err)

	assert.Equal(t, 6, result.Metrics.TotalRequests)
	assert.Equal(t, 6, result.Metrics.Successes)
	assert.InDelta(t, 2.0/6.0, result.Metrics.SLOViolationRate, 0.0001)
	assert.InDelta(t, 1.0, result.Metrics.SLOViolationArea, 0.0001)
	assert.Equal(t, 2, result.Metrics.BadChannelExposureAfterFault)
	assert.Equal(t, 1, result.Metrics.BadChannelExposureAfterDetection)
	assert.Equal(t, time.Second, result.Metrics.DetectionDelay)
	assert.Equal(t, 1, result.Metrics.DetectionObservations)
	assert.Equal(t, 2*time.Second, result.Metrics.MitigationDelay)
	assert.Equal(t, 1, result.Metrics.RouteReversals)
	assert.InDelta(t, 1.0/6.0, result.Metrics.ProbeCost, 0.0001)
	assert.Greater(t, result.Metrics.ThroughputPerSecond, 1.0)
}

func TestRouteReversalsTrackDominantAllocationInsteadOfProbeRequests(t *testing.T) {
	scenario := Scenario{
		Name: "probe-not-flap", Arrivals: []time.Duration{0, time.Second, 2 * time.Second}, OutputTokens: 1,
		Channels: []Channel{
			{ID: "primary", ChannelID: 1, Priority: 100, Weight: 1, Concurrency: 1, Timeline: []Phase{{TTFT: time.Millisecond}}},
			{ID: "fallback", ChannelID: 2, Priority: 50, Weight: 1, Concurrency: 1, Timeline: []Phase{{TTFT: time.Millisecond}}},
		},
		SLO: SLO{TTFT: time.Second, TPOT: time.Second},
	}
	policy := &scriptedPolicy{decisions: []Decision{
		{CandidateID: "primary", DominantCandidate: "primary"},
		{CandidateID: "fallback", Probe: true, DominantCandidate: "primary"},
		{CandidateID: "primary", DominantCandidate: "primary"},
	}}

	result, err := Run(scenario, policy)
	require.NoError(t, err)
	assert.Zero(t, result.Metrics.RouteReversals)
}

func TestThroughputUsesFinalCompletionAcrossAllRequests(t *testing.T) {
	requests := []RequestResult{
		{ArrivedAt: 0, CompletedAt: 10 * time.Second, Success: true, TTFT: 10 * time.Second},
		{ArrivedAt: time.Second, CompletedAt: 2 * time.Second, Success: true, TTFT: time.Second},
	}

	metrics := calculateMetrics(requests, Scenario{})

	assert.InDelta(t, 0.2, metrics.ThroughputPerSecond, 0.0001)
}
