package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetUserCacheHitPolicy(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 || len(c.Query("model")) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid cache hit policy query"})
		return
	}
	if _, err := model.GetUserById(userID, false); err != nil {
		common.ApiError(c, err)
		return
	}
	policies, revision, err := model.GetUserCacheHitPolicies(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	daily, statsErr := service.GetUserCacheHitDailyUsage(userID, c.Query("model"))
	statsError := ""
	if statsErr != nil {
		statsError = "Daily statistics unavailable; cache billing uses real usage when Redis is unavailable"
	}
	common.ApiSuccess(c, gin.H{"policies": policies, "revision": revision, "daily": daily, "statistics_error": statsError})
}

func PutUserCacheHitPolicy(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		Model    string `json:"model"`
		Revision string `json:"revision"`
		Enabled  *bool  `json:"enabled"`
		MinBPS   *int   `json:"min_bps"`
		MaxBPS   *int   `json:"max_bps"`
	}
	if err != nil || userID <= 0 || common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10), &request) != nil || request.Enabled == nil || request.MinBPS == nil || request.MaxBPS == nil || request.Revision == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid cache hit policy"})
		return
	}
	policy := model.UserCacheHitPolicy{Enabled: *request.Enabled, MinBPS: *request.MinBPS, MaxBPS: *request.MaxBPS}
	if request.Model == "" || request.Model != strings.TrimSpace(request.Model) || len(request.Model) > 255 || policy.Validate() != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model or target range"})
		return
	}
	if _, err := model.GetUserById(userID, false); err != nil {
		common.ApiError(c, err)
		return
	}
	if policy.Enabled {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if !common.RedisEnabled || common.RDB == nil || common.RDB.Ping(ctx).Err() != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Redis is required to enable a daily cache hit policy"})
			return
		}
	}
	if err := model.UpdateUserCacheHitPolicy(userID, request.Model, request.Revision, policy); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrCacheHitPolicyConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAuditFor(c, userID, "user.cache_hit_policy.update", map[string]any{"model": request.Model, "enabled": policy.Enabled, "min_bps": policy.MinBPS, "max_bps": policy.MaxBPS})
	common.ApiSuccess(c, nil)
}
