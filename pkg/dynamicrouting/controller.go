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

	DegradationThreshold      float64
	RecoveryThreshold         float64
	CriticalThreshold         float64
	CandidateAdvantage        float64
	Aggressiveness            float64
	RecoveryStep              float64
	Cooldown                  time.Duration
	HardFailureThreshold      int
	HardFailureCooldown       time.Duration
	HardFailureMaxCooldown    time.Duration
	RecoveryObservationWindow time.Duration
}

type RouteKey struct {
	Group         string
	Model         string
	Policy        string
	discountFirst bool
	priorityOnly  bool
}

// DiscountLimits defines the service quality required before cost takes precedence.
type DiscountLimits struct {
	MaxTTFT time.Duration
	MaxTPOT time.Duration
}

type Candidate struct {
	ChannelID    int
	Priority     int64
	Weight       uint
	CostDiscount *float64
	// PrefixMatchedBytes is request-local and does not change route shares or
	// the candidate fingerprint. It prefers warm peers within a healthy tier.
	PrefixMatchedBytes int
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
	mu                    sync.Mutex
	config                Config
	observations          map[ObservationKey][]Sample
	healthObservations    map[ObservationKey][]Sample
	performanceAfter      map[ObservationKey]time.Time
	routes                map[RouteKey]*routeState
	selections            map[RouteKey]*selectionState
	monitor               map[RouteKey]*routeMonitor
	monitorScratch        *monitorScratch
	nextDispatchID        uint64
	lastCleanup           time.Time
	observationMaxSamples int
}

type selectionState struct {
	debts      map[int]float64
	lastAccess time.Time
}

type routeState struct {
	shares         map[int]float64
	probes         map[int]bool
	candidates     map[int]candidateFingerprint
	degraded       map[int]bool
	ejections      map[int]hardEjection
	backoffs       map[int]hardFailureBackoff
	lastChanged    time.Time
	lastAccess     time.Time
	discountLimits DiscountLimits
	config         Config
	configured     bool
}

type candidateFingerprint struct {
	priority int64
	weight   uint
	discount float64
}

type hardEjection struct {
	ejectedUntil   time.Time
	lastEligible   time.Time
	probeDebt      float64
	probeIssuedAt  []time.Time
	observingSince time.Time
}

type hardFailureBackoff struct {
	cooldown     time.Duration
	lastHardAt   time.Time
	lastEligible time.Time
	recoveredAt  time.Time
}

func (state *routeState) expired(config Config, now time.Time) bool {
	retention := config.MaxAge
	if len(state.backoffs) > 0 || len(state.ejections) > 0 {
		retention = max(retention, config.HardFailureMaxCooldown, config.RecoveryObservationWindow)
	}
	expiresAt := state.lastAccess.Add(retention)
	for _, ejection := range state.ejections {
		if deadline := ejection.ejectedUntil.Add(config.MaxAge); deadline.After(expiresAt) {
			expiresAt = deadline
		}
	}
	return !state.lastAccess.IsZero() && now.After(expiresAt)
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
		config:                normalized,
		observationMaxSamples: normalized.MaxSamples,
		observations:          make(map[ObservationKey][]Sample),
		healthObservations:    make(map[ObservationKey][]Sample),
		performanceAfter:      make(map[ObservationKey]time.Time),
		routes:                make(map[RouteKey]*routeState),
		selections:            make(map[RouteKey]*selectionState),
		monitor:               make(map[RouteKey]*routeMonitor),
	}, nil
}

// ValidateConfig checks tuning without allocating controller state.
func ValidateConfig(config Config) error {
	_, err := normalizeConfig(config)
	return err
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
	if config.HardFailureMaxCooldown == 0 {
		config.HardFailureMaxCooldown = max(15*time.Minute, config.HardFailureCooldown)
	}
	if config.HardFailureMaxCooldown < config.HardFailureCooldown {
		return Config{}, errors.New("maximum hard failure cooldown cannot be below hard failure cooldown")
	}
	if config.RecoveryObservationWindow == 0 {
		config.RecoveryObservationWindow = time.Minute
	}
	if config.RecoveryObservationWindow < 0 {
		return Config{}, errors.New("recovery observation window must be positive")
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
		preservedRoutes := make(map[RouteKey]*routeState)
		for key, state := range c.routes {
			if !state.configured && len(state.ejections) == 0 && len(state.backoffs) == 0 {
				continue
			}
			// Tuning changes must not erase failure history or jump a recovering
			// channel back to its original traffic share.
			preservedRoutes[key] = state
		}
		c.routes = preservedRoutes
		c.selections = make(map[RouteKey]*selectionState)
	}
	c.config = normalized
	c.observationMaxSamples = max(c.observationMaxSamples, normalized.MaxSamples)
	c.lastCleanup = time.Time{}
	for key, samples := range c.observations {
		if len(samples) > c.observationMaxSamples {
			c.observations[key] = append([]Sample(nil), samples[len(samples)-c.observationMaxSamples:]...)
		}
	}
	for key, samples := range c.healthObservations {
		if len(samples) > c.observationMaxSamples {
			c.healthObservations[key] = append([]Sample(nil), samples[len(samples)-c.observationMaxSamples:]...)
		}
	}
	return nil
}

