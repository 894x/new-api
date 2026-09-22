package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			result, err := adaptor.ParseTaskResult(body)
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
