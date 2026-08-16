package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeVideoResolutionCanonicalizesPublicRequestValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "label", value: "1080P", want: "1080p"},
		{name: "standard definition dimensions", value: "640x360", want: "360p"},
		{name: "full standard definition dimensions", value: "854x480", want: "480p"},
		{name: "qhd dimensions", value: "960x540", want: "540p"},
		{name: "landscape dimensions", value: "1920x1080", want: "1080p"},
		{name: "portrait dimensions", value: "1080*1920", want: "1080p"},
		{name: "four k dimensions", value: "3840 x 2160", want: "4k"},
		{name: "provider specific dimensions", value: "1792x1024", want: "1792x1024"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeVideoResolution(test.value)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestVideoCapabilityConfigValidateRejectsAmbiguousResolutionRules(t *testing.T) {
	tests := []struct {
		name   string
		config VideoCapabilityConfig
	}{
		{
			name: "empty allowlist",
			config: VideoCapabilityConfig{Models: map[string]VideoModelCapability{
				"video-model": {Resolutions: []string{}},
			}},
		},
		{
			name: "duplicate after normalization",
			config: VideoCapabilityConfig{Models: map[string]VideoModelCapability{
				"video-model": {Resolutions: []string{"1080P", "1920x1080"}},
			}},
		},
		{
			name: "invalid resolution",
			config: VideoCapabilityConfig{Models: map[string]VideoModelCapability{
				"video-model": {Resolutions: []string{"high"}},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, test.config.Validate())
		})
	}
}

func TestVideoCapabilityConfigValidateAcceptsPerModelResolutionAllowlists(t *testing.T) {
	config := VideoCapabilityConfig{Models: map[string]VideoModelCapability{
		"video-model-a": {Resolutions: []string{"720p", "1080p"}},
		"video-model-b": {Resolutions: []string{"1792x1024"}},
	}}

	require.NoError(t, config.Validate())
}

func TestVideoCapabilityConfigSupportsResolutionUsesPerPublicModelAllowlist(t *testing.T) {
	config := &VideoCapabilityConfig{Models: map[string]VideoModelCapability{
		"video-model": {Resolutions: []string{"720p"}},
	}}

	assert.True(t, config.SupportsResolution("video-model", "1280x720"))
	assert.False(t, config.SupportsResolution("video-model", "1080p"))
	assert.True(t, config.SupportsResolution("unconfigured-model", "1080p"))
	assert.True(t, config.SupportsResolution("video-model", ""))
	assert.True(t, (*VideoCapabilityConfig)(nil).SupportsResolution("video-model", "1080p"))
}
