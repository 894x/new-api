package doubao

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNativeRequestUsesExistingDoubaoBillingAndPreservesPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model:  "doubao-seedance-2-0-260128",
		Prompt: "Generate a tracking shot",
		Metadata: map[string]any{
			"model":          "doubao-seedance-2-0-260128",
			"resolution":     "1080p",
			"duration":       float64(5),
			"generate_audio": false,
			"content": []any{
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/input.mp4"}},
				map[string]any{"type": "text", "text": "Generate a tracking shot"},
			},
		},
	})

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mapped-seedance-model",
		},
	}

	ratios := adaptor.EstimateBilling(ctx, info)
	require.Contains(t, ratios, "video_input")
	assert.InDelta(t, 31.0/46.0, ratios["video_input"], 1e-12)

	requestBody, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(encoded, &payload))
	assert.Equal(t, "mapped-seedance-model", payload["model"])
	assert.Equal(t, "1080p", payload["resolution"])
	assert.Equal(t, false, payload["generate_audio"])
	content, ok := payload["content"].([]any)
	require.True(t, ok)
	assert.Len(t, content, 2)
}

func TestSeedance25BillingRatios(t *testing.T) {
	tests := []struct {
		name       string
		resolution string
		content    []any
		wantRatio  float64
	}{
		{
			name:       "1080p text to video",
			resolution: "1080p",
			content:    []any{map[string]any{"type": "text", "text": "A cinematic tracking shot"}},
			wantRatio:  77.0 / 70.0,
		},
		{
			name:       "1080p video input",
			resolution: "1080p",
			content: []any{
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/reference.mp4"}},
				map[string]any{"type": "text", "text": "Extend this video"},
			},
			wantRatio: 46.0 / 70.0,
		},
		{
			name:       "720p video input",
			resolution: "720p",
			content: []any{
				map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://example.com/reference.mp4"}},
				map[string]any{"type": "text", "text": "Extend this video"},
			},
			wantRatio: 42.0 / 70.0,
		},
		{
			name:       "720p text to video uses base price",
			resolution: "720p",
			content:    []any{map[string]any{"type": "text", "text": "A cinematic tracking shot"}},
			wantRatio:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("task_request", relaycommon.TaskSubmitReq{Metadata: map[string]any{
				"resolution": tt.resolution,
				"content":    tt.content,
			}})
			info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-5-260628"}

			ratios := (&TaskAdaptor{}).EstimateBilling(ctx, info)
			if tt.wantRatio == 1 {
				assert.Empty(t, ratios)
				return
			}
			require.Contains(t, ratios, "video_input")
			assert.InDelta(t, tt.wantRatio, ratios["video_input"], 1e-12)
		})
	}
}

func TestDoubaoModelListIncludesSeedance25(t *testing.T) {
	assert.Contains(t, (&TaskAdaptor{}).GetModelList(), "doubao-seedance-2-5-260628")
}

