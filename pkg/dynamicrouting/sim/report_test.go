package sim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReportPreservesWinningAndLosingRunsInEveryFormat(t *testing.T) {
	report := TuningReport{
		SignificantImprovementPercent: 20,
		Rankings: []ParameterRanking{
			{Rank: 1, Parameters: ParameterSet{Name: "winner"}, AverageImprovementPercent: 35, Significant: true},
			{Rank: 2, Parameters: ParameterSet{Name: "loser"}, AverageImprovementPercent: -8, Significant: false, RegressedRuns: 1},
		},
		Runs: []RunComparison{
			{Parameters: "winner", Scenario: "outage", Seed: 1, ImprovementPercent: 35, Significant: true,
				Static:  Metrics{BadChannelExposureAfterFault: 8, BadChannelExposureAfterDetection: 7},
				Dynamic: Metrics{DetectionDelay: 2, DetectionObservations: 3, BadChannelExposureAfterFault: 4, BadChannelExposureAfterDetection: 3}},
			{Parameters: "loser", Scenario: "outage", Seed: 1, ImprovementPercent: -8, Regressed: true, Dynamic: Metrics{DetectionDelay: -1, MitigationDelay: -1}},
		},
	}
	directory := t.TempDir()

	err := WriteReport(directory, report)
	require.NoError(t, err)

	jsonBytes, err := os.ReadFile(filepath.Join(directory, "report.json"))
	require.NoError(t, err)
	var decoded TuningReport
	require.NoError(t, common.Unmarshal(jsonBytes, &decoded))
	assert.Equal(t, report, decoded)

	markdown, err := os.ReadFile(filepath.Join(directory, "report.md"))
	require.NoError(t, err)
	assert.Contains(t, string(markdown), "winner")
	assert.Contains(t, string(markdown), "loser")
	assert.Contains(t, string(markdown), "Detection observations")
	assert.Contains(t, string(markdown), "Acceptance gates")
	assert.Contains(t, string(markdown), "Static bad exposure after detection")
	assert.Contains(t, string(markdown), "Dynamic bad exposure after detection")
	assert.Contains(t, string(markdown), "| 8 | 4 | 7 | 3 |")
	assert.Contains(t, string(markdown), "Static throughput")

	csvBytes, err := os.ReadFile(filepath.Join(directory, "ranking.csv"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(csvBytes)), "\n")
	assert.Len(t, lines, 3)
	assert.Contains(t, lines[1], "winner")
	assert.Contains(t, lines[2], "loser")
}
