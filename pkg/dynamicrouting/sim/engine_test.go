package sim

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunKeepsArrivalsOpenLoopAndAppliesQueueDelay(t *testing.T) {
	scenario := Scenario{
		Name: "queue-feedback",
		Arrivals: []time.Duration{
			0,
			10 * time.Millisecond,
		},
		OutputTokens: 2,
		Channels: []Channel{
			{
				ID:          "primary",
				Priority:    0,
				Weight:      1,
				Concurrency: 1,
				Timeline: []Phase{
					{
						Start: 0,
						TTFT:  100 * time.Millisecond,
						TPOT:  20 * time.Millisecond,
					},
				},
			},
		},
		SLO: SLO{TTFT: time.Second, TPOT: time.Second},
	}

	result, err := Run(scenario, NewStaticPolicy(7))
	require.NoError(t, err)
	require.Len(t, result.Requests, 2)

	assert.Equal(t, time.Duration(0), result.Requests[0].ArrivedAt)
	assert.Equal(t, 10*time.Millisecond, result.Requests[1].ArrivedAt)
	assert.Equal(t, 100*time.Millisecond, result.Requests[0].TTFT)
	assert.Equal(t, 210*time.Millisecond, result.Requests[1].TTFT)
	assert.Equal(t, 20*time.Millisecond, result.Metrics.P95TPOT)
	assert.Equal(t, 210*time.Millisecond, result.Metrics.P95TTFT)
}

func TestRunAppliesTimeVaryingFaultsAndRecovery(t *testing.T) {
	scenario := Scenario{
		Name:         "outage-recovery",
		Seed:         11,
		Arrivals:     []time.Duration{0, time.Second, 2 * time.Second},
		OutputTokens: 2,
		Channels: []Channel{{
			ID: "primary", Weight: 1, Concurrency: 1,
			Timeline: []Phase{
				{Start: 0, TTFT: 100 * time.Millisecond, TPOT: 20 * time.Millisecond},
				{Start: time.Second, TTFT: 50 * time.Millisecond, TPOT: 20 * time.Millisecond, HTTP503Rate: 1},
				{Start: 2 * time.Second, TTFT: 80 * time.Millisecond, TPOT: 15 * time.Millisecond, LongTailRate: 1, LongTailDelay: 40 * time.Millisecond},
			},
		}},
		SLO: SLO{TTFT: 150 * time.Millisecond, TPOT: 30 * time.Millisecond},
	}

	result, err := Run(scenario, NewStaticPolicy(5))
	require.NoError(t, err)
	require.Len(t, result.Requests, 3)

	assert.True(t, result.Requests[0].Success)
	assert.False(t, result.Requests[1].Success)
	assert.Equal(t, FailureHTTP503, result.Requests[1].Failure)
	assert.True(t, result.Requests[2].Success)
	assert.Equal(t, 120*time.Millisecond, result.Requests[2].TTFT)
}

func TestRunJitterIsBoundedAndRepeatableBySeed(t *testing.T) {
	arrivals := make([]time.Duration, 10)
	for i := range arrivals {
		arrivals[i] = time.Duration(i) * time.Second
	}
	scenario := Scenario{
		Name: "jitter", Seed: 33, Arrivals: arrivals, OutputTokens: 3,
		Channels: []Channel{{
			ID: "primary", Weight: 1, Concurrency: 1,
			Timeline: []Phase{{
				TTFT: 100 * time.Millisecond, TPOT: 20 * time.Millisecond,
				TTFTJitter: 20 * time.Millisecond, TPOTJitter: 5 * time.Millisecond,
			}},
		}},
		SLO: SLO{TTFT: time.Second, TPOT: time.Second},
	}

	first, err := Run(scenario, NewStaticPolicy(2))
	require.NoError(t, err)
	second, err := Run(scenario, NewStaticPolicy(2))
	require.NoError(t, err)
	assert.Equal(t, first.Requests, second.Requests)

	for _, request := range first.Requests {
		assert.GreaterOrEqual(t, request.TTFT, 80*time.Millisecond)
		assert.LessOrEqual(t, request.TTFT, 120*time.Millisecond)
		assert.GreaterOrEqual(t, request.TPOT, 15*time.Millisecond)
		assert.LessOrEqual(t, request.TPOT, 25*time.Millisecond)
	}
}

func TestP95TTFTIncludesUserObservedErrorResponseLatency(t *testing.T) {
	scenario := Scenario{
		Name: "error-latency", Seed: 1, Arrivals: []time.Duration{0, time.Second}, OutputTokens: 2,
		Channels: []Channel{{
			ID: "primary", Weight: 1, Concurrency: 1,
			Timeline: []Phase{
				{TTFT: 100 * time.Millisecond, TPOT: 10 * time.Millisecond},
				{Start: time.Second, TTFT: 700 * time.Millisecond, HTTP503Rate: 1},
			},
		}},
		SLO: SLO{TTFT: time.Second, TPOT: time.Second},
	}

	result, err := Run(scenario, NewStaticPolicy(1))
	require.NoError(t, err)
	assert.Equal(t, 700*time.Millisecond, result.Metrics.P95TTFT)
}
