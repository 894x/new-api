package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJimengNativePublicTaskViews(t *testing.T) {
	source, err := builtinplugins.Source("jimeng")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "jimeng"})
	require.NoError(t, err)
	for _, tc := range []struct{ name, task, expected string }{
		{"redacted failure", `{"task_id":"task_public_failed","status":"FAILURE","fail_reason":"public failure reason"}`, `{"code":500,"message":"public failure reason","data":{"task_id":"task_public_failed"}}`},
		{"successful response", `{"task_id":"public","status":"SUCCESS","data":{"code":10000,"request_id":"request-1","data":{"task_id":"provider","status":"done","video_url":"https://cdn.example/video.mp4"}}}`, `{"code":10000,"request_id":"request-1","data":{"task_id":"public","status":"done","video_url":"https://cdn.example/video.mp4"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var task map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.task, &task))
			value, err := plugin.Engine.CallMember(t.Context(), "native", "renderTask", map[string]any{}, []any{task})
			require.NoError(t, err)
			encoded, err := common.Marshal(value)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(encoded))
		})
	}
}

func TestJimengResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "jimeng",
		model:     "jimeng_vgfm_t2v_l20",
		requestBody: map[string]any{
			"model":   "jimeng_vgfm_t2v_l20",
			"input":   "a paper boat on a river",
			"seconds": 10,
			"metadata": map[string]any{
				"aspect_ratio": "16:9",
			},
		},
		wantAction: "text_to_video",
		wantRequest: map[string]any{
			"model":    "jimeng_vgfm_t2v_l20",
			"prompt":   "a paper boat on a river",
			"duration": float64(10),
			"metadata": map[string]any{"aspect_ratio": "16:9"},
		},
		wantUsageKeys:  []string{"product", "seconds"},
		wantVendorName: "jimeng",
	})
}

func loadJimengPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("jimeng")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "jimeng"})
	require.NoError(t, err)
	return plugin
}

func TestJimengSubmitStateDrivesQueryReqKey(t *testing.T) {
	plugin := loadJimengPlugin(t)
	submitValue, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{
		"upstreamModel": "jimeng_vgfm_i2v_l20",
		"requestBody":   map[string]any{"images": []any{"https://cdn.example/frame.png"}},
	}, map[string]any{"body": map[string]any{"code": 10000, "data": map[string]any{"task_id": "t1"}}})
	require.NoError(t, err)
	encoded, err := common.Marshal(submitValue)
	require.NoError(t, err)
	var submit map[string]any
	require.NoError(t, common.Unmarshal(encoded, &submit))
	state, ok := submit["state"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "jimeng_vgfm_i2v_l20", state["req_key"])

	queryValue, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
		"taskId":  "t1",
		"action":  "text_to_video",
		"baseUrl": "https://jimeng.example",
		"apiKey":  "sk-test",
		"state":   map[string]any{"req_key": "custom_req_key"},
	})
	require.NoError(t, err)
	queryEncoded, err := common.Marshal(queryValue)
	require.NoError(t, err)
	var query map[string]any
	require.NoError(t, common.Unmarshal(queryEncoded, &query))
	var body map[string]any
	require.NoError(t, common.UnmarshalJsonStr(common.Interface2String(query["body"]), &body))
	assert.Equal(t, "custom_req_key", body["req_key"])
	assert.Equal(t, "t1", body["task_id"])
}

func TestJimengParseTaskResultUnknownStatus(t *testing.T) {
	plugin := loadJimengPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{
		"code": 10000,
		"data": map[string]any{"status": "weird"},
	})
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	assert.Equal(t, "UNKNOWN", result["status"])
	assert.Contains(t, common.Interface2String(result["reason"]), "weird")
}
