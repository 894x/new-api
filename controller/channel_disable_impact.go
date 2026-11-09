package controller

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func GetChannelDisableImpact(c *gin.Context) {
	getChannelStatusImpact(c, common.ChannelStatusManuallyDisabled)
}

func GetChannelEnableImpact(c *gin.Context) {
	getChannelStatusImpact(c, common.ChannelStatusEnabled)
}

func getChannelStatusImpact(c *gin.Context, targetStatus int) {
	// Channel operators must also be allowed to read customer routing details.
	if !authz.Can(c.GetInt("id"), c.GetInt("role"), authz.UserGroupConfigRead) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "No permission to view customer routing"})
		return
	}
	var request struct {
		IDs []int   `json:"ids"`
		Tag *string `json:"tag"`
	}
	if common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10), &request) != nil ||
		(request.Tag == nil && len(request.IDs) == 0) || (request.Tag != nil && len(request.IDs) > 0) || len(request.IDs) > 1000 ||
		request.Tag != nil && (strings.TrimSpace(*request.Tag) == "" || len(*request.Tag) > 255) ||
		slices.ContainsFunc(request.IDs, func(id int) bool { return id <= 0 }) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel selection"})
		return
	}
	slices.Sort(request.IDs)
	request.IDs = slices.Compact(request.IDs)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	impact, err := service.GetChannelStatusImpact(ctx, request.IDs, request.Tag, c.GetInt("role"), targetStatus)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, impact)
}
