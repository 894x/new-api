package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doubaoPlugin(t *testing.T) *pluginruntime.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("doubao")
	require.NoError(t, err)
	plugin, err := pluginruntime.NewRegistry().RegisterFactory(source, pluginruntime.Options{Key: "doubao"})
	require.NoError(t, err)
	return plugin
}

func TestDoubaoPluginTransportContract(t *testing.T) {
	plugin := doubaoPlugin(t)
	for _, tc := range []struct{ mode, submit, query, wantSubmit, wantQuery string }{
		{"", "", "", "/api/v3/contents/generations/tasks", "/api/v3/contents/generations/tasks/task%2F123"},
		{"v3", "", "", "/api/v3/contents/generations/tasks", "/api/v3/contents/generations/tasks/task%2F123"},
		{"video_generations", "", "", "/v1/video/generations", "/v1/video/generations/task%2F123"},
		{"custom", "/custom/tasks", "/custom/results/{id}", "/custom/tasks", "/custom/results/task%2F123"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			ctx := map[string]any{
				"baseUrl": "https://provider.example/", "apiKey": "submit-key", "taskId": "task/123",
				"requestBody":     map[string]any{"model": "alias", "prompt": "fox", "seconds": 5, "metadata": map[string]any{"generate_audio": false}},
				"upstreamModel":   "mapped-model",
				"channelSettings": map[string]any{"doubao_video_api_mode": tc.mode, "doubao_video_submit_path": tc.submit, "doubao_video_fetch_path": tc.query},
			}
			value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			request := value.(map[string]any)
			assert.Equal(t, "https://provider.example"+tc.wantSubmit, request["url"])
			assert.Equal(t, "POST", request["method"])
			assert.Equal(t, "Bearer submit-key", request["headers"].(map[string]any)["Authorization"])
			body, err := common.Marshal(request["body"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"model":"mapped-model","duration":5,"generate_audio":false,"content":[{"type":"text","text":"fox"}]}`, string(body))
			ctx["apiKey"] = "poll-key"
			value, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
			require.NoError(t, err)
			query := value.(map[string]any)
			assert.Equal(t, "https://provider.example"+tc.wantQuery, query["url"])
			assert.Equal(t, "GET", query["method"])
			assert.Equal(t, "Bearer poll-key", query["headers"].(map[string]any)["Authorization"])
		})
	}
	for _, tc := range []struct{ mode, submit, query string }{
		{"custom", "", "/tasks/{id}"},
		{"custom", "//other.example/tasks", "/tasks/{id}"},
		{"custom", "https://other.example/tasks", "/tasks/{id}"},
		{"custom", "/tasks", "//other.example/tasks/{id}"},
		{"custom", "/tasks", "/tasks/result"},
		{"unsupported", "", ""},
	} {
		t.Run(tc.mode+tc.submit+tc.query, func(t *testing.T) {
			ctx := map[string]any{"baseUrl": "https://provider.example", "requestBody": map[string]any{"prompt": "fox"},
				"channelSettings": map[string]any{"doubao_video_api_mode": tc.mode, "doubao_video_submit_path": tc.submit, "doubao_video_fetch_path": tc.query}}
			for _, hook := range []string{"buildSubmitRequest", "buildQueryRequest"} {
				_, err := plugin.Engine.Call(t.Context(), hook, ctx)
				require.Error(t, err, hook)
			}
		})
	}
}

func TestDoubaoPluginNativeDraftContract(t *testing.T) {
	plugin := doubaoPlugin(t)
	const raw = `{"model":"doubao-seedance-2-0-260128","duration":5,"seed":0,"generate_audio":false,"future_field":{"enabled":false},"content":[{"type":"text","text":"first","weight":0},{"type":"video_url","video_url":{"url":"https://cdn.example/input.mp4"}},{"type":"text","text":"second"},{"type":"draft_task","draft_task":{"id":"public-draft","extra":false}}]}`
	var request map[string]any
	require.NoError(t, common.UnmarshalJsonStr(raw, &request))
	value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": request}})
	require.NoError(t, err)
	intent := value.(map[string]any)
	ids, err := common.Marshal(intent["originTaskIds"])
	require.NoError(t, err)
	assert.JSONEq(t, `["public-draft"]`, string(ids))
	ctx := map[string]any{"baseUrl": "https://provider.example", "upstreamModel": "mapped-model", "requestBody": intent["requestBody"]}
	_, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.ErrorContains(t, err, "origin task is unavailable")
	ctx["originTasks"] = []any{map[string]any{"taskId": "public-draft", "upstreamTaskId": "provider-draft"}}
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	body, err := common.Marshal(value.(map[string]any)["body"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"mapped-model","duration":5,"seed":0,"generate_audio":false,"future_field":{"enabled":false},"content":[{"type":"text","text":"first","weight":0},{"type":"video_url","video_url":{"url":"https://cdn.example/input.mp4"}},{"type":"text","text":"second"},{"type":"draft_task","draft_task":{"id":"provider-draft","extra":false}}]}`, string(body))
	unchanged, err := common.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, raw, string(unchanged), "the canonical request must remain usable by another channel retry")
}

func TestDoubaoPluginPreservesPricingAndApproved25Rates(t *testing.T) {
	plugin := doubaoPlugin(t)
	for _, tc := range []struct {
		model, resolution string
		video             bool
		want              float64
	}{
		{"doubao-seedance-2-5-260628", "1080p", false, 11.7 / 10.7}, {"doubao-seedance-2-5-260628", "1080p", true, 7.0 / 10.7},
		{"doubao-seedance-2-5-260628", "720p", false, 1}, {"doubao-seedance-2-5-260628", "720p", true, 42.0 / 70.0},
		{"doubao-seedance-2-0-260128", "720p", false, 1}, {"doubao-seedance-2-0-260128", "720p", true, 28.0 / 46.0},
		{"doubao-seedance-2-0-260128", "1080p", false, 51.0 / 46.0}, {"doubao-seedance-2-0-260128", "1080p", true, 31.0 / 46.0},
		{"doubao-seedance-2-0-260128", "4k", false, 26.0 / 46.0}, {"doubao-seedance-2-0-260128", "4k", true, 16.0 / 46.0},
		{"doubao-seedance-2-0-fast-260128", "720p", true, 22.0 / 37.0}, {"doubao-seedance-2-0-mini-260615", "720p", true, 14.0 / 23.0},
		{"public-model-alias", "1080p", true, 1},
	} {
		content := []any{map[string]any{"type": "text", "text": "fox"}}
		if tc.video {
			content = append(content, map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/input.mp4"}})
		}
		value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
			"model": tc.model, "usagePurpose": "billing_ratios",
			"requestBody": map[string]any{"metadata": map[string]any{"resolution": tc.resolution, "content": content}},
		})
		require.NoError(t, err)
		if tc.want == 1 {
			assert.Nil(t, value)
		} else {
			assert.InDelta(t, tc.want, value.(map[string]any)["video_input_ratio"], 1e-12)
		}
	}
}

func TestDoubaoPluginRejectsDurationBypasses(t *testing.T) {
	for _, input := range []string{
		`{"duration":3601}`,
		`{"seconds":5,"metadata":{"duration":3601}}`,
		`{"metadata":"{\"duration\":3601}"}`,
		`{"duration":1.5}`,
		`{"duration":true}`,
		`{"metadata":{"frames":86401}}`,
		`{"metadata":{"frames":true}}`,
	} {
		t.Run(input, func(t *testing.T) {
			var request map[string]any
			require.NoError(t, common.UnmarshalJsonStr(input, &request))
			request["model"] = "doubao-seedance-2-0-260128"
			request["content"] = []any{map[string]any{"type": "text", "text": "fox"}}
			plugin := doubaoPlugin(t)
			_, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": request}})
			require.Error(t, err)
			for _, hook := range []string{"buildSubmitRequest", "extractUsage"} {
				_, err = plugin.Engine.Call(t.Context(), hook, map[string]any{"baseUrl": "https://provider.example", "requestBody": request})
				require.Error(t, err, hook)
			}
		})
	}
}
