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
