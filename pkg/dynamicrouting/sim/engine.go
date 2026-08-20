package sim

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

func Run(scenario Scenario, policy Policy) (Result, error) {
	if policy == nil {
		return Result{}, fmt.Errorf("policy is required")
	}
	if scenario.OutputTokens < 1 {
		return Result{}, fmt.Errorf("output tokens must be positive")
	}

	channels := make(map[string]Channel, len(scenario.Channels))
	candidates := make([]Candidate, 0, len(scenario.Channels))
	slots := make(map[string][]time.Duration, len(scenario.Channels))
	for _, channel := range scenario.Channels {
		if channel.ID == "" || channel.Concurrency < 1 || len(channel.Timeline) == 0 {
			return Result{}, fmt.Errorf("channel %q is invalid", channel.ID)
		}
		channels[channel.ID] = channel
		candidates = append(candidates, Candidate{ID: channel.ID, ChannelID: channel.ChannelID, Priority: channel.Priority, Weight: channel.Weight})
		slots[channel.ID] = make([]time.Duration, channel.Concurrency)
	}

	result := Result{Scenario: scenario.Name, Requests: make([]RequestResult, 0, len(scenario.Arrivals))}
	random := rand.New(rand.NewSource(scenario.Seed))
	pending := make([]Sample, 0, len(scenario.Arrivals))
	for _, arrivedAt := range scenario.Arrivals {
		sort.SliceStable(pending, func(i, j int) bool { return pending[i].CompletedAt < pending[j].CompletedAt })
		observed := 0
		for observed < len(pending) && pending[observed].CompletedAt <= arrivedAt {
			policy.Observe(pending[observed])
			observed++
		}
		pending = pending[observed:]

		decision := policy.Select(arrivedAt, candidates)
		channel, ok := channels[decision.CandidateID]
		if !ok {
			return Result{}, fmt.Errorf("policy selected unknown channel %q", decision.CandidateID)
		}
		phase := channel.Timeline[0]
		for _, candidatePhase := range channel.Timeline[1:] {
			if candidatePhase.Start > arrivedAt {
				break
			}
			phase = candidatePhase
		}

		channelSlots := slots[channel.ID]
		slotIndex := 0
		for i := 1; i < len(channelSlots); i++ {
			if channelSlots[i] < channelSlots[slotIndex] {
				slotIndex = i
			}
		}
		startedAt := arrivedAt
		if channelSlots[slotIndex] > startedAt {
			startedAt = channelSlots[slotIndex]
		}
		failure := chooseFailure(random.Float64(), phase)
		ttftBase := phase.TTFT
		if failure == FailureNone && random.Float64() < phase.LongTailRate {
			ttftBase += phase.LongTailDelay
		}
		ttftBase = addJitter(random, ttftBase, phase.TTFTJitter)
		tpot := addJitter(random, phase.TPOT, phase.TPOTJitter)
		serviceDuration := ttftBase
		if failure == FailureNone {
			serviceDuration += tpot * time.Duration(scenario.OutputTokens-1)
		}
		completedAt := startedAt + serviceDuration
		channelSlots[slotIndex] = completedAt
		slots[channel.ID] = channelSlots
		ttft := startedAt - arrivedAt + ttftBase
		success := failure == FailureNone
		request := RequestResult{
			ArrivedAt: arrivedAt, StartedAt: startedAt, CompletedAt: completedAt,
			ChannelID: channel.ID, DominantCandidate: decision.DominantCandidate,
			TTFT: ttft, TPOT: tpot, Success: success, Failure: failure, Probe: decision.Probe,
			DegradedCandidates:   append([]string(nil), decision.DegradedCandidates...),
			EmergencyCandidates:  append([]string(nil), decision.EmergencyCandidates...),
			UnverifiedCandidates: append([]string(nil), decision.UnverifiedCandidates...),
		}
		result.Requests = append(result.Requests, request)
		pending = append(pending, Sample{
			CandidateID: channel.ID, ArrivedAt: arrivedAt, CompletedAt: completedAt,
			TTFT: ttft, TPOT: tpot, Success: success, Failure: failure,
		})
	}
	for _, sample := range pending {
		policy.Observe(sample)
	}
	result.Metrics = calculateMetrics(result.Requests, scenario)
	return result, nil
}

func addJitter(random *rand.Rand, base, maximum time.Duration) time.Duration {
	if maximum <= 0 {
		return base
	}
	value := base + time.Duration(random.Int63n(2*int64(maximum)+1)-int64(maximum))
	if value < 0 {
		return 0
	}
	return value
}

func chooseFailure(roll float64, phase Phase) FailureKind {
	if roll < phase.HardFailureRate {
		return FailureHard
	}
	roll -= phase.HardFailureRate
	if roll < phase.HTTP429Rate {
		return FailureHTTP429
	}
	roll -= phase.HTTP429Rate
	if roll < phase.HTTP503Rate {
		return FailureHTTP503
	}
	return FailureNone
}

