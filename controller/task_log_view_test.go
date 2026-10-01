package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskLogDTOSeparatesUserAdminAndRootDetails(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_public",
		Platform: "document-parser",
		PrivateData: model.TaskPrivateData{
			Key:            "channel-secret-canary",
			UpstreamTaskID: "upstream-private",
			NodeName:       "node-a",
			Execution: &model.TaskExecutionSnapshot{
				RequestID:   "request-public",
				RequestPath: "/v1/documents",
				TaskPlugin: &model.TaskPluginSnapshot{
					Key:     "document-parser",
					Name:    "Document Parser",
					Version: "1.2.3",
					Author: &model.TaskPluginAuthorSnapshot{
						Name: "Community Author",
						URL:  "https://plugins.example/author",
					},
					APIVersion: 1,
					Generation: 42,
				},
			},
		},
	}

	userView := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
	assert.Nil(t, userView.AdminInfo)
	assert.Nil(t, userView.RootInfo)

	adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]
	require.NotNil(t, adminView.AdminInfo)
	require.NotNil(t, adminView.AdminInfo.TaskPlugin)
	assert.Equal(t, "document-parser", adminView.AdminInfo.TaskPlugin.Key)
	assert.Equal(t, "Document Parser", adminView.AdminInfo.TaskPlugin.Name)
	assert.Equal(t, "1.2.3", adminView.AdminInfo.TaskPlugin.Version)
	require.NotNil(t, adminView.AdminInfo.TaskPlugin.Author)
	assert.Equal(t, "Community Author", adminView.AdminInfo.TaskPlugin.Author.Name)
	assert.Equal(t, "https://plugins.example/author", adminView.AdminInfo.TaskPlugin.Author.URL)
	assert.Equal(t, "request-public", adminView.AdminInfo.RequestID)
	assert.Equal(t, "/v1/documents", adminView.AdminInfo.RequestPath)
	assert.Nil(t, adminView.RootInfo)

	rootView := tasksToDto([]*model.Task{task}, false, common.RoleRootUser)[0]
	require.NotNil(t, rootView.AdminInfo)
	require.NotNil(t, rootView.RootInfo)
	require.NotNil(t, rootView.RootInfo.TaskPlugin)
	assert.Equal(t, 1, rootView.RootInfo.TaskPlugin.APIVersion)
	assert.Equal(t, uint64(42), rootView.RootInfo.TaskPlugin.Generation)
	assert.Equal(t, "upstream-private", rootView.RootInfo.UpstreamTaskID)
	assert.Equal(t, "node-a", rootView.RootInfo.NodeName)

	adminJSON, err := common.Marshal(adminView)
	require.NoError(t, err)
	assert.NotContains(t, string(adminJSON), "channel-secret-canary")
	assert.NotContains(t, string(adminJSON), "upstream-private")

	rootJSON, err := common.Marshal(rootView)
	require.NoError(t, err)
	assert.NotContains(t, string(rootJSON), "channel-secret-canary")
	assert.Contains(t, string(rootJSON), "upstream-private")
}

func TestTaskLogDTODoesNotInventHistoricalPluginProvenance(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_without_snapshot",
		Platform: "document-parser",
	}

	adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]

	assert.Nil(t, adminView.AdminInfo)
	assert.Nil(t, adminView.RootInfo)
}

func TestTaskLogDTOReplacesLegacyVideoURLWithAvailabilityFlag(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_legacy_video",
		Platform:   "jimeng",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusSuccess,
		FailReason: "https://private-upstream.invalid/video.mp4?signature=secret",
	}

	view := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
	assert.True(t, view.LegacyVideoAvailable)
	assert.Empty(t, view.ResultURL)
	assert.Empty(t, view.FailReason)
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-upstream.invalid")
	assert.NotContains(t, string(encoded), "result_url")
	assert.Contains(t, string(encoded), "legacy_video_available")
}

