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
	paramOverride := `{"temperature":0.2,"operations":[{"description":"cap output length","mode":"set","path":"max_tokens","value":2048,"conditions":[{"path":"original_model","mode":"full","value":"public-model"}],"logic":"AND"}]}`
	channel := &Channel{
		Id:            6201,
		Type:          1,
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Name:          "primary-channel",
		Weight:        &weight,
		Models:        "public-model",
		Group:         "default,vip",
		Priority:      &priority,
		ModelMapping:  &mapping,
		ParamOverride: &paramOverride,
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
	assert.True(t, capability.ParameterOverrideConfigured)
	assert.Equal(t, "mixed", capability.ParameterOverrideMode)
	assert.Equal(t, float64(0.2), capability.ParameterOverrideLegacy["temperature"])
	require.Len(t, capability.ParameterOverrideOperations, 1)
	operation := capability.ParameterOverrideOperations[0]
	assert.Equal(t, 1, operation.Order)
	assert.Equal(t, "cap output length", operation.Description)
	assert.Equal(t, "set", operation.Mode)
	assert.Equal(t, "max_tokens", operation.Path)
	assert.Equal(t, float64(2048), operation.Value)
	assert.True(t, operation.ValueConfigured)
	assert.Equal(t, "AND", operation.Logic)
	require.Len(t, operation.Conditions, 1)
	assert.Equal(t, 1, operation.Conditions[0].Order)
	assert.Equal(t, "original_model", operation.Conditions[0].Path)
	assert.Equal(t, "public-model", operation.Conditions[0].Value)
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
	assert.Equal(t, []string{"openai"}, result.Channels[0].EndpointTypes)
	assert.False(t, result.Channels[0].ParameterCapabilitiesConfigured)
	assert.False(t, result.Channels[0].VideoCapabilitiesConfigured)
	var persisted Channel
	require.NoError(t, DB.First(&persisted, channel.Id).Error)
	assert.Equal(t, invalidSettings, persisted.OtherSettings)
}

func TestListModelChannelCapabilitiesMatchesRuntimeFallbackForMalformedOperations(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(0)
	weight := uint(0)
	paramOverride := `{"operations":[{"path":"max_tokens","value":2048}]}`
	channel := &Channel{
		Id:            6203,
		Type:          1,
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Name:          "legacy-fallback-channel",
		Weight:        &weight,
		Models:        "public-model",
		Group:         "default",
		Priority:      &priority,
		ParamOverride: &paramOverride,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	result, err := ListModelChannelCapabilities("public-model")

	require.NoError(t, err)
	require.Len(t, result.Channels, 1)
	capability := result.Channels[0]
	assert.True(t, capability.ParameterOverrideConfigured)
	assert.Equal(t, "legacy", capability.ParameterOverrideMode)
	assert.Contains(t, capability.ParameterOverrideLegacy, "operations")
	assert.Empty(t, capability.ParameterOverrideOperations)
}

func TestListModelChannelCapabilitiesNormalizesRuntimeOperationDefaultsAndObjectConditions(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(0)
	weight := uint(0)
	paramOverride := `{"operations":[{"mode":"set","path":"max_tokens","value":null,"keep_origin":"yes","logic":5,"conditions":{"original_model":"public-model"}}]}`
	channel := &Channel{
		Id:            6204,
		Type:          1,
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Name:          "object-condition-channel",
		Weight:        &weight,
		Models:        "public-model",
		Group:         "default",
		Priority:      &priority,
		ParamOverride: &paramOverride,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	result, err := ListModelChannelCapabilities("public-model")

	require.NoError(t, err)
	require.Len(t, result.Channels, 1)
	capability := result.Channels[0]
	assert.Equal(t, "operations", capability.ParameterOverrideMode)
	require.Len(t, capability.ParameterOverrideOperations, 1)
	operation := capability.ParameterOverrideOperations[0]
	assert.True(t, operation.ValueConfigured)
	assert.Nil(t, operation.Value)
	assert.False(t, operation.KeepOrigin)
	assert.Equal(t, "OR", operation.Logic)
	require.Len(t, operation.Conditions, 1)
	assert.Equal(t, "original_model", operation.Conditions[0].Path)
	assert.Equal(t, "full", operation.Conditions[0].Mode)
	assert.Equal(t, "public-model", operation.Conditions[0].Value)
}

func TestListModelChannelCapabilitiesFallsBackToLegacyForInvalidConditions(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(0)
	weight := uint(0)
	paramOverride := `{"operations":[{"mode":"set","path":"max_tokens","conditions":[{"path":"","mode":"full"}]}]}`
	channel := &Channel{
		Id:            6205,
		Type:          1,
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Name:          "invalid-condition-channel",
		Weight:        &weight,
		Models:        "public-model",
		Group:         "default",
		Priority:      &priority,
		ParamOverride: &paramOverride,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	result, err := ListModelChannelCapabilities("public-model")

	require.NoError(t, err)
	require.Len(t, result.Channels, 1)
	capability := result.Channels[0]
	assert.Equal(t, "legacy", capability.ParameterOverrideMode)
	assert.Contains(t, capability.ParameterOverrideLegacy, "operations")
	assert.Empty(t, capability.ParameterOverrideOperations)
}

func TestListModelChannelCapabilitiesRedactsSensitiveOverrideValues(t *testing.T) {
	clearChannelModelRoutingTables(t)
	priority := int64(0)
	weight := uint(0)
	paramOverride := `{"api_key":"legacy-secret","operations":[{"mode":"set_header","path":"Authorization","value":"Bearer upstream-secret"},{"mode":"set","path":"credentials.token","value":"nested-secret","conditions":[{"path":"request_headers.Authorization","mode":"full","value":"Bearer client-secret"}]},{"mode":"set","path":"max_tokens","value":2048}]}`
	channel := &Channel{
		Id:            6206,
		Type:          1,
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Name:          "sensitive-override-channel",
		Weight:        &weight,
		Models:        "public-model",
		Group:         "default",
		Priority:      &priority,
		ParamOverride: &paramOverride,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	result, err := ListModelChannelCapabilities("public-model")

	require.NoError(t, err)
	require.Len(t, result.Channels, 1)
	capability := result.Channels[0]
	assert.Equal(t, "[REDACTED]", capability.ParameterOverrideLegacy["api_key"])
	require.Len(t, capability.ParameterOverrideOperations, 3)
	assert.Equal(t, "[REDACTED]", capability.ParameterOverrideOperations[0].Value)
	assert.Equal(t, "[REDACTED]", capability.ParameterOverrideOperations[1].Value)
	require.Len(t, capability.ParameterOverrideOperations[1].Conditions, 1)
	assert.Equal(t, "[REDACTED]", capability.ParameterOverrideOperations[1].Conditions[0].Value)
	assert.Equal(t, float64(2048), capability.ParameterOverrideOperations[2].Value)
	responseJSON, err := common.Marshal(capability)
	require.NoError(t, err)
	assert.NotContains(t, string(responseJSON), "legacy-secret")
	assert.NotContains(t, string(responseJSON), "upstream-secret")
	assert.NotContains(t, string(responseJSON), "nested-secret")
	assert.NotContains(t, string(responseJSON), "client-secret")
}
