package doubao

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

func TestNativeRequestRewritesLogicalAssetForCurrentChannel(t *testing.T) {
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

	assetId := "asset-na-0123456789abcdef0123456789abcdef"
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
