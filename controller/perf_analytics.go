package controller

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/gin-gonic/gin"
)

func GetPerfAnalyticsSelf(c *gin.Context) {
	getPerfAnalytics(c, c.GetInt("id"), false)
}

func GetPerfAnalyticsAdmin(c *gin.Context) {
	userId, ok := parsePositiveQueryInt(c, "user_id")
	if !ok {
		return
	}
	getPerfAnalytics(c, userId, true)
}

func GetPerfAnalyticsSelfOptions(c *gin.Context) {
	options, err := model.GetPerfAnalyticsOptions(c.GetInt("id"), false)
	if err != nil {
		perfAnalyticsInternalError(c, "failed to load self options", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": options})
}

func GetPerfAnalyticsAdminOptions(c *gin.Context) {
	userId, ok := parsePositiveQueryInt(c, "user_id")
	if !ok {
		return
	}
	options, err := model.GetPerfAnalyticsOptions(userId, true)
	if err != nil {
		perfAnalyticsInternalError(c, "failed to load admin options", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": options})
}

func getPerfAnalytics(c *gin.Context, userId int, admin bool) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" || len(modelName) > 128 {
		perfAnalyticsBadRequest(c, "model is required")
		return
	}

	startTs, endTs, ok := parsePerfAnalyticsTimeRange(c)
	if !ok {
		return
	}
	tokenId, ok := parsePositiveQueryInt(c, "token_id")
	if !ok {
		return
	}
	if admin && tokenId > 0 && userId == 0 {
		perfAnalyticsBadRequest(c, "user_id is required when token_id is specified")
		return
	}
	if tokenId > 0 {
		belongs, err := model.PerfAnalyticsTokenBelongsToUser(tokenId, userId)
		if err != nil {
			perfAnalyticsInternalError(c, "failed to validate token ownership", err)
			return
		}
		if !belongs {
			perfAnalyticsBadRequest(c, "token does not belong to user")
			return
		}
	}

	result, err := perfmetrics.QueryAnalytics(perfmetrics.AnalyticsQueryParams{
		Model: modelName, UserId: userId, TokenId: tokenId, StartTs: startTs, EndTs: endTs,
	})
	if err != nil {
		perfAnalyticsInternalError(c, "failed to query analytics", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func parsePerfAnalyticsTimeRange(c *gin.Context) (int64, int64, bool) {
	endTs := time.Now().Unix()
	startTs := endTs - 24*60*60
	var err error
	if rawStart := c.Query("start_timestamp"); rawStart != "" {
		startTs, err = strconv.ParseInt(rawStart, 10, 64)
		if err != nil {
			perfAnalyticsBadRequest(c, "invalid start_timestamp")
			return 0, 0, false
		}
	}
	if rawEnd := c.Query("end_timestamp"); rawEnd != "" {
		endTs, err = strconv.ParseInt(rawEnd, 10, 64)
		if err != nil {
			perfAnalyticsBadRequest(c, "invalid end_timestamp")
			return 0, 0, false
		}
	}
	if startTs <= 0 || endTs <= startTs || endTs-startTs > 30*24*60*60 {
		perfAnalyticsBadRequest(c, "time range must be between 1 second and 30 days")
		return 0, 0, false
	}
	return startTs, endTs, true
}

func parsePositiveQueryInt(c *gin.Context, name string) (int, bool) {
	rawValue := c.Query(name)
	if rawValue == "" {
		return 0, true
	}
	value, err := strconv.Atoi(rawValue)
	if err != nil || value <= 0 {
		perfAnalyticsBadRequest(c, "invalid "+name)
		return 0, false
	}
	return value, true
}

func perfAnalyticsBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": message})
}

func perfAnalyticsInternalError(c *gin.Context, operation string, err error) {
	logger.LogError(c, operation+": "+err.Error())
	c.JSON(http.StatusInternalServerError, gin.H{
		"success": false,
		"message": "Failed to load performance data",
	})
}
