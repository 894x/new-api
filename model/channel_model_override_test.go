package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func clearChannelModelRoutingTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channel_model_overrides").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
}

func createChannelModelRoutingTestChannel(t *testing.T, id int, models string, priority int64, weight uint, status int) *Channel {
	t.Helper()
	channel := &Channel{
		Id:       id,
		Type:     1,
		Key:      "test-key",
		Status:   status,
		Name:     "test-channel",
		Weight:   &weight,
		Models:   models,
		Group:    "default",
		Priority: &priority,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	return channel
}

func TestPatchChannelModelOverridesMaterializesEffectiveAbilityValues(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5101, "model-a,model-b", 10, 20, common.ChannelStatusEnabled)
	zeroPriority := int64(0)
	zeroWeight := uint(0)

	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", Priority: &zeroPriority, Weight: &zeroWeight},
	}))

	var overridden Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&overridden).Error)
	require.NotNil(t, overridden.Priority)
	assert.Equal(t, int64(0), *overridden.Priority)
	assert.Equal(t, uint(0), overridden.Weight)

	var inherited Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-b").First(&inherited).Error)
	require.NotNil(t, inherited.Priority)
	assert.Equal(t, int64(10), *inherited.Priority)
	assert.Equal(t, uint(20), inherited.Weight)

	routings, err := ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	require.Len(t, routings, 2)
	assert.Equal(t, int64(0), routings[0].EffectivePriority)
	assert.Equal(t, uint(0), routings[0].EffectiveWeight)
	assert.NotNil(t, routings[0].PriorityOverride)
	assert.NotNil(t, routings[0].WeightOverride)
}

func TestChannelModelRPMAndTPMOverridesMaterializeEffectiveCapacity(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(10)
	weight := uint(20)
	defaultRPM := int64(60)
	defaultTPM := int64(6000)
	channel := &Channel{
		Id:       5107,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "capacity-channel",
		Weight:   &weight,
		Models:   "model-a,model-b",
		Group:    "default",
		Priority: &priority,
		RPM:      &defaultRPM,
		TPM:      &defaultTPM,
	}
	require.NoError(t, channel.Insert())

	unlimited := int64(0)
	modelTPM := int64(1000)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", RPM: &unlimited, TPM: &modelTPM},
	}))

	var overridden Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&overridden).Error)
	assert.Equal(t, int64(0), overridden.RPM, "an explicit zero override disables the inherited RPM cap")
	assert.Equal(t, int64(1000), overridden.TPM)

	var inherited Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-b").First(&inherited).Error)
	assert.Equal(t, int64(60), inherited.RPM)
	assert.Equal(t, int64(6000), inherited.TPM)

	routings, err := ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	require.Len(t, routings, 2)
	assert.Equal(t, int64(60), routings[0].DefaultRPM)
	assert.Equal(t, int64(6000), routings[0].DefaultTPM)
	assert.NotNil(t, routings[0].RPMOverride)
	assert.Equal(t, int64(0), *routings[0].RPMOverride)
	assert.NotNil(t, routings[0].TPMOverride)
	assert.Equal(t, int64(1000), *routings[0].TPMOverride)
	assert.Equal(t, int64(0), routings[0].EffectiveRPM)
	assert.Equal(t, int64(1000), routings[0].EffectiveTPM)
}

