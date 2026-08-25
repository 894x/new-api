package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminAssetLibrarySyncRequest struct {
	ChannelIds []int `json:"channel_ids,omitempty"`
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
