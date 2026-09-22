package plugins_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestH3PluginActualUsageAndFrozenImageSurcharge(t *testing.T) {
	plugin := h3Plugin(t)
	for _, tc := range []struct {
		name, body           string
		input, output, units float64
		images               *int
	}{
		{"actual image count", `{"task":{"status":"succeeded","resolution":"2K","usage":{"input_seconds":2,"output_seconds":4,"input_image_count":7}}}`, 2, 4, 10.4, common.GetPointer(7)},
		{"omitted image count keeps reservation", `{"task":{"status":"succeeded","resolution":"2K","usage":{"input_seconds":2,"output_seconds":4}}}`, 2, 4, 10.4, nil},
		{"explicit zero removes image surcharge", `{"task":{"status":"succeeded","usage":{"input_seconds":2,"output_seconds":4,"input_image_count":0}}}`, 2, 4, 9.6, common.GetPointer(0)},
		{"total-only fallback", `{"task":{"status":"succeeded","resolution":"768P","usage":{"total_seconds":18,"input_image_count":0}}}`, 3, 15, 18, common.GetPointer(0)},
		{"provider quantities bounded", `{"task":{"status":"succeeded","resolution":"2K","usage":{"input_seconds":1e30,"output_seconds":1e30,"input_image_count":100000}}}`, 3600, 15, 5785.6, common.GetPointer(9)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := taskplugin.New(plugin)
			result, err := adaptor.ParseTaskResult([]byte(tc.body))
			require.NoError(t, err)
			require.NotNil(t, result.Usage)
			assert.Equal(t, tc.input, result.Usage.Input)
			assert.Equal(t, tc.output, result.Usage.Output)
			assert.Equal(t, tc.input+tc.output, result.Usage.Total)
			assert.Equal(t, tc.images, result.Usage.InputImages)
			for _, resolutionKey := range []string{"resolution", "resolution_multiplier"} {
				task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, Data: []byte(tc.body), Properties: model.Properties{OriginModelName: "MiniMax-H3"},
					PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{ModelRatio: 0.14, OtherRatios: map[string]float64{"seconds": 4, resolutionKey: 1.6, "image_surcharge": 1.125, "customer_ratio": 0.75}}}}
				assert.Zero(t, adaptor.AdjustBillingOnComplete(task, result))
				assert.Equal(t, common.QuotaRound(tc.units*common.QuotaPerUnit/2), result.TotalTokens)
				assert.Equal(t, map[string]float64{"seconds": 1, resolutionKey: 1, "image_surcharge": 1, "customer_ratio": 0.75}, task.PrivateData.BillingContext.OtherRatios)
			}
		})
	}
	t.Run("missing usage retains the estimate", func(t *testing.T) {
		adaptor := taskplugin.New(plugin)
		body := []byte(`{"task":{"status":"succeeded","resolution":"2K"}}`)
		result, err := adaptor.ParseTaskResult(body)
		require.NoError(t, err)
		task := &model.Task{Data: body, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{ModelRatio: 0.14, OtherRatios: map[string]float64{"seconds": 7}}}}
		adaptor.AdjustBillingOnComplete(task, result)
		assert.Nil(t, result.Usage)
		assert.Zero(t, result.TotalTokens)
		assert.Equal(t, 7.0, task.PrivateData.BillingContext.OtherRatios["seconds"])
	})
}

