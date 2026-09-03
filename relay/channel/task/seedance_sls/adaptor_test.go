package seedance_sls

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
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildRequestURLUsesSLSVideoEndpoint(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://lm.sls.cn/"}}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	got, err := adaptor.BuildRequestURL(info)

	require.NoError(t, err)
	assert.Equal(t, "https://lm.sls.cn/v1/video/generations", got)
}

func TestValidateNativeSLSRequestStoresLosslessPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{
		"model":"doubao-seedance-2-0",
		"content":[{"type":"text","text":"A cat runs through neon rain"}],
		"duration":5,
		"generate_audio":false,
		"seed":0
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(ctx)
	info := &relaycommon.RelayInfo{}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, info)

	require.Nil(t, taskErr)
	req, err := relaycommon.GetTaskRequest(ctx)
	require.NoError(t, err)
	assert.Equal(t, "A cat runs through neon rain", req.Prompt)
	assert.Equal(t, 5, req.Duration)
	assert.Equal(t, constant.TaskActionGenerate, info.Action)
}

func TestEstimateBillingUsesNativeSLSResolutionAndVideoInputPricing(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantRatio float64
	}{
		{
			name: "1080p text to video",
			body: `{
				"model":"doubao-seedance-2-0-260128",
				"content":[{"type":"text","text":"A cat runs through neon rain"}],
				"resolution":"1080p"
			}`,
			wantRatio: 51.0 / 46.0,
		},
		{
			name: "1080p video input",
			body: `{
				"model":"doubao-seedance-2-0-260128",
				"content":[
					{"type":"video_url","video_url":{"url":"https://example.com/reference.mp4"}},
					{"type":"text","text":"Extend this video"}
				],
				"resolution":"1080p"
			}`,
			wantRatio: 31.0 / 46.0,
		},
		{
			name: "4k text to video",
			body: `{
				"model":"doubao-seedance-2-0-260128",
				"content":[{"type":"text","text":"A cat runs through neon rain"}],
				"resolution":"4K"
			}`,
			wantRatio: 26.0 / 46.0,
		},
		{
			name: "720p text to video uses base price",
			body: `{
				"model":"doubao-seedance-2-0-260128",
				"content":[{"type":"text","text":"A cat runs through neon rain"}],
				"resolution":"720p"
			}`,
			wantRatio: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(tt.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(ctx)
			info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-0-260128"}
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))

			ratios := adaptor.EstimateBilling(ctx, info)
			if tt.wantRatio == 1 {
				assert.Empty(t, ratios)
				return
			}
			require.Contains(t, ratios, "video_input")
			assert.InDelta(t, tt.wantRatio, ratios["video_input"], 1e-12)
		})
	}
}

func TestEstimateBillingUsesCompatibleSLSMetadataPricing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Metadata: map[string]any{
			"resolution": "1080p",
			"content": []any{
				map[string]any{"type": "text", "text": "A cat runs through neon rain"},
			},
		},
	})
	info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-0-260128"}

	ratios := (&TaskAdaptor{}).EstimateBilling(ctx, info)

	require.Contains(t, ratios, "video_input")
	assert.InDelta(t, 51.0/46.0, ratios["video_input"], 1e-12)
}

func TestEstimateBillingUsesSeedance25Pricing(t *testing.T) {
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
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			payload := map[string]any{
				"model":      "doubao-seedance-2-5-260628",
				"content":    tt.content,
				"resolution": tt.resolution,
			}
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(string(body)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(ctx)
			info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-5-260628"}
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))

			ratios := adaptor.EstimateBilling(ctx, info)
			if tt.wantRatio == 1 {
				assert.Empty(t, ratios)
				return
			}
			require.Contains(t, ratios, "video_input")
			assert.InDelta(t, tt.wantRatio, ratios["video_input"], 1e-12)
		})
	}
}

func TestSeedanceSLSRejectsMultipartOpenAIVideoRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader("multipart-body"))
	ctx.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test-boundary")

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, &relaycommon.RelayInfo{})

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Contains(t, taskErr.Message, "application/json")
}

