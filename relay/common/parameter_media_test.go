package common

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestPoliciesTransformFinalOverrideUsingMappedModel(t *testing.T) {
	info := &RelayInfo{OriginModelName: "public", ChannelMeta: &ChannelMeta{
		UpstreamModelName: "upstream", ParamOverride: map[string]interface{}{"image_url": "https://media.test/replaced"},
		ChannelOtherSettings: dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{Rules: []dto.ModelParameterCapabilityRule{{
			Selector:   dto.ParameterCapabilitySelector{Type: "exact", Value: "upstream"},
			Parameters: map[string]dto.ParameterCapability{"image_url": {Transform: dto.ParameterTransformImage}},
		}}}},
	}}
	result, err := ApplyRequestPoliciesWithRelayInfo([]byte(`{"image_url":"https://media.test/original"}`), info, func(source, kind string) (string, error) {
		assert.Equal(t, "https://media.test/replaced", source)
		assert.Equal(t, "image", kind)
		return "data:image/png;base64,YQ==", nil
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"image_url":"data:image/png;base64,YQ=="}`, string(result))
	require.Len(t, info.ParameterCapabilityAudit, 1)
	assert.Empty(t, info.ParameterCapabilityAudit[0].From)
}

func TestMediaTransformRejectsGlobalAndChannelPassThrough(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.PassThroughRequestEnabled
	t.Cleanup(func() { settings.PassThroughRequestEnabled = original })
	for _, global := range []bool{false, true} {
		settings.PassThroughRequestEnabled = global
		info := &RelayInfo{OriginModelName: "model", ChannelMeta: &ChannelMeta{
			ChannelSetting:       dto.ChannelSettings{PassThroughBodyEnabled: !global},
			ChannelOtherSettings: dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{"image_url": {Transform: dto.ParameterTransformImage}}}},
		}}
		require.ErrorContains(t, CheckMediaTransformPassThrough(info), "pass-through")
		info.ChannelOtherSettings.ParameterCapabilities.Defaults["image_url"] = dto.ParameterCapability{Transform: dto.ParameterTransformNone}
		require.NoError(t, CheckMediaTransformPassThrough(info))
	}
}
