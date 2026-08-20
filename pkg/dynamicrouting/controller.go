package dynamicrouting

import (
	"errors"
	"math"
	"sort"
	"sync"
	"time"
)

type ObservationKey struct {
	ChannelID int
	Model     string
}

type Config struct {
	Enabled       bool
	MaxSamples    int
	MaxAge        time.Duration
	MinSamples    int
	ProbeFraction float64

	DegradationThreshold float64
	RecoveryThreshold    float64
	CriticalThreshold    float64
	CandidateAdvantage   float64
	Aggressiveness       float64
	RecoveryStep         float64
	Cooldown             time.Duration
	HardFailureThreshold int
	HardFailureCooldown  time.Duration
}

type RouteKey struct {
	Group string
	Model string
}

type Candidate struct {
	ChannelID int
	Priority  int64
	Weight    uint
}

type Allocation struct {
	ChannelID int
	Share     float64
	Probe     bool
}

type Decision struct {
	Dynamic              bool
	Allocations          []Allocation
	SelectedChannelID    int
	HasSelection         bool
	DegradedCandidates   []int
	EmergencyCandidates  []int
	UnverifiedCandidates []int
}

type Sample struct {
	ObservedAt        time.Time
	UpstreamStartedAt time.Time
	TTFT              time.Duration
	TPOT              time.Duration
	HasTTFT           bool
	HasTPOT           bool
	Success           bool
	HardFailure       bool
}

type ObservationStats struct {
	SampleCount     int
	TTFTSampleCount int
	TPOTSampleCount int
	TTFT            time.Duration
	TPOT            time.Duration
	HasTTFT         bool
	HasTPOT         bool
}

type Controller struct {
	mu                 sync.Mutex
	config             Config
	observations       map[ObservationKey][]Sample
	healthObservations map[ObservationKey][]Sample
	performanceAfter   map[ObservationKey]time.Time
	routes             map[RouteKey]*routeState
	selections         map[RouteKey]*selectionState
	lastCleanup        time.Time
}

type selectionState struct {
	debts      map[int]float64
	lastAccess time.Time
}

type routeState struct {
	shares      map[int]float64
	probes      map[int]bool
	candidates  map[int]candidateFingerprint
	degraded    map[int]bool
	ejections   map[int]hardEjection
	lastChanged time.Time
	lastAccess  time.Time
}

type candidateFingerprint struct {
	priority int64
	weight   uint
}

type hardEjection struct {
	lastHardAt    time.Time
	ejectedUntil  time.Time
	lastEligible  time.Time
	probeDebt     float64
	probeIssuedAt []time.Time
}

type channelMeasurement struct {
	verified   bool
	ttft       time.Duration
	tpot       time.Duration
	hasTTFT    bool
	hasTPOT    bool
	ratio      float64
	hardFail   bool
	lastHardAt time.Time
}

func NewController(config Config) (*Controller, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	return &Controller{
		config:             normalized,
		observations:       make(map[ObservationKey][]Sample),
		healthObservations: make(map[ObservationKey][]Sample),
		performanceAfter:   make(map[ObservationKey]time.Time),
		routes:             make(map[RouteKey]*routeState),
		selections:         make(map[RouteKey]*selectionState),
	}, nil
}

