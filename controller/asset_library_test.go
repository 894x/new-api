package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAssetLibraryControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
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

func TestAssetLibraryActionRejectsUnsupportedVersionWithOfficialEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/asset-library?Action=ListAssets&Version=wrong", bytes.NewBufferString(`{}`))
	context.Set("id", 1)

	AssetLibraryAction(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response assetLibraryResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "ListAssets", response.ResponseMetadata.Action)
	assert.Equal(t, "InvalidParameter.Version", response.ResponseMetadata.Error.Code)
	assert.Equal(t, "ark", response.ResponseMetadata.Service)
}

func TestGetAllChannelsIncludesAssetLibraryEnabledState(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.Channel{
		{Id: 71, Name: "asset-enabled", Key: "key-71", Status: common.ChannelStatusEnabled},
		{Id: 72, Name: "asset-disabled", Key: "key-72", Status: common.ChannelStatusEnabled},
	}).Error)
	require.NoError(t, db.Create(&[]model.ChannelAssetConfig{
		{ChannelId: 71, Enabled: true},
		{ChannelId: 72, Enabled: false},
	}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel?p=1&page_size=20", nil)

	GetAllChannels(context)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Id                  int  `json:"id"`
				AssetLibraryEnabled bool `json:"asset_library_enabled"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.Items, 2)
	states := make(map[int]bool, len(response.Data.Items))
	for _, item := range response.Data.Items {
		states[item.Id] = item.AssetLibraryEnabled
	}
	assert.True(t, states[71])
	assert.False(t, states[72])
}

func TestAssetLibraryActionEnforcesAccountOwnership(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	require.NoError(t, db.Create(&model.UserAssetGroup{
		Id: "group-na-0123456789abcdef0123456789abcdef", UserId: 2, Name: "other",
		GroupType: "AIGC", ProjectName: "default",
	}).Error)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost,
		"/api/asset-library?Action=GetAssetGroup&Version=2024-01-01",
		bytes.NewBufferString(`{"Id":"group-na-0123456789abcdef0123456789abcdef"}`),
	)
	context.Set("id", 1)

	AssetLibraryAction(context)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	var response assetLibraryResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "NotFound.GroupId", response.ResponseMetadata.Error.Code)
}

func TestGetAdminAssetReplicaDetailsRefreshesEveryUpstreamBeforeReturning(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/v1/volcengine/assets/lass_admin", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"success": true,
			"data": {"logical_id":"lass_admin","status":"Active"}
		}`))
	}))
	t.Cleanup(server.Close)

	require.NoError(t, db.Create(&model.Channel{Id: 41, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 41, Enabled: false, Backend: service.AssetLibraryBackendSeedanceSLS,
		BaseURL: server.URL, AuthType: service.AssetLibraryAuthBearer, APIKey: "sls-key",
	}).Error)
	group := &model.UserAssetGroup{Id: "group-na-44444444444444444444444444444444", UserId: 7, Name: "characters"}
	asset := &model.UserAsset{
		Id: "asset-na-44444444444444444444444444444444", UserId: 7, GroupId: group.Id,
		AssetType: "Image", SourceURL: "https://example.com/admin.png",
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: asset.Id, ChannelId: 41, UpstreamAssetId: "lass_admin",
		State: model.AssetReplicaStateProcessing, UpstreamStatus: "Processing",
	}).Error)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/asset-library/admin/assets/"+asset.Id+"/replicas", nil)
	context.Params = gin.Params{{Key: "id", Value: asset.Id}}
	context.Set("id", 7)

	GetAdminAssetReplicaDetails(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Asset struct {
				URL         string                   `json:"URL"`
				Status      string                   `json:"Status"`
				Replication *dto.AssetReplicaSummary `json:"Replication"`
			} `json:"asset"`
			Replicas []struct {
				ChannelId      int    `json:"channel_id"`
				State          string `json:"state"`
				UpstreamStatus string `json:"upstream_status"`
			} `json:"replicas"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, "https://example.com/admin.png", response.Data.Asset.URL)
	assert.Equal(t, "Active", response.Data.Asset.Status)
	require.NotNil(t, response.Data.Asset.Replication)
	assert.Zero(t, response.Data.Asset.Replication.Ready)
	require.Len(t, response.Data.Replicas, 1)
	assert.Equal(t, 41, response.Data.Replicas[0].ChannelId)
	assert.Equal(t, model.AssetReplicaStateReady, response.Data.Replicas[0].State)
	assert.Equal(t, "Active", response.Data.Replicas[0].UpstreamStatus)
	assert.Equal(t, int32(1), calls.Load())
}

func TestGetAdminAssetReplicaDetailsEnforcesOwnership(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	asset := &model.UserAsset{Id: "asset-na-55555555555555555555555555555555", UserId: 8, GroupId: "group-na-55555555555555555555555555555555", AssetType: "Image"}
	require.NoError(t, db.Create(asset).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/asset-library/admin/assets/"+asset.Id+"/replicas", nil)
	context.Params = gin.Params{{Key: "id", Value: asset.Id}}
	context.Set("id", 7)

	GetAdminAssetReplicaDetails(context)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestCreateAssetGroupWithoutEnabledAssetChannel(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost,
		"/api/asset-library?Action=CreateAssetGroup&Version=2024-01-01",
		bytes.NewBufferString(`{"Name":"character"}`),
	)
	context.Set("id", 1)

	AssetLibraryAction(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Result assetLibraryMutationResult `json:"Result"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Regexp(t, `^group-na-[0-9a-f]{32}$`, response.Result.Id)
	group, err := model.GetUserAssetGroup(1, response.Result.Id)
	require.NoError(t, err)
	assert.Equal(t, "character", group.Name)
	var replicaCount int64
	require.NoError(t, db.Model(&model.UserAssetGroupReplica{}).Where("group_id = ?", group.Id).Count(&replicaCount).Error)
	assert.Zero(t, replicaCount)
}

