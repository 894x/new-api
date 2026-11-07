package controller

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type userChannelRoutingRow struct {
	ChannelID          int    `json:"channel_id"`
	ChannelName        string `json:"channel_name"`
	DefaultPriority    int64  `json:"default_priority"`
	ModelPriority      *int64 `json:"model_priority"`
	InheritedPriority  int64  `json:"inherited_priority"`
	PriorityOverride   *int64 `json:"priority_override"`
	EffectivePriority  int64  `json:"effective_priority"`
	Enabled            bool   `json:"enabled"`
	Available          bool   `json:"available"`
	Configured         bool   `json:"configured"`
	WeightOverride     *uint  `json:"weight_override"`
	InheritedWeight    uint   `json:"inherited_weight"`
	EffectiveWeight    uint   `json:"effective_weight"`
	RetryTimesOverride *int   `json:"retry_times_override,omitempty"`
}

type userChannelRoutingResponse struct {
	Models               []string                        `json:"models"`
	ConfiguredModels     []string                        `json:"configured_models"`
	ConfiguredKeyGroups  map[string][]string             `json:"configured_key_groups"`
	KeyGroups            map[string]string               `json:"key_groups"`
	Model                string                          `json:"model"`
	RoutingGroup         string                          `json:"routing_group,omitempty"`
	Revision             string                          `json:"revision"`
	Channels             []userChannelRoutingRow         `json:"channels"`
	UserID               int                             `json:"user_id"`
	Username             string                          `json:"username"`
	UserRole             int                             `json:"user_role"`
	ModelGroups          map[string][]string             `json:"model_groups,omitempty"`
	Source               string                          `json:"source,omitempty"`
	Mode                 string                          `json:"mode,omitempty"`
	StrategyMode         string                          `json:"strategy_mode"`
	StrategyRevision     string                          `json:"strategy_revision"`
	StrategyUserOverride bool                            `json:"strategy_user_override"`
	MaxTTFTMS            int64                           `json:"max_ttft_ms"`
	MaxTPOTMS            int64                           `json:"max_tpot_ms"`
	Configured           bool                            `json:"configured"`
	PoolChannels         []model.ModelRoutingPoolChannel `json:"pool_channels,omitempty"`
}