func TestChannelModelCapacityRejectsNegativeDefaultsAndOverrides(t *testing.T) {
	clearChannelModelRoutingTables(t)
	negative := int64(-1)
	priority := int64(1)
	weight := uint(2)
	channel := &Channel{
		Id: 5108, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled,
		Name: "negative-capacity", Models: "model-a", Group: "default", Priority: &priority, Weight: &weight,
		RPM: &negative,
	}
	require.Error(t, channel.Insert())

	channel.RPM = nil
	require.NoError(t, channel.Insert())
	err := PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", TPM: &negative},
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, DB.Model(&ChannelModelOverride{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestChannelUpdatePersistsExplicitZeroCapacityAndRebuildsAbilities(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(10)
	weight := uint(20)
	rpm := int64(60)
	tpm := int64(6000)
	channel := &Channel{
		Id: 5109, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled,
		Name: "capacity-update", Models: "model-a", Group: "default", Priority: &priority, Weight: &weight,
		RPM: &rpm, TPM: &tpm,
	}
	require.NoError(t, channel.Insert())

	unlimited := int64(0)
	channel.RPM = &unlimited
	channel.TPM = &unlimited
	require.NoError(t, channel.Update())

	var persisted Channel
	require.NoError(t, DB.First(&persisted, channel.Id).Error)
	require.NotNil(t, persisted.RPM)
	require.NotNil(t, persisted.TPM)
	assert.Zero(t, *persisted.RPM)
	assert.Zero(t, *persisted.TPM)

	var ability Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&ability).Error)
	assert.Zero(t, ability.RPM)
	assert.Zero(t, ability.TPM)
}

func TestResolveChannelModelRateLimitsUsesExactAndNormalizedOverrides(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(10)
	weight := uint(20)
	defaultRPM := int64(60)
	defaultTPM := int64(6000)
	channel := &Channel{
		Id: 5110, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled,
		Name: "capacity-resolver", Models: "gpt-4o-gizmo-*", Group: "default", Priority: &priority, Weight: &weight,
		RPM: &defaultRPM, TPM: &defaultTPM,
	}
	require.NoError(t, channel.Insert())
	overrideRPM := int64(12)
	overrideTPM := int64(1200)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{{
		ChannelId: channel.Id,
		Model:     "gpt-4o-gizmo-*",
		RPM:       &overrideRPM,
		TPM:       &overrideTPM,
	}}))

	rpm, tpm, err := ResolveChannelModelRateLimits(channel, "gpt-4o-gizmo-2026-08-20")
	require.NoError(t, err)
	assert.Equal(t, int64(12), rpm)
	assert.Equal(t, int64(1200), tpm)

	rpm, tpm, err = ResolveChannelModelRateLimits(channel, "unconfigured-model")
	require.NoError(t, err)
	assert.Equal(t, int64(60), rpm)
	assert.Equal(t, int64(6000), tpm)
}

func TestBatchInsertChannelsRejectsInvalidCapacityBeforeWriting(t *testing.T) {
	clearChannelModelRoutingTables(t)
	negative := int64(-1)
	priority := int64(1)
	weight := uint(1)
	err := BatchInsertChannels([]Channel{{
		Id: 5111, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled,
		Name: "invalid-batch-capacity", Models: "model-a", Group: "default",
		Priority: &priority, Weight: &weight, TPM: &negative,
	}})
	require.ErrorContains(t, err, "channel tpm must not be negative")

	var count int64
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 5111).Count(&count).Error)
	assert.Zero(t, count)
}

func TestChannelDefaultChangesPreserveSparseOverrides(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5104, "model-a,model-b", 10, 20, common.ChannelStatusEnabled)
	overridePriority := int64(0)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", Priority: &overridePriority},
	}))

	newPriority := int64(30)
	newWeight := uint(40)
	channel.Priority = &newPriority
	channel.Weight = &newWeight
	require.NoError(t, channel.Update())

	routings, err := ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	require.Len(t, routings, 2)
	assert.Equal(t, int64(0), routings[0].EffectivePriority)
	assert.Equal(t, uint(40), routings[0].EffectiveWeight)
	assert.Equal(t, int64(30), routings[1].EffectivePriority)
	assert.Equal(t, uint(40), routings[1].EffectiveWeight)
}

func TestChannelUpdateRollsBackDefaultsWhenAbilityRebuildFails(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5105, "model-a", 10, 20, common.ChannelStatusEnabled)
	callbackName := "test:fail_channel_model_ability_create"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("injected ability create failure"))
		}
	}))
	t.Cleanup(func() {
		DB.Callback().Create().Remove(callbackName)
	})

	newPriority := int64(99)
	channel.Priority = &newPriority
	err := channel.Update()
	require.Error(t, err)

	var persisted Channel
	require.NoError(t, DB.First(&persisted, channel.Id).Error)
	assert.Equal(t, int64(10), persisted.GetPriority())
	var ability Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&ability).Error)
	require.NotNil(t, ability.Priority)
	assert.Equal(t, int64(10), *ability.Priority)
}

