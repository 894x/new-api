package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moyuWan3Plugin(t *testing.T) *pluginruntime.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("moyu-wan3")
	require.NoError(t, err)
	plugin, err := pluginruntime.NewRegistry().RegisterFactory(source, pluginruntime.Options{Key: "moyu-wan3"})
	require.NoError(t, err)
	return plugin
}

func TestMoyuWan3TranslatesOfficialWanRequest(t *testing.T) {
	plugin := moyuWan3Plugin(t)
	request := map[string]any{
		"model": "wan3.0-video",
		"input": map[string]any{
			"prompt": "a cat running across a moonlit roof",
			"media":  []any{map[string]any{"type": "first_frame", "url": "https://cdn.example/start.png"}},
		},
		"parameters": map[string]any{
			"resolution": "720P",
			"ratio":      "16:9",
			"duration":   5,
			"seed":       0,
			"watermark":  false,
		},
	}
	value, err := plugin.Engine.CallMember(t.Context(), "native", "createVideoTask", map[string]any{
		"model": "wan3.0-video",
		"body":  map[string]any{"kind": "json", "value": request},
	})
	require.NoError(t, err)
	intent := value.(map[string]any)

	descriptorValue, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"model":                "wan3.0-video",
		"upstreamModel":        "wan3.0-video",
		"requestBody":          intent["requestBody"],
		"baseUrl":              "https://www.moyu.info/",
		"apiKey":               "moyu-key",
		"modelMappingResolved": true,
	})
	require.NoError(t, err)
	descriptor := descriptorValue.(map[string]any)
	assert.Equal(t, "https://www.moyu.info/v1/video/generations", descriptor["url"])
	assert.Equal(t, "Bearer moyu-key", descriptor["headers"].(map[string]any)["Authorization"])
	body, err := common.Marshal(descriptor["body"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"wan3.0-video","prompt":"a cat running across a moonlit roof","media":[{"type":"first_frame","url":"https://cdn.example/start.png"}],"resolution":"720P","ratio":"16:9","duration":5,"seed":0,"watermark":false}`, string(body))

	usageValue, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"requestBody": intent["requestBody"]})
	require.NoError(t, err)
	usage, err := common.Marshal(usageValue)
	require.NoError(t, err)
	assert.JSONEq(t, `{"seconds":5,"resolution":"720P"}`, string(usage))
}

func TestMoyuWan3ParsesMoyuTaskAndRendersWanStatus(t *testing.T) {
	plugin := moyuWan3Plugin(t)
	var submit map[string]any
	require.NoError(t, common.UnmarshalJsonStr(`{"id":"provider-task","object":"video","model":"wan3.0-video","status":"queued"}`, &submit))
	value, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"body": submit})
	require.NoError(t, err)
	assert.Equal(t, "provider-task", value.(map[string]any)["taskId"])

	queryDescriptor, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
		"baseUrl": "https://www.moyu.info",
		"taskId":  "provider/task",
		"apiKey":  "moyu-key",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://www.moyu.info/v1/video/generations/provider%2Ftask", queryDescriptor.(map[string]any)["url"])

	var query map[string]any
	require.NoError(t, common.UnmarshalJsonStr(`{"code":"success","message":"","data":{"task_id":"provider-task","status":"SUCCESS","progress":"100%","result_url":"https://cdn.moyu.info/video/result.mp4"}}`, &query))
	value, err = plugin.Engine.Call(t.Context(), "parseTaskResult", nil, query)
	require.NoError(t, err)
	result := value.(map[string]any)
	assert.Equal(t, "SUCCESS", result["status"])
	assert.Equal(t, "https://cdn.moyu.info/video/result.mp4", result["url"])
	publicValue, err := plugin.Engine.Call(t.Context(), "sanitizeTaskData", query, "public-task")
	require.NoError(t, err)
	assert.Equal(t, "public-task", publicValue.(map[string]any)["data"].(map[string]any)["task_id"])

	statusValue, err := plugin.Engine.CallMember(t.Context(), "native", "taskStatus", map[string]any{}, map[string]any{
		"task_id": "public-task",
		"status":  "SUCCESS",
		"data":    query,
	})
	require.NoError(t, err)
	status, err := common.Marshal(statusValue)
	require.NoError(t, err)
	assert.JSONEq(t, `{"request_id":"","output":{"task_id":"public-task","task_status":"SUCCEEDED","video_url":"https://cdn.moyu.info/video/result.mp4"}}`, string(status))
}

func TestMoyuWan3UsesOfficialTaskIDFromDataEnvelope(t *testing.T) {
	plugin := moyuWan3Plugin(t)
	var submit map[string]any
	require.NoError(t, common.UnmarshalJsonStr(`{"code":"success","message":"","data":{"task_id":"dc153518-99ab-4185-980a-e9baf7cf0ba2","action":"textGenerate","status":"IN_PROGRESS","fail_reason":"","submit_time":1787558977,"start_time":1787558980,"finish_time":0,"progress":"30%"}}`, &submit))

	value, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"body": submit})
	require.NoError(t, err)
	assert.Equal(t, "dc153518-99ab-4185-980a-e9baf7cf0ba2", value.(map[string]any)["taskId"])

	publicValue, err := plugin.Engine.Call(t.Context(), "sanitizeTaskData", submit, "task_public")
	require.NoError(t, err)
	assert.Equal(t, "task_public", publicValue.(map[string]any)["data"].(map[string]any)["task_id"])
}

func TestMoyuWan3RejectsUnsupportedMediaAndBounds(t *testing.T) {
	plugin := moyuWan3Plugin(t)
	for _, request := range []map[string]any{
		{"model": "wan3.0-video", "input": map[string]any{"prompt": "x", "media": []any{map[string]any{"type": "first_frame", "url": "a"}, map[string]any{"type": "reference_image", "url": "b"}}}},
		{"model": "wan3.0-video", "input": map[string]any{"prompt": "x"}, "parameters": map[string]any{"duration": 31}},
		{"model": "wan3.0-video", "input": map[string]any{"prompt": "x"}, "parameters": map[string]any{"resolution": "4K"}},
	} {
		_, err := plugin.Engine.CallMember(t.Context(), "native", "createVideoTask", map[string]any{"body": map[string]any{"kind": "json", "value": request}})
		require.Error(t, err)
	}
}
