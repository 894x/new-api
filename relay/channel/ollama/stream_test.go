package ollama

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaChatHandlerNonStreamToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "compact json per-line parse path",
			raw:  `{"model":"llama3.1","created_at":"2026-05-27T12:00:00Z","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"get_weather","arguments":{"city":"Paris","days":0}}}]},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":7}`,
		},
		{
			name: "pretty json fallback parse path",
			raw: `{
  "model": "llama3.1",
  "created_at": "2026-05-27T12:00:00Z",
  "message": {
    "role": "assistant",
    "content": "",
    "tool_calls": [
      {
        "function": {
          "name": "get_weather",
          "arguments": {
            "city": "Paris",
            "days": 0
          }
        }
      }
    ]
  },
  "done": true,
  "done_reason": "stop",
  "prompt_eval_count": 5,
  "eval_count": 7
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.raw)),
			}

			usage, apiErr := ollamaChatHandler(c, &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fallback-model"},
			}, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 12, usage.TotalTokens)

			var out dto.OpenAITextResponse
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
			require.Len(t, out.Choices, 1)
			assert.Equal(t, constant.FinishReasonToolCalls, out.Choices[0].FinishReason)

			var toolCalls []dto.ToolCallResponse
			require.NoError(t, common.Unmarshal(out.Choices[0].Message.ToolCalls, &toolCalls))
			require.Len(t, toolCalls, 1)
			assert.NotEmpty(t, toolCalls[0].ID)
			assert.Equal(t, "function", toolCalls[0].Type)
			assert.Equal(t, "get_weather", toolCalls[0].Function.Name)
			assert.Nil(t, toolCalls[0].Index)

			var args map[string]any
			require.NoError(t, common.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args))
			assert.Equal(t, "Paris", args["city"])
			assert.Equal(t, float64(0), args["days"])
		})
	}
}

func TestOllamaStreamAttemptObservesOnlyVisibleContentAndLifecycle(t *testing.T) {
	body := strings.Join([]string{
		`{"model":"llama3.1","message":{"role":"assistant","content":"","thinking":"hidden"},"done":false}`,
		`{"model":"llama3.1","message":{"role":"assistant","content":"hel"},"done":false}`,
		`{"model":"llama3.1","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"lookup","arguments":{}}}]},"done":false}`,
		`{"model":"llama3.1","message":{"role":"assistant","content":"lo"},"done":false}`,
		`{"model":"llama3.1","done":true,"done_reason":"stop","prompt_eval_count":2,"eval_count":3}`,
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama3.1"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()

	usage, apiErr := ollamaStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(body))})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, "hello", info.DynamicRoutingAttemptVisibleText())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	sample, observed := info.FinishDynamicRoutingAttempt(nil)
	require.True(t, observed)
	assert.True(t, sample.Success)
}

func TestOllamaStreamErrorEnvelopeIsHardFailure(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama3.1"}}
	info.BeginDynamicRoutingAttempt(4, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(http.StatusOK)

	_, handlerErr := ollamaStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"error":"model runner failed"}`))})
	require.NotNil(t, handlerErr)
	assert.Equal(t, http.StatusBadGateway, handlerErr.StatusCode)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestOllamaDoneOnlyStreamIsProtocolFailure(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "llama3.1"}}
	info.BeginDynamicRoutingAttempt(4, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(http.StatusOK)

	_, handlerErr := ollamaStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"model":"llama3.1","done":true,"done_reason":"stop"}`))})
	require.Nil(t, handlerErr)
	sample, observed := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}