func TestPatchChannelModelOverridesClearsInheritanceAndPrunesRemovedModels(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5102, "model-a,model-b", 3, 7, common.ChannelStatusEnabled)
	priority := int64(30)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", Priority: &priority},
		{ChannelId: channel.Id, Model: "model-b", Priority: &priority},
	}))

	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a"},
	}))
	var count int64
	require.NoError(t, DB.Model(&ChannelModelOverride{}).
		Where("channel_id = ? AND model = ?", channel.Id, "model-a").Count(&count).Error)
	assert.Zero(t, count)

	channel.Models = "model-a"
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("models", channel.Models).Error)
	require.NoError(t, channel.UpdateAbilities(nil))
	require.NoError(t, DB.Model(&ChannelModelOverride{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ? AND model = ?", channel.Id, "model-b").Count(&count).Error)
	assert.Zero(t, count)
}

func TestPatchChannelModelOverridesRejectsUnsupportedModelAtomically(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5103, "model-a", 1, 2, common.ChannelStatusEnabled)
	priority := int64(9)

	err := PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", Priority: &priority},
		{ChannelId: channel.Id, Model: "missing-model", Priority: &priority},
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, DB.Model(&ChannelModelOverride{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestPatchChannelModelOverridesRejectsOversizedModelNameAtomically(t *testing.T) {
	clearChannelModelRoutingTables(t)
	oversizedModel := strings.Repeat("模", 86)
	channel := createChannelModelRoutingTestChannel(t, 5106, "model-a,"+oversizedModel, 1, 2, common.ChannelStatusEnabled)
	priority := int64(9)

	err := PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", Priority: &priority},
		{ChannelId: channel.Id, Model: oversizedModel, Priority: &priority},
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, DB.Model(&ChannelModelOverride{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
	assert.Zero(t, count)
}

func TestChannelDeletionRemovesModelRoutingOverrides(t *testing.T) {
	tests := []struct {
		name   string
		delete func(t *testing.T, channel *Channel)
	}{
		{
			name: "single delete",
			delete: func(t *testing.T, channel *Channel) {
				require.NoError(t, channel.Delete())
			},
		},
		{
			name: "batch delete",
			delete: func(t *testing.T, channel *Channel) {
				deleted, err := BatchDeleteChannels([]int{channel.Id})
				require.NoError(t, err)
				assert.Equal(t, int64(1), deleted)
			},
		},
		{
			name: "disabled delete",
			delete: func(t *testing.T, channel *Channel) {
				deleted, err := DeleteDisabledChannel()
				require.NoError(t, err)
				assert.Equal(t, int64(1), deleted)
			},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearChannelModelRoutingTables(t)
			channel := createChannelModelRoutingTestChannel(
				t,
				5200+index,
				"model-a",
				1,
				2,
				common.ChannelStatusManuallyDisabled,
			)
			priority := int64(5)
			require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
				{ChannelId: channel.Id, Model: "model-a", Priority: &priority},
			}))

			test.delete(t, channel)

			var count int64
			require.NoError(t, DB.Model(&ChannelModelOverride{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
			assert.Zero(t, count)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestCloneChannelWithModelOverridesCopiesSparseState(t *testing.T) {
	clearChannelModelRoutingTables(t)
	source := createChannelModelRoutingTestChannel(t, 5301, "model-a", 1, 2, common.ChannelStatusEnabled)
	source.Name = "current-source"
	source.Balance = 12.5
	source.UsedQuota = 99
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", source.Id).Updates(map[string]any{
		"name":       source.Name,
		"balance":    source.Balance,
		"used_quota": source.UsedQuota,
	}).Error)
	priority := int64(8)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: source.Id, Model: "model-a", Priority: &priority},
	}))

	clone, err := CloneChannelWithModelOverrides(source.Id, "-clone", true)
	require.NoError(t, err)
	require.NotZero(t, clone.Id)
	assert.Equal(t, "current-source-clone", clone.Name)
	assert.Zero(t, clone.Balance)
	assert.Zero(t, clone.UsedQuota)

	var override ChannelModelOverride
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", clone.Id, "model-a").First(&override).Error)
	require.NotNil(t, override.Priority)
	assert.Equal(t, int64(8), *override.Priority)
	var ability Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", clone.Id, "model-a").First(&ability).Error)
	require.NotNil(t, ability.Priority)
	assert.Equal(t, int64(8), *ability.Priority)
}

func TestCloneChannelWithModelOverridesRollsBackAllState(t *testing.T) {
	clearChannelModelRoutingTables(t)
	source := createChannelModelRoutingTestChannel(t, 5302, "model-a", 1, 2, common.ChannelStatusEnabled)
	priority := int64(8)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: source.Id, Model: "model-a", Priority: &priority},
	}))

	callbackName := "test:fail_cloned_ability_create"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("injected cloned ability failure"))
		}
	}))
	t.Cleanup(func() {
		DB.Callback().Create().Remove(callbackName)
	})

	clone, err := CloneChannelWithModelOverrides(source.Id, "-clone", false)
	require.Error(t, err)
	assert.Nil(t, clone)

	var channelCount int64
	require.NoError(t, DB.Model(&Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
	var overrideCount int64
	require.NoError(t, DB.Model(&ChannelModelOverride{}).Count(&overrideCount).Error)
	assert.Equal(t, int64(1), overrideCount)
}

func TestChannelDefaultWeightLimitRollsBackWrites(t *testing.T) {
	clearChannelModelRoutingTables(t)
	oversized := MaxChannelWeight + 1

	t.Run("insert", func(t *testing.T) {
		channel := &Channel{
			Id: 5310, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled,
			Name: "oversized", Models: "model-a", Group: "default", Weight: &oversized,
		}
		require.Error(t, channel.Insert())
		var count int64
		require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("update", func(t *testing.T) {
		channel := createChannelModelRoutingTestChannel(t, 5311, "model-a", 1, 2, common.ChannelStatusEnabled)
		channel.Weight = &oversized
		require.Error(t, channel.Update())

		var persisted Channel
		require.NoError(t, DB.First(&persisted, channel.Id).Error)
		assert.Equal(t, 2, persisted.GetWeight())
		var ability Ability
		require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
		assert.Equal(t, uint(2), ability.Weight)
	})

	t.Run("override inheritance", func(t *testing.T) {
		channel := &Channel{
			Id: 5312, Type: constant.ChannelTypeOpenAI, Key: "key", Status: common.ChannelStatusEnabled,
			Name: "legacy-oversized", Models: "model-a", Group: "default", Weight: &oversized,
		}
		require.NoError(t, DB.Create(channel).Error)
		priority := int64(9)
		require.Error(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
			{ChannelId: channel.Id, Model: "model-a", Priority: &priority},
		}))
		var count int64
		require.NoError(t, DB.Model(&ChannelModelOverride{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
		assert.Zero(t, count)
	})
}

func TestInitChannelCacheUsesEffectiveModelPriority(t *testing.T) {
	clearChannelModelRoutingTables(t)
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		if originalMemoryCacheEnabled {
			InitChannelCache()
		}
	})

	low := createChannelModelRoutingTestChannel(t, 5401, "model-a,model-b", 1, 100, common.ChannelStatusEnabled)
	high := createChannelModelRoutingTestChannel(t, 5402, "model-a,model-b", 2, 100, common.ChannelStatusEnabled)
	overridePriority := int64(5)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: low.Id, Model: "model-a", Priority: &overridePriority},
	}))
	InitChannelCache()

	selected, err := GetRandomSatisfiedChannel("default", "model-a", 0, "")
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, low.Id, selected.Id)

	selected, err = GetRandomSatisfiedChannel("default", "model-b", 0, "")
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, high.Id, selected.Id)
}

