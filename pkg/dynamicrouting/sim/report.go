package sim

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func WriteReport(directory string, report TuningReport) error {
	if directory == "" {
		return fmt.Errorf("report directory is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}

	jsonBytes, err := common.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal JSON report: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "report.json"), jsonBytes, 0o644); err != nil {
		return fmt.Errorf("write JSON report: %w", err)
	}

	var markdown strings.Builder
	markdown.WriteString("# Dynamic routing simulation\n\n")
	fmt.Fprintf(&markdown, "Significant improvement gate: %.2f%%; maximum regression: %.2f%%; minimum scenario win rate: %.2f.\n\n",
		report.SignificantImprovementPercent, report.MaxRegressionPercent, report.MinScenarioWinRate)
	markdown.WriteString("## Ranking\n\n")
	markdown.WriteString("| Rank | Parameters | Average improvement | Worst seed | Win rate | Acceptance gates | Significant | Regressed runs |\n")
	markdown.WriteString("|---:|---|---:|---:|---:|:---:|:---:|---:|\n")
	for _, ranking := range report.Rankings {
		fmt.Fprintf(&markdown, "| %d | %s | %.2f%% | %.2f%% | %.2f | %t | %t | %d |\n",
			ranking.Rank, ranking.Parameters.Name, ranking.AverageImprovementPercent,
			ranking.WorstImprovementPercent, ranking.ScenarioWinRate, ranking.Acceptance.AllPassed, ranking.Significant, ranking.RegressedRuns)
	}
	markdown.WriteString("\n## Acceptance gates\n\n")
	markdown.WriteString("| Parameters | Gate | Actual | Required | Applicable | Passed |\n")
	markdown.WriteString("|---|---|---:|---:|:---:|:---:|\n")
	for _, ranking := range report.Rankings {
		gates := []struct {
			name string
			gate GateResult
		}{
			{"Degradation/outage p95 TTFT improvement", ranking.Acceptance.DegradationTTFT},
			{"SLO violation area improvement", ranking.Acceptance.SLOViolationArea},
			{"Capacity throughput change", ranking.Acceptance.CapacityThroughput},
			{"Bad exposure reduction", ranking.Acceptance.BadExposure},
			{"Healthy p95 TTFT regression", ranking.Acceptance.HealthyTTFT},
			{"Healthy success regression", ranking.Acceptance.HealthySuccess},
			{"Stability route reversals", ranking.Acceptance.StabilityReversals},
		}
		for _, item := range gates {
			fmt.Fprintf(&markdown, "| %s | %s | %.2f | %s %.2f | %t | %t |\n",
				ranking.Parameters.Name, item.name, item.gate.Actual, item.gate.Comparator,
				item.gate.Required, item.gate.Applicable, item.gate.Passed)
		}
	}
	markdown.WriteString("\n## Every scenario and seed\n\n")
	markdown.WriteString("p95 TTFT includes the user-observed latency to either the first streamed response or an error response; TPOT only covers successful responses with output timing.\n\n")
	markdown.WriteString("| Parameters | Scenario | Seed | Static p95 TTFT | Dynamic p95 TTFT | Static p95 TPOT | Dynamic p95 TPOT | Static SLO violations | Dynamic SLO violations | Static success | Dynamic success | Static throughput | Dynamic throughput | Detection elapsed | Detection observations | Static bad exposure after fault | Dynamic bad exposure after fault | Static bad exposure after detection | Dynamic bad exposure after detection | Mitigation elapsed | Route reversals | Probe cost | Improvement | Result |\n")
	markdown.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, run := range report.Runs {
		result := "neutral"
		if run.Significant {
			result = "win"
		} else if run.Regressed {
			result = "regression"
		}
		fmt.Fprintf(&markdown, "| %s | %s | %d | %s | %s | %s | %s | %.3f | %.3f | %.3f | %.3f | %.3f | %.3f | %s | %d | %d | %d | %d | %d | %s | %d | %.3f | %.2f%% | %s |\n",
			run.Parameters, run.Scenario, run.Seed,
			run.Static.P95TTFT, run.Dynamic.P95TTFT, run.Static.P95TPOT, run.Dynamic.P95TPOT,
			run.Static.SLOViolationRate, run.Dynamic.SLOViolationRate,
			run.Static.SuccessRate, run.Dynamic.SuccessRate,
			run.Static.ThroughputPerSecond, run.Dynamic.ThroughputPerSecond,
			durationLabel(run.Dynamic.DetectionDelay), run.Dynamic.DetectionObservations,
			run.Static.BadChannelExposureAfterFault, run.Dynamic.BadChannelExposureAfterFault,
			run.Static.BadChannelExposureAfterDetection, run.Dynamic.BadChannelExposureAfterDetection,
			durationLabel(run.Dynamic.MitigationDelay),
			run.Dynamic.RouteReversals, run.Dynamic.ProbeCost, run.ImprovementPercent, result)
	}
	if err := os.WriteFile(filepath.Join(directory, "report.md"), []byte(markdown.String()), 0o644); err != nil {
		return fmt.Errorf("write Markdown report: %w", err)
	}

	csvFile, err := os.Create(filepath.Join(directory, "ranking.csv"))
	if err != nil {
		return fmt.Errorf("create CSV ranking: %w", err)
	}
	writer := csv.NewWriter(csvFile)
	writeErr := writer.Write([]string{"rank", "parameters", "average_improvement_percent", "worst_improvement_percent", "scenario_win_rate", "acceptance_all_passed", "significant", "regressed_runs"})
	for _, ranking := range report.Rankings {
		if writeErr != nil {
			break
		}
		writeErr = writer.Write([]string{
			strconv.Itoa(ranking.Rank), ranking.Parameters.Name,
			strconv.FormatFloat(ranking.AverageImprovementPercent, 'f', 4, 64),
			strconv.FormatFloat(ranking.WorstImprovementPercent, 'f', 4, 64),
			strconv.FormatFloat(ranking.ScenarioWinRate, 'f', 4, 64),
			strconv.FormatBool(ranking.Acceptance.AllPassed), strconv.FormatBool(ranking.Significant), strconv.Itoa(ranking.RegressedRuns),
		})
	}
	writer.Flush()
	if writeErr == nil {
		writeErr = writer.Error()
	}
	closeErr := csvFile.Close()
	if writeErr != nil {
		return fmt.Errorf("write CSV ranking: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close CSV ranking: %w", closeErr)
	}
	return nil
}

func durationLabel(value time.Duration) string {
	if value < 0 {
		return "not detected"
	}
	return value.String()
}
