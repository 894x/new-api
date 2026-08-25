package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAssetLibraryServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.ChannelAssetConfig{},
		&model.UserAssetGroup{},
		&model.UserAsset{},
		&model.UserAssetGroupReplica{},
		&model.UserAssetReplica{},
	))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func TestReplicateSeedanceSLSAssetDefersGroupAndWaitsUntilActive(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.RequestURI())
		assert.Equal(t, "Bearer sls-key", request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodPost:
			var body map[string]any
			require.NoError(t, common.DecodeJson(request.Body, &body))
			assert.Equal(t, "characters", body["group_name"])
			_, _ = writer.Write([]byte(`{
				"success": true,
				"data": {
					"logical_id": "lass_abc123",
					"logical_group_id": "lasg_xyz789",
					"status": "Processing"
				}
			}`))
		case http.MethodGet:
			_, _ = writer.Write([]byte(`{
				"success": true,
				"data": {
					"logical_id": "lass_abc123",
					"logical_group_id": "lasg_xyz789",
					"status": "Active",
					"asset_type": "Image",
					"name": "character"
				}
			}`))
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	channel := &model.Channel{Id: 17, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 17, Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}).Error)
	group := &model.UserAssetGroup{
		Id: "group-na-0123456789abcdef0123456789abcdef", UserId: 7, Name: "characters", GroupType: "AIGC",
	}
	asset := &model.UserAsset{
		Id: "asset-na-0123456789abcdef0123456789abcdef", UserId: 7, GroupId: group.Id,
		SourceURL: "https://example.com/character.png", AssetType: "Image", Name: "character",
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(asset).Error)

	report, err := ReplicateAsset(t.Context(), asset)
	require.NoError(t, err)
	assert.Empty(t, report.Errors)
	assert.Equal(t, 1, report.Summary.Processing)
	assert.Zero(t, report.Summary.Ready)
	assert.Equal(t, []string{"POST /v1/volcengine/assets"}, requests)

	groupReplica, err := model.GetUserAssetGroupReplica(group.Id, 17)
	require.NoError(t, err)
	assert.Equal(t, "lasg_xyz789", groupReplica.UpstreamGroupId)
	assert.Equal(t, model.AssetReplicaStateReady, groupReplica.State)
	assetReplica, err := model.GetUserAssetReplica(asset.Id, 17)
	require.NoError(t, err)
	assert.Equal(t, "lass_abc123", assetReplica.UpstreamAssetId)
	assert.Equal(t, model.AssetReplicaStateProcessing, assetReplica.State)

	_, err = RewriteAssetReferences(7, 17, map[string]any{"image_url": "asset://" + asset.Id})
	require.ErrorContains(t, err, "unavailable")
	details, err := RefreshAssetLibraryAsset(t.Context(), asset.Id)
	require.NoError(t, err)
	assert.Equal(t, "Active", details.Status)
	assetReplica, err = model.GetUserAssetReplica(asset.Id, 17)
	require.NoError(t, err)
	assert.Equal(t, model.AssetReplicaStateReady, assetReplica.State)
	rewritten, err := RewriteAssetReferences(7, 17, map[string]any{"image_url": "asset://" + asset.Id})
	require.NoError(t, err)
	assert.Equal(t, "asset://lass_abc123", rewritten["image_url"])
	assert.Equal(t, []string{
		"POST /v1/volcengine/assets",
		"GET /v1/volcengine/assets/lass_abc123",
	}, requests)
}

