package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModelChannelCapabilitiesAggregatesEffectiveChannelConfiguration(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(10)
	weight := uint(20)
	mapping := `{"public-model":"mapped-model","mapped-model":"upstream-model"}`
	channel := &Channel{
		Id:           6201,
		Type:         1,
		Key:          "test-key",
		Status:       common.ChannelStatusEnabled,
		Name:         "primary-channel",
		Weight:       &weight,
		Models:       "public-model",
		Group:        "default,vip",
		Priority:     &priority,
		ModelMapping: &mapping,
	}
	supported := true
	unsupported := false
	defaultMax := 2.0
	exactMax := 1.0
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		ParameterCapabilities: &dto.ParameterCapabilityConfig{
			Defaults: map[string]dto.ParameterCapability{
				"temperature": {Supported: &supported, Max: &defaultMax},
			},
			Rules: []dto.ModelParameterCapabilityRule{
				{
					Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: "upstream-model"},
					Parameters: map[string]dto.ParameterCapability{
						"temperature": {Supported: &supported, Max: &exactMax},
						"top_p":       {Supported: &unsupported},
					},
				},
			},
		},
		VideoCapabilities: &dto.VideoCapabilityConfig{
			Models: map[string]dto.VideoModelCapability{
				"public-model": {Resolutions: []string{"720p", "1080p"}},
			},
		},
	})
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	overridePriority := int64(30)
	require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "public-model", Priority: &overridePriority},
	}))
	require.NoError(t, DB.Model(&Ability{}).
		Where("channel_id = ? AND model = ? AND "+commonGroupCol+" = ?", channel.Id, "public-model", "vip").
		Update("enabled", false).Error)

	result, err := ListModelChannelCapabilities("public-model")

	require.NoError(t, err)
	assert.Equal(t, "public-model", result.Model)
	require.Len(t, result.Channels, 1)
	capability := result.Channels[0]
	assert.Equal(t, channel.Id, capability.ChannelId)
	assert.Equal(t, "upstream-model", capability.UpstreamModel)
	assert.True(t, capability.ModelMapped)
	assert.Equal(t, int64(30), capability.EffectivePriority)
	assert.Equal(t, uint(20), capability.EffectiveWeight)
	assert.Equal(t, []ModelChannelCapabilityGroup{
		{Group: "default", Enabled: true},
		{Group: "vip", Enabled: false},
	}, capability.Groups)
	assert.Equal(t, []string{"openai"}, capability.EndpointTypes)
	assert.True(t, capability.ParameterCapabilitiesConfigured)
	require.Contains(t, capability.ParameterCapabilities, "temperature")
	assert.Equal(t, 1.0, *capability.ParameterCapabilities["temperature"].Max)
	require.Contains(t, capability.ParameterCapabilities, "top_p")
	assert.False(t, *capability.ParameterCapabilities["top_p"].Supported)
	assert.True(t, capability.VideoCapabilitiesConfigured)
	assert.Equal(t, []string{"720p", "1080p"}, capability.VideoResolutions)
}

func TestListModelChannelCapabilitiesReportsInvalidReadOnlyConfigurationWithoutPersisting(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(0)
	weight := uint(0)
	invalidSettings := "{invalid"
	channel := &Channel{
		Id:            6202,
		Type:          1,
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Name:          "invalid-channel",
		Weight:        &weight,
		Models:        "public-model",
		Group:         "default",
		Priority:      &priority,
		OtherSettings: invalidSettings,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	result, err := ListModelChannelCapabilities("public-model")

	require.NoError(t, err)
	require.Len(t, result.Channels, 1)
	assert.NotEmpty(t, result.Channels[0].ConfigurationError)
	assert.False(t, result.Channels[0].ParameterCapabilitiesConfigured)
	assert.False(t, result.Channels[0].VideoCapabilitiesConfigured)
	var persisted Channel
	require.NoError(t, DB.First(&persisted, channel.Id).Error)
	assert.Equal(t, invalidSettings, persisted.OtherSettings)
}
