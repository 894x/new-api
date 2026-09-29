package service

import (
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
	p.userRouting = make(map[int]model.UserChannelRoutingOverride, len(rows))
	for _, row := range rows {
		p.userRouting[row.ChannelId] = row
	}
	p.userRoutingLoaded = true
	return p.userRouting, nil
}
