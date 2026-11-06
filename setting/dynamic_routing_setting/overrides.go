package dynamic_routing_setting

// Overrides contains only pool-adjustable tuning. Nil fields inherit the
// current system default; explicit zero remains a value and is validated.
type Overrides struct {
	MaxSamples                    *int     `json:"max_samples,omitempty"`
	MaxAgeSeconds                 *int     `json:"max_age_seconds,omitempty"`
	MinSamples                    *int     `json:"min_samples,omitempty"`
	ProbeFraction                 *float64 `json:"probe_fraction,omitempty"`
	DegradationThreshold          *float64 `json:"degradation_threshold,omitempty"`
	RecoveryThreshold             *float64 `json:"recovery_threshold,omitempty"`
	CriticalThreshold             *float64 `json:"critical_threshold,omitempty"`
	CandidateAdvantage            *float64 `json:"candidate_advantage,omitempty"`
	Aggressiveness                *float64 `json:"aggressiveness,omitempty"`
	RecoveryStep                  *float64 `json:"recovery_step,omitempty"`
	CooldownSeconds               *int     `json:"cooldown_seconds,omitempty"`
	HardFailureThreshold          *int     `json:"hard_failure_threshold,omitempty"`
	HardFailureCooldownSeconds    *int     `json:"hard_failure_cooldown_seconds,omitempty"`
	HardFailureMaxCooldownSeconds *int     `json:"hard_failure_max_cooldown_seconds,omitempty"`
	RecoveryObservationSeconds    *int     `json:"recovery_observation_seconds,omitempty"`
}

// Resolve validates one complete effective configuration without changing the
// global defaults or materializing inherited fields into stored overrides.
func (overrides *Overrides) Resolve(base DynamicRoutingSetting) (DynamicRoutingSetting, error) {
	base.Enabled = true
	if overrides != nil {
		if overrides.MaxSamples != nil {
			base.MaxSamples = *overrides.MaxSamples
		}
		if overrides.MaxAgeSeconds != nil {
			base.MaxAgeSeconds = *overrides.MaxAgeSeconds
		}
		if overrides.MinSamples != nil {
			base.MinSamples = *overrides.MinSamples
		}
		if overrides.ProbeFraction != nil {
			base.ProbeFraction = *overrides.ProbeFraction
		}
		if overrides.DegradationThreshold != nil {
			base.DegradationThreshold = *overrides.DegradationThreshold
		}
		if overrides.RecoveryThreshold != nil {
			base.RecoveryThreshold = *overrides.RecoveryThreshold
		}
		if overrides.CriticalThreshold != nil {
			base.CriticalThreshold = *overrides.CriticalThreshold
		}
		if overrides.CandidateAdvantage != nil {
			base.CandidateAdvantage = *overrides.CandidateAdvantage
		}
		if overrides.Aggressiveness != nil {
			base.Aggressiveness = *overrides.Aggressiveness
		}
		if overrides.RecoveryStep != nil {
			base.RecoveryStep = *overrides.RecoveryStep
		}
		if overrides.CooldownSeconds != nil {
			base.CooldownSeconds = *overrides.CooldownSeconds
		}
		if overrides.HardFailureThreshold != nil {
			base.HardFailureThreshold = *overrides.HardFailureThreshold
		}
		if overrides.HardFailureCooldownSeconds != nil {
			base.HardFailureCooldownSeconds = *overrides.HardFailureCooldownSeconds
		}
		if overrides.HardFailureMaxCooldownSeconds != nil {
			base.HardFailureMaxCooldownSeconds = *overrides.HardFailureMaxCooldownSeconds
		}
		if overrides.RecoveryObservationSeconds != nil {
			base.RecoveryObservationSeconds = *overrides.RecoveryObservationSeconds
		}
	}
	return base, Validate(base)
}