func (c *Controller) Select(key RouteKey, candidates []Candidate, now time.Time) Decision {
	return c.selectAvoiding(key, candidates, nil, now, DiscountLimits{})
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
	return c.selectAvoiding(key, candidates, excludedChannelIDs, now, DiscountLimits{})
}

// SelectDiscountAvoiding shares health, exploration and fair selection with smart
// routing, but only moves traffic away from cheap channels when quality fails.
func (c *Controller) SelectDiscountAvoiding(key RouteKey, candidates []Candidate, excludedChannelIDs map[int]struct{}, limits DiscountLimits, now time.Time) Decision {
	if limits.MaxTTFT <= 0 || limits.MaxTPOT <= 0 {
		return Decision{}
	}
	key.discountFirst = true
	return c.selectAvoiding(key, candidates, excludedChannelIDs, now, limits)
}

// SelectConfiguredAvoiding applies validated pool tuning to this decision only.
// Observations stay shared, while allocation and failure recovery remain scoped
// to the route. A nil limits value selects the normal smart strategy.
func (c *Controller) SelectConfiguredAvoiding(key RouteKey, candidates []Candidate, excludedChannelIDs map[int]struct{}, now time.Time, config Config, limits *DiscountLimits) (Decision, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return Decision{}, err
	}
	discountLimits := DiscountLimits{}
	if limits != nil {
		if limits.MaxTTFT <= 0 || limits.MaxTPOT <= 0 {
			return Decision{}, errors.New("discount latency limits must be positive")
		}
		key.discountFirst = true
		discountLimits = *limits
	}
	return c.selectAvoiding(key, candidates, excludedChannelIDs, now, discountLimits, normalized), nil
}

