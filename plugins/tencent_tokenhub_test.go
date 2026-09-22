package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tokenHubPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("tencent-tokenhub")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "tencent-tokenhub"})
	require.NoError(t, err)
	return plugin
}

func tokenHubCall(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

func TestTokenHubPluginPreservesWireContract(t *testing.T) {
	plugin := tokenHubPlugin(t)
	for _, tc := range []struct{ name, model, request, want string }{
		{
			"provider fields and objects", "yt-video-fx",
			`{"model":"alias","template":"hug","images":[{"url":"https://example.com/portrait.png"}],"bgm":false,"future_provider_field":{"mode":"fast"}}`,
			`{"model":"yt-video-fx","template":"hug","images":[{"url":"https://example.com/portrait.png"}],"bgm":false,"future_provider_field":{"mode":"fast"}}`,
		},
		{
			"metadata precedence and protected identity", "yt-video-humanactor",
			`{"model":"alias","prompt":"local","size":"1080p","id":"attacker","metadata":"{\"model\":\"attacker\",\"id\":\"attacker\",\"prompt\":\"ignored\",\"audio_url\":\"https://example.com/voice.mp3\",\"billing_duration_seconds\":12,\"frame_rate\":50}"}`,
			`{"model":"yt-video-humanactor","prompt":"local","resolution":"1080p","audio_url":"https://example.com/voice.mp3","frame_rate":50}`,
		},
		{
			"OpenAI wrapper images and seconds", "hy-video-1.5",
			`{"model":"alias","seconds":"5","image":"data:image/png;base64,c2Vjb25k","images":["https://example.com/first.png",{"base64":"c2Vjb25k"}]}`,
			`{"model":"hy-video-1.5","duration":5,"image":{"base64":"c2Vjb25k"},"images":[{"url":"https://example.com/first.png"},{"base64":"c2Vjb25k"}]}`,
		},
		{
			"input reference URL", "kl-video-v3",
			`{"model":"alias","input_reference":" https://example.com/first.png ","duration":5}`,
			`{"model":"kl-video-v3","image":{"url":"https://example.com/first.png"},"duration":5}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.request, &request))
			result := tokenHubCall(t, plugin, "buildSubmitRequest", map[string]any{
				"requestBody": request, "upstreamModel": tc.model, "model": "alias",
				"baseUrl": "https://tokenhub.example/", "apiKey": "test-key",
			})
			assert.Equal(t, "https://tokenhub.example/v1/api/video/submit", result["url"])
			assert.Equal(t, "POST", result["method"])
			assert.Equal(t, "Bearer test-key", result["headers"].(map[string]any)["Authorization"])
			encoded, err := common.Marshal(result["body"])
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestTokenHubPluginPreservesPricingVariants(t *testing.T) {
	plugin := tokenHubPlugin(t)
	for _, tc := range []struct {
		model, request string
		want           map[string]any
	}{
		{"hy-video-1.5", `{}`, nil},
		{"yt-video-2.0", `{"resolution":"720p"}`, map[string]any{"resolution": 2.5}},
		{"yt-video-fx", `{"size":"720p"}`, map[string]any{"resolution": float64(2)}},
		{"yt-video-humanactor", `{"metadata":{"billing_duration_seconds":12}}`, map[string]any{"seconds": float64(12), "resolution": float64(2)}},
		{"kl-video-v3", `{"duration":5,"resolution":"1920x1080","sound":true}`, map[string]any{"seconds": float64(5), "resolution": float64(2)}},
		{"kl-video-v3", `{"duration":5,"resolution":"1080p","sound":false,"metadata":{"audio":true}}`, map[string]any{"seconds": float64(5), "resolution": 4.0 / 3}},
		{"kl-video-v2-6", `{"metadata":{"duration_seconds":8,"resolution":"1080p","generate_audio":true,"voice_id":"voice-1"}}`, map[string]any{"seconds": float64(8), "resolution": float64(4)}},
		{"kl-video-v2-6", `{"resolution":"4k"}`, map[string]any{"seconds": float64(5), "resolution": float64(10)}},
		{"kl-video-v2-5-turbo", `{"resolution":"1080p","seconds":"7"}`, map[string]any{"seconds": float64(7), "resolution": 5.0 / 3}},
		{"kl-video-v2-1-master", `{}`, map[string]any{"seconds": float64(5), "resolution": float64(1)}},
		{"kl-video-v2-1", `{"resolution":"1080p"}`, map[string]any{"seconds": float64(5), "resolution": 1.75}},
		{"vd-video-q3-pro", `{"seconds":"16","size":"1280x720"}`, map[string]any{"seconds": float64(16), "resolution": 20.0 / 9}},
		{"vd-video-q3-turbo", `{"resolution":"1080p"}`, map[string]any{"seconds": float64(5), "resolution": 13.0 / 7}},
	} {
		t.Run(tc.model+tc.request, func(t *testing.T) {
			var req map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.request, &req))
			assert.Equal(t, tc.want, tokenHubCall(t, plugin, "extractUsage", map[string]any{
				"upstreamModel": tc.model, "requestBody": req, "usagePurpose": "billing_ratios",
			}))
		})
	}
}

func TestTokenHubPluginBoundsAllDurationPaths(t *testing.T) {
	plugin := tokenHubPlugin(t)
	for _, body := range []string{
		`{"duration":0}`, `{"duration":-1}`, `{"seconds":"3601"}`,
		`{"duration":1.5}`, `{"duration":true}`, `{"duration":"18446744073709551615"}`,
		`{"metadata":{"duration_seconds":3601}}`, `{"metadata":"{\"billing_duration_seconds\":3601}"}`,
	} {
		t.Run(body, func(t *testing.T) {
			var req map[string]any
			require.NoError(t, common.UnmarshalJsonStr(body, &req))
			_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
				"requestBody": req, "upstreamModel": "hy-video-1.5", "baseUrl": "https://tokenhub.example",
			})
			require.ErrorContains(t, err, "between 1 and 3600")
		})
	}
	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"requestBody": map[string]any{}, "upstreamModel": "yt-video-humanactor", "baseUrl": "https://tokenhub.example",
	})
	require.ErrorContains(t, err, "billing_duration_seconds is required")
}

func TestTokenHubPluginQueryAndResultContracts(t *testing.T) {
	plugin := tokenHubPlugin(t)
	query := tokenHubCall(t, plugin, "buildQueryRequest", map[string]any{
		"taskId": "upstream-id", "requestBody": map[string]any{"model": "hy-video-1.5"},
		"baseUrl": "https://tokenhub.example/", "apiKey": "test-key",
	})
	assert.Equal(t, "POST", query["method"])
	assert.Equal(t, "https://tokenhub.example/v1/api/video/query", query["url"])
	assert.Equal(t, map[string]any{"model": "hy-video-1.5", "id": "upstream-id"}, query["body"])
	result := tokenHubCall(t, plugin, "parseTaskResult", nil, map[string]any{
		"status": "completed", "progress": 100, "data": map[string]any{"url": "https://cdn.example/video.mp4"},
	})
	assert.Equal(t, map[string]any{"status": "SUCCESS", "progress": "100%", "url": "https://cdn.example/video.mp4"}, result)
	failure := tokenHubCall(t, plugin, "parseTaskResult", nil, map[string]any{
		"status": "failed", "error": map[string]any{"message": "invalid image"},
	})
	assert.Equal(t, "invalid image", failure["reason"])
	_, err := plugin.Engine.Call(t.Context(), "parseTaskResult", nil, map[string]any{"status": "future"})
	require.ErrorContains(t, err, "unknown Tencent TokenHub task status")
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", nil, map[string]any{"body": map[string]any{}})
	require.ErrorContains(t, err, "missing id")
}

func TestTokenHubResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "tencent-tokenhub", model: "hy-video-1.5",
		requestBody:    map[string]any{"model": "hy-video-1.5", "input": "a paper boat", "seconds": 5},
		wantAction:     "text_to_video",
		wantRequest:    map[string]any{"model": "hy-video-1.5", "prompt": "a paper boat", "seconds": float64(5)},
		wantUsageKeys:  []string{"seconds", "outputResolution", "audioEnabled", "specifiedVoice"},
		wantVendorName: "tencent-tokenhub",
	})
}
