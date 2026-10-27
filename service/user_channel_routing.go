package service

import (
	"maps"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// One database snapshot per request, shared by retries and copied RetryParams.
// No process-local policy cache: the next request sees edits on every replica.
func (p *RetryParam) UserChannelRouting() (map[int]model.UserChannelRoutingOverride, error) {
	if p.userRoutingLoaded {
		return p.userRouting, nil
	}
	rows, err := model.ListUserChannelRoutingOverrides(common.GetContextKeyInt(p.Ctx, constant.ContextKeyUserId), p.ModelName)
	if err != nil {
		return nil, err
	}
	groupRows, err := model.ListUserGroupChannelRoutingOverrides(common.GetContextKeyInt(p.Ctx, constant.ContextKeyUserId), p.ModelName)
	if err != nil {
		return nil, err
	}
	p.userGroupRouting = groupRows
	p.userRouting = make(map[int]model.UserChannelRoutingOverride, len(rows))
	for _, row := range rows {
		p.userRouting[row.ChannelId] = row
	}
	p.userRoutingLoaded = true
	return p.userRouting, nil
}

// Resolve only the actual outlet group. In particular, an auto-group retry
// must not reuse the previous group's priorities or disabled channels.
func (p *RetryParam) UserChannelRoutingForGroup(group string) (map[int]model.UserChannelRoutingOverride, error) {
	legacy, err := p.UserChannelRouting()
	if err != nil {
		return nil, err
	}
	if len(p.userGroupRouting) == 0 {
		return legacy, nil
	}
	resolved := make(map[int]model.UserChannelRoutingOverride, len(legacy))
	maps.Copy(resolved, legacy)
	for _, scoped := range p.userGroupRouting {
		if scoped.RoutingGroup != group {
			continue
		}
		row := scoped.UserChannelRoutingOverride
		if row.Priority == nil {
			inherited, exists := legacy[row.ChannelId]
			row.Priority = inherited.Priority
			// Keep the configured row visible in the editor while restoring
			// ordinary affinity/dynamic routing when no override remains.
			if !exists && !row.Disabled {
				continue
			}
		}
		resolved[row.ChannelId] = row
	}
	return resolved, nil
}