func normalizeConfig(config Config) (Config, error) {
	if config.MaxSamples <= 0 {
		return Config{}, errors.New("max samples must be positive")
	}
	if config.MaxAge <= 0 {
		return Config{}, errors.New("max age must be positive")
	}
	if config.MinSamples < 0 {
		return Config{}, errors.New("min samples cannot be negative")
	}
	if config.MinSamples == 0 {
		config.MinSamples = 3
		if config.MaxSamples < config.MinSamples {
			config.MinSamples = config.MaxSamples
		}
	}
	if config.MinSamples > config.MaxSamples {
		return Config{}, errors.New("min samples cannot exceed max samples")
	}
	if !finiteFloat(config.ProbeFraction) {
		return Config{}, errors.New("probe fraction must be finite")
	}
	if config.ProbeFraction < 0 || config.ProbeFraction >= 1 {
		return Config{}, errors.New("probe fraction must be in [0, 1)")
	}
	if config.Enabled && config.ProbeFraction == 0 {
		return Config{}, errors.New("probe fraction must be positive when dynamic routing is enabled")
	}
	if config.DegradationThreshold == 0 {
		config.DegradationThreshold = 1.4
	}
	if !finiteFloat(config.DegradationThreshold) {
		return Config{}, errors.New("degradation threshold must be finite")
	}
	if config.DegradationThreshold <= 1 {
		return Config{}, errors.New("degradation threshold must be greater than 1")
	}
	if config.RecoveryThreshold == 0 {
		config.RecoveryThreshold = 1.15
	}
	if !finiteFloat(config.RecoveryThreshold) {
		return Config{}, errors.New("recovery threshold must be finite")
	}
	if config.RecoveryThreshold <= 1 || config.RecoveryThreshold >= config.DegradationThreshold {
		return Config{}, errors.New("recovery threshold must be greater than 1 and below degradation threshold")
	}
	if config.CriticalThreshold == 0 {
		config.CriticalThreshold = 2
	}
	if !finiteFloat(config.CriticalThreshold) {
		return Config{}, errors.New("critical threshold must be finite")
	}
	if config.CriticalThreshold <= config.DegradationThreshold {
		return Config{}, errors.New("critical threshold must exceed degradation threshold")
	}
	if config.CandidateAdvantage == 0 {
		config.CandidateAdvantage = 1.1
	}
	if !finiteFloat(config.CandidateAdvantage) {
		return Config{}, errors.New("candidate advantage must be finite")
	}
	if config.CandidateAdvantage <= 1 {
		return Config{}, errors.New("candidate advantage must be greater than 1")
	}
	if config.Aggressiveness == 0 {
		config.Aggressiveness = 0.8
	}
	if !finiteFloat(config.Aggressiveness) {
		return Config{}, errors.New("aggressiveness must be finite")
	}
	if config.Aggressiveness <= 0 || config.Aggressiveness > 1 {
		return Config{}, errors.New("aggressiveness must be in (0, 1]")
	}
	if config.RecoveryStep == 0 {
		config.RecoveryStep = 0.05
	}
	if !finiteFloat(config.RecoveryStep) {
		return Config{}, errors.New("recovery step must be finite")
	}
	if config.RecoveryStep <= 0 || config.RecoveryStep > 1 {
		return Config{}, errors.New("recovery step must be in (0, 1]")
	}
	if config.Cooldown < 0 {
		return Config{}, errors.New("cooldown cannot be negative")
	}
	if config.HardFailureThreshold < 0 {
		return Config{}, errors.New("hard failure threshold cannot be negative")
	}
	if config.HardFailureThreshold == 0 {
		config.HardFailureThreshold = 1
	}
	if config.HardFailureThreshold > config.MaxSamples {
		return Config{}, errors.New("hard failure threshold cannot exceed max samples")
	}
	if config.HardFailureCooldown == 0 {
		config.HardFailureCooldown = 30 * time.Second
	}
	if config.HardFailureCooldown <= 0 {
		return Config{}, errors.New("hard failure cooldown must be positive")
	}
	return config, nil
}

func finiteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (c *Controller) UpdateConfig(config Config) error {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	routingChanged := c.config
	routingChanged.Enabled = normalized.Enabled
	if routingChanged != normalized {
		preservedEjections := make(map[RouteKey]*routeState)
		for key, state := range c.routes {
			if len(state.ejections) == 0 {
				continue
			}
			ejections := make(map[int]hardEjection, len(state.ejections))
			for channelID, ejection := range state.ejections {
				ejections[channelID] = ejection
			}
			preservedEjections[key] = &routeState{
				shares:     make(map[int]float64),
				probes:     make(map[int]bool),
				candidates: make(map[int]candidateFingerprint),
				degraded:   make(map[int]bool),
				ejections:  ejections,
				lastAccess: state.lastAccess,
			}
		}
		c.routes = preservedEjections
		c.selections = make(map[RouteKey]*selectionState)
	}
	c.config = normalized
	c.lastCleanup = time.Time{}
	for key, samples := range c.observations {
		if len(samples) > normalized.MaxSamples {
			c.observations[key] = append([]Sample(nil), samples[len(samples)-normalized.MaxSamples:]...)
		}
	}
	for key, samples := range c.healthObservations {
		if len(samples) > normalized.MaxSamples {
			c.healthObservations[key] = append([]Sample(nil), samples[len(samples)-normalized.MaxSamples:]...)
		}
	}
	return nil
}

func (c *Controller) Select(key RouteKey, candidates []Candidate, now time.Time) Decision {
	return c.selectAvoiding(key, candidates, nil, now)
}

// SelectAvoiding makes a routing decision without selecting any channel in
// excludedChannelIDs. The complete candidate set still participates in route
// health, allocation, and weighted-fair accounting.
func (c *Controller) SelectAvoiding(
	key RouteKey,
	candidates []Candidate,
	excludedChannelIDs map[int]struct{},
	now time.Time,
) Decision {
	return c.selectAvoiding(key, candidates, excludedChannelIDs, now)
}

