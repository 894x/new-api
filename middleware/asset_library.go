package middleware

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const localAssetURIPrefix = "asset://asset-na-"

// AssetLibraryRouting restricts a video request to channels that have an
// upstream mapping for every local asset URI in its JSON payload.
func AssetLibraryRouting() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		if !strings.HasPrefix(c.GetHeader("Content-Type"), gin.MIMEJSON) {
			c.Next()
			return
		}

		var request any
		if err := common.UnmarshalBodyReusable(c, &request); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
			return
		}
		assetIds := localAssetIds(request)
		if hasInvalidAssetURI(request) {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid asset URI; use an account asset ID")
			return
		}
		if len(assetIds) == 0 {
			c.Next()
			return
		}

		allowedChannelIds, err := model.GetAssetReplicaChannelIntersection(
			common.GetContextKeyInt(c, constant.ContextKeyUserId),
			assetIds,
		)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			abortWithOpenAiMessage(c, http.StatusForbidden, "Asset does not exist or does not belong to the current account", types.ErrorCodeAccessDenied)
			return
		}
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to resolve asset replicas")
			return
		}
		if len(allowedChannelIds) == 0 {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "No channel has replicas for all referenced assets")
			return
		}

		common.SetContextKey(c, constant.ContextKeyAssetAllowedChannelIds, allowedChannelIds)
		c.Next()
	}
}

func localAssetIds(value any) []string {
	unique := make(map[string]struct{})
	collectLocalAssetIds(value, unique)
	assetIds := make([]string, 0, len(unique))
	for assetId := range unique {
		assetIds = append(assetIds, assetId)
	}
	sort.Strings(assetIds)
	return assetIds
}

func hasInvalidAssetURI(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, item := range typed {
			if hasInvalidAssetURI(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if hasInvalidAssetURI(item) {
				return true
			}
		}
	case string:
		if !hasAssetURIScheme(typed) {
			return false
		}
		return !strings.HasPrefix(typed, "asset://") || !isLocalAssetId(strings.TrimPrefix(typed, "asset://"))
	}
	return false
}

func hasAssetURIScheme(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= len("asset://") && strings.EqualFold(value[:len("asset://")], "asset://")
}

func collectLocalAssetIds(value any, assetIds map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		for _, item := range typed {
			collectLocalAssetIds(item, assetIds)
		}
	case []any:
		for _, item := range typed {
			collectLocalAssetIds(item, assetIds)
		}
	case string:
		if !strings.HasPrefix(typed, localAssetURIPrefix) || strings.ContainsAny(strings.TrimPrefix(typed, "asset://"), "?#/ \t\r\n") {
			return
		}
		assetId := strings.TrimPrefix(typed, "asset://")
		if isLocalAssetId(assetId) {
			assetIds[assetId] = struct{}{}
		}
	}
}

func isLocalAssetId(assetId string) bool {
	const prefix = "asset-na-"
	if !strings.HasPrefix(assetId, prefix) || len(assetId) != len(prefix)+32 {
		return false
	}
	for _, char := range assetId[len(prefix):] {
		if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' {
			continue
		}
		return false
	}
	return true
}
