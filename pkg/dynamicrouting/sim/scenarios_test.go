package sim

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinScenariosCoverRequiredFailureModes(t *testing.T) {
	scenarios := BuiltinScenarios()
	want := []string{
		"gradual_degradation",
		"sudden_outage",
		"capacity_aggregation",
		"transient_spike",
		"stale_candidate",
		"recovery_no_flap",
		"all_channels_bad",
		"low_traffic",
		"healthy_steady_state",
	}

	require.Len(t, scenarios, len(want))
	for i, scenario := range scenarios {
		assert.Equal(t, want[i], scenario.Name)
		assert.NotEmpty(t, scenario.Arrivals)
		assert.GreaterOrEqual(t, len(scenario.Channels), 2)
		if scenario.Name == "healthy_steady_state" {
			assert.Empty(t, scenario.Fault.BadChannels)
		} else {
			assert.NotEmpty(t, scenario.Fault.BadChannels)
		}
		for j := 1; j < len(scenario.Arrivals); j++ {
			assert.Greater(t, scenario.Arrivals[j], scenario.Arrivals[j-1])
		}
	}

	assert.Greater(t, len(scenarios[0].Channels[0].Timeline), 2)
	assert.Less(t, len(scenarios[7].Arrivals), len(scenarios[0].Arrivals)/10)
}
