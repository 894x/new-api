package sim

import (
	"time"

	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
)

type DynamicPolicy struct {
	controller *dynamicrouting.Controller
	routeKey   dynamicrouting.RouteKey
	epoch      time.Time
	channelIDs map[string]int
	credits    map[int]float64
}

func NewDynamicPolicy(config dynamicrouting.Config, _ int64, routeKey dynamicrouting.RouteKey) (*DynamicPolicy, error) {
	controller, err := dynamicrouting.NewController(config)
	if err != nil {
		return nil, err
	}
	return &DynamicPolicy{
		controller: controller,
		routeKey:   routeKey,
		epoch:      time.Unix(0, 0).UTC(),
		channelIDs: make(map[string]int),
		credits:    make(map[int]float64),
	}, nil
}

func (p *DynamicPolicy) Select(now time.Duration, candidates []Candidate) Decision {
	coreCandidates := make([]dynamicrouting.Candidate, 0, len(candidates))
	labels := make(map[int]string, len(candidates))
	for _, candidate := range candidates {
		coreCandidates = append(coreCandidates, dynamicrouting.Candidate{
			ChannelID: candidate.ChannelID,
			Priority:  candidate.Priority,
			Weight:    candidate.Weight,
		})
		labels[candidate.ChannelID] = candidate.ID
		p.channelIDs[candidate.ID] = candidate.ChannelID
	}
	decision := p.controller.Select(p.routeKey, coreCandidates, p.epoch.Add(now))
	if len(decision.Allocations) == 0 {
		return Decision{}
	}
	selected := decision.Allocations[0]
	dominant := decision.Allocations[0]
	if decision.HasSelection {
		for _, allocation := range decision.Allocations {
			if allocation.ChannelID == decision.SelectedChannelID {
				selected = allocation
				break
			}
		}
	} else {
		active := make(map[int]struct{}, len(decision.Allocations))
		totalShare := 0.0
		selectedCredit := -1e100
		for _, allocation := range decision.Allocations {
			active[allocation.ChannelID] = struct{}{}
			totalShare += allocation.Share
			p.credits[allocation.ChannelID] += allocation.Share
			if p.credits[allocation.ChannelID] > selectedCredit {
				selected = allocation
				selectedCredit = p.credits[allocation.ChannelID]
			}
		}
		for channelID := range p.credits {
			if _, exists := active[channelID]; !exists {
				delete(p.credits, channelID)
			}
		}
		p.credits[selected.ChannelID] -= totalShare
	}
	for _, allocation := range decision.Allocations {
		if allocation.Share > dominant.Share {
			dominant = allocation
		}
	}
	result := Decision{
		CandidateID: labels[selected.ChannelID], DominantCandidate: labels[dominant.ChannelID], Probe: selected.Probe,
	}
	for _, channelID := range decision.DegradedCandidates {
		result.DegradedCandidates = append(result.DegradedCandidates, labels[channelID])
	}
	for _, channelID := range decision.EmergencyCandidates {
		result.EmergencyCandidates = append(result.EmergencyCandidates, labels[channelID])
	}
	for _, channelID := range decision.UnverifiedCandidates {
		result.UnverifiedCandidates = append(result.UnverifiedCandidates, labels[channelID])
	}
	return result
}

func (p *DynamicPolicy) Observe(sample Sample) {
	channelID, ok := p.channelIDs[sample.CandidateID]
	if !ok {
		return
	}
	p.controller.Observe(dynamicrouting.ObservationKey{ChannelID: channelID, Model: p.routeKey.Model}, dynamicrouting.Sample{
		ObservedAt:        p.epoch.Add(sample.CompletedAt),
		UpstreamStartedAt: p.epoch.Add(sample.ArrivedAt),
		TTFT:              sample.TTFT,
		TPOT:              sample.TPOT,
		HasTTFT:           sample.Success && sample.TTFT > 0,
		HasTPOT:           sample.Success && sample.TPOT > 0,
		Success:           sample.Success,
		HardFailure: sample.Failure == FailureHard ||
			sample.Failure == FailureHTTP429 || sample.Failure == FailureHTTP503,
	})
}
