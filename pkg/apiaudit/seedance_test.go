package apiaudit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedanceCase(id string, content []any) CaseDefinition {
	return CaseDefinition{
		ID: id, Name: id, Dimension: "compatibility", Protocol: "seedance", Kind: "seedance_task", Default: id == "V001", Severity: "normal",
		Request: RequestDefinition{Method: http.MethodPost, Path: "/api/v3/contents/generations/tasks", Body: map[string]any{
			"content": content, "duration": float64(4), "resolution": "480p", "ratio": "16:9",
		}},
	}
}

func TestExpandRunsPreservesSafeDefaultAndBuildsFullMatrix(t *testing.T) {
	cases := make([]CaseDefinition, 0, 6)
	for i := 1; i <= 6; i++ {
		cases = append(cases, seedanceCase(fmt.Sprintf("V%03d", i), []any{}))
	}

	defaultRuns, err := ExpandRuns(RunConfig{Suite: "seedance", Model: DefaultSeedanceModel, DryRun: false}, cases[:1])
	require.NoError(t, err)
	require.Len(t, defaultRuns, 1)
	assert.Equal(t, DefaultSeedanceModel, defaultRuns[0].Model)
	assert.Equal(t, "V001", defaultRuns[0].ResultID)

	models := []string{DefaultSeedanceModel, "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615"}
	fullRuns, err := ExpandRuns(RunConfig{Suite: "seedance", Models: models, DryRun: true}, cases)
	require.NoError(t, err)
	assert.Len(t, fullRuns, 18)
	assert.Equal(t, "V001@doubao-seedance-2-0-260128", fullRuns[0].ResultID)
	assert.Equal(t, "V006@doubao-seedance-2-0-mini-260615", fullRuns[17].ResultID)
}

func TestExpandRunsRequiresConfirmationForMultipleLiveSeedanceTasks(t *testing.T) {
	cases := []CaseDefinition{seedanceCase("V001", nil), seedanceCase("V002", nil)}

	_, err := ExpandRuns(RunConfig{Suite: "seedance", Model: DefaultSeedanceModel}, cases)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirm-paid-suite")
}

func TestRunSeedanceCaseSendsOfficialRolesAndPollsToSuccess(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body map[string]any
			require.NoError(t, common.DecodeJson(r.Body, &body))
			assert.Equal(t, DefaultSeedanceModel, body["model"])
			content, _ := body["content"].([]any)
			require.Len(t, content, 3)
			first, _ := content[1].(map[string]any)
			last, _ := content[2].(map[string]any)
			assert.Equal(t, "first_frame", first["role"])
			assert.Equal(t, "last_frame", last["role"])
			_, _ = w.Write([]byte(`{"id":"task_1","status":"queued"}`))
			return
		}
		assert.Equal(t, "/api/v3/contents/generations/tasks/task_1", r.URL.Path)
		if polls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"id":"task_1","status":"running","progress":50}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"task_1","status":"succeeded","content":{"video_url":"https://video.example/result.mp4?X-Tos-Signature=secret"},"usage":{"completion_tokens":120,"total_tokens":120},"duration":4,"resolution":"480p","ratio":"16:9"}`))
	}))
	defer server.Close()
	definition := seedanceCase("V003", []any{
		map[string]any{"type": "text", "text": "test"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://assets.example/first.jpg"}, "role": "first_frame"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://assets.example/last.jpg"}, "role": "last_frame"},
	})
	config := RunConfig{Suite: "seedance", BaseURL: server.URL, APIKey: "secret", Model: DefaultSeedanceModel, PollInterval: time.Millisecond, Timeout: time.Second}

	result := RunSeedanceCase(context.Background(), server.Client(), config, PlannedRun{Case: definition, Model: DefaultSeedanceModel, ResultID: "V003"})

	assert.Equal(t, StatusPass, result.Status)
	assert.Equal(t, float64(120), result.Usage["total_tokens"])
	assert.Contains(t, result.Evidence, "task_1")
	assert.Len(t, result.Exchanges, 3)
	assert.True(t, strings.Contains(result.Exchanges[2].ResponseBody, "X-Tos-Signature"), "raw exchange remains available until report redaction")
}

func TestRunSeedanceCaseDryRunDoesNotCallNetwork(t *testing.T) {
	definition := seedanceCase("V001", []any{map[string]any{"type": "text", "text": "test"}})
	config := RunConfig{Suite: "seedance", BaseURL: "https://gateway.example", Model: DefaultSeedanceModel, DryRun: true}

	result := RunSeedanceCase(context.Background(), nil, config, PlannedRun{Case: definition, Model: DefaultSeedanceModel, ResultID: "V001"})

	assert.Equal(t, StatusUnknown, result.Status)
	assert.Contains(t, result.Evidence, "dry-run")
	require.Len(t, result.Exchanges, 1)
	assert.Equal(t, float64(4), result.Exchanges[0].RequestBody["duration"])
}
