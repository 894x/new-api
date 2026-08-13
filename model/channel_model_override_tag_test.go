package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
