package plugins_test

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alibabaPlugin(t *testing.T) *pluginruntime.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("alibaba")
	require.NoError(t, err)
	plugin, err := pluginruntime.NewRegistry().RegisterFactory(source, pluginruntime.Options{Key: "alibaba"})
	require.NoError(t, err)
	return plugin
}

func TestWan3NativePluginPreservesPayloadAndMappedModels(t *testing.T) {
	plugin := alibabaPlugin(t)
	const body = `{"model":"customer-wan","input":{"media":[{"type":"reference_video","url":"https://cdn.example/input.mp4"}],"future_input":false},"parameters":{"duration":-1,"resolution":"1080P","ratio":"adaptive","seed":0,"audio":false,"watermark":false,"prompt_extend":false},"future_option":{"enabled":false,"count":0}}`
	var request map[string]any
	require.NoError(t, common.UnmarshalJsonStr(body, &request))
	intent, err := plugin.Engine.CallMember(context.Background(), "native", "createVideoTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
	require.NoError(t, err)
	canonical := intent.(map[string]any)["requestBody"]
	ctx := map[string]any{"model": "customer-wan", "upstreamModel": "wan3.0-video-prime", "requestBody": canonical, "baseUrl": "https://provider.example", "apiKey": "submit-key", "modelMappingResolved": true}
	value, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	descriptor := value.(map[string]any)
	expected := map[string]any{}
	require.NoError(t, common.UnmarshalJsonStr(body, &expected))
	expected["model"] = "wan3.0-video-prime"
	expectedJSON, err := common.Marshal(expected)
	require.NoError(t, err)
	bodyJSON, err := common.Marshal(descriptor["body"])
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedJSON), string(bodyJSON))
	assert.Equal(t, map[string]any{"Authorization": "Bearer submit-key", "Content-Type": "application/json", "X-DashScope-Async": "enable"}, descriptor["headers"])
	assert.Equal(t, "https://provider.example/api/v1/services/aigc/video-generation/video-synthesis", descriptor["url"])
	assert.Equal(t, "image_to_video", descriptor["action"])
	ctx["usagePurpose"] = "billing_ratios"
	ratios, err := plugin.Engine.Call(context.Background(), "extractUsage", ctx)
	require.NoError(t, err)
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	assert.JSONEq(t, `{"seconds":30,"resolution-1080P":4}`, string(encoded))
	ctx["upstreamModel"] = "wan2.7-t2v"
	_, err = plugin.Engine.Call(context.Background(), "buildSubmitRequest", ctx)
	require.ErrorContains(t, err, "parameters.duration must be between 1 and 3600")
}

