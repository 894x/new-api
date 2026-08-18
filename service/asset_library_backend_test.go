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

func TestSeedanceSLSAssetBackendCreatesFirstAssetWithGroupName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v1/volcengine/assets", request.URL.Path)
		assert.Empty(t, request.URL.RawQuery)
		assert.Equal(t, "Bearer sls-key", request.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, common.DecodeJson(request.Body, &body))
		assert.Equal(t, map[string]any{
			"source_url": "https://example.com/character.png",
			"asset_type": "Image",
			"name":       "character",
			"group_name": "characters",
		}, body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"success": true,
			"message": "Asset created successfully, processing",
			"data": {
				"logical_id": "lass_abc123",
				"logical_group_id": "lasg_xyz789",
				"group_id": "group_volc123",
				"status": "Processing"
			}
		}`))
	}))
	t.Cleanup(server.Close)

	backend := assetLibraryBackendForChannelType(constant.ChannelTypeSeedanceSLS)
	result, err := backend.CreateAsset(t.Context(), &model.ChannelAssetConfig{
		Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}, &model.UserAssetGroup{Name: "characters"}, &model.UserAssetGroupReplica{}, &model.UserAsset{
		SourceURL: "https://example.com/character.png", AssetType: "Image", Name: "character",
	})
	require.NoError(t, err)
	assert.Equal(t, "lass_abc123", result.AssetID)
	assert.Equal(t, "lasg_xyz789", result.GroupID)
	assert.Equal(t, "Processing", result.Status)
}

func TestSeedanceSLSAssetBackendReusesLogicalGroupID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		require.NoError(t, common.DecodeJson(request.Body, &body))
		assert.Equal(t, "lasg_existing", body["group_id"])
		assert.NotContains(t, body, "group_name")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"success": true,
			"data": {
				"logical_id": "lass_second",
				"logical_group_id": "lasg_existing",
				"status": "Active"
			}
		}`))
	}))
	t.Cleanup(server.Close)

	backend := assetLibraryBackendForChannelType(constant.ChannelTypeSeedanceSLS)
	result, err := backend.CreateAsset(t.Context(), &model.ChannelAssetConfig{
		Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}, &model.UserAssetGroup{Name: "characters"}, &model.UserAssetGroupReplica{
		UpstreamGroupId: "lasg_existing",
	}, &model.UserAsset{SourceURL: "https://example.com/second.mp4", AssetType: "Video"})
	require.NoError(t, err)
	assert.Equal(t, "lass_second", result.AssetID)
	assert.Equal(t, "lasg_existing", result.GroupID)
	assert.Equal(t, "Active", result.Status)
}

func TestSeedanceSLSAssetBackendGetsAndDeletesLogicalAsset(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		assert.Equal(t, "/v1/volcengine/assets/lass_abc123", request.URL.Path)
		assert.Equal(t, "Bearer sls-key", request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_, _ = writer.Write([]byte(`{
				"success": true,
				"data": {
					"logical_id": "lass_abc123",
					"logical_group_id": "lasg_xyz789",
					"name": "character",
					"asset_type": "Image",
					"status": "Active"
				}
			}`))
			return
		}
		_, _ = writer.Write([]byte(`{"success":true,"message":"Asset deleted successfully"}`))
	}))
	t.Cleanup(server.Close)

	backend := assetLibraryBackendForChannelType(constant.ChannelTypeSeedanceSLS)
	config := &model.ChannelAssetConfig{
		Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}
	details, err := backend.GetAsset(t.Context(), config, "lass_abc123")
	require.NoError(t, err)
	assert.Equal(t, "lass_abc123", details.Id)
	assert.Equal(t, "lasg_xyz789", details.GroupId)
	assert.Equal(t, "character", details.Name)
	assert.Equal(t, "Image", details.AssetType)
	assert.Equal(t, "Active", details.Status)
	require.NoError(t, backend.DeleteAsset(t.Context(), config, "lass_abc123"))
	assert.Equal(t, []string{http.MethodGet, http.MethodDelete}, methods)
}

func TestSeedanceSLSAssetBackendRejectsUnsuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"success":false,"message":"invalid asset"}`))
	}))
	t.Cleanup(server.Close)

	backend := assetLibraryBackendForChannelType(constant.ChannelTypeSeedanceSLS)
	_, err := backend.GetAsset(t.Context(), &model.ChannelAssetConfig{
		Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}, "lass_missing")
	var upstreamErr *AssetLibraryUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	assert.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	assert.Equal(t, "invalid asset", upstreamErr.Message)
}
