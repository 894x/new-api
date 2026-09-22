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

func TestHailuoHistoricalResultURLsRemainDownloadable(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.Task{}))
	previousSecret, previousAddress := common.CryptoSecret, system_setting.TaskPublicAddress
	previousFetch := *system_setting.GetFetchSetting()
	common.CryptoSecret, system_setting.TaskPublicAddress = "hailuo-historical-secret", "https://gateway.example"
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	service.InitHttpClient()
	t.Cleanup(func() {
		common.CryptoSecret, system_setting.TaskPublicAddress = previousSecret, previousAddress
		*system_setting.GetFetchSetting() = previousFetch
		service.InitHttpClient()
	})
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "legacy CDN requests must not receive channel credentials")
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "historical-hailuo-video")
	}))
	t.Cleanup(media.Close)
	user := model.User{Username: "hailuo-historical", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "hailuohistorical", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(&token).Error)
	channel := model.Channel{Type: constant.ChannelTypeMiniMax, Name: "hailuo-historical", Status: common.ChannelStatusEnabled,
		Key: "private-channel-key", BaseURL: common.GetPointer("https://provider.example"), Models: "MiniMax-H3,T2V-01-Director", Group: "default"}
	require.NoError(t, model.DB.Create(&channel).Error)
	model.InitChannelCache()
	engine := gin.New()
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	SetTaskRouter(engine)
	for _, modelName := range []string{"MiniMax-H3", "T2V-01-Director"} {
		for _, storage := range []string{"private_result_url", "legacy_fail_reason"} {
			t.Run(modelName+"/"+storage, func(t *testing.T) {
				task := model.Task{TaskID: model.GenerateTaskID(), Platform: "35", Status: model.TaskStatusSuccess, Action: constant.TaskActionGenerate,
					UserId: user.Id, ChannelId: channel.Id, Data: []byte(`{}`),
					Properties:  model.Properties{OriginModelName: modelName, UpstreamModelName: modelName},
					PrivateData: model.TaskPrivateData{UpstreamTaskID: "private-upstream-task"}}
				if storage == "private_result_url" {
					task.PrivateData.ResultURL = media.URL + "/video.mp4"
				} else {
					task.FailReason = media.URL + "/video.mp4"
				}
				require.NoError(t, model.DB.Create(&task).Error)
				request := httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
				request.Header.Set("Authorization", "Bearer hailuohistorical")
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				assert.NotContains(t, recorder.Body.String(), media.URL)
				assert.NotContains(t, recorder.Body.String(), "private-upstream-task")
				var video struct {
					ID       string         `json:"id"`
					Metadata map[string]any `json:"metadata"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &video))
				assert.Equal(t, task.TaskID, video.ID)
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
				assert.Equal(t, "historical-hailuo-video", download.Body.String())
			})
		}
	}
}
