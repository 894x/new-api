package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTasksToDtoRestrictsVideoRawDataToAdmins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	videoData := json.RawMessage(`{"status":"succeeded","internal":{"seed":42}}`)
	audioData := json.RawMessage(`[{"audio_url":"https://example.com/audio.mp3"}]`)

	tasks := []*model.Task{
		{
			TaskID: "video-task",
			Action: constant.TaskActionTextGenerate,
			Data:   videoData,
		},
		{
			TaskID: "audio-task",
			Action: "MUSIC",
			Data:   audioData,
		},
	}

	adminContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	adminContext.Set("role", common.RoleAdminUser)
	adminTasks := tasksToDto(adminContext, tasks, false)
	require.Len(t, adminTasks, 2)
	assert.JSONEq(t, string(videoData), string(adminTasks[0].Data))
	assert.JSONEq(t, string(audioData), string(adminTasks[1].Data))

	userContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	userContext.Set("role", common.RoleCommonUser)
	userTasks := tasksToDto(userContext, tasks, false)
	require.Len(t, userTasks, 2)
	assert.Empty(t, userTasks[0].Data)
	assert.JSONEq(t, string(audioData), string(userTasks[1].Data))
}
