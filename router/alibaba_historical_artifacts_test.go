package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlibabaHistoricalResultURLsRemainDownloadable(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.Task{}))
	previousSecret, previousAddress := common.CryptoSecret, system_setting.TaskPublicAddress
	previousFetch := *system_setting.GetFetchSetting()
	common.CryptoSecret, system_setting.TaskPublicAddress = "wan-historical-secret", "https://gateway.example"
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	service.InitHttpClient()
	t.Cleanup(func() {
		common.CryptoSecret, system_setting.TaskPublicAddress = previousSecret, previousAddress
		*system_setting.GetFetchSetting() = previousFetch
		service.InitHttpClient()
	})
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "CDN requests must not receive channel credentials")
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "historical-wan-video")
	}))
	t.Cleanup(media.Close)
	user := model.User{Username: "wan-historical", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "wanhistorical", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	channel := model.Channel{Type: constant.ChannelTypeAli, Name: "wan-historical", Status: common.ChannelStatusEnabled,
		Key: "private-channel-key", BaseURL: common.GetPointer("https://provider.example"), Models: "wan3.0-video,wan2.5-i2v-preview", Group: "default"}
	require.NoError(t, model.DB.Create(&channel).Error)
	model.InitChannelCache()
	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetTaskRouter(engine)
	engine.NoRoute(SetPluginRouter(engine))
	for _, modelName := range []string{"wan3.0-video", "wan2.5-i2v-preview"} {
		for _, storage := range []string{"private_result_url", "legacy_fail_reason"} {
			t.Run(modelName+"/"+storage, func(t *testing.T) {
				task := model.Task{TaskID: model.GenerateTaskID(), Platform: "17", Status: model.TaskStatusSuccess, Action: constant.TaskActionGenerate,
					UserId: user.Id, ChannelId: channel.Id, Data: []byte(`{}`),
					Properties:  model.Properties{OriginModelName: modelName, UpstreamModelName: modelName},
					PrivateData: model.TaskPrivateData{UpstreamTaskID: "private-upstream-task"}}
				if storage == "private_result_url" {
					task.PrivateData.ResultURL = media.URL + "/video.mp4"
				} else {
					task.FailReason = media.URL + "/video.mp4"
				}
				require.NoError(t, model.DB.Create(&task).Error)
				for _, prefix := range []string{"", "/ali"} {
					request := httptest.NewRequest(http.MethodGet, prefix+"/api/v1/tasks/"+task.TaskID, nil)
					request.Header.Set("Authorization", "Bearer wanhistorical")
					recorder := httptest.NewRecorder()
					engine.ServeHTTP(recorder, request)
					require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
					assert.JSONEq(t, `{"output":{"task_id":"`+task.TaskID+`","task_status":"SUCCEEDED","video_url":"`+media.URL+`/video.mp4"}}`, recorder.Body.String())
				}
				request := httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
				request.Header.Set("Authorization", "Bearer wanhistorical")
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				assert.NotContains(t, recorder.Body.String(), media.URL)
				assert.NotContains(t, recorder.Body.String(), "private-upstream-task")
				var video struct {
					ID       string         `json:"id"`
					Status   string         `json:"status"`
					Metadata map[string]any `json:"metadata"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &video))
				assert.Equal(t, task.TaskID, video.ID)
				assert.Equal(t, "completed", video.Status)
				contentURL, ok := video.Metadata["url"].(string)
				require.True(t, ok, "historical success must retain its video artifact")
				parsed, err := url.Parse(contentURL)
				require.NoError(t, err)
				assert.Equal(t, "gateway.example", parsed.Host)
				denied := httptest.NewRecorder()
				engine.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, parsed.Path, nil))
				assert.NotEqual(t, http.StatusOK, denied.Code)
				download := httptest.NewRecorder()
				engine.ServeHTTP(download, httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil))
				require.Equal(t, http.StatusOK, download.Code, download.Body.String())
				assert.Equal(t, "historical-wan-video", download.Body.String())
			})
		}
	}
}