func TestNativeRequestRewritesLogicalAssetForCurrentChannel(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{}, &model.ChannelAssetConfig{}, &model.UserAsset{}, &model.UserAssetReplica{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.Channel{
		Id: 11, Type: constant.ChannelTypeDoubaoVideo, Key: "action-key", Name: "Doubao Video",
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, Backend: service.AssetLibraryBackendAction,
		BaseURL: service.DefaultAssetLibraryBaseURL, AuthType: service.AssetLibraryAuthAKSK,
		AccessKey: "access-key", SecretKey: "secret-key",
	}).Error)
	require.NoError(t, db.Create(&model.UserAsset{
		Id: assetId, UserId: 7, GroupId: "group-na-test", AssetType: "Image",
		SourceURL: "https://example.com/a.png", ProjectName: "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetId, ChannelId: 11, UpstreamAssetId: "asset-upstream-11",
		State: model.AssetReplicaStateReady,
	}).Error)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model: "doubao-seedance-2-0-260128",
		Metadata: map[string]any{
			"content": []any{map[string]any{
				"type": "image_url", "image_url": map[string]any{"url": "asset://" + assetId},
			}},
		},
	})
	info := &relaycommon.RelayInfo{UserId: 7, ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId: 11, UpstreamModelName: "mapped-model",
	}}

	requestBody, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"url":"asset://asset-upstream-11"`)
	assert.NotContains(t, string(encoded), assetId)
}

func TestVolcengineAssetReferenceIntegrationRewritesBeforeTaskSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type observedRequest struct {
		action        string
		authorization string
		body          map[string]any
	}

	assetRequests := make(chan observedRequest, 3)
	taskRequests := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := common.DecodeJson(r.Body, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/" {
			action := r.URL.Query().Get("Action")
			assetRequests <- observedRequest{
				action: action, authorization: r.Header.Get("Authorization"), body: body,
			}
			switch action {
			case "CreateAssetGroup":
				_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"req-group"},"Result":{"Id":"group-official-e2e"}}`)
			case "CreateAsset":
				_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"req-asset"},"Result":{"Id":"asset-official-e2e"}}`)
			case "GetAsset":
				_, _ = io.WriteString(w, `{"ResponseMetadata":{"RequestId":"req-get"},"Result":{"Id":"asset-official-e2e","Name":"Official E2E reference","URL":"https://example.com/official-e2e.png","GroupId":"group-official-e2e","AssetType":"Image","Status":"Active","ProjectName":"default"}}`)
			default:
				http.Error(w, "unexpected asset action", http.StatusBadRequest)
			}
			return
		}
		if r.URL.Path == "/api/v3/contents/generations/tasks" {
			taskRequests <- observedRequest{authorization: r.Header.Get("Authorization"), body: body}
			_, _ = io.WriteString(w, `{"id":"task-upstream-official-e2e"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{}, &model.ChannelAssetConfig{}, &model.UserAssetGroup{}, &model.UserAsset{},
		&model.UserAssetGroupReplica{}, &model.UserAssetReplica{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	const (
		userID    = 7
		channelID = 12
		assetID   = "asset-na-fedcba9876543210fedcba9876543210"
		groupID   = "group-na-fedcba9876543210fedcba9876543210"
	)
	require.NoError(t, db.Create(&model.Channel{
		Id: channelID, Type: constant.ChannelTypeDoubaoVideo, Key: "video-key-e2e", Name: "Mock Volcengine Video",
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: channelID, Enabled: true, Backend: service.AssetLibraryBackendAction,
		BaseURL: server.URL, AuthType: service.AssetLibraryAuthAKSK,
		AccessKey: "access-key-e2e", SecretKey: "secret-key-e2e", Region: service.DefaultAssetLibraryRegion,
		ProjectName: service.DefaultAssetLibraryProject,
	}).Error)
	group := &model.UserAssetGroup{
		Id: groupID, UserId: userID, Name: "Official E2E group", GroupType: "VideoGen", ProjectName: "default",
	}
	require.NoError(t, db.Create(group).Error)
	asset := &model.UserAsset{
		Id: assetID, UserId: userID, GroupId: groupID, Name: "Official E2E reference",
		SourceURL: "https://example.com/official-e2e.png", AssetType: "Image", ProjectName: "default",
	}
	require.NoError(t, db.Create(asset).Error)

	replication, err := service.ReplicateAsset(context.Background(), asset)
	require.NoError(t, err)
	require.Empty(t, replication.Errors)
	require.NotNil(t, replication.Summary)
	assert.Equal(t, "processing", replication.Summary.Status)
	details, err := service.RefreshAssetLibraryAsset(context.Background(), assetID)
	require.NoError(t, err)
	assert.Equal(t, "Active", details.Status)

	groupRequest := <-assetRequests
	assert.Equal(t, "CreateAssetGroup", groupRequest.action)
	assert.True(t, strings.HasPrefix(groupRequest.authorization, "HMAC-SHA256 Credential=access-key-e2e/"))
	assert.Equal(t, "Official E2E group", groupRequest.body["Name"])
	assetRequest := <-assetRequests
	assert.Equal(t, "CreateAsset", assetRequest.action)
	assert.Equal(t, "group-official-e2e", assetRequest.body["GroupId"])
	assert.Equal(t, "https://example.com/official-e2e.png", assetRequest.body["URL"])
	getRequest := <-assetRequests
	assert.Equal(t, "GetAsset", getRequest.action)
	assert.Equal(t, "asset-official-e2e", getRequest.body["Id"])

	router := gin.New()
	router.POST(
		"/api/v3/contents/generations/tasks",
		middleware.DoubaoVideoRequestConvert(),
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, userID)
			c.Next()
		},
		middleware.AssetLibraryRouting(),
		func(c *gin.Context) {
			info := &relaycommon.RelayInfo{
				UserId: userID, OriginModelName: "doubao-seedance-2-0-260128",
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: constant.ChannelTypeDoubaoVideo, ChannelId: channelID,
					ChannelBaseUrl: server.URL, ApiKey: "video-key-e2e",
					UpstreamModelName: "doubao-seedance-2-0-260128",
				},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task-public-official-e2e"},
			}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			requestBody, buildErr := adaptor.BuildRequestBody(c, info)
			require.NoError(t, buildErr)
			response, requestErr := adaptor.DoRequest(c, info, requestBody)
			require.NoError(t, requestErr)
			upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, response, info)
			require.Nil(t, taskErr)
			assert.Equal(t, "task-upstream-official-e2e", upstreamTaskID)
			assert.Contains(t, string(taskData), "task-upstream-official-e2e")
		},
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(fmt.Sprintf(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"image_url","image_url":{"url":"asset://%s"}},
			{"type":"text","text":"Animate the official asset"}
		]
	}`, assetID)))
	request.Header.Set("Content-Type", "application/json")
	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusOK, responseRecorder.Code)
	assert.JSONEq(t, `{"id":"task-public-official-e2e"}`, responseRecorder.Body.String())
	taskRequest := <-taskRequests
	assert.Equal(t, "Bearer video-key-e2e", taskRequest.authorization)
	content, ok := taskRequest.body["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	imagePart, ok := content[0].(map[string]any)
	require.True(t, ok)
	imageURL, ok := imagePart["image_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset://asset-official-e2e", imageURL["url"])
	assert.NotContains(t, fmt.Sprint(taskRequest.body), assetID)
}

func TestCompatibleRequestRejectsRawAssetURI(t *testing.T) {
	for _, request := range []relaycommon.TaskSubmitReq{
		{
			Model:  "doubao-seedance-2-0-260128",
			Prompt: "Generate a tracking shot",
			Images: []string{"asset://Asset-upstream-owned-by-another-user"},
		},
		{
			Model:  "doubao-seedance-2-0-260128",
			Prompt: "Generate a tracking shot",
			Images: []string{"asset://asset-na-0123456789abcdef0123456789abcdef"},
		},
		{
			Model:  "doubao-seedance-2-0-260128",
			Prompt: "Generate a tracking shot",
			Metadata: map[string]any{"content": []any{map[string]any{
				"type": "image_url", "image_url": map[string]any{"url": "ASSET://Asset-upstream"},
			}}},
		},
	} {
		gin.SetMode(gin.TestMode)
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Set("task_request", request)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mapped-model",
		}}

		_, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)

		require.ErrorContains(t, err, "only supported by the native asset-library endpoint")
	}
}

func TestNativeSubmitResponseUsesPublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"upstream-task-id"}`)),
	}

	adaptor := &TaskAdaptor{}
	upstreamID, taskData, taskErr := adaptor.DoResponse(ctx, response, &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	})

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-task-id", upstreamID)
	assert.JSONEq(t, `{"id":"upstream-task-id"}`, string(taskData))
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestConvertToNativeVideoNormalizesOfficialResponse(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		CreatedAt: 100,
		UpdatedAt: 200,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
		PrivateData: model.TaskPrivateData{ResultURL: "https://example.com/output.mp4"},
		Data: json.RawMessage(`{
			"id":"upstream-task-id",
			"model":"upstream-model",
			"status":"succeeded",
			"content":{
				"video_url":"https://example.com/output.mp4",
				"kz_video_url":"https://channel.example.com/output.mp4",
				"last_frame_url":"https://example.com/frame.png"
			},
			"seed":93073,
			"resolution":"480p",
			"ratio":"16:9",
			"duration":4,
			"framespersecond":24,
			"service_tier":"default",
			"execution_expires_after":172800,
			"generate_audio":true,
			"tools":[{"type":"web_search"}],
			"safety_identifier":"user-123",
			"priority":0,
			"draft":false,
			"draft_task_id":"task_draft",
			"usage":{
				"completion_tokens":35000,
				"total_tokens":35000,
				"tool_usage":{"web_search":1}
			}
		}`),
	}

	encoded, err := (&TaskAdaptor{}).ConvertToNativeVideo(task)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"task_public",
		"model":"doubao-seedance-2-0-260128",
		"status":"succeeded",
		"created_at":100,
		"updated_at":200,
		"content":{
			"video_url":"https://example.com/output.mp4",
			"last_frame_url":"https://example.com/frame.png"
		},
		"seed":93073,
		"resolution":"480p",
		"ratio":"16:9",
		"duration":4,
		"framespersecond":24,
		"generate_audio":true,
		"tools":[{"type":"web_search"}],
		"safety_identifier":"user-123",
		"priority":0,
		"draft":false,
		"draft_task_id":"task_draft",
		"service_tier":"default",
		"execution_expires_after":172800,
		"usage":{
			"completion_tokens":35000,
			"total_tokens":35000,
			"tool_usage":{"web_search":1}
		}
	}`, string(encoded))
}

func TestConvertToNativeVideoReturnsOfficialFailureShape(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_failed",
		Status:     model.TaskStatusFailure,
		CreatedAt:  100,
		UpdatedAt:  200,
		FailReason: "upstream generation failed",
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
	}

	encoded, err := (&TaskAdaptor{}).ConvertToNativeVideo(task)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"task_failed",
		"model":"doubao-seedance-2-0-260128",
		"status":"failed",
		"created_at":100,
		"updated_at":200,
		"error":{"code":"","message":"upstream generation failed"}
	}`, string(encoded))
}
