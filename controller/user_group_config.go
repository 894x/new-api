/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

var userGroupConfigMutex sync.Mutex

const dedicatedUserGroupPrefix = "企业客户-"

type userGroupConfigUpdateRequest struct {
	Group              string                 `json:"group"`
	Revision           string                 `json:"revision"`
	Discounts          map[string]float64     `json:"discounts"`
	RateLimitEnabled   bool                   `json:"rate_limit_enabled"`
	RateLimit          setting.GroupRateLimit `json:"rate_limit"`
	ModelChannelGroups map[string][]string    `json:"model_channel_groups"`
}

type userGroupRateLimitResponse struct {
	Limits [3]int                            `json:"limits"`
	Models map[string]setting.ModelRateLimit `json:"models"`
}

type userGroupConfigResponse struct {
	UserID                 int                        `json:"user_id"`
	Username               string                     `json:"username"`
	Group                  string                     `json:"group"`
	DedicatedGroupName     string                     `json:"dedicated_group_name"`
	IsDedicatedGroup       bool                       `json:"is_dedicated_group"`
	Revision               string                     `json:"revision"`
	Discounts              map[string]float64         `json:"discounts"`
	RateLimitEnabled       bool                       `json:"rate_limit_enabled"`
	GlobalRateLimitEnabled bool                       `json:"global_rate_limit_enabled"`
	RateLimit              userGroupRateLimitResponse `json:"rate_limit"`
	ModelChannelGroups     map[string][]string        `json:"model_channel_groups"`
	AvailableGroupRatios   map[string]float64         `json:"available_group_ratios"`
	GroupUserCount         int64                      `json:"group_user_count"`
}

type createDedicatedUserGroupRequest struct {
	Group    string `json:"group"`
	Revision string `json:"revision"`
}

type previewUserGroupChannelsRequest struct {
	Group              string              `json:"group"`
	ModelChannelGroups map[string][]string `json:"model_channel_groups"`
}

type channelPoolPreview struct {
	CandidateCount int `json:"candidate_count"`
}

// PreviewUserGroupChannels counts enabled channel candidates in each draft
// pool. Token group and request capability filters can further reduce them.
func PreviewUserGroupChannels(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}
	var request previewUserGroupChannelsRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.ModelChannelGroups == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel pool preview"})
		return
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Group != user.Group {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": model.ErrUserGroupChanged.Error()})
		return
	}
	encoded, err := common.Marshal(setting.GroupModelChannelGroups{user.Group: request.ModelChannelGroups})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := setting.ParseGroupModelChannelGroups(string(encoded)); err != nil {
		common.ApiError(c, err)
		return
	}
	previews := make(map[string]channelPoolPreview, len(request.ModelChannelGroups))
	for modelName, groups := range request.ModelChannelGroups {
		candidates, err := service.ChannelGroupsAllowedChannelIDs(groups, modelName)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		previews[modelName] = channelPoolPreview{CandidateCount: len(candidates)}
	}
	common.ApiSuccess(c, previews)
}

