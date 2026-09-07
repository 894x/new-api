package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/channelcapacity"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// Keep counters exactly representable by both Go integers and Redis Lua numbers.
const MaxChannelModelRateLimit int64 = channelcapacity.MaxLimit

// ListChannelSelectionCandidates applies the same eligibility rules as normal
// routing, before priority/weight selection. Capacity spillover uses this set.
func ListChannelSelectionCandidates(group, model string, filters ChannelSelectionFilters) ([]Ability, error) {
	if !common.MemoryCacheEnabled {
		return listDBChannelCandidates(group, model, filters)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	routings, err := listCachedChannelCandidates(group, model, filters)
	if err != nil {
		return nil, err
	}
	result := make([]Ability, 0, len(routings))
	for _, route := range routings {
		if _, ok := channelsIDM[route.ChannelId]; !ok {
			return nil, fmt.Errorf("channel %d is missing", route.ChannelId)
		}
		result = append(result, Ability{ChannelId: route.ChannelId, Priority: common.GetPointer(route.Priority), Weight: route.Weight, RPM: route.RPM, TPM: route.TPM})
	}
	return result, nil
}

func ResolveChannelModelRateLimits(channel *Channel, publicModel string) (int64, int64, error) {
	models := []string{publicModel}
	if normalized := ratio_setting.FormatMatchingModelName(publicModel); normalized != "" && normalized != publicModel {
		models = append(models, normalized)
	}
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		for _, name := range models {
			for _, group := range group2model2channels {
				for _, route := range group[name] {
					if route.ChannelId == channel.Id {
						return route.RPM, route.TPM, nil
					}
				}
			}
		}
	} else {
		var overrides []ChannelModelOverride
		if err := DB.Where("channel_id = ? AND model IN ?", channel.Id, models).Find(&overrides).Error; err != nil {
			return 0, 0, err
		}
		for _, name := range models {
			for _, override := range overrides {
				if override.Model == name {
					routing := effectiveChannelModelRouting(channel, name, &override)
					return routing.EffectiveRPM, routing.EffectiveTPM, nil
				}
			}
		}
	}
	return channel.GetRPM(), channel.GetTPM(), nil
}

func ValidateChannelModelRateLimit(limit *int64) error {
	if limit != nil && (*limit < 0 || *limit > MaxChannelModelRateLimit) {
		return fmt.Errorf("must be an integer between 0 and %d", MaxChannelModelRateLimit)
	}
	return nil
}

func ValidateChannelRoutingLimits(channel *Channel) error {
	if err := ValidateChannelWeight(channel.Weight); err != nil {
		return err
	}
	if err := ValidateChannelModelRateLimit(channel.RPM); err != nil {
		return fmt.Errorf("channel rpm: %w", err)
	}
	if err := ValidateChannelModelRateLimit(channel.TPM); err != nil {
		return fmt.Errorf("channel tpm: %w", err)
	}
	return nil
}

func (channel *Channel) GetRPM() int64 {
	if channel.RPM == nil {
		return 0
	}
	return *channel.RPM
}

func (channel *Channel) GetTPM() int64 {
	if channel.TPM == nil {
		return 0
	}
	return *channel.TPM
}
