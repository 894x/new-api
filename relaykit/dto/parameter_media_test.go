package dto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParameterMediaRulesInheritAndCanExplicitlyDisable(t *testing.T) {
	config := &ParameterCapabilityConfig{Defaults: map[string]ParameterCapability{"image_url": {Transform: ParameterTransformImage}}, Rules: []ModelParameterCapabilityRule{
		{Selector: ParameterCapabilitySelector{Type: "pattern", Value: "model-*"}, Parameters: map[string]ParameterCapability{"image_url": {OnViolation: "reject"}}},
		{Selector: ParameterCapabilitySelector{Type: "exact", Value: "model-native"}, Parameters: map[string]ParameterCapability{"image_url": {Transform: ParameterTransformNone}}},
	}}
	require.NoError(t, config.Validate())
	assert.Equal(t, ParameterTransformImage, config.Resolve("model-a")["image_url"].Transform)
	assert.False(t, config.HasMediaTransforms("model-native"))
	assert.True(t, config.HasMediaTransforms("model-a"))
}

func TestParameterMediaConfigRejectsUnsafeOrUnknownTransforms(t *testing.T) {
	minimum := 0.0
	for _, tc := range []struct {
		path       string
		capability ParameterCapability
	}{
		{"image_url", ParameterCapability{Transform: "download"}},
		{"max_tokens", ParameterCapability{Transform: ParameterTransformVideo}},
		{"image_url", ParameterCapability{Transform: ParameterTransformImage, Min: &minimum}},
		{"image_url", ParameterCapability{Transform: ParameterTransformImage, AllowedValues: []string{"base64"}}},
	} {
		assert.Error(t, (&ParameterCapabilityConfig{Defaults: map[string]ParameterCapability{tc.path: tc.capability}}).Validate())
	}
}
