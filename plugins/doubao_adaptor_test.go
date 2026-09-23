package plugins_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDoubaoPluginHistoricalArtifactsAndLastFrame(t *testing.T) {
	adaptor := taskplugin.New(doubaoPlugin(t))
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example", ApiKey: "private-key"}})
	for _, tc := range []struct {
		name, data, key, wantURL string
		count                    int
	}{
		{"legacy URL", "", "video", "https://legacy.example/video.mp4", 1},
		{"provider URL wins", `{"content":{"video_url":"https://cdn.example/current.mp4"}}`, "video", "https://cdn.example/current.mp4", 1},
		{"last frame", `{"content":{"last_frame_url":"https://cdn.example/last.png"}}`, "last_frame", "https://cdn.example/last.png", 2},
		{"wrapped provider data", `{"data":{"task_id":"public-task","data":{"content":{"video_url":"https://cdn.example/wrapped.mp4"}}}}`, "video", "https://cdn.example/wrapped.mp4", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{TaskID: "public-task", Status: model.TaskStatusSuccess, Data: []byte(tc.data),
				PrivateData: model.TaskPrivateData{ResultURL: "https://legacy.example/video.mp4"}}
			artifacts, err := adaptor.ListArtifacts(task)
			require.NoError(t, err)
			require.Len(t, artifacts, tc.count)
			descriptor, err := adaptor.BuildContentRequest(task, tc.key, channel.TaskArtifactClientRequest{Method: http.MethodHead})
			require.NoError(t, err)
			assert.Equal(t, tc.wantURL, descriptor.URL)
			assert.Equal(t, http.MethodHead, descriptor.Method)
			assert.True(t, descriptor.Credentialless)
			assert.Empty(t, descriptor.Headers)
			assert.Empty(t, descriptor.Body)
			for _, status := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusFailure} {
				task.Status = status
				artifacts, err = adaptor.ListArtifacts(task)
				require.NoError(t, err)
				assert.Empty(t, artifacts)
			}
		})
	}
}

func TestDoubaoPluginRetainsPublicModelsAndDefaultPrices(t *testing.T) {
	assert.Equal(t, []string{"doubao-seedance-1-0-pro-250528", "doubao-seedance-1-0-lite-t2v", "doubao-seedance-1-0-lite-i2v", "doubao-seedance-1-5-pro-251215",
		"doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615", "doubao-seedance-2-5-260628"}, taskplugin.New(doubaoPlugin(t)).GetModelList())
	assert.InDelta(t, 4.794520547945205, ratio_setting.GetDefaultModelRatioMap()["doubao-seedance-2-5-260628"], 1e-12)
}

func TestDoubaoPluginConfiguredTransportPaths(t *testing.T) {
	for _, tc := range []struct {
		name, settings, submit, query string
	}{
		{"default", "{}", "/api/v3/contents/generations/tasks", "/api/v3/contents/generations/tasks/task_123"},
		{"v3", `{"doubao_video_api_mode":"v3"}`, "/api/v3/contents/generations/tasks", "/api/v3/contents/generations/tasks/task_123"},
		{"preset", `{"doubao_video_api_mode":"video_generations"}`, "/v1/video/generations", "/v1/video/generations/task_123"},
		{"custom", `{"doubao_video_api_mode":"custom","doubao_video_submit_path":"/custom/video/tasks","doubao_video_fetch_path":"/custom/video/results/{id}"}`, "/custom/video/tasks", "/custom/video/results/task_123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type capturedRequest struct{ method, path, auth, body string }
			requests := make(chan capturedRequest, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				requests <- capturedRequest{r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), string(body)}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"task_123","status":"queued"}`)
			}))
			t.Cleanup(server.Close)
			var settings kitdto.ChannelOtherSettings
			require.NoError(t, common.UnmarshalJsonStr(tc.settings, &settings))
			info := &relaycommon.RelayInfo{
				OriginModelName: "doubao-seedance-2-0-260128",
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl: server.URL + "/", ApiKey: "channel-test-key",
					UpstreamModelName: "mapped-model", ChannelOtherSettings: settings,
				},
			}
			adaptor := taskplugin.New(doubaoPlugin(t))
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"doubao-seedance-2-0-260128","prompt":"a fox","seconds":5,"image":"https://cdn.example/frame.png","metadata":{"generate_audio":false}}`))
			c.Request.Header.Set("Content-Type", "application/json")
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			response, err := adaptor.DoRequest(c, info, body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			submit := <-requests
			assert.Equal(t, http.MethodPost, submit.method)
			assert.Equal(t, tc.submit, submit.path)
			assert.Equal(t, "Bearer channel-test-key", submit.auth)
			assert.JSONEq(t, `{"model":"mapped-model","duration":5,"generate_audio":false,"content":[{"type":"image_url","image_url":{"url":"https://cdn.example/frame.png"}},{"type":"text","text":"a fox"}]}`, submit.body)

			response, err = adaptor.FetchTask(server.URL+"/", "poll-key", &model.Task{TaskID: "task_123"}, "")
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			query := <-requests
			assert.Equal(t, http.MethodGet, query.method)
			assert.Equal(t, tc.query, query.path)
			assert.Equal(t, "Bearer poll-key", query.auth)
		})
	}
}

