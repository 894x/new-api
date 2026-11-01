package service

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type ChannelDisableImpactChannel struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ChannelDisableImpactCustomer struct {
	UserID              int    `json:"user_id"`
	Username            string `json:"username"`
	DisplayName         string `json:"display_name"`
	ChannelID           int    `json:"channel_id"`
	Model               string `json:"model"`
	KeyGroup            string `json:"key_group"`
	EffectivePriority   int64  `json:"effective_priority"`
	Primary             bool   `json:"primary"`
	AlternativeChannels int    `json:"alternative_channels"`
}

type ChannelDisableImpact struct {
	Channels      []ChannelDisableImpactChannel  `json:"channels"`
	Customers     []ChannelDisableImpactCustomer `json:"customers"`
	CustomerCount int                            `json:"customer_count"`
}

type channelImpactRoute struct {
	group string
	model string
}

// channelImpactKeyRoutes resolves the model/group routes reachable through a
// customer's active keys, including inherited groups and token-scoped Auto.
func channelImpactKeyRoutes(user model.User, tokens []model.Token, models map[string]bool) ([]channelImpactRoute, error) {
	seen := make(map[channelImpactRoute]bool)
	for _, token := range tokens {
		groups := []string{token.Group}
		if token.Group == "" {
			groups = []string{user.Group}
		} else if token.Group == "auto" {
			groups = GetUserAutoGroup(user.Group)
			configured, err := token.GetAutoGroups()
			if err != nil {
				return nil, err
			}
			if len(configured) > 0 {
				groups = FilterUserTokenAutoGroups(user.Group, configured)
			}
		}
		limits := token.GetModelLimitsMap()
		for _, group := range groups {
			if !GroupInUserUsableGroups(user.Group, group) {
				continue
			}
			for name := range models {
				if token.ModelLimitsEnabled && !limits[name] && !limits[ratio_setting.FormatMatchingModelName(name)] && !limits[ratio_setting.RoutingMatchModelName(name)] {
					continue
				}
				seen[channelImpactRoute{group, name}] = true
			}
		}
	}
	routes := make([]channelImpactRoute, 0, len(seen))
	for route := range seen {
		routes = append(routes, route)
	}
	return routes, nil
}

// GetChannelDisableImpact previews configuration eligibility for active users
// with usable API keys. It does not claim that a request is currently in flight
// or predict request-specific affinity, capacity, or parameter constraints.
func GetChannelDisableImpact(ctx context.Context, ids []int, tag *string, operatorRole int) (ChannelDisableImpact, error) {
	result := ChannelDisableImpact{Channels: []ChannelDisableImpactChannel{}, Customers: []ChannelDisableImpactCustomer{}}
	snapshot, err := model.LoadChannelDisableImpactSnapshot(ctx, ids, tag, operatorRole)
	if err != nil {
		return result, err
	}
	selected := make(map[int]bool, len(snapshot.Channels))
	modelNames := make(map[string]bool)
	for _, channel := range snapshot.Channels {
		selected[channel.Id] = true
		result.Channels = append(result.Channels, ChannelDisableImpactChannel{ID: channel.Id, Name: channel.Name})
		for name := range strings.SplitSeq(channel.Models, ",") {
			if name != "" {
				modelNames[name] = true
			}
		}
	}
	tokens := make(map[int][]model.Token)
	for _, token := range snapshot.Tokens {
		tokens[token.UserId] = append(tokens[token.UserId], token)
	}
	type userModel struct {
		userID int
		model  string
	}
	scoped := make(map[userModel][]model.UserGroupChannelRoutingOverride)
	for _, row := range snapshot.GroupOverrides {
		key := userModel{row.UserId, row.Model}
		scoped[key] = append(scoped[key], row)
	}
	// Shared public-group candidates are resolved once,
	// rather than issuing a candidate query for every customer or API key.
	candidates := make(map[channelImpactRoute][]model.Ability)
	defaultPools := make(map[channelImpactRoute]*model.ModelRoutingPool)
	for _, user := range snapshot.Users {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		affected := false
		keyRoutes, err := channelImpactKeyRoutes(user, tokens[user.Id], modelNames)
		if err != nil {
			return result, err
		}
		for _, key := range keyRoutes {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			group, name := key.group, key.model
			routes, loaded := candidates[key]
			if !loaded {
				routes, err = model.ListChannelSelectionCandidates(group, name, model.ChannelSelectionFilters{})
				if err != nil {
					return result, err
				}
				candidates[key] = routes
			}
			policyKey := userModel{user.Id, name}
			param := RetryParam{ModelName: name, userRoutingLoaded: true, userGroupRouting: scoped[policyKey]}
			routing, err := param.UserChannelRoutingForGroup(group)
			if err != nil {
				return result, err
			}
			var poolChannels map[int]model.ModelRoutingPoolChannel
			if len(routing) == 0 {
				pool, loaded := defaultPools[key]
				if !loaded {
					pool, err = param.ModelRoutingPool(group)
					if err != nil {
						return result, err
					}
					defaultPools[key] = pool
				}
				if pool != nil {
					poolChannels = make(map[int]model.ModelRoutingPoolChannel, len(pool.Channels))
					for _, entry := range pool.Channels {
						poolChannels[entry.ChannelID] = entry
					}
				}
			}
			eligible := make(map[int]int64)
			var highest int64
			remaining := 0
			for _, route := range routes {
				override, member := routing[route.ChannelId]
				if len(routing) > 0 && !member || override.Disabled {
					continue
				}
				priority := int64(0)
				if route.Priority != nil {
					priority = *route.Priority
				}
				if poolChannels != nil {
					entry, member := poolChannels[route.ChannelId]
					if !member || !entry.Enabled {
						continue
					}
					priority = entry.Priority
				}
				if override.Priority != nil {
					priority = *override.Priority
				}
				if len(eligible) == 0 || priority > highest {
					highest = priority
				}
				eligible[route.ChannelId] = priority
				if !selected[route.ChannelId] {
					remaining++
				}
			}
			for id, priority := range eligible {
				if !selected[id] {
					continue
				}
				affected = true
				result.Customers = append(result.Customers, ChannelDisableImpactCustomer{
					UserID: user.Id, Username: user.Username, DisplayName: user.DisplayName,
					ChannelID: id, Model: name, KeyGroup: group, EffectivePriority: priority,
					Primary: priority == highest, AlternativeChannels: remaining,
				})
			}
		}
		if affected {
			result.CustomerCount++
		}
	}
	slices.SortFunc(result.Customers, func(a, b ChannelDisableImpactCustomer) int {
		if a.Primary != b.Primary {
			if a.Primary {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(a.UserID, b.UserID), cmp.Compare(a.Model, b.Model), cmp.Compare(a.KeyGroup, b.KeyGroup), cmp.Compare(a.ChannelID, b.ChannelID))
	})
	return result, nil
}
