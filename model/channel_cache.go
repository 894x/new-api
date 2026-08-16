package model

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type cachedChannelRouting struct {
	ChannelId int
	Priority  int64
	Weight    uint
}

var group2model2channels map[string]map[string][]cachedChannelRouting // enabled channel routing
var channelsIDM map[int]*Channel                                      // all channels include disabled
// channel2advancedCustomConfig caches parsed Advanced Custom (type 58) configs so
// path-aware selection avoids re-parsing JSON per request. Refreshed on full sync.
var channel2advancedCustomConfig map[int]*dto.AdvancedCustomConfig
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		InvalidatePricingCache()
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	newChannel2advancedCustomConfig := make(map[int]*dto.AdvancedCustomConfig)
	var channels []*Channel
	if err := DB.Find(&channels).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to sync channels from database: %v", err))
		return
	}
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
				newChannel2advancedCustomConfig[channel.Id] = config
			}
		}
	}
	var abilities []*Ability
	if err := DB.Where("enabled = ?", true).Find(&abilities).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to sync channel abilities from database: %v", err))
		return
	}
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]cachedChannelRouting)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]cachedChannelRouting)
	}
	for _, ability := range abilities {
		if _, ok := newChannelId2channel[ability.ChannelId]; !ok {
			continue
		}
		if _, ok := newGroup2model2channels[ability.Group]; !ok {
			newGroup2model2channels[ability.Group] = make(map[string][]cachedChannelRouting)
		}
		newGroup2model2channels[ability.Group][ability.Model] = append(
			newGroup2model2channels[ability.Group][ability.Model],
			cachedChannelRouting{
				ChannelId: ability.ChannelId,
				Priority:  effectiveAbilityPriority(ability),
				Weight:    ability.Weight,
			},
		)
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, routings := range model2channels {
			sort.Slice(routings, func(i, j int) bool {
				return routings[i].Priority > routings[j].Priority
			})
			newGroup2model2channels[group][model] = routings
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channel2advancedCustomConfig = newChannel2advancedCustomConfig
	channelSyncLock.Unlock()
	// Lock ordering: InvalidatePricingCache acquires updatePricingLock, and
	// GetPricing (holding updatePricingLock) nests channelSyncLock.RLock via
	// loadPricingAdvancedCustomConfigs. channelSyncLock MUST be released before
	// invalidating the pricing cache, otherwise the reversed order deadlocks.
	InvalidatePricingCache()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(group string, model string, retry int, requestPath string) (*Channel, error) {
	return GetRandomSatisfiedChannelWithFilter(group, model, retry, requestPath, nil)
}

// GetRandomSatisfiedChannelWithFilter selects a channel from the supplied
// allowed set. A nil set preserves the ordinary unconstrained selection path.
func GetRandomSatisfiedChannelWithFilter(group string, model string, retry int, requestPath string, allowedChannelIds map[int]struct{}) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannelWithFilter(group, model, retry, requestPath, allowedChannelIds)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	routings := filterChannelsByRequestPathAndModel(group2model2channels[group][model], requestPath, model)
	routings = filterChannelRoutingsByAllowedIds(routings, allowedChannelIds)

	// If no channels found, try to find channels with the normalized model name.
	if len(routings) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		routings = filterChannelsByRequestPathAndModel(group2model2channels[group][normalizedModel], requestPath, model)
		routings = filterChannelRoutingsByAllowedIds(routings, allowedChannelIds)
	}

	if len(routings) == 0 {
		return nil, nil
	}

	if len(routings) == 1 {
		if channel, ok := channelsIDM[routings[0].ChannelId]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", routings[0].ChannelId)
	}

	uniquePriorities := make(map[int64]bool)
	for _, routing := range routings {
		if _, ok := channelsIDM[routing.ChannelId]; !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", routing.ChannelId)
		}
		uniquePriorities[routing.Priority] = true
	}
	var sortedUniquePriorities []int64
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Slice(sortedUniquePriorities, func(i, j int) bool { return sortedUniquePriorities[i] > sortedUniquePriorities[j] })

	if retry < 0 {
		retry = 0
	}
	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := sortedUniquePriorities[retry]

	// get the priority for the given retry number
	var targetRoutings []cachedChannelRouting
	for _, routing := range routings {
		if routing.Priority == targetPriority {
			targetRoutings = append(targetRoutings, routing)
		}
	}

	if len(targetRoutings) == 0 {
		return nil, fmt.Errorf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority)
	}

	abilities := make([]Ability, 0, len(targetRoutings))
	for _, routing := range targetRoutings {
		abilities = append(abilities, Ability{ChannelId: routing.ChannelId, Weight: routing.Weight})
	}
	channelId := chooseChannelIdByWeight(abilities, common.GetRandomInt)
	if channel, ok := channelsIDM[channelId]; ok {
		return channel, nil
	}
	return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
}

