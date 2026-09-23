package plugins_test

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlibabaPluginHistoricalArtifactsAndProviderPriority(t *testing.T) {
	adaptor := taskplugin.New(alibabaPlugin(t))
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example", ApiKey: "private-key"}})
	for _, tc := range []struct{ name, data, wantURL string }{
		{"private fallback", "", "https://legacy.example/video.mp4"},
		{"provider URL wins", `{"output":{"video_url":"https://cdn.example/current.mp4"}}`, "https://cdn.example/current.mp4"},
		{"wrapped provider data", `{"data":{"task_id":"public-task","data":{"output":{"video_url":"https://cdn.example/wrapped.mp4"}}}}`, "https://cdn.example/wrapped.mp4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{TaskID: "public-task", Status: model.TaskStatusSuccess, Data: []byte(tc.data),
				PrivateData: model.TaskPrivateData{ResultURL: "https://legacy.example/video.mp4"}}
			artifacts, err := adaptor.ListArtifacts(task)
			require.NoError(t, err)
			assert.Equal(t, []channel.TaskArtifact{{Key: "video", Type: "video"}}, artifacts)
			descriptor, err := adaptor.BuildContentRequest(task, "video", channel.TaskArtifactClientRequest{Method: http.MethodHead})
			require.NoError(t, err)
			assert.Equal(t, tc.wantURL, descriptor.URL)
			assert.Equal(t, http.MethodHead, descriptor.Method)
			assert.True(t, descriptor.Credentialless)
			assert.Empty(t, descriptor.Headers)
			assert.Empty(t, descriptor.Body)
			_, err = adaptor.BuildContentRequest(task, "unknown", channel.TaskArtifactClientRequest{Method: http.MethodGet})
			require.ErrorContains(t, err, "artifact_not_found")
			for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusFailure} {
				task.Status = status
				artifacts, err = adaptor.ListArtifacts(task)
				require.NoError(t, err)
				assert.Empty(t, artifacts)
			}
		})
	}
}

func TestAlibabaPluginRetainsPublicModelsAndDefaultPrices(t *testing.T) {
	models := taskplugin.New(alibabaPlugin(t)).GetModelList()
	assert.Subset(t, models, []string{"wan3.0-video-prime", "wan3.0-video", "wan2.7-i2v", "wan2.7-t2v", "wan2.5-i2v-preview",
		"wan2.2-i2v-flash", "wan2.2-i2v-plus", "wanx2.1-i2v-plus", "wanx2.1-i2v-turbo"})
	defaults := ratio_setting.GetDefaultModelRatioMap()
	assert.InDelta(t, 2*0.3/ratio_setting.USD2RMB, defaults["wan3.0-video"], 1e-12)
	assert.InDelta(t, 2*0.45/ratio_setting.USD2RMB, defaults["wan3.0-video-prime"], 1e-12)
}

func TestWan3PluginSettlesActualDurationWithFrozenResolution(t *testing.T) {
	plugin := alibabaPlugin(t)
	for _, tc := range []struct {
		name, usage           string
		input, output, billed float64
	}{
		{"input plus output", `{"input_video_duration":7.5,"output_video_duration":5}`, 7.5, 5, 12.5},
		{"string fallback", `{"input_video_duration":"2.5","duration":"5"}`, 2.5, 5, 7.5},
		{"negative input", `{"input_video_duration":-10,"output_video_duration":5}`, 0, 5, 5},
		{"combined cap", `{"input_video_duration":20,"output_video_duration":25}`, 20, 25, 30},
		{"huge upstream duration", `{"input_video_duration":1e30,"output_video_duration":1e30}`, 3600, 3600, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := taskplugin.New(plugin)
			body := []byte(`{"output":{"task_id":"private","task_status":"SUCCEEDED"},"usage":` + tc.usage + `}`)
			result, err := adaptor.ParseTaskResult(&model.Task{}, &http.Response{StatusCode: http.StatusOK}, body)
			require.NoError(t, err)
			require.NotNil(t, result.Usage)
			assert.Equal(t, tc.input, result.Usage.Input)
			assert.Equal(t, tc.output, result.Usage.Output)
			assert.Equal(t, tc.input+tc.output, result.Usage.Total)
			task := &model.Task{TaskID: "public", Status: model.TaskStatusSuccess, Data: body, Properties: model.Properties{OriginModelName: "customer-wan", UpstreamModelName: "wan3.0-video"}, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{ModelRatio: 0.1, OtherRatios: map[string]float64{"seconds": 30, "resolution-1080P": 4}}}}
			adaptor.AdjustBillingOnComplete(task, result)
			assert.Equal(t, common.QuotaRound(tc.billed*common.QuotaPerUnit/2), result.TotalTokens)
			assert.Equal(t, map[string]float64{"seconds": 1, "resolution-1080P": 4}, task.PrivateData.BillingContext.OtherRatios)
		})
	}
}
