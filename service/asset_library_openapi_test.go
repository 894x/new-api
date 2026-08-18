package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIAssetBackendCreatesGroupAndAsset(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "Bearer upstream-key", request.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, common.DecodeJson(request.Body, &body))
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/openapi/v1/asset/group/create":
			assert.Equal(t, map[string]any{
				"group_name":  "characters",
				"description": "digital human assets",
				"group_type":  float64(1),
			}, body)
			_, _ = writer.Write([]byte(`{"code":0,"message":"","data":{"id":101},"trace_id":"trace-group"}`))
		case "/openapi/v1/asset/create":
			assert.Equal(t, map[string]any{
				"group_id":   float64(101),
				"url":        "https://example.com/clip.mp4",
				"asset_type": float64(2),
				"asset_name": "clip",
			}, body)
			_, _ = writer.Write([]byte(`{"code":0,"message":"","data":{"id":5001},"trace_id":"trace-asset"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	backend := openAPIAssetLibraryBackend{}
	config := &model.ChannelAssetConfig{
		Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "upstream-key",
	}
	group := &model.UserAssetGroup{Name: "characters", Description: "digital human assets"}
	groupResult, err := backend.CreateGroup(t.Context(), config, group)
	require.NoError(t, err)
	assert.Equal(t, "101", groupResult.GroupID)
	assert.False(t, groupResult.Deferred)

	assetResult, err := backend.CreateAsset(t.Context(), config, group, &model.UserAssetGroupReplica{
		UpstreamGroupId: groupResult.GroupID,
	}, &model.UserAsset{
		SourceURL: "https://example.com/clip.mp4", AssetType: "Video", Name: "clip",
	})
	require.NoError(t, err)
	assert.Equal(t, "5001", assetResult.AssetID)
	assert.Equal(t, "101", assetResult.GroupID)
	assert.Equal(t, "Pending", assetResult.Status)
	assert.Equal(t, []string{
		"/openapi/v1/asset/group/create",
		"/openapi/v1/asset/create",
	}, paths)
}

func TestOpenAPIAssetBackendUpdatesGetsAndDeletesAsset(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		assert.Equal(t, http.MethodPost, request.Method)
		var body map[string]any
		require.NoError(t, common.DecodeJson(request.Body, &body))
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/openapi/v1/asset/group/update":
			assert.Equal(t, float64(101), body["id"])
			assert.Equal(t, "renamed group", body["group_name"])
			_, _ = writer.Write([]byte(`{"code":0,"message":"","data":{},"trace_id":"trace-1"}`))
		case "/openapi/v1/asset/update":
			assert.Equal(t, map[string]any{"id": float64(5001), "asset_name": "renamed asset"}, body)
			_, _ = writer.Write([]byte(`{"code":0,"message":"","data":{},"trace_id":"trace-2"}`))
		case "/openapi/v1/asset/get":
			assert.Equal(t, map[string]any{"id": float64(5001)}, body)
			_, _ = writer.Write([]byte(`{
				"code":0,
				"message":"",
				"data":{"asset":{
					"id":5001,
					"group_id":101,
					"asset_name":"renamed asset",
					"asset_type":2,
					"url":"https://example.com/clip.mp4",
					"asset_status":3,
					"sync_status":2,
					"sync_error":"",
					"created_at":"2026-07-01 10:10:00",
					"updated_at":"2026-07-01 10:11:00"
				}},
				"trace_id":"trace-3"
			}`))
		case "/openapi/v1/asset/delete":
			assert.Equal(t, map[string]any{"id": float64(5001)}, body)
			_, _ = writer.Write([]byte(`{"code":0,"message":"","data":{},"trace_id":"trace-4"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	backend := openAPIAssetLibraryBackend{}
	config := &model.ChannelAssetConfig{
		Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "upstream-key",
	}
	require.NoError(t, backend.UpdateGroup(t.Context(), config, &model.UserAssetGroup{
		Name: "renamed group", Description: "description",
	}, "101"))
	require.NoError(t, backend.UpdateAsset(t.Context(), config, &model.UserAsset{Name: "renamed asset"}, "5001"))
	details, err := backend.GetAsset(t.Context(), config, "5001")
	require.NoError(t, err)
	assert.Equal(t, "5001", details.Id)
	assert.Equal(t, "101", details.GroupId)
	assert.Equal(t, "renamed asset", details.Name)
	assert.Equal(t, "Video", details.AssetType)
	assert.Equal(t, "Active", details.Status)
	assert.Equal(t, "https://example.com/clip.mp4", details.URL)
	assert.Equal(t, "2026-07-01 10:10:00", details.CreateTime)
	assert.Equal(t, "2026-07-01 10:11:00", details.UpdateTime)
	require.NoError(t, backend.DeleteAsset(t.Context(), config, "5001"))
	require.NoError(t, backend.DeleteGroup(t.Context(), config, "101"))
	assert.Equal(t, []string{
		"/openapi/v1/asset/group/update",
		"/openapi/v1/asset/update",
		"/openapi/v1/asset/get",
		"/openapi/v1/asset/delete",
	}, paths)
}

func TestOpenAPIAssetBackendMapsSyncFailureAndBusinessError(t *testing.T) {
	t.Run("sync failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{
				"code":0,
				"message":"",
				"data":{"asset":{"id":5001,"group_id":101,"asset_type":1,"sync_status":3,"sync_error":"upstream rejected image"}},
				"trace_id":"trace-failed"
			}`))
		}))
		t.Cleanup(server.Close)
		backend := openAPIAssetLibraryBackend{}
		details, err := backend.GetAsset(t.Context(), &model.ChannelAssetConfig{
			Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "upstream-key",
		}, "5001")
		require.NoError(t, err)
		assert.Equal(t, "Failed", details.Status)
		require.NotNil(t, details.Error)
		assert.Equal(t, "AssetSyncFailed", details.Error.Code)
		assert.Equal(t, "upstream rejected image", details.Error.Message)
	})

	t.Run("business error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":3004,"message":"group not synced","data":{},"trace_id":"trace-error"}`))
		}))
		t.Cleanup(server.Close)
		backend := openAPIAssetLibraryBackend{}
		_, err := backend.CreateAsset(t.Context(), &model.ChannelAssetConfig{
			Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "upstream-key",
		}, &model.UserAssetGroup{}, &model.UserAssetGroupReplica{UpstreamGroupId: "101"}, &model.UserAsset{
			SourceURL: "https://example.com/image.png", AssetType: "Image",
		})
		var upstreamErr *AssetLibraryUpstreamError
		require.ErrorAs(t, err, &upstreamErr)
		assert.Equal(t, "3004", upstreamErr.Code)
		assert.Equal(t, "group not synced", upstreamErr.Message)
	})
}

