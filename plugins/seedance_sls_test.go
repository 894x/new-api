package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedanceSLSPlugin(t *testing.T) *pluginruntime.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("seedance-sls")
	require.NoError(t, err)
	plugin, err := pluginruntime.NewRegistry().RegisterFactory(source, pluginruntime.Options{Key: "seedance-sls"})
	require.NoError(t, err)
	return plugin
}

func TestSeedanceSLSPluginAutomaticDurationAndWireContract(t *testing.T) {
	plugin := seedanceSLSPlugin(t)
	const raw = `{"model":"public-alias","content":[{"type":"text","text":"first","weight":0},{"type":"text","text":"second"}],"duration":-1,"resolution":"720p","seed":0,"generate_audio":false,"future":{"enabled":false}}`
	var request map[string]any
	require.NoError(t, common.UnmarshalJsonStr(raw, &request))
	value, err := plugin.Engine.CallMember(t.Context(), "native", "createTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
	require.NoError(t, err)
	intent := value.(map[string]any)
	canonical := intent["requestBody"].(map[string]any)
	assert.Equal(t, true, canonical["automaticDuration"])
	assert.NotContains(t, canonical["payload"], "duration", "automatic mode must not reach generic billing as a negative duration")
	ctx := map[string]any{"model": "doubao-seedance-2-5-260628", "upstreamModel": "doubao-seedance-2-5-260628", "requestBody": canonical, "baseUrl": "https://sls.example/", "apiKey": "submit-key"}
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	descriptor := value.(map[string]any)
	assert.Equal(t, "https://sls.example/v1/video/generations", descriptor["url"])
	assert.Equal(t, "POST", descriptor["method"])
	assert.Equal(t, "Bearer submit-key", descriptor["headers"].(map[string]any)["Authorization"])
	body, err := common.Marshal(descriptor["body"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"first","weight":0},{"type":"text","text":"second"}],"duration":-1,"resolution":"720p","seed":0,"generate_audio":false,"future":{"enabled":false}}`, string(body))
	value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	usage, err := common.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tokens":324000,"resolution":"720p"}`, string(usage))
	unchanged, err := common.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, raw, string(unchanged))
	ctx["taskId"], ctx["apiKey"] = "task/one", "poll-key"
	value, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
	require.NoError(t, err)
	query := value.(map[string]any)
	assert.Equal(t, "https://sls.example/v1/video/generations/task%2Fone", query["url"])
	assert.Equal(t, "GET", query["method"])
	assert.Equal(t, "Bearer poll-key", query["headers"].(map[string]any)["Authorization"])
}