// CreateDedicatedUserGroup gives one user a stable identity group while
// retaining the source group's billing, rate-limit, and routing policies.
func CreateDedicatedUserGroup(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}
	var request createDedicatedUserGroupRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Group == "" || request.Revision == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user group configuration"})
		return
	}

	userGroupConfigMutex.Lock()
	defer userGroupConfigMutex.Unlock()

	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user.Group != request.Group {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": model.ErrUserGroupChanged.Error()})
		return
	}
	if user.Role != common.RoleCommonUser {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "dedicated groups are only available for common users"})
		return
	}
	currentConfig, err := buildUserGroupConfig(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Revision != currentConfig.Revision {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "user group policies changed; reload the configuration"})
		return
	}
	newGroup := dedicatedUserGroupPrefix + user.Username
	if utf8.RuneCountInString(newGroup) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "dedicated user group name exceeds 64 characters"})
		return
	}
	if user.Group == newGroup {
		config, err := buildUserGroupConfig(userID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, config)
		return
	}
	if strings.HasPrefix(user.Group, dedicatedUserGroupPrefix) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "user already has a dedicated group"})
		return
	}
	if _, exists := ratio_setting.GetGroupRatioCopy()[newGroup]; exists {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "dedicated user group name is already in use"})
		return
	}
	var groupUsers int64
	if err := model.DB.Model(&model.User{}).Where(&model.User{Group: newGroup}).Count(&groupUsers).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if groupUsers > 0 {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "dedicated user group name is already in use"})
		return
	}

	discounts := make(map[string]map[string]float64)
	if err := common.UnmarshalJsonStr(ratio_setting.GroupGroupRatio2JSONString(), &discounts); err != nil {
		common.ApiError(c, err)
		return
	}
	if _, exists := discounts[newGroup]; exists {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "dedicated user group policy already exists"})
		return
	}
	if source, exists := discounts[user.Group]; exists {
		discounts[newGroup] = maps.Clone(source)
	}

	rateLimits := make(map[string]json.RawMessage)
	if err := common.UnmarshalJsonStr(setting.ModelRequestRateLimitGroup2JSONString(), &rateLimits); err != nil {
		common.ApiError(c, err)
		return
	}
	if _, exists := rateLimits[newGroup]; exists {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "dedicated user group policy already exists"})
		return
	}
	if source, exists := rateLimits[user.Group]; exists {
		rateLimits[newGroup] = append(json.RawMessage(nil), source...)
	}

	channelPolicies, err := setting.ParseGroupModelChannelGroups(setting.GroupModelChannelGroupsJSON())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, exists := channelPolicies[newGroup]; exists {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "dedicated user group policy already exists"})
		return
	}
	if source, exists := channelPolicies[user.Group]; exists {
		copy := make(map[string][]string, len(source))
		for modelName, groups := range source {
			copy[modelName] = append([]string(nil), groups...)
			if groups != nil && len(groups) == 0 {
				copy[modelName] = []string{}
			}
		}
		channelPolicies[newGroup] = copy
	}

	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	if _, exists := specialGroups[newGroup]; exists {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "dedicated user group policy already exists"})
		return
	}
	if source, exists := specialGroups[user.Group]; exists {
		specialGroups[newGroup] = maps.Clone(source)
	}

	values := make(map[string]string, 4)
	for key, value := range map[string]any{
		"GroupGroupRatio":                                discounts,
		"ModelRequestRateLimitGroup":                     rateLimits,
		setting.GroupModelChannelGroupsOptionKey:         channelPolicies,
		"group_ratio_setting.group_special_usable_group": specialGroups,
	} {
		encoded, err := common.Marshal(value)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		values[key] = string(encoded)
	}
	if err := model.UpdateOptionsBulkWithUserGroupChange(values, model.UserGroupTransition{
		UserID: userID, ExpectedGroup: user.Group, NewGroup: newGroup,
	}); err != nil {
		if errors.Is(err, model.ErrUserGroupChanged) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
			return
		}
		common.ApiError(c, err)
		return
	}
	if err := model.PublishUserAuthCache(userID); err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := model.RevokeAllUserSessions(userID, "admin_user_group_change"); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, userID, "user.group_config.create_dedicated", map[string]any{
		"source_group": user.Group,
		"group":        newGroup,
	})
	config, err := buildUserGroupConfig(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}

func GetUserGroupConfig(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}

	config, err := buildUserGroupConfig(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}

func UpdateUserGroupConfig(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		common.ApiErrorMsg(c, "invalid user id")
		return
	}

	var request userGroupConfigUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Revision == "" || request.Discounts == nil || request.ModelChannelGroups == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user group configuration"})
		return
	}

	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(user.Group) == "" {
		common.ApiErrorMsg(c, "user group must not be empty")
		return
	}
	if request.Group != user.Group {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "user group changed; reload the configuration"})
		return
	}
	for targetGroup, ratio := range request.Discounts {
		if targetGroup == "" || strings.TrimSpace(targetGroup) != targetGroup || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
			common.ApiErrorMsg(c, fmt.Sprintf("invalid discount for group %q", targetGroup))
			return
		}
	}

	userGroupConfigMutex.Lock()
	defer userGroupConfigMutex.Unlock()
	currentConfig, err := buildUserGroupConfig(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if request.Revision != currentConfig.Revision {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "user group policies changed; reload the configuration"})
		return
	}

	discountPolicies := make(map[string]map[string]float64)
	if err := common.UnmarshalJsonStr(ratio_setting.GroupGroupRatio2JSONString(), &discountPolicies); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(request.Discounts) == 0 {
		delete(discountPolicies, user.Group)
	} else {
		discountPolicies[user.Group] = request.Discounts
	}

	rateLimits := make(map[string]json.RawMessage)
	if err := common.UnmarshalJsonStr(setting.ModelRequestRateLimitGroup2JSONString(), &rateLimits); err != nil {
		common.ApiError(c, err)
		return
	}
	if request.RateLimitEnabled {
		encoded, err := common.Marshal(request.RateLimit)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		rateLimits[user.Group] = encoded
	} else {
		delete(rateLimits, user.Group)
	}

	channelPolicies, err := setting.ParseGroupModelChannelGroups(setting.GroupModelChannelGroupsJSON())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(request.ModelChannelGroups) == 0 {
		delete(channelPolicies, user.Group)
	} else {
		channelPolicies[user.Group] = request.ModelChannelGroups
	}

	discountsJSON, err := common.Marshal(discountPolicies)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rateLimitsJSON, err := common.Marshal(rateLimits)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelPoliciesJSON, err := common.Marshal(channelPolicies)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := setting.CheckModelRequestRateLimitGroup(string(rateLimitsJSON)); err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := setting.ParseGroupModelChannelGroups(string(channelPoliciesJSON)); err != nil {
		common.ApiError(c, err)
		return
	}

	if err := model.UpdateOptionsBulk(map[string]string{
		"GroupGroupRatio":                        string(discountsJSON),
		"ModelRequestRateLimitGroup":             string(rateLimitsJSON),
		setting.GroupModelChannelGroupsOptionKey: string(channelPoliciesJSON),
	}); err != nil {
		common.ApiError(c, err)
		return
	}

	recordManageAuditFor(c, userID, "user.group_config.update", map[string]any{
		"group": user.Group,
		"keys":  []string{"GroupGroupRatio", "ModelRequestRateLimitGroup", setting.GroupModelChannelGroupsOptionKey},
	})
	config, err := buildUserGroupConfig(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, config)
}

