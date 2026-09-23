package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

func GetAssetStorageUsage(c *gin.Context) {
	writeAssetStorageUsage(c, c.GetInt("id"), false)
}

func writeAssetStorageUsage(c *gin.Context, userID int, canEdit bool) {
	account, err := model.GetAssetStorageAccount(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	quota, err := account.QuotaBytes()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	remaining := quota - account.UsedBytes
	if remaining < 0 {
		remaining = 0
	}
	config, configErr := system_setting.LoadAssetStorageConfig()
	common.ApiSuccess(c, gin.H{"enabled": configErr == nil && config.Enabled, "quota_mb": quota / system_setting.AssetQuotaMB, "quota_override_mb": account.QuotaOverrideMB, "default_quota_mb": system_setting.GetAssetStorageSetting().DefaultQuotaMB, "used_bytes": account.UsedBytes, "remaining_bytes": remaining, "bytes_per_mb": system_setting.AssetQuotaMB, "can_edit": canEdit})
}

func GetAdminAssetStorageUsage(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	writeAssetStorageUsage(c, userID, canManageTargetRole(c.GetInt("role"), user.Role))
}

func UpdateAdminAssetStorageQuota(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if !canManageTargetRole(c.GetInt("role"), user.Role) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	var request struct {
		QuotaMB json.RawMessage `json:"quota_mb"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || len(request.QuotaMB) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "quota_mb must be null or an integer between 0 and 1000000 MB"})
		return
	}
	var quotaMB *int64
	if err := common.Unmarshal(request.QuotaMB, &quotaMB); err != nil || (quotaMB != nil && (*quotaMB < 0 || *quotaMB > system_setting.MaxAssetQuotaMB)) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "quota_mb must be null or an integer between 0 and 1000000 MB"})
		return
	}
	if err := model.SetAssetStorageQuotaOverride(userID, quotaMB); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), "Updated user asset storage quota", c.ClientIP(), "asset_library.storage.quota.update", map[string]interface{}{"user_id": userID, "quota_mb": quotaMB}, auditOperatorInfo(c), nil, c)
	markAuditLogged(c)
	writeAssetStorageUsage(c, userID, true)
}

func GetAssetLibraryContent(c *gin.Context) {
	userID := c.GetInt("id")
	if target := c.Param("user_id"); target != "" {
		if !model.IsAdmin(userID) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		var err error
		userID, err = strconv.Atoi(target)
		if err != nil || userID <= 0 {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
	}
	asset, err := model.GetUserAsset(userID, c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	response, err := service.ReadAssetContent(c.Request.Context(), asset, c.GetHeader("Range"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	for _, key := range []string{"Content-Type", "Content-Range", "Accept-Ranges", "ETag"} {
		if value := response.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	if response.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	c.Status(response.StatusCode)
	if c.Request.Method != http.MethodHead {
		_, _ = io.Copy(c.Writer, response.Body)
	}
}

// UploadAssetLibraryFile uses the same verified custody path as URL imports.
func UploadAssetLibraryFile(c *gin.Context) {
	config, err := system_setting.LoadAssetStorageConfig()
	if err != nil || !config.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Platform asset storage is not configured"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 201*1024*1024)
	if err := c.Request.ParseMultipartForm(1024 * 1024); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	group, err := model.GetUserAssetGroup(c.GetInt("id"), c.PostForm("GroupId"))
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	asset := &model.UserAsset{Id: "asset-na-" + common.GetUUID(), UserId: group.UserId, GroupId: group.Id, ProjectName: group.ProjectName, Name: strings.TrimSpace(c.PostForm("Name")), AssetType: c.PostForm("AssetType")}
	if len([]rune(asset.Name)) > 64 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	input, err := file.Open()
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	defer input.Close()
	if err := service.ImportAssetFile(c.Request.Context(), asset, input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := service.CreateAssetLibraryRecord(c.Request.Context(), asset); err != nil {
		writeAssetLibraryInternalError(c, "UploadAsset", err)
		return
	}
	recordAssetLibraryAudit(c, asset.UserId, "asset_library.asset.create", map[string]interface{}{"id": asset.Id, "asset_type": asset.AssetType, "group_id": asset.GroupId})
	report, err := service.ReplicateAsset(c.Request.Context(), asset)
	if err != nil {
		writeAssetLibraryInternalError(c, "UploadAsset", err)
		return
	}
	result := assetLibraryMutationResult{Id: asset.Id}
	if model.IsAdmin(asset.UserId) {
		result.Replication = report.Summary
	}
	writeAssetLibrarySuccess(c, "UploadAsset", result)
}
