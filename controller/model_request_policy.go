package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetModelRequestPolicy(c *gin.Context) {
	name := c.Query("model")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Model name is required"})
		return
	}
	policy := model.GetModelRequestPolicy(name)
	if policy == nil {
		policy = &model.ModelRequestPolicy{Model: name}
	}
	common.ApiSuccess(c, policy)
}

func SaveModelRequestPolicy(c *gin.Context) {
	var policy model.ModelRequestPolicy
	if err := common.DecodeJson(c.Request.Body, &policy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	encoded, err := common.Marshal(policy)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := model.ParseModelRequestPolicy(string(encoded)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.SaveModelRequestPolicy(policy); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "model.request_policy.update", map[string]any{"model": policy.Model})
	common.ApiSuccess(c, policy)
}
