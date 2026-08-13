package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateChannelStatusPersistsPartialMultiKeyDisableWithAndWithoutCache(t *testing.T) {
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

			priority := int64(1)
			weight := uint(2)
			channel := &Channel{
				Id:       5701,
				Type:     constant.ChannelTypeOpenAI,
				Key:      "key-a\nkey-b",
				Status:   common.ChannelStatusEnabled,
				Name:     "multi-key-routing",
				Models:   "model-a",
				Group:    "default",
				Priority: &priority,
				Weight:   &weight,
				ChannelInfo: ChannelInfo{
					IsMultiKey:         true,
					MultiKeySize:       2,
					MultiKeyStatusList: map[int]int{},
				},
			}
			require.NoError(t, DB.Create(channel).Error)
			require.NoError(t, channel.AddAbilities(nil))
			if memoryCacheEnabled {
				InitChannelCache()
			}

			assert.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "test"))

			var persisted Channel
			require.NoError(t, DB.First(&persisted, channel.Id).Error)
			assert.Equal(t, common.ChannelStatusEnabled, persisted.Status)
			assert.Equal(t, common.ChannelStatusAutoDisabled, persisted.ChannelInfo.MultiKeyStatusList[0])
			var ability Ability
			require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
			assert.True(t, ability.Enabled)
		})
	}
}

func TestUpdateChannelStatusKeepsAbilityAndCacheInSync(t *testing.T) {
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
			channel := createChannelModelRoutingTestChannel(t, 5702, "model-a", 1, 2, common.ChannelStatusEnabled)
			if memoryCacheEnabled {
				InitChannelCache()
			}

			assert.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "test"))
			var ability Ability
			require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
			assert.False(t, ability.Enabled)
			selected, err := GetRandomSatisfiedChannel("default", "model-a", 0, "")
			require.NoError(t, err)
			assert.Nil(t, selected)

			assert.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "test"))
			require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
			assert.True(t, ability.Enabled)
			selected, err = GetRandomSatisfiedChannel("default", "model-a", 0, "")
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, channel.Id, selected.Id)
		})
	}
}
