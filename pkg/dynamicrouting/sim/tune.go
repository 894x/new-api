package sim

import (
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
)

type ParameterSet struct {
	Name   string                `json:"name"`
	Config dynamicrouting.Config `json:"config"`
}

func DefaultParameterGrid() []ParameterSet {
	return []ParameterSet{
		lowProbeParameters("probe-010-m3-age90", 0.01, 3, 90*time.Second, 0.90, 0.02, 3*time.Second, 1),
		lowProbeParameters("probe-015-m3-age90", 0.015, 3, 90*time.Second, 0.90, 0.02, 3*time.Second, 1),
		lowProbeParameters("probe-020-m3-age90", 0.02, 3, 90*time.Second, 0.90, 0.02, 3*time.Second, 1),
		lowProbeParameters("probe-025-m3-age90", 0.025, 3, 90*time.Second, 0.90, 0.02, 3*time.Second, 1),
		lowProbeParameters("probe-015-m3-age60", 0.015, 3, 60*time.Second, 0.85, 0.02, 4*time.Second, 1),
		lowProbeParameters("probe-020-m3-age60", 0.02, 3, 60*time.Second, 0.85, 0.02, 4*time.Second, 1),
		lowProbeParameters("probe-025-m3-age60", 0.025, 3, 60*time.Second, 0.85, 0.02, 4*time.Second, 1),
		lowProbeParameters("probe-020-m4-age90", 0.02, 4, 90*time.Second, 0.80, 0.015, 5*time.Second, 1),
		lowProbeParameters("probe-025-m4-age90", 0.025, 4, 90*time.Second, 0.80, 0.015, 5*time.Second, 1),
		lowProbeParameters("probe-025-critical-fast", 0.025, 3, 60*time.Second, 0.95, 0.015, 2*time.Second, 1),
		lowProbeParameters("probe-020-slow-recovery", 0.02, 3, 90*time.Second, 0.85, 0.01, 5*time.Second, 1),
		lowProbeParameters("probe-015-hard-confirmed", 0.015, 3, 90*time.Second, 0.90, 0.02, 3*time.Second, 2),
	}
}

func lowProbeParameters(name string, probe float64, minSamples int, maxAge time.Duration, aggressiveness, recoveryStep float64, cooldown time.Duration, hardFailures int) ParameterSet {
	return ParameterSet{Name: name, Config: dynamicrouting.Config{
		Enabled: true, MaxSamples: 60, MaxAge: maxAge, MinSamples: minSamples, ProbeFraction: probe,
		DegradationThreshold: 1.3, RecoveryThreshold: 1.1, CriticalThreshold: 1.9, CandidateAdvantage: 1.1,
		Aggressiveness: aggressiveness, RecoveryStep: recoveryStep, Cooldown: cooldown, HardFailureThreshold: hardFailures,
		HardFailureCooldown: 30 * time.Second,
	}}
}

type PolicyFactory func(parameters ParameterSet, seed int64) (Policy, error)

type TuneConfig struct {
	Scenarios                     []Scenario
	Seeds                         []int64
	Parameters                    []ParameterSet
	DynamicFactory                PolicyFactory
	SignificantImprovementPercent float64
	MaxRegressionPercent          float64
	MinScenarioWinRate            float64
	AcceptanceThresholds          AcceptanceThresholds
}

type AcceptanceThresholds struct {
	MinDegradationTTFTImprovementPercent  float64 `json:"min_degradation_ttft_improvement_percent"`
	MinSLOViolationAreaImprovementPercent float64 `json:"min_slo_violation_area_improvement_percent"`
	MinCapacityThroughputChangePercent    float64 `json:"min_capacity_throughput_change_percent"`
	MinBadExposureReductionPercent        float64 `json:"min_bad_exposure_reduction_percent"`
	MaxHealthyTTFTRegressionPercent       float64 `json:"max_healthy_ttft_regression_percent"`
	MaxHealthySuccessRegressionPercent    float64 `json:"max_healthy_success_regression_percent"`
	MaxStabilityRouteReversals            int     `json:"max_stability_route_reversals"`
}

func DefaultAcceptanceThresholds() AcceptanceThresholds {
	return AcceptanceThresholds{
		MinDegradationTTFTImprovementPercent:  15,
		MinSLOViolationAreaImprovementPercent: 20,
		MinCapacityThroughputChangePercent:    0,
		MinBadExposureReductionPercent:        60,
		MaxHealthyTTFTRegressionPercent:       5,
		MaxHealthySuccessRegressionPercent:    5,
		MaxStabilityRouteReversals:            2,
	}
}