func TestChannelSelectionMatchesEffectivePriorityWithAndWithoutCache(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "database", true: "memory cache"}[memoryCacheEnabled], func(t *testing.T) {
			clearChannelModelRoutingTables(t)
			originalMemoryCacheEnabled := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = memoryCacheEnabled
			t.Cleanup(func() {
				common.MemoryCacheEnabled = originalMemoryCacheEnabled
				if originalMemoryCacheEnabled {
					InitChannelCache()
				}
			})

			high := createChannelModelRoutingTestChannel(t, 5601, "model-a,model-b", 9, 100, common.ChannelStatusEnabled)
			middle := createChannelModelRoutingTestChannel(t, 5602, "model-a,model-b", 5, 100, common.ChannelStatusEnabled)
			low := createChannelModelRoutingTestChannel(t, 5603, "model-a,model-b", -1, 100, common.ChannelStatusEnabled)
			overridePriority := int64(0)
			require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
				{ChannelId: high.Id, Model: "model-a", Priority: &overridePriority},
			}))
			if memoryCacheEnabled {
				InitChannelCache()
			}

			tests := []struct {
				model    string
				retry    int
				expected int
			}{
				{model: "model-a", retry: 0, expected: middle.Id},
				{model: "model-a", retry: 1, expected: high.Id},
				{model: "model-a", retry: 2, expected: low.Id},
				{model: "model-a", retry: 99, expected: low.Id},
				{model: "model-b", retry: 0, expected: high.Id},
				{model: "model-b", retry: 1, expected: middle.Id},
				{model: "model-b", retry: 2, expected: low.Id},
				{model: "model-b", retry: 99, expected: low.Id},
			}
			for _, test := range tests {
				selected, err := GetRandomSatisfiedChannel("default", test.model, test.retry, "")
				require.NoError(t, err)
				require.NotNil(t, selected)
				assert.Equal(t, test.expected, selected.Id)
			}
		})
	}
}

