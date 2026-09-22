package plugins_test

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func h3Plugin(t *testing.T) *pluginruntime.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("hailuo")
	require.NoError(t, err)
	plugin, err := pluginruntime.NewRegistry().RegisterFactory(source, pluginruntime.Options{Key: "hailuo"})
	require.NoError(t, err)
	return plugin
}

func TestH3PluginCompletionPricingContract(t *testing.T) {
	plugin := h3Plugin(t)
	for _, tc := range []struct {
		name, body           string
		input, output, units float64
		images               *int
	}{
		{"actual images", `{"task":{"status":"succeeded","resolution":"2K","usage":{"input_seconds":2,"output_seconds":4,"input_image_count":7}}}`, 2, 4, 10.4, common.GetPointer(7)},
		{"omitted images retain reservation", `{"task":{"status":"succeeded","resolution":"2K","usage":{"input_seconds":2,"output_seconds":4}}}`, 2, 4, 10.4, nil},
		{"explicit zero removes surcharge", `{"task":{"status":"succeeded","usage":{"input_seconds":2,"output_seconds":4,"input_image_count":0}}}`, 2, 4, 9.6, common.GetPointer(0)},
		{"total fallback", `{"task":{"status":"succeeded","resolution":"768P","usage":{"total_seconds":18,"input_image_count":0}}}`, 3, 15, 18, common.GetPointer(0)},
		{"bounded provider quantities", `{"task":{"status":"succeeded","resolution":"2K","usage":{"input_seconds":1e30,"output_seconds":1e30,"input_image_count":100000}}}`, 3600, 15, 5785.6, common.GetPointer(9)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.body, &body))
			value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", nil, body)
			require.NoError(t, err)
			encoded, err := common.Marshal(value)
			require.NoError(t, err)
			var result struct {
				Usage struct {
					Input, Output, Total float64
					Images               *int `json:"input_images"`
				}
			}
			require.NoError(t, common.Unmarshal(encoded, &result))
			assert.Equal(t, tc.input, result.Usage.Input)
			assert.Equal(t, tc.output, result.Usage.Output)
			assert.Equal(t, tc.input+tc.output, result.Usage.Total)
			assert.Equal(t, tc.images, result.Usage.Images)
			for _, key := range []string{"resolution", "resolution_multiplier"} {
				value, err = plugin.Engine.Call(t.Context(), "extractBillingOnComplete", map[string]any{"data": body}, nil,
					map[string]any{"otherRatios": map[string]any{"seconds": 4, key: 1.6, "image_surcharge": 1.125, "customer_ratio": 0.75}})
				require.NoError(t, err)
				billing := value.(map[string]any)
				assert.InDelta(t, tc.units, billing["modelUnits"], 1e-12)
				consumed, err := common.Marshal(billing["consumedRatios"])
				require.NoError(t, err)
				assert.JSONEq(t, `["seconds","resolution","resolution_multiplier","image_surcharge"]`, string(consumed))
			}
		})
	}
	value, err := plugin.Engine.Call(t.Context(), "extractBillingOnComplete", map[string]any{"data": map[string]any{"task": map[string]any{"status": "succeeded"}}}, nil, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, value, "missing actual usage must not replace the reservation")
}

