package controller

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type userChannelRoutingRow struct {
	ChannelID         int    `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	DefaultPriority   int64  `json:"default_priority"`
	ModelPriority     *int64 `json:"model_priority"`
	InheritedPriority int64  `json:"inherited_priority"`
	PriorityOverride  *int64 `json:"priority_override"`
	EffectivePriority int64  `json:"effective_priority"`
	Enabled           bool   `json:"enabled"`
	Available         bool   `json:"available"`
}

type userChannelRoutingResponse struct {
	Models   []string                `json:"models"`
	Model    string                  `json:"model"`
	Revision string                  `json:"revision"`
	Channels []userChannelRoutingRow `json:"channels"`
}

// GET without model lists selectable and previously configured public models.
// Availability is a preview across usable groups; token/request filters still
// apply at request time and overrides never grant channel access.
func GetUserChannelRouting(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	modelName := strings.TrimSpace(c.Query("model"))
	if err != nil || userID <= 0 || len(modelName) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user channel routing query"})
		return
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	overrides, err := model.ListUserChannelRoutingOverrides(userID, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := userChannelRoutingResponse{Models: []string{}, Model: modelName, Channels: []userChannelRoutingRow{}}
	groups := make([]string, 0)
	for group := range service.GetUserUsableGroups(user.Group) {
		if group != "auto" {
			groups = append(groups, group)
		}
	}
	if modelName == "" {
		models, err := service.GetUserGroupsEnabledModels(user.Group, groups)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, override := range overrides {
			models = append(models, override.Model)
		}
		slices.Sort(models)
		response.Models = slices.Compact(models)
		common.ApiSuccess(c, response)
		return
	}
	response.Revision, err = model.UserChannelRoutingRevision(overrides)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	draftChannelGroups := false
	allowed, err := service.GroupModelAllowedChannelIDs(user.Group, modelName)
	if rawGroups, ok := c.GetQuery("channel_groups"); ok {
		draftChannelGroups = true
		var groups []string
		if len(rawGroups) > 64<<10 || common.UnmarshalJsonStr(rawGroups, &groups) != nil || groups == nil || len(groups) > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel groups"})
			return
		}
		encoded, marshalErr := common.Marshal(setting.GroupModelChannelGroups{"draft": map[string][]string{modelName: groups}})
		if marshalErr != nil {
			common.ApiError(c, marshalErr)
			return
		}
		if _, parseErr := setting.ParseGroupModelChannelGroups(string(encoded)); parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel groups"})
			return
		}
		allowed, err = service.ChannelGroupsAllowedChannelIDs(groups, modelName)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	available := make(map[int]struct{})
	for _, group := range groups {
		candidates, err := model.ListChannelSelectionCandidates(group, modelName, model.ChannelSelectionFilters{AllowedChannelIds: allowed})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, candidate := range candidates {
			available[candidate.ChannelId] = struct{}{}
		}
	}
	overrideByID := make(map[int]model.UserChannelRoutingOverride, len(overrides))
	for _, override := range overrides {
		overrideByID[override.ChannelId] = override
	}
	modelNames := []string{modelName}
	if normalized := ratio_setting.RoutingMatchModelName(modelName); normalized != "" && normalized != modelName {
		modelNames = append(modelNames, normalized)
	}
	seen := make(map[int]bool)
	for _, name := range modelNames {
		routes, err := model.ListModelChannelRoutings(name)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, route := range routes {
			override, configured := overrideByID[route.ChannelId]
			_, usable := available[route.ChannelId]
			if seen[route.ChannelId] || (!usable && (draftChannelGroups || !configured)) {
				continue
			}
			seen[route.ChannelId] = true
			row := userChannelRoutingRow{ChannelID: route.ChannelId, ChannelName: route.ChannelName,
				DefaultPriority: route.DefaultPriority, ModelPriority: route.PriorityOverride,
				InheritedPriority: route.EffectivePriority, EffectivePriority: route.EffectivePriority,
				PriorityOverride: override.Priority, Enabled: !override.Disabled, Available: usable}
			if override.Priority != nil {
				row.EffectivePriority = *override.Priority
			}
			response.Channels = append(response.Channels, row)
		}
	}
	// Stale records remain visible and can be reset even after channel removal.
	if !draftChannelGroups {
		for _, override := range overrides {
			if !seen[override.ChannelId] {
				row := userChannelRoutingRow{ChannelID: override.ChannelId, PriorityOverride: override.Priority, Enabled: !override.Disabled}
				if override.Priority != nil {
					row.EffectivePriority = *override.Priority
				}
				response.Channels = append(response.Channels, row)
			}
		}
	}
	slices.SortStableFunc(response.Channels, func(a, b userChannelRoutingRow) int {
		if a.EffectivePriority > b.EffectivePriority {
			return -1
		}
		if a.EffectivePriority < b.EffectivePriority {
			return 1
		}
		return a.ChannelID - b.ChannelID
	})
	common.ApiSuccess(c, response)
}

func PatchUserChannelRouting(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		Model     string                          `json:"model"`
		Revision  string                          `json:"revision"`
		Overrides []model.UserChannelRoutingPatch `json:"overrides"`
	}
	if err != nil || userID <= 0 || common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10), &request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user channel routing"})
		return
	}
	if err := model.PatchUserChannelRouting(userID, request.Model, request.Revision, request.Overrides); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrUserChannelRoutingConflict) {
			status = http.StatusConflict
		}
		if errors.Is(err, model.ErrInvalidUserChannelRouting) || errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAuditFor(c, userID, "user.channel_routing.update", map[string]any{"model": request.Model, "channels": len(request.Overrides)})
	common.ApiSuccess(c, nil)
}