func (c *Controller) selectAvoiding(
	key RouteKey,
	candidates []Candidate,
	excludedChannelIDs map[int]struct{},
	now time.Time,
	limits DiscountLimits,
	configs ...Config,
) (decision Decision) {
	c.mu.Lock()
	defer c.mu.Unlock()

	config := c.config
	if len(configs) > 0 {
		config = configs[0]
	}
	c.observationMaxSamples = max(c.observationMaxSamples, config.MaxSamples)

	decision = Decision{Dynamic: config.Enabled}
	c.cleanupExpiredLocked(now, config)
	if state, exists := c.routes[key]; exists && state.expired(state.config, now) {
		delete(c.routes, key)
		delete(c.selections, key)
	}
	if len(candidates) == 0 {
		delete(c.selections, key)
		return decision
	}

	ordered := deduplicateCandidates(candidates)
	sort.Slice(ordered, func(i, j int) bool {
		if key.discountFirst && candidateDiscount(ordered[i]) != candidateDiscount(ordered[j]) {
			return candidateDiscount(ordered[i]) < candidateDiscount(ordered[j])
		}
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority > ordered[j].Priority
		}
		return ordered[i].ChannelID < ordered[j].ChannelID
	})
	defer func() { c.recordMonitorLocked(key, ordered, decision, now) }()

	mainCandidates := make([]Candidate, 0, len(ordered))
	probeCandidates := make([]Candidate, 0, len(ordered))
	for _, candidate := range ordered {
		if sameCandidateTier(candidate, ordered[0], key.discountFirst) {
			mainCandidates = append(mainCandidates, candidate)
		} else {
			probeCandidates = append(probeCandidates, candidate)
		}
	}

	probeShare := config.ProbeFraction
	if !config.Enabled || len(probeCandidates) == 0 {
		probeShare = 0
	}
	initial := append(allocateByWeight(mainCandidates, 1-probeShare, false), allocateByWeight(probeCandidates, probeShare, true)...)
	if !config.Enabled {
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
			if now.After(retentionStart.Add(config.MaxAge)) {
				delete(state.ejections, channelID)
				delete(state.backoffs, channelID)
			}
		}
	}
	if !exists || !candidateFingerprintMatches(state, ordered) || state.discountLimits != limits {
		ejections := make(map[int]hardEjection, len(ordered))
		backoffs := make(map[int]hardFailureBackoff)
		var previousShares map[int]float64
		var previousProbes map[int]bool
		if exists {
			for channelID, ejection := range state.ejections {
				ejections[channelID] = ejection
			}
			backoffs = state.backoffs
			if len(backoffs) > 0 {
				previousShares, previousProbes = state.shares, state.probes
			}
		}
		if backoffs == nil {
			backoffs = make(map[int]hardFailureBackoff)
		}
		state = &routeState{
			shares:         make(map[int]float64, len(initial)),
			probes:         make(map[int]bool, len(initial)),
			candidates:     make(map[int]candidateFingerprint, len(ordered)),
			degraded:       make(map[int]bool, len(ordered)),
			ejections:      ejections,
			backoffs:       backoffs,
			lastAccess:     now,
			discountLimits: limits,
			config:         config,
			configured:     len(configs) > 0,
		}
		for _, allocation := range initial {
			state.shares[allocation.ChannelID] = allocation.Share
			state.probes[allocation.ChannelID] = allocation.Probe
			_, recovering := backoffs[allocation.ChannelID]
			if share, found := previousShares[allocation.ChannelID]; found && recovering {
				state.shares[allocation.ChannelID] = share
				state.probes[allocation.ChannelID] = previousProbes[allocation.ChannelID]
			}
		}
		if len(previousShares) > 0 {
			preserved, remaining := 0.0, 0.0
			for id, share := range state.shares {
				if _, recovering := backoffs[id]; recovering {
					preserved += share
				} else {
					remaining += share
				}
			}
			if remaining > 0 {
				for id := range state.shares {
					if _, recovering := backoffs[id]; !recovering {
						state.shares[id] *= max(0, 1-preserved) / remaining
					}
				}
			} else if preserved > 0 {
				for id := range state.shares {
					state.shares[id] /= preserved
				}
			}
		}
		for _, candidate := range ordered {
			state.candidates[candidate.ChannelID] = candidateFingerprint{
				priority: candidate.Priority,
				weight:   candidate.Weight,
				discount: candidateDiscount(candidate),
			}
		}
		c.routes[key] = state
		delete(c.selections, key)
	}
	state.lastAccess = now
	state.config, state.configured = config, len(configs) > 0
	for channelID, backoff := range state.backoffs {
		if _, present := state.candidates[channelID]; present {
			backoff.lastEligible = now
			state.backoffs[channelID] = backoff
		} else if _, ejected := state.ejections[channelID]; !ejected && now.After(backoff.lastEligible.Add(max(config.MaxAge, config.HardFailureMaxCooldown))) {
			delete(state.backoffs, channelID)
		}
	}

	measurements := make(map[int]channelMeasurement, len(ordered))
	for _, candidate := range ordered {
		measurements[candidate.ChannelID] = c.measureChannelLocked(ObservationKey{
			ChannelID: candidate.ChannelID,
			Model:     key.Model,
		}, now, config)
	}
	cooldownEjected := make(map[int]bool, len(ordered))
	probeEjected := make(map[int]bool, len(ordered))
	ejectionTransition := false
	for _, candidate := range ordered {
		measurement := measurements[candidate.ChannelID]
		ejection, ejected := state.ejections[candidate.ChannelID]
		backoff, backedOff := state.backoffs[candidate.ChannelID]
		observationKey := ObservationKey{ChannelID: candidate.ChannelID, Model: key.Model}
		if measurement.lastHardAt.After(backoff.lastHardAt) && (measurement.hardFail || ejected || backedOff) {
			cooldown := config.HardFailureCooldown
			if backedOff {
				cooldown = max(cooldown, backoff.cooldown)
				// Concurrent failures from traffic already in flight extend the
				// same cooldown. A failed recovery attempt doubles it, safely capped.
				if !ejected || !measurement.lastHardAt.Before(ejection.ejectedUntil) {
					cooldown += min(cooldown, config.HardFailureMaxCooldown-cooldown)
				}
			}
			cooldown = min(cooldown, config.HardFailureMaxCooldown)
			state.backoffs[candidate.ChannelID] = hardFailureBackoff{
				cooldown: cooldown, lastHardAt: measurement.lastHardAt, lastEligible: now,
			}
			ejection = hardEjection{
				ejectedUntil: measurement.lastHardAt.Add(cooldown),
				lastEligible: now,
			}
			state.ejections[candidate.ChannelID] = ejection
			if cutoff := c.performanceAfter[observationKey]; measurement.lastHardAt.After(cutoff) {
				c.performanceAfter[observationKey] = measurement.lastHardAt
			}
			ejected = true
			ejectionTransition = true
		}
		if ejected && !now.Before(ejection.ejectedUntil) {
			streak, lastSuccess := c.recoverySuccessStreakLocked(observationKey, ejection, now, config)
			if streak < config.MinSamples || measurement.ratio > config.RecoveryThreshold {
				ejection.observingSince = time.Time{}
			} else {
				if ejection.observingSince.IsZero() {
					ejection.observingSince = now
				}
				// Time alone cannot restore traffic: require a fresh successful
				// probe at the end of the continuous observation window as well.
				if deadline := ejection.observingSince.Add(config.RecoveryObservationWindow); !lastSuccess.Before(deadline) {
					delete(state.ejections, candidate.ChannelID)
					delete(state.degraded, candidate.ChannelID)
					backoff := state.backoffs[candidate.ChannelID]
					backoff.recoveredAt = now
					state.backoffs[candidate.ChannelID] = backoff
					ejected = false
					ejectionTransition = true
				}
			}
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
	ejectionProbeFloors := probeFloorsForCandidates(ordered, probeEjected, config.ProbeFraction)
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
			effectiveRatios[candidate.ChannelID] = config.CriticalThreshold
			decision.DegradedCandidates = append(decision.DegradedCandidates, candidate.ChannelID)
			continue
		}
		if measurement.hardFail {
			emergency = true
			degraded[candidate.ChannelID] = true
			state.degraded[candidate.ChannelID] = true
			effectiveRatios[candidate.ChannelID] = config.CriticalThreshold
			decision.EmergencyCandidates = append(decision.EmergencyCandidates, candidate.ChannelID)
			decision.DegradedCandidates = append(decision.DegradedCandidates, candidate.ChannelID)
			continue
		}

		effectiveRatio := measurement.ratio
		if key.discountFirst {
			// A more expensive channel being faster is not a quality violation.
			effectiveRatio = 0
			if measurement.hasTTFT {
				effectiveRatio = float64(measurement.ttft) / float64(limits.MaxTTFT)
			}
			if measurement.hasTPOT {
				effectiveRatio = max(effectiveRatio, float64(measurement.tpot)/float64(limits.MaxTPOT))
			}
			effectiveRatio *= config.DegradationThreshold
		} else {
			for _, possible := range ordered {
				if possible.ChannelID == candidate.ChannelID {
					continue
				}
				possibleMeasurement := measurements[possible.ChannelID]
				if !possibleMeasurement.verified || possibleMeasurement.hardFail {
					continue
				}
				if gap, better := performanceGap(measurement, possibleMeasurement, config.CandidateAdvantage); better && gap > effectiveRatio {
					effectiveRatio = gap
				}
			}
		}
		effectiveRatios[candidate.ChannelID] = effectiveRatio
		isDegraded := effectiveRatio >= config.DegradationThreshold
		if key.discountFirst {
			isDegraded = effectiveRatio > config.DegradationThreshold
		}
		if state.degraded[candidate.ChannelID] && (!measurement.verified || effectiveRatio > config.RecoveryThreshold) {
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
	if !emergency && !ejectionTransition && !state.lastChanged.IsZero() && now.Sub(state.lastChanged) < config.Cooldown {
		decision.Allocations = allocationsFromState(ordered, state)
		return c.completeDecisionLocked(key, decision, excludedChannelIDs, now, ordered)
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
	if key.discountFirst {
		// Even the original cheapest primary needs samples after shedding traffic.
		all := make(map[int]bool, len(ordered))
		for _, candidate := range ordered {
			all[candidate.ChannelID] = true
		}
		probeFloors = probeFloorsForCandidates(ordered, all, config.ProbeFraction)
		for id, floor := range ejectionProbeFloors {
			probeFloors[id] = max(probeFloors[id], floor)
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
			if key.discountFirst && len(receivers) > 0 && !sameCandidateTier(possible, receivers[0], true) {
				continue
			}
			if key.discountFirst || measurement.hardFail || candidateOutperforms(measurement, possibleMeasurement, config.CandidateAdvantage) {
				receivers = append(receivers, possible)
			}
		}
		if measurement.hardFail && len(receivers) == 0 {
			for _, possible := range ordered {
				if possible.ChannelID == candidate.ChannelID || measurements[possible.ChannelID].hardFail || probeEjected[possible.ChannelID] {
					continue
				}
				if len(receivers) == 0 || sameCandidateTier(possible, receivers[0], key.discountFirst) {
					receivers = append(receivers, possible)
				}
			}
		}
		if len(receivers) == 0 {
			continue
		}

		unload := 1.0
		if !measurement.hardFail {
			unload = config.Aggressiveness
			severity := (effectiveRatios[candidate.ChannelID] - config.DegradationThreshold) /
				(config.CriticalThreshold - config.DegradationThreshold)
			if severity > 1 {
				severity = 1
			}
			unload = 1 - (1-unload)*(1-unload*severity)
		}
		movableShare := state.shares[candidate.ChannelID]
		if !measurement.hardFail && (key.discountFirst || state.probes[candidate.ChannelID]) {
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
		if key.discountFirst && !measurement.hardFail && state.shares[candidate.ChannelID] <= probeFloors[candidate.ChannelID]+1e-9 {
			state.probes[candidate.ChannelID] = true
		}
		for _, allocation := range allocateByWeight(receivers, moved, false) {
			state.shares[allocation.ChannelID] += allocation.Share
			state.probes[allocation.ChannelID] = false
		}
		changed = true
	}
	recoveryTarget := initial
	if key.discountFirst {
		recoveryTarget = discountRecoveryTarget(ordered, state, degraded, measurements, probeEjected, config.ProbeFraction)
		changed = recoverRouteShares(state, recoveryTarget, config.RecoveryStep) || changed
	}
	if changed {
		state.lastChanged = now
	} else if !key.discountFirst && len(degraded) == 0 && recoverRouteShares(state, initial, config.RecoveryStep) {
		state.lastChanged = now
	}
	for _, target := range recoveryTarget {
		backoff, exists := state.backoffs[target.ChannelID]
		if !exists || backoff.recoveredAt.IsZero() || degraded[target.ChannelID] || math.Abs(state.shares[target.ChannelID]-target.Share) > 1e-9 {
			continue
		}
		health := c.freshHealthSamplesLocked(ObservationKey{ChannelID: target.ChannelID, Model: key.Model}, now, config)
		if len(health) == 0 {
			continue
		}
		latest := health[len(health)-1]
		if latest.Success && !latest.HardFailure && !latest.UpstreamStartedAt.Before(backoff.recoveredAt) && !latest.ObservedAt.Before(backoff.recoveredAt.Add(config.RecoveryObservationWindow)) {
			delete(state.backoffs, target.ChannelID)
		}
	}
	decision.Allocations = allocationsFromState(ordered, state)
	return c.completeDecisionLocked(key, decision, excludedChannelIDs, now, ordered)
}

func (c *Controller) cleanupExpiredLocked(now time.Time, configs ...Config) {
	cleanupInterval := min(c.config.MaxAge/4, time.Minute)
	if cleanupInterval <= 0 {
		cleanupInterval = time.Millisecond
	}
	if !c.lastCleanup.IsZero() && (now.Before(c.lastCleanup) || now.Sub(c.lastCleanup) < cleanupInterval) {
		return
	}
	maxSamples, maxAge := c.config.MaxSamples, c.config.MaxAge
	for key, state := range c.routes {
		if state.expired(state.config, now) {
			delete(c.routes, key)
			delete(c.selections, key)
			continue
		}
		maxSamples, maxAge = max(maxSamples, state.config.MaxSamples), max(maxAge, state.config.MaxAge)
	}
	for _, config := range configs {
		maxSamples, maxAge = max(maxSamples, config.MaxSamples), max(maxAge, config.MaxAge)
	}
	c.observationMaxSamples = maxSamples
	cutoff := now.Add(-maxAge)
	for _, window := range []map[ObservationKey][]Sample{c.observations, c.healthObservations} {
		for key, samples := range window {
			first := sort.Search(len(samples), func(i int) bool { return !samples[i].ObservedAt.Before(cutoff) })
			first = max(first, len(samples)-maxSamples)
			if first == len(samples) {
				delete(window, key)
			} else if first > 0 {
				window[key] = append([]Sample(nil), samples[first:]...)
			}
		}
	}
	for key, resetAt := range c.performanceAfter {
		if resetAt.Before(cutoff) {
			delete(c.performanceAfter, key)
		}
	}
	for key, state := range c.selections {
		if !state.lastAccess.IsZero() && state.lastAccess.Before(cutoff) {
			delete(c.selections, key)
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
	candidates []Candidate,
) Decision {
	config := c.config
	if state := c.routes[key]; state != nil {
		config = state.config
	}
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
			if len(ejection.probeIssuedAt) > config.MaxSamples {
				ejection.probeIssuedAt = append([]time.Time(nil), ejection.probeIssuedAt[len(ejection.probeIssuedAt)-config.MaxSamples:]...)
			}
			route.ejections[selected] = ejection
		}
	}
	// Keep accounting against the original weighted choice, so request-local
	// prefix affinity cannot accumulate scheduling debt or starve probes.
	// Only healthy, non-probe peers in that choice's priority/cost tier compete.
	hasPrefixPreference := false
	for _, candidate := range candidates {
		if candidate.PrefixMatchedBytes > 0 {
			hasPrefixPreference = true
			break
		}
	}
	if hasSelection && !hasForcedProbe && hasPrefixPreference {
		warmEligible := make(map[int]bool, len(decision.Allocations))
		for _, allocation := range decision.Allocations {
			_, excluded := excludedChannelIDs[allocation.ChannelID]
			warmEligible[allocation.ChannelID] = allocation.Share > 0 && !allocation.Probe && !excluded
		}
		for _, id := range decision.DegradedCandidates {
			warmEligible[id] = false
		}
		if warmEligible[selected] {
			var selectedCandidate Candidate
			for _, candidate := range candidates {
				if candidate.ChannelID == selected {
					selectedCandidate = candidate
					break
				}
			}
			bestPrefix := selectedCandidate.PrefixMatchedBytes
			for _, candidate := range candidates {
				if warmEligible[candidate.ChannelID] && sameCandidateTier(candidate, selectedCandidate, key.discountFirst) &&
					candidate.PrefixMatchedBytes > bestPrefix {
					selected, bestPrefix = candidate.ChannelID, candidate.PrefixMatchedBytes
				}
			}
		}
	}
	decision.SelectedChannelID = selected
	decision.HasSelection = hasSelection
	return decision
}

func (c *Controller) measureChannelLocked(key ObservationKey, now time.Time, configs ...Config) channelMeasurement {
	config := c.config
	if len(configs) > 0 {
		config = configs[0]
	}
	samples := c.freshSamplesLocked(key, now, config)
	if cutoff := c.performanceAfter[key]; !cutoff.IsZero() {
		firstAfterCutoff := sort.Search(len(samples), func(index int) bool {
			return samples[index].ObservedAt.After(cutoff)
		})
		samples = samples[firstAfterCutoff:]
	}
	healthSamples := c.freshHealthSamplesLocked(key, now, config)
	measurement := channelMeasurement{}
	consecutiveHardFailures := 0
	lastHardAt := time.Time{}
	seenSuccess := false
	for index := len(healthSamples) - 1; index >= 0; index-- {
		sample := healthSamples[index]
		if sample.HardFailure {
			if lastHardAt.IsZero() {
				lastHardAt = sample.ObservedAt
			}
			if !seenSuccess {
				consecutiveHardFailures++
			}
			continue
		}
		if sample.Success {
			seenSuccess = true
		}
	}
	measurement.hardFail = consecutiveHardFailures >= config.HardFailureThreshold
	measurement.lastHardAt = lastHardAt
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
	measurement.verified = ttftSamples >= config.MinSamples || tpotSamples >= config.MinSamples
	ttft, hasTTFT, ttftRatio := summarizeMetric(samples, config.MinSamples, func(sample Sample) (time.Duration, bool) {
		return sample.TTFT, sample.HasTTFT
	})
	tpot, hasTPOT, tpotRatio := summarizeMetric(samples, config.MinSamples, func(sample Sample) (time.Duration, bool) {
		return sample.TPOT, sample.HasTPOT
	})
	if ttftSamples >= config.MinSamples && hasTTFT {
		measurement.ttft = ttft
		measurement.hasTTFT = true
		measurement.ratio = ttftRatio
	}
	if tpotSamples >= config.MinSamples && hasTPOT {
		measurement.tpot = tpot
		measurement.hasTPOT = true
	}
	if measurement.hasTPOT && tpotRatio > measurement.ratio {
		measurement.ratio = tpotRatio
	}
	return measurement
}

func (c *Controller) recoverySuccessStreakLocked(key ObservationKey, ejection hardEjection, now time.Time, configs ...Config) (int, time.Time) {
	samples := c.freshHealthSamplesLocked(key, now, configs...)
	issuedProbes := append([]time.Time(nil), ejection.probeIssuedAt...)
	sort.Slice(issuedProbes, func(i, j int) bool { return issuedProbes[i].Before(issuedProbes[j]) })
	usedProbes := 0
	streak := 0
	var lastSuccess time.Time
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
		lastSuccess = sample.ObservedAt
	}
	return streak, lastSuccess
}

func (c *Controller) freshSamplesLocked(key ObservationKey, now time.Time, configs ...Config) []Sample {
	return c.freshWindowSamplesLocked(c.observations, key, now, configs...)
}

func (c *Controller) freshHealthSamplesLocked(key ObservationKey, now time.Time, configs ...Config) []Sample {
	return c.freshWindowSamplesLocked(c.healthObservations, key, now, configs...)
}

func (c *Controller) freshWindowSamplesLocked(window map[ObservationKey][]Sample, key ObservationKey, now time.Time, configs ...Config) []Sample {
	config := c.config
	if len(configs) > 0 {
		config = configs[0]
	}
	samples := window[key]
	cutoff := now.Add(-config.MaxAge)
	first := sort.Search(len(samples), func(i int) bool { return !samples[i].ObservedAt.Before(cutoff) })
	first = max(first, len(samples)-config.MaxSamples)
	// Reading a short pool window must not discard observations needed by
	// another pool with a longer window. Shared cleanup owns physical pruning.
	return samples[first:]
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

// Unknown cost ranks behind every configured discount, including explicit zero.
func candidateDiscount(candidate Candidate) float64 {
	if candidate.CostDiscount == nil || !finiteFloat(*candidate.CostDiscount) || *candidate.CostDiscount < 0 || *candidate.CostDiscount > 1 {
		return 2
	}
	return *candidate.CostDiscount
}

func sameCandidateTier(left, right Candidate, discountFirst bool) bool {
	return left.Priority == right.Priority && (!discountFirst || candidateDiscount(left) == candidateDiscount(right))
}

// Recovery targets the cheapest usable tier. Degraded probes do not block it.
func discountRecoveryTarget(ordered []Candidate, state *routeState, degraded map[int]bool, measurements map[int]channelMeasurement, ejected map[int]bool, probeShare float64) []Allocation {
	main := make([]Candidate, 0, len(ordered))
	probes := make([]Candidate, 0, len(ordered))
	for _, candidate := range ordered {
		if measurements[candidate.ChannelID].hardFail {
			continue
		}
		if !degraded[candidate.ChannelID] && !ejected[candidate.ChannelID] && (len(main) == 0 || sameCandidateTier(candidate, main[0], true)) {
			main = append(main, candidate)
		} else {
			probes = append(probes, candidate)
		}
	}
	if len(main) == 0 {
		return nil
	}
	allocations := allocateByWeight(probes, probeShare, true)
	reserved := 0.0
	for i := range allocations {
		id := allocations[i].ChannelID
		if degraded[id] || ejected[id] {
			// Preserve health probe floors instead of repeatedly restoring and
			// shedding their traffic while a cheap primary is recovering.
			allocations[i].Share = state.shares[id]
		}
		reserved += allocations[i].Share
	}
	if reserved >= 1 {
		return nil
	}
	return append(allocateByWeight(main, 1-reserved, false), allocations...)
}

func candidateFingerprintMatches(state *routeState, candidates []Candidate) bool {
	if len(state.candidates) != len(candidates) {
		return false
	}
	for _, candidate := range candidates {
		fingerprint, exists := state.candidates[candidate.ChannelID]
		if !exists || fingerprint.priority != candidate.Priority || fingerprint.weight != candidate.Weight || fingerprint.discount != candidateDiscount(candidate) {
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
	if len(samples) > c.observationMaxSamples {
		samples = samples[len(samples)-c.observationMaxSamples:]
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
