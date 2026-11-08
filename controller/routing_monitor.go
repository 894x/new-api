package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Target authorization is supplied by the existing user-group configuration
// middleware on the user route. The channel route always returns base scope.
func GetRoutingMonitor(c *gin.Context) {
	userID := 0
	if rawID := c.Param("id"); rawID != "" {
		var err error
		userID, err = strconv.Atoi(rawID)
		if err != nil || userID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user id"})
			return
		}
	}
	if c.Query("user_id") != "" || c.Query("scope") != "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "use the user routing monitor endpoint to select a user"})
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, service.GetRoutingMonitor(c.Request.Context(), userID))
}

// The table contains only aggregate timings, shared across pools and users.
func GetChatEarlyChunkGrid(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("channel_id"))
	name := strings.TrimSpace(c.Query("model"))
	probe := int64(-1)
	if raw := c.Query("probe_ms"); raw != "" {
		probe, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil || id <= 0 || name == "" || len(name) > 256 || probe < -1 || probe > 30000 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid prediction grid parameters"})
		return
	}
	// Credentials participate in the internal generation fingerprint only.
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "channel not found"})
		return
	}
	grid, err := service.GetChatEarlyChunkGrid(c.Request.Context(), name, channel, probe)
	if err != nil {
		common.SysError("early chunk grid read failed: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "prediction statistics unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, grid)
}