func TestH3PluginPreservesRequestDefaultsMappingAndFrames(t *testing.T) {
	plugin := h3Plugin(t)
	for _, tc := range []struct {
		name, model, upstream, input, expected string
	}{
		{"defaults", "MiniMax-H3", "MiniMax-H3", `{"prompt":"a boy playing basketball"}`, `{"model":"MiniMax-H3","content":[{"type":"text","text":"a boy playing basketball"}],"duration":5,"resolution":"768P","ratio":"16:9"}`},
		{"mapped vendor", "MiniMax-H3", "vendor-h3", `{"prompt":"mapped model"}`, `{"model":"vendor-h3","content":[{"type":"text","text":"mapped model"}],"duration":5,"resolution":"768P","ratio":"16:9"}`},
		{"reverse alias", "customer-h3", "MiniMax-H3", `{"prompt":"mapped model"}`, `{"model":"MiniMax-H3","content":[{"type":"text","text":"mapped model"}],"duration":5,"resolution":"768P","ratio":"16:9"}`},
		{"frames override ratio", "MiniMax-H3", "MiniMax-H3", `{"prompt":"animate","images":["first.png","last.png"],"metadata":{"ratio":"16:9","aigc_watermark":false,"callback_url":""}}`, `{"model":"MiniMax-H3","content":[{"type":"text","text":"animate"},{"type":"image_url","role":"first_frame","image_url":{"url":"first.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"last.png"}}],"duration":5,"resolution":"768P","ratio":"adaptive","aigc_watermark":false,"callback_url":""}`},
		{"audio reference", "MiniMax-H3", "MiniMax-H3", `{"prompt":"follow the voice","metadata":{"reference_audio":"voice.mp3","resolution":"2K","duration":7}}`, `{"model":"MiniMax-H3","content":[{"type":"text","text":"follow the voice"},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"voice.mp3"}}],"duration":7,"resolution":"2K","ratio":"adaptive"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.input, &request))
			value, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", map[string]any{
				"model": tc.model, "upstreamModel": tc.upstream, "requestBody": request, "baseUrl": "https://provider.example/", "apiKey": "test-key",
			})
			require.NoError(t, err)
			descriptor := value.(map[string]any)
			assert.Equal(t, "https://provider.example/v2/video_generation", descriptor["url"])
			body, err := common.Marshal(descriptor["body"])
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(body))
		})
	}
}

func TestLegacyHailuoPluginPreservesMetadataOverridesAndExplicitFalse(t *testing.T) {
	plugin := h3Plugin(t)
	metadata := map[string]any{"duration": 10, "resolution": "768P", "prompt": "metadata prompt", "prompt_optimizer": false, "fast_pretreatment": false, "aigc_watermark": false, "first_frame_image": "first.png", "last_frame_image": "last.png", "callback_url": "https://callback.example"}
	encoded, err := common.Marshal(metadata)
	require.NoError(t, err)
	for _, value := range []any{metadata, string(encoded)} {
		request, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", map[string]any{"model": "MiniMax-Hailuo-2.3", "upstreamModel": "MiniMax-Hailuo-2.3", "baseUrl": "https://provider.example", "requestBody": map[string]any{"prompt": "original", "duration": 6, "metadata": value}})
		require.NoError(t, err)
		descriptor := request.(map[string]any)
		assert.Equal(t, "https://provider.example/v1/video_generation", descriptor["url"])
		body, err := common.Marshal(descriptor["body"])
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"MiniMax-Hailuo-2.3","prompt":"metadata prompt","duration":10,"resolution":"768P","prompt_optimizer":false,"fast_pretreatment":false,"aigc_watermark":false,"first_frame_image":"first.png","last_frame_image":"last.png","callback_url":"https://callback.example"}`, string(body))
	}
	for _, invalid := range []any{[]any{7}, true} {
		_, err := plugin.Engine.CallPath(context.Background(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "MiniMax-H3", "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "test", "duration": invalid}}})
		require.Error(t, err)
	}
}

func TestHailuoDirectorPreservesDefaultResolution(t *testing.T) {
	plugin := h3Plugin(t)
	for _, size := range []string{"", "unknown-size"} {
		value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "T2V-01-Director", "upstreamModel": "T2V-01-Director", "baseUrl": "https://provider.example",
			"requestBody": map[string]any{"prompt": "animate", "size": size},
		})
		require.NoError(t, err)
		encoded, err := common.Marshal(value.(map[string]any)["body"])
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"T2V-01-Director","prompt":"animate","duration":6,"resolution":"768P"}`, string(encoded))
	}
}

func TestH3PluginStrictNativeBoundsAndSharedValidation(t *testing.T) {
	plugin := h3Plugin(t)
	for _, tc := range []struct {
		name  string
		patch map[string]any
		want  string
	}{
		{"missing duration", map[string]any{"duration": nil}, "duration is required"},
		{"string duration", map[string]any{"duration": "5"}, "between 4 and 15"},
		{"array duration", map[string]any{"duration": []any{5}}, "between 4 and 15"},
		{"fractional duration", map[string]any{"duration": 5.5}, "between 4 and 15"},
		{"excess duration", map[string]any{"duration": 16}, "between 4 and 15"},
		{"negative duration", map[string]any{"duration": -1}, "between 4 and 15"},
		{"missing resolution", map[string]any{"resolution": nil}, "resolution is required"},
		{"wrong resolution", map[string]any{"resolution": "1080P"}, "resolution must be"},
		{"wrong native casing", map[string]any{"resolution": "2k"}, "resolution must be"},
		{"missing text ratio", map[string]any{"ratio": nil}, "ratio is required"},
		{"text adaptive", map[string]any{"ratio": "adaptive"}, "requires an image"},
		{"invalid ratio", map[string]any{"ratio": "16:10"}, "ratio must be one of"},
		{"unknown model", map[string]any{"model": "MiniMax-H3-Max"}, "supports model MiniMax-H3"},
		{"prompt limit", map[string]any{"content": []any{map[string]any{"type": "text", "text": strings.Repeat("界", 7001)}}}, "7000 characters"},
		{"multiple texts", map[string]any{"content": []any{map[string]any{"type": "text", "text": "a"}, map[string]any{"type": "text", "text": "b"}}}, "exactly one text"},
		{"unknown content", map[string]any{"content": []any{map[string]any{"type": "unknown"}}}, "unsupported type"},
		{"empty reference", map[string]any{"content": []any{map[string]any{"type": "text", "text": "a"}, map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{}}}}, "non-empty video_url.url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{"model": "MiniMax-H3", "duration": 5, "resolution": "768P", "ratio": "16:9", "content": []any{map[string]any{"type": "text", "text": "prompt"}}}
			for key, value := range tc.patch {
				request[key] = value
			}
			_, err := plugin.Engine.CallMember(context.Background(), "native", "createH3", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
			require.ErrorContains(t, err, tc.want)
		})
	}
	_, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", map[string]any{"model": "customer-h3", "upstreamModel": "MiniMax-H3", "requestBody": map[string]any{"prompt": "invalid mapped duration", "metadata": map[string]any{"duration": 16}}, "baseUrl": "https://provider.example"})
	require.ErrorContains(t, err, "between 4 and 15")
}

func TestH3PluginQueryRoutesAndTransientFailures(t *testing.T) {
	plugin := h3Plugin(t)
	for _, tc := range []struct{ origin, upstream, expected string }{
		{"MiniMax-H3", "vendor-h3", "https://provider.example/v2/query/video_generation/task%2Fone"},
		{"customer-h3", "MiniMax-H3", "https://provider.example/v2/query/video_generation/task%2Fone"},
		{"MiniMax-Hailuo-2.3", "MiniMax-Hailuo-2.3", "https://provider.example/v1/query/video_generation?task_id=task%2Fone"},
	} {
		value, err := plugin.Engine.Call(context.Background(), "buildQueryRequest", map[string]any{"baseUrl": "https://provider.example", "taskId": "task/one", "apiKey": "poll-key", "requestBody": map[string]any{"origin_model": tc.origin, "model": tc.upstream}})
		require.NoError(t, err)
		assert.Equal(t, tc.expected, value.(map[string]any)["url"])
	}
	for _, status := range []int{408, 429, 500, 529} {
		_, err := plugin.Engine.Call(context.Background(), "parseTaskResult", map[string]any{}, map[string]any{"error": map[string]any{"http_code": status, "message": "temporary failure"}})
		require.ErrorContains(t, err, "temporary failure")
	}
	for _, status := range []string{"failed", "cancelled"} {
		value, err := plugin.Engine.Call(context.Background(), "parseTaskResult", map[string]any{}, map[string]any{"task": map[string]any{"id": "upstream-id", "status": status, "error": map[string]any{"message": "provider failed"}}})
		require.NoError(t, err)
		result := value.(map[string]any)
		assert.Equal(t, "FAILURE", result["status"])
		assert.Equal(t, "provider failed", result["reason"])
	}
}
