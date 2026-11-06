package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
)

var dynamicRoutingRuntime = struct {
	sync.Mutex
	controller *dynamicrouting.Controller
	version    uint64
}{}

func DynamicRoutingEnabled() bool {
	return dynamic_routing_setting.GetSnapshot().Config.Enabled
}

func currentDynamicRoutingController(force bool) (*dynamicrouting.Controller, bool, error) {
	dynamicRoutingRuntime.Lock()
	defer dynamicRoutingRuntime.Unlock()
	snapshot := dynamic_routing_setting.GetSnapshot()
	if !snapshot.Config.Enabled && !force {
		return nil, false, nil
	}
	// Explicit smart pools use the shared tuning settings independently of the
	// legacy global switch. Static pools never enter this controller.
	snapshot.Config.Enabled = true

	if dynamicRoutingRuntime.controller == nil {
		controller, err := dynamicrouting.NewController(snapshot.Config)
		if err != nil {
			return nil, false, err
		}
		dynamicRoutingRuntime.controller = controller
		dynamicRoutingRuntime.version = snapshot.Version
		startRoutingMonitorReporter()
	} else if dynamicRoutingRuntime.version != snapshot.Version {
		if err := dynamicRoutingRuntime.controller.UpdateConfig(snapshot.Config); err != nil {
			return nil, false, err
		}
		dynamicRoutingRuntime.version = snapshot.Version
	}
	return dynamicRoutingRuntime.controller, true, nil
}

// ObserveDynamicRoutingSample records one completed upstream attempt. Callers
// enable observation for the legacy global switch or an explicit smart pool.
func ObserveDynamicRoutingSample(key dynamicrouting.ObservationKey, sample dynamicrouting.Sample, explicitSmartPool ...bool) {
	force := len(explicitSmartPool) > 0 && explicitSmartPool[0]
	controller, enabled, err := currentDynamicRoutingController(force)
	if err != nil || !enabled {
		return
	}
	controller.Observe(key, sample)
}

func getSatisfiedChannelForRoute(param *RetryParam, group string, retry int) (*model.Channel, bool, error) {
	userRouting, err := param.UserChannelRoutingForGroup(group)
	if err != nil {
		return nil, false, err
	}
	filtered, mediaSelection, filterErr := filterVideoMediaChannels(param, group)
	if filterErr != nil {
		return nil, false, filterErr
	}
	param = filtered
	if mediaSelection {
		// Attempted channels are already removed; do not skip another priority tier.
		retry = 0
	}
	pool, err := param.ModelRoutingPool(group)
	if err != nil {
		return nil, false, err
	}
	strategy, err := param.RoutingStrategy(group)
	if err != nil {
		return nil, false, err
	}
	smart := strategy.UsesDynamicRouting()
	param.Ctx.Set(modelRoutingSmartContextKey, smart)
	if param.DynamicRoutingEligible && (smart || strategy == nil && len(userRouting) == 0) {
		channel, handled, allAttempted, err := selectDynamicSatisfiedChannel(param, group)
		if handled || err != nil {
			return channel, allAttempted, err
		}
	}
	if pool != nil && len(userRouting) == 0 && len(param.AttemptedChannelIds) > 0 {
		// Attempted channels are excluded by the request-local pool filter;
		// skipping another priority tier would skip a valid fallback as well.
		retry = 0
	}
	channel, err := selectChannelWithCapacity(param, group, retry)
	allAttempted := false
	if pool != nil && channel == nil {
		for _, entry := range pool.Channels {
			allAttempted = allAttempted || param.HasAttempted(entry.ChannelID)
		}
	}
	return channel, allAttempted, err
}

