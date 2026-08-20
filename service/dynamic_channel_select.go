package service

import (
	"sync"
	"time"

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

func currentDynamicRoutingController() (*dynamicrouting.Controller, bool, error) {
	snapshot := dynamic_routing_setting.GetSnapshot()
	if !snapshot.Config.Enabled {
		return nil, false, nil
	}

	dynamicRoutingRuntime.Lock()
	defer dynamicRoutingRuntime.Unlock()

	if dynamicRoutingRuntime.controller == nil {
		controller, err := dynamicrouting.NewController(snapshot.Config)
		if err != nil {
			return nil, false, err
		}
		dynamicRoutingRuntime.controller = controller
		dynamicRoutingRuntime.version = snapshot.Version
	} else if dynamicRoutingRuntime.version != snapshot.Version {
		if err := dynamicRoutingRuntime.controller.UpdateConfig(snapshot.Config); err != nil {
			return nil, false, err
		}
		dynamicRoutingRuntime.version = snapshot.Version
	}
	return dynamicRoutingRuntime.controller, true, nil
}

// ObserveDynamicRoutingSample records one completed upstream attempt. Samples
// are ignored while dynamic routing is disabled.
func ObserveDynamicRoutingSample(key dynamicrouting.ObservationKey, sample dynamicrouting.Sample) {
	controller, enabled, err := currentDynamicRoutingController()
	if err != nil || !enabled {
		return
	}
	controller.Observe(key, sample)
}

func getSatisfiedChannelForRoute(param *RetryParam, group string, retry int) (*model.Channel, bool, error) {
	if param.DynamicRoutingEligible {
		channel, handled, allAttempted, err := selectDynamicSatisfiedChannel(param, group)
		if handled || err != nil {
			return channel, allAttempted, err
		}
	}
	channel, err := model.GetRandomSatisfiedChannelWithFilters(
		group,
		param.ModelName,
		retry,
		param.RequestPath,
		param.VideoResolution,
		param.AllowedChannelIds,
	)
	return channel, false, err
}

func selectDynamicSatisfiedChannel(param *RetryParam, group string) (*model.Channel, bool, bool, error) {
	controller, enabled, err := currentDynamicRoutingController()
	if err != nil || !enabled {
		return nil, false, false, err
	}

	eligible, err := model.ListSatisfiedChannelCandidatesWithFilters(
		group,
		param.ModelName,
		param.RequestPath,
		param.VideoResolution,
		param.AllowedChannelIds,
	)
	if err != nil {
		return nil, true, false, err
	}
	if len(eligible) == 0 {
		return nil, true, false, nil
	}
	allAttempted := true
	for _, candidate := range eligible {
		if !param.HasAttempted(candidate.ChannelId) {
			allAttempted = false
			break
		}
	}

	candidates := make([]dynamicrouting.Candidate, 0, len(eligible))
	for _, candidate := range eligible {
		candidates = append(candidates, dynamicrouting.Candidate{
			ChannelID: candidate.ChannelId,
			Priority:  candidate.Priority,
			Weight:    candidate.Weight,
		})
	}
	decision := controller.SelectAvoiding(dynamicrouting.RouteKey{
		Group: group,
		Model: param.ModelName,
	}, candidates, param.AttemptedChannelIds, time.Now())
	if !decision.HasSelection {
		return nil, true, allAttempted, nil
	}

	channel, err := model.CacheGetChannel(decision.SelectedChannelID)
	return channel, true, false, err
}