func buildUserGroupConfig(userID int) (*userGroupConfigResponse, error) {
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return nil, err
	}
	var groupUserCount int64
	if err := model.DB.Model(&model.User{}).Where(&model.User{Group: user.Group}).Count(&groupUserCount).Error; err != nil {
		return nil, err
	}

	discountPolicies := make(map[string]map[string]float64)
	if err := common.UnmarshalJsonStr(ratio_setting.GroupGroupRatio2JSONString(), &discountPolicies); err != nil {
		return nil, err
	}
	discounts := discountPolicies[user.Group]
	if discounts == nil {
		discounts = map[string]float64{}
	}

	rateLimits := setting.GetModelRequestRateLimitGroups()
	rateLimit, rateLimitEnabled := rateLimits[user.Group]
	if !rateLimitEnabled {
		rateLimit = setting.GroupRateLimit{Limits: [3]int{0, 1, 0}, Models: map[string]setting.ModelRateLimit{}}
	}
	if rateLimit.Models == nil {
		rateLimit.Models = map[string]setting.ModelRateLimit{}
	}

	channelPolicies, err := setting.ParseGroupModelChannelGroups(setting.GroupModelChannelGroupsJSON())
	if err != nil {
		return nil, err
	}
	modelChannelGroups := channelPolicies[user.Group]
	if modelChannelGroups == nil {
		modelChannelGroups = map[string][]string{}
	}
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()[user.Group]
	revisionPayload, err := common.Marshal(struct {
		Username           string                 `json:"username"`
		Group              string                 `json:"group"`
		Discounts          map[string]float64     `json:"discounts"`
		RateLimitEnabled   bool                   `json:"rate_limit_enabled"`
		RateLimit          setting.GroupRateLimit `json:"rate_limit"`
		ModelChannelGroups map[string][]string    `json:"model_channel_groups"`
		SpecialGroups      map[string]string      `json:"special_groups"`
	}{user.Username, user.Group, discounts, rateLimitEnabled, rateLimit, modelChannelGroups, specialGroups})
	if err != nil {
		return nil, err
	}
	revision := fmt.Sprintf("%x", sha256.Sum256(revisionPayload))

	return &userGroupConfigResponse{
		UserID:                 user.Id,
		Username:               user.Username,
		Group:                  user.Group,
		DedicatedGroupName:     dedicatedUserGroupPrefix + user.Username,
		IsDedicatedGroup:       strings.HasPrefix(user.Group, dedicatedUserGroupPrefix),
		Revision:               revision,
		Discounts:              discounts,
		RateLimitEnabled:       rateLimitEnabled,
		GlobalRateLimitEnabled: setting.ModelRequestRateLimitEnabled,
		RateLimit: userGroupRateLimitResponse{
			Limits: rateLimit.Limits,
			Models: rateLimit.Models,
		},
		ModelChannelGroups:   modelChannelGroups,
		AvailableGroupRatios: ratio_setting.GetGroupRatioCopy(),
		GroupUserCount:       groupUserCount,
	}, nil
}
