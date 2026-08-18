package controller

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type channelAssetLibraryConfigRequest struct {
	Enabled     bool   `json:"enabled"`
	Backend     string `json:"backend"`
	BaseURL     string `json:"base_url"`
	AuthType    string `json:"auth_type"`
	AccessKey   string `json:"access_key"`
	SecretKey   string `json:"secret_key"`
	APIKey      string `json:"api_key"`
	Region      string `json:"region"`
	ProjectName string `json:"project_name"`
}

type channelAssetLibraryConfigResponse struct {
	ChannelId    int    `json:"channel_id"`
	Enabled      bool   `json:"enabled"`
	Backend      string `json:"backend"`
	BaseURL      string `json:"base_url"`
	AuthType     string `json:"auth_type"`
	Region       string `json:"region"`
	ProjectName  string `json:"project_name"`
	HasAccessKey bool   `json:"has_access_key"`
	HasSecretKey bool   `json:"has_secret_key"`
	HasAPIKey    bool   `json:"has_api_key"`
	ReplicaCount int64  `json:"replica_count"`
}

type channelAssetLibrarySyncResponse struct {
	ChannelId     int `json:"channel_id"`
	GroupsCreated int `json:"groups_created"`
	GroupsSkipped int `json:"groups_skipped"`
	GroupsFailed  int `json:"groups_failed"`
	AssetsCreated int `json:"assets_created"`
	AssetsSkipped int `json:"assets_skipped"`
	AssetsFailed  int `json:"assets_failed"`
}

func GetChannelAssetLibraryConfig(c *gin.Context) {
	channelId, ok := parseAssetLibraryChannelId(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(channelId, false)
	if err != nil {
		writeChannelAssetLibraryModelError(c, err)
		return
	}
	config, err := model.GetChannelAssetConfig(channelId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		backend, baseURL, authType := channelAssetLibraryDefaults(channel)
		common.ApiSuccess(c, channelAssetLibraryConfigResponse{
			ChannelId:   channelId,
			Backend:     backend,
			BaseURL:     baseURL,
			AuthType:    authType,
			Region:      service.DefaultAssetLibraryRegion,
			ProjectName: service.DefaultAssetLibraryProject,
		})
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(config.Backend) == "" {
		config.Backend = service.DefaultAssetLibraryBackend(channel.Type)
	}
	replicaCount, err := model.CountChannelAssetReplicas(channelId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildChannelAssetLibraryConfigResponse(config, replicaCount))
}

func UpdateChannelAssetLibraryConfig(c *gin.Context) {
	channelId, ok := parseAssetLibraryChannelId(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(channelId, false)
	if err != nil {
		writeChannelAssetLibraryModelError(c, err)
		return
	}
	var request channelAssetLibraryConfigRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body: " + err.Error()})
		return
	}
	existing, err := model.GetChannelAssetConfig(channelId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiError(c, err)
		return
	}
	config, err := normalizeChannelAssetLibraryConfig(channel, &request, existing)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	changedFields, err := service.SaveAssetLibraryChannelConfig(config)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	replicaCount, err := model.CountChannelAssetReplicas(channelId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.asset_library.update", map[string]interface{}{
		"id":             channelId,
		"changed_fields": strings.Join(changedFields, ","),
	})
	if config.Enabled {
		common.RelayCtxGo(context.Background(), func() {
			if _, err := service.SyncAssetLibraryChannel(context.Background(), channelId); err != nil {
				common.SysError("failed to sync asset library channel " + strconv.Itoa(channelId) + ": " + err.Error())
			}
		})
	}
	response := buildChannelAssetLibraryConfigResponse(config, replicaCount)
	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"message":        "",
		"data":           response,
		"changed_fields": changedFields,
	})
}

func DeleteChannelAssetLibraryConfig(c *gin.Context) {
	channelId, ok := parseAssetLibraryChannelId(c)
	if !ok {
		return
	}
	if _, err := model.GetChannelById(channelId, false); err != nil {
		writeChannelAssetLibraryModelError(c, err)
		return
	}
	if err := service.DeleteAssetLibraryChannelConfig(channelId); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.asset_library.delete", map[string]interface{}{"id": channelId})
	common.ApiSuccess(c, nil)
}

func SyncChannelAssetLibrary(c *gin.Context) {
	channelId, ok := parseAssetLibraryChannelId(c)
	if !ok {
		return
	}
	if _, err := model.GetChannelById(channelId, false); err != nil {
		writeChannelAssetLibraryModelError(c, err)
		return
	}
	result, err := service.SyncAssetLibraryChannel(c.Request.Context(), channelId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.asset_library.sync", map[string]interface{}{
		"id":             channelId,
		"groups_created": result.GroupsCreated,
		"groups_failed":  result.GroupsFailed,
		"assets_created": result.AssetsCreated,
		"assets_failed":  result.AssetsFailed,
	})
	common.ApiSuccess(c, channelAssetLibrarySyncResponse{
		ChannelId:     result.ChannelId,
		GroupsCreated: result.GroupsCreated,
		GroupsSkipped: result.GroupsSkipped,
		GroupsFailed:  result.GroupsFailed,
		AssetsCreated: result.AssetsCreated,
		AssetsSkipped: result.AssetsSkipped,
		AssetsFailed:  result.AssetsFailed,
	})
}

