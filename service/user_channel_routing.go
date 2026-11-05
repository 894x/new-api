package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// Resolve strategy separately from the pool: a strategy-only override must not
// synthesize an empty candidate pool or grant any additional channel access.
func (p *RetryParam) RoutingStrategy(group string) (*model.UserRoutingStrategy, error) {
	if strategy, loaded := p.routingStrategies[group]; loaded {
		return strategy, nil
	}
	userID := common.GetContextKeyInt(p.Ctx, constant.ContextKeyUserId)
	names := []string{p.ModelName}
	if normalized := ratio_setting.RoutingMatchModelName(p.ModelName); normalized != "" && normalized != p.ModelName {
		names = append(names, normalized)
	}
	var strategy *model.UserRoutingStrategy
	for _, name := range names {
		rows, err := model.ListUserRoutingStrategies(userID, name)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.RoutingGroup == group {
				strategy = &row
				break
			}
		}
		if strategy != nil {
			break
		}
	}
	if strategy == nil {
		pool, err := p.ModelRoutingPool(group)
		if err != nil {
			return nil, err
		}
		if pool != nil {
			strategy = &model.UserRoutingStrategy{Mode: pool.Mode, MaxTTFTMS: pool.MaxTTFTMS, MaxTPOTMS: pool.MaxTPOTMS}
		}
	}
	if p.routingStrategies == nil {
		p.routingStrategies = make(map[string]*model.UserRoutingStrategy)
	}
	p.routingStrategies[group] = strategy
	return strategy, nil
}

// One database snapshot per request, shared by retries and copied RetryParams.
// No process-local policy cache: the next request sees edits on every replica.
func (p *RetryParam) UserChannelRoutingForGroup(group string) (map[int]model.UserChannelRoutingOverride, error) {
	if !p.userRoutingLoaded {
		userID := common.GetContextKeyInt(p.Ctx, constant.ContextKeyUserId)
		rows, err := model.ListUserGroupChannelRoutingOverrides(userID, p.ModelName)
		if err != nil {
			return nil, err
		}
		if normalized := ratio_setting.RoutingMatchModelName(p.ModelName); normalized != "" && normalized != p.ModelName {
			baseRows, err := model.ListUserGroupChannelRoutingOverrides(userID, normalized)
			if err != nil {
				return nil, err
			}
			rows = append(rows, baseRows...)
		}
		p.userGroupRouting, p.userRoutingLoaded = rows, true
	}
	resolved := make(map[int]model.UserChannelRoutingOverride)
	base := make(map[int]model.UserChannelRoutingOverride)
	for _, scoped := range p.userGroupRouting {
		if scoped.RoutingGroup != group {
			continue
		}
		row := scoped.UserChannelRoutingOverride
		if row.Model != p.ModelName {
			base[row.ChannelId] = row
			continue
		}
		resolved[row.ChannelId] = row
	}
	if len(resolved) == 0 {
		return base, nil
	}
	return resolved, nil
}
