package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

var ErrGroupModelChannelDenied = errors.New("the user's group does not allow this channel for the requested model")

// GroupModelAllowedChannelIDs returns the union of the configured pools.
// nil means unrestricted; a non-nil empty map must remain deny-all.
func GroupModelAllowedChannelIDs(userGroup, modelName string) (map[int]struct{}, error) {
	groups, configured := setting.GetGroupModelChannelGroups(userGroup, modelName)
	if !configured {
		return nil, nil
	}
	allowed := make(map[int]struct{})
	for _, group := range groups {
		candidates, err := model.ListChannelSelectionCandidates(group, modelName, model.ChannelSelectionFilters{})
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			allowed[candidate.ChannelId] = struct{}{}
		}
	}
	return allowed, nil
}

// SelectionFilters intersects user policy with asset constraints without
// mutating either source. Each retry resolves the current policy again.
func (p *RetryParam) SelectionFilters() (model.ChannelSelectionFilters, error) {
	allowed, err := GroupModelAllowedChannelIDs(common.GetContextKeyString(p.Ctx, constant.ContextKeyUserGroup), p.ModelName)
	if err != nil {
		return model.ChannelSelectionFilters{}, err
	}
	if allowed == nil {
		allowed = p.AllowedChannelIds
	} else if p.AllowedChannelIds != nil {
		for id := range allowed {
			if _, ok := p.AllowedChannelIds[id]; !ok {
				delete(allowed, id)
			}
		}
	}
	return model.ChannelSelectionFilters{RequestPath: p.RequestPath, RequestBody: p.RequestBody, AllowedChannelIds: allowed}, nil
}

// ValidateSelectedChannelGroupPolicy protects affinity hits, pinned channels
// and task retries. Token-group membership is checked independently so policy
// never grants access outside the token's own candidate groups.
func ValidateSelectedChannelGroupPolicy(c *gin.Context, channelID int, modelName string) error {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	allowed, err := GroupModelAllowedChannelIDs(userGroup, modelName)
	if err != nil {
		return err
	}
	if allowed == nil {
		return nil
	}
	if _, ok := allowed[channelID]; !ok {
		return ErrGroupModelChannelDenied
	}
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group != "auto" {
		if model.IsChannelEnabledForGroupModel(group, modelName, channelID) {
			return nil
		}
		return ErrGroupModelChannelDenied
	}
	selectedGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	for _, candidate := range GetRequestAutoGroups(c, userGroup) {
		if selectedGroup != "" && candidate != selectedGroup {
			continue
		}
		if model.IsChannelEnabledForGroupModel(candidate, modelName, channelID) {
			return nil
		}
	}
	return ErrGroupModelChannelDenied
}

// GetUserGroupsEnabledModels exposes only models with an actual channel in
// both the user's model pools and one of the requested token groups.
func GetUserGroupsEnabledModels(userGroup string, groups []string) ([]string, error) {
	seen := make(map[string]bool)
	models := make([]string, 0)
	for _, group := range groups {
		for _, modelName := range model.GetGroupEnabledModels(group) {
			if seen[modelName] {
				continue
			}
			allowed, err := GroupModelAllowedChannelIDs(userGroup, modelName)
			if err != nil {
				return nil, err
			}
			if allowed != nil {
				candidates, err := model.ListChannelSelectionCandidates(group, modelName, model.ChannelSelectionFilters{AllowedChannelIds: allowed})
				if err != nil {
					return nil, err
				}
				if len(candidates) == 0 {
					continue
				}
			}
			seen[modelName] = true
			models = append(models, modelName)
		}
	}
	return models, nil
}