func TestSeedanceSLSReplicaUpdateIsLocalAndDeleteUsesRESTEndpoint(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.RequestURI())
		assert.Equal(t, http.MethodDelete, request.Method)
		assert.Equal(t, "/v1/volcengine/assets/lass_abc123", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"message":"Asset deleted successfully"}`))
	}))
	t.Cleanup(server.Close)

	require.NoError(t, db.Create(&model.Channel{Id: 17, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 17, Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}).Error)
	group := &model.UserAssetGroup{Id: "group-na-0123456789abcdef0123456789abcdef", UserId: 7, Name: "renamed"}
	asset := &model.UserAsset{
		Id: "asset-na-0123456789abcdef0123456789abcdef", UserId: 7, GroupId: group.Id,
		AssetType: "Image", Name: "renamed",
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Create(&model.UserAssetGroupReplica{
		GroupId: group.Id, ChannelId: 17, UpstreamGroupId: "lasg_xyz789", State: model.AssetReplicaStateReady,
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: asset.Id, ChannelId: 17, UpstreamAssetId: "lass_abc123",
		State: model.AssetReplicaStateProcessing, UpstreamStatus: "Processing",
	}).Error)

	groupReport, err := UpdateAssetGroupReplicas(t.Context(), group)
	require.NoError(t, err)
	assert.Empty(t, groupReport.Errors)
	assetReport, err := UpdateAssetReplicas(t.Context(), asset)
	require.NoError(t, err)
	assert.Empty(t, assetReport.Errors)
	assetReplica, err := model.GetUserAssetReplica(asset.Id, 17)
	require.NoError(t, err)
	assert.Equal(t, model.AssetReplicaStateProcessing, assetReplica.State)
	assert.Empty(t, requests)

	deleteErrors, err := DeleteAssetReplicas(t.Context(), asset.Id)
	require.NoError(t, err)
	assert.Empty(t, deleteErrors)
	groupDeleteErrors, err := DeleteAssetGroupReplicas(t.Context(), group.Id)
	require.NoError(t, err)
	assert.Empty(t, groupDeleteErrors)
	assert.Equal(t, []string{"DELETE /v1/volcengine/assets/lass_abc123"}, requests)
}

func TestRefreshAssetLibraryAssetRefreshesEveryEnabledReplica(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	var actionCalls atomic.Int32
	actionServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		actionCalls.Add(1)
		assert.Equal(t, "GetAsset", request.URL.Query().Get("Action"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"ResponseMetadata": {},
			"Result": {"Id":"Asset-action","Status":"Active"}
		}`))
	}))
	t.Cleanup(actionServer.Close)
	var slsCalls atomic.Int32
	slsServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		slsCalls.Add(1)
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/v1/volcengine/assets/lass_sls", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"success": true,
			"data": {"logical_id":"lass_sls","status":"Active"}
		}`))
	}))
	t.Cleanup(slsServer.Close)

	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 11, Type: constant.ChannelTypeDoubaoVideo, Key: "action-key", Name: "Action"},
		{Id: 12, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS"},
	}).Error)
	require.NoError(t, db.Create(&[]model.ChannelAssetConfig{
		{ChannelId: 11, Enabled: true, BaseURL: actionServer.URL, AuthType: AssetLibraryAuthBearer, APIKey: "action-key"},
		{ChannelId: 12, Enabled: true, BaseURL: slsServer.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key"},
	}).Error)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&[]model.UserAssetReplica{
		{AssetId: assetId, ChannelId: 11, UpstreamAssetId: "Asset-action", State: model.AssetReplicaStateProcessing},
		{AssetId: assetId, ChannelId: 12, UpstreamAssetId: "lass_sls", State: model.AssetReplicaStateProcessing},
	}).Error)

	details, err := RefreshAssetLibraryAsset(t.Context(), assetId)
	require.NoError(t, err)
	assert.Equal(t, "Active", details.Status)
	assert.Equal(t, int32(1), actionCalls.Load())
	assert.Equal(t, int32(1), slsCalls.Load())
	for _, channelId := range []int{11, 12} {
		replica, err := model.GetUserAssetReplica(assetId, channelId)
		require.NoError(t, err)
		assert.Equal(t, model.AssetReplicaStateReady, replica.State)
	}
}

func TestGetAssetReplicaDetailsListsEveryConfiguredChannel(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 31, Name: "Primary"},
		{Id: 32, Name: "Disabled backup"},
	}).Error)
	require.NoError(t, db.Create(&[]model.ChannelAssetConfig{
		{ChannelId: 31, Enabled: true, Backend: AssetLibraryBackendAction, BaseURL: "https://primary.example.com"},
		{ChannelId: 32, Enabled: false, Backend: AssetLibraryBackendOpenAPI, BaseURL: "https://backup.example.com"},
	}).Error)
	assetId := "asset-na-11111111111111111111111111111111"
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetId, ChannelId: 31, UpstreamAssetId: "Asset-primary",
		State: model.AssetReplicaStateReady, UpstreamStatus: "Active",
	}).Error)

	details, err := GetAssetReplicaDetails(assetId)

	require.NoError(t, err)
	require.Len(t, details.Replicas, 2)
	assert.Equal(t, "Primary", details.Replicas[0].ChannelName)
	assert.True(t, details.Replicas[0].Enabled)
	assert.Equal(t, model.AssetReplicaStateReady, details.Replicas[0].State)
	assert.Equal(t, "Asset-primary", details.Replicas[0].UpstreamAssetId)
	assert.Equal(t, "Disabled backup", details.Replicas[1].ChannelName)
	assert.False(t, details.Replicas[1].Enabled)
	assert.Equal(t, AssetReplicaStateNotSynced, details.Replicas[1].State)
	assert.Empty(t, details.Replicas[1].UpstreamAssetId)
	require.NotNil(t, details.Summary)
	assert.Equal(t, 1, details.Summary.Total)
	assert.Equal(t, 1, details.Summary.Ready)
}

func TestSyncAssetReplicasRefreshesExistingUpstreamReplica(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"success": true,
			"data": {"logical_id":"lass_existing","status":"Active"}
		}`))
	}))
	t.Cleanup(server.Close)

	require.NoError(t, db.Create(&model.Channel{Id: 33, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 33, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS,
		BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}).Error)
	group := &model.UserAssetGroup{Id: "group-na-22222222222222222222222222222222", UserId: 7, Name: "characters"}
	asset := &model.UserAsset{
		Id: "asset-na-22222222222222222222222222222222", UserId: 7, GroupId: group.Id,
		SourceURL: "https://example.com/character.png", AssetType: "Image", Name: "character",
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: asset.Id, ChannelId: 33, UpstreamAssetId: "lass_existing",
		State: model.AssetReplicaStateProcessing, UpstreamStatus: "Processing",
	}).Error)

	report, err := SyncAssetReplicas(t.Context(), asset, []int{33})

	require.NoError(t, err)
	assert.Empty(t, report.Errors)
	assert.Equal(t, []string{"GET /v1/volcengine/assets/lass_existing"}, requests)
	replica, err := model.GetUserAssetReplica(asset.Id, 33)
	require.NoError(t, err)
	assert.Equal(t, model.AssetReplicaStateReady, replica.State)
	assert.Equal(t, "Active", replica.UpstreamStatus)
}

