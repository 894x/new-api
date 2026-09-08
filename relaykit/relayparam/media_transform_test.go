package relayparam

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaTransformsPreserveMediaShapesAndOriginalBody(t *testing.T) {
	for _, tc := range []struct{ name, path, transform, input, uri, want string }{
		{"image object", "messages.*.content.*.image_url", dto.ParameterTransformImage, `{"messages":[{"content":[{"image_url":{"url":"https://media.test/a","detail":"high"}},{"text":"https://leave.test"}]}]}`, "data:image/png;base64,YQ==", `{"messages":[{"content":[{"image_url":{"url":"data:image/png;base64,YQ==","detail":"high"}},{"text":"https://leave.test"}]}]}`},
		{"responses image", "input.*.content.*.image_url", dto.ParameterTransformImage, `{"input":[{"content":[{"image_url":"https://media.test/a"}]}]}`, "data:image/png;base64,YQ==", `{"input":[{"content":[{"image_url":"data:image/png;base64,YQ=="}]}]}`},
		{"audio URL", "audio_url", dto.ParameterTransformAudio, `{"audio_url":{"url":"https://media.test/a"}}`, "data:audio/mpeg;base64,YQ==", `{"audio_url":{"url":"data:audio/mpeg;base64,YQ=="}}`},
		{"input audio", "messages.*.content.*.input_audio", dto.ParameterTransformAudio, `{"messages":[{"content":[{"input_audio":{"data":"https://media.test/a","format":"mp3"}}]}]}`, "data:audio/mpeg;base64,YQ==", `{"messages":[{"content":[{"input_audio":{"data":"YQ==","format":"mp3"}}]}]}`},
		{"video URL", "video_url", dto.ParameterTransformVideo, `{"video_url":{"url":"https://media.test/a","fps":2}}`, "data:video/mp4;base64,YQ==", `{"video_url":{"url":"data:video/mp4;base64,YQ==","fps":2}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{tc.path: {Transform: tc.transform}}}
			require.NoError(t, config.Validate())
			input := []byte(tc.input)
			result, changes, err := ApplyMediaTransforms(input, config, "model", func(source, kind string) (string, error) {
				assert.Equal(t, "https://media.test/a", source)
				assert.Contains(t, tc.uri, "data:"+kind+"/")
				return tc.uri, nil
			})
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(result))
			assert.Equal(t, tc.input, string(input), "channel retry must see original URLs")
			require.Len(t, changes, 1)
			assert.Empty(t, changes[0].From)
			assert.Empty(t, changes[0].To)
		})
	}
}

func TestMediaTransformsSkipMissingInlineAndDisabledRules(t *testing.T) {
	config := &dto.ParameterCapabilityConfig{
		Defaults: map[string]dto.ParameterCapability{"image_url": {Transform: dto.ParameterTransformImage}},
		Rules:    []dto.ModelParameterCapabilityRule{{Selector: dto.ParameterCapabilitySelector{Type: "exact", Value: "native"}, Parameters: map[string]dto.ParameterCapability{"image_url": {Transform: dto.ParameterTransformNone}}}},
	}
	for _, tc := range []struct{ model, input string }{
		{"native", `{"image_url":"https://media.test/a"}`},
		{"model", `{"image_url":"data:image/png;base64,YQ=="}`},
		{"model", `{"text":"https://media.test/a"}`},
	} {
		result, changes, err := ApplyMediaTransforms([]byte(tc.input), config, tc.model, nil)
		require.NoError(t, err)
		assert.Equal(t, tc.input, string(result))
		assert.Empty(t, changes)
	}
}

func TestMediaTransformFailuresAreAtomicAndDoNotLeakSources(t *testing.T) {
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{"image_url": {Transform: dto.ParameterTransformImage}}}
	for _, source := range []string{"file:///etc/passwd", "https://user:secret@media.test/a", "https://media.test/a?token=secret"} {
		result, _, err := ApplyMediaTransforms([]byte(`{"image_url":"`+source+`"}`), config, "model", func(string, string) (string, error) { return "", errors.New("download failed") })
		require.Error(t, err)
		assert.Nil(t, result)
		assert.NotContains(t, err.Error(), source)
		var violation *CapabilityViolationError
		require.ErrorAs(t, err, &violation)
		assert.Empty(t, violation.Value)
	}
}

func TestMediaTransformSelectionDoesNotDownloadOrRejectConvertibleURL(t *testing.T) {
	enabled := true
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{"image_url": {Supported: &enabled, Transform: dto.ParameterTransformImage, ParticipateInSelection: &enabled}}}
	require.NoError(t, CheckSelectionCapabilities([]byte(`{"image_url":"https://media.test/a"}`), config, "model"))
}

func TestInputAudioRejectsMismatchedFormatAndPreservesInlineData(t *testing.T) {
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{"messages.*.content.*.input_audio": {Transform: dto.ParameterTransformAudio}}}
	inline := []byte(`{"messages":[{"content":[{"input_audio":{"data":"YQ==","format":"wav"}}]}]}`)
	result, _, err := ApplyMediaTransforms(inline, config, "model", nil)
	require.NoError(t, err)
	assert.Equal(t, inline, result)
	result, _, err = ApplyMediaTransforms([]byte(`{"messages":[{"content":[{"input_audio":{"data":"https://media.test/audio","format":"wav"}}]}]}`), config, "model", func(string, string) (string, error) { return "data:audio/mpeg;base64,YQ==", nil })
	require.ErrorContains(t, err, "format does not match")
	assert.Nil(t, result)
}