func TestChannelSelectionNormalizesModelWithAndWithoutCache(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "database", true: "memory cache"}[memoryCacheEnabled], func(t *testing.T) {
			clearChannelModelRoutingTables(t)
			originalMemoryCacheEnabled := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = memoryCacheEnabled
			t.Cleanup(func() {
				common.MemoryCacheEnabled = originalMemoryCacheEnabled
				if originalMemoryCacheEnabled {
					InitChannelCache()
				}
			})

			channel := createChannelModelRoutingTestChannel(t, 5701, "gpt-4o-gizmo-*", 1, 100, common.ChannelStatusEnabled)
			if memoryCacheEnabled {
				InitChannelCache()
			}

			selected, err := GetRandomSatisfiedChannel("default", "gpt-4o-gizmo-customer-model", 0, "")
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, channel.Id, selected.Id)
		})
	}
}

func TestChannelSelectionFiltersPathBeforePriorityWithAndWithoutCache(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "database", true: "memory cache"}[memoryCacheEnabled], func(t *testing.T) {
			clearChannelModelRoutingTables(t)
			originalMemoryCacheEnabled := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = memoryCacheEnabled
			t.Cleanup(func() {
				common.MemoryCacheEnabled = originalMemoryCacheEnabled
				if originalMemoryCacheEnabled {
					InitChannelCache()
				}
			})

			high := createChannelModelRoutingTestChannel(t, 5801, "model-a", 9, 100, common.ChannelStatusEnabled)
			low := createChannelModelRoutingTestChannel(t, 5802, "model-a", 5, 100, common.ChannelStatusEnabled)
			for _, configured := range []struct {
				channel *Channel
				path    string
			}{
				{channel: high, path: "/v1/embeddings"},
				{channel: low, path: "/v1/chat/completions"},
			} {
				configured.channel.Type = constant.ChannelTypeAdvancedCustom
				configured.channel.SetOtherSettings(dto.ChannelOtherSettings{
					AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{
						{
							IncomingPath: configured.path,
							UpstreamPath: configured.path,
							Converter:    "none",
							Models:       []string{"model-a"},
						},
					}},
				})
				require.NoError(t, DB.Model(&Channel{}).Where("id = ?", configured.channel.Id).Updates(map[string]any{
					"type":     configured.channel.Type,
					"settings": configured.channel.OtherSettings,
				}).Error)
			}
			if memoryCacheEnabled {
				InitChannelCache()
			}

			selected, err := GetRandomSatisfiedChannel("default", "model-a", 0, "/v1/chat/completions")
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, low.Id, selected.Id)
		})
	}
}

func TestChooseChannelIdByWeightUsesSharedPlusTenSemantics(t *testing.T) {
	abilities := []Ability{
		{ChannelId: 1, Weight: 0},
		{ChannelId: 2, Weight: 10},
	}

	tests := []struct {
		name       string
		randomDraw int
		expected   int
	}{
		{name: "zero weight retains baseline share", randomDraw: 0, expected: 1},
		{name: "first boundary belongs to first channel", randomDraw: 9, expected: 1},
		{name: "second channel starts after baseline share", randomDraw: 10, expected: 2},
		{name: "last draw belongs to second channel", randomDraw: 29, expected: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selected := chooseChannelIdByWeight(abilities, func(max int) int {
				assert.Equal(t, 30, max)
				return test.randomDraw
			})
			assert.Equal(t, test.expected, selected)
		})
	}
}
