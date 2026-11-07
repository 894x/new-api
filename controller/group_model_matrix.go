package controller

import (
	"errors"
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func GetGroupModelMatrix(c *gin.Context) {
	config, revision, err := model.GetGroupModelAccess()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var abilities []model.Ability
	if err := model.DB.Select("group", "model").Distinct().Find(&abilities).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	groups, models := make(map[string]bool), make(map[string]bool)
	available := make(map[string][]string)
	for group := range setting.GetUserUsableGroupsCopy() {
		if group != "" && group != "auto" && ratio_setting.ContainsGroupRatio(group) {
			groups[group] = true
		}
	}
	for _, ability := range abilities {
		if !groups[ability.Group] {
			continue
		}
		name := ratio_setting.RoutingMatchModelName(ability.Model)
		models[name] = true
		if !slices.Contains(available[ability.Group], name) {
			available[ability.Group] = append(available[ability.Group], name)
		}
	}
	groupNames, modelNames := make([]string, 0, len(groups)), make([]string, 0, len(models))
	for group := range groups {
		groupNames = append(groupNames, group)
	}
	for name := range models {
		modelNames = append(modelNames, name)
	}
	slices.Sort(groupNames)
	slices.Sort(modelNames)
	common.ApiSuccess(c, gin.H{"groups": groupNames, "models": modelNames, "available": available, "config": config, "revision": revision})
}

func SaveGroupModelMatrix(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<20)
	var request struct {
		Config   model.GroupModelAccess `json:"config" binding:"required"`
		Revision string                 `json:"revision" binding:"required,len=64"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid group-model matrix"})
		return
	}
	_, err := model.SaveGroupModelAccess(request.Config, request.Revision)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrInvalidGroupModelAccess) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, model.ErrGroupModelAccessConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	// Return the canonical model names stored by the policy.
	config, revision, err := model.GetGroupModelAccess()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"config": config, "revision": revision})
}
