package controller

import (
	"cmp"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type modelRoutingPoolView struct {
	model.ModelRoutingPool
	Candidates     []model.ModelRoutingPoolCandidate             `json:"candidates"`
	TuningDefaults dynamic_routing_setting.DynamicRoutingSetting `json:"tuning_defaults"`
	UserID         int                                           `json:"user_id,omitempty"`
	Username       string                                        `json:"username,omitempty"`
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
	channels := make([]struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}, 0)
	if err := model.DB.Model(&model.Channel{}).Select("id", "name").Order("name, id").Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"groups": groups, "models": models, "channels": channels})
}

func ListModelRoutingPools(c *gin.Context) {
	filter := struct {
		Model       string `form:"model" binding:"max=255"`
		Group       string `form:"routing_group" binding:"max=64"`
		Mode        string `form:"mode" binding:"omitempty,oneof=priority smart discount"`
		Scope       string `form:"scope" binding:"omitempty,oneof=all"`
		ChannelID   int    `form:"channel_id" binding:"gte=0"`
		ChannelRole string `form:"channel_role" binding:"omitempty,oneof=primary non_primary"`
		Page        int    `form:"page" binding:"gte=1,lte=1000000"`
		PageSize    int    `form:"page_size" binding:"gte=1,lte=50"`
	}{Page: 1, PageSize: 12}
	if err := c.ShouldBindQuery(&filter); err != nil || filter.ChannelRole != "" && filter.ChannelID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid routing pool filters"})
		return
	}
	if filter.Scope == "all" && !authz.Can(c.GetInt("id"), c.GetInt("role"), authz.UserGroupConfigRead) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "insufficient user routing permissions"})
		return
	}
	query := model.DB.Model(&model.ModelRoutingPool{})
	if filter.Model != "" {
		query = query.Where("model LIKE ?", "%"+strings.TrimSpace(filter.Model)+"%")
	}
	if filter.Group != "" {
		query = query.Where("routing_group = ?", filter.Group)
	}
	var rows []model.ModelRoutingPool
	if err := query.Order("model, routing_group").Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	allowedPairs, err := model.GetAllowedGroupModelPairs()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows = slices.DeleteFunc(rows, func(row model.ModelRoutingPool) bool {
		return !allowedPairs[row.RoutingGroup][ratio_setting.RoutingMatchModelName(row.Model)]
	})
	items := make([]modelRoutingPoolView, 0, len(rows))
	defaults := make(map[string]model.ModelRoutingPool, len(rows))
	for _, row := range rows {
		if err := common.UnmarshalJsonStr(row.ChannelsJSON, &row.Channels); err != nil {
			common.ApiError(c, err)
			return
		}
		if err := row.DecodeTuning(); err != nil {
			common.ApiError(c, err)
			return
		}
		defaults[row.RoutingGroup+"\x00"+row.Model] = row
		items = append(items, modelRoutingPoolView{ModelRoutingPool: row, TuningDefaults: dynamic_routing_setting.GetSetting()})
	}
	if filter.Scope == "all" {
		// Query only explicit memberships and strategy overrides. Inherited users
		// do not create duplicate copies of every default pool.
		var overrides []model.UserGroupChannelRoutingOverride
		var strategies []model.UserRoutingStrategy
		for _, dest := range []any{&overrides, &strategies} {
			userQuery := model.DB
			if filter.Model != "" {
				userQuery = userQuery.Where("model LIKE ?", "%"+strings.TrimSpace(filter.Model)+"%")
			}
			if filter.Group != "" {
				userQuery = userQuery.Where("routing_group = ?", filter.Group)
			}
			if err := userQuery.Find(dest).Error; err != nil {
				common.ApiError(c, err)
				return
			}
		}
		userIDs := make([]int, 0, len(overrides)+len(strategies))
		for _, row := range overrides {
			userIDs = append(userIDs, row.UserId)
		}
		for _, row := range strategies {
			userIDs = append(userIDs, row.UserId)
		}
		slices.Sort(userIDs)
		userIDs = slices.Compact(userIDs)
		users := make([]model.User, 0)
		for ids := range slices.Chunk(userIDs, 500) {
			var batch []model.User
			if err := model.DB.Select([]string{"id", "username", "role", "group"}).Where("id IN ?", ids).Find(&batch).Error; err != nil {
				common.ApiError(c, err)
				return
			}
			users = append(users, batch...)
		}
		byUser := make(map[int]model.User, len(users))
		usableGroups := make(map[int]map[string]string, len(users))
		publicGroups := setting.GetUserUsableGroupsCopy()
		for _, user := range users {
			if canManageTargetRole(c.GetInt("role"), user.Role) {
				byUser[user.Id] = user
				usableGroups[user.Id] = service.GetUserUsableGroups(user.Group)
			}
		}
		type poolIdentity struct {
			userID       int
			group, model string
		}
		userPools := make(map[poolIdentity]*modelRoutingPoolView)
		priorityOverrides := make(map[poolIdentity]map[int]*int64)
		for _, row := range overrides {
			key := poolIdentity{row.UserId, row.RoutingGroup, row.Model}
			pool := userPools[key]
			if pool == nil {
				pool = &modelRoutingPoolView{ModelRoutingPool: model.ModelRoutingPool{Model: row.Model, RoutingGroup: row.RoutingGroup}}
				userPools[key] = pool
			}
			pool.Channels = append(pool.Channels, model.ModelRoutingPoolChannel{ChannelID: row.ChannelId, Enabled: !row.Disabled})
			if filter.ChannelRole != "" {
				if priorityOverrides[key] == nil {
					priorityOverrides[key] = make(map[int]*int64)
				}
				priorityOverrides[key][row.ChannelId] = row.Priority
			}
		}
		for _, row := range strategies {
			if row.Mode == "inherit" {
				continue
			}
			key := poolIdentity{row.UserId, row.RoutingGroup, row.Model}
			pool := userPools[key]
			if pool == nil {
				pool = &modelRoutingPoolView{ModelRoutingPool: model.ModelRoutingPool{Model: row.Model, RoutingGroup: row.RoutingGroup}}
				userPools[key] = pool
			}
			pool.Mode = row.Mode
		}
		candidateCache := make(map[string][]model.ModelRoutingPoolCandidate)
		for key, pool := range userPools {
			user, exists := byUser[key.userID]
			_, usable := usableGroups[key.userID][key.group]
			_, public := publicGroups[key.group]
			if !exists || !usable || !public || key.group == "auto" || !ratio_setting.ContainsGroupRatio(key.group) || !allowedPairs[key.group][ratio_setting.RoutingMatchModelName(key.model)] {
				continue
			}
			defaultKey := key.group + "\x00" + key.model
			inherited, cached := defaults[defaultKey]
			if !cached {
				defaultPool, err := model.GetModelRoutingPool(key.group, key.model)
				if err == nil && defaultPool == nil {
					if normalized := ratio_setting.RoutingMatchModelName(key.model); normalized != key.model {
						defaultPool, err = model.GetModelRoutingPool(key.group, normalized)
					}
				}
				if err != nil {
					common.ApiError(c, err)
					return
				}
				if defaultPool != nil {
					inherited = *defaultPool
				}
				defaults[defaultKey] = inherited
			}
			if pool.Mode == "" {
				pool.Mode = inherited.Mode
				if pool.Mode == "" {
					pool.Mode = model.RoutingModePriority
					if service.DynamicRoutingEnabled() {
						pool.Mode = model.RoutingModeSmart
					}
				}
			}
			var candidates []model.ModelRoutingPoolCandidate
			if filter.ChannelRole != "" && len(pool.Channels) > 0 || len(pool.Channels) == 0 && filter.ChannelID > 0 && inherited.Mode == "" {
				var cached bool
				candidates, cached = candidateCache[defaultKey]
				if !cached {
					var err error
					candidates, err = model.ListModelRoutingPoolCandidates(key.group, key.model)
					if err != nil {
						common.ApiError(c, err)
						return
					}
					candidateCache[defaultKey] = candidates
				}
			}
			if len(pool.Channels) == 0 && filter.ChannelID > 0 {
				pool.Channels = inherited.Channels
				if inherited.Mode == "" {
					for _, candidate := range candidates {
						pool.Channels = append(pool.Channels, model.ModelRoutingPoolChannel{ChannelID: candidate.ChannelID, Priority: candidate.Priority, Weight: candidate.Weight, Enabled: candidate.Available})
					}
				}
			}
			if filter.ChannelRole != "" {
				// Match the user card's effective priority order, preserving explicit
				// zero priorities and inheriting channel values only for nil overrides.
				defaultPriorities := make(map[int]int64, len(candidates))
				for _, candidate := range candidates {
					defaultPriorities[candidate.ChannelID] = candidate.Priority
				}
				for i := range pool.Channels {
					entry := &pool.Channels[i]
					priority, member := priorityOverrides[key][entry.ChannelID]
					if !member {
						continue
					}
					if priority != nil {
						entry.Priority = *priority
						continue
					}
					entry.Priority = defaultPriorities[entry.ChannelID]
				}
				pool.Channels = slices.Clone(pool.Channels)
				slices.SortFunc(pool.Channels, func(a, b model.ModelRoutingPoolChannel) int {
					return cmp.Or(cmp.Compare(b.Priority, a.Priority), cmp.Compare(a.ChannelID, b.ChannelID))
				})
			}
			pool.UserID, pool.Username = user.Id, user.Username
			items = append(items, *pool)
		}
	}
	items = slices.DeleteFunc(items, func(pool modelRoutingPoolView) bool {
		if filter.Mode != "" && pool.Mode != filter.Mode {
			return true
		}
		if filter.ChannelID == 0 {
			return false
		}
		if !slices.ContainsFunc(pool.Channels, func(channel model.ModelRoutingPoolChannel) bool { return channel.ChannelID == filter.ChannelID }) {
			return true
		}
		if filter.ChannelRole == "" {
			return false
		}
		firstEnabled := slices.IndexFunc(pool.Channels, func(channel model.ModelRoutingPoolChannel) bool { return channel.Enabled })
		primary := firstEnabled >= 0 && pool.Channels[firstEnabled].ChannelID == filter.ChannelID
		return primary != (filter.ChannelRole == "primary")
	})
	slices.SortFunc(items, func(a, b modelRoutingPoolView) int {
		return cmp.Or(cmp.Compare(a.Model, b.Model), cmp.Compare(a.RoutingGroup, b.RoutingGroup), cmp.Compare(a.UserID, b.UserID))
	})
	total := len(items)
	start := min((filter.Page-1)*filter.PageSize, total)
	items = items[start:min(start+filter.PageSize, total)]
	for i := range items {
		row := &items[i]
		if row.UserID > 0 {
			continue
		}
		candidates, err := model.ListModelRoutingPoolCandidates(row.RoutingGroup, row.Model)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		candidates = slices.DeleteFunc(candidates, func(candidate model.ModelRoutingPoolCandidate) bool {
			return !slices.ContainsFunc(row.Channels, func(entry model.ModelRoutingPoolChannel) bool { return entry.ChannelID == candidate.ChannelID })
		})
		row.Candidates = candidates
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
	// Keep the editor's exact pool identity while reporting the same inherited
	// scheduling mode used for requests with model modifiers.
	effectivePool := pool
	if normalized := ratio_setting.RoutingMatchModelName(name); effectivePool == nil && normalized != "" && normalized != name {
		effectivePool, err = model.GetModelRoutingPool(group, normalized)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	var effectiveMode *string
	if effectivePool != nil {
		effectiveMode = &effectivePool.Mode
	}
	candidates, err := model.ListModelRoutingPoolCandidates(group, name)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"pool": pool, "candidates": candidates, "effective_mode": effectiveMode, "tuning_defaults": dynamic_routing_setting.GetSetting()})
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
		Tuning    json.RawMessage                  `json:"tuning"`
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
	var tuning *dynamic_routing_setting.Overrides
	if len(request.Tuning) > 0 {
		if err := common.Unmarshal(request.Tuning, &tuning); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid routing tuning"})
			return
		}
	} else if request.Revision != "" {
		// Older clients editing membership do not know about tuning. Omission
		// preserves it; an explicit empty object or null restores inheritance.
		current, err := model.GetModelRoutingPool(request.Group, request.Model)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if current != nil {
			tuning = current.Tuning
		}
	}
	pool := model.ModelRoutingPool{RoutingGroup: request.Group, Model: request.Model, Mode: request.Mode, Channels: *request.Channels, MaxTTFTMS: request.MaxTTFTMS, MaxTPOTMS: request.MaxTPOTMS, Tuning: tuning}
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