func (c *Controller) selectAvoiding(
	key RouteKey,
	candidates []Candidate,
	excludedChannelIDs map[int]struct{},
	now time.Time,
) Decision {
	c.mu.Lock()
	defer c.mu.Unlock()

	decision := Decision{Dynamic: c.config.Enabled}
	c.cleanupExpiredLocked(now)
	cutoff := now.Add(-c.config.MaxAge)
	if state, exists := c.routes[key]; exists && !state.lastAccess.IsZero() && state.lastAccess.Before(cutoff) {
		delete(c.routes, key)
		delete(c.selections, key)
	}
	if len(candidates) == 0 {
		delete(c.selections, key)
		return decision
	}

	ordered := deduplicateCandidates(candidates)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority > ordered[j].Priority
		}
		return ordered[i].ChannelID < ordered[j].ChannelID
	})

	highestPriority := ordered[0].Priority
	mainCandidates := make([]Candidate, 0, len(ordered))
	probeCandidates := make([]Candidate, 0, len(ordered))
	for _, candidate := range ordered {
		if candidate.Priority == highestPriority {
			mainCandidates = append(mainCandidates, candidate)
		} else {
			probeCandidates = append(probeCandidates, candidate)
		}
	}

	probeShare := c.config.ProbeFraction
	if !c.config.Enabled || len(probeCandidates) == 0 {
		probeShare = 0
	}
	initial := append(allocateByWeight(mainCandidates, 1-probeShare, false), allocateByWeight(probeCandidates, probeShare, true)...)
	if !c.config.Enabled {
		decision.Allocations = initial
		return decision
	}

	state, exists := c.routes[key]
	if exists && len(state.ejections) > 0 {
		present := make(map[int]struct{}, len(ordered))
		for _, candidate := range ordered {
			present[candidate.ChannelID] = struct{}{}
		}
		for channelID, ejection := range state.ejections {
			if _, stillEligible := present[channelID]; stillEligible {
				continue
			}
			retentionStart := ejection.ejectedUntil
			if ejection.lastEligible.After(retentionStart) {
				retentionStart = ejection.lastEligible
			}
			if now.After(retentionStart.Add(c.config.MaxAge)) {
				delete(state.ejections, channelID)
			}
		}
	}
	if !exists || !candidateFingerprintMatches(state, ordered) {
		ejections := make(map[int]hardEjection, len(ordered))
		if exists {
			for channelID, ejection := range state.ejections {
				ejections[channelID] = ejection
			}
		}
		state = &routeState{
			shares:     make(map[int]float64, len(initial)),
			probes:     make(map[int]bool, len(initial)),
			candidates: make(map[int]candidateFingerprint, len(ordered)),
			degraded:   make(map[int]bool, len(ordered)),
			ejections:  ejections,
			lastAccess: now,
		}
		for _, allocation := range initial {
			state.shares[allocation.ChannelID] = allocation.Share
			state.probes[allocation.ChannelID] = allocation.Probe
		}
		for _, candidate := range ordered {
			state.candidates[candidate.ChannelID] = candidateFingerprint{
				priority: candidate.Priority,
				weight:   candidate.Weight,
			}
		}
		c.routes[key] = state
		delete(c.selections, key)
	}
	state.lastAccess = now

	measurements := make(map[int]channelMeasurement, len(ordered))
	for _, candidate := range ordered {
		measurements[candidate.ChannelID] = c.measureChannelLocked(ObservationKey{
			ChannelID: candidate.ChannelID,
			Model:     key.Model,
		}, now)
	}
	cooldownEjected := make(map[int]bool, len(ordered))
	probeEjected := make(map[int]bool, len(ordered))
	ejectionTransition := false
	for _, candidate := range ordered {
		measurement := measurements[candidate.ChannelID]
		ejection, ejected := state.ejections[candidate.ChannelID]
		observationKey := ObservationKey{ChannelID: candidate.ChannelID, Model: key.Model}
		if measurement.hardFail && (!ejected || measurement.lastHardAt.After(ejection.lastHardAt)) {
			ejection = hardEjection{
				lastHardAt:   measurement.lastHardAt,
				ejectedUntil: measurement.lastHardAt.Add(c.config.HardFailureCooldown),
				lastEligible: now,
			}
			state.ejections[candidate.ChannelID] = ejection
			if cutoff := c.performanceAfter[observationKey]; measurement.lastHardAt.After(cutoff) {
				c.performanceAfter[observationKey] = measurement.lastHardAt
			}
			ejected = true
			ejectionTransition = true
		}
		if ejected && !now.Before(ejection.ejectedUntil) &&
			c.recoverySuccessStreakLocked(observationKey, ejection, now) >= c.config.MinSamples {
			delete(state.ejections, candidate.ChannelID)
			delete(state.degraded, candidate.ChannelID)
			ejected = false
			ejectionTransition = true
		}
		if ejected {
			ejection.lastEligible = now
			state.ejections[candidate.ChannelID] = ejection
			if now.Before(ejection.ejectedUntil) {
				cooldownEjected[candidate.ChannelID] = true
			} else {
				probeEjected[candidate.ChannelID] = true
			}
		}
		measurement.hardFail = cooldownEjected[candidate.ChannelID]
		measurements[candidate.ChannelID] = measurement
	}
	ejectionProbeFloors := probeFloorsForCandidates(ordered, probeEjected, c.config.ProbeFraction)
	ejectionProbeChanged := capEjectedProbeShares(state, ordered, ejectionProbeFloors, cooldownEjected)
	if ejectionProbeChanged {
		ejectionTransition = true
	}
	emergency := false
	degraded := make(map[int]bool, len(ordered))
	effectiveRatios := make(map[int]float64, len(ordered))
	for _, candidate := range ordered {
		measurement := measurements[candidate.ChannelID]
		if !measurement.verified {
			decision.UnverifiedCandidates = append(decision.UnverifiedCandidates, candidate.ChannelID)
		}
		if probeEjected[candidate.ChannelID] {
			degraded[candidate.ChannelID] = true
			state.degraded[candidate.ChannelID] = true
			effectiveRatios[candidate.ChannelID] = c.config.CriticalThreshold
			decision.DegradedCandidates = append(decision.DegradedCandidates, candidate.ChannelID)
			continue
		}
		if measurement.hardFail {
			emergency = true
			degraded[candidate.ChannelID] = true
			state.degraded[candidate.ChannelID] = true
			effectiveRatios[candidate.ChannelID] = c.config.CriticalThreshold
			decision.EmergencyCandidates = append(decision.EmergencyCandidates, candidate.ChannelID)
			decision.DegradedCandidates = append(decision.DegradedCandidates, candidate.ChannelID)
			continue
		}

		effectiveRatio := measurement.ratio
		for _, possible := range ordered {
			if possible.ChannelID == candidate.ChannelID {
				continue
			}
			possibleMeasurement := measurements[possible.ChannelID]
			if !possibleMeasurement.verified || possibleMeasurement.hardFail {
				continue
			}
			if gap, better := performanceGap(measurement, possibleMeasurement, c.config.CandidateAdvantage); better && gap > effectiveRatio {
				effectiveRatio = gap
			}
		}
		effectiveRatios[candidate.ChannelID] = effectiveRatio
		isDegraded := effectiveRatio >= c.config.DegradationThreshold
		if state.degraded[candidate.ChannelID] && (!measurement.verified || effectiveRatio > c.config.RecoveryThreshold) {
			isDegraded = true
		}
		if isDegraded {
			degraded[candidate.ChannelID] = true
			state.degraded[candidate.ChannelID] = true
			decision.DegradedCandidates = append(decision.DegradedCandidates, candidate.ChannelID)
		} else {
			delete(state.degraded, candidate.ChannelID)
		}
	}
	if !emergency && !ejectionTransition && !state.lastChanged.IsZero() && now.Sub(state.lastChanged) < c.config.Cooldown {
		decision.Allocations = allocationsFromState(ordered, state)
		return c.completeDecisionLocked(key, decision, excludedChannelIDs, now)
	}

	changed := ejectionProbeChanged
	probeFloors := make(map[int]float64, len(initial))
	for _, allocation := range initial {
		if allocation.Probe {
			probeFloors[allocation.ChannelID] = allocation.Share
		}
	}
	for channelID, floor := range ejectionProbeFloors {
		if floor > probeFloors[channelID] {
			probeFloors[channelID] = floor
		}
	}
	for _, candidate := range ordered {
		measurement := measurements[candidate.ChannelID]
		if !degraded[candidate.ChannelID] {
			continue
		}

		receivers := make([]Candidate, 0, len(ordered)-1)
		for _, possible := range ordered {
			if possible.ChannelID == candidate.ChannelID {
				continue
			}
			possibleMeasurement := measurements[possible.ChannelID]
			if !possibleMeasurement.verified || possibleMeasurement.hardFail || degraded[possible.ChannelID] || probeEjected[possible.ChannelID] {
				continue
			}
			if measurement.hardFail || candidateOutperforms(measurement, possibleMeasurement, c.config.CandidateAdvantage) {
				receivers = append(receivers, possible)
			}
		}
		if measurement.hardFail && len(receivers) == 0 {
			fallbackPriority := int64(0)
			hasFallbackPriority := false
			for _, possible := range ordered {
				if possible.ChannelID == candidate.ChannelID || measurements[possible.ChannelID].hardFail || probeEjected[possible.ChannelID] {
					continue
				}
				if !hasFallbackPriority || possible.Priority > fallbackPriority {
					fallbackPriority = possible.Priority
					receivers = receivers[:0]
					hasFallbackPriority = true
				}
				if possible.Priority == fallbackPriority {
					receivers = append(receivers, possible)
				}
			}
		}
		if len(receivers) == 0 {
			continue
		}

		unload := 1.0
		if !measurement.hardFail {
			unload = c.config.Aggressiveness
			severity := (effectiveRatios[candidate.ChannelID] - c.config.DegradationThreshold) /
				(c.config.CriticalThreshold - c.config.DegradationThreshold)
			if severity > 1 {
				severity = 1
			}
			unload = 1 - (1-unload)*(1-unload*severity)
		}
		movableShare := state.shares[candidate.ChannelID]
		if !measurement.hardFail && state.probes[candidate.ChannelID] {
			movableShare -= probeFloors[candidate.ChannelID]
		}
		if movableShare < 0 {
			movableShare = 0
		}
		moved := movableShare * unload
		if moved <= 0 {
			continue
		}
		state.shares[candidate.ChannelID] -= moved
		for _, allocation := range allocateByWeight(receivers, moved, false) {
			state.shares[allocation.ChannelID] += allocation.Share
			state.probes[allocation.ChannelID] = false
		}
		changed = true
	}
	if changed {
		state.lastChanged = now
	} else if len(degraded) == 0 && recoverRouteShares(state, initial, c.config.RecoveryStep) {
		state.lastChanged = now
	}
	decision.Allocations = allocationsFromState(ordered, state)
	return c.completeDecisionLocked(key, decision, excludedChannelIDs, now)
}