type GateResult struct {
	Applicable bool    `json:"applicable"`
	Passed     bool    `json:"passed"`
	Actual     float64 `json:"actual"`
	Required   float64 `json:"required"`
	Comparator string  `json:"comparator"`
}

type AcceptanceResult struct {
	AllPassed          bool       `json:"all_passed"`
	DegradationTTFT    GateResult `json:"degradation_ttft"`
	SLOViolationArea   GateResult `json:"slo_violation_area"`
	CapacityThroughput GateResult `json:"capacity_throughput"`
	BadExposure        GateResult `json:"bad_exposure"`
	HealthyTTFT        GateResult `json:"healthy_ttft"`
	HealthySuccess     GateResult `json:"healthy_success"`
	StabilityReversals GateResult `json:"stability_reversals"`
}

type RunComparison struct {
	Parameters         string  `json:"parameters"`
	Scenario           string  `json:"scenario"`
	Seed               int64   `json:"seed"`
	Static             Metrics `json:"static"`
	Dynamic            Metrics `json:"dynamic"`
	StaticCost         float64 `json:"static_cost"`
	DynamicCost        float64 `json:"dynamic_cost"`
	ImprovementPercent float64 `json:"improvement_percent"`
	Significant        bool    `json:"significant"`
	Regressed          bool    `json:"regressed"`
}

type ParameterRanking struct {
	Rank                      int              `json:"rank"`
	Parameters                ParameterSet     `json:"parameters"`
	AverageImprovementPercent float64          `json:"average_improvement_percent"`
	WorstImprovementPercent   float64          `json:"worst_improvement_percent"`
	ScenarioWinRate           float64          `json:"scenario_win_rate"`
	Significant               bool             `json:"significant"`
	RegressedRuns             int              `json:"regressed_runs"`
	Acceptance                AcceptanceResult `json:"acceptance"`
}

type TuningReport struct {
	SignificantImprovementPercent float64              `json:"significant_improvement_percent"`
	MaxRegressionPercent          float64              `json:"max_regression_percent"`
	MinScenarioWinRate            float64              `json:"min_scenario_win_rate"`
	AcceptanceThresholds          AcceptanceThresholds `json:"acceptance_thresholds"`
	Rankings                      []ParameterRanking   `json:"rankings"`
	Runs                          []RunComparison      `json:"runs"`
}