func TestCreateAssetWithoutEnabledAssetChannel(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	group := &model.UserAssetGroup{
		Id: "group-na-0123456789abcdef0123456789abcdef", UserId: 1, Name: "character",
		GroupType: "AIGC", ProjectName: "default",
	}
	require.NoError(t, db.Create(group).Error)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost,
		"/api/asset-library?Action=CreateAsset&Version=2024-01-01",
		bytes.NewBufferString(`{"GroupId":"`+group.Id+`","URL":"https://example.com/portrait.png","AssetType":"Image","Name":"portrait"}`),
	)
	context.Set("id", 1)

	AssetLibraryAction(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Result assetLibraryMutationResult `json:"Result"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Regexp(t, `^asset-na-[0-9a-f]{32}$`, response.Result.Id)
	asset, err := model.GetUserAsset(1, response.Result.Id)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/portrait.png", asset.SourceURL)
	var replicaCount int64
	require.NoError(t, db.Model(&model.UserAssetReplica{}).Where("asset_id = ?", asset.Id).Count(&replicaCount).Error)
	assert.Zero(t, replicaCount)
}

func TestCreateAssetGroupRejectsValuesThatExceedDatabaseColumns(t *testing.T) {
	setupAssetLibraryControllerTestDB(t)
	gin.SetMode(gin.TestMode)

	for _, body := range []string{
		`{"Name":"character","GroupType":"abcdefghijklmnopqrstuvwxyz1234567"}`,
		`{"Name":"character","ProjectName":"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxy"}`,
	} {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost,
			"/api/asset-library?Action=CreateAssetGroup&Version=2024-01-01",
			bytes.NewBufferString(body),
		)
		context.Set("id", 1)

		AssetLibraryAction(context)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	}
}

func TestNormalizeChannelAssetLibraryConfigPreservesBlankCredential(t *testing.T) {
	existing := &model.ChannelAssetConfig{
		ChannelId: 3, Enabled: true, BaseURL: "https://old.example.com", AuthType: "aksk",
		AccessKey: "old-ak", SecretKey: "old-sk", Region: "cn-beijing", ProjectName: "project-a",
	}
	request := &channelAssetLibraryConfigRequest{
		Enabled: true, BaseURL: "https://old.example.com/", AuthType: "aksk",
		Region: "cn-beijing", ProjectName: "project-a",
	}

	config, err := normalizeChannelAssetLibraryConfig(&model.Channel{Id: 3}, request, existing)

	require.NoError(t, err)
	assert.Equal(t, "https://old.example.com", config.BaseURL)
	assert.Equal(t, "old-ak", config.AccessKey)
	assert.Equal(t, "old-sk", config.SecretKey)
}

func TestNormalizeChannelAssetLibraryConfigRequiresCredentialsForNewBaseURL(t *testing.T) {
	existing := &model.ChannelAssetConfig{
		ChannelId: 3, Enabled: true, BaseURL: "https://old.example.com", AuthType: "bearer",
		APIKey: "old-key", Region: "cn-beijing", ProjectName: "project-a",
	}

	_, err := normalizeChannelAssetLibraryConfig(&model.Channel{Id: 3}, &channelAssetLibraryConfigRequest{
		Enabled: true, BaseURL: "https://new.example.com", AuthType: "bearer",
		Region: "cn-beijing", ProjectName: "project-a",
	}, existing)

	require.ErrorContains(t, err, "api_key")

	config, err := normalizeChannelAssetLibraryConfig(&model.Channel{Id: 3}, &channelAssetLibraryConfigRequest{
		Enabled: true, BaseURL: "https://new.example.com", AuthType: "bearer", APIKey: "new-key",
		Region: "cn-beijing", ProjectName: "project-a",
	}, existing)
	require.NoError(t, err)
	assert.Equal(t, "new-key", config.APIKey)
}

func TestNormalizeChannelAssetLibraryConfigRequiresCredentialsWhenAuthTypeChanges(t *testing.T) {
	existing := &model.ChannelAssetConfig{
		ChannelId: 3, Enabled: true, BaseURL: "https://assets.example.com", AuthType: "bearer",
		APIKey: "old-key", Region: "cn-beijing", ProjectName: "project-a",
	}

	_, err := normalizeChannelAssetLibraryConfig(&model.Channel{Id: 3}, &channelAssetLibraryConfigRequest{
		Enabled: true, BaseURL: existing.BaseURL, AuthType: "aksk",
		Region: "cn-beijing", ProjectName: "project-a",
	}, existing)

	require.ErrorContains(t, err, "access_key and secret_key")
}

func TestNormalizeChannelAssetLibraryConfigValidatesAuthentication(t *testing.T) {
	_, err := normalizeChannelAssetLibraryConfig(&model.Channel{Id: 3}, &channelAssetLibraryConfigRequest{
		Enabled: true, BaseURL: "https://assets.example.com", AuthType: "bearer",
	}, nil)
	require.ErrorContains(t, err, "api_key")

	_, err = normalizeChannelAssetLibraryConfig(&model.Channel{Id: 3}, &channelAssetLibraryConfigRequest{
		Enabled: true, BaseURL: "ftp://assets.example.com", AuthType: "aksk",
		AccessKey: "ak", SecretKey: "sk",
	}, nil)
	require.ErrorContains(t, err, "http or https")
}

func TestNormalizeChannelAssetLibraryConfigUsesSeedanceSLSProtocolDefaults(t *testing.T) {
	channel := &model.Channel{Id: 17, Type: constant.ChannelTypeSeedanceSLS}
	config, err := normalizeChannelAssetLibraryConfig(channel, &channelAssetLibraryConfigRequest{
		Enabled: true,
		APIKey:  "sls-key",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://lm.sls.cn", config.BaseURL)
	assert.Equal(t, service.AssetLibraryBackendSeedanceSLS, config.Backend)
	assert.Equal(t, "bearer", config.AuthType)
	assert.Equal(t, "sls-key", config.APIKey)

	_, err = normalizeChannelAssetLibraryConfig(channel, &channelAssetLibraryConfigRequest{
		Enabled: true, AuthType: "aksk", AccessKey: "ak", SecretKey: "sk",
	}, nil)
	require.ErrorContains(t, err, "Bearer")
}

func TestNormalizeChannelAssetLibraryConfigSupportsBearerOpenAPIBackend(t *testing.T) {
	channel := &model.Channel{Id: 21, Type: constant.ChannelTypeOpenAI}
	config, err := normalizeChannelAssetLibraryConfig(channel, &channelAssetLibraryConfigRequest{
		Enabled: true, Backend: service.AssetLibraryBackendOpenAPI,
		BaseURL: "https://token.example.com", APIKey: "upstream-key",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, service.AssetLibraryBackendOpenAPI, config.Backend)
	assert.Equal(t, service.AssetLibraryAuthBearer, config.AuthType)

	_, err = normalizeChannelAssetLibraryConfig(channel, &channelAssetLibraryConfigRequest{
		Enabled: true, Backend: service.AssetLibraryBackendOpenAPI,
		BaseURL: "https://token.example.com", AuthType: service.AssetLibraryAuthAKSK,
		AccessKey: "ak", SecretKey: "sk",
	}, nil)
	require.ErrorContains(t, err, "Bearer")
}

func TestNormalizeChannelAssetLibraryConfigUsesChannelBaseURLForOpenAPIBackend(t *testing.T) {
	channelBaseURL := "https://assets.channel.example.com"
	channel := &model.Channel{
		Id:      21,
		Type:    constant.ChannelTypeOpenAI,
		BaseURL: &channelBaseURL,
	}
	config, err := normalizeChannelAssetLibraryConfig(channel, &channelAssetLibraryConfigRequest{
		Enabled: true,
		Backend: service.AssetLibraryBackendOpenAPI,
		APIKey:  "upstream-key",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, channelBaseURL, config.BaseURL)
}

func TestNormalizeChannelAssetLibraryConfigUsesProviderDefaultChannelBaseURLForOpenAPIBackend(t *testing.T) {
	channelBaseURL := ""
	channel := &model.Channel{
		Id:      21,
		Type:    constant.ChannelTypeOpenAI,
		BaseURL: &channelBaseURL,
	}
	config, err := normalizeChannelAssetLibraryConfig(channel, &channelAssetLibraryConfigRequest{
		Enabled: true,
		Backend: service.AssetLibraryBackendOpenAPI,
		APIKey:  "upstream-key",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, constant.ChannelBaseURLs[constant.ChannelTypeOpenAI], config.BaseURL)
}

func TestChannelAssetLibraryResponseDoesNotExposeCredentials(t *testing.T) {
	response := buildChannelAssetLibraryConfigResponse(&model.ChannelAssetConfig{
		ChannelId: 3, Enabled: true, Backend: service.AssetLibraryBackendOpenAPI,
		BaseURL: "https://assets.example.com", AuthType: "aksk",
		AccessKey: "sensitive-ak", SecretKey: "sensitive-sk", APIKey: "sensitive-api-key",
		Region: "cn-beijing", ProjectName: "default",
	}, 2)

	data, err := common.Marshal(response)
	require.NoError(t, err)
	serialized := string(data)
	assert.NotContains(t, serialized, "sensitive-ak")
	assert.NotContains(t, serialized, "sensitive-sk")
	assert.NotContains(t, serialized, "sensitive-api-key")
	assert.True(t, response.HasAccessKey)
	assert.True(t, response.HasSecretKey)
	assert.True(t, response.HasAPIKey)
	assert.Equal(t, service.AssetLibraryBackendOpenAPI, response.Backend)
}

func TestAssetLibraryResultDoesNotExposeChannelOrUpstreamError(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 17, Enabled: true, BaseURL: "https://assets.example.com", AuthType: "bearer",
		APIKey: "secret", Region: "cn-beijing", ProjectName: "default",
	}).Error)
	asset := &model.UserAsset{
		Id: assetId, UserId: 1, GroupId: "group-na-0123456789abcdef0123456789abcdef",
		Name: "portrait", SourceURL: "https://example.com/a.png", AssetType: "Image", ProjectName: "default",
	}
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetId, ChannelId: 17, State: model.AssetReplicaStateReady, UpstreamAssetId: "Asset-secret",
		UpstreamStatus: "Failed", LastErrorCode: "UpstreamSensitiveCode", LastError: "upstream sensitive details",
	}).Error)

	result, err := buildAssetLibraryResult(asset, nil, false)
	require.NoError(t, err)
	data, err := common.Marshal(result)
	require.NoError(t, err)
	serialized := string(data)
	assert.NotContains(t, serialized, `"ChannelId"`)
	assert.NotContains(t, serialized, `"channel_id"`)
	assert.NotContains(t, serialized, "Asset-secret")
	assert.NotContains(t, serialized, "UpstreamSensitiveCode")
	assert.NotContains(t, serialized, "upstream sensitive details")
	assert.Equal(t, "AssetProcessingFailed", result.Error.Code)
}

func TestAssetLibraryResultAlwaysUsesLogicalSourceURL(t *testing.T) {
	setupAssetLibraryControllerTestDB(t)
	asset := &model.UserAsset{
		Id:          "asset-na-0123456789abcdef0123456789abcdef",
		UserId:      1,
		GroupId:     "group-na-0123456789abcdef0123456789abcdef",
		Name:        "portrait",
		SourceURL:   "https://example.com/portrait.png",
		AssetType:   "Image",
		ProjectName: "default",
	}

	result, err := buildAssetLibraryResult(asset, &service.AssetLibraryAssetDetails{
		Status: "Active",
		URL:    "https://upstream.example.com/private-preview.png",
	}, false)

	require.NoError(t, err)
	assert.Equal(t, "https://example.com/portrait.png", result.URL)
}

func TestGetAssetKeepsSourcePreviewWhenUpstreamRefreshFails(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	require.NoError(t, db.Create(&model.User{
		Id: 1, Username: "common-user", Password: "password", Role: common.RoleCommonUser, AffCode: "common-aff",
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 17, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-key", Name: "Seedance SLS",
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 17, Enabled: true, Backend: service.AssetLibraryBackendSeedanceSLS,
		BaseURL: server.URL, AuthType: service.AssetLibraryAuthBearer, APIKey: "sls-key",
	}).Error)
	asset := &model.UserAsset{
		Id: "asset-na-0123456789abcdef0123456789abcdef", UserId: 1,
		GroupId: "group-na-0123456789abcdef0123456789abcdef", Name: "portrait",
		SourceURL: "https://example.com/portrait.png", AssetType: "Image", ProjectName: "default",
	}
	require.NoError(t, db.Create(asset).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: asset.Id, ChannelId: 17, UpstreamAssetId: "lass_sls", State: model.AssetReplicaStateReady,
	}).Error)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost,
		"/api/asset-library?Action=GetAsset&Version=2024-01-01",
		bytes.NewBufferString(`{"Id":"`+asset.Id+`"}`),
	)
	context.Set("id", 1)

	AssetLibraryAction(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"URL":"https://example.com/portrait.png"`)
	assert.NotContains(t, recorder.Body.String(), `"Error"`)
}