func (c *Controller) cleanupExpiredLocked(now time.Time) {
	cleanupInterval := c.config.MaxAge / 4
	if cleanupInterval > time.Minute {
		cleanupInterval = time.Minute
	}
	if cleanupInterval <= 0 {
		cleanupInterval = time.Millisecond
	}
	if !c.lastCleanup.IsZero() {
		if now.Before(c.lastCleanup) || now.Sub(c.lastCleanup) < cleanupInterval {
			return
		}
	}
	cutoff := now.Add(-c.config.MaxAge)
	for observationKey, samples := range c.observations {
		if len(samples) == 0 || samples[len(samples)-1].ObservedAt.Before(cutoff) {
			delete(c.observations, observationKey)
		}
	}
	for observationKey, samples := range c.healthObservations {
		if len(samples) == 0 || samples[len(samples)-1].ObservedAt.Before(cutoff) {
			delete(c.healthObservations, observationKey)
		}
	}
	for observationKey, resetAt := range c.performanceAfter {
		if resetAt.Before(cutoff) {
			delete(c.performanceAfter, observationKey)
		}
	}
	for routeKey, state := range c.routes {
		if !state.lastAccess.IsZero() && state.lastAccess.Before(cutoff) {
			delete(c.routes, routeKey)
		}
	}
	for routeKey, state := range c.selections {
		if !state.lastAccess.IsZero() && state.lastAccess.Before(cutoff) {
			delete(c.selections, routeKey)
		}
	}
	c.lastCleanup = now
}

