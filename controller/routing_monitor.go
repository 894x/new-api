package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
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
