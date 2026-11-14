package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
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
	binding, err := p.RoutingServiceType(group)
	if err != nil {
		return nil, err
	}
	serviceTypeID := 0
	if binding != nil {
		serviceTypeID = binding.ServiceTypeID
	}
	pool, err := model.ResolveModelRoutingPool(group, p.ModelName, serviceTypeID)
	if err != nil {
		return nil, err
	}
	p.routingPools[group] = pool
	return pool, nil
}

func (p *RetryParam) RoutingServiceType(group string) (*model.UserRoutingServiceType, error) {
	if !p.serviceTypesLoaded {
		rows, err := model.ListUserRoutingServiceTypes(common.GetContextKeyInt(p.Ctx, constant.ContextKeyUserId))
		if err != nil {
			return nil, err
		}
		p.serviceTypeBindings, p.serviceTypesLoaded = rows, true
	}
	return model.ResolveUserRoutingServiceType(p.serviceTypeBindings, p.ModelName, group), nil
}

// Service types share configuration. Observations retain user isolation so a
// customer's strategy overrides cannot alter another customer's route state.
func (p *RetryParam) RoutingPolicyKey(group string) (string, error) {
	overrides, err := p.UserChannelRoutingForGroup(group)
	if err != nil {
		return "", err
	}
	strategy, err := p.RoutingStrategy(group)
	if err != nil {
		return "", err
	}
	pool, err := p.ModelRoutingPool(group)
	if err != nil {
		return "", err
	}
	if len(overrides) > 0 || strategy != nil && strategy.UserId > 0 || pool != nil && pool.ServiceTypeID > 0 {
		return fmt.Sprintf("user:%d", common.GetContextKeyInt(p.Ctx, constant.ContextKeyUserId)), nil
	}
	return "", nil
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
