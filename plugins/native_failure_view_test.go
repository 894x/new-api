package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativePresentersSupportRedactedFailureViews(t *testing.T) {
	const taskID, reason = "task_public_failed", "public failure reason"
	for _, tc := range []struct {
		plugin, render, expected string
	}{
		{"doubao", "taskStatus", `{"id":"task_public_failed","model":"","status":"failed","created_at":100,"updated_at":200,"error":{"message":"public failure reason"}}`},
		{"alibaba", "taskStatus", `{"output":{"task_id":"task_public_failed","task_status":"FAILED","code":"task_failed","message":"public failure reason"}}`},
		{"kling", "taskStatus", `{"code":0,"data":{"task_id":"task_public_failed","task_status":"failed","task_status_msg":"public failure reason"}}`},
		{"sunoapi", "renderTask", `{"code":"success","message":"","data":{"created_at":100,"updated_at":200,"task_id":"task_public_failed","platform":"sunoapi","status":"FAILURE","fail_reason":"public failure reason","submit_time":100,"finish_time":0,"progress":"100%","data":null}}`},
	} {
		t.Run(tc.plugin, func(t *testing.T) {
			source, err := builtinplugins.Source(tc.plugin)
			require.NoError(t, err)
			plugin, err := pluginruntime.NewRegistry().RegisterFactory(source, pluginruntime.Options{Key: tc.plugin})
			require.NoError(t, err)
			view := map[string]any{"task_id": taskID, "status": "FAILURE", "progress": "100%", "fail_reason": reason, "created_at": 100, "updated_at": 200}
			value, err := plugin.Engine.CallMember(t.Context(), "native", tc.render, map[string]any{}, view)
			require.NoError(t, err)
			encoded, err := common.Marshal(value)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(encoded))
		})
	}
}
