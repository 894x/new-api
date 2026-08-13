package apiaudit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openAIConfig(server *httptest.Server) RunConfig {
	return RunConfig{
		Suite:   "openai-chat",
		BaseURL: server.URL,
		APIKey:  "secret-test-key",
		Model:   "audit-model",
	}
}

func TestRunOpenAIChatCaseValidatesSynchronousResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer secret-test-key", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &body))
		assert.Equal(t, "audit-model", body["model"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-good","choices":[{"message":{"content":"5"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`))
	}))
	defer server.Close()
	definition := CaseDefinition{
		ID: "T001", Name: "同步响应", Dimension: "boundary", Protocol: "openai-chat", Kind: "chat_sync", Severity: "critical",
		Request: RequestDefinition{Method: http.MethodPost, Path: "/v1/chat/completions", Body: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "2+3=?"}}}},
	}

	result := RunOpenAIChatCase(context.Background(), server.Client(), openAIConfig(server), definition)

	assert.Equal(t, StatusPass, result.Status)
	assert.Equal(t, 200, result.HTTPStatus)
	assert.Contains(t, result.Evidence, "finish_reason=stop")
	assert.Equal(t, float64(9), result.Usage["total_tokens"])
	require.Len(t, result.Exchanges, 1)
	assert.NotContains(t, result.Exchanges[0].ResponseBody, "secret-test-key")
}

func TestRunOpenAIChatCaseChecksModelList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"audit-model","object":"model"}]}`))
	}))
	defer server.Close()
	definition := CaseDefinition{ID: "T002", Name: "模型列表", Dimension: "identity", Protocol: "openai-chat", Kind: "models_contains", Request: RequestDefinition{Method: http.MethodGet, Path: "/v1/models"}}

	result := RunOpenAIChatCase(context.Background(), server.Client(), openAIConfig(server), definition)

	assert.Equal(t, StatusPass, result.Status)
	assert.Contains(t, result.Evidence, "audit-model")
}

func TestRunOpenAIChatCaseParsesStreamingUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-stream\",\"choices\":[{\"delta\":{\"content\":\"hel\"},\"finish_reason\":null}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-stream\",\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	definition := CaseDefinition{
		ID: "T004", Name: "流式 usage", Dimension: "billing", Protocol: "openai-chat", Kind: "stream_usage",
		Request: RequestDefinition{Method: http.MethodPost, Path: "/v1/chat/completions", Body: map[string]any{"messages": []any{}, "stream": true, "stream_options": map[string]any{"include_usage": true}}},
	}

	result := RunOpenAIChatCase(context.Background(), server.Client(), openAIConfig(server), definition)

	assert.Equal(t, StatusPass, result.Status)
	assert.Contains(t, result.Evidence, "4 SSE frames")
	assert.Equal(t, float64(5), result.Usage["total_tokens"])
	assert.Contains(t, result.Exchanges[0].ResponseBody, "[DONE]")
}

func TestRunOpenAIChatCaseDetectsInconsistentResponseIDPrefixes(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := "chatcmpl-"
		if calls.Add(1) > 2 {
			prefix = "c-"
		}
		_, _ = fmt.Fprintf(w, `{"id":"%svalue","choices":[{"message":{"content":"5"},"finish_reason":"stop"}]}`, prefix)
	}))
	defer server.Close()
	definition := CaseDefinition{
		ID: "T010", Name: "ID 格式一致性", Dimension: "stability", Protocol: "openai-chat", Kind: "id_consistency", Severity: "critical",
		Request: RequestDefinition{Method: http.MethodPost, Path: "/v1/chat/completions", Body: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "5"}}}},
		Options: map[string]any{"repetitions": float64(4)},
	}

	result := RunOpenAIChatCase(context.Background(), server.Client(), openAIConfig(server), definition)

	assert.Equal(t, StatusFail, result.Status)
	assert.Contains(t, result.Evidence, "2 prefixes")
	assert.Len(t, result.Exchanges, 4)
}

func TestRunOpenAIChatCaseClassifiesMalformedSuccessAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chatcmpl-empty","choices":[]}`))
	}))
	defer server.Close()
	definition := CaseDefinition{ID: "T001", Name: "同步响应", Dimension: "boundary", Protocol: "openai-chat", Kind: "chat_sync", Request: RequestDefinition{Method: http.MethodPost, Path: "/v1/chat/completions", Body: map[string]any{}}}

	result := RunOpenAIChatCase(context.Background(), server.Client(), openAIConfig(server), definition)

	assert.Equal(t, StatusFail, result.Status)
	assert.True(t, strings.Contains(result.Evidence, "choices"))
}

