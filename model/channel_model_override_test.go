package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	priority := int64(8)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: source.Id, Model: "model-a", Priority: &priority},
	}))

	clone := *source
	clone.Id = 0
	clone.Name = "clone"
	require.NoError(t, CloneChannelWithModelOverrides(source.Id, &clone))
	require.NotZero(t, clone.Id)

	var override ChannelModelOverride
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", clone.Id, "model-a").First(&override).Error)
	require.NotNil(t, override.Priority)
	assert.Equal(t, int64(8), *override.Priority)
	var ability Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", clone.Id, "model-a").First(&ability).Error)
	require.NotNil(t, ability.Priority)
	assert.Equal(t, int64(8), *ability.Priority)
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
