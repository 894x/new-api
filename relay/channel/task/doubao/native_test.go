package doubao

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeRequestUsesExistingDoubaoBillingAndPreservesPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "Generate a tracking shot",
		Metadata: map[string]any{
			"model":          "doubao-seedance-2-0-260128",
			"resolution":     "1080p",
			"duration":       float64(5),
			"generate_audio": false,
			"content": []any{
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/input.mp4"}},
				map[string]any{"type": "text", "text": "Generate a tracking shot"},
			},
		},
	})

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mapped-seedance-model",
		},
	}

	ratios := adaptor.EstimateBilling(ctx, info)
	require.Contains(t, ratios, "video_input")
	assert.InDelta(t, 31.0/46.0, ratios["video_input"], 1e-12)

	requestBody, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(encoded, &payload))
	assert.Equal(t, "mapped-seedance-model", payload["model"])
	assert.Equal(t, "1080p", payload["resolution"])
	assert.Equal(t, false, payload["generate_audio"])
	content, ok := payload["content"].([]any)
	require.True(t, ok)
	assert.Len(t, content, 2)
}

func TestNativeSubmitResponseUsesPublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"upstream-task-id"}`)),
	}

	adaptor := &TaskAdaptor{}
	upstreamID, taskData, taskErr := adaptor.DoResponse(ctx, response, &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	})

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-task-id", upstreamID)
	assert.JSONEq(t, `{"id":"upstream-task-id"}`, string(taskData))
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestConvertToNativeVideoPreservesUsageAndRewritesIdentity(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		CreatedAt: 100,
		UpdatedAt: 200,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
		PrivateData: model.TaskPrivateData{ResultURL: "https://example.com/output.mp4"},
		Data: json.RawMessage(`{
			"id":"upstream-task-id",
			"model":"upstream-model",
			"status":"succeeded",
			"content":{"video_url":"https://example.com/output.mp4"},
			"service_tier":"default",
			"usage":{"completion_tokens":35000,"total_tokens":35000}
		}`),
	}

	encoded, err := (&TaskAdaptor{}).ConvertToNativeVideo(task)
	require.NoError(t, err)
	var response struct {
		ID          string `json:"id"`
		Model       string `json:"model"`
		Status      string `json:"status"`
		ServiceTier string `json:"service_tier"`
		Usage       struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, common.Unmarshal(encoded, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "doubao-seedance-2-0-260128", response.Model)
	assert.Equal(t, "succeeded", response.Status)
	assert.Equal(t, "default", response.ServiceTier)
	assert.Equal(t, 35000, response.Usage.CompletionTokens)
	assert.Equal(t, 35000, response.Usage.TotalTokens)
}
