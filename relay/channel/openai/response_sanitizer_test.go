package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const chatUsageWithForeignFields = `"prompt_tokens":11,"completion_tokens":3,"total_tokens":14,"cached_tokens":2,"input_tokens":11,"output_tokens":3,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":1},"usage_semantic":"openai","usage_source":"anthropic","billing_usage":{"source":"claude_messages","semantic":"anthropic","claude_usage":{"input_tokens":9,"cache_read_input_tokens":2,"output_tokens":3}},"prompt_tokens_details":{"cached_tokens":2,"cached_creation_tokens":2,"text_tokens":9},"completion_tokens_details":{"reasoning_tokens":1},"claude_cache_creation_5_m_tokens":2,"claude_cache_creation_1_h_tokens":0`

const responsesUsageWithForeignFields = `"input_tokens":11,"output_tokens":3,"total_tokens":14,"prompt_tokens":11,"completion_tokens":3,"cached_tokens":2,"prompt_cache_hit_tokens":2,"usage_semantic":"openai","usage_source":"oai_chat","billing_usage":{"source":"oai_chat","semantic":"openai","openai_usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}},"prompt_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":1},"input_tokens_details":{"cached_tokens":2,"cached_creation_tokens":2,"cache_write_tokens":2},"output_tokens_details":{"reasoning_tokens":1},"claude_cache_creation_5_m_tokens":2,"claude_cache_creation_1_h_tokens":0`

func assertChatUsageSanitized(t *testing.T, body, usagePath string) {
	t.Helper()
	for _, field := range []string{
		"billing_usage", "usage_semantic", "usage_source",
		"input_tokens", "output_tokens", "input_tokens_details", "output_tokens_details",
		"claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens",
		"prompt_tokens_details.cached_creation_tokens",
	} {
		assert.False(t, gjson.Get(body, usagePath+field).Exists(), field)
	}
	assert.EqualValues(t, 11, gjson.Get(body, usagePath+"prompt_tokens").Int())
	assert.EqualValues(t, 3, gjson.Get(body, usagePath+"completion_tokens").Int())
	assert.EqualValues(t, 14, gjson.Get(body, usagePath+"total_tokens").Int())
	assert.EqualValues(t, 2, gjson.Get(body, usagePath+"prompt_tokens_details.cached_tokens").Int())
}

func assertResponsesUsageSanitized(t *testing.T, body, usagePath string) {
	t.Helper()
	for _, field := range []string{
		"billing_usage", "usage_semantic", "usage_source",
		"prompt_tokens", "completion_tokens", "cached_tokens", "prompt_cache_hit_tokens",
		"prompt_tokens_details", "completion_tokens_details",
		"claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens",
		"input_tokens_details.cached_creation_tokens",
	} {
		assert.False(t, gjson.Get(body, usagePath+field).Exists(), field)
	}
	assert.EqualValues(t, 11, gjson.Get(body, usagePath+"input_tokens").Int())
	assert.EqualValues(t, 3, gjson.Get(body, usagePath+"output_tokens").Int())
	assert.EqualValues(t, 14, gjson.Get(body, usagePath+"total_tokens").Int())
	assert.EqualValues(t, 2, gjson.Get(body, usagePath+"input_tokens_details.cached_tokens").Int())
}

func newResponsesSanitizationTestContext(t *testing.T, body string, isStream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, common.NewRequestId())
	info := &relaycommon.RelayInfo{
		OriginModelName: "kimi-k3",
		RelayFormat:     types.RelayFormatOpenAIResponses,
		DisablePing:     true,
		IsStream:        isStream,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "kimi-k3"},
	}
	contentType := "application/json"
	if isStream {
		contentType = "text/event-stream"
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{contentType}},
	}
	return c, recorder, resp, info
}

func ssePayloadWithUsage(t *testing.T, body, usagePath string) string {
	t.Helper()
	for line := range strings.SplitSeq(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if gjson.Get(payload, usagePath).Exists() {
			return payload
		}
	}
	t.Fatalf("no SSE payload contains %s", usagePath)
	return ""
}

