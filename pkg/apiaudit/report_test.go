package apiaudit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReportAggregatesDimensionsAndCriticalVerdict(t *testing.T) {
	config := RunConfig{Suite: "openai-chat", BaseURL: "https://gateway.example", Model: "audit-model"}
	results := []CaseResult{
		{ID: "T001", Dimension: "boundary", Status: StatusPass},
		{ID: "T002", Dimension: "identity", Status: StatusWarning},
		{ID: "T003", Dimension: "billing", Status: StatusFail, Severity: "critical"},
		{ID: "T004", Dimension: "billing", Status: StatusUnknown},
	}

	report := BuildReport(config, results)

	assert.Equal(t, "rejected", report.Overall)
	assert.Contains(t, report.Verdict, "CRITICAL")
	assert.Equal(t, SummaryCounts{Pass: 1, Warning: 1, Fail: 1, Unknown: 1}, report.Summary)
	require.Len(t, report.Dimensions, 3)
	assert.Equal(t, "billing", report.Dimensions[0].Dimension)
	require.Len(t, report.Failures, 1)
	assert.Equal(t, "T003", report.Failures[0].ID)
	require.Len(t, report.Warnings, 1)
	assert.Equal(t, "T002", report.Warnings[0].ID)
}

func TestWriteReportCreatesJSONHTMLAndRedactedRawArtifacts(t *testing.T) {
	output := t.TempDir()
	report := BuildReport(RunConfig{Suite: "seedance", BaseURL: "https://gateway.example", Model: "video-model", APIKey: "known-key-value"}, []CaseResult{{
		ID: "V001", Name: "视频", Dimension: "compatibility", Protocol: "seedance", Model: "video-model", Status: StatusFail, Evidence: "failed at https://video.example/result.mp4?X-Tos-Signature=secret",
		Exchanges: []HTTPExchange{{Method: "GET", URL: "https://video.example/result.mp4?X-Tos-Signature=secret", RequestBody: map[string]any{"api_key": "third-party-key"}, ResponseBody: `{"video_url":"https://video.example/result.mp4?X-Tos-Signature=secret","authorization":"Bearer upstream-token-value","echo":"known-key-value","token":"sk-sensitive-value"}`}},
	}})

	err := WriteReport(output, report)

	require.NoError(t, err)
	jsonBytes, err := os.ReadFile(filepath.Join(output, "report.json"))
	require.NoError(t, err)
	htmlBytes, err := os.ReadFile(filepath.Join(output, "report.html"))
	require.NoError(t, err)
	rawBytes, err := os.ReadFile(filepath.Join(output, "raw", "V001", "exchange-01.json"))
	require.NoError(t, err)
	for _, content := range []string{string(jsonBytes), string(htmlBytes), string(rawBytes)} {
		assert.NotContains(t, content, "X-Tos-Signature")
		assert.NotContains(t, content, "secret")
		assert.NotContains(t, content, "upstream-token-value")
		assert.NotContains(t, content, "known-key-value")
		assert.NotContains(t, content, "third-party-key")
		assert.NotContains(t, content, "sk-sensitive-value")
	}
	assert.Contains(t, string(htmlBytes), "测试总结")
	assert.Contains(t, string(htmlBytes), "compatibility")
	assert.Contains(t, string(htmlBytes), "HTTP")
	assert.Contains(t, string(htmlBytes), "raw/V001")
}

func TestWriteReportRejectsUnsafeArtifactID(t *testing.T) {
	output := t.TempDir()
	report := BuildReport(RunConfig{Suite: "openai-chat", BaseURL: "https://gateway.example", Model: "model"}, []CaseResult{{
		ID: "../../outside", Status: StatusFail, Dimension: "boundary",
	}})

	err := WriteReport(output, report)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsafe result id")
}