func TestAlibabaNativeCreationPreservesProviderFieldsAndPublicIdentity(t *testing.T) {
	plugin := alibabaPlugin(t)
	response := map[string]any{"request_id": "request-1", "future_field": false, "output": map[string]any{"task_id": "provider-private", "task_status": "RUNNING", "queued_count": 0}}
	parsed, err := plugin.Engine.Call(context.Background(), "parseSubmitResponse", map[string]any{"publicTaskId": "task_public"}, map[string]any{"body": response})
	require.NoError(t, err)
	result := parsed.(map[string]any)
	assert.Equal(t, "provider-private", result["taskId"])
	view, err := plugin.Engine.CallMember(context.Background(), "native", "taskCreated", map[string]any{}, map[string]any{"task_id": "task_public", "data": result["taskData"]})
	require.NoError(t, err)
	body, err := common.Marshal(view)
	require.NoError(t, err)
	assert.JSONEq(t, `{"request_id":"request-1","future_field":false,"output":{"task_id":"task_public","task_status":"RUNNING","queued_count":0}}`, string(body))
	assert.Equal(t, "provider-private", response["output"].(map[string]any)["task_id"])
	failed, err := plugin.Engine.Call(context.Background(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": map[string]any{"code": "InvalidInput", "message": "provider rejected"}})
	require.NoError(t, err)
	failure, err := common.Marshal(failed)
	require.NoError(t, err)
	assert.JSONEq(t, `{"error":{"code":"ali_api_error","message":"InvalidInput: provider rejected","httpStatus":502}}`, string(failure))
}

func TestWan3NativeNullDurationUsesDefaultReservationWithoutRewritingPayload(t *testing.T) {
	plugin := alibabaPlugin(t)
	request := map[string]any{"model": "wan3.0-video", "input": map[string]any{"prompt": "animate"}, "parameters": map[string]any{"duration": nil, "resolution": nil}}
	intent, err := plugin.Engine.CallMember(context.Background(), "native", "createVideoTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
	require.NoError(t, err)
	ctx := map[string]any{"model": "wan3.0-video", "upstreamModel": "wan3.0-video", "modelMappingResolved": true, "requestBody": intent.(map[string]any)["requestBody"], "usagePurpose": "billing_ratios"}
	value, err := plugin.Engine.Call(context.Background(), "extractUsage", ctx)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"seconds":5,"resolution-1080P":4}`, string(encoded))
	descriptor, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	payload, err := common.Marshal(descriptor.(map[string]any)["body"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"wan3.0-video","input":{"prompt":"animate"},"parameters":{"duration":null,"resolution":null}}`, string(payload))
}

func TestWan3NativePluginRejectsInvalidQuantitiesBeforeSubmit(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct {
		name  string
		patch map[string]any
	}{
		{"negative", map[string]any{"duration": -2}}, {"zero", map[string]any{"duration": 0}},
		{"too short", map[string]any{"duration": 1}}, {"too long", map[string]any{"duration": 31}},
		{"huge", map[string]any{"duration": 1e30}}, {"fraction", map[string]any{"duration": 2.5}},
		{"boolean", map[string]any{"duration": true}}, {"array", map[string]any{"duration": []any{5}}},
		{"native string duration", map[string]any{"duration": "5"}}, {"resolution", map[string]any{"resolution": "4K"}},
		{"seed", map[string]any{"seed": 2147483648.0}}, {"ratio", map[string]any{"ratio": "2:1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{"model": "wan3.0-video", "input": map[string]any{"prompt": "animate"}, "parameters": tc.patch}
			_, err := plugin.Engine.CallMember(context.Background(), "native", "createVideoTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
			require.Error(t, err)
		})
	}
}

func TestWanPluginSharedMediaDefaultsAndOptionalZeros(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct{ name, model, input, expected string }{
		{"wan3 frames", "wan3.0-video", `{"prompt":"animate","images":["first.png","last.png"],"metadata":{"input":{"audio_url":"voice.mp3"},"parameters":{"seed":0,"audio":false,"watermark":false,"prompt_extend":false}}}`, `{"model":"wan3.0-video","input":{"prompt":"animate","media":[{"type":"first_frame","url":"first.png"},{"type":"last_frame","url":"last.png"},{"type":"reference_audio","url":"voice.mp3"}]},"parameters":{"duration":5,"resolution":"1080P","ratio":"adaptive","seed":0,"audio":false,"watermark":false,"prompt_extend":false}}`},
		{"wan27 frames", "wan2.7-i2v", `{"prompt":"animate","images":["first.png","last.png"],"metadata":{"input":{"audio_url":"voice.mp3"},"parameters":{"seed":0,"watermark":false,"prompt_extend":false}}}`, `{"model":"wan2.7-i2v","input":{"prompt":"animate","media":[{"type":"first_frame","url":"first.png"},{"type":"last_frame","url":"last.png"},{"type":"driving_audio","url":"voice.mp3"}]},"parameters":{"duration":5,"resolution":"720P","seed":0,"watermark":false,"prompt_extend":false}}`},
		{"wan25 legacy image", "wan2.5-i2v-preview", `{"prompt":"animate","image":"preferred.png","images":["other.png"]}`, `{"model":"wan2.5-i2v-preview","input":{"prompt":"animate","img_url":"preferred.png"},"parameters":{"duration":5,"resolution":"1080P","watermark":false,"prompt_extend":true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request any
			require.NoError(t, common.UnmarshalJsonStr(tc.input, &request))
			value, err := plugin.Engine.Call(context.Background(), "buildSubmitRequest", map[string]any{"model": tc.model, "upstreamModel": tc.model, "baseUrl": "https://provider.example", "requestBody": request})
			require.NoError(t, err)
			body, err := common.Marshal(value.(map[string]any)["body"])
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(body))
		})
	}
}
