package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRandomSatisfiedChannelFiltersParameterCapabilitiesBeforePriority(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	disabled := false
	participates := true

	highPriority := &Channel{Id: 731, Status: common.ChannelStatusEnabled}
	highPriority.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{
		Defaults: map[string]dto.ParameterCapability{
			"messages.*.content.*.image_url": {
				Supported:              &disabled,
				ParticipateInSelection: &participates,
			},
		},
	}})
	lowPriority := &Channel{Id: 732, Status: common.ChannelStatusEnabled}

	channelSyncLock.Lock()
	originalGroups := group2model2channels
	originalChannels := channelsIDM
	originalAdvanced := channel2advancedCustomConfig
	originalParameter := channel2parameterCapabilityConfig
	group2model2channels = map[string]map[string][]cachedChannelRouting{
		"default": {"vision-model": {
			{ChannelId: 731, Priority: 100},
			{ChannelId: 732, Priority: 10},
		}},
	}
	channelsIDM = map[int]*Channel{731: highPriority, 732: lowPriority}
	channel2advancedCustomConfig = map[int]*dto.AdvancedCustomConfig{}
	channel2parameterCapabilityConfig = map[int]*dto.ParameterCapabilityConfig{
		731: highPriority.GetOtherSettings().ParameterCapabilities,
	}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		group2model2channels = originalGroups
		channelsIDM = originalChannels
		channel2advancedCustomConfig = originalAdvanced
		channel2parameterCapabilityConfig = originalParameter
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

	request := []byte(`{"model":"vision-model","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`)
	selected, err := GetRandomSatisfiedChannelWithSelectionFilters("default", "vision-model", 0, ChannelSelectionFilters{
		RequestPath: "/v1/chat/completions",
		RequestBody: request,
	})

	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 732, selected.Id)
}

func TestGetRandomSatisfiedChannelReportsUnsupportedParameterCapability(t *testing.T) {
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	disabled := false
	participates := true
	channel := &Channel{Id: 733, Status: common.ChannelStatusEnabled}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{
		Defaults: map[string]dto.ParameterCapability{
			"messages.*.content.*.image_url": {Supported: &disabled, ParticipateInSelection: &participates},
		},
	}})

	channelSyncLock.Lock()
	originalGroups := group2model2channels
	originalChannels := channelsIDM
	originalAdvanced := channel2advancedCustomConfig
	originalParameter := channel2parameterCapabilityConfig
	group2model2channels = map[string]map[string][]cachedChannelRouting{
		"default": {"vision-model": {{ChannelId: 733}}},
	}
	channelsIDM = map[int]*Channel{733: channel}
	channel2advancedCustomConfig = map[int]*dto.AdvancedCustomConfig{}
	channel2parameterCapabilityConfig = map[int]*dto.ParameterCapabilityConfig{733: channel.GetOtherSettings().ParameterCapabilities}
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		channelSyncLock.Lock()
		group2model2channels = originalGroups
		channelsIDM = originalChannels
		channel2advancedCustomConfig = originalAdvanced
		channel2parameterCapabilityConfig = originalParameter
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

	request := []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`)
	selected, err := GetRandomSatisfiedChannelWithSelectionFilters("default", "vision-model", 0, ChannelSelectionFilters{RequestBody: request})

	assert.Nil(t, selected)
	assert.ErrorIs(t, err, ErrParameterCapabilityUnsupported)
	assert.True(t, errors.Is(err, ErrParameterCapabilityUnsupported))
}

func TestGetChannelFiltersParameterCapabilitiesBeforePriorityWithoutMemoryCache(t *testing.T) {
	resetPricingEndpointTestTables(t)
	common.MemoryCacheEnabled = false
	disabled := false
	participates := true
	priorityHigh := int64(100)
	priorityLow := int64(10)
	channels := []*Channel{
		{Id: 741, Name: "no-vision", Key: "key-741", Status: common.ChannelStatusEnabled, Group: "default", Models: "vision-model", Priority: &priorityHigh},
		{Id: 742, Name: "vision", Key: "key-742", Status: common.ChannelStatusEnabled, Group: "default", Models: "vision-model", Priority: &priorityLow},
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{
		Defaults: map[string]dto.ParameterCapability{
			"messages.*.content.*.image_url": {Supported: &disabled, ParticipateInSelection: &participates},
		},
	}})
	for _, channel := range channels {
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}

	request := []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`)
	selected, err := GetChannelWithSelectionFilters("default", "vision-model", 0, ChannelSelectionFilters{RequestBody: request})

	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 742, selected.Id)
}

func TestChannelSelectionParametersResolveCapabilitiesAgainstMappedUpstreamModel(t *testing.T) {
	disabled := false
	participates := true
	mapping := `{"vision-model":"upstream-text-only"}`
	channel := &Channel{Id: 751, ModelMapping: &mapping}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{
		Rules: []dto.ModelParameterCapabilityRule{{
			Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: "upstream-text-only"},
			Parameters: map[string]dto.ParameterCapability{
				"messages.*.content.*.image_url": {Supported: &disabled, ParticipateInSelection: &participates},
			},
		}},
	}})
	request := []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`)

	supported, err := channel.SupportsSelectionParameters("vision-model", request)

	assert.False(t, supported)
	assert.Error(t, err)
}

func TestCacheUpdateChannelRefreshesParameterCapabilities(t *testing.T) {
	resetPricingEndpointTestTables(t)
	disabled := false
	participates := true
	channel := &Channel{Id: 752, Status: common.ChannelStatusEnabled}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{
		Defaults: map[string]dto.ParameterCapability{
			"tools": {Supported: &disabled, ParticipateInSelection: &participates},
		},
	}})

	CacheUpdateChannel(channel)
	require.NotNil(t, channel2parameterCapabilityConfig[752])

	channel.SetOtherSettings(dto.ChannelOtherSettings{})
	CacheUpdateChannel(channel)
	assert.Nil(t, channel2parameterCapabilityConfig[752])
}

func TestCachedParameterFilteringKeepsValidCandidatesWhenAnotherChannelIsMisconfigured(t *testing.T) {
	participates := true
	invalidMapping := "{"
	misconfigured := &Channel{Id: 761, ModelMapping: &invalidMapping}
	valid := &Channel{Id: 762}
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"tools": {ParticipateInSelection: &participates},
	}}

	filtered, violation, configurationErr := filterChannelRoutingsBySelectionParameters(
		[]cachedChannelRouting{{ChannelId: 761}, {ChannelId: 762}},
		map[int]*dto.ParameterCapabilityConfig{761: config},
		map[int]*Channel{761: misconfigured, 762: valid},
		"model-a",
		[]byte(`{"tools":[]}`),
	)

	require.Len(t, filtered, 1)
	assert.Equal(t, 762, filtered[0].ChannelId)
	assert.NoError(t, violation)
	assert.Error(t, configurationErr)
}
