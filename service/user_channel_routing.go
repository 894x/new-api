package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

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