func deduplicateCandidates(candidates []Candidate) []Candidate {
	byChannel := make(map[int]Candidate, len(candidates))
	for _, candidate := range candidates {
		existing, exists := byChannel[candidate.ChannelID]
		if !exists || candidate.Priority > existing.Priority ||
			(candidate.Priority == existing.Priority && candidate.Weight > existing.Weight) {
			byChannel[candidate.ChannelID] = candidate
		}
	}
	deduplicated := make([]Candidate, 0, len(byChannel))
	for _, candidate := range byChannel {
		deduplicated = append(deduplicated, candidate)
	}
	return deduplicated
}

func (c *Controller) completeDecisionLocked(
	key RouteKey,
	decision Decision,
	excludedChannelIDs map[int]struct{},
	now time.Time,
) Decision {
	if len(decision.Allocations) == 0 {
		delete(c.selections, key)
		return decision
	}
	hasSelectableAllocation := false
	for _, allocation := range decision.Allocations {
		if allocation.Share <= 0 {
			continue
		}
		if _, excluded := excludedChannelIDs[allocation.ChannelID]; !excluded {
			hasSelectableAllocation = true
			break
		}
	}
	if !hasSelectableAllocation {
		return decision
	}
	forcedChannelID := 0
	forcedDebt := 0.0
	hasForcedProbe := false
	route := c.routes[key]
	if route != nil {
		for _, allocation := range decision.Allocations {
			ejection, ejected := route.ejections[allocation.ChannelID]
			if !ejected || now.Before(ejection.ejectedUntil) || !allocation.Probe {
				continue
			}
			ejection.probeDebt += allocation.Share
			route.ejections[allocation.ChannelID] = ejection
			if _, excluded := excludedChannelIDs[allocation.ChannelID]; excluded {
				continue
			}
			if ejection.probeDebt >= 1-1e-12 && (!hasForcedProbe || ejection.probeDebt > forcedDebt+1e-12) {
				forcedChannelID = allocation.ChannelID
				forcedDebt = ejection.probeDebt
				hasForcedProbe = true
			}
		}
	}

	state, exists := c.selections[key]
	if !exists {
		state = &selectionState{debts: make(map[int]float64, len(decision.Allocations))}
		c.selections[key] = state
	}
	active := make(map[int]struct{}, len(decision.Allocations))
	selected := 0
	selectedDebt := 0.0
	hasSelection := false
	for _, allocation := range decision.Allocations {
		active[allocation.ChannelID] = struct{}{}
		state.debts[allocation.ChannelID] += allocation.Share
		if _, excluded := excludedChannelIDs[allocation.ChannelID]; excluded {
			continue
		}
		debt := state.debts[allocation.ChannelID]
		if !hasSelection || debt > selectedDebt+1e-12 {
			selected = allocation.ChannelID
			selectedDebt = debt
			hasSelection = true
		}
	}
	if hasForcedProbe {
		selected = forcedChannelID
		hasSelection = true
	}
	for channelID := range state.debts {
		if _, exists := active[channelID]; !exists {
			delete(state.debts, channelID)
		}
	}
	if hasSelection {
		state.debts[selected]--
	}
	state.lastAccess = now
	if route != nil && hasSelection {
		selectedEjectionProbe := false
		for _, allocation := range decision.Allocations {
			if allocation.ChannelID == selected {
				selectedEjectionProbe = allocation.Probe
				break
			}
		}
		if ejection, ejected := route.ejections[selected]; ejected && selectedEjectionProbe && !now.Before(ejection.ejectedUntil) {
			ejection.probeDebt = 0
			ejection.probeIssuedAt = append(ejection.probeIssuedAt, now)
			if len(ejection.probeIssuedAt) > c.config.MaxSamples {
				ejection.probeIssuedAt = append([]time.Time(nil), ejection.probeIssuedAt[len(ejection.probeIssuedAt)-c.config.MaxSamples:]...)
			}
			route.ejections[selected] = ejection
		}
	}
	decision.SelectedChannelID = selected
	decision.HasSelection = hasSelection
	return decision
}