func TestManualSyncRequiresAtLeastOneEnabledChannel(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	group := &model.UserAssetGroup{Id: "group-na-99999999999999999999999999999999", UserId: 7, Name: "characters"}
	asset := &model.UserAsset{
		Id: "asset-na-99999999999999999999999999999999", UserId: 7, GroupId: group.Id,
		SourceURL: "https://example.com/character.png", AssetType: "Image", Name: "character",
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 99, Enabled: false, Backend: AssetLibraryBackendAction,
		BaseURL: "https://assets.example.com", AuthType: AssetLibraryAuthAKSK,
		AccessKey: "ak", SecretKey: "sk",
	}).Error)

	_, assetErr := SyncAssetReplicas(t.Context(), asset, nil)
	_, groupErr := SyncAssetGroupReplicas(t.Context(), group, nil)

	require.ErrorContains(t, assetErr, "no enabled asset library channels")
	require.ErrorContains(t, groupErr, "no enabled asset library channels")
}

func TestSyncAssetGroupReplicasSynchronizesEveryChildAsset(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := requestCount.Add(1)
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v1/volcengine/assets", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(fmt.Sprintf(`{
			"success": true,
			"data": {
				"logical_id":"lass_child_%d",
				"logical_group_id":"lasg_group",
				"status":"Active"
			}
		}`, count)))
	}))
	t.Cleanup(server.Close)

	require.NoError(t, db.Create(&model.Channel{Id: 34, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 34, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS,
		BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}).Error)
	group := &model.UserAssetGroup{Id: "group-na-33333333333333333333333333333333", UserId: 7, Name: "characters"}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(&[]model.UserAsset{
		{Id: "asset-na-33333333333333333333333333333331", UserId: 7, GroupId: group.Id, SourceURL: "https://example.com/a.png", AssetType: "Image", Name: "a"},
		{Id: "asset-na-33333333333333333333333333333332", UserId: 7, GroupId: group.Id, SourceURL: "https://example.com/b.png", AssetType: "Image", Name: "b"},
	}).Error)

	report, err := SyncAssetGroupReplicas(t.Context(), group, []int{34})

	require.NoError(t, err)
	assert.Empty(t, report.Errors)
	assert.Equal(t, int32(2), requestCount.Load())
	groupReplica, err := model.GetUserAssetGroupReplica(group.Id, 34)
	require.NoError(t, err)
	assert.Equal(t, model.AssetReplicaStateReady, groupReplica.State)
	assert.Equal(t, "lasg_group", groupReplica.UpstreamGroupId)
	for _, assetId := range []string{
		"asset-na-33333333333333333333333333333331",
		"asset-na-33333333333333333333333333333332",
	} {
		replica, err := model.GetUserAssetReplica(assetId, 34)
		require.NoError(t, err)
		assert.Equal(t, model.AssetReplicaStateReady, replica.State)
		assert.NotEmpty(t, replica.UpstreamAssetId)
	}
}

func TestSyncAssetGroupReplicasContinuesAfterOneChildFails(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count := requestCount.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		if count <= 2 {
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"success":false,"message":"temporary failure"}`))
			return
		}
		_, _ = writer.Write([]byte(`{
			"success": true,
			"data": {
				"logical_id":"lass_second",
				"logical_group_id":"lasg_group",
				"status":"Active"
			}
		}`))
	}))
	t.Cleanup(server.Close)

	require.NoError(t, db.Create(&model.Channel{Id: 35, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 35, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS,
		BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key",
	}).Error)
	group := &model.UserAssetGroup{Id: "group-na-66666666666666666666666666666666", UserId: 7, Name: "characters"}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(&[]model.UserAsset{
		{Id: "asset-na-66666666666666666666666666666661", UserId: 7, GroupId: group.Id, SourceURL: "https://example.com/a.png", AssetType: "Image", Name: "a", CreatedTime: 1},
		{Id: "asset-na-66666666666666666666666666666662", UserId: 7, GroupId: group.Id, SourceURL: "https://example.com/b.png", AssetType: "Image", Name: "b", CreatedTime: 2},
		{Id: "asset-na-66666666666666666666666666666663", UserId: 7, GroupId: group.Id, SourceURL: "https://example.com/c.png", AssetType: "Image", Name: "c", CreatedTime: 3},
	}).Error)

	report, err := SyncAssetGroupReplicas(t.Context(), group, []int{35})

	require.NoError(t, err)
	require.Len(t, report.Errors, 2)
	assert.Equal(t, "asset-na-66666666666666666666666666666661", report.Errors[0].AssetId)
	assert.Equal(t, "asset-na-66666666666666666666666666666662", report.Errors[1].AssetId)
	assert.Equal(t, int32(3), requestCount.Load())
	thirdReplica, err := model.GetUserAssetReplica("asset-na-66666666666666666666666666666663", 35)
	require.NoError(t, err)
	assert.Equal(t, model.AssetReplicaStateReady, thirdReplica.State)
}

