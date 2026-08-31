package router

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmitVideoTaskWithAccountAssetURIEndToEnd(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelAssetConfig{},
		&model.UserAsset{},
		&model.UserAssetReplica{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.Task{},
		&model.UserSubscription{},
	))
	ratio_setting.InitRatioSettings()

	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

	type observedRequest struct {
		path          string
		authorization string
		body          map[string]any
	}
	upstreamRequests := make(chan observedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := common.DecodeJson(request.Body, &body); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		upstreamRequests <- observedRequest{
			path: request.URL.Path, authorization: request.Header.Get("Authorization"), body: body,
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"task-upstream-route-e2e"}`)
	}))
	t.Cleanup(upstream.Close)

	user := model.User{
		Username: "asset-task-route-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 1_000_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId: user.Id, Key: "assettaskroutee2ekey", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	priority := int64(100)
	channel := model.Channel{
		Type: constant.ChannelTypeDoubaoVideo, Name: "asset-task-route-upstream", Key: "video-upstream-key",
		Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL),
		Models: "doubao-seedance-2-0-260128", Group: "default", Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, model.DB.Create(&model.ChannelAssetConfig{
		ChannelId: channel.Id, Enabled: true, Backend: service.AssetLibraryBackendAction,
		BaseURL: service.DefaultAssetLibraryBaseURL, AuthType: service.AssetLibraryAuthAKSK,
		AccessKey: "asset-access-key", SecretKey: "asset-secret-key",
		Region: service.DefaultAssetLibraryRegion, ProjectName: service.DefaultAssetLibraryProject,
	}).Error)

	const accountAssetID = "asset-na-11111111111111111111111111111111"
	require.NoError(t, model.DB.Create(&model.UserAsset{
		Id: accountAssetID, UserId: user.Id, GroupId: "group-na-route-e2e",
		Name: "route e2e reference", SourceURL: "https://example.com/route-e2e.png",
		AssetType: "Image", ProjectName: "default",
	}).Error)
	require.NoError(t, model.DB.Create(&model.UserAssetReplica{
		AssetId: accountAssetID, ChannelId: channel.Id, UpstreamAssetId: "asset-upstream-route-e2e",
		State: model.AssetReplicaStateReady, UpstreamStatus: "Active",
	}).Error)
	model.InitChannelCache()

	engine := gin.New()
	SetVideoRouter(engine)
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)

	requestBody := []byte(fmt.Sprintf(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"image_url","image_url":{"url":"asset://%s"}},
			{"type":"text","text":"Animate the account asset"}
		],
		"duration":4,
		"resolution":"480p",
		"ratio":"16:9"
	}`, accountAssetID))
	request, err := http.NewRequest(
		http.MethodPost,
		gateway.URL+"/api/v3/contents/generations/tasks",
		bytes.NewReader(requestBody),
	)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer assettaskroutee2ekey")
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(responseBody))

	var publicResponse struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(responseBody, &publicResponse))
	assert.NotEmpty(t, publicResponse.ID)
	assert.NotEqual(t, "task-upstream-route-e2e", publicResponse.ID)

	upstreamRequest := <-upstreamRequests
	assert.Equal(t, "/api/v3/contents/generations/tasks", upstreamRequest.path)
	assert.Equal(t, "Bearer video-upstream-key", upstreamRequest.authorization)
	content, ok := upstreamRequest.body["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	imagePart, ok := content[0].(map[string]any)
	require.True(t, ok)
	imageURL, ok := imagePart["image_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset://asset-upstream-route-e2e", imageURL["url"])
	assert.NotContains(t, fmt.Sprint(upstreamRequest.body), accountAssetID)

	var persistedTask model.Task
	require.NoError(t, model.DB.Where("task_id = ?", publicResponse.ID).First(&persistedTask).Error)
	assert.Equal(t, channel.Id, persistedTask.ChannelId)
	assert.Equal(t, "task-upstream-route-e2e", persistedTask.PrivateData.UpstreamTaskID)
}
