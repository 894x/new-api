package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type timeoutAwareDoer struct{}

func (timeoutAwareDoer) Do(request *http.Request) (*http.Response, error) {
	select {
	case <-request.Context().Done():
		return nil, request.Context().Err()
	case <-time.After(100 * time.Millisecond):
		return nil, fmt.Errorf("request context was not cancelled")
	}
}

func writeCLICase(t *testing.T, root, suite, folder, body string) {
	t.Helper()
	dir := filepath.Join(root, suite, folder)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "case.json"), []byte(body), 0o644))
}

func TestRunListShowsFolderScopedCases(t *testing.T) {
	root := t.TempDir()
	writeCLICase(t, root, "openai-chat", "T001-sync", `{"id":"T001","name":"同步响应","dimension":"boundary","protocol":"openai-chat","kind":"chat_sync","default":true,"request":{"method":"POST","path":"/v1/chat/completions"}}`)
	var stdout, stderr bytes.Buffer

	code := run([]string{"list", "--suite", "openai-chat", "--cases-root", root}, func(string) string { return "" }, &stdout, &stderr, nil)

	assert.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "T001")
	assert.Contains(t, stdout.String(), "同步响应")
}

func TestRunSeedanceDryRunKeepsOneTaskSafeDefault(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(t.TempDir(), "report")
	writeCLICase(t, root, "seedance", "V001-text", `{"id":"V001","name":"纯文本","dimension":"compatibility","protocol":"seedance","kind":"seedance_task","default":true,"request":{"method":"POST","path":"/api/v3/contents/generations/tasks","body":{"content":[{"type":"text","text":"test"}],"duration":4,"resolution":"480p","ratio":"16:9"}}}`)
	var stdout, stderr bytes.Buffer

	code := run([]string{"run", "--suite", "seedance", "--cases-root", root, "--base-url", "https://gateway.example", "--dry-run", "--output", output}, func(string) string { return "" }, &stdout, &stderr, nil)

	assert.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "PLAN 1")
	reportBytes, err := os.ReadFile(filepath.Join(output, "report.json"))
	require.NoError(t, err)
	assert.Contains(t, string(reportBytes), `"resolution":"480p"`)
	assert.Contains(t, string(reportBytes), `"duration":4`)
}

func TestRunOpenAIDryRunSelectsOneFolderCase(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(t.TempDir(), "report")
	writeCLICase(t, root, "openai-chat", "T001-sync", `{"id":"T001","name":"默认","dimension":"boundary","protocol":"openai-chat","kind":"chat_sync","default":true,"request":{"method":"POST","path":"/v1/chat/completions"}}`)
	writeCLICase(t, root, "openai-chat", "T002-models", `{"id":"T002","name":"模型列表","dimension":"identity","protocol":"openai-chat","kind":"models_contains","default":false,"request":{"method":"GET","path":"/v1/models"}}`)
	var stdout, stderr bytes.Buffer

	code := run([]string{"run", "--suite", "openai-chat", "--cases-root", root, "--base-url", "https://gateway.example", "--model", "test-model", "--case", "T002", "--dry-run", "--output", output}, func(string) string { return "" }, &stdout, &stderr, nil)

	assert.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "PLAN 1")
	reportBytes, err := os.ReadFile(filepath.Join(output, "report.json"))
	require.NoError(t, err)
	assert.Contains(t, string(reportBytes), `"id":"T002"`)
	assert.NotContains(t, string(reportBytes), `"id":"T001"`)
}

func TestRunLiveAuditRequiresKeyEnvironmentVariable(t *testing.T) {
	root := t.TempDir()
	writeCLICase(t, root, "openai-chat", "T001-sync", `{"id":"T001","name":"同步响应","dimension":"boundary","protocol":"openai-chat","kind":"chat_sync","default":true,"request":{"method":"POST","path":"/v1/chat/completions"}}`)
	var stdout, stderr bytes.Buffer

	code := run([]string{"run", "--suite", "openai-chat", "--cases-root", root, "--base-url", "https://gateway.example", "--model", "test-model"}, func(string) string { return "" }, &stdout, &stderr, nil)

	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "API_AUDIT_API_KEY")
}

func TestRunRejectsUnconfirmedMultiTaskSeedancePlan(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"V001", "V002"} {
		writeCLICase(t, root, "seedance", id, `{"id":"`+id+`","name":"case","dimension":"compatibility","protocol":"seedance","kind":"seedance_task","default":false,"request":{"method":"POST","path":"/api/v3/contents/generations/tasks","body":{"content":[],"duration":4,"resolution":"480p","ratio":"16:9"}}}`)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"run", "--suite", "seedance", "--cases-root", root, "--base-url", "https://gateway.example", "--all-cases"}, func(name string) string {
		if name == "API_AUDIT_API_KEY" {
			return "secret"
		}
		return ""
	}, &stdout, &stderr, nil)

	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "confirm-paid-suite")
}

func TestRunAppliesPerCaseTimeoutToOpenAIRequests(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(t.TempDir(), "report")
	writeCLICase(t, root, "openai-chat", "T001-sync", `{"id":"T001","name":"同步响应","dimension":"boundary","protocol":"openai-chat","kind":"chat_sync","default":true,"request":{"method":"POST","path":"/v1/chat/completions","body":{"messages":[{"role":"user","content":"ping"}]}}}`)
	var stdout, stderr bytes.Buffer

	code := run([]string{"run", "--suite", "openai-chat", "--cases-root", root, "--base-url", "https://gateway.example", "--model", "test-model", "--timeout", "1ms", "--output", output}, func(name string) string {
		if name == "API_AUDIT_API_KEY" {
			return "secret"
		}
		return ""
	}, &stdout, &stderr, timeoutAwareDoer{})

	assert.Equal(t, 1, code, stderr.String())
	reportBytes, err := os.ReadFile(filepath.Join(output, "report.json"))
	require.NoError(t, err)
	assert.Contains(t, string(reportBytes), "context deadline exceeded")
	assert.NotContains(t, string(reportBytes), `"elapsed_ms":0`)
}
