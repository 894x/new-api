package seedance_sls

import (
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
	require.NoError(t, db.AutoMigrate(&model.UserAsset{}, &model.UserAssetReplica{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	const assetID = "asset-na-0123456789abcdef0123456789abcdef"
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
		"doubao-seedance-2-0",
		"doubao-seedance-2-0-fast",
		"doubao-seedance-2-0-mini",
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
}