func (c *Controller) measureChannelLocked(key ObservationKey, now time.Time) channelMeasurement {
	samples := c.freshSamplesLocked(key, now)
	if cutoff := c.performanceAfter[key]; !cutoff.IsZero() {
		firstAfterCutoff := sort.Search(len(samples), func(index int) bool {
			return samples[index].ObservedAt.After(cutoff)
		})
		samples = samples[firstAfterCutoff:]
	}
	healthSamples := c.freshHealthSamplesLocked(key, now)
	measurement := channelMeasurement{}
	consecutiveHardFailures := 0
	lastHardAt := time.Time{}
	for index := len(healthSamples) - 1; index >= 0; index-- {
		sample := healthSamples[index]
		if sample.HardFailure {
			if lastHardAt.IsZero() {
				lastHardAt = sample.ObservedAt
			}
			consecutiveHardFailures++
			continue
		}
		if sample.Success {
			break
		}
	}
	measurement.hardFail = consecutiveHardFailures >= c.config.HardFailureThreshold
	if measurement.hardFail {
		measurement.lastHardAt = lastHardAt
	}
	ttftSamples := 0
	tpotSamples := 0
	for _, sample := range samples {
		if sample.Success && !sample.HardFailure {
			if sample.HasTTFT && sample.TTFT > 0 {
				ttftSamples++
			}
			if sample.HasTPOT && sample.TPOT > 0 {
				tpotSamples++
			}
		}
	}
	measurement.verified = ttftSamples >= c.config.MinSamples || tpotSamples >= c.config.MinSamples
	ttft, hasTTFT, ttftRatio := summarizeMetric(samples, c.config.MinSamples, func(sample Sample) (time.Duration, bool) {
		return sample.TTFT, sample.HasTTFT
	})
	tpot, hasTPOT, tpotRatio := summarizeMetric(samples, c.config.MinSamples, func(sample Sample) (time.Duration, bool) {
		return sample.TPOT, sample.HasTPOT
	})
	if ttftSamples >= c.config.MinSamples && hasTTFT {
		measurement.ttft = ttft
		measurement.hasTTFT = true
		measurement.ratio = ttftRatio
	}
	if tpotSamples >= c.config.MinSamples && hasTPOT {
		measurement.tpot = tpot
		measurement.hasTPOT = true
	}
	if measurement.hasTPOT && tpotRatio > measurement.ratio {
		measurement.ratio = tpotRatio
	}
	return measurement
}

func (c *Controller) recoverySuccessStreakLocked(key ObservationKey, ejection hardEjection, now time.Time) int {
	samples := c.freshHealthSamplesLocked(key, now)
	issuedProbes := append([]time.Time(nil), ejection.probeIssuedAt...)
	sort.Slice(issuedProbes, func(i, j int) bool { return issuedProbes[i].Before(issuedProbes[j]) })
	usedProbes := 0
	streak := 0
	for _, sample := range samples {
		if sample.ObservedAt.Before(ejection.ejectedUntil) {
			continue
		}
		if sample.ObservedAt.After(now) {
			break
		}
		startedAfterCooldown := !sample.UpstreamStartedAt.IsZero() &&
			!sample.UpstreamStartedAt.Before(ejection.ejectedUntil) &&
			!sample.UpstreamStartedAt.After(sample.ObservedAt)
		if !sample.Success || sample.HardFailure || !startedAfterCooldown {
			streak = 0
			continue
		}

		availableProbes := sort.Search(len(issuedProbes), func(index int) bool {
			return issuedProbes[index].After(sample.UpstreamStartedAt)
		})
		validMetric := (sample.HasTTFT && sample.TTFT > 0) || (sample.HasTPOT && sample.TPOT > 0)
		if !validMetric && availableProbes <= usedProbes {
			streak = 0
			continue
		}
		if availableProbes > usedProbes {
			usedProbes++
		}
		streak++
	}
	return streak
}

func (c *Controller) freshSamplesLocked(key ObservationKey, now time.Time) []Sample {
	return c.freshWindowSamplesLocked(c.observations, key, now)
}

func (c *Controller) freshHealthSamplesLocked(key ObservationKey, now time.Time) []Sample {
	return c.freshWindowSamplesLocked(c.healthObservations, key, now)
}