func TestTaskLogDTOKeepsFailureReasonAndDoesNotMarkPluginTaskLegacy(t *testing.T) {
	failed := &model.Task{
		TaskID:     "task_failed",
		Platform:   "jimeng",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusFailure,
		FailReason: "provider rejected the request",
	}
	failedView := tasksToDto([]*model.Task{failed}, false, common.RoleCommonUser)[0]
	assert.Equal(t, service.PublicErrorMessage(""), failedView.FailReason)
	assert.NotContains(t, failedView.FailReason, "provider rejected")
	adminFailedView := tasksToDto([]*model.Task{failed}, false, common.RoleAdminUser)[0]
	assert.Equal(t, "provider rejected the request", adminFailedView.FailReason)
	assert.False(t, failedView.LegacyVideoAvailable)

	pluginTask := &model.Task{
		TaskID:     "task_plugin_video",
		Platform:   "community-video",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusSuccess,
		FailReason: "https://stale-upstream.invalid/plugin-video.mp4",
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://private-upstream.invalid/plugin-video.mp4",
			Execution: &model.TaskExecutionSnapshot{
				TaskPlugin: &model.TaskPluginSnapshot{Key: "community-video"},
			},
		},
	}
	pluginView := tasksToDto([]*model.Task{pluginTask}, false, common.RoleCommonUser)[0]
	assert.False(t, pluginView.LegacyVideoAvailable)
	assert.Empty(t, pluginView.ResultURL)
	assert.Empty(t, pluginView.FailReason)
}

func TestTaskRequestParametersReadCanonicalPluginPayload(t *testing.T) {
	parameters := taskRequestParametersFromRequest(map[string]any{
		"model": "doubao-seedance-2-0-fast-260128",
		"payload": map[string]any{
			"resolution": "480P", "duration": 5, "ratio": "adaptive",
			"api_key": "private-key", "content": []any{"private-prompt"},
		},
	})
	require.NotNil(t, parameters)
	assert.Equal(t, &model.TaskRequestParameters{Resolution: "480P", Duration: 5, Ratio: "adaptive"}, parameters)
	encoded, err := common.Marshal(model.Properties{RequestParameters: parameters})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-key")
	assert.NotContains(t, string(encoded), "private-prompt")
}

func TestTaskLogDTORecoversHistoricalDisplayParametersWithoutExposingRawData(t *testing.T) {
	response, err := common.Marshal(map[string]any{
		"code": 0,
		"data": map[string]any{
			"resolution": "720p", "duration": 10, "ratio": "16:9",
			"video_url": "https://private.invalid/video?signature=secret",
			"prompt":    "private-prompt",
		},
	})
	require.NoError(t, err)
	for _, tc := range []struct {
		name      string
		persisted *model.TaskRequestParameters
		data      []byte
		want      *model.TaskRequestParameters
	}{
		{"missing snapshot", nil, response, &model.TaskRequestParameters{Resolution: "720p", Duration: 10, Ratio: "16:9"}},
		{"partial snapshot", &model.TaskRequestParameters{Ratio: "adaptive"}, response, &model.TaskRequestParameters{Resolution: "720p", Duration: 10, Ratio: "adaptive"}},
		{"original request wins", &model.TaskRequestParameters{Resolution: "480P", Duration: 5, Ratio: "adaptive"}, response, &model.TaskRequestParameters{Resolution: "480P", Duration: 5, Ratio: "adaptive"}},
		{"missing provider parameters", nil, []byte(`{"code":0,"data":null}`), nil},
		{"malformed historical data", nil, []byte(`{"data":`), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{
				TaskID: "task_sls_parameters", Platform: "seedance-sls", Status: model.TaskStatusSuccess,
				Properties: model.Properties{RequestParameters: tc.persisted}, Data: tc.data,
			}
			before, err := common.Marshal(task.Properties)
			require.NoError(t, err)
			for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser} {
				view := tasksToDto([]*model.Task{task}, false, role)[0]
				properties, ok := view.Properties.(model.Properties)
				require.True(t, ok)
				assert.Equal(t, tc.want, properties.RequestParameters)
				if role == common.RoleCommonUser {
					assert.Empty(t, view.Data)
					encoded, err := common.Marshal(view)
					require.NoError(t, err)
					assert.NotContains(t, string(encoded), "private.invalid")
					assert.NotContains(t, string(encoded), "private-prompt")
				}
			}
			after, err := common.Marshal(task.Properties)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}
