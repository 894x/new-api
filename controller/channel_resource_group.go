package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func CreateChannelResourceGroup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	var request struct {
		GroupName     string `json:"group_name"`
		ReuseExisting bool   `json:"reuse_existing"`
	}
	if err != nil || id <= 0 || common.DecodeJson(c.Request.Body, &request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid resource group request"})
		return
	}
	result, err := model.CreateChannelResourceGroup(id, request.GroupName, request.ReuseExisting)
	if err != nil {
		var conflict *model.ResourceGroupConflict
		if errors.As(err, &conflict) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "code": "RESOURCE_GROUP_EXISTS", "message": conflict.Error(), "data": conflict})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Channel not found"})
			return
		}
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.resource_group", map[string]any{
		"id": id, "group_name": result.GroupName, "created": result.Created,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
