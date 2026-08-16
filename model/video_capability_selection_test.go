package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRandomSatisfiedChannelFiltersVideoResolutionBeforePriority(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true

	channel720 := &Channel{Id: 701, Status: common.ChannelStatusEnabled}
	channel720.SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"video-model": {Resolutions: []string{"720p"}}},
	}})
	channel720.Priority = common.GetPointer[int64](100)
	channel1080 := &Channel{Id: 702, Status: common.ChannelStatusEnabled}
	channel1080.SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"video-model": {Resolutions: []string{"1080p"}}},
	}})
	channel1080.Priority = common.GetPointer[int64](10)

	channelSyncLock.Lock()
	originalGroups := group2model2channels
	originalChannels := channelsIDM
	originalAdvanced := channel2advancedCustomConfig
	originalVideo := channel2videoCapabilityConfig
	group2model2channels = map[string]map[string][]cachedChannelRouting{
		"default": {"video-model": {
			{ChannelId: 701, Priority: 100},
			{ChannelId: 702, Priority: 10},
		}},
	}
	channelsIDM = map[int]*Channel{701: channel720, 702: channel1080}
	channel2advancedCustomConfig = map[int]*dto.AdvancedCustomConfig{}
	channel2videoCapabilityConfig = map[int]*dto.VideoCapabilityConfig{
		701: channel720.GetOtherSettings().VideoCapabilities,
		702: channel1080.GetOtherSettings().VideoCapabilities,
	}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		group2model2channels = originalGroups
		channelsIDM = originalChannels
		channel2advancedCustomConfig = originalAdvanced
		channel2videoCapabilityConfig = originalVideo
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

	selected, err := GetRandomSatisfiedChannelWithFilters("default", "video-model", 0, "/v1/video/generations", "1920x1080", nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 702, selected.Id)

	selected, err = GetRandomSatisfiedChannelWithFilters("default", "video-model", 0, "/v1/chat/completions", "", nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 701, selected.Id)
}

func TestGetRandomSatisfiedChannelReportsUnsupportedVideoResolution(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	channel := &Channel{Id: 703, Status: common.ChannelStatusEnabled}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"video-model": {Resolutions: []string{"720p"}}},
	}})

	channelSyncLock.Lock()
	originalGroups := group2model2channels
	originalChannels := channelsIDM
	originalAdvanced := channel2advancedCustomConfig
	originalVideo := channel2videoCapabilityConfig
	group2model2channels = map[string]map[string][]cachedChannelRouting{
		"default": {"video-model": {{ChannelId: 703}}},
	}
	channelsIDM = map[int]*Channel{703: channel}
	channel2advancedCustomConfig = map[int]*dto.AdvancedCustomConfig{}
	channel2videoCapabilityConfig = map[int]*dto.VideoCapabilityConfig{703: channel.GetOtherSettings().VideoCapabilities}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		group2model2channels = originalGroups
		channelsIDM = originalChannels
		channel2advancedCustomConfig = originalAdvanced
		channel2videoCapabilityConfig = originalVideo
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

	selected, err := GetRandomSatisfiedChannelWithFilters("default", "video-model", 0, "/v1/video/generations", "1080p", nil)
	assert.Nil(t, selected)
	assert.ErrorIs(t, err, ErrVideoResolutionUnsupported)
	assert.True(t, errors.Is(err, ErrVideoResolutionUnsupported))
}

func TestGetChannelFiltersVideoResolutionBeforePriorityWithoutMemoryCache(t *testing.T) {
	resetPricingEndpointTestTables(t)
	common.MemoryCacheEnabled = false

	priorityHigh := int64(100)
	priorityLow := int64(10)
	channels := []*Channel{
		{Id: 711, Name: "720-only", Key: "key-711", Status: common.ChannelStatusEnabled, Group: "default", Models: "video-model", Priority: &priorityHigh},
		{Id: 712, Name: "1080-only", Key: "key-712", Status: common.ChannelStatusEnabled, Group: "default", Models: "video-model", Priority: &priorityLow},
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"video-model": {Resolutions: []string{"720p"}}},
	}})
	channels[1].SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"video-model": {Resolutions: []string{"1080p"}}},
	}})
	for _, channel := range channels {
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}

	selected, err := GetChannelWithFilters("default", "video-model", 0, "/v1/video/generations", "1080p", nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 712, selected.Id)
}

func TestCacheUpdateChannelRefreshesVideoCapabilities(t *testing.T) {
	resetPricingEndpointTestTables(t)
	channel := &Channel{Id: 721, Status: common.ChannelStatusEnabled}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"video-model": {Resolutions: []string{"720p"}}},
	}})

	CacheUpdateChannel(channel)
	require.NotNil(t, channel2videoCapabilityConfig[721])
	assert.True(t, channel2videoCapabilityConfig[721].SupportsResolution("video-model", "720p"))
	assert.False(t, channel2videoCapabilityConfig[721].SupportsResolution("video-model", "1080p"))

	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"video-model": {Resolutions: []string{"1080p"}}},
	}})
	CacheUpdateChannel(channel)
	require.NotNil(t, channel2videoCapabilityConfig[721])
	assert.False(t, channel2videoCapabilityConfig[721].SupportsResolution("video-model", "720p"))
	assert.True(t, channel2videoCapabilityConfig[721].SupportsResolution("video-model", "1080p"))

	channel.SetOtherSettings(dto.ChannelOtherSettings{})
	CacheUpdateChannel(channel)
	assert.Nil(t, channel2videoCapabilityConfig[721])
}
