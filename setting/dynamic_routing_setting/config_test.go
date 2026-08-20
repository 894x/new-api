package dynamic_routing_setting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSnapshotUsesTunedDisabledConfiguration(t *testing.T) {
	snapshot := GetSnapshot()

	assert.False(t, snapshot.Config.Enabled)
	assert.Equal(t, 60, snapshot.Config.MaxSamples)
	assert.Equal(t, 90*time.Second, snapshot.Config.MaxAge)
	assert.Equal(t, 3, snapshot.Config.MinSamples)
	assert.InDelta(t, 0.015, snapshot.Config.ProbeFraction, 0.000001)
	assert.InDelta(t, 0.90, snapshot.Config.Aggressiveness, 0.000001)
	assert.InDelta(t, 0.02, snapshot.Config.RecoveryStep, 0.000001)
	assert.Equal(t, 3*time.Second, snapshot.Config.Cooldown)
	assert.Equal(t, 30*time.Second, snapshot.Config.HardFailureCooldown)
}

func TestReplaceAndSyncPublishesOneCompleteValidatedSetting(t *testing.T) {
	original := GetSetting()
	t.Cleanup(func() {
		require.NoError(t, ReplaceAndSync(original))
	})

	next := original
	next.Enabled = true
	next.MaxSamples = 80
	next.MinSamples = 4
	next.ProbeFraction = 0.02
	next.Aggressiveness = 0.95
	before := GetSnapshot()

	require.NoError(t, ReplaceAndSync(next))
	after := GetSnapshot()

	assert.Equal(t, before.Version+1, after.Version)
	assert.Equal(t, next, GetSetting())
	assert.True(t, after.Config.Enabled)
	assert.Equal(t, 80, after.Config.MaxSamples)
	assert.Equal(t, 4, after.Config.MinSamples)
	assert.InDelta(t, 0.02, after.Config.ProbeFraction, 0.000001)
	assert.InDelta(t, 0.95, after.Config.Aggressiveness, 0.000001)
}

func TestReplaceAndSyncRejectsWholeInvalidSettingWithoutPartialPublish(t *testing.T) {
	original := GetSetting()
	before := GetSnapshot()
	invalid := original
	invalid.Enabled = true
	invalid.ProbeFraction = 0
	invalid.MaxSamples = 2
	invalid.MinSamples = 3

	require.Error(t, ReplaceAndSync(invalid))

	assert.Equal(t, original, GetSetting())
	assert.Equal(t, before, GetSnapshot())
}

func TestMergeOptionValuesValidatesInterdependentFieldsAsOneCandidate(t *testing.T) {
	base := GetSetting()
	merged, err := MergeOptionValues(base, map[string]string{
		OptionPrefix + "recovery_threshold":    "1.05",
		OptionPrefix + "degradation_threshold": "1.1",
		OptionPrefix + "critical_threshold":    "1.2",
	})
	require.NoError(t, err)

	assert.InDelta(t, 1.05, merged.RecoveryThreshold, 0.000001)
	assert.InDelta(t, 1.1, merged.DegradationThreshold, 0.000001)
	assert.InDelta(t, 1.2, merged.CriticalThreshold, 0.000001)
	assert.Equal(t, base, GetSetting(), "merging persisted values must not publish them")
}

func TestUpdateAndSyncKeepsLastGoodSnapshotOnInvalidConfiguration(t *testing.T) {
	original := dynamicRoutingSetting
	t.Cleanup(func() {
		dynamicRoutingSetting = original
		require.NoError(t, UpdateAndSync())
	})

	valid := original
	valid.Enabled = true
	dynamicRoutingSetting = valid
	require.NoError(t, UpdateAndSync())
	before := GetSnapshot()
	require.True(t, before.Config.Enabled)

	invalid := valid
	invalid.MinSamples = invalid.MaxSamples + 1
	dynamicRoutingSetting = invalid
	require.Error(t, UpdateAndSync())
	after := GetSnapshot()

	assert.Equal(t, before.Version, after.Version)
	assert.Equal(t, before.Config, after.Config)
}

func TestValidateRejectsAdminZeroValuesThatControllerWouldDefault(t *testing.T) {
	base := GetSetting()
	tests := []struct {
		name   string
		mutate func(*DynamicRoutingSetting)
	}{
		{name: "minimum samples", mutate: func(setting *DynamicRoutingSetting) { setting.MinSamples = 0 }},
		{name: "degradation threshold", mutate: func(setting *DynamicRoutingSetting) { setting.DegradationThreshold = 0 }},
		{name: "recovery threshold", mutate: func(setting *DynamicRoutingSetting) { setting.RecoveryThreshold = 0 }},
		{name: "critical threshold", mutate: func(setting *DynamicRoutingSetting) { setting.CriticalThreshold = 0 }},
		{name: "candidate advantage", mutate: func(setting *DynamicRoutingSetting) { setting.CandidateAdvantage = 0 }},
		{name: "aggressiveness", mutate: func(setting *DynamicRoutingSetting) { setting.Aggressiveness = 0 }},
		{name: "recovery step", mutate: func(setting *DynamicRoutingSetting) { setting.RecoveryStep = 0 }},
		{name: "hard failure threshold", mutate: func(setting *DynamicRoutingSetting) { setting.HardFailureThreshold = 0 }},
		{name: "hard failure cooldown", mutate: func(setting *DynamicRoutingSetting) { setting.HardFailureCooldownSeconds = 0 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			candidate.Enabled = false
			test.mutate(&candidate)
			require.Error(t, Validate(candidate))
		})
	}
}
