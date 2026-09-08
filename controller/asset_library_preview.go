package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAssetLibraryImagePreview(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	userID := c.GetInt("id")
	if userID <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
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
	asset, err := model.GetUserAsset(userID, strings.TrimSpace(c.Param("id")))
	if err != nil {
		writeAssetLibraryLookupError(c, "PreviewAsset", "NotFound.AssetId", "asset not found", err)
		return
	}
	if asset.AssetType != "Image" {
		c.AbortWithStatus(http.StatusUnsupportedMediaType)
		return
	}
	data, contentType, err := service.LoadAssetLibraryImagePreview(c.Request.Context(), asset.SourceURL)
	if err != nil {
		writeAssetLibraryError(c, "PreviewAsset", http.StatusBadGateway, "AssetPreviewUnavailable", "Failed to load asset preview.", nil)
		return
	}
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Data(http.StatusOK, contentType, data)
}
