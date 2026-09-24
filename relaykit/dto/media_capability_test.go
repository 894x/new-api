package dto

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMediaCapabilityInheritancePreservesFormatsAndExplicitFalse(t *testing.T) {
	yes, no := true, false
	urlLimit, baseLimit := int64(500), int64(20)
	const path = "messages.*.content.*.video_url"
	config := &ParameterCapabilityConfig{Defaults: map[string]ParameterCapability{path: {Media: &MediaCapability{Kind: "video", Formats: map[string]MediaFormatCapability{"url": {Supported: &yes, MaxMediaBytes: &urlLimit}, "base64": {Supported: &yes, MaxMediaBytes: &baseLimit}}, Conversions: MediaConversions{URLToBase64: &yes, Base64ToURL: &yes}}}}, Rules: []ModelParameterCapabilityRule{
		{Selector: ParameterCapabilitySelector{Type: "pattern", Value: "kimi-*"}, Parameters: map[string]ParameterCapability{path: {Media: &MediaCapability{Conversions: MediaConversions{Base64ToURL: &no}}}}},
		{Selector: ParameterCapabilitySelector{Type: "exact", Value: "kimi-k3"}, Parameters: map[string]ParameterCapability{path: {Media: &MediaCapability{Formats: map[string]MediaFormatCapability{"base64": {Supported: &no}}}}}},
	}}
	require.NoError(t, config.Validate())
	resolved := config.Resolve("kimi-k3")[path].Media
	assert.Equal(t, int64(500), *resolved.Formats["url"].MaxMediaBytes)
	assert.Equal(t, int64(20), *resolved.Formats["base64"].MaxMediaBytes)
	assert.False(t, *resolved.Formats["base64"].Supported)
	assert.True(t, *resolved.Conversions.URLToBase64)
	assert.False(t, *resolved.Conversions.Base64ToURL)
	assert.True(t, *config.Defaults[path].Media.Formats["base64"].Supported)
	assert.True(t, *config.Defaults[path].Media.Conversions.Base64ToURL)
	zero := int64(0)
	config.Defaults[path] = ParameterCapability{Media: &MediaCapability{Formats: map[string]MediaFormatCapability{"url": {MaxMediaBytes: &zero}}}}
	assert.Error(t, config.Validate(), "zero cannot silently remove the limit")
}
