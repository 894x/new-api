package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const modelRoutingSmartContextKey = "__model_routing_smart"

// Pool snapshots are request-local and shared by copied RetryParams. A new
// request observes persisted edits on every application replica.
func (p *RetryParam) ModelRoutingPool(group string) (*model.ModelRoutingPool, error) {
	if group == "" || group == "auto" {
		return nil, nil
	}
	if p.routingPools == nil {
		p.routingPools = make(map[string]*model.ModelRoutingPool)
	}
	if pool, loaded := p.routingPools[group]; loaded {
		return pool, nil
	}
	pool, err := model.GetModelRoutingPool(group, p.ModelName)
	if err != nil {
		return nil, err
	}
	// Request modifiers use the same base-model fallback as channel abilities.
	// They must not bypass an explicitly restricted or empty base-model pool.
	if normalized := ratio_setting.RoutingMatchModelName(p.ModelName); pool == nil && normalized != "" && normalized != p.ModelName {
		pool, err = model.GetModelRoutingPool(group, normalized)
		if err != nil {
			return nil, err
		}
	}
	p.routingPools[group] = pool
	return pool, nil
}

func RequestDynamicRoutingEnabled(c *gin.Context) bool {
	forced, _ := c.Get(modelRoutingSmartContextKey)
	return forced == true || DynamicRoutingEnabled()
}

func (p *RetryParam) ModelRoutingPoolAllows(group string, channelID int) (bool, error) {
	userRouting, err := p.UserChannelRoutingForGroup(group)
	if err != nil {
		return false, err
	}
	// Customer-specific membership/priority rules are the upper layer. The
	// administrator's model/group mode still governs how that pool is scheduled.
	if len(userRouting) > 0 {
		override, member := userRouting[channelID]
		return member && !override.Disabled, nil
	}
	pool, err := p.ModelRoutingPool(group)
	if err != nil || pool == nil {
		return pool == nil && err == nil, err
	}
	for _, entry := range pool.Channels {
		if entry.ChannelID == channelID {
			return entry.Enabled, nil
		}
	}
	return false, nil
}
