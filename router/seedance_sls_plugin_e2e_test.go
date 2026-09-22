package router

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceSLSPluginLifecycleAndHistorical104(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.ChannelAssetConfig{}, &model.Task{}, &model.Log{}, &model.UserSubscription{}))
	previousPrices, previousRatios := ratio_setting.ModelPrice2JSONString(), ratio_setting.ModelRatio2JSONString()
	previousQuota, previousLog, previousBatch, previousCache := common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled
	previousFactory := service.GetTaskAdaptorFunc
	previousHide := operation_setting.ShouldHideErrorDetails()
	previousSecret, previousAddress := common.CryptoSecret, system_setting.TaskPublicAddress
	previousFetch := *system_setting.GetFetchSetting()
	common.CryptoSecret, system_setting.TaskPublicAddress = "sls-artifact-test-secret", "https://gateway.example"
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { savedConfig[key] = value; return nil }))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatios))
		common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = previousQuota, previousLog, previousBatch, previousCache
		service.GetTaskAdaptorFunc = previousFactory
		operation_setting.UpdateHideErrorDetails(previousHide)
		common.CryptoSecret, system_setting.TaskPublicAddress = previousSecret, previousAddress
		*system_setting.GetFetchSetting() = previousFetch
		service.InitHttpClient()
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = 1000, true, false, true
	operation_setting.UpdateHideErrorDetails(false)
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
	service.InitHttpClient()
	t.Setenv("ASSET_STORAGE_ENABLED", "false")
	var submits atomic.Int32
	requests := make(chan map[string]any, 4)
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "sls-video-bytes")
	}))
	t.Cleanup(media.Close)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sls-test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/v1/video/generations" {
			var body map[string]any
			if err := common.DecodeJson(r.Body, &body); !assert.NoError(t, err) {
				http.Error(w, "invalid body", 400)
				return
			}
			requests <- body
			_, _ = fmt.Fprintf(w, `{"task_id":"upstream-%d","data":{"upstream_task_id":"nested-provider-id"}}`, submits.Add(1))
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/video/generations/upstream-") {
			if strings.HasSuffix(r.URL.Path, "2") || strings.HasSuffix(r.URL.Path, "4") || strings.HasSuffix(r.URL.Path, "6") || strings.HasSuffix(r.URL.Path, "8") {
				_, _ = io.WriteString(w, `{"code":"success","data":{"task_id":"gateway-id","status":"FAILURE","fail_reason":"generation failed","result_url":"provider safety detail"}}`)
			} else {
				_, _ = fmt.Fprintf(w, `{"code":"success","data":{"task_id":"gateway-id","status":"SUCCESS","progress":100,"seed":0,"generate_audio":false,"data":{"code":"success","data":{"task_id":"nested-provider-id","upstream_task_id":"inner-id","total_tokens":1070,"result_url":%q,"last_frame_url":"https://cdn.example/frame.png"}}}}`, media.URL+"/video.mp4")
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)
	user := model.User{Username: "sls-plugin-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 10000000}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "slsplugine2e", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 10000000}
	require.NoError(t, model.DB.Create(&token).Error)
	otherUser := model.User{Username: "sls-other-owner", Status: common.UserStatusEnabled, Group: "default", Quota: 10000, AffCode: "sls-other"}
	require.NoError(t, model.DB.Create(&otherUser).Error)
	otherToken := model.Token{UserId: otherUser.Id, Key: "slsothertoken", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 10000}
	require.NoError(t, model.DB.Create(&otherToken).Error)
	overrides := `{"resolution":"1080p","duration":5}`
	mapping := `{"sls-alias":"doubao-seedance-2-5-260628"}`
	ch := model.Channel{Type: constant.ChannelTypeSeedanceSLS, Name: "sls-plugin", Key: "sls-test-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL), Models: "doubao-seedance-2-5-260628,sls-alias", Group: "default", ParamOverride: &overrides, ModelMapping: &mapping}
	require.NoError(t, model.DB.Create(&ch).Error)
	require.NoError(t, ch.AddAbilities(nil))
	model.InitChannelCache()
	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetTaskRouter(engine)
	engine.NoRoute(SetPluginRouter(engine))
	for index, tc := range []struct {
		path, model string
		historical  bool
		expression  bool
	}{
		{"/v1/videos", "doubao-seedance-2-5-260628", false, false},
		{"/v1/video/generations", "doubao-seedance-2-5-260628", false, false},
		{"/api/v3/contents/generations/tasks", "doubao-seedance-2-5-260628", true, false},
		{"/seedance-sls/api/v3/contents/generations/tasks", "sls-alias", true, false},
		{"/v1/videos", "doubao-seedance-2-5-260628", false, true},
		{"/v1/video/generations", "doubao-seedance-2-5-260628", false, true},
		{"/api/v3/contents/generations/tasks", "doubao-seedance-2-5-260628", true, true},
		{"/seedance-sls/api/v3/contents/generations/tasks", "sls-alias", true, true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			expression := `tier("base", u("tokens") / 1000) * (param("resolution") == "480p" ? 2 : 1) * (header("X-Billing-Plan") == "premium" ? 3 : 1)`
			if tc.expression {
				encoded, err := common.Marshal(map[string]string{tc.model: expression})
				require.NoError(t, err)
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": fmt.Sprintf(`{%q:"tiered_expr"}`, tc.model), "billing_setting.billing_expr": string(encoded)}))
			} else {
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{}`}))
			}
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"doubao-seedance-2-5-260628":1,"sls-alias":1}`))
			request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(fmt.Sprintf(`{"model":%q,"content":[{"type":"text","text":"a fox"}],"resolution":"480p","duration":-1,"seed":0,"generate_audio":false}`, tc.model)))
			request.Header.Set("Authorization", "Bearer slsplugine2e")
			request.Header.Set("X-Billing-Plan", "premium")
			request.Header.Set("Content-Type", gin.MIMEJSON)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var receipt struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &receipt))
			require.NotEmpty(t, receipt.ID)
			assert.NotContains(t, recorder.Body.String(), "upstream-")
			wire := <-requests
			assert.Equal(t, "doubao-seedance-2-5-260628", wire["model"])
			assert.Equal(t, "1080p", wire["resolution"])
			assert.Equal(t, float64(5), wire["duration"])
			assert.Equal(t, float64(0), wire["seed"])
			assert.Equal(t, false, wire["generate_audio"])
			var task model.Task
			require.NoError(t, model.DB.Where("task_id = ?", receipt.ID).First(&task).Error)
			assert.Equal(t, constant.TaskPlatform("seedance-sls"), task.Platform)
			assert.NotContains(t, string(task.Data), "nested-provider-id")
			require.NotNil(t, task.PrivateData.BillingContext)
			assert.Positive(t, task.Quota)
			if tc.expression {
				snap := task.PrivateData.BillingContext.TieredSnapshot
				require.NotNil(t, snap)
				require.NotNil(t, snap.RequestInput)
				assert.Equal(t, map[string]any{"resolution": "480p"}, snap.RequestInput.Params)
				assert.Equal(t, map[string]string{"x-billing-plan": "premium"}, snap.RequestInput.Headers)
				assert.Empty(t, snap.RequestInput.Body)
				assert.Equal(t, "1080p", snap.UsageFacts["resolution"])
				assert.Equal(t, float64(task.Quota), snap.UsageFacts["tokens"].(float64)*6)
				require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": fmt.Sprintf(`{%q:"9999"}`, tc.model)}))
			}
			if tc.historical {
				task.Platform = "104"
				task.PrivateData.Execution = nil
				if !tc.expression && tc.model == "doubao-seedance-2-5-260628" {
					// Old tasks keep the old 2.5 price frozen at submit time,
					// even though new tasks use the approved rc27 multiplier.
					task.PrivateData.BillingContext.OtherRatios["video_input"] = 77.0 / 70.0
				}
				require.NoError(t, model.DB.Save(&task).Error)
			}
			// Completion must keep the submit-time model price and ratio.
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"doubao-seedance-2-5-260628":9,"sls-alias":9}`))
			require.NoError(t, service.UpdateVideoTasks(context.Background(), task.Platform, map[int][]string{ch.Id: {task.PrivateData.UpstreamTaskID}}, map[string]*model.Task{task.PrivateData.UpstreamTaskID: &task}))
			require.NoError(t, model.DB.First(&task, task.ID).Error)
			for _, private := range []string{"gateway-id", "nested-provider-id", "inner-id"} {
				assert.NotContains(t, string(task.Data), private)
			}
			if index%2 == 0 {
				assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
				if tc.expression {
					assert.Equal(t, 6420, task.Quota)
					require.Len(t, task.PrivateData.BillingContext.TieredSnapshot.RequestRules, 2)
					for _, rule := range task.PrivateData.BillingContext.TieredSnapshot.RequestRules {
						assert.True(t, rule.Matched)
					}
				} else if tc.historical {
					assert.Equal(t, 1177, task.Quota)
				} else {
					assert.Equal(t, 1170, task.Quota)
				}
			} else {
				assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
				assert.Equal(t, "generation failed\nprovider safety detail", task.FailReason)
				assert.Zero(t, task.Quota)
			}
			for _, prefix := range []string{"/api/v3/contents/generations/tasks/", "/seedance-sls/api/v3/contents/generations/tasks/"} {
				query := httptest.NewRequest(http.MethodGet, prefix+task.TaskID, nil)
				query.Header.Set("Authorization", "Bearer slsplugine2e")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, query)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var projection map[string]any
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &projection))
				assert.Equal(t, task.TaskID, projection["id"])
				assert.Equal(t, tc.model, projection["model"])
				assert.NotContains(t, response.Body.String(), "nested-provider-id")
				if index%2 == 0 {
					assert.Equal(t, "succeeded", projection["status"])
					assert.Equal(t, false, projection["generate_audio"])
					assert.Equal(t, float64(0), projection["seed"])
					assert.Equal(t, map[string]any{"completion_tokens": float64(1070), "total_tokens": float64(1070)}, projection["usage"])
				} else {
					assert.Equal(t, "failed", projection["status"])
					operation_setting.UpdateHideErrorDetails(true)
					hiddenRequest := httptest.NewRequest(http.MethodGet, prefix+task.TaskID, nil)
					hiddenRequest.Header.Set("Authorization", "Bearer slsplugine2e")
					hiddenResponse := httptest.NewRecorder()
					engine.ServeHTTP(hiddenResponse, hiddenRequest)
					require.Equal(t, http.StatusOK, hiddenResponse.Code, hiddenResponse.Body.String())
					assert.NotContains(t, hiddenResponse.Body.String(), "provider safety detail")
					operation_setting.UpdateHideErrorDetails(false)
				}
				unauthorized := httptest.NewRequest(http.MethodGet, prefix+task.TaskID, nil)
				unauthorized.Header.Set("Authorization", "Bearer slsothertoken")
				denied := httptest.NewRecorder()
				engine.ServeHTTP(denied, unauthorized)
				assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, denied.Code, denied.Body.String())
				assert.NotContains(t, denied.Body.String(), "cdn.example")
			}
			if index%2 == 0 {
				if tc.historical {
					// Some old successful rows no longer have provider payloads.
					task.Data = nil
					require.NoError(t, model.DB.Save(&task).Error)
				}
				query := httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
				query.Header.Set("Authorization", "Bearer slsplugine2e")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, query)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var video struct {
					Metadata map[string]any `json:"metadata"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &video))
				assert.NotContains(t, response.Body.String(), media.URL)
				contentURL, ok := video.Metadata["url"].(string)
				require.True(t, ok, "historical SLS video must remain downloadable")
				parsed, err := url.Parse(contentURL)
				require.NoError(t, err)
				assert.Equal(t, "gateway.example", parsed.Host)
				download := httptest.NewRecorder()
				engine.ServeHTTP(download, httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil))
				require.Equal(t, http.StatusOK, download.Code, download.Body.String())
				assert.Equal(t, "sls-video-bytes", download.Body.String())
			}
		})
	}
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 10000000-15187, user.Quota)
	assert.Equal(t, 10000000-15187, token.RemainQuota)
	assert.Equal(t, int32(8), submits.Load())
}
