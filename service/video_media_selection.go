package service

import (
	"bytes"
	"errors"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/relayparam"
)

// Requests with video fields must compare delivery tiers before using affinity.
func RequestHasVideoMedia(body []byte) bool {
	return bytes.Contains(body, []byte(`"video_url"`))
}

// Media comparisons run outside the model cache lock. Only Inspect may perform
// metadata I/O, once per source per request; no candidate triggers conversion.
func filterVideoMediaChannels(param *RetryParam, group string) (*RetryParam, bool, error) {
	if !RequestHasVideoMedia(param.RequestBody) {
		return param, false, nil
	}
	filters, err := param.SelectionFilters()
	if err != nil {
		return nil, false, err
	}
	candidates, err := model.ListChannelSelectionCandidates(group, param.ModelName, filters)
	if err != nil {
		return nil, false, err
	}
	processor := requestVideoMedia(param.Ctx)
	tiers := [2]map[int]struct{}{make(map[int]struct{}), make(map[int]struct{})}
	configured, hadEligible := false, false
	var lastViolation error
	for _, candidate := range candidates {
		channel, err := model.CacheGetChannel(candidate.ChannelId)
		if err != nil {
			return nil, false, err
		}
		upstreamModel, _, err := channel.ResolveUpstreamModelName(param.ModelName)
		if err != nil {
			return nil, false, err
		}
		plans, tier, err := relayparam.PlanMediaDelivery(param.RequestBody, channel.GetOtherSettings().ParameterCapabilities, upstreamModel, processor, true)
		if err != nil {
			configured = true
			lastViolation = err
			continue
		}
		if len(plans) > 0 {
			configured = true
		}
		hadEligible = true
		if param.HasAttempted(candidate.ChannelId) {
			continue
		}
		if param.Capacity != nil {
			if _, blocked := param.Capacity.blocked[candidate.ChannelId]; blocked {
				continue
			}
		}
		tiers[tier][candidate.ChannelId] = struct{}{}
	}
	if !configured {
		return param, false, nil
	}
	if len(tiers[0]) == 0 && len(tiers[1]) == 0 && !hadEligible && lastViolation != nil {
		return nil, true, errors.Join(model.ErrParameterCapabilityUnsupported, lastViolation)
	}
	copyParam := *param
	copyParam.AllowedChannelIds = tiers[0]
	if len(tiers[0]) == 0 {
		copyParam.AllowedChannelIds = tiers[1]
	}
	return &copyParam, true, nil
}
