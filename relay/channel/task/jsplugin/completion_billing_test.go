package jsplugin

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletionBillingUnitsRemainHostControlled(t *testing.T) {
	for _, tc := range []struct {
		name, units string
		tokens      int
		clamped     bool
	}{
		{"valid", "10.4", 2600000, false},
		{"saturated", "1e30", math.MaxInt32, true},
		{"negative", "-1", 0, false},
		{"zero", "0", 0, false},
		{"tiny", "1e-30", 0, false},
		{"not finite", "Infinity", 0, false},
		{"not a number", "NaN", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := mockPlugin + fmt.Sprintf(`
export function extractBillingOnComplete(task, result, context) {
  if (task.private_data || task.user_id || context.modelRatio || context.apiKey) throw new Error("private context leaked");
  if (task.data.task_id !== "task_public") throw new Error("private task ID leaked");
  return {modelUnits: %s, consumedRatios: ["seconds", "unknown"]};
}`, tc.units)
			plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
			require.NoError(t, err)
			task := &model.Task{TaskID: "task_public", UserId: 123, Data: []byte(`{"task_id":"upstream_private"}`), PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream_private", BillingContext: &model.TaskBillingContext{ModelRatio: 0.5, OtherRatios: map[string]float64{"seconds": 7, "discount": 0.8}}}}
			result := &relaycommon.TaskInfo{}
			adaptor := New(plugin)
			adaptor.AdjustBillingOnComplete(task, result)
			// The mock's independent usage hook returns 23 tokens; invalid model
			// units must not replace those facts or consume reserved multipliers.
			if tc.tokens == 0 {
				assert.Equal(t, 23, result.TotalTokens)
				assert.Equal(t, 7.0, task.PrivateData.BillingContext.OtherRatios["seconds"])
			} else {
				assert.Equal(t, tc.tokens, result.TotalTokens)
				assert.Equal(t, 1.0, task.PrivateData.BillingContext.OtherRatios["seconds"])
			}
			assert.Equal(t, tc.clamped, result.QuotaClamp != nil)
			assert.Equal(t, 0.8, task.PrivateData.BillingContext.OtherRatios["discount"])
			assert.NotContains(t, task.PrivateData.BillingContext.OtherRatios, "unknown")
		})
	}
}

func TestCompletionBillingDoesNotOverrideFixedOrExpressionPricing(t *testing.T) {
	plugin, err := pluginruntime.NewRegistry().Register(mockPlugin+`
export function extractBillingOnComplete() { return {modelUnits: 10, consumedRatios: ["seconds"]}; }
`, pluginruntime.Options{})
	require.NoError(t, err)
	for _, bc := range []*model.TaskBillingContext{
		{ModelRatio: 1, PerCallBilling: true},
		{ModelRatio: 1, TieredSnapshot: &billingexpr.BillingSnapshot{}},
		{ModelRatio: 0},
	} {
		bc.OtherRatios = map[string]float64{"seconds": 7}
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: bc}}
		result := &relaycommon.TaskInfo{}
		New(plugin).AdjustBillingOnComplete(task, result)
		assert.Equal(t, 23, result.TotalTokens)
		assert.Equal(t, 7.0, bc.OtherRatios["seconds"])
	}
}

func TestNormalizedVideoUsageRejectsInvalidProviderQuantities(t *testing.T) {
	source := strings.Replace(mockPlugin, `return {taskId: body.id, status: "SUCCESS", progress: "100%", url: body.url};`, `return {status: "SUCCESS", usage: body};`, 1)
	plugin, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{})
	require.NoError(t, err)
	adaptor := New(plugin)
	for _, body := range []string{
		`{"kind":"video_duration","unit":"second","input":-1,"output":2,"total":1}`,
		`{"kind":"video_duration","unit":"second","input":3601,"output":2,"total":3603}`,
		`{"kind":"video_duration","unit":"second","input":1,"output":2,"total":99}`,
		`{"kind":"video_duration","unit":"second","input":1,"output":2,"total":3,"input_images":129}`,
		`{"kind":"video_duration","unit":"token","input":1,"output":2,"total":3}`,
	} {
		_, err := adaptor.ParseTaskResult(&model.Task{}, &http.Response{StatusCode: http.StatusOK}, []byte(body))
		assert.Error(t, err, body)
	}
	result, err := adaptor.ParseTaskResult(&model.Task{}, &http.Response{StatusCode: http.StatusOK}, []byte(`{"kind":"video_duration","unit":"second","input":1,"output":2,"total":3,"input_images":0}`))
	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	require.NotNil(t, result.Usage.InputImages)
	assert.Zero(t, *result.Usage.InputImages)
}
