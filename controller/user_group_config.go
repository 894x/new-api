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
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

var userGroupConfigMutex sync.Mutex

type userGroupConfigUpdateRequest struct {
	Group              string                 `json:"group"`
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
	Discounts              map[string]float64         `json:"discounts"`
	RateLimitEnabled       bool                       `json:"rate_limit_enabled"`
	GlobalRateLimitEnabled bool                       `json:"global_rate_limit_enabled"`
	RateLimit              userGroupRateLimitResponse `json:"rate_limit"`
	ModelChannelGroups     map[string][]string        `json:"model_channel_groups"`
	AvailableGroupRatios   map[string]float64         `json:"available_group_ratios"`
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
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Discounts == nil || request.ModelChannelGroups == nil {
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

	return &userGroupConfigResponse{
		UserID:                 user.Id,
		Username:               user.Username,
		Group:                  user.Group,
		Discounts:              discounts,
		RateLimitEnabled:       rateLimitEnabled,
		GlobalRateLimitEnabled: setting.ModelRequestRateLimitEnabled,
		RateLimit: userGroupRateLimitResponse{
			Limits: rateLimit.Limits,
			Models: rateLimit.Models,
		},
		ModelChannelGroups:   modelChannelGroups,
		AvailableGroupRatios: ratio_setting.GetGroupRatioCopy(),
	}, nil
}
