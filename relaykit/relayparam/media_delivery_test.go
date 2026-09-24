package relayparam

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBase64VideoSizeAndValidation(t *testing.T) {
	for _, tc := range []struct {
		payload string
		size    int64
		valid   bool
	}{
		{"AA==", 1, true}, {"AAA=", 2, true}, {"AAAA", 3, true}, {"AA", 1, true}, {"AAA", 2, true},
		{"", 0, false}, {"A", 0, false}, {"AA===", 0, false}, {"A=AA", 0, false}, {"AB==", 0, false}, {"AB", 0, false}, {"A!AA", 0, false}, {"AA\nAA", 0, false},
	} {
		t.Run(tc.payload, func(t *testing.T) {
			info, err := Base64VideoInfo("data:video/mp4;base64," + tc.payload)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.size, info.Bytes)
			assert.True(t, info.Exact)
		})
	}
}

type deliveryTestProcessor struct{ conversions int }

func (p *deliveryTestProcessor) Inspect(source string) (MediaInfo, error) {
	if source == "https://example.test/video" {
		return MediaInfo{Format: "url", Bytes: 24}, nil
	}
	return Base64VideoInfo(source)
}
func (p *deliveryTestProcessor) Convert(source, target string, limit int64) (string, error) {
	p.conversions++
	return "data:video/mp4;base64,AAAA", nil
}

func TestVideoPlanUsesAllFieldsAndNewCapabilityOverridesLegacyTransform(t *testing.T) {
	yes := true
	limit := int64(24)
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{"messages.*.content.*.video_url": {Transform: dto.ParameterTransformVideo, ParticipateInSelection: &yes, Media: &dto.MediaCapability{Formats: map[string]dto.MediaFormatCapability{"url": {Supported: &yes, MaxMediaBytes: &limit}, "base64": {Supported: &yes, MaxMediaBytes: &limit}}}}}}
	body := []byte(`{"messages":[{"content":[{"video_url":{"url":"https://example.test/video","fps":2}},{"video_url":{"url":"data:video/mp4;base64,AAAA"}}]}]}`)
	processor := &deliveryTestProcessor{}
	plans, tier, err := PlanMediaDelivery(body, config, "kimi-k3", processor, true)
	require.NoError(t, err)
	assert.Len(t, plans, 2)
	assert.Zero(t, tier)
	result, changes, err := ApplyMediaDelivery(body, config, "kimi-k3", processor)
	require.NoError(t, err)
	assert.JSONEq(t, string(body), string(result))
	assert.Empty(t, changes)
	result, changes, err = ApplyMediaTransforms(result, config, "kimi-k3", nil)
	require.NoError(t, err)
	assert.JSONEq(t, string(body), string(result))
	assert.Empty(t, changes)
	assert.Zero(t, processor.conversions)
	limit = 2
	_, _, err = PlanMediaDelivery(body, config, "kimi-k3", processor, true)
	assert.Error(t, err, "every video must fit, not just the first")
}