func TestHailuoPluginPreservesProviderBusinessErrors(t *testing.T) {
	plugin := h3Plugin(t)
	for _, tc := range []struct {
		name, body, code, message       string
		transportStatus, expectedStatus int
	}{
		{"H3 business rejection over HTTP 200", `{"error":{"type":"unprocessable_entity_error","message":"rejected","http_code":"422"}}`, "unprocessable_entity_error", "rejected", 200, 422},
		{"legacy business rejection", `{"base_resp":{"status_code":1026,"status_msg":"sensitive content"}}`, "1026", "hailuo api error: sensitive content", 200, 400},
		{"invalid body status", `{"error":{"type":"server_error","message":"failed","http_code":200}}`, "server_error", "failed", 200, 502},
		{"HTTP status authoritative on failure", `{"error":{"type":"server_error","message":"failed","http_code":500}}`, "server_error", "failed", 429, 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v2/video_generation", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			adaptor := taskplugin.New(plugin)
			adaptor.Init(info)
			response := &http.Response{StatusCode: tc.transportStatus, Body: io.NopCloser(strings.NewReader(tc.body))}
			if tc.transportStatus != http.StatusOK {
				err := adaptor.ParseSubmitError(ctx, response, info)
				require.NotNil(t, err)
				assert.Equal(t, tc.expectedStatus, err.StatusCode)
				assert.Equal(t, tc.code, err.Code)
				assert.Equal(t, tc.message, err.Message)
				return
			}
			result, err := adaptor.ParseResponse(ctx, response, info)
			assert.Nil(t, result)
			require.NotNil(t, err)
			assert.Equal(t, tc.expectedStatus, err.StatusCode)
			assert.Equal(t, tc.code, err.Code)
			assert.Equal(t, tc.message, err.Message)
		})
	}
}

func TestHailuoArtifactContentProxy(t *testing.T) {
	source, err := builtinplugins.Source("hailuo")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "hailuo"})
	require.NoError(t, err)
	adaptor := taskplugin.New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:         "test-ak",
			ChannelBaseUrl: "https://api.minimax.example",
		},
	})
	data, err := common.Marshal(map[string]any{"file_id": "file/with space"})
	require.NoError(t, err)
	task := &model.Task{TaskID: "task-public", Status: model.TaskStatusSuccess, Data: data}

	artifacts, err := adaptor.ListArtifacts(task)
	require.NoError(t, err)
	assert.Equal(t, []channel.TaskArtifact{{Key: "video", Type: "video", MimeType: "video/mp4"}}, artifacts)

	descriptor, err := adaptor.BuildContentRequest(task, "video", channel.TaskArtifactClientRequest{Method: http.MethodHead})
	require.NoError(t, err)
	require.NotNil(t, descriptor)
	assert.Equal(t, "https://api.minimax.example/v1/files/download?file_id=file%2Fwith%20space", descriptor.URL)
	assert.Equal(t, http.MethodHead, descriptor.Method)
	assert.Equal(t, map[string]string{"Accept": "video/*", "Authorization": "Bearer test-ak"}, descriptor.Headers)
	assert.False(t, descriptor.Credentialless)

	// Existing provider data stays authoritative even when an older stored URL
	// is available; use the private fallback only when neither data shape exists.
	task.PrivateData.ResultURL = "https://legacy.example/video.mp4"
	descriptor, err = adaptor.BuildContentRequest(task, "video", channel.TaskArtifactClientRequest{Method: http.MethodGet})
	require.NoError(t, err)
	assert.Equal(t, "https://api.minimax.example/v1/files/download?file_id=file%2Fwith%20space", descriptor.URL)
	task.Data = []byte(`{"task":{"content":{"url":"https://cdn.example/current.mp4"}}}`)
	descriptor, err = adaptor.BuildContentRequest(task, "video", channel.TaskArtifactClientRequest{Method: http.MethodGet})
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/current.mp4", descriptor.URL)
	assert.True(t, descriptor.Credentialless)
	assert.Empty(t, descriptor.Headers)
	task.Data = nil
	descriptor, err = adaptor.BuildContentRequest(task, "video", channel.TaskArtifactClientRequest{Method: http.MethodHead})
	require.NoError(t, err)
	assert.Equal(t, "https://legacy.example/video.mp4", descriptor.URL)
	assert.Equal(t, http.MethodHead, descriptor.Method)
	assert.True(t, descriptor.Credentialless)
	assert.Empty(t, descriptor.Headers)
	for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusFailure} {
		task.Status = status
		artifacts, err := adaptor.ListArtifacts(task)
		require.NoError(t, err)
		assert.Empty(t, artifacts)
	}
}