func Tune(config TuneConfig) (TuningReport, error) {
	if len(config.Scenarios) == 0 || len(config.Seeds) == 0 || len(config.Parameters) == 0 {
		return TuningReport{}, fmt.Errorf("scenarios, seeds, and parameters are required")
	}
	if config.DynamicFactory == nil {
		return TuningReport{}, fmt.Errorf("dynamic policy factory is required")
	}
	if config.MinScenarioWinRate < 0 || config.MinScenarioWinRate > 1 {
		return TuningReport{}, fmt.Errorf("minimum scenario win rate must be in [0, 1]")
	}

	report := TuningReport{
		SignificantImprovementPercent: config.SignificantImprovementPercent,
		MaxRegressionPercent:          config.MaxRegressionPercent,
		MinScenarioWinRate:            config.MinScenarioWinRate,
		AcceptanceThresholds:          config.AcceptanceThresholds,
		Runs:                          make([]RunComparison, 0, len(config.Parameters)*len(config.Scenarios)*len(config.Seeds)),
	}
	for _, parameters := range config.Parameters {
		for _, scenario := range config.Scenarios {
			for _, seed := range config.Seeds {
				seededScenario := scenario
				seededScenario.Seed = seed
				staticResult, err := Run(seededScenario, NewStaticPolicy(seed))
				if err != nil {
					return TuningReport{}, fmt.Errorf("run static %s seed %d: %w", scenario.Name, seed, err)
				}
				dynamicPolicy, err := config.DynamicFactory(parameters, seed)
				if err != nil {
					return TuningReport{}, fmt.Errorf("create dynamic policy %s seed %d: %w", parameters.Name, seed, err)
				}
				dynamicResult, err := Run(seededScenario, dynamicPolicy)
				if err != nil {
					return TuningReport{}, fmt.Errorf("run dynamic %s/%s seed %d: %w", parameters.Name, scenario.Name, seed, err)
				}
				staticMetrics := staticResult.Metrics
				dynamicMetrics := dynamicResult.Metrics
				detectionAt := scenario.Fault.At
				if dynamicMetrics.DetectionDelay >= 0 {
					detectionAt += dynamicMetrics.DetectionDelay
				}
				staticMetrics.BadChannelExposureAfterDetection = badExposureAfter(staticResult.Requests, scenario.Fault.BadChannels, detectionAt)
				dynamicMetrics.BadChannelExposureAfterDetection = badExposureAfter(dynamicResult.Requests, scenario.Fault.BadChannels, detectionAt)
				staticCost := experienceCost(staticMetrics, scenario.SLO)
				dynamicCost := experienceCost(dynamicMetrics, scenario.SLO)
				improvement := improvementPercent(staticCost, dynamicCost)
				regressed := improvement < -config.MaxRegressionPercent || dynamicMetrics.SuccessRate+0.05 < staticMetrics.SuccessRate
				report.Runs = append(report.Runs, RunComparison{
					Parameters: parameters.Name, Scenario: scenario.Name, Seed: seed,
					Static: staticMetrics, Dynamic: dynamicMetrics,
					StaticCost: staticCost, DynamicCost: dynamicCost, ImprovementPercent: improvement,
					Significant: improvement >= config.SignificantImprovementPercent && !regressed,
					Regressed:   regressed,
				})
			}
		}
	}

	for _, parameters := range config.Parameters {
		ranking := ParameterRanking{Parameters: parameters, WorstImprovementPercent: 100}
		parameterRuns := 0
		wins := 0
		for _, run := range report.Runs {
			if run.Parameters != parameters.Name {
				continue
			}
			parameterRuns++
			ranking.AverageImprovementPercent += run.ImprovementPercent
			if run.ImprovementPercent < ranking.WorstImprovementPercent {
				ranking.WorstImprovementPercent = run.ImprovementPercent
			}
			if run.Significant {
				wins++
			}
			if run.Regressed {
				ranking.RegressedRuns++
			}
		}
		if parameterRuns > 0 {
			ranking.AverageImprovementPercent /= float64(parameterRuns)
			ranking.ScenarioWinRate = float64(wins) / float64(parameterRuns)
		}
		ranking.Acceptance = evaluateAcceptance(report.Runs, parameters.Name, config.AcceptanceThresholds)
		ranking.Significant = ranking.AverageImprovementPercent >= config.SignificantImprovementPercent &&
			ranking.ScenarioWinRate >= config.MinScenarioWinRate && ranking.RegressedRuns == 0 && ranking.Acceptance.AllPassed
		report.Rankings = append(report.Rankings, ranking)
	}
	sort.Slice(report.Rankings, func(i, j int) bool {
		if report.Rankings[i].Significant != report.Rankings[j].Significant {
			return report.Rankings[i].Significant
		}
		if report.Rankings[i].AverageImprovementPercent != report.Rankings[j].AverageImprovementPercent {
			return report.Rankings[i].AverageImprovementPercent > report.Rankings[j].AverageImprovementPercent
		}
		return report.Rankings[i].Parameters.Name < report.Rankings[j].Parameters.Name
	})
	for i := range report.Rankings {
		report.Rankings[i].Rank = i + 1
	}
	return report, nil
}

func badExposureAfter(requests []RequestResult, badChannelIDs []string, after time.Duration) int {
	badChannels := make(map[string]struct{}, len(badChannelIDs))
	for _, channelID := range badChannelIDs {
		badChannels[channelID] = struct{}{}
	}
	exposure := 0
	for _, request := range requests {
		if request.ArrivedAt < after {
			continue
		}
		if _, bad := badChannels[request.ChannelID]; bad {
			exposure++
		}
	}
	return exposure
}

