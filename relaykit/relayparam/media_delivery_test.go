package relayparam

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	if target == "url" {
		return "https://example.test/video", nil
	}
	return "data:video/mp4;base64,AAAA", nil
}

func TestMessagesVideoDeliveryPreservesNativeSourceAndOtherMedia(t *testing.T) {
	yes, no := true, false
	limit := int64(3)
	const path = "messages.*.content.*.source"
	for _, tc := range []struct {
		name, source, wantSource string
		url, base64              bool
		conversions              int
		wantError                bool
	}{
		{name: "Base64 at decoded limit", source: `{"type":"base64","media_type":"video/mp4","data":"AAAA"}`, base64: true},
		{name: "URL converted to native Base64", source: `{"type":"url","url":"https://example.test/video"}`, wantSource: `{"type":"base64","media_type":"video/mp4","data":"AAAA"}`, base64: true, conversions: 1},
		{name: "Base64 converted to native URL", source: `{"type":"base64","media_type":"video/mp4","data":"AAAA"}`, wantSource: `{"type":"url","url":"https://example.test/video"}`, url: true, conversions: 1},
		{name: "Decoded video exceeds limit", source: `{"type":"base64","media_type":"video/mp4","data":"AAAAAA=="}`, base64: true, wantError: true},
		{name: "Base64 MIME required", source: `{"type":"base64","data":"AAAA"}`, base64: true, wantError: true},
		{name: "Malformed Base64 rejected", source: `{"type":"base64","media_type":"video/mp4","data":"A!=="}`, base64: true, wantError: true},
		{name: "Unknown source type rejected", source: `{"type":"file","file_id":"file-test"}`, base64: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			urlLimit := int64(24)
			base64Limit := limit
			if tc.conversions > 0 {
				base64Limit = 24
			}
			config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{path: {
				ParticipateInSelection: &yes,
				Media: &dto.MediaCapability{Kind: "video", Formats: map[string]dto.MediaFormatCapability{
					"url": {Supported: &tc.url, MaxMediaBytes: &urlLimit}, "base64": {Supported: &tc.base64, MaxMediaBytes: &base64Limit},
				}, Conversions: dto.MediaConversions{URLToBase64: &yes, Base64ToURL: &yes}},
			}}}
			require.NoError(t, config.Validate())
			body := []byte(`{"messages":[{"content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"image-data"}},{"type":"video","fps":2,"source":` + tc.source + `}]}]}`)
			processor := &deliveryTestProcessor{}
			result, changes, err := ApplyMediaDelivery(body, config, "video-model", processor)
			if tc.wantError {
				require.Error(t, err)
				assert.Zero(t, processor.conversions)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.conversions, processor.conversions)
			assert.Len(t, changes, tc.conversions)
			wantSource := tc.wantSource
			if wantSource == "" {
				wantSource = tc.source
			}
			assert.JSONEq(t, `{"messages":[{"content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"image-data"}},{"type":"video","fps":2,"source":`+wantSource+`}]}]}`, string(result))
			_, _, err = ApplyCapabilities(result, config, "video-model")
			require.NoError(t, err)
		})
	}

	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{path: {Supported: &no, ParticipateInSelection: &yes, Media: &dto.MediaCapability{Kind: "video"}}}}
	body := []byte(`{"messages":[{"content":[{"type":"image","source":{"type":"url","url":"https://example.test/image"}},{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"pdf-data"}}]}]}`)
	result, _, err := ApplyMediaDelivery(body, config, "video-model", &deliveryTestProcessor{})
	require.NoError(t, err)
	assert.JSONEq(t, string(body), string(result))
	require.NoError(t, CheckSelectionCapabilities(body, config, "video-model"))
	result, _, err = ApplyCapabilities(body, config, "video-model")
	require.NoError(t, err)
	assert.JSONEq(t, string(body), string(result))
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