// GET without model lists selectable and previously configured public models.
// Availability is a preview across usable groups; token/request filters still
// apply at request time and overrides never grant channel access.
func GetUserChannelRouting(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	modelName := strings.TrimSpace(c.Query("model"))
	routingGroup := c.Query("routing_group")
	if err != nil || userID <= 0 || len(modelName) > 255 || modelName != "" && routingGroup == "" || len(routingGroup) > 64 || strings.TrimSpace(routingGroup) != routingGroup || routingGroup == "auto" || strings.ContainsAny(routingGroup, ",\x00\r\n") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user channel routing query"})
		return
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groupOverrides, err := model.ListUserGroupChannelRoutingOverrides(userID, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	strategies, err := model.ListUserRoutingStrategies(userID, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := userChannelRoutingResponse{Models: []string{}, ConfiguredModels: []string{}, ConfiguredKeyGroups: map[string][]string{}, KeyGroups: map[string]string{}, Model: modelName, RoutingGroup: routingGroup, Channels: []userChannelRoutingRow{}}
	response.UserID, response.Username, response.UserRole = user.Id, user.Username, user.Role
	groups := make([]string, 0)
	usable := service.GetUserUsableGroups(user.Group)
	for group := range usable {
		if group != "auto" {
			groups = append(groups, group)
		}
	}
	if modelName == "" {
		// Only globally published, currently usable Key groups belong in this
		// editor. Resource pools and the implicit customer identity group do not.
		for group, description := range setting.GetUserUsableGroupsCopy() {
			if _, ok := usable[group]; ok && group != "auto" && ratio_setting.ContainsGroupRatio(group) {
				response.KeyGroups[group] = description
			}
		}
		models, err := service.GetUserGroupsEnabledModels(user.Group, groups)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, override := range groupOverrides {
			models = append(models, override.Model)
			if _, public := response.KeyGroups[override.RoutingGroup]; public {
				response.ConfiguredModels = append(response.ConfiguredModels, override.Model)
				response.ConfiguredKeyGroups[override.Model] = append(response.ConfiguredKeyGroups[override.Model], override.RoutingGroup)
			}
		}
		for _, strategy := range strategies {
			models = append(models, strategy.Model)
			if _, public := response.KeyGroups[strategy.RoutingGroup]; public {
				response.ConfiguredModels = append(response.ConfiguredModels, strategy.Model)
				response.ConfiguredKeyGroups[strategy.Model] = append(response.ConfiguredKeyGroups[strategy.Model], strategy.RoutingGroup)
			}
		}
		for modelName, groups := range response.ConfiguredKeyGroups {
			slices.Sort(groups)
			response.ConfiguredKeyGroups[modelName] = slices.Compact(groups)
		}
		slices.Sort(response.ConfiguredModels)
		response.ConfiguredModels = slices.Compact(response.ConfiguredModels)
		slices.Sort(models)
		response.Models = slices.Compact(models)
		if c.Query("view") == "pool" {
			response.ModelGroups = make(map[string][]string)
			for group := range response.KeyGroups {
				for _, name := range model.GetGroupEnabledModels(group) {
					if slices.Contains(response.Models, name) {
						response.ModelGroups[name] = append(response.ModelGroups[name], group)
					}
				}
			}
			for name, groups := range response.ConfiguredKeyGroups {
				response.ModelGroups[name] = append(response.ModelGroups[name], groups...)
			}
			for name, groups := range response.ModelGroups {
				slices.Sort(groups)
				response.ModelGroups[name] = slices.Compact(groups)
			}
		}
		common.ApiSuccess(c, response)
		return
	}
	overrides := make([]model.UserChannelRoutingOverride, 0)
	for _, row := range groupOverrides {
		if row.RoutingGroup == routingGroup {
			overrides = append(overrides, row.UserChannelRoutingOverride)
		}
	}
	groups = []string{routingGroup}
	response.Revision, err = model.UserChannelRoutingRevision(overrides)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response.Configured = len(overrides) > 0
	available := make(map[int]struct{})
	for _, group := range groups {
		candidates, err := model.ListChannelSelectionCandidates(group, modelName, model.ChannelSelectionFilters{})
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
	poolMembers := make(map[int]bool)
	if c.Query("view") == "pool" && routingGroup != "" {
		members, err := model.ListModelRoutingPoolCandidates(routingGroup, modelName)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, member := range members {
			poolMembers[member.ChannelID] = true
		}
	}
	for _, name := range modelNames {
		routes, err := model.ListModelChannelRoutings(name)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, route := range routes {
			override, configured := overrideByID[route.ChannelId]
			_, usable := available[route.ChannelId]
			if seen[route.ChannelId] || c.Query("view") != "pool" && !usable && (routingGroup != "" || !configured) || c.Query("view") == "pool" && !usable && !configured && !poolMembers[route.ChannelId] {
				continue
			}
			seen[route.ChannelId] = true
			inherited := route.EffectivePriority
			inheritedWeight := route.EffectiveWeight
			row := userChannelRoutingRow{ChannelID: route.ChannelId, ChannelName: route.ChannelName,
				DefaultPriority: route.DefaultPriority, ModelPriority: route.PriorityOverride,
				InheritedPriority: inherited, EffectivePriority: inherited,
				PriorityOverride: override.Priority, Enabled: !override.Disabled, Available: usable, Configured: configured}
			row.WeightOverride, row.InheritedWeight, row.EffectiveWeight = override.Weight, inheritedWeight, inheritedWeight
			row.RetryTimesOverride = override.RetryTimes
			if override.Priority != nil {
				row.EffectivePriority = *override.Priority
			}
			if override.Weight != nil {
				row.EffectiveWeight = *override.Weight
			}
			response.Channels = append(response.Channels, row)
		}
	}
	// Stale records remain visible and can be reset even after channel removal.
	for _, override := range overrides {
		if !seen[override.ChannelId] {
			row := userChannelRoutingRow{ChannelID: override.ChannelId, PriorityOverride: override.Priority, WeightOverride: override.Weight, Enabled: !override.Disabled, Configured: true, RetryTimesOverride: override.RetryTimes}
			if override.Priority != nil {
				row.EffectivePriority = *override.Priority
			}
			if override.Weight != nil {
				row.EffectiveWeight = *override.Weight
			}
			response.Channels = append(response.Channels, row)
		}
	}
	if c.Query("view") == "pool" {
		previewContext := c.Copy()
		common.SetContextKey(previewContext, constant.ContextKeyUserId, userID)
		common.SetContextKey(previewContext, constant.ContextKeyUserGroup, user.Group)
		param := &service.RetryParam{Ctx: previewContext, ModelName: modelName}
		response.Source, response.Mode = "channel_defaults", model.RoutingModePriority
		if service.DynamicRoutingEnabled() {
			response.Mode = model.RoutingModeSmart
		}
		pool, err := param.ModelRoutingPool(routingGroup)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if pool != nil {
			response.Source, response.Mode = "default_pool", pool.Mode
		}
		response.StrategyMode = "inherit"
		for _, strategy := range strategies {
			if strategy.RoutingGroup == routingGroup {
				response.StrategyMode, response.StrategyRevision = strategy.Mode, strategy.Revision
			}
		}
		strategy, err := param.RoutingStrategy(routingGroup)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if strategy != nil {
			response.Mode, response.MaxTTFTMS, response.MaxTPOTMS = strategy.Mode, strategy.MaxTTFTMS, strategy.MaxTPOTMS
			response.StrategyUserOverride = strategy.UserId > 0
		}
		resolved, err := param.UserChannelRoutingForGroup(routingGroup)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if len(resolved) > 0 {
			response.Source = "user"
		}
		effective := make(map[int]model.Ability)
		for _, group := range groups {
			filters, err := param.SelectionFilters(group)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			candidates, err := model.ListChannelSelectionCandidates(group, modelName, filters)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			for _, candidate := range candidates {
				effective[candidate.ChannelId] = candidate
			}
		}
		response.PoolChannels = make([]model.ModelRoutingPoolChannel, 0, len(response.Channels))
		for i := range response.Channels {
			row := &response.Channels[i]
			candidate, enabled := effective[row.ChannelID]
			if enabled {
				row.EffectivePriority, row.EffectiveWeight = 0, candidate.Weight
				if candidate.Priority != nil {
					row.EffectivePriority = *candidate.Priority
				}
			}
			if len(resolved) > 0 {
				member, exists := resolved[row.ChannelID]
				if !exists {
					continue
				}
				enabled = !member.Disabled
			} else if pool != nil {
				continue
			} else if !enabled {
				continue
			}
			response.PoolChannels = append(response.PoolChannels, model.ModelRoutingPoolChannel{ChannelID: row.ChannelID, Priority: row.EffectivePriority, Weight: row.EffectiveWeight, Enabled: enabled})
		}
		if len(resolved) == 0 && pool != nil {
			response.PoolChannels = append(response.PoolChannels, pool.Channels...)
		}
		slices.SortStableFunc(response.PoolChannels, func(a, b model.ModelRoutingPoolChannel) int {
			if a.Priority > b.Priority {
				return -1
			}
			if a.Priority < b.Priority {
				return 1
			}
			return a.ChannelID - b.ChannelID
		})
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
		Model        string                          `json:"model"`
		RoutingGroup string                          `json:"routing_group"`
		Revision     string                          `json:"revision"`
		Overrides    []model.UserChannelRoutingPatch `json:"overrides"`
		Reset        bool                            `json:"reset"`
		Strategy     *model.UserRoutingStrategy      `json:"strategy"`
	}
	if err != nil || userID <= 0 || common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10), &request) != nil || request.RoutingGroup == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user channel routing"})
		return
	}
	if request.Strategy != nil && (request.Reset || request.Overrides != nil) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "strategy changes cannot include channel changes"})
		return
	}
	if request.Reset && len(request.Overrides) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "reset cannot include overrides"})
		return
	}
	if request.Strategy != nil {
		request.Strategy.UserId, request.Strategy.Model, request.Strategy.RoutingGroup = userID, request.Model, request.RoutingGroup
		err = model.SaveUserRoutingStrategy(request.Strategy, request.Strategy.Revision)
	} else if request.Reset {
		err = model.ResetUserChannelRoutingForGroup(userID, request.Model, request.RoutingGroup, request.Revision)
	} else {
		err = model.PatchUserChannelRoutingForGroup(userID, request.Model, request.RoutingGroup, request.Revision, request.Overrides)
	}
	if err != nil {
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
	audit := map[string]any{"model": request.Model, "routing_group": request.RoutingGroup, "channels": len(request.Overrides), "reset": request.Reset}
	if request.Strategy != nil {
		audit["strategy"] = request.Strategy.Mode
	}
	recordManageAuditFor(c, userID, "user.channel_routing.update", audit)
	common.ApiSuccess(c, nil)
}