func calculateMetrics(requests []RequestResult, scenario Scenario) Metrics {
	ttfts := make([]time.Duration, 0, len(requests))
	tpots := make([]time.Duration, 0, len(requests))
	metrics := Metrics{TotalRequests: len(requests), DetectionDelay: -1, MitigationDelay: -1}
	badChannels := make(map[string]struct{}, len(scenario.Fault.BadChannels))
	for _, channelID := range scenario.Fault.BadChannels {
		badChannels[channelID] = struct{}{}
	}
	violations := 0
	probeCount := 0
	var firstArrival time.Duration
	var finalCompletion time.Duration
	if len(requests) > 0 {
		firstArrival = requests[0].ArrivedAt
		finalCompletion = requests[0].CompletedAt
	}
	for _, request := range requests {
		if request.ArrivedAt < firstArrival {
			firstArrival = request.ArrivedAt
		}
		if request.CompletedAt > finalCompletion {
			finalCompletion = request.CompletedAt
		}
		if request.Probe {
			probeCount++
		}
		if request.TTFT > 0 {
			ttfts = append(ttfts, request.TTFT)
		}
		if !request.Success {
			violations++
			metrics.SLOViolationArea += 2
			continue
		}
		metrics.Successes++
		tpots = append(tpots, request.TPOT)
		violated := false
		if scenario.SLO.TTFT > 0 && request.TTFT > scenario.SLO.TTFT {
			violated = true
			metrics.SLOViolationArea += float64(request.TTFT-scenario.SLO.TTFT) / float64(scenario.SLO.TTFT)
		}
		if scenario.SLO.TPOT > 0 && request.TPOT > scenario.SLO.TPOT {
			violated = true
			metrics.SLOViolationArea += float64(request.TPOT-scenario.SLO.TPOT) / float64(scenario.SLO.TPOT)
		}
		if violated {
			violations++
		}
	}
	metrics.P95TTFT = percentile95(ttfts)
	metrics.P95TPOT = percentile95(tpots)
	if len(requests) > 0 {
		metrics.SuccessRate = float64(metrics.Successes) / float64(len(requests))
		metrics.SLOViolationRate = float64(violations) / float64(len(requests))
		metrics.SLOViolationArea /= float64(len(requests))
		metrics.ProbeCost = float64(probeCount) / float64(len(requests))
		duration := finalCompletion - firstArrival
		if duration > 0 {
			metrics.ThroughputPerSecond = float64(metrics.Successes) / duration.Seconds()
		}
	}
	calculateFaultMetrics(requests, scenario.Fault, badChannels, &metrics)
	metrics.RouteReversals = countRouteReversals(requests)
	return metrics
}

func calculateFaultMetrics(requests []RequestResult, fault FaultSpec, badChannels map[string]struct{}, metrics *Metrics) {
	if len(badChannels) == 0 {
		return
	}
	postFault := make([]RequestResult, 0, len(requests))
	for _, request := range requests {
		if request.ArrivedAt < fault.At {
			continue
		}
		postFault = append(postFault, request)
		if _, bad := badChannels[request.ChannelID]; bad {
			metrics.BadChannelExposureAfterFault++
		}
		if metrics.DetectionDelay < 0 {
			for _, degraded := range request.DegradedCandidates {
				if _, bad := badChannels[degraded]; bad {
					metrics.DetectionDelay = request.ArrivedAt - fault.At
					break
				}
			}
		}
	}
	if metrics.DetectionDelay >= 0 {
		detectedAt := fault.At + metrics.DetectionDelay
		for _, request := range requests {
			if request.CompletedAt < fault.At || request.CompletedAt > detectedAt {
				continue
			}
			if _, bad := badChannels[request.ChannelID]; bad {
				metrics.DetectionObservations++
			}
		}
		for _, request := range requests {
			if request.ArrivedAt < detectedAt {
				continue
			}
			if _, bad := badChannels[request.ChannelID]; bad {
				metrics.BadChannelExposureAfterDetection++
			}
		}
	}
	window := fault.MitigationWindow
	if window <= 0 {
		window = 10
	}
	for i := window - 1; i < len(postFault); i++ {
		badCount := 0
		for _, request := range postFault[i-window+1 : i+1] {
			if _, bad := badChannels[request.ChannelID]; bad {
				badCount++
			}
		}
		if float64(badCount)/float64(window) < fault.MitigatedBadShare {
			metrics.MitigationDelay = postFault[i].ArrivedAt - fault.At
			break
		}
	}
}

func countRouteReversals(requests []RequestResult) int {
	routes := make([]string, 0, len(requests))
	for _, request := range requests {
		route := request.DominantCandidate
		if route == "" {
			route = request.ChannelID
		}
		if len(routes) == 0 || routes[len(routes)-1] != route {
			routes = append(routes, route)
		}
	}
	reversals := 0
	for i := 2; i < len(routes); i++ {
		if routes[i] == routes[i-2] {
			reversals++
		}
	}
	return reversals
}

func percentile95(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (95*len(sorted) + 99) / 100
	return sorted[index-1]
}