func TestRewriteAssetReferencesUsesConfiguredOpenAPIFormat(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.Create(&model.Channel{
		Id: 21, Type: constant.ChannelTypeOpenAI, Key: "upstream-key", Name: "OpenAPI upstream",
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 21, Enabled: true, Backend: AssetLibraryBackendOpenAPI,
		BaseURL: "https://token.example.com", AuthType: AssetLibraryAuthBearer, APIKey: "upstream-key",
	}).Error)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.UserAsset{
		Id: assetId, UserId: 7, GroupId: "group-na-0123456789abcdef0123456789abcdef", AssetType: "Video",
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetId, ChannelId: 21, UpstreamAssetId: "5001", State: model.AssetReplicaStateReady,
	}).Error)

	rewritten, err := RewriteAssetReferences(7, 21, map[string]any{"video": "asset://" + assetId})
	require.NoError(t, err)
	assert.Equal(t, "asset:local:5001", rewritten["video"])
}

func TestOpenAPINotFoundBusinessCodesAreIdempotentForReplicaDeletion(t *testing.T) {
	assert.True(t, isAssetLibraryNotFound(&AssetLibraryUpstreamError{Code: "3001", Message: "group not found"}))
	assert.True(t, isAssetLibraryNotFound(&AssetLibraryUpstreamError{Code: "3002", Message: "asset not found"}))
}