func parseAssetLibraryChannelId(c *gin.Context) (int, bool) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return 0, false
	}
	return channelId, true
}

func writeChannelAssetLibraryModelError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "channel not found"})
		return
	}
	common.ApiError(c, err)
}

func normalizeChannelAssetLibraryConfig(channel *model.Channel, request *channelAssetLibraryConfigRequest, existing *model.ChannelAssetConfig) (*model.ChannelAssetConfig, error) {
	if channel == nil {
		return nil, errors.New("channel is required")
	}
	defaultBackend, _, _ := channelAssetLibraryDefaults(channel)
	backend := strings.TrimSpace(request.Backend)
	if backend == "" {
		backend = defaultBackend
	}
	if !service.IsSupportedAssetLibraryBackend(backend) {
		return nil, errors.New("asset library backend must be volcengine, seedance_sls, or openapi")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if baseURL == "" {
		baseURL = service.DefaultAssetLibraryBackendBaseURL(backend)
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.User != nil {
		return nil, errors.New("asset library base_url must be a valid http or https URL without embedded credentials")
	}
	authType := strings.ToLower(strings.TrimSpace(request.AuthType))
	if authType == "" {
		authType = service.DefaultAssetLibraryBackendAuthType(backend)
	}
	if authType != service.AssetLibraryAuthAKSK && authType != service.AssetLibraryAuthBearer {
		return nil, errors.New("asset library auth_type must be aksk or bearer")
	}
	if service.AssetLibraryBackendRequiresBearer(backend) && authType != service.AssetLibraryAuthBearer {
		return nil, errors.New("selected asset library backend requires Bearer authentication")
	}
	region := strings.TrimSpace(request.Region)
	if region == "" {
		region = service.DefaultAssetLibraryRegion
	}
	projectName := strings.TrimSpace(request.ProjectName)
	if projectName == "" {
		projectName = service.DefaultAssetLibraryProject
	}
	if len(region) > 64 {
		return nil, errors.New("asset library region must be 64 characters or fewer")
	}
	if len(projectName) > 128 {
		return nil, errors.New("asset library project_name must be 128 characters or fewer")
	}
	config := &model.ChannelAssetConfig{
		ChannelId:   channel.Id,
		Enabled:     request.Enabled,
		Backend:     backend,
		BaseURL:     baseURL,
		AuthType:    authType,
		AccessKey:   strings.TrimSpace(request.AccessKey),
		SecretKey:   strings.TrimSpace(request.SecretKey),
		APIKey:      strings.TrimSpace(request.APIKey),
		Region:      region,
		ProjectName: projectName,
	}
	if existing != nil {
		// Blank credentials mean "keep the stored value" only while the
		// destination is unchanged. Reusing a stored credential after changing
		// the Base URL would send that secret to a different upstream.
		existingBackend := strings.TrimSpace(existing.Backend)
		if existingBackend == "" {
			existingBackend = service.DefaultAssetLibraryBackend(channel.Type)
		}
		sameDestination := backend == existingBackend &&
			baseURL == strings.TrimRight(strings.TrimSpace(existing.BaseURL), "/") &&
			authType == existing.AuthType
		preserveCredentials := sameDestination
		if preserveCredentials && config.AccessKey == "" {
			config.AccessKey = existing.AccessKey
		}
		if preserveCredentials && config.SecretKey == "" {
			config.SecretKey = existing.SecretKey
		}
		if preserveCredentials && config.APIKey == "" {
			config.APIKey = existing.APIKey
		}
		config.CreatedTime = existing.CreatedTime
	}
	if authType == service.AssetLibraryAuthAKSK {
		config.APIKey = ""
		if config.Enabled && (config.AccessKey == "" || config.SecretKey == "") {
			return nil, errors.New("access_key and secret_key are required when AK/SK asset library is enabled")
		}
	} else {
		config.AccessKey = ""
		config.SecretKey = ""
		if config.Enabled && config.APIKey == "" {
			return nil, errors.New("api_key is required when Bearer asset library is enabled")
		}
	}
	return config, nil
}

func channelAssetLibraryDefaults(channel *model.Channel) (string, string, string) {
	channelType := 0
	if channel != nil {
		channelType = channel.Type
	}
	backend := service.DefaultAssetLibraryBackend(channelType)
	return backend, service.DefaultAssetLibraryBackendBaseURL(backend), service.DefaultAssetLibraryBackendAuthType(backend)
}

func buildChannelAssetLibraryConfigResponse(config *model.ChannelAssetConfig, replicaCount int64) channelAssetLibraryConfigResponse {
	return channelAssetLibraryConfigResponse{
		ChannelId:    config.ChannelId,
		Enabled:      config.Enabled,
		Backend:      config.Backend,
		BaseURL:      config.BaseURL,
		AuthType:     config.AuthType,
		Region:       config.Region,
		ProjectName:  config.ProjectName,
		HasAccessKey: config.AccessKey != "",
		HasSecretKey: config.SecretKey != "",
		HasAPIKey:    config.APIKey != "",
		ReplicaCount: replicaCount,
	}
}
