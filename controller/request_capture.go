package controller

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetUserRequestCapture(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if _, err := model.GetUserById(id, false); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	policy, err := model.GetRequestCapturePolicy(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"enabled": policy.Enabled, "available": service.RequestCaptureStorageAvailable()}})
}

func UpdateUserRequestCapture(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 1024), &input) != nil || input.Enabled == nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if !canManageTargetRole(c.GetInt("role"), user.Role) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if *input.Enabled && !service.RequestCaptureStorageAvailable() {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	policy := model.RequestCapturePolicy{UserID: id, Enabled: *input.Enabled}
	if err := model.SetRequestCapturePolicy(c.Request.Context(), policy); err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "request capture policy: user_id="+strconv.Itoa(id)+", enabled="+strconv.FormatBool(policy.Enabled))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": policy})
}

func GetRequestCapture(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	requestID := c.Param("request_id")
	if len(requestID) == 0 || len(requestID) > 64 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	record, err := model.FindRequestCapture(c.Request.Context(), requestID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "not_captured"}})
		return
	}
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if record.ExpiresAt <= time.Now().Unix() {
		record.Status = "expired"
	}
	data := gin.H{"status": record.Status, "record": record}
	if c.Query("body") == "true" && (record.Status == "ready" || record.Status == "partial") {
		parts, err := service.ReadRequestCapture(record)
		if err != nil {
			data["status"] = "unavailable"
		} else {
			data["parts"] = parts
			model.RecordLog(c.GetInt("id"), model.LogTypeManage, "view request capture: request_id="+record.RequestID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
