package service

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func TestBuildTaskPluginViewRewritesOnlyStructuredTaskIDFields(t *testing.T) {
	const (
		privateTaskID = "upstream-task-123"
		publicTaskID  = "task_public_123"
		resultURL     = "https://cdn.example.com/results/upstream-task-123/video.mp4"
	)

	taskData, err := common.Marshal(map[string]any{
		"task_id": privateTaskID,
		"id":      privateTaskID,
		"taskId":  privateTaskID,
		"url":     resultURL,
		"message": "completed upstream-task-123",
		"nested": []any{
			map[string]any{
				"task_id": privateTaskID,
				"url":     resultURL,
			},
			privateTaskID,
		},
		privateTaskID: "opaque map key",
	})
	require.NoError(t, err)
	task := &model.Task{
		TaskID: publicTaskID,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: privateTaskID,
		},
		Data: taskData,
	}

	view, err := BuildTaskPluginView(task)
	require.NoError(t, err)

	data, ok := view.Data.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, publicTaskID, data["task_id"])
	assert.Equal(t, publicTaskID, data["id"])
	assert.Equal(t, publicTaskID, data["taskId"])
	assert.Equal(t, resultURL, data["url"])
	assert.Equal(t, "completed upstream-task-123", data["message"])
	assert.Equal(t, "opaque map key", data[privateTaskID])

	nested, ok := data["nested"].([]any)
	require.True(t, ok)
	nestedData, ok := nested[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, publicTaskID, nestedData["task_id"])
	assert.Equal(t, resultURL, nestedData["url"])
	assert.Equal(t, privateTaskID, nested[1])

}

func TestTaskPluginClientViewPreservesDiagnosticsOnlyForAllowedViewers(t *testing.T) {
	previous := operation_setting.GetErrorSetting().HideErrorDetails
	t.Cleanup(func() { operation_setting.UpdateHideErrorDetails(previous) })
	task := &model.Task{TaskID: "task_public", UserId: 123, ChannelId: 456, Quota: 789,
		Status: model.TaskStatusFailure, FailReason: "provider secret request id: upstream-private",
		Properties:  model.Properties{OriginModelName: "public-model", UpstreamModelName: "private-model"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "private-task", ResultURL: "https://private.example/result"},
	}
	task.SetData(map[string]any{"error": map[string]any{"message": "provider secret"}, "arbitrary_debug": "provider secret"})
	original, err := common.Marshal(task)
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		role     int
		hide     bool
		redacted bool
	}{
		{"customer hidden", common.RoleCommonUser, true, true},
		{"admin visible", common.RoleAdminUser, true, false},
		{"customer visible", common.RoleCommonUser, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.UpdateHideErrorDetails(tc.hide)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("role", tc.role)
			c.Set(common.RequestIdKey, "local-query-id")
			view, err := BuildTaskPluginViewForClient(c, task)
			require.NoError(t, err)
			assert.Equal(t, "public-model", view.Model)
			assert.Empty(t, view.ResultURL, "a failed task cannot expose an old success URL")
			if tc.redacted {
				assert.Nil(t, view.Data)
				assert.Equal(t, PublicErrorMessage("local-query-id"), view.FailReason)
			} else {
				assert.NotNil(t, view.Data)
				assert.Equal(t, "provider secret request id: local-query-id", view.FailReason)
			}
			encoded, err := common.Marshal(view)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, common.Unmarshal(encoded, &fields))
			for _, field := range []string{"user_id", "channel_id", "quota", "properties", "private_data"} {
				assert.NotContains(t, fields, field)
			}
			assert.NotContains(t, string(encoded), "private-model")
			current, err := common.Marshal(task)
			require.NoError(t, err)
			assert.Equal(t, original, current, "viewer filtering must not mutate stored diagnostics")
		})
	}
	legacy := &model.Task{Status: model.TaskStatusSuccess, FailReason: "https://cdn.example/legacy.mp4"}
	view, err := BuildTaskPluginView(legacy)
	require.NoError(t, err)
	assert.Equal(t, legacy.FailReason, view.ResultURL)
}