func (c *Controller) freshWindowSamplesLocked(window map[ObservationKey][]Sample, key ObservationKey, now time.Time) []Sample {
	samples := window[key]
	cutoff := now.Add(-c.config.MaxAge)
	firstFresh := 0
	for firstFresh < len(samples) && samples[firstFresh].ObservedAt.Before(cutoff) {
		firstFresh++
	}
	if firstFresh > 0 {
		samples = append([]Sample(nil), samples[firstFresh:]...)
		if len(samples) == 0 {
			delete(window, key)
		} else {
			window[key] = samples
		}
	}
	return samples
}

func summarizeMetric(samples []Sample, currentCount int, value func(Sample) (time.Duration, bool)) (time.Duration, bool, float64) {
	values := make([]time.Duration, 0, len(samples))
	for _, sample := range samples {
		metric, present := value(sample)
		if sample.Success && !sample.HardFailure && present && metric > 0 {
			values = append(values, metric)
		}
	}
	if len(values) == 0 {
		return 0, false, 0
	}
	if currentCount > len(values) {
		currentCount = len(values)
	}
	var currentTotal time.Duration
	for _, metric := range values[len(values)-currentCount:] {
		currentTotal += metric
	}
	current := currentTotal / time.Duration(currentCount)

	baselineValues := append([]time.Duration(nil), values...)
	sort.Slice(baselineValues, func(i, j int) bool { return baselineValues[i] < baselineValues[j] })
	baselineCount := len(baselineValues) / 2
	if baselineCount == 0 {
		baselineCount = 1
	}
	var baselineTotal time.Duration
	for _, metric := range baselineValues[:baselineCount] {
		baselineTotal += metric
	}
	baseline := baselineTotal / time.Duration(baselineCount)
	if baseline <= 0 {
		return current, true, 0
	}
	return current, true, float64(current) / float64(baseline)
}

func candidateOutperforms(current channelMeasurement, candidate channelMeasurement, advantage float64) bool {
	_, better := performanceGap(current, candidate, advantage)
	return better
}

func performanceGap(current channelMeasurement, candidate channelMeasurement, advantage float64) (float64, bool) {
	compared := false
	better := false
	gap := 0.0
	if current.hasTTFT && candidate.hasTTFT && candidate.ttft > 0 {
		compared = true
		ratio := float64(current.ttft) / float64(candidate.ttft)
		if ratio < 1/advantage {
			return 0, false
		}
		better = better || ratio >= advantage
		if ratio > gap {
			gap = ratio
		}
	}
	if current.hasTPOT && candidate.hasTPOT && candidate.tpot > 0 {
		compared = true
		ratio := float64(current.tpot) / float64(candidate.tpot)
		if ratio < 1/advantage {
			return 0, false
		}
		better = better || ratio >= advantage
		if ratio > gap {
			gap = ratio
		}
	}
	return gap, compared && better
}

func allocationsFromState(candidates []Candidate, state *routeState) []Allocation {
	allocations := make([]Allocation, 0, len(candidates))
	for _, candidate := range candidates {
		share := state.shares[candidate.ChannelID]
		if share <= 0 {
			continue
		}
		allocations = append(allocations, Allocation{
			ChannelID: candidate.ChannelID,
			Share:     share,
			Probe:     state.probes[candidate.ChannelID],
		})
	}
	return allocations
}

func candidateFingerprintMatches(state *routeState, candidates []Candidate) bool {
	if len(state.candidates) != len(candidates) {
		return false
	}
	for _, candidate := range candidates {
		fingerprint, exists := state.candidates[candidate.ChannelID]
		if !exists || fingerprint.priority != candidate.Priority || fingerprint.weight != candidate.Weight {
			return false
		}
	}
	return true
}

func probeFloorsForCandidates(candidates []Candidate, included map[int]bool, totalShare float64) map[int]float64 {
	probeCandidates := make([]Candidate, 0, len(included))
	for _, candidate := range candidates {
		if included[candidate.ChannelID] {
			probeCandidates = append(probeCandidates, candidate)
		}
	}
	floors := make(map[int]float64, len(probeCandidates))
	for _, allocation := range allocateByWeight(probeCandidates, totalShare, true) {
		floors[allocation.ChannelID] = allocation.Share
	}
	return floors
}

func capEjectedProbeShares(state *routeState, candidates []Candidate, targets map[int]float64, cooldownEjected map[int]bool) bool {
	if len(targets) == 0 {
		return false
	}
	hasSafeAlternative := false
	for _, candidate := range candidates {
		if _, ejected := targets[candidate.ChannelID]; !ejected && !cooldownEjected[candidate.ChannelID] {
			hasSafeAlternative = true
			break
		}
	}
	for channelID := range targets {
		state.probes[channelID] = true
	}
	if !hasSafeAlternative {
		return false
	}

	targetTotal := 0.0
	remainingCurrent := 0.0
	changed := false
	for _, candidate := range candidates {
		channelID := candidate.ChannelID
		if target, ejected := targets[channelID]; ejected {
			targetTotal += target
			if math.Abs(state.shares[channelID]-target) > 1e-9 {
				changed = true
			}
			continue
		}
		remainingCurrent += state.shares[channelID]
	}
	remainingTarget := 1 - targetTotal
	for channelID, target := range targets {
		state.shares[channelID] = target
	}
	if remainingCurrent > 1e-9 {
		scale := remainingTarget / remainingCurrent
		for _, candidate := range candidates {
			if _, ejected := targets[candidate.ChannelID]; !ejected {
				state.shares[candidate.ChannelID] *= scale
			}
		}
		return changed
	}

	for _, candidate := range candidates {
		if _, ejected := targets[candidate.ChannelID]; !ejected {
			state.shares[candidate.ChannelID] = 0
		}
	}
	safeCandidates := make([]Candidate, 0, len(candidates)-len(targets))
	for _, candidate := range candidates {
		if _, ejected := targets[candidate.ChannelID]; !ejected && !cooldownEjected[candidate.ChannelID] {
			safeCandidates = append(safeCandidates, candidate)
		}
	}
	for _, allocation := range allocateByWeight(safeCandidates, remainingTarget, false) {
		state.shares[allocation.ChannelID] = allocation.Share
	}
	return true
}

