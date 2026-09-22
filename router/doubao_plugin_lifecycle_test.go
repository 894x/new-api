package router

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoubaoPluginLifecyclePreservesHistoricalPricesAndDownloads(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.ChannelAssetConfig{}, &model.Task{}, &model.Log{}, &model.UserSubscription{}))
	previousPrices, previousRatios := ratio_setting.ModelPrice2JSONString(), ratio_setting.ModelRatio2JSONString()
	previousQuota, previousLog, previousBatch, previousCache := common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled
	previousFactory := service.GetTaskAdaptorFunc
	previousSecret, previousAddress := common.CryptoSecret, system_setting.TaskPublicAddress
	previousFetch := *system_setting.GetFetchSetting()
	common.CryptoSecret, system_setting.TaskPublicAddress = "doubao-artifact-secret", "https://gateway.example"
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = 1000, true, false, true
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
	service.InitHttpClient()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatios))
		common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = previousQuota, previousLog, previousBatch, previousCache
		service.GetTaskAdaptorFunc = previousFactory
		common.CryptoSecret, system_setting.TaskPublicAddress = previousSecret, previousAddress
		*system_setting.GetFetchSetting() = previousFetch
		service.InitHttpClient()
	})
	t.Setenv("ASSET_STORAGE_ENABLED", "false")
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "doubao-video-bytes")
	}))
	t.Cleanup(media.Close)
	requests := make(chan map[string]any, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer doubao-test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks" {
			var body map[string]any
			if err := common.DecodeJson(r.Body, &body); !assert.NoError(t, err) {
				http.Error(w, "invalid body", 400)
				return
			}
			requests <- body
			_, _ = io.WriteString(w, `{"id":"private-provider-task"}`)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v3/contents/generations/tasks/") {
			if strings.HasSuffix(r.URL.Path, "failed") {
				_, _ = io.WriteString(w, `{"id":"private-provider-task","status":"failed","error":{"code":"InvalidInput","message":"generation failed"}}`)
			} else {
				_, _ = fmt.Fprintf(w, `{"id":"private-provider-task","status":"succeeded","seed":0,"generate_audio":false,"content":{"video_url":%q},"usage":{"completion_tokens":1070,"total_tokens":1070,"tool_usage":{"web_search":1}}}`, media.URL+"/video.mp4")
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)
	const initialQuota = 10000000
	user := model.User{Username: "doubao-lifecycle", Status: common.UserStatusEnabled, Group: "default", Quota: initialQuota}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "doubaolifecycle", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: initialQuota}
	require.NoError(t, model.DB.Create(&token).Error)
	ch := model.Channel{Type: constant.ChannelTypeDoubaoVideo, Name: "doubao-lifecycle", Key: "doubao-test-key", Status: common.ChannelStatusEnabled,
		BaseURL: common.GetPointer(upstream.URL), Models: "doubao-seedance-2-5-260628", Group: "default"}
	require.NoError(t, model.DB.Create(&ch).Error)
	require.NoError(t, ch.AddAbilities(nil))
	model.InitChannelCache()
	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetTaskRouter(engine)
	engine.NoRoute(SetPluginRouter(engine))
	for _, tc := range []struct{ path, historical string }{
		{"/v1/videos", ""}, {"/v1/video/generations", ""},
		{"/api/v3/contents/generations/tasks", "54"}, {"/doubao/api/v3/contents/generations/tasks", "45"},
	} {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failed=%t", tc.path, failed), func(t *testing.T) {
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"doubao-seedance-2-5-260628":1}`))
				request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"a fox"}],"resolution":"1080p","duration":5,"seed":0,"generate_audio":false}`))
				request.Header.Set("Authorization", "Bearer doubaolifecycle")
				request.Header.Set("Content-Type", gin.MIMEJSON)
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				var receipt struct {
					ID string `json:"id"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &receipt))
				require.NotEmpty(t, receipt.ID)
				wire := <-requests
				assert.Equal(t, "doubao-seedance-2-5-260628", wire["model"])
				assert.Equal(t, false, wire["generate_audio"])
				assert.Equal(t, float64(0), wire["seed"])
				var task model.Task
				require.NoError(t, model.DB.Where("task_id = ?", receipt.ID).First(&task).Error)
				require.NotNil(t, task.PrivateData.BillingContext)
				assert.InDelta(t, 11.7/10.7, task.PrivateData.BillingContext.OtherRatios["video_input_ratio"], 1e-12)
				require.Positive(t, task.Quota)
				reserved := task.Quota
				if tc.historical != "" {
					task.Platform = constant.TaskPlatform(tc.historical)
					task.PrivateData.Execution = nil
					// Persist the exact old key as well as the old frozen rate.
					task.PrivateData.BillingContext.OtherRatios = map[string]float64{"video_input": 77.0 / 70.0}
				}
				if failed {
					task.PrivateData.UpstreamTaskID = "private-failed"
				}
				require.NoError(t, model.DB.Save(&task).Error)
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"doubao-seedance-2-5-260628":9}`))
				require.NoError(t, service.UpdateVideoTasks(context.Background(), task.Platform, map[int][]string{ch.Id: {task.PrivateData.UpstreamTaskID}}, map[string]*model.Task{task.PrivateData.UpstreamTaskID: &task}))
				require.NoError(t, model.DB.First(&task, task.ID).Error)
				if failed {
					assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
					assert.Zero(t, task.Quota)
					assert.Equal(t, reserved, task.PrivateData.BillingContext.RefundedQuota)
					assert.Equal(t, model.TaskRefundStateCommitted, task.PrivateData.BillingContext.RefundState)
					return
				}
				wantQuota := 1170
				if tc.historical != "" {
					wantQuota = 1177
				}
				assert.Equal(t, wantQuota, task.Quota)
				assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
				if tc.historical != "" {
					task.Data = nil
					if tc.historical == "45" {
						task.FailReason = task.GetResultURL()
						task.PrivateData.ResultURL = ""
					}
					require.NoError(t, model.DB.Save(&task).Error)
				}
				query := httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
				query.Header.Set("Authorization", "Bearer doubaolifecycle")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, query)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.NotContains(t, response.Body.String(), media.URL)
				var video struct {
					Metadata map[string]any `json:"metadata"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &video))
				link, ok := video.Metadata["url"].(string)
				require.True(t, ok, "historical Doubao video must remain downloadable")
				parsed, err := url.Parse(link)
				require.NoError(t, err)
				assert.Equal(t, "gateway.example", parsed.Host)
				download := httptest.NewRecorder()
				engine.ServeHTTP(download, httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil))
				require.Equal(t, http.StatusOK, download.Code, download.Body.String())
				assert.Equal(t, "doubao-video-bytes", download.Body.String())
			})
		}
	}
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	require.NoError(t, model.DB.First(&ch, ch.Id).Error)
	const consumed = 1170*2 + 1177*2
	assert.Equal(t, initialQuota-consumed, user.Quota)
	assert.Equal(t, consumed, user.UsedQuota)
	assert.Equal(t, initialQuota-consumed, token.RemainQuota)
	assert.Equal(t, consumed, token.UsedQuota)
	assert.EqualValues(t, consumed, ch.UsedQuota)
	assert.Equal(t, 8, user.RequestCount)
}
