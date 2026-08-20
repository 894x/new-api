package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapacity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type simulatedChannelCapacity struct {
	channelID int
	limits    channelcapacity.Limits
}

type lateRejectionCapacitySimulation struct {
	successes      int
	deniedAttempts int
}

func simulateLateCapacityRejection(
	t *testing.T,
	channels []simulatedChannelCapacity,
	reservations []int64,
	now time.Time,
) lateRejectionCapacitySimulation {
	t.Helper()
	limiter := channelcapacity.NewMemoryLimiter()
	result := lateRejectionCapacitySimulation{}
	for _, tokens := range reservations {
		for _, channel := range channels {
			decision, err := limiter.Acquire(
				context.Background(),
				channelcapacity.Key{ChannelID: channel.channelID, Model: "simulation-model"},
				channel.limits,
				tokens,
				now,
			)
			require.NoError(t, err)
			if !decision.Allowed {
				result.deniedAttempts++
				continue
			}
			result.successes++
			break
		}
	}
	return result
}

func TestProactiveCapacitySimulationMatchesLatePolicyAggregateAdmissions(t *testing.T) {
	tests := []struct {
		name         string
		highLimits   channelcapacity.Limits
		lowLimits    channelcapacity.Limits
		reservations []int64
		wantHigh     int
		wantLow      int
	}{
		{
			name:         "rpm",
			highLimits:   channelcapacity.Limits{RPM: 2},
			lowLimits:    channelcapacity.Limits{RPM: 3},
			reservations: []int64{1, 1, 1, 1, 1, 1},
			wantHigh:     2,
			wantLow:      3,
		},
		{
			name:         "tpm",
			highLimits:   channelcapacity.Limits{TPM: 100},
			lowLimits:    channelcapacity.Limits{TPM: 150},
			reservations: []int64{60, 50, 90, 40, 20},
			wantHigh:     2,
			wantLow:      2,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupChannelSelectAutoGroupsTest(t)
			configureDynamicRoutingForTest(t, false)
			now := time.Unix(120, 0)
			configureChannelCapacityTestRuntime(t, now)
			modelName := "capacity-simulation-" + test.name
			highID := 6601 + index*10
			lowID := highID + 1
			createPrioritizedChannelSelectFixture(t, model.DB, highID, "default", modelName, 100)
			createPrioritizedChannelSelectFixture(t, model.DB, lowID, "default", modelName, 10)
			setCandidateCapacity(t, highID, modelName, test.highLimits.RPM, test.highLimits.TPM)
			setCandidateCapacity(t, lowID, modelName, test.lowLimits.RPM, test.lowLimits.TPM)
			model.InitChannelCache()

			selectedCounts := map[int]int{}
			localExhaustions := 0
			for _, tokens := range test.reservations {
				retry := 0
				selected, err := runFinalCapacityAdmissionTestRequest(t, &RetryParam{
					Ctx:         newChannelSelectContext(),
					TokenGroup:  "default",
					ModelName:   modelName,
					RequestPath: "/v1/chat/completions",
					Retry:       &retry,
				}, tokens)
				if err != nil {
					var capacityErr *ChannelModelCapacityError
					require.True(t, errors.As(err, &capacityErr))
					localExhaustions++
					continue
				}
				require.NotNil(t, selected)
				selectedCounts[selected.Id]++
				assert.Zero(t, retry)
			}

			lateRejection := simulateLateCapacityRejection(t, []simulatedChannelCapacity{
				{channelID: highID, limits: test.highLimits},
				{channelID: lowID, limits: test.lowLimits},
			}, test.reservations, now)

			assert.Equal(t, test.wantHigh, selectedCounts[highID])
			assert.Equal(t, test.wantLow, selectedCounts[lowID])
			assert.Equal(t, 1, localExhaustions)
			assert.Equal(t, lateRejection.successes, selectedCounts[highID]+selectedCounts[lowID],
				"the selector-side admission model must preserve aggregate accepted requests")
			assert.Positive(t, lateRejection.deniedAttempts,
				"the comparison policy discovers exhausted capacity only at its late check")
		})
	}
}