func TestRunOpenAIChatCaseSupportsBoundaryAndSchemaChecks(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		statusCode int
		response   string
		options    map[string]any
		wantStatus string
		wantText   string
	}{
		{name: "response id", kind: "response_id", statusCode: 200, response: `{"id":"chatcmpl-valid","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`, wantStatus: StatusPass, wantText: "OpenAI-style"},
		{name: "stop parameter", kind: "stop_parameter", statusCode: 200, response: `{"id":"chatcmpl-stop","choices":[{"message":{"content":"apple, banana"},"finish_reason":"stop"}]}`, options: map[string]any{"stop_text": "STOP_AUDIT"}, wantStatus: StatusPass, wantText: "excluded"},
		{name: "error schema", kind: "error_schema", statusCode: 400, response: `{"error":{"message":"invalid max_tokens","type":"invalid_request_error","code":"bad_request"}}`, wantStatus: StatusPass, wantText: "error.message"},
		{name: "structured json", kind: "structured_json", statusCode: 200, response: `{"id":"chatcmpl-json","choices":[{"message":{"content":"{\"name\":\"Alice\",\"age\":18}"},"finish_reason":"stop"}]}`, options: map[string]any{"required_keys": []any{"name", "age"}}, wantStatus: StatusPass, wantText: "name, age"},
		{name: "tool call", kind: "tool_call", statusCode: 200, response: `{"id":"chatcmpl-tool","choices":[{"message":{"content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}}]},"finish_reason":"tool_calls"}]}`, wantStatus: StatusPass, wantText: "get_weather"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()
			definition := CaseDefinition{
				ID: "TX", Name: tt.name, Dimension: "boundary", Protocol: "openai-chat", Kind: tt.kind,
				Request: RequestDefinition{Method: http.MethodPost, Path: "/v1/chat/completions", Body: map[string]any{"messages": []any{}}}, Options: tt.options,
			}

			result := RunOpenAIChatCase(context.Background(), server.Client(), openAIConfig(server), definition)

			assert.Equal(t, tt.wantStatus, result.Status)
			assert.Contains(t, result.Evidence, tt.wantText)
		})
	}
}

func TestRunOpenAIChatCaseAppliesDeclarativeContentAndUsageConstraints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chatcmpl-constraints","choices":[{"message":{"content":"CHECK123"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`))
	}))
	defer server.Close()
	definition := CaseDefinition{
		ID: "T006", Name: "隐藏前置 Prompt", Dimension: "billing", Protocol: "openai-chat", Kind: "chat_sync",
		Request: RequestDefinition{Method: http.MethodPost, Path: "/v1/chat/completions", Body: map[string]any{"messages": []any{}}},
		Options: map[string]any{
			"expected_exact":        "CHECK123",
			"forbidden_substrings":  []any{"system prompt", "anthropic"},
			"require_usage":         true,
			"max_completion_tokens": float64(3),
			"max_elapsed_ms":        float64(5000),
		},
	}

	result := RunOpenAIChatCase(context.Background(), server.Client(), openAIConfig(server), definition)

	assert.Equal(t, StatusPass, result.Status)
	assert.Contains(t, result.Evidence, "5 constraints")
}

func TestRunOpenAIChatCaseReturnsUnknownForExplicitBaselineDependentCase(t *testing.T) {
	definition := CaseDefinition{
		ID: "T015", Name: "logprobs 指纹", Dimension: "identity", Protocol: "openai-chat", Kind: "manual_unknown",
		Options: map[string]any{"reason": "缺少官方 logprobs 指纹基线"},
	}

	result := RunOpenAIChatCase(context.Background(), nil, RunConfig{Model: "audit-model"}, definition)

	assert.Equal(t, StatusUnknown, result.Status)
	assert.Contains(t, result.Evidence, "指纹基线")
	assert.Empty(t, result.Exchanges)
}