func TestDoubaoPluginRejectsInvalidConfiguredPaths(t *testing.T) {
	for _, tc := range []struct{ mode, submit, query string }{
		{"custom", "", "/tasks/{id}"},
		{"custom", "//other.example/tasks", "/tasks/{id}"},
		{"custom", "https://other.example/tasks", "/tasks/{id}"},
		{"custom", "/tasks", "//other.example/tasks/{id}"},
		{"custom", "/tasks", "/tasks/result"},
		{"unsupported", "", ""},
	} {
		t.Run(tc.mode+tc.submit+tc.query, func(t *testing.T) {
			adaptor := taskplugin.New(doubaoPlugin(t))
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: "https://provider.example",
				ChannelOtherSettings: kitdto.ChannelOtherSettings{
					DoubaoVideoAPIMode: kitdto.DoubaoVideoAPIMode(tc.mode), DoubaoVideoSubmitPath: tc.submit, DoubaoVideoFetchPath: tc.query,
				},
			}}
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			c.Set("task_request", map[string]any{"model": "model", "prompt": "fox"})
			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			_, err := adaptor.FetchTask("https://provider.example", "", &model.Task{TaskID: "task_123"}, "")
			require.Error(t, err)
		})
	}
}

func TestDoubaoPluginNativePayloadPreservesContentAndMappedIdentity(t *testing.T) {
	plugin := doubaoPlugin(t)
	var request map[string]any
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"doubao-seedance-2-0-260128","duration":5,"seed":0,"generate_audio":false,"future_field":{"enabled":false},"resolution":"1080p","content":[{"type":"text","text":"first","weight":0},{"type":"video_url","video_url":{"url":"https://cdn.example/input.mp4"}},{"type":"text","text":"second"},{"type":"draft_task","draft_task":{"id":"public-draft","extra":false}}]}`, &request))
	value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": request}})
	require.NoError(t, err)
	intent, ok := value.(map[string]any)
	require.True(t, ok)
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{OriginTasks: []relaycommon.OriginTaskRef{{TaskID: "public-draft", UpstreamTaskID: "provider-draft"}}},
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example", UpstreamModelName: "mapped-model"},
	}
	adaptor := taskplugin.New(plugin)
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/doubao/api/v3/contents/generations/tasks", nil)
	c.Set("task_request", intent["requestBody"])
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	ratios, err := adaptor.EstimateBillingValidated(c, info)
	require.NoError(t, err)
	assert.InDelta(t, 31.0/46.0, ratios["video_input_ratio"], 1e-12)
	facts, err := adaptor.ExtractUsageFactsValidated(c, info)
	require.NoError(t, err)
	assert.Equal(t, "video", facts["video_input"])
	assert.Equal(t, "1080p", facts["resolution"])
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"mapped-model","duration":5,"seed":0,"generate_audio":false,"future_field":{"enabled":false},"resolution":"1080p","content":[{"type":"text","text":"first","weight":0},{"type":"video_url","video_url":{"url":"https://cdn.example/input.mp4"}},{"type":"text","text":"second"},{"type":"draft_task","draft_task":{"id":"provider-draft","extra":false}}]}`, string(encoded))
}