func evaluateAcceptance(runs []RunComparison, parameterName string, thresholds AcceptanceThresholds) AcceptanceResult {
	result := AcceptanceResult{}
	var degradationStaticTTFT float64
	var degradationDynamicTTFT float64
	var degradationStaticArea float64
	var degradationDynamicArea float64
	var degradationRuns int
	var capacityStaticThroughput float64
	var capacityDynamicThroughput float64
	var capacityRuns int
	var staticExposure float64
	var dynamicExposure float64
	var exposureRuns int
	var healthyStaticTTFT float64
	var healthyDynamicTTFT float64
	var healthyStaticSuccess float64
	var healthyDynamicSuccess float64
	var healthyRuns int
	maxReversals := 0
	stabilityRuns := 0
	for _, run := range runs {
		if run.Parameters != parameterName {
			continue
		}
		switch run.Scenario {
		case "gradual_degradation", "sudden_outage", "stale_candidate":
			degradationStaticTTFT += float64(run.Static.P95TTFT)
			degradationDynamicTTFT += float64(run.Dynamic.P95TTFT)
			degradationStaticArea += run.Static.SLOViolationArea
			degradationDynamicArea += run.Dynamic.SLOViolationArea
			degradationRuns++
		}
		switch run.Scenario {
		case "gradual_degradation", "sudden_outage", "stale_candidate", "recovery_no_flap":
			staticExposure += float64(run.Static.BadChannelExposureAfterDetection)
			dynamicExposure += float64(run.Dynamic.BadChannelExposureAfterDetection)
			exposureRuns++
		}
		if run.Scenario == "capacity_aggregation" {
			capacityStaticThroughput += run.Static.ThroughputPerSecond
			capacityDynamicThroughput += run.Dynamic.ThroughputPerSecond
			capacityRuns++
		}
		if run.Scenario == "healthy_steady_state" {
			healthyStaticTTFT += float64(run.Static.P95TTFT)
			healthyDynamicTTFT += float64(run.Dynamic.P95TTFT)
			healthyStaticSuccess += run.Static.SuccessRate
			healthyDynamicSuccess += run.Dynamic.SuccessRate
			healthyRuns++
		}
		if run.Scenario == "transient_spike" || run.Scenario == "recovery_no_flap" {
			if run.Dynamic.RouteReversals > maxReversals {
				maxReversals = run.Dynamic.RouteReversals
			}
			stabilityRuns++
		}
	}
	result.DegradationTTFT = minimumGate(degradationRuns > 0, improvementPercent(degradationStaticTTFT, degradationDynamicTTFT), thresholds.MinDegradationTTFTImprovementPercent)
	result.SLOViolationArea = minimumGate(degradationRuns > 0, improvementPercent(degradationStaticArea, degradationDynamicArea), thresholds.MinSLOViolationAreaImprovementPercent)
	result.CapacityThroughput = minimumGate(capacityRuns > 0, improvementPercent(capacityStaticThroughput, capacityDynamicThroughput)*-1, thresholds.MinCapacityThroughputChangePercent)
	result.BadExposure = minimumGate(exposureRuns > 0, improvementPercent(staticExposure, dynamicExposure), thresholds.MinBadExposureReductionPercent)
	result.HealthyTTFT = maximumGate(healthyRuns > 0, -improvementPercent(healthyStaticTTFT, healthyDynamicTTFT), thresholds.MaxHealthyTTFTRegressionPercent)
	result.HealthySuccess = maximumGate(healthyRuns > 0, improvementPercent(healthyStaticSuccess, healthyDynamicSuccess), thresholds.MaxHealthySuccessRegressionPercent)
	result.StabilityReversals = maximumGate(stabilityRuns > 0, float64(maxReversals), float64(thresholds.MaxStabilityRouteReversals))
	result.AllPassed = result.DegradationTTFT.Passed && result.SLOViolationArea.Passed &&
		result.CapacityThroughput.Passed && result.BadExposure.Passed && result.HealthyTTFT.Passed &&
		result.HealthySuccess.Passed && result.StabilityReversals.Passed
	return result
}

func minimumGate(applicable bool, actual, required float64) GateResult {
	return GateResult{Applicable: applicable, Passed: !applicable || actual >= required, Actual: actual, Required: required, Comparator: ">="}
}

func maximumGate(applicable bool, actual, required float64) GateResult {
	return GateResult{Applicable: applicable, Passed: !applicable || actual <= required, Actual: actual, Required: required, Comparator: "<="}
}

func experienceCost(metrics Metrics, slo SLO) float64 {
	ttftCost := 0.0
	if slo.TTFT > 0 {
		ttftCost = float64(metrics.P95TTFT) / float64(slo.TTFT)
	}
	tpotCost := 0.0
	if slo.TPOT > 0 {
		tpotCost = float64(metrics.P95TPOT) / float64(slo.TPOT)
	}
	return ttftCost + tpotCost + 3*metrics.SLOViolationRate + metrics.SLOViolationArea + 5*(1-metrics.SuccessRate)
}

func improvementPercent(baseline, candidate float64) float64 {
	if baseline == 0 {
		if candidate == 0 {
			return 0
		}
		return -100
	}
	return (baseline - candidate) / baseline * 100
}
