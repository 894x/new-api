package jsplugin

import (
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVideoCompatibilityUsesHostArtifactAccess(t *testing.T) {
	previousSecret, previousAddress := common.CryptoSecret, system_setting.TaskPublicAddress
	common.CryptoSecret, system_setting.TaskPublicAddress = "video-compatibility-test-secret", "https://gateway.example/prefix"
	t.Cleanup(func() { common.CryptoSecret, system_setting.TaskPublicAddress = previousSecret, previousAddress })
	source := strings.Replace(mockPlugin, `export function listArtifacts() { return []; }`, `export function listArtifacts(task) {
  if (task.status !== "SUCCESS") throw new Error("unfinished artifact queried");
  return [{key:"video",type:"video"}];
}`, 1)
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	adaptor := New(plugin)
	for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusFailure, model.TaskStatusSuccess} {
		t.Run(string(status), func(t *testing.T) {
			task := &model.Task{TaskID: "public-task", Status: status, PrivateData: model.TaskPrivateData{
				UpstreamTaskID: "private-task", ResultURL: "https://provider.example/private-task.mp4?secret=private",
			}}
			encoded, err := adaptor.ConvertToOpenAIVideo(task)
			require.NoError(t, err)
			var video dto.OpenAIVideo
			require.NoError(t, common.Unmarshal(encoded, &video))
			assert.Equal(t, task.TaskID, video.ID)
			assert.Equal(t, task.TaskID, video.TaskID)
			assert.NotContains(t, string(encoded), "provider.example")
			assert.NotContains(t, string(encoded), "private-task")
			if status != model.TaskStatusSuccess {
				assert.Empty(t, video.Metadata)
				return
			}
			contentURL, ok := video.Metadata["url"].(string)
			require.True(t, ok)
			parsed, err := url.Parse(contentURL)
			require.NoError(t, err)
			assert.Equal(t, "gateway.example", parsed.Host)
			assert.Equal(t, "/prefix/v1/tasks/public-task/artifacts/video/content", parsed.Path)
			access := parsed.Query().Get(service.TaskArtifactAccessQueryParameter)
			assert.True(t, service.VerifyTaskArtifactAccess(access, task.TaskID, "video"))
			assert.False(t, service.VerifyTaskArtifactAccess(access, "other-task", "video"))
			assert.False(t, service.VerifyTaskArtifactAccess(access, task.TaskID, "last_frame"))
		})
	}
	system_setting.TaskPublicAddress = "https://gateway.example?invalid=true"
	_, err = adaptor.ConvertToOpenAIVideo(&model.Task{TaskID: "public-task", Status: model.TaskStatusSuccess})
	require.ErrorContains(t, err, "build video content URL")
}
