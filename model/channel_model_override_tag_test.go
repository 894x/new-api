package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEditChannelByTagRebuildsEffectiveRoutingWithoutLosingOverrides(t *testing.T) {
	clearChannelModelRoutingTables(t)
	tag := "routing-tag"
	channel := createChannelModelRoutingTestChannel(t, 5501, "model-a,model-b", 1, 2, common.ChannelStatusEnabled)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("tag", tag).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Update("tag", tag).Error)
	overridePriority := int64(0)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", Priority: &overridePriority},
	}))

	newDefaultPriority := int64(9)
	newDefaultWeight := uint(7)
	require.NoError(t, EditChannelByTag(tag, nil, nil, nil, nil, &newDefaultPriority, &newDefaultWeight, nil, nil))

	routings, err := ListChannelModelRoutings(channel.Id)
	require.NoError(t, err)
	require.Len(t, routings, 2)
	assert.Equal(t, int64(0), routings[0].EffectivePriority)
	assert.Equal(t, uint(7), routings[0].EffectiveWeight)
	assert.Equal(t, int64(9), routings[1].EffectivePriority)
	assert.Equal(t, uint(7), routings[1].EffectiveWeight)
	var overrideCount int64
	require.NoError(t, DB.Model(&ChannelModelOverride{}).Where("channel_id = ?", channel.Id).Count(&overrideCount).Error)
	assert.Equal(t, int64(1), overrideCount)
}

func TestChannelStatusByTagRollsBackChannelAndAbilityTogether(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus int
		update        func(string) error
	}{
		{
			name:          "disable",
			initialStatus: common.ChannelStatusEnabled,
			update:        DisableChannelByTag,
		},
		{
			name:          "enable",
			initialStatus: common.ChannelStatusManuallyDisabled,
			update:        EnableChannelByTag,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearChannelModelRoutingTables(t)
			tag := "atomic-status-" + test.name
			channel := createChannelModelRoutingTestChannel(t, 5510+index, "model-a", 1, 2, test.initialStatus)
			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("tag", tag).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Updates(map[string]any{
				"tag":     tag,
				"enabled": test.initialStatus == common.ChannelStatusEnabled,
			}).Error)

			callbackName := "test:fail_tag_ability_update_" + test.name
			require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Table == "abilities" {
					tx.AddError(errors.New("injected ability update failure"))
				}
			}))
			t.Cleanup(func() {
				DB.Callback().Update().Remove(callbackName)
			})

			require.Error(t, test.update(tag))
			var persisted Channel
			require.NoError(t, DB.First(&persisted, channel.Id).Error)
			assert.Equal(t, test.initialStatus, persisted.Status)
			var ability Ability
			require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
			assert.Equal(t, test.initialStatus == common.ChannelStatusEnabled, ability.Enabled)
		})
	}
}

func TestEditChannelByTagRejectsOversizedWeightAtomically(t *testing.T) {
	clearChannelModelRoutingTables(t)
	tag := "weight-limit-tag"
	channel := createChannelModelRoutingTestChannel(t, 5520, "model-a", 1, 2, common.ChannelStatusEnabled)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("tag", tag).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Update("tag", tag).Error)
	oversized := MaxChannelWeight + 1

	require.Error(t, EditChannelByTag(tag, nil, nil, nil, nil, nil, &oversized, nil, nil))

	var persisted Channel
	require.NoError(t, DB.First(&persisted, channel.Id).Error)
	assert.Equal(t, 2, persisted.GetWeight())
	var ability Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
	assert.Equal(t, uint(2), ability.Weight)
}

func TestFixAbilityPreservesChannelModelOverrides(t *testing.T) {
	clearChannelModelRoutingTables(t)
	channel := createChannelModelRoutingTestChannel(t, 5502, "model-a", 1, 2, common.ChannelStatusEnabled)
	overridePriority := int64(6)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", Priority: &overridePriority},
	}))

	success, failed, err := FixAbility()
	require.NoError(t, err)
	assert.Equal(t, 1, success)
	assert.Zero(t, failed)

	var override ChannelModelOverride
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&override).Error)
	var ability Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&ability).Error)
	require.NotNil(t, ability.Priority)
	assert.Equal(t, int64(6), *ability.Priority)
}