func TestSeedanceSLSPluginAdmissionAndPreparedBounds(t *testing.T) {
	plugin := seedanceSLSPlugin(t)
	_, err := plugin.Engine.CallMember(t.Context(), "native", "createTask", map[string]any{"body": map[string]any{"kind": "multipart"}})
	require.ErrorContains(t, err, "requires application/json")
	for _, fields := range []map[string]any{
		{"duration": 3601}, {"duration": -2}, {"duration": 1.5}, {"duration": true},
		{"duration": "18446744073709551615"}, {"frames": 86401}, {"frames": true},
	} {
		request := map[string]any{"model": "doubao-seedance-2-0-260128", "content": []any{map[string]any{"type": "text", "text": "fox"}}}
		for key, value := range fields {
			request[key] = value
		}
		_, err := plugin.Engine.CallMember(t.Context(), "native", "createTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
		require.Error(t, err)
		_, err = plugin.Engine.Call(t.Context(), "validatePreparedRequest", nil, request)
		require.Error(t, err, "post-policy payloads need the same bounds")
	}
}

func TestSeedanceSLSPluginNestedPollingAndIdentityContract(t *testing.T) {
	plugin := seedanceSLSPlugin(t)
	const raw = `{"code":"success","data":{"task_id":"gateway","status":"SUCCESS","data":{"code":"success","data":{"task_id":"provider","upstream_task_id":"secret","status":"SUCCESS","result_url":"https://cdn.example/provider.mp4?task_id=provider","last_frame_url":"https://cdn.example/frame.png","total_tokens":87300}}}}`
	var body map[string]any
	require.NoError(t, common.UnmarshalJsonStr(raw, &body))
	value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", nil, body)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"taskId":"gateway","status":"SUCCESS","progress":"100%","reason":"","url":"https://cdn.example/provider.mp4?task_id=provider","totalTokens":87300}`, string(encoded))
	value, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", nil, nil, body)
	require.NoError(t, err)
	encoded, err = common.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tokens":87300}`, string(encoded))
	value, err = plugin.Engine.Call(t.Context(), "sanitizeTaskData", body, "public")
	require.NoError(t, err)
	encoded, err = common.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"code":"success","data":{"task_id":"public","status":"SUCCESS","data":{"code":"success","data":{"task_id":"public","upstream_task_id":"public","status":"SUCCESS","result_url":"https://cdn.example/provider.mp4?task_id=provider","last_frame_url":"https://cdn.example/frame.png","total_tokens":87300}}}}`, string(encoded))
	unchanged, err := common.Marshal(body)
	require.NoError(t, err)
	assert.JSONEq(t, raw, string(unchanged), "identity sanitization must not mutate provider data used by billing or content fetch")
	value, err = plugin.Engine.Call(t.Context(), "parseTaskResult", nil, map[string]any{"status": "FAILED", "fail_reason": "generation failed", "result_url": "provider safety detail"})
	require.NoError(t, err)
	assert.Equal(t, "generation failed\nprovider safety detail", value.(map[string]any)["reason"])
	assert.Empty(t, value.(map[string]any)["url"])
}

func TestSeedanceSLSPluginPreservesResolutionAndVideoRatios(t *testing.T) {
	plugin := seedanceSLSPlugin(t)
	for _, tc := range []struct {
		model      string
		resolution string
		video      bool
		want       float64
	}{
		{"doubao-seedance-2-0-260128", "720p", false, 1},
		{"doubao-seedance-2-0-260128", "720p", true, 28.0 / 46},
		{"doubao-seedance-2-0-260128", "1080p", false, 51.0 / 46},
		{"doubao-seedance-2-0-260128", "1080p", true, 31.0 / 46},
		{"doubao-seedance-2-0-260128", "4K", false, 26.0 / 46},
		{"doubao-seedance-2-0-260128", "4K", true, 16.0 / 46},
		{"doubao-seedance-2-0-fast-260128", "720p", true, 22.0 / 37},
		{"doubao-seedance-2-0-mini-260615", "720p", true, 14.0 / 23},
		{"doubao-seedance-2-5-260628", "1080p", false, 11.7 / 10.7},
		{"doubao-seedance-2-5-260628", "1080p", true, 7 / 10.7},
		{"doubao-seedance-2-5-260628", "720p", true, 42.0 / 70},
		{"doubao-seedance-2-5-260628", "720p", false, 1},
		{"public-alias", "1080p", true, 1},
	} {
		content := []any{map[string]any{"type": "text", "text": "a fox"}}
		if tc.video {
			content = append(content, map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://cdn.example/ref.mp4"}})
		}
		value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
			"model": tc.model, "usagePurpose": "billing_ratios",
			"preparedRequestBody": map[string]any{"content": content, "resolution": tc.resolution},
		})
		require.NoError(t, err)
		if tc.want == 1 {
			assert.Nil(t, value, tc.model)
			continue
		}
		ratios, ok := value.(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, tc.want, ratios["video_input"], 1e-12)
	}
}

func TestSeedanceSLSPluginCompatiblePromptImagesAndMetadata(t *testing.T) {
	plugin := seedanceSLSPlugin(t)
	metadata := map[string]any{"model": "must-not-replace-mapping", "duration": 4, "seed": 0, "generate_audio": false, "future": map[string]any{"enabled": false}}
	encodedMetadata, err := common.Marshal(metadata)
	require.NoError(t, err)
	for _, metadataValue := range []any{metadata, string(encodedMetadata)} {
		intent, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
			"model": "public-alias", "body": map[string]any{"kind": "json", "value": map[string]any{
				"prompt": "Animate the frame", "images": []any{"https://cdn.example/frame.png"}, "seconds": 5, "metadata": metadataValue,
			}},
		})
		require.NoError(t, err)
		value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
			"model": "public-alias", "upstreamModel": "mapped-model", "baseUrl": "https://sls.example",
			"requestBody": intent.(map[string]any)["requestBody"],
		})
		require.NoError(t, err)
		body, err := common.Marshal(value.(map[string]any)["body"])
		require.NoError(t, err)
		assert.JSONEq(t, `{"model":"mapped-model","content":[{"type":"image_url","image_url":{"url":"https://cdn.example/frame.png"}},{"type":"text","text":"Animate the frame"}],"duration":5,"seed":0,"generate_audio":false,"future":{"enabled":false}}`, string(body))
	}
}

func TestSeedanceSLSPluginNativeProjectionPreservesExplicitZeroAndFalse(t *testing.T) {
	plugin := seedanceSLSPlugin(t)
	var task map[string]any
	require.NoError(t, common.UnmarshalJsonStr(`{"task_id":"public","model":"public-alias","status":"SUCCESS","created_at":100,"updated_at":200,"data":{"code":"success","data":{"seed":0,"duration":0,"frames":0,"generate_audio":false,"data":{"seed":10,"duration":5,"frames":100,"generate_audio":true,"total_tokens":87300,"result_url":"https://cdn.example/video.mp4","last_frame_url":"https://cdn.example/frame.png"}}}}`, &task))
	value, err := plugin.Engine.CallMember(t.Context(), "native", "taskStatus", map[string]any{}, task)
	require.NoError(t, err)
	data, err := common.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"public","model":"public-alias","status":"succeeded","created_at":100,"updated_at":200,"seed":0,"duration":0,"frames":0,"generate_audio":false,"usage":{"completion_tokens":87300,"total_tokens":87300},"content":{"video_url":"https://cdn.example/video.mp4","last_frame_url":"https://cdn.example/frame.png"}}`, string(data))
}