func TestRefreshAssetLibraryAssetPrefersActiveReplicaWithPreviewURL(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	slsServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/v1/volcengine/assets/lass_sls", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"success": true,
			"data": {"logical_id":"lass_sls","status":"Active"}
		}`))
	}))
	t.Cleanup(slsServer.Close)
	openAPIServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/openapi/v1/asset/get", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"code": 0,
			"message": "",
			"data": {"asset": {
				"id": 5001,
				"group_id": 101,
				"asset_type": 1,
				"url": "https://example.com/preview.png",
				"sync_status": 2
			}}
		}`))
	}))
	t.Cleanup(openAPIServer.Close)

	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 11, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS"},
		{Id: 12, Type: constant.ChannelTypeOpenAI, Key: "openapi-key", Name: "OpenAPI"},
	}).Error)
	require.NoError(t, db.Create(&[]model.ChannelAssetConfig{
		{ChannelId: 11, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS, BaseURL: slsServer.URL, AuthType: AssetLibraryAuthBearer, APIKey: "sls-key"},
		{ChannelId: 12, Enabled: true, Backend: AssetLibraryBackendOpenAPI, BaseURL: openAPIServer.URL, AuthType: AssetLibraryAuthBearer, APIKey: "openapi-key"},
	}).Error)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&[]model.UserAssetReplica{
		{AssetId: assetId, ChannelId: 11, UpstreamAssetId: "lass_sls", State: model.AssetReplicaStateProcessing},
		{AssetId: assetId, ChannelId: 12, UpstreamAssetId: "5001", State: model.AssetReplicaStateProcessing},
	}).Error)

	details, err := RefreshAssetLibraryAsset(t.Context(), assetId)

	require.NoError(t, err)
	assert.Equal(t, "Active", details.Status)
	assert.Equal(t, "https://example.com/preview.png", details.URL)
}

