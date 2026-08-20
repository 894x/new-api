package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting/sim"
)

func main() {
	output := flag.String("output", "tools/dynamic-routing-sim/results/latest", "directory for JSON, Markdown, and CSV reports")
	seedList := flag.String("seeds", "11,29,47,71,101,131,173", "comma-separated deterministic simulation seeds")
	scenarioList := flag.String("scenarios", "", "optional comma-separated built-in scenario names")
	significant := flag.Float64("significant", 15, "minimum average experience-cost improvement percentage")
	maxRegression := flag.Float64("max-regression", 8, "maximum allowed per-run regression percentage")
	minWinRate := flag.Float64("min-win-rate", 0, "optional minimum fraction of scenario/seed runs meeting the composite improvement gate")
	requireSignificant := flag.Bool("require-significant", false, "exit unsuccessfully when no parameter set passes every gate")
	flag.Parse()

	seeds, err := parseSeeds(*seedList)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	scenarios, err := selectScenarios(sim.BuiltinScenarios(), *scenarioList)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	report, err := sim.Tune(sim.TuneConfig{
		Scenarios:  scenarios,
		Seeds:      seeds,
		Parameters: sim.DefaultParameterGrid(),
		DynamicFactory: func(parameters sim.ParameterSet, seed int64) (sim.Policy, error) {
			return sim.NewDynamicPolicy(parameters.Config, seed+1_000_003, dynamicrouting.RouteKey{
				Group: "simulation", Model: "simulation-model",
			})
		},
		SignificantImprovementPercent: *significant,
		MaxRegressionPercent:          *maxRegression,
		MinScenarioWinRate:            *minWinRate,
		AcceptanceThresholds:          sim.DefaultAcceptanceThresholds(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := sim.WriteReport(*output, report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(report.Rankings) == 0 {
		fmt.Fprintln(os.Stderr, "tuning produced no rankings")
		os.Exit(1)
	}
	best := report.Rankings[0]
	fmt.Printf("best=%s improvement=%.2f%% win_rate=%.2f worst=%.2f%% significant=%t output=%s\n",
		best.Parameters.Name, best.AverageImprovementPercent, best.ScenarioWinRate,
		best.WorstImprovementPercent, best.Significant, *output)
	if *requireSignificant && !best.Significant {
		os.Exit(3)
	}
}

func parseSeeds(value string) ([]int64, error) {
	parts := strings.Split(value, ",")
	seeds := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		seed, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse seed %q: %w", part, err)
		}
		seeds = append(seeds, seed)
	}
	if len(seeds) == 0 {
		return nil, errors.New("at least one seed is required")
	}
	return seeds, nil
}

func selectScenarios(available []sim.Scenario, value string) ([]sim.Scenario, error) {
	if strings.TrimSpace(value) == "" {
		return available, nil
	}
	wanted := make(map[string]struct{})
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			wanted[name] = struct{}{}
		}
	}
	selected := make([]sim.Scenario, 0, len(wanted))
	for _, scenario := range available {
		if _, ok := wanted[scenario.Name]; ok {
			selected = append(selected, scenario)
			delete(wanted, scenario.Name)
		}
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for name := range wanted {
			missing = append(missing, name)
		}
		return nil, fmt.Errorf("unknown scenarios: %s", strings.Join(missing, ", "))
	}
	return selected, nil
}
