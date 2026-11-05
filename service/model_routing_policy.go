package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

var ErrGroupModelChannelDenied = errors.New("the user's group does not allow this channel for the requested model")

// SelectionFilters intersects user policy with asset constraints without
// mutating either source. Each retry resolves the current policy again.
func (p *RetryParam) SelectionFilters(group string) (model.ChannelSelectionFilters, error) {
	userRouting, err := p.UserChannelRoutingForGroup(group)
	if err != nil {
		return model.ChannelSelectionFilters{}, err
	}
	allowed := p.AllowedChannelIds
	pool, err := p.ModelRoutingPool(group)
	if err != nil {
		return model.ChannelSelectionFilters{}, err
	}
	var poolChannels map[int]model.ModelRoutingPoolChannel
	if pool != nil && len(userRouting) == 0 {
		poolChannels = make(map[int]model.ModelRoutingPoolChannel, len(pool.Channels))
		for _, entry := range pool.Channels {
			if p.HasAttempted(entry.ChannelID) && !(p.DynamicRoutingEligible && pool.UsesDynamicRouting()) {
				continue
			}
			poolChannels[entry.ChannelID] = entry
		}
	}
	return model.ChannelSelectionFilters{UserRoutingOverrides: userRouting, RoutingPoolChannels: poolChannels, Constraints: GetChannelConstraints(p.Ctx).Filters, RequestPath: p.RequestPath, RequestBody: p.RequestBody, RequestBodySize: p.RequestBodySize, AllowedChannelIds: allowed}, nil
}

// ValidateSelectedChannelGroupPolicy protects affinity hits, pinned channels
// and task retries. Token-group membership is checked independently so policy
// never grants access outside the token's own candidate groups.
func ValidateSelectedChannelGroupPolicy(c *gin.Context, channelID int, modelName string) error {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	selectedGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	param := &RetryParam{Ctx: c, ModelName: modelName}
	if group != "auto" {
		permitted, poolErr := param.ModelRoutingPoolAllows(group, channelID)
		if poolErr != nil {
			return poolErr
		}
		if permitted && model.IsChannelEnabledForGroupModel(group, modelName, channelID) {
			return nil
		}
		return ErrGroupModelChannelDenied
	}
	for _, candidate := range GetRequestAutoGroups(c, userGroup) {
		if selectedGroup != "" && candidate != selectedGroup {
			continue
		}
		permitted, err := param.ModelRoutingPoolAllows(candidate, channelID)
		if err != nil {
			return err
		}
		if permitted && model.IsChannelEnabledForGroupModel(candidate, modelName, channelID) {
			return nil
		}
	}
	return ErrGroupModelChannelDenied
}

// GetUserGroupsEnabledModels exposes models with an enabled channel in the
// requested public groups. Routing pools control channel membership separately.
func GetUserGroupsEnabledModels(_ string, groups []string) ([]string, error) {
	seen := make(map[string]bool)
	models := make([]string, 0)
	for _, group := range groups {
		for _, modelName := range model.GetGroupEnabledModels(group) {
			if seen[modelName] {
				continue
			}
			seen[modelName] = true
			models = append(models, modelName)
		}
	}
	return models, nil
}