func filterChannelRoutingsByAllowedIds(routings []cachedChannelRouting, allowedChannelIds map[int]struct{}) []cachedChannelRouting {
	if allowedChannelIds == nil {
		return routings
	}
	filtered := make([]cachedChannelRouting, 0, len(routings))
	for _, routing := range routings {
		if _, ok := allowedChannelIds[routing.ChannelId]; ok {
			filtered = append(filtered, routing)
		}
	}
	return filtered
}

// filterChannelsByRequestPathAndModel restricts candidates by request path and
// model. Only Advanced Custom (type 58) channels are path-checked: they are kept
// only when one of their configured routes matches requestPath and model. All
// other channel types always pass. When requestPath is empty, filtering is skipped.
// Caller must hold channelSyncLock (read lock). The cached slice is never mutated.
func filterChannelsByRequestPathAndModel(routings []cachedChannelRouting, requestPath string, model string) []cachedChannelRouting {
	if requestPath == "" || len(routings) == 0 {
		return routings
	}
	filtered := make([]cachedChannelRouting, 0, len(routings))
	for _, routing := range routings {
		channel, ok := channelsIDM[routing.ChannelId]
		if !ok {
			// keep it so the downstream consistency error is raised as before
			filtered = append(filtered, routing)
			continue
		}
		if channel.Type != constant.ChannelTypeAdvancedCustom {
			filtered = append(filtered, routing)
			continue
		}
		if config := channel2advancedCustomConfig[routing.ChannelId]; config != nil && config.SupportsPathForModel(requestPath, model) {
			filtered = append(filtered, routing)
		}
	}
	return filtered
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, routings := range model2channels {
				for i, routing := range routings {
					if routing.ChannelId == id {
						// remove the channel from the slice
						group2model2channels[group][model] = append(routings[:i], routings[i+1:]...)
						break
					}
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	if channel == nil {
		channelSyncLock.Unlock()
		return
	}

	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if oldChannel, ok := channelsIDM[channel.Id]; ok {
		logger.LogDebug(nil, "CacheUpdateChannel before: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, oldChannel.ChannelInfo.MultiKeyPollingIndex)
	}
	channelsIDM[channel.Id] = channel
	if channel2advancedCustomConfig == nil {
		channel2advancedCustomConfig = make(map[int]*dto.AdvancedCustomConfig)
	}
	delete(channel2advancedCustomConfig, channel.Id)
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if config := channel.GetOtherSettings().AdvancedCustom; config != nil {
			channel2advancedCustomConfig[channel.Id] = config
		}
	}
	logger.LogDebug(nil, "CacheUpdateChannel after: id=%d, name=%s, status=%d, polling_index=%d", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)
	// Lock ordering: do NOT hold channelSyncLock while calling
	// InvalidatePricingCache. GetPricing acquires updatePricingLock first and then
	// channelSyncLock.RLock (via loadPricingAdvancedCustomConfigs); acquiring
	// updatePricingLock while holding channelSyncLock would be an AB-BA deadlock.
	channelSyncLock.Unlock()
	InvalidatePricingCache()
}
