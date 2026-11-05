package controller

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type modelRoutingPoolView struct {
	model.ModelRoutingPool
	Candidates []model.ModelRoutingPoolCandidate `json:"candidates"`
}

func GetModelRoutingPoolCatalog(c *gin.Context) {
	groups := make(map[string]string)
	groupNames := make([]string, 0)
	for group, description := range setting.GetUserUsableGroupsCopy() {
		if group != "auto" && ratio_setting.ContainsGroupRatio(group) {
			groups[group] = description
			groupNames = append(groupNames, group)
		}
	}
	models := make([]string, 0)
	query := model.DB.Model(&model.Ability{}).Where("enabled = ?", true)
	if group := c.Query("routing_group"); group != "" {
		if _, exists := groups[group]; !exists {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid public group"})
			return
		}
		groupNames = []string{group}
	}
	if len(groupNames) > 0 {
		var abilities []model.Ability
		if err := query.Where(map[string]any{"group": groupNames}).Select("group", "model").Distinct().Order("model").Find(&abilities).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		seen := make(map[string]bool)
		for _, ability := range abilities {
			if !seen[ability.Model] && model.IsGroupModelAllowed(ability.Group, ability.Model) {
				seen[ability.Model] = true
				models = append(models, ability.Model)
			}
		}
	}
	common.ApiSuccess(c, gin.H{"groups": groups, "models": models})
}

func ListModelRoutingPools(c *gin.Context) {
	filter := struct {
		Model    string `form:"model" binding:"max=255"`
		Group    string `form:"routing_group" binding:"max=64"`
		Mode     string `form:"mode" binding:"omitempty,oneof=priority smart discount"`
		Page     int    `form:"page" binding:"gte=1,lte=1000000"`
		PageSize int    `form:"page_size" binding:"gte=1,lte=50"`
	}{Page: 1, PageSize: 12}
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid routing pool filters"})
		return
	}
	query := model.DB.Model(&model.ModelRoutingPool{})
	if filter.Model != "" {
		query = query.Where("model LIKE ?", "%"+strings.TrimSpace(filter.Model)+"%")
	}
	if filter.Group != "" {
		query = query.Where("routing_group = ?", filter.Group)
	}
	if filter.Mode != "" {
		query = query.Where("mode = ?", filter.Mode)
	}
	var rows []model.ModelRoutingPool
	if err := query.Order("model, routing_group").Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	rows = slices.DeleteFunc(rows, func(row model.ModelRoutingPool) bool {
		return !model.IsGroupModelAllowed(row.RoutingGroup, row.Model)
	})
	total := len(rows)
	start := min((filter.Page-1)*filter.PageSize, total)
	rows = rows[start:min(start+filter.PageSize, total)]
	items := make([]modelRoutingPoolView, 0, len(rows))
	for _, row := range rows {
		if err := common.UnmarshalJsonStr(row.ChannelsJSON, &row.Channels); err != nil {
			common.ApiError(c, err)
			return
		}
		candidates, err := model.ListModelRoutingPoolCandidates(row.RoutingGroup, row.Model)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		candidates = slices.DeleteFunc(candidates, func(candidate model.ModelRoutingPoolCandidate) bool {
			return !slices.ContainsFunc(row.Channels, func(entry model.ModelRoutingPoolChannel) bool { return entry.ChannelID == candidate.ChannelID })
		})
		items = append(items, modelRoutingPoolView{ModelRoutingPool: row, Candidates: candidates})
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total, "page": filter.Page, "page_size": filter.PageSize})
}

func GetModelRoutingPool(c *gin.Context) {
	group, name := c.Query("routing_group"), c.Query("model")
	if group == "" || group == "auto" || name == "" || len(group) > 64 || len(name) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid routing pool identity"})
		return
	}
	if !model.IsGroupModelAllowed(group, name) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "the group-model matrix does not allow this model"})
		return
	}
	pool, err := model.GetModelRoutingPool(group, name)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	candidates, err := model.ListModelRoutingPoolCandidates(group, name)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"pool": pool, "candidates": candidates})
}

func SaveModelRoutingPool(c *gin.Context) {
	var request struct {
		Group     string                           `json:"routing_group"`
		Model     string                           `json:"model"`
		Mode      string                           `json:"mode"`
		Revision  string                           `json:"revision"`
		Channels  *[]model.ModelRoutingPoolChannel `json:"channels"`
		MaxTTFTMS int64                            `json:"max_ttft_ms"`
		MaxTPOTMS int64                            `json:"max_tpot_ms"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Channels == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid routing pool configuration"})
		return
	}
	if _, public := setting.GetUserUsableGroupsCopy()[request.Group]; !public || !ratio_setting.ContainsGroupRatio(request.Group) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid public group"})
		return
	}
	if !model.IsGroupModelAllowed(request.Group, request.Model) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "the group-model matrix does not allow this model"})
		return
	}
	pool := model.ModelRoutingPool{RoutingGroup: request.Group, Model: request.Model, Mode: request.Mode, Channels: *request.Channels, MaxTTFTMS: request.MaxTTFTMS, MaxTPOTMS: request.MaxTPOTMS}
	if err := model.SaveModelRoutingPool(&pool, request.Revision); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrInvalidRoutingPool) {
			status = http.StatusBadRequest
		}
		if errors.Is(err, model.ErrRoutingPoolConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, pool)
}

func DeleteModelRoutingPool(c *gin.Context) {
	var request struct {
		Group    string `json:"routing_group"`
		Model    string `json:"model"`
		Revision string `json:"revision"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid routing pool identity"})
		return
	}
	if err := model.DeleteModelRoutingPool(request.Group, request.Model, request.Revision); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrRoutingPoolConflict) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, nil)
}