func TestBuildNativeSLSRequestPreservesOptionalZeroValuesAndMapsModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{
		"model":"doubao-seedance-2-0",
		"content":[{"type":"text","text":"A cat runs through neon rain"}],
		"generate_audio":false,
		"seed":0
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(ctx)
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mapped-seedance-model",
			IsModelMapped:     true,
		},
	}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))

	body, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"model":"mapped-seedance-model",
		"content":[{"type":"text","text":"A cat runs through neon rain"}],
		"generate_audio":false,
		"seed":0
	}`, string(encoded))
}

func TestNativeSLSRequestRejectsUnsafeDuration(t *testing.T) {
	for _, duration := range []string{"3601", "1e100"} {
		t.Run(duration, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(fmt.Sprintf(`{
				"model":"doubao-seedance-2-0",
				"content":[{"type":"text","text":"A safe prompt"}],
				"duration":%s
			}`, duration)))
			ctx.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(ctx)

			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, &relaycommon.RelayInfo{})

			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, "invalid_seconds", taskErr.Code)
		})
	}
}

func TestCompatibleSLSRequestRejectsUnsafeMetadataDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{
		"model":"doubao-seedance-2-0",
		"prompt":"A safe prompt",
		"metadata":{"duration":9999999999}
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(ctx)

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(ctx, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	})

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "invalid_seconds", taskErr.Code)
}

func TestBuildNativeSLSRequestRewritesAccountAsset(t *testing.T) {
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

	const assetID = "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.Channel{
		Id: 11, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS",
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, Backend: service.AssetLibraryBackendSeedanceSLS,
		BaseURL: "https://lm.sls.cn", AuthType: service.AssetLibraryAuthBearer, APIKey: "sls-key",
	}).Error)
	require.NoError(t, db.Create(&model.UserAsset{
		Id: assetID, UserId: 7, GroupId: "group-na-test", AssetType: "Image",
		SourceURL: "https://example.com/reference.png", ProjectName: "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetID, ChannelId: 11, UpstreamAssetId: "asset-upstream-sls",
		State: model.AssetReplicaStateReady,
	}).Error)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{
		"model":"doubao-seedance-2-0",
		"content":[
			{"type":"image_url","image_url":{"url":"asset://asset-na-0123456789abcdef0123456789abcdef"}},
			{"type":"text","text":"Animate this image"}
		]
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(ctx)
	info := &relaycommon.RelayInfo{
		UserId: 7,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         11,
			UpstreamModelName: "doubao-seedance-2-0",
		},
	}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))

	body, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"url":"asset://asset-upstream-sls"`)
	assert.NotContains(t, string(encoded), assetID)
}

func TestReplicateAndSubmitAccountAssetIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type observedRequest struct {
		authorization string
		body          map[string]any
	}

	assetRequests := make(chan observedRequest, 1)
	taskRequests := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := common.DecodeJson(r.Body, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/volcengine/assets":
			assetRequests <- observedRequest{authorization: r.Header.Get("Authorization"), body: body}
			_, _ = io.WriteString(w, `{"success":true,"data":{"logical_id":"lass_e2e_asset","logical_group_id":"lass_e2e_group","status":"Active"}}`)
		case "/v1/video/generations":
			taskRequests <- observedRequest{authorization: r.Header.Get("Authorization"), body: body}
			_, _ = io.WriteString(w, `{"task_id":"task_upstream_e2e","status":"QUEUED"}`)
		default:
			http.NotFound(w, r)
		}
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
		channelID = 11
		assetID   = "asset-na-0123456789abcdef0123456789abcdef"
		groupID   = "group-na-0123456789abcdef0123456789abcdef"
	)
	require.NoError(t, db.Create(&model.Channel{
		Id: channelID, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-video-key", Name: "Mock Seedance SLS",
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: channelID, Enabled: true, Backend: service.AssetLibraryBackendSeedanceSLS,
		BaseURL: server.URL, AuthType: service.AssetLibraryAuthBearer, APIKey: "sls-asset-key",
	}).Error)
	group := &model.UserAssetGroup{
		Id: groupID, UserId: userID, Name: "E2E group", GroupType: "VideoGen", ProjectName: "default",
	}
	require.NoError(t, db.Create(group).Error)
	asset := &model.UserAsset{
		Id: assetID, UserId: userID, GroupId: groupID, Name: "E2E reference",
		SourceURL: "https://example.com/e2e-reference.png", AssetType: "Image", ProjectName: "default",
	}
	require.NoError(t, db.Create(asset).Error)

	replication, err := service.ReplicateAsset(context.Background(), asset)
	require.NoError(t, err)
	require.Empty(t, replication.Errors)
	require.NotNil(t, replication.Summary)
	assert.Equal(t, 1, replication.Summary.Ready)
	assert.Equal(t, "ready", replication.Summary.Status)
	assetRequest := <-assetRequests
	assert.Equal(t, "Bearer sls-asset-key", assetRequest.authorization)
	assert.Equal(t, "https://example.com/e2e-reference.png", assetRequest.body["source_url"])
	assert.Equal(t, "E2E group", assetRequest.body["group_name"])

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(fmt.Sprintf(`{
		"model":"doubao-seedance-2-0",
		"content":[
			{"type":"image_url","image_url":{"url":"asset://%s"}},
			{"type":"text","text":"Animate the E2E reference"}
		]
	}`, assetID)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(ctx)
	info := &relaycommon.RelayInfo{
		UserId: userID, OriginModelName: "doubao-seedance-2-0",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID, ChannelBaseUrl: server.URL, ApiKey: "sls-video-key",
			UpstreamModelName: "doubao-seedance-2-0",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_e2e"},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(ctx, info))
	requestBody, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	response, err := adaptor.DoRequest(ctx, info, requestBody)
	require.NoError(t, err)
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(ctx, response, info)
	require.Nil(t, taskErr)

	taskRequest := <-taskRequests
	assert.Equal(t, "Bearer sls-video-key", taskRequest.authorization)
	content, ok := taskRequest.body["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	imagePart, ok := content[0].(map[string]any)
	require.True(t, ok)
	imageURL, ok := imagePart["image_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset://lass_e2e_asset", imageURL["url"])
	assert.NotContains(t, string(taskData), "task_upstream_e2e")
	assert.Contains(t, string(taskData), "task_public_e2e")
	assert.Equal(t, "task_upstream_e2e", upstreamTaskID)
	assert.Contains(t, recorder.Body.String(), "task_public_e2e")
}

func TestBuildCompatibleRequestConvertsPromptAndImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-2-0-fast",
		Prompt:   "Camera circles the subject",
		Images:   []string{"https://example.com/reference.png"},
		Duration: 5,
		Metadata: map[string]any{"generate_audio": false, "seed": float64(0)},
	})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "doubao-seedance-2-0-fast"}}

	body, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"model":"doubao-seedance-2-0-fast",
		"content":[
			{"type":"image_url","image_url":{"url":"https://example.com/reference.png"}},
			{"type":"text","text":"Camera circles the subject"}
		],
		"duration":5,
		"generate_audio":false,
		"seed":0
	}`, string(encoded))
}

func TestBuildDoubaoV3RequestPreservesNativeContentForSLSUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    "doubao-seedance-2-0",
		Prompt:   "Animate this image",
		Duration: 5,
		Metadata: map[string]any{
			"model": "doubao-seedance-2-0",
			"content": []any{
				map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": "https://example.com/reference.png"},
				},
				map[string]any{"type": "text", "text": "Animate this image"},
			},
			"duration":       float64(5),
			"generate_audio": false,
			"seed":           float64(0),
		},
	})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		UpstreamModelName: "mapped-seedance-model",
		IsModelMapped:     true,
	}}

	body, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"model":"mapped-seedance-model",
		"content":[
			{"type":"image_url","image_url":{"url":"https://example.com/reference.png"}},
			{"type":"text","text":"Animate this image"}
		],
		"duration":5,
		"generate_audio":false,
		"seed":0
	}`, string(encoded))
}

func TestSubmitResponseAcceptsSLSTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"task_id":"task_upstream","status":"QUEUED"}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}

	upstreamID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, resp, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "task_upstream", upstreamID)
	assert.NotContains(t, string(taskData), "task_upstream")
	assert.Contains(t, string(taskData), "task_public")
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestParseSLSWrappedTaskResult(t *testing.T) {
	tests := []struct {
		name string
		body string
		want *relaycommon.TaskInfo
	}{
		{
			name: "completed",
			body: `{"code":"success","message":"","data":{"task_id":"task_upstream","status":"SUCCESS","progress":"100%","result_url":"https://example.com/video.mp4","total_tokens":108900}}`,
			want: &relaycommon.TaskInfo{TaskID: "task_upstream", Status: string(model.TaskStatusSuccess), Progress: "100%", Url: "https://example.com/video.mp4", TotalTokens: 108900},
		},
		{
			name: "failed",
			body: `{"code":"success","message":"","data":{"task_id":"task_upstream","status":"FAILURE","progress":"100%","fail_reason":"content rejected"}}`,
			want: &relaycommon.TaskInfo{TaskID: "task_upstream", Status: string(model.TaskStatusFailure), Progress: "100%", Reason: "content rejected"},
		},
		{
			name: "failed reason in result url",
			body: `{"code":"success","message":"","data":{"task_id":"task_upstream","status":"FAILURE","progress":"100%","result_url":"The parameter ratio specified in the request is not valid. Request id: 0217882828433820e23c04e8b740c94d8512f0597aa5fa6ac2318 (code=InvalidParameter.TaskTypeConstraint)"}}`,
			want: &relaycommon.TaskInfo{TaskID: "task_upstream", Status: string(model.TaskStatusFailure), Progress: "100%", Reason: "The parameter ratio specified in the request is not valid. Request id: 0217882828433820e23c04e8b740c94d8512f0597aa5fa6ac2318 (code=InvalidParameter.TaskTypeConstraint)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&TaskAdaptor{}).ParseTaskResult([]byte(tt.body))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseNestedNewAPIWrapperMergesProviderUsage(t *testing.T) {
	body := []byte(`{
		"code":"success",
		"data":{
			"task_id":"task_gateway",
			"status":"SUCCESS",
			"progress":"100%",
			"data":{
				"code":"success",
				"data":{
					"task_id":"task_provider",
					"status":"SUCCESS",
					"result_url":"https://example.com/nested.mp4",
					"total_tokens":87300
				}
			}
		}
	}`)

	result, err := (&TaskAdaptor{}).ParseWrappedTaskResult(body)

	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "https://example.com/nested.mp4", result.Url)
	assert.Equal(t, 87300, result.TotalTokens)
}

func TestConvertToNativeVideoMapsNestedSLSResultToDoubaoV3(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		CreatedAt: 100,
		UpdatedAt: 200,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0",
		},
		Data: json.RawMessage(`{
			"code":"success",
			"data":{
				"task_id":"task_public",
				"status":"SUCCESS",
				"result_url":"https://example.com/output.mp4",
				"last_frame_url":"https://example.com/last-frame.png",
				"duration":5,
				"resolution":"720p",
				"ratio":"16:9",
				"total_tokens":108900
			}
		}`),
	}

	encoded, err := (&TaskAdaptor{}).ConvertToNativeVideo(task)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"task_public",
		"model":"doubao-seedance-2-0",
		"status":"succeeded",
		"created_at":100,
		"updated_at":200,
		"content":{
			"video_url":"https://example.com/output.mp4",
			"last_frame_url":"https://example.com/last-frame.png"
		},
		"duration":5,
		"resolution":"720p",
		"ratio":"16:9",
		"usage":{"completion_tokens":108900,"total_tokens":108900}
	}`, string(encoded))
}

func TestConvertToNativeVideoReturnsDoubaoV3FailureShape(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_failed",
		Status:     model.TaskStatusFailure,
		CreatedAt:  100,
		UpdatedAt:  200,
		FailReason: "content rejected",
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0"},
	}

	encoded, err := (&TaskAdaptor{}).ConvertToNativeVideo(task)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"task_failed",
		"model":"doubao-seedance-2-0",
		"status":"failed",
		"created_at":100,
		"updated_at":200,
		"error":{"code":"","message":"content rejected"}
	}`, string(encoded))
}