func TestAssetLibraryReplicationMetadataIsAdminOnly(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "common-user", Password: "password", Role: common.RoleCommonUser, AffCode: "common-aff"},
		{Id: 2, Username: "admin-user", Password: "password", Role: common.RoleAdminUser, AffCode: "admin-aff"},
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 17,
		Enabled:   true,
		BaseURL:   "https://assets.example.com",
		AuthType:  "bearer",
		APIKey:    "secret",
	}).Error)
	for _, fixture := range []struct {
		userId  int
		assetId string
	}{
		{userId: 1, assetId: "asset-na-0123456789abcdef0123456789abcde1"},
		{userId: 2, assetId: "asset-na-0123456789abcdef0123456789abcde2"},
	} {
		require.NoError(t, db.Create(&model.UserAsset{
			Id: fixture.assetId, UserId: fixture.userId, GroupId: "group-na-0123456789abcdef0123456789abcdef",
			Name: "portrait", SourceURL: "https://example.com/a.png", AssetType: "Image", ProjectName: "default",
		}).Error)
		require.NoError(t, db.Create(&model.UserAssetReplica{
			AssetId: fixture.assetId, ChannelId: 17, State: model.AssetReplicaStateReady,
			UpstreamAssetId: "asset-upstream", UpstreamStatus: "Active",
		}).Error)
	}

	for _, testCase := range []struct {
		name              string
		userId            int
		expectReplication bool
	}{
		{name: "common user", userId: 1, expectReplication: false},
		{name: "admin API token", userId: 2, expectReplication: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost,
				"/api/asset-library?Action=ListAssets&Version=2024-01-01",
				bytes.NewBufferString(`{}`),
			)
			context.Set("id", testCase.userId)

			AssetLibraryAction(context)

			require.Equal(t, http.StatusOK, recorder.Code)
			if testCase.expectReplication {
				assert.Contains(t, recorder.Body.String(), `"Replication"`)
			} else {
				assert.NotContains(t, recorder.Body.String(), `"Replication"`)
			}
		})
	}
}

func TestDeleteAssetGroupRequiresEmptyGroup(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	groupId := "group-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.UserAssetGroup{
		Id: groupId, UserId: 1, Name: "character", GroupType: "AIGC", ProjectName: "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserAsset{
		Id: "asset-na-0123456789abcdef0123456789abcdef", UserId: 1, GroupId: groupId,
		AssetType: "Image", SourceURL: "https://example.com/a.png", ProjectName: "default",
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost,
		"/api/asset-library?Action=DeleteAssetGroup&Version=2024-01-01",
		bytes.NewBufferString(`{"Id":"`+groupId+`"}`),
	)
	context.Set("id", 1)

	AssetLibraryAction(context)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var response assetLibraryResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "AssetGroupNotEmpty", response.ResponseMetadata.Error.Code)
}
