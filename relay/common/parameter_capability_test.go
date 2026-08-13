package common

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyRequestPoliciesValidatesFinalOverriddenBody(t *testing.T) {
	max := 1.0
	info := &RelayInfo{
		OriginModelName: "client-model",
		ChannelMeta: &ChannelMeta{
			UpstreamModelName: "upstream-model",
			ParamOverride: map[string]interface{}{
				"temperature": 1.5,
			},
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ParameterCapabilities: &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
					"temperature": {Max: &max, OnViolation: dto.ParameterCapabilityActionClamp},
				}},
			},
		},
	}

	result, err := ApplyRequestPoliciesWithRelayInfo([]byte(`{"temperature":0.2}`), info)
	require.NoError(t, err)
	assert.JSONEq(t, `{"temperature":1}`, string(result))
	require.Len(t, info.ParameterCapabilityAudit, 1)
	assert.Equal(t, "temperature", info.ParameterCapabilityAudit[0].Parameter)
}

func TestApplyRequestPoliciesDoesNotRestoreParameterDeletedByOverride(t *testing.T) {
	max := 1.0
	info := &RelayInfo{
		OriginModelName: "upstream-model",
		ChannelMeta: &ChannelMeta{
			UpstreamModelName: "upstream-model",
			ParamOverride: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"path": "temperature",
						"mode": "delete",
					},
				},
			},
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ParameterCapabilities: &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
					"temperature": {Max: &max, OnViolation: dto.ParameterCapabilityActionClamp},
				}},
			},
		},
	}

	result, err := ApplyRequestPoliciesWithRelayInfo([]byte(`{"model":"upstream-model","temperature":2}`), info)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"upstream-model"}`, string(result))
	assert.Empty(t, info.ParameterCapabilityAudit)
}

func TestApplyParameterCapabilitiesUsesMappedUpstreamModel(t *testing.T) {
	disabled := false
	info := &RelayInfo{
		OriginModelName: "public-model",
		ChannelMeta: &ChannelMeta{
			UpstreamModelName: "vendor-model-v2",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ParameterCapabilities: &dto.ParameterCapabilityConfig{Rules: []dto.ModelParameterCapabilityRule{
					{
						Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: "vendor-model-v2"},
						Parameters: map[string]dto.ParameterCapability{
							"top_k": {Supported: &disabled},
						},
					},
				}},
			},
		},
	}

	_, err := ApplyParameterCapabilitiesWithRelayInfo([]byte(`{"top_k":10}`), info)
	violation, ok := AsParameterCapabilityViolation(err)
	require.True(t, ok)
	assert.Equal(t, "vendor-model-v2", violation.Model)
}

func TestApplyRequestPoliciesRejectsIncompatibleTokenHubKimiTemperatureOverride(t *testing.T) {
	config := &dto.ParameterCapabilityConfig{Rules: []dto.ModelParameterCapabilityRule{
		{
			Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: "kimi-k3"},
			Parameters: map[string]dto.ParameterCapability{
				"temperature": {AllowedValues: []string{"1"}, OnViolation: dto.ParameterCapabilityActionReject},
			},
		},
		{
			Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: "kimi-k2.7-code"},
			Parameters: map[string]dto.ParameterCapability{
				"temperature": {AllowedValues: []string{"1"}, OnViolation: dto.ParameterCapabilityActionReject},
			},
		},
		{
			Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: "kimi-k2.6"},
			Parameters: map[string]dto.ParameterCapability{
				"temperature": {AllowedValues: []string{"0.6", "1"}, OnViolation: dto.ParameterCapabilityActionReject},
			},
		},
		{
			Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: "kimi-k2.5"},
			Parameters: map[string]dto.ParameterCapability{
				"temperature": {AllowedValues: []string{"0.6", "1"}, OnViolation: dto.ParameterCapabilityActionReject},
			},
		},
	}}

	models := []string{"kimi-k3", "kimi-k2.7-code", "kimi-k2.6", "kimi-k2.5"}
	for _, modelName := range models {
		t.Run(modelName, func(t *testing.T) {
			info := &RelayInfo{
				OriginModelName: modelName,
				ChannelMeta: &ChannelMeta{
					UpstreamModelName: modelName,
					ParamOverride: map[string]interface{}{
						"temperature": 1.0,
					},
					ChannelOtherSettings: dto.ChannelOtherSettings{ParameterCapabilities: config},
				},
			}

			result, err := ApplyRequestPoliciesWithRelayInfo([]byte(`{"temperature":0.2}`), info)
			require.NoError(t, err)
			assert.JSONEq(t, `{"temperature":1}`, string(result))

			info.ParamOverride["temperature"] = 0.2
			_, err = ApplyRequestPoliciesWithRelayInfo([]byte(`{"temperature":1}`), info)
			violation, ok := AsParameterCapabilityViolation(err)
			require.True(t, ok)
			assert.Equal(t, modelName, violation.Model)
			assert.Equal(t, "temperature", violation.Parameter)
		})
	}
}

func TestApplyRequestPoliciesClampsInvalidTokenHubKimiTemperature(t *testing.T) {
	fixedTemperature := 1.0
	models := []string{"kimi-k3", "kimi-k2.7-code", "kimi-k2.6", "kimi-k2.5"}
	for _, modelName := range models {
		t.Run(modelName, func(t *testing.T) {
			info := &RelayInfo{
				OriginModelName: modelName,
				ChannelMeta: &ChannelMeta{
					UpstreamModelName: modelName,
					ChannelOtherSettings: dto.ChannelOtherSettings{
						ParameterCapabilities: &dto.ParameterCapabilityConfig{Rules: []dto.ModelParameterCapabilityRule{
							{
								Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: modelName},
								Parameters: map[string]dto.ParameterCapability{
									"temperature": {
										Min:         &fixedTemperature,
										Max:         &fixedTemperature,
										OnViolation: dto.ParameterCapabilityActionClamp,
									},
								},
							},
						}},
					},
				},
			}

			result, err := ApplyRequestPoliciesWithRelayInfo([]byte(`{"temperature":0.2}`), info)
			require.NoError(t, err)
			assert.JSONEq(t, `{"temperature":1}`, string(result))
			require.Len(t, info.ParameterCapabilityAudit, 1)
			assert.Equal(t, dto.ParameterCapabilityActionClamp, info.ParameterCapabilityAudit[0].Action)
			assert.Equal(t, "0.2", info.ParameterCapabilityAudit[0].From)
			assert.Equal(t, "1", info.ParameterCapabilityAudit[0].To)
		})
	}
}
