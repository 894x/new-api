package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParameterCapabilityConfigResolveUsesExactRuleAfterPattern(t *testing.T) {
	supported := true
	disabled := false
	defaultMin := 0.0
	patternMax := 2.0
	exactMax := 1.0
	config := &ParameterCapabilityConfig{
		Defaults: map[string]ParameterCapability{
			"temperature": {Supported: &supported, Min: &defaultMin},
		},
		Rules: []ModelParameterCapabilityRule{
			{
				Selector: ParameterCapabilitySelector{Type: ParameterCapabilitySelectorExact, Value: "gpt-5-mini"},
				Parameters: map[string]ParameterCapability{
					"temperature": {Max: &exactMax},
					"top_k":       {Supported: &disabled},
				},
			},
			{
				Selector: ParameterCapabilitySelector{Type: ParameterCapabilitySelectorPattern, Value: "gpt-5*"},
				Parameters: map[string]ParameterCapability{
					"temperature": {Max: &patternMax, OnViolation: ParameterCapabilityActionClamp},
				},
			},
		},
	}

	require.NoError(t, config.Validate())
	resolved := config.Resolve("gpt-5-mini")

	require.NotNil(t, resolved["temperature"].Min)
	require.NotNil(t, resolved["temperature"].Max)
	assert.Equal(t, 0.0, *resolved["temperature"].Min)
	assert.Equal(t, 1.0, *resolved["temperature"].Max)
	assert.Equal(t, ParameterCapabilityActionClamp, resolved["temperature"].OnViolation)
	require.NotNil(t, resolved["top_k"].Supported)
	assert.False(t, *resolved["top_k"].Supported)
}

func TestParameterCapabilityConfigValidateRejectsUnsafeConfiguration(t *testing.T) {
	min := 2.0
	max := 1.0
	disabled := false
	tests := []struct {
		name   string
		config ParameterCapabilityConfig
	}{
		{
			name: "invalid path",
			config: ParameterCapabilityConfig{Defaults: map[string]ParameterCapability{
				"choices.0.text": {},
			}},
		},
		{
			name: "inverted range",
			config: ParameterCapabilityConfig{Defaults: map[string]ParameterCapability{
				"temperature": {Min: &min, Max: &max},
			}},
		},
		{
			name: "clamp without bounds",
			config: ParameterCapabilityConfig{Defaults: map[string]ParameterCapability{
				"temperature": {OnViolation: ParameterCapabilityActionClamp},
			}},
		},
		{
			name: "billing multiplier cannot be clamped after pricing",
			config: ParameterCapabilityConfig{Defaults: map[string]ParameterCapability{
				"max_tokens": {Max: &max, OnViolation: ParameterCapabilityActionClamp},
			}},
		},
		{
			name: "provider-shaped billing multiplier cannot be dropped after pricing",
			config: ParameterCapabilityConfig{Defaults: map[string]ParameterCapability{
				"generationConfig.maxOutputTokens": {Supported: &disabled, OnViolation: ParameterCapabilityActionDrop},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Error(t, test.config.Validate())
		})
	}
}