func selectDynamicSatisfiedChannel(param *RetryParam, group string) (*model.Channel, bool, bool, error) {
	strategy, err := param.RoutingStrategy(group)
	if err != nil {
		return nil, true, false, err
	}
	controller, enabled, err := currentDynamicRoutingController(strategy.UsesDynamicRouting())
	if err != nil || !enabled {
		return nil, false, false, err
	}

	filters, err := param.SelectionFilters(group)
	if err != nil {
		return nil, true, false, err
	}
	eligible, err := model.ListChannelSelectionCandidates(group, param.ModelName, filters)
	if err != nil {
		return nil, true, false, err
	}
	if len(eligible) == 0 {
		return nil, true, false, nil
	}
	// Exhausting an upstream retry must honor the group's retry policy even
	// when remaining candidates were denied locally by capacity admission.
	allExcluded, hasAttempted := true, false
	for _, candidate := range eligible {
		if param.HasAttempted(candidate.ChannelId) {
			hasAttempted = true
			continue
		}
		if param.Capacity != nil {
			if _, blocked := param.Capacity.blocked[candidate.ChannelId]; blocked {
				continue
			}
		}
		allExcluded = false
	}
	allAttempted := allExcluded && hasAttempted

	excluded := make(map[int]struct{}, len(param.AttemptedChannelIds))
	for id := range param.AttemptedChannelIds {
		excluded[id] = struct{}{}
	}
	if param.Capacity != nil {
		for _, candidate := range eligible {
			param.Capacity.eligible[candidate.ChannelId] = struct{}{}
		}
		for id := range param.Capacity.blocked {
			excluded[id] = struct{}{}
		}
	}
	var discounts map[int]*float64
	if strategy != nil && strategy.Mode == model.RoutingModeDiscount {
		discounts, err = model.ResolveChannelSelectionDiscounts(eligible)
		if err != nil {
			return nil, true, false, err
		}
	}
	candidates := make([]dynamicrouting.Candidate, 0, len(eligible))
	for _, candidate := range eligible {
		priority := int64(0)
		if candidate.Priority != nil {
			priority = *candidate.Priority
		}
		candidates = append(candidates, dynamicrouting.Candidate{
			ChannelID:    candidate.ChannelId,
			Priority:     priority,
			Weight:       candidate.Weight,
			CostDiscount: discounts[candidate.ChannelId],
		})
	}
	key := dynamicrouting.RouteKey{
		Group: group,
		Model: param.ModelName,
	}
	if len(filters.UserRoutingOverrides) > 0 || strategy != nil && strategy.UserId > 0 {
		key.Policy = fmt.Sprintf("user:%d", common.GetContextKeyInt(param.Ctx, constant.ContextKeyUserId))
	}
	var decision dynamicrouting.Decision
	pool, err := param.ModelRoutingPool(group)
	if err != nil {
		return nil, true, false, err
	}
	if pool != nil && pool.Tuning != nil {
		effective, err := pool.Tuning.Resolve(dynamic_routing_setting.GetSetting())
		if err != nil {
			return nil, true, false, err
		}
		config, err := dynamic_routing_setting.RuntimeConfig(effective)
		if err != nil {
			return nil, true, false, err
		}
		var limits *dynamicrouting.DiscountLimits
		if strategy != nil && strategy.Mode == model.RoutingModeDiscount {
			limits = &dynamicrouting.DiscountLimits{MaxTTFT: time.Duration(strategy.MaxTTFTMS) * time.Millisecond, MaxTPOT: time.Duration(strategy.MaxTPOTMS) * time.Millisecond}
		}
		decision, err = controller.SelectConfiguredAvoiding(key, candidates, excluded, time.Now(), config, limits)
		if err != nil {
			return nil, true, false, err
		}
	} else if strategy != nil && strategy.Mode == model.RoutingModeDiscount {
		limits := dynamicrouting.DiscountLimits{MaxTTFT: time.Duration(strategy.MaxTTFTMS) * time.Millisecond, MaxTPOT: time.Duration(strategy.MaxTPOTMS) * time.Millisecond}
		decision = controller.SelectDiscountAvoiding(key, candidates, excluded, limits, time.Now())
	} else {
		decision = controller.SelectAvoiding(key, candidates, excluded, time.Now())
	}
	if !decision.HasSelection {
		return nil, true, allAttempted, nil
	}

	channel, err := model.CacheGetChannel(decision.SelectedChannelID)
	if err == nil && channel != nil {
		discount := strategy != nil && strategy.Mode == model.RoutingModeDiscount
		param.Ctx.Set(routingMonitorDispatchContextKey, routingMonitorDispatch{channelID: decision.SelectedChannelID, started: func(at time.Time) {
			controller.RecordDispatch(key, discount, decision.SelectedChannelID, at)
		}})
	}
	return channel, true, false, err
}
