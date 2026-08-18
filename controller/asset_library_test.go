package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

func TestCreateAssetGroupRequiresEnabledAssetChannel(t *testing.T) {
	setupAssetLibraryControllerTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost,
		"/api/asset-library?Action=CreateAssetGroup&Version=2024-01-01",
		bytes.NewBufferString(`{"Name":"character"}`),
	)
	context.Set("id", 1)

	AssetLibraryAction(context)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	var response assetLibraryResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "AssetLibraryUnavailable", response.ResponseMetadata.Error.Code)
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

	result, err := buildAssetLibraryResult(asset, nil)
	require.NoError(t, err)
	data, err := common.Marshal(result)
	require.NoError(t, err)
	serialized := string(data)
	assert.NotContains(t, serialized, "17")
	assert.NotContains(t, serialized, "Asset-secret")
	assert.NotContains(t, serialized, "UpstreamSensitiveCode")
	assert.NotContains(t, serialized, "upstream sensitive details")
	assert.Equal(t, "AssetProcessingFailed", result.Error.Code)
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