func TestChatResponsesOnlyExposeChatUsage(t *testing.T) {
	t.Run("native non-stream", func(t *testing.T) {
		body := `{"id":"chatcmpl_1","object":"chat.completion","created":1710000000,"model":"kimi-k3","choices":[],"usage":{` + chatUsageWithForeignFields + `}}`
		c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, false)

		usage, apiErr := OpenaiHandler(c, info, resp)
		require.Nil(t, apiErr)
		assertChatUsageSanitized(t, recorder.Body.String(), "usage.")
		require.NotNil(t, usage.BillingUsage)
		assert.Equal(t, dto.BillingUsageSemanticAnthropic, usage.BillingUsage.Semantic)
	})

	t.Run("native stream", func(t *testing.T) {
		body := strings.Join([]string{
			`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`,
			`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[],"usage":{` + chatUsageWithForeignFields + `}}`,
			`data: [DONE]`,
			``,
		}, "\n")
		c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, true)
		info.ChannelSetting.ForceFormat = false

		usage, apiErr := OaiStreamHandler(c, info, resp)
		require.Nil(t, apiErr)
		payload := ssePayloadWithUsage(t, recorder.Body.String(), "usage")
		assertChatUsageSanitized(t, payload, "usage.")
		require.NotNil(t, usage.BillingUsage)
		assert.Equal(t, dto.BillingUsageSemanticAnthropic, usage.BillingUsage.Semantic)
	})

	t.Run("converted from responses", func(t *testing.T) {
		body := `{"id":"resp_1","object":"response","created_at":1710000000,"status":"completed","model":"kimi-k3","output":[],"usage":{` + responsesUsageWithForeignFields + `}}`
		c, recorder, resp, info := newResponsesSanitizationTestContext(t, body, false)
		info.RelayFormat = types.RelayFormatOpenAI

		usage, apiErr := OaiResponsesToChatHandler(c, info, resp)
		require.Nil(t, apiErr)
		assertChatUsageSanitized(t, recorder.Body.String(), "usage.")
		require.NotNil(t, usage.BillingUsage)
		assert.Equal(t, dto.BillingUsageSemanticOpenAI, usage.BillingUsage.Semantic)
	})
}

func TestResponsesOnlyExposeResponsesUsage(t *testing.T) {
	t.Run("native non-stream", func(t *testing.T) {
		body := `{"id":"resp_1","object":"response","created_at":1710000000,"status":"completed","model":"kimi-k3","output":[],"usage":{` + responsesUsageWithForeignFields + `}}`
		c, recorder, resp, info := newResponsesSanitizationTestContext(t, body, false)

		usage, apiErr := OaiResponsesHandler(c, info, resp)
		require.Nil(t, apiErr)
		assertResponsesUsageSanitized(t, recorder.Body.String(), "usage.")
		assert.EqualValues(t, 2, gjson.Get(recorder.Body.String(), "usage.input_tokens_details.cache_write_tokens").Int())
		assert.EqualValues(t, 1, gjson.Get(recorder.Body.String(), "usage.output_tokens_details.reasoning_tokens").Int())
		require.NotNil(t, usage.BillingUsage)
		assert.Equal(t, dto.BillingUsageSemanticOpenAI, usage.BillingUsage.Semantic)
	})

	t.Run("native compaction", func(t *testing.T) {
		body := `{"id":"cmp_1","object":"response.compaction","created_at":1710000000,"output":[],"usage":{` + responsesUsageWithForeignFields + `}}`
		c, recorder, resp, _ := newResponsesSanitizationTestContext(t, body, false)

		usage, apiErr := OaiResponsesCompactionHandler(c, resp)
		require.Nil(t, apiErr)
		assertResponsesUsageSanitized(t, recorder.Body.String(), "usage.")
		assert.EqualValues(t, 14, usage.TotalTokens)
	})

	t.Run("native stream", func(t *testing.T) {
		body := `data: {"type":"response.completed","response":{"id":"resp_1","object":"response","created_at":1710000000,"status":"completed","model":"kimi-k3","output":[],"usage":{` + responsesUsageWithForeignFields + `}}}` + "\n\n"
		c, recorder, resp, info := newResponsesSanitizationTestContext(t, body, true)

		usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
		require.Nil(t, apiErr)
		payload := ssePayloadWithUsage(t, recorder.Body.String(), "response.usage")
		assertResponsesUsageSanitized(t, payload, "response.usage.")
		require.NotNil(t, usage.BillingUsage)
		assert.Equal(t, dto.BillingUsageSemanticOpenAI, usage.BillingUsage.Semantic)
	})

	t.Run("converted from chat", func(t *testing.T) {
		body := `{"id":"chatcmpl_1","object":"chat.completion","created":1710000000,"model":"kimi-k3","choices":[],"usage":{` + chatUsageWithForeignFields + `}}`
		c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, false)
		info.RelayFormat = types.RelayFormatOpenAIResponses

		usage, apiErr := OaiChatToResponsesHandler(c, info, resp)
		require.Nil(t, apiErr)
		assertResponsesUsageSanitized(t, recorder.Body.String(), "usage.")
		require.NotNil(t, usage.BillingUsage)
		assert.Equal(t, dto.BillingUsageSemanticAnthropic, usage.BillingUsage.Semantic)
	})
}