func TestDoubaoPluginSeedance25ApprovedUpstreamRatios(t *testing.T) {
	for _, tc := range []struct {
		resolution string
		video      bool
		want       float64
	}{
		{"1080p", false, 11.7 / 10.7},
		{"1080p", true, 7.0 / 10.7},
		{"720p", true, 42.0 / 70.0},
		{"720p", false, 1},
	} {
		t.Run(tc.resolution+map[bool]string{true: "-video", false: "-text"}[tc.video], func(t *testing.T) {
			adaptor := taskplugin.New(doubaoPlugin(t))
			info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-5-260628", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{}}
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			content := []any{map[string]any{"type": "text", "text": "fox"}}
			if tc.video {
				content = append(content, map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/input.mp4"}})
			}
			c.Set("task_request", map[string]any{"metadata": map[string]any{"resolution": tc.resolution, "content": content}})
			ratios, err := adaptor.EstimateBillingValidated(c, info)
			require.NoError(t, err)
			if tc.want == 1 {
				assert.Empty(t, ratios)
			} else {
				assert.InDelta(t, tc.want, ratios["video_input_ratio"], 1e-12)
			}
		})
	}
}

func TestDoubaoPluginUsesOwnedReplicaForSelectedChannel(t *testing.T) {
	t.Setenv("ASSET_STORAGE_ENABLED", "false")
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAssetConfig{}, &model.UserAsset{}, &model.UserAssetReplica{}))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	const assetID = "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.UserAsset{Id: assetID, UserId: 7, GroupId: "group-na-test", AssetType: "Image", SourceURL: "https://example.com/image.png"}).Error)
	for _, id := range []int{11, 12} {
		require.NoError(t, db.Create(&model.Channel{Id: id, Type: constant.ChannelTypeDoubaoVideo, Key: "provider-key"}).Error)
		require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: id, Enabled: true, Backend: service.AssetLibraryBackendAction, BaseURL: service.DefaultAssetLibraryBaseURL, AuthType: service.AssetLibraryAuthAKSK, AccessKey: "private-ak", SecretKey: "private-sk"}).Error)
		require.NoError(t, db.Create(&model.UserAssetReplica{AssetId: assetID, ChannelId: id, UpstreamAssetId: fmt.Sprintf("provider-asset-%d", id), State: model.AssetReplicaStateReady}).Error)
	}
	for _, tc := range []struct{ user, channel int }{{7, 11}, {7, 12}, {8, 11}} {
		t.Run(fmt.Sprintf("user-%d-channel-%d", tc.user, tc.channel), func(t *testing.T) {
			adaptor := taskplugin.New(doubaoPlugin(t))
			info := &relaycommon.RelayInfo{UserId: tc.user, OriginModelName: "doubao-seedance-2-0-260128", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: tc.channel, ChannelBaseUrl: "https://provider.example", UpstreamModelName: "mapped-model"}}
			adaptor.Init(info)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
			c.Set("task_request", map[string]any{"model": "doubao-seedance-2-0-260128", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "asset://" + assetID}}}})
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			reader, err := adaptor.BuildRequestBody(c, info)
			if tc.user != 7 {
				require.Error(t, err)
				assert.Nil(t, reader)
				return
			}
			require.NoError(t, err)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			assert.Contains(t, string(body), fmt.Sprintf("asset://provider-asset-%d", tc.channel))
			assert.NotContains(t, string(body), assetID)
			assert.NotContains(t, string(body), "private-ak")
			assert.NotContains(t, string(body), "private-sk")
		})
	}
}
