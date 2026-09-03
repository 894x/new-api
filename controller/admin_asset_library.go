package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminAssetLibrarySyncRequest struct {
	ChannelIds []int `json:"channel_ids,omitempty"`
}

// AdminAssetLibraryAction exposes read-only asset library queries for a
// specific user. Mutations remain scoped to the owner's regular endpoint.
func AdminAssetLibraryAction(c *gin.Context) {
	action := strings.TrimSpace(c.Query("Action"))
	version := strings.TrimSpace(c.Query("Version"))
	if version != service.AssetLibraryVersion {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.Version", "Version must be "+service.AssetLibraryVersion, nil)
		return
	}
	adminId := c.GetInt("id")
	if adminId <= 0 {
		writeAssetLibraryError(c, action, http.StatusUnauthorized, "Unauthorized", "user identity is missing", nil)
		return
	}
	if !model.IsAdmin(adminId) {
		writeAssetLibraryError(c, action, http.StatusForbidden, "AccessDenied", "administrator access is required", nil)
		return
	}
	targetUserId, err := strconv.Atoi(strings.TrimSpace(c.Param("user_id")))
	if err != nil || targetUserId <= 0 {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.UserId", "UserId must be a positive integer", nil)
		return
	}
	if _, err = model.GetUserById(targetUserId, false); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeAssetLibraryError(c, action, http.StatusNotFound, "NotFound.UserId", "user not found", nil)
			return
		}
		writeAssetLibraryInternalError(c, action, err)
		return
	}

	switch action {
	case "ListAssetGroups":
		listAssetLibraryGroups(c, targetUserId, true)
	case "ListAssets":
		listAssetLibraryAssets(c, targetUserId, true)
	case "GetAssetGroup":
		getAssetLibraryGroup(c, targetUserId, true)
	case "GetAsset":
		var request dto.GetAssetRequest
		if !decodeAssetLibraryRequest(c, "GetAsset", &request) {
			return
		}
		asset, err := model.GetUserAsset(targetUserId, strings.TrimSpace(request.Id))
		if err != nil {
			writeAssetLibraryLookupError(c, "GetAsset", "NotFound.AssetId", "asset not found", err)
			return
		}
		result, err := buildAssetLibraryResult(asset, nil, true)
		if err != nil {
			writeAssetLibraryInternalError(c, "GetAsset", err)
			return
		}
		writeAssetLibrarySuccess(c, "GetAsset", result)
	default:
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.Action", "admin asset library access is read-only", nil)
	}
}

func GetAdminAssetReplicaDetails(c *gin.Context) {
	asset, ok := getOwnedAdminAsset(c)
	if !ok {
		return
	}
	upstreamDetails, _ := service.RefreshAdminAssetLibraryAsset(c.Request.Context(), asset.Id)
	details, err := service.GetAssetReplicaDetails(asset.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	assetResult, err := buildAssetLibraryResult(asset, upstreamDetails, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"asset":    assetResult,
		"summary":  details.Summary,
		"replicas": details.Replicas,
	})
}

func SyncAdminAssetReplicas(c *gin.Context) {
	asset, ok := getOwnedAdminAsset(c)
	if !ok {
		return
	}
	var request adminAssetLibrarySyncRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body: " + err.Error()})
		return
	}
	report, err := service.SyncAssetReplicas(c.Request.Context(), asset, request.ChannelIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "asset_library.asset.sync", map[string]interface{}{
		"id":          asset.Id,
		"channel_ids": request.ChannelIds,
		"error_count": len(report.Errors),
	})
	common.ApiSuccess(c, report)
}

func GetAdminAssetGroupReplicaDetails(c *gin.Context) {
	group, ok := getOwnedAdminAssetGroup(c)
	if !ok {
		return
	}
	details, err := service.GetAssetGroupReplicaDetails(group.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, details)
}

func SyncAdminAssetGroupReplicas(c *gin.Context) {
	group, ok := getOwnedAdminAssetGroup(c)
	if !ok {
		return
	}
	var request adminAssetLibrarySyncRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body: " + err.Error()})
		return
	}
	report, err := service.SyncAssetGroupReplicas(c.Request.Context(), group, request.ChannelIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "asset_library.group.sync", map[string]interface{}{
		"id":          group.Id,
		"channel_ids": request.ChannelIds,
		"error_count": len(report.Errors),
	})
	common.ApiSuccess(c, report)
}

func getOwnedAdminAsset(c *gin.Context) (*model.UserAsset, bool) {
	assetId := strings.TrimSpace(c.Param("id"))
	if assetId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "asset id is required"})
		return nil, false
	}
	asset, err := model.GetUserAsset(c.GetInt("id"), assetId)
	if err != nil {
		writeAdminAssetLibraryModelError(c, err, "asset not found")
		return nil, false
	}
	return asset, true
}

func getOwnedAdminAssetGroup(c *gin.Context) (*model.UserAssetGroup, bool) {
	groupId := strings.TrimSpace(c.Param("id"))
	if groupId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "asset group id is required"})
		return nil, false
	}
	group, err := model.GetUserAssetGroup(c.GetInt("id"), groupId)
	if err != nil {
		writeAdminAssetLibraryModelError(c, err, "asset group not found")
		return nil, false
	}
	return group, true
}

func writeAdminAssetLibraryModelError(c *gin.Context, err error, notFoundMessage string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": notFoundMessage})
		return
	}
	common.ApiError(c, err)
}
