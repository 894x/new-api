package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginTerminalBillingEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name                                    string
		immediate, expression, monthly, failure bool
		retryChange                             string
		inherited                               bool
	}{
		{"immediate expression", true, true, false, false, "", false},
		{"immediate expression failure", true, true, false, true, "", false},
		{"immediate monthly expression", true, true, true, false, "", false},
		{"immediate monthly expression failure", true, true, true, true, "", false},
		{"polled monthly expression", false, true, true, false, "", false},
		{"polled monthly expression failure", false, true, true, true, "", false},
		{"immediate ratio", true, false, false, false, "", false},
		{"immediate ratio failure", true, false, false, true, "", false},
		{"immediate monthly ratio", true, false, true, false, "", false},
		{"retry retains expression and quota unit", true, true, false, false, "expression", false},
		{"retry retains expression mode", false, true, false, false, "mode", false},
		{"retry retains deleted expression through monthly polling", false, true, true, false, "deleted", false},
		{"mapped expression overrides existing alias per-call price", true, true, false, false, "", true},
		{"retry freezes inherited expression and quota unit", true, true, false, false, "expression", true},
		{"retry retains inherited expression mode", false, true, false, false, "mode", true},
		{"retry retains deleted inherited expression through monthly polling", false, true, true, false, "deleted", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupRelayRouterTestDB(t)
			require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.ChannelAssetConfig{}, &model.Task{}, &model.Log{}, &model.UserSubscription{}, &model.UserGroupModelMonthlyUsage{}, &model.GroupModelDiscountSettlement{}, &model.GroupModelDiscountAdjustment{}))
			saved := map[string]string{}
			require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
			prices, ratios := ratio_setting.ModelPrice2JSONString(), ratio_setting.ModelRatio2JSONString()
			oldQuota, oldLog, oldBatch, oldCache := common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled
			oldRegistry, oldFactory := pluginruntime.DefaultRegistry, service.GetTaskAdaptorFunc
			oldRetries := common.RetryTimes
			t.Cleanup(func() {
				require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(prices))
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(ratios))
				common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = oldQuota, oldLog, oldBatch, oldCache
				pluginruntime.DefaultRegistry, service.GetTaskAdaptorFunc = oldRegistry, oldFactory
				common.RetryTimes = oldRetries
			})
			common.RetryTimes = 1
			common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = 1000, true, false, true
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"billing-fixture":2}`))
			settings := map[string]string{"billing_setting.billing_mode": `{}`, "group_ratio_setting.group_ratio": `{"default":0.25}`, "group_ratio_setting.model_tiered_ratios": `{}`}
			expressionModel := "billing-fixture"
			if tc.inherited {
				expressionModel = "billing-target"
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"billing-fixture":0.003}`))
			}
			if tc.expression {
				settings["billing_setting.billing_mode"] = fmt.Sprintf(`{%q:"tiered_expr"}`, expressionModel)
				settings["billing_setting.billing_expr"] = fmt.Sprintf(`{%q:%q}`, expressionModel, `tier("base", u("seconds") * (param("quality") == "high" ? 2 : 1))`)
				if tc.inherited {
					settings["billing_setting.billing_mode"] = `{"billing-fixture":"ratio","billing-target":"tiered_expr"}`
				}
			}
			if tc.monthly {
				settings["group_ratio_setting.model_tiered_ratios"] = `{"default":{"billing-fixture":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":1},{"min_monthly_original_quota":10000,"ratio":0.5}]}}}`
			}
			require.NoError(t, config.GlobalConfig.LoadFromDB(settings))
			pluginruntime.DefaultRegistry = pluginruntime.NewRegistry()
			_, err := pluginruntime.DefaultRegistry.RegisterFactory(`
export const meta = {apiVersion:1,key:"billing-fixture",name:"Billing fixture",version:"1.0.0",author:{name:"Test"},models:["billing-fixture","billing-target"],fetchMode:"per_task",usageSchema:{seconds:{type:"number",unit:"second",description:"Video duration"}},routes:[{method:"POST",path:"/billing-fixture/jobs",type:"submit",decode:"decode",render:"created"}]};
export const native = {decode(ctx){return {kind:"submit",model:ctx.body.value.model,action:"text_to_video",requestBody:ctx.body.value};},created(ctx,task){return {id:task.task_id,status:task.status};}};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/submit",method:"POST",body:ctx.requestBody};}
export function extractUsage(ctx){return {seconds:ctx.requestBody.seconds};}
export function parseSubmitResponse(ctx,response){const b=response.body;return {taskId:"provider-job",taskData:b,immediate:b.immediate?{taskId:"provider-job",status:b.status,progress:"100%",reason:"provider failure",totalTokens:b.tokens}:undefined};}
export function buildQueryRequest(ctx){return {url:ctx.baseUrl+"/query",method:"GET"};}
export function parseTaskResult(ctx,b){return {taskId:"provider-job",status:b.status,progress:"100%",reason:"provider failure",totalTokens:b.tokens};}
export function extractUsageOnComplete(task,result,b){return {seconds:b.seconds};}
`, pluginruntime.Options{})
			require.NoError(t, err)
			service.GetTaskAdaptorFunc = func(p constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(p) }
			service.InitHttpClient()
			t.Setenv("ASSET_STORAGE_ENABLED", "false")
			status := "SUCCESS"
			if tc.failure {
				status = "FAILURE"
			}
			var submits, polls atomic.Int32
			var retryBalance atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					if submits.Add(1) == 1 && tc.retryChange != "" {
						changed := map[string]string{}
						switch tc.retryChange {
						case "expression":
							changed["billing_setting.billing_expr"] = fmt.Sprintf(`{%q:%q}`, expressionModel, `u("seconds") * 99`)
						case "mode":
							changed["billing_setting.billing_mode"] = `{}`
						case "deleted":
							changed["billing_setting.billing_expr"] = `{}`
						}
						assert.NoError(t, config.GlobalConfig.LoadFromDB(changed))
						common.QuotaPerUnit = 2000
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte(`{"error":{"message":"retry another channel"}}`))
						return
					}
					if tc.retryChange != "" {
						var admitted model.User
						if assert.NoError(t, model.DB.First(&admitted).Error) {
							retryBalance.Store(int64(admitted.Quota))
						}
					}
				} else {
					polls.Add(1)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"immediate":%t,"status":%q,"seconds":8,"tokens":800}`, tc.immediate, status)
			}))
			t.Cleanup(upstream.Close)
			user := model.User{Username: "terminal-billing", Group: "default", Status: common.UserStatusEnabled, Quota: 100000}
			require.NoError(t, model.DB.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "terminalbilling", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100000}
			require.NoError(t, model.DB.Create(&token).Error)
			channelSettings := `{"task_plugin_key":"billing-fixture"}`
			ch := model.Channel{Type: constant.ChannelTypeTaskPlugin, Name: "billing-fixture", Key: "provider-key", Models: "billing-fixture", Group: "default", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL, Setting: &channelSettings, AutoBan: common.GetPointer(0), Priority: common.GetPointer(int64(10))}
			if tc.inherited {
				ch.ModelMapping = common.GetPointer(`{"billing-fixture":"billing-target"}`)
			}
			require.NoError(t, model.DB.Create(&ch).Error)
			require.NoError(t, ch.AddAbilities(nil))
			var retryChannel model.Channel
			if tc.retryChange != "" {
				retryChannel = ch
				retryChannel.Id, retryChannel.Name, retryChannel.Priority = 0, "billing-fixture-retry", common.GetPointer(int64(1))
				require.NoError(t, model.DB.Create(&retryChannel).Error)
				require.NoError(t, retryChannel.AddAbilities(nil))
			}
			model.InitChannelCache()
			engine := gin.New()
			engine.NoRoute(SetPluginRouter(engine))
			req := httptest.NewRequest(http.MethodPost, "/billing-fixture/jobs", strings.NewReader(`{"model":"billing-fixture","prompt":"test","seconds":5,"quality":"high"}`))
			req.Header.Set("Content-Type", gin.MIMEJSON)
			req.Header.Set("Authorization", "Bearer terminalbilling")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var task model.Task
			require.NoError(t, model.DB.First(&task).Error)
			if tc.inherited {
				assert.Equal(t, "billing-fixture", task.Properties.OriginModelName)
				assert.Equal(t, "billing-target", task.Properties.UpstreamModelName)
				require.NotNil(t, task.PrivateData.BillingContext)
				require.NotNil(t, task.PrivateData.BillingContext.TieredSnapshot)
				assert.Equal(t, "billing-fixture", task.PrivateData.BillingContext.TieredSnapshot.ModelName)
			}
			if tc.retryChange != "" {
				wantReserve := 2500
				if tc.monthly {
					wantReserve = 10000
				}
				assert.EqualValues(t, 100000-wantReserve, retryBalance.Load(), "the retry must reserve the frozen expression estimate before dispatch")
				assert.Equal(t, retryChannel.Id, task.ChannelId)
				require.NoError(t, model.DB.First(&ch, ch.Id).Error)
				assert.Zero(t, ch.UsedQuota, "failed attempt must not consume channel quota")
				ch = retryChannel
				require.NotNil(t, task.PrivateData.BillingContext)
				snapshot := task.PrivateData.BillingContext.TieredSnapshot
				require.NotNil(t, snapshot)
				assert.Equal(t, `tier("base", u("seconds") * (param("quality") == "high" ? 2 : 1))`, snapshot.ExprString)
				assert.Equal(t, 1000.0, snapshot.QuotaPerUnit)
				if !tc.immediate {
					assert.Equal(t, wantReserve, task.Quota, "initial settlement must use the same expression as completion")
				}
			}
			if !tc.immediate {
				require.NoError(t, service.UpdateVideoTasks(context.Background(), task.Platform, map[int][]string{ch.Id: {task.GetUpstreamTaskID()}}, map[string]*model.Task{task.GetUpstreamTaskID(): &task}))
				require.NoError(t, model.DB.First(&task, task.ID).Error)
			}
			wantOriginal, wantCharge := 8000, 2000
			if tc.expression {
				wantOriginal, wantCharge = 16000, 4000
			}
			if tc.monthly {
				wantCharge = wantOriginal
				if wantOriginal > 10000 {
					wantCharge = 10000 + (wantOriginal-10000)/2
				}
			}
			if tc.failure {
				wantOriginal, wantCharge = 0, 0
			}
			assert.Equal(t, model.TaskStatus(status), task.Status)
			assert.Equal(t, wantCharge, task.Quota)
			require.NoError(t, model.DB.First(&user, user.Id).Error)
			require.NoError(t, model.DB.First(&token, token.Id).Error)
			require.NoError(t, model.DB.First(&ch, ch.Id).Error)
			assert.Equal(t, 100000-wantCharge, user.Quota)
			assert.Equal(t, 100000-wantCharge, token.RemainQuota)
			assert.Equal(t, wantCharge, user.UsedQuota)
			assert.Equal(t, int64(wantCharge), ch.UsedQuota)
			assert.Equal(t, 1, user.RequestCount)
			if tc.monthly {
				var usage model.UserGroupModelMonthlyUsage
				query := model.DB.Find(&usage)
				require.NoError(t, query.Error)
				if tc.immediate && tc.failure {
					assert.Zero(t, query.RowsAffected, "an already failed task must not advance monthly consumption")
				} else {
					require.EqualValues(t, 1, query.RowsAffected)
				}
				assert.EqualValues(t, wantOriginal, usage.OriginalQuota)
				assert.EqualValues(t, wantCharge, usage.ChargedQuota)
			}
			wantSubmits := int32(1)
			if tc.retryChange != "" {
				wantSubmits = 2
			}
			assert.Equal(t, wantSubmits, submits.Load())
			if tc.immediate {
				assert.Zero(t, polls.Load())
			} else {
				assert.Equal(t, int32(1), polls.Load())
			}
		})
	}
}
