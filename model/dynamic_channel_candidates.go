package model

import (
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// SatisfiedChannelCandidate describes an eligible channel before priority or
// weight selection is applied.
type SatisfiedChannelCandidate struct {
	ChannelId int
	Priority  int64
	Weight    uint
}

// ListSatisfiedChannelCandidatesWithFilters returns every channel that passes
// the same group, model, request-path, video-resolution, and asset filters as
// the existing static selector. The returned slice is detached from the
// channel cache and is therefore safe to use after this function returns.
func ListSatisfiedChannelCandidatesWithFilters(
	group string,
	model string,
	requestPath string,
	videoResolution string,
	allowedChannelIds map[int]struct{},
) ([]SatisfiedChannelCandidate, error) {
	if allowedChannelIds != nil && len(allowedChannelIds) == 0 {
		return nil, nil
	}

	var candidates []SatisfiedChannelCandidate
	var err error
	if common.MemoryCacheEnabled {
		candidates, err = listCachedSatisfiedChannelCandidates(
			group,
			model,
			requestPath,
			videoResolution,
			allowedChannelIds,
		)
	} else {
		candidates, err = listDBSatisfiedChannelCandidates(
			group,
			model,
			requestPath,
			videoResolution,
			allowedChannelIds,
		)
	}
	if err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return candidates[i].ChannelId < candidates[j].ChannelId
	})
	return candidates, nil
}

func listCachedSatisfiedChannelCandidates(
	group string,
	model string,
	requestPath string,
	videoResolution string,
	allowedChannelIds map[int]struct{},
) ([]SatisfiedChannelCandidate, error) {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	routings := filterChannelsByRequestPathAndModel(group2model2channels[group][model], requestPath, model)
	routings = filterChannelRoutingsByAllowedIds(routings, allowedChannelIds)
	if len(routings) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		routings = filterChannelsByRequestPathAndModel(group2model2channels[group][normalizedModel], requestPath, model)
		routings = filterChannelRoutingsByAllowedIds(routings, allowedChannelIds)
	}
	resolutionCandidates := routings
	routings = filterChannelRoutingsByVideoResolution(routings, channel2videoCapabilityConfig, model, videoResolution)
	if videoResolution != "" && len(resolutionCandidates) > 0 && len(routings) == 0 {
		return nil, newVideoResolutionUnsupportedError(model, videoResolution)
	}

	candidates := make([]SatisfiedChannelCandidate, 0, len(routings))
	for _, routing := range routings {
		if _, ok := channelsIDM[routing.ChannelId]; !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", routing.ChannelId)
		}
		candidates = append(candidates, SatisfiedChannelCandidate{
			ChannelId: routing.ChannelId,
			Priority:  routing.Priority,
			Weight:    routing.Weight,
		})
	}
	return candidates, nil
}

func listDBSatisfiedChannelCandidates(
	group string,
	model string,
	requestPath string,
	videoResolution string,
	allowedChannelIds map[int]struct{},
) ([]SatisfiedChannelCandidate, error) {
	allowedChannelIdSlice := make([]int, 0, len(allowedChannelIds))
	for channelId := range allowedChannelIds {
		allowedChannelIdSlice = append(allowedChannelIdSlice, channelId)
	}

	load := func(abilityModel string) ([]Ability, int, error) {
		var abilities []Ability
		query := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, abilityModel, true)
		if allowedChannelIds != nil {
			query = query.Where("channel_id IN ?", allowedChannelIdSlice)
		}
		if err := query.Order("weight DESC").Find(&abilities).Error; err != nil {
			return nil, 0, err
		}
		return filterAbilitiesByRequestPathModelAndVideoResolution(abilities, requestPath, model, videoResolution)
	}

	abilities, pathEligibleCount, err := load(model)
	if err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		if normalizedModel != "" && normalizedModel != model {
			var normalizedPathEligibleCount int
			abilities, normalizedPathEligibleCount, err = load(normalizedModel)
			if err != nil {
				return nil, err
			}
			pathEligibleCount += normalizedPathEligibleCount
		}
	}
	if videoResolution != "" && pathEligibleCount > 0 && len(abilities) == 0 {
		return nil, newVideoResolutionUnsupportedError(model, videoResolution)
	}

	candidates := make([]SatisfiedChannelCandidate, 0, len(abilities))
	for i := range abilities {
		candidates = append(candidates, SatisfiedChannelCandidate{
			ChannelId: abilities[i].ChannelId,
			Priority:  effectiveAbilityPriority(&abilities[i]),
			Weight:    abilities[i].Weight,
		})
	}
	return candidates, nil
}