func TestRewriteAssetReferencesRewritesNestedPayloadWithoutMutation(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.Create(&model.Channel{
		Id: 11, Type: constant.ChannelTypeDoubaoVideo, Key: "action-key", Name: "Action",
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, Backend: AssetLibraryBackendAction,
		BaseURL: DefaultAssetLibraryBaseURL, AuthType: AssetLibraryAuthBearer, APIKey: "action-key",
	}).Error)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	otherAssetId := "asset-na-abcdef0123456789abcdef0123456789"
	require.NoError(t, db.Create(&model.UserAsset{
		Id: assetId, UserId: 7, GroupId: "group-na-0123456789abcdef0123456789abcdef",
		SourceURL: "https://example.com/a.png", AssetType: "Image", ProjectName: "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetId, ChannelId: 11, UpstreamAssetId: "Asset-upstream", State: model.AssetReplicaStateReady,
	}).Error)
	payload := map[string]any{
		"content": []any{
			map[string]any{"image_url": "asset://" + assetId},
			"asset://" + otherAssetId,
		},
		"similar": "prefix asset://" + assetId,
	}

	_, err := RewriteAssetReferences(7, 11, payload)
	require.ErrorContains(t, err, otherAssetId)
	assert.Equal(t, "asset://"+assetId, payload["content"].([]any)[0].(map[string]any)["image_url"])

	payload["content"] = payload["content"].([]any)[:1]
	rewritten, err := RewriteAssetReferences(7, 11, payload)
	require.NoError(t, err)
	assert.Equal(t, "asset://Asset-upstream", rewritten["content"].([]any)[0].(map[string]any)["image_url"])
	assert.Equal(t, "asset://"+assetId, payload["content"].([]any)[0].(map[string]any)["image_url"])
	assert.Equal(t, payload["similar"], rewritten["similar"])
}

func TestRewriteAssetReferencesRejectsAnotherUsersReplica(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.UserAsset{
		Id: assetId, UserId: 8, GroupId: "group-na-0123456789abcdef0123456789abcdef",
		SourceURL: "https://example.com/a.png", AssetType: "Image", ProjectName: "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetId, ChannelId: 11, UpstreamAssetId: "Asset-upstream", State: model.AssetReplicaStateReady,
	}).Error)

	_, err := RewriteAssetReferences(7, 11, map[string]any{"url": "asset://" + assetId})
	require.ErrorContains(t, err, "unavailable")
}

func TestRewriteAssetReferencesRejectsRawUpstreamAssetURI(t *testing.T) {
	setupAssetLibraryServiceTestDB(t)

	for _, uri := range []string{
		"asset://Asset-upstream-owned-by-another-user",
		"ASSET://Asset-upstream-owned-by-another-user",
		" asset://Asset-upstream-owned-by-another-user ",
	} {
		_, err := RewriteAssetReferences(7, 11, map[string]any{"url": uri})
		require.ErrorContains(t, err, "use an account asset ID")
	}
}