func TestSanitizeTaskDataReplacesNestedUpstreamTaskID(t *testing.T) {
	body := []byte(`{
		"code":"success",
		"data":{
			"task_id":"task_gateway",
			"status":"SUCCESS",
			"data":{"code":"success","data":{"task_id":"task_provider"}}
		}
	}`)

	sanitized := (&TaskAdaptor{}).SanitizeTaskData(body, "task_public")

	assert.NotContains(t, string(sanitized), "task_gateway")
	assert.NotContains(t, string(sanitized), "task_provider")
	assert.Contains(t, string(sanitized), "task_public")
}

func TestFetchTaskUsesSLSPathAndBearerToken(t *testing.T) {
	type requestData struct {
		path string
		auth string
	}
	requests := make(chan requestData, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- requestData{path: r.URL.EscapedPath(), auth: r.Header.Get("Authorization")}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)

	resp, err := (&TaskAdaptor{}).FetchTask(server.URL, "sk-test", map[string]any{"task_id": "task/123"}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	request := <-requests
	assert.Equal(t, "/v1/video/generations/task%2F123", request.path)
	assert.Equal(t, "Bearer sk-test", request.auth)
}

func TestSeedanceSLSModelList(t *testing.T) {
	assert.Equal(t, []string{
		"doubao-seedance-1-0-pro-250528",
		"doubao-seedance-1-0-lite-t2v",
		"doubao-seedance-1-0-lite-i2v",
		"doubao-seedance-1-5-pro-251215",
		"doubao-seedance-2-0-260128",
		"doubao-seedance-2-0-fast-260128",
		"doubao-seedance-2-0-mini-260615",
		"doubao-seedance-2-5-260628",
	}, (&TaskAdaptor{}).GetModelList())
}

func TestSeedanceSLSModelsHaveDefaultBillingRatios(t *testing.T) {
	aliases := map[string]string{
		"doubao-seedance-2-0":      "doubao-seedance-2-0-260128",
		"doubao-seedance-2-0-fast": "doubao-seedance-2-0-fast-260128",
		"doubao-seedance-2-0-mini": "doubao-seedance-2-0-mini-260615",
	}
	defaults := ratio_setting.GetDefaultModelRatioMap()
	for alias, versioned := range aliases {
		assert.Equal(t, defaults[versioned], defaults[alias], alias)
		assert.Positive(t, defaults[alias], alias)
	}
	assert.InDelta(t, 4.794520547945205, defaults["doubao-seedance-2-5-260628"], 1e-12)
}
