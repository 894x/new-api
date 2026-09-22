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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenHubPluginSubmitQueryAndRefundEndToEnd(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.Task{}, &model.Log{}, &model.UserSubscription{}))
	previousPrices := ratio_setting.ModelPrice2JSONString()
	previousQuota, previousLog, previousBatch, previousCache := common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled
	previousFactory := service.GetTaskAdaptorFunc
	previousSecret, previousAddress := common.CryptoSecret, system_setting.TaskPublicAddress
	previousFetch := *system_setting.GetFetchSetting()
	common.CryptoSecret, system_setting.TaskPublicAddress = "tokenhub-artifact-test-secret", "https://gateway.example"
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
		common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = previousQuota, previousLog, previousBatch, previousCache
		service.GetTaskAdaptorFunc = previousFactory
		common.CryptoSecret, system_setting.TaskPublicAddress = previousSecret, previousAddress
		*system_setting.GetFetchSetting() = previousFetch
		service.InitHttpClient()
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"yt-video-fx":1,"fx-alias":1}`))
	common.QuotaPerUnit, common.LogConsumeEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled = 100, false, false, true
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
	service.InitHttpClient()
	t.Setenv("ASSET_STORAGE_ENABLED", "false")
	var submits atomic.Int32
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "channel credentials must not accompany artifact fetches")
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "video-bytes")
	}))
	t.Cleanup(media.Close)
	requests := make(chan map[string]any, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
		var body map[string]any
		if err := common.DecodeJson(r.Body, &body); !assert.NoError(t, err) {
			http.Error(w, "bad request", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/video/submit":
			requests <- body
			_, _ = fmt.Fprintf(w, `{"id":"upstream-%d","status":"queued"}`, submits.Add(1))
		case "/v1/api/video/query":
			assert.Equal(t, "yt-video-fx", body["model"])
			if body["id"] == "upstream-1" {
				_, _ = io.WriteString(w, `{"status":"failed","error":{"message":"provider rejected image"}}`)
			} else {
				assert.Equal(t, "upstream-2", body["id"])
				_, _ = fmt.Fprintf(w, `{"status":"completed","progress":100,"data":{"url":%q}}`, media.URL+"/video.mp4")
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	user := model.User{Username: "tokenhub-plugin-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 10000}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "tokenhubplugine2e", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 10000}
	require.NoError(t, model.DB.Create(&token).Error)
	mapping := `{"fx-alias":"yt-video-fx"}`
	channel := model.Channel{
		Type: constant.ChannelTypeTokenHub, Name: "tokenhub-plugin", Key: "upstream-key",
		Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL),
		Models: "yt-video-fx,fx-alias", Group: "default", ModelMapping: &mapping,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()
	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetTaskRouter(engine)

	for index, publicModel := range []string{"fx-alias", "yt-video-fx"} {
		request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(fmt.Sprintf(`{"model":%q,"template":"hug","resolution":"720p","future_field":false}`, publicModel)))
		request.Header.Set("Authorization", "Bearer tokenhubplugine2e")
		request.Header.Set("Content-Type", "application/json")
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
		assert.Equal(t, "yt-video-fx", wire["model"])
		assert.Equal(t, "hug", wire["template"])
		assert.Equal(t, false, wire["future_field"])
		var task model.Task
		require.NoError(t, model.DB.Where("task_id = ?", receipt.ID).First(&task).Error)
		assert.Equal(t, "tencent-tokenhub", string(task.Platform))
		assert.Equal(t, publicModel, task.Properties.OriginModelName)
		assert.Equal(t, "yt-video-fx", task.Properties.UpstreamModelName)
		assert.Equal(t, 200, task.Quota)
		require.NotNil(t, task.Properties.RequestParameters)
		assert.Equal(t, "720p", task.Properties.RequestParameters.Resolution)
		require.NotNil(t, task.PrivateData.Execution)
		require.NoError(t, service.UpdateVideoTasks(context.Background(), task.Platform,
			map[int][]string{channel.Id: {task.PrivateData.UpstreamTaskID}},
			map[string]*model.Task{task.PrivateData.UpstreamTaskID: &task}))
		require.NoError(t, model.DB.First(&task, task.ID).Error)
		if index == 0 {
			assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
			assert.Zero(t, task.Quota)
		} else {
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
			assert.Equal(t, 200, task.Quota)
		}
		query := httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
		query.Header.Set("Authorization", "Bearer tokenhubplugine2e")
		queryRecorder := httptest.NewRecorder()
		engine.ServeHTTP(queryRecorder, query)
		require.Equal(t, http.StatusOK, queryRecorder.Code, queryRecorder.Body.String())
		var video struct {
			ID       string         `json:"id"`
			TaskID   string         `json:"task_id"`
			Metadata map[string]any `json:"metadata"`
		}
		require.NoError(t, common.Unmarshal(queryRecorder.Body.Bytes(), &video))
		assert.Equal(t, task.TaskID, video.ID)
		assert.Equal(t, task.TaskID, video.TaskID)
		assert.NotContains(t, queryRecorder.Body.String(), media.URL)
		if index == 0 {
			assert.Empty(t, video.Metadata)
			continue
		}
		contentURL, ok := video.Metadata["url"].(string)
		require.True(t, ok)
		parsed, err := url.Parse(contentURL)
		require.NoError(t, err)
		assert.Equal(t, "gateway.example", parsed.Host)
		assert.Equal(t, "/v1/tasks/"+task.TaskID+"/artifacts/video/content", parsed.Path)
		for _, target := range []string{parsed.Path, parsed.Path + "?access=invalid"} {
			denied := httptest.NewRecorder()
			engine.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, target, nil))
			assert.NotEqual(t, http.StatusOK, denied.Code)
			assert.NotContains(t, denied.Body.String(), "video-bytes")
		}
		download := httptest.NewRecorder()
		engine.ServeHTTP(download, httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil))
		require.Equal(t, http.StatusOK, download.Code, download.Body.String())
		assert.Equal(t, "video-bytes", download.Body.String())
	}
	require.NoError(t, model.DB.First(&user, user.Id).Error)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 9800, user.Quota)
	assert.Equal(t, 9800, token.RemainQuota)
	assert.Equal(t, int32(2), submits.Load())
}