func TestSaveAssetLibraryChannelConfigClearsReplicasOnlyWhenIdentityChanges(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	existing := &model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, BaseURL: "https://assets.example.com", AuthType: AssetLibraryAuthBearer,
		APIKey: "old-key", Region: DefaultAssetLibraryRegion, ProjectName: "project-a",
	}
	require.NoError(t, db.Create(existing).Error)
	require.NoError(t, db.Create(&model.UserAssetGroupReplica{
		GroupId: "group-na-0123456789abcdef0123456789abcdef", ChannelId: 11,
		UpstreamGroupId: "group-upstream", State: model.AssetReplicaStateReady,
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: "asset-na-0123456789abcdef0123456789abcdef", ChannelId: 11,
		UpstreamAssetId: "asset-upstream", State: model.AssetReplicaStateReady,
	}).Error)

	changed, err := SaveAssetLibraryChannelConfig(&model.ChannelAssetConfig{
		ChannelId: 11, Enabled: false, BaseURL: existing.BaseURL, AuthType: existing.AuthType,
		APIKey: existing.APIKey, Region: existing.Region, ProjectName: existing.ProjectName,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"enabled"}, changed)
	count, err := model.CountChannelAssetReplicas(11)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	changed, err = SaveAssetLibraryChannelConfig(&model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, BaseURL: existing.BaseURL, AuthType: existing.AuthType,
		APIKey: "new-key", Region: existing.Region, ProjectName: existing.ProjectName,
	})
	require.NoError(t, err)
	assert.Contains(t, changed, "credentials")
	count, err = model.CountChannelAssetReplicas(11)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestSaveAssetLibraryChannelConfigClearsReplicasWhenBackendChanges(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	existing := &model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, Backend: AssetLibraryBackendAction,
		BaseURL: "https://assets.example.com", AuthType: AssetLibraryAuthBearer,
		APIKey: "key", Region: DefaultAssetLibraryRegion, ProjectName: "project-a",
	}
	require.NoError(t, db.Create(existing).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: "asset-na-0123456789abcdef0123456789abcdef", ChannelId: 11,
		UpstreamAssetId: "asset-upstream", State: model.AssetReplicaStateReady,
	}).Error)

	changed, err := SaveAssetLibraryChannelConfig(&model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, Backend: AssetLibraryBackendOpenAPI,
		BaseURL: existing.BaseURL, AuthType: existing.AuthType,
		APIKey: existing.APIKey, Region: existing.Region, ProjectName: existing.ProjectName,
	})
	require.NoError(t, err)
	assert.Contains(t, changed, "backend")
	count, err := model.CountChannelAssetReplicas(11)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestAssetReplicationSummaryCountsReplicaStateInsteadOfAssignedID(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.Create(&[]model.ChannelAssetConfig{
		{ChannelId: 11, Enabled: true},
		{ChannelId: 12, Enabled: true},
		{ChannelId: 13, Enabled: true},
	}).Error)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&[]model.UserAssetReplica{
		{AssetId: assetId, ChannelId: 11, UpstreamAssetId: "asset-processing", State: model.AssetReplicaStateProcessing},
		{AssetId: assetId, ChannelId: 12, UpstreamAssetId: "asset-ready", State: model.AssetReplicaStateReady},
		{AssetId: assetId, ChannelId: 13, UpstreamAssetId: "asset-failed", State: model.AssetReplicaStateFailed},
	}).Error)

	summary, err := GetAssetReplicationSummary(assetId)
	require.NoError(t, err)
	assert.Equal(t, 3, summary.Total)
	assert.Equal(t, 1, summary.Ready)
	assert.Equal(t, 1, summary.Processing)
	assert.Equal(t, 1, summary.Failed)
}

func TestAssetGroupReplicationSummaryCountsReplicaStateInsteadOfAssignedID(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.Create(&[]model.ChannelAssetConfig{
		{ChannelId: 11, Enabled: true},
		{ChannelId: 12, Enabled: true},
	}).Error)
	groupId := "group-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&[]model.UserAssetGroupReplica{
		{GroupId: groupId, ChannelId: 11, UpstreamGroupId: "group-processing", State: model.AssetReplicaStateProcessing},
		{GroupId: groupId, ChannelId: 12, UpstreamGroupId: "group-ready", State: model.AssetReplicaStateReady},
	}).Error)

	summary, err := GetAssetGroupReplicationSummary(groupId)
	require.NoError(t, err)
	assert.Equal(t, 2, summary.Total)
	assert.Equal(t, 1, summary.Ready)
	assert.Equal(t, 1, summary.Processing)
}

func TestReplicateAssetGroupSerializesConcurrentCreatesPerChannel(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	var upstreamCalls atomic.Int32
	firstCallEntered := make(chan struct{})
	releaseFirstCall := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "CreateAssetGroup", request.URL.Query().Get("Action"))
		if upstreamCalls.Add(1) == 1 {
			close(firstCallEntered)
			<-releaseFirstCall
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"group-upstream"}}`))
	}))
	t.Cleanup(server.Close)
	require.NoError(t, db.Create(&model.Channel{Id: 9, Type: constant.ChannelTypeDoubaoVideo, Key: "test-key", Name: "Action"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 9, Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer,
		APIKey: "test-key", Region: DefaultAssetLibraryRegion, ProjectName: "channel-project",
	}).Error)
	group := &model.UserAssetGroup{
		Id: "group-na-0123456789abcdef0123456789abcdef", UserId: 7, Name: "character",
		GroupType: "AIGC", ProjectName: "logical-project",
	}
	require.NoError(t, db.Create(group).Error)

	var waitGroup sync.WaitGroup
	errorsByCall := make(chan error, 2)
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		_, err := ReplicateAssetGroup(t.Context(), group)
		errorsByCall <- err
	}()
	<-firstCallEntered
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		_, err := ReplicateAssetGroup(t.Context(), group)
		errorsByCall <- err
	}()
	close(releaseFirstCall)
	waitGroup.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), upstreamCalls.Load())
	replica, err := model.GetUserAssetGroupReplica(group.Id, 9)
	require.NoError(t, err)
	assert.Equal(t, "group-upstream", replica.UpstreamGroupId)
}