func recoverRouteShares(state *routeState, target []Allocation, step float64) bool {
	targetShares := make(map[int]float64, len(target))
	targetProbes := make(map[int]bool, len(target))
	deficitTotal := 0.0
	excessTotal := 0.0
	for _, allocation := range target {
		targetShares[allocation.ChannelID] = allocation.Share
		targetProbes[allocation.ChannelID] = allocation.Probe
	}
	for channelID := range state.candidates {
		difference := targetShares[channelID] - state.shares[channelID]
		if difference > 0 {
			deficitTotal += difference
		} else {
			excessTotal -= difference
		}
	}
	if deficitTotal < 1e-9 || excessTotal < 1e-9 {
		return false
	}
	move := step
	if move > deficitTotal {
		move = deficitTotal
	}
	if move > excessTotal {
		move = excessTotal
	}
	for channelID := range state.candidates {
		difference := targetShares[channelID] - state.shares[channelID]
		if difference > 0 {
			state.shares[channelID] += move * difference / deficitTotal
		} else if difference < 0 {
			state.shares[channelID] += move * difference / excessTotal
		}
		if targetProbes[channelID] && state.shares[channelID] <= targetShares[channelID]+1e-9 {
			state.probes[channelID] = true
		} else {
			state.probes[channelID] = false
		}
	}
	return true
}

func allocateByWeight(candidates []Candidate, totalShare float64, probe bool) []Allocation {
	if len(candidates) == 0 || totalShare <= 0 {
		return nil
	}
	totalWeight := 0.0
	for _, candidate := range candidates {
		totalWeight += effectiveCandidateWeight(candidate.Weight)
	}
	allocations := make([]Allocation, 0, len(candidates))
	for _, candidate := range candidates {
		weight := effectiveCandidateWeight(candidate.Weight)
		allocations = append(allocations, Allocation{
			ChannelID: candidate.ChannelID,
			Share:     totalShare * weight / totalWeight,
			Probe:     probe,
		})
	}
	return allocations
}

func effectiveCandidateWeight(weight uint) float64 {
	maxUint := ^uint(0)
	if weight > maxUint-10 {
		return float64(maxUint)
	}
	return float64(weight + 10)
}

func (c *Controller) Observe(key ObservationKey, sample Sample) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cleanupExpiredLocked(sample.ObservedAt)
	validMetric := (sample.HasTTFT && sample.TTFT > 0) || (sample.HasTPOT && sample.TPOT > 0)
	if sample.Success && !sample.HardFailure && validMetric {
		c.appendWindowSampleLocked(c.observations, key, sample)
	}
	if sample.Success || sample.HardFailure {
		c.appendWindowSampleLocked(c.healthObservations, key, sample)
	}
}

func (c *Controller) appendWindowSampleLocked(window map[ObservationKey][]Sample, key ObservationKey, sample Sample) {
	samples := append(window[key], sample)
	sort.SliceStable(samples, func(i, j int) bool {
		return samples[i].ObservedAt.Before(samples[j].ObservedAt)
	})
	if len(samples) > c.config.MaxSamples {
		samples = samples[len(samples)-c.config.MaxSamples:]
	}
	window[key] = samples
}

func (c *Controller) ObservationStats(key ObservationKey, now time.Time) ObservationStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	samples := c.freshSamplesLocked(key, now)

	stats := ObservationStats{SampleCount: len(samples)}
	var ttftTotal time.Duration
	var tpotTotal time.Duration
	var ttftCount int
	var tpotCount int
	for _, sample := range samples {
		if sample.HasTTFT {
			ttftTotal += sample.TTFT
			ttftCount++
		}
		if sample.HasTPOT {
			tpotTotal += sample.TPOT
			tpotCount++
		}
	}
	if ttftCount > 0 {
		stats.HasTTFT = true
		stats.TTFTSampleCount = ttftCount
		stats.TTFT = ttftTotal / time.Duration(ttftCount)
	}
	if tpotCount > 0 {
		stats.HasTPOT = true
		stats.TPOTSampleCount = tpotCount
		stats.TPOT = tpotTotal / time.Duration(tpotCount)
	}
	return stats
}
