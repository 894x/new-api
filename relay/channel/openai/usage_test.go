package openai

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
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForceFormattedChatUsageIncludesResponsesAliases(t *testing.T) {
	body := `{"id":"chatcmpl-usage","object":"chat.completion","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":5}}}`
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, false)
	info.ChannelType = constant.ChannelTypeOpenAI

	usage, apiErr := OpenaiHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.InputTokens)
	assert.Nil(t, usage.OutputTokensDetails)
	assert.Equal(t, int64(100), gjson.GetBytes(recorder.Body.Bytes(), "usage.input_tokens").Int())
	assert.Equal(t, int64(20), gjson.GetBytes(recorder.Body.Bytes(), "usage.output_tokens").Int())
	assert.Equal(t, int64(40), gjson.GetBytes(recorder.Body.Bytes(), "usage.input_tokens_details.cached_tokens").Int())
	assert.Equal(t, int64(5), gjson.GetBytes(recorder.Body.Bytes(), "usage.output_tokens_details.reasoning_tokens").Int())
	assert.Equal(t, int64(120), gjson.GetBytes(recorder.Body.Bytes(), "usage.total_tokens").Int())
}

func TestForceFormattedChatStreamUsageIncludesResponsesAliases(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":5}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, true)
	info.ChannelType = constant.ChannelTypeOpenAI

	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.InputTokens)
	assert.Contains(t, recorder.Body.String(), `"input_tokens":100`)
	assert.Contains(t, recorder.Body.String(), `"output_tokens":20`)
	assert.Contains(t, recorder.Body.String(), `"input_tokens_details":{"cached_tokens":40`)
	assert.Contains(t, recorder.Body.String(), `"output_tokens_details":{"text_tokens":0,"audio_tokens":0,"image_tokens":0,"reasoning_tokens":5}`)
}

func TestForceFormattedChatStreamEstimatedUsageIncludesResponsesAliases(t *testing.T) {
	body := "data: " + `{"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, true)
	info.ChannelType = constant.ChannelTypeOpenAI
	info.SetEstimatePromptTokens(7)

	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 7, usage.PromptTokens)

	var usageFrame string
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if strings.HasPrefix(line, "data: {") && gjson.Get(strings.TrimPrefix(line, "data: "), "usage").Exists() {
			usageFrame = strings.TrimPrefix(line, "data: ")
		}
	}
	require.NotEmpty(t, usageFrame)
	assert.Equal(t, int64(7), gjson.Get(usageFrame, "usage.input_tokens").Int())
	assert.Equal(t, int64(usage.CompletionTokens), gjson.Get(usageFrame, "usage.output_tokens").Int())
	assert.Equal(t, int64(usage.TotalTokens), gjson.Get(usageFrame, "usage.total_tokens").Int())
}

func TestForceFormattedResponsesUsageIncludesChatAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, common.NewRequestId())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{ForceFormat: true}}}
	body := `{"id":"resp_usage","object":"response","created_at":1710000000,"status":"completed","model":"gpt-test","output":[],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cached_tokens":40},"output_tokens_details":{"reasoning_tokens":5},"provider_tokens":3}}`
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}

	usage, apiErr := OaiResponsesHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, int64(100), gjson.GetBytes(recorder.Body.Bytes(), "usage.prompt_tokens").Int())
	assert.Equal(t, int64(20), gjson.GetBytes(recorder.Body.Bytes(), "usage.completion_tokens").Int())
	assert.Equal(t, int64(40), gjson.GetBytes(recorder.Body.Bytes(), "usage.prompt_tokens_details.cached_tokens").Int())
	assert.Equal(t, int64(5), gjson.GetBytes(recorder.Body.Bytes(), "usage.completion_tokens_details.reasoning_tokens").Int())
	assert.Equal(t, int64(120), gjson.GetBytes(recorder.Body.Bytes(), "usage.total_tokens").Int())
	assert.Equal(t, int64(3), gjson.GetBytes(recorder.Body.Bytes(), "usage.provider_tokens").Int())
}

func TestForceFormattedResponsesStreamUsageIncludesChatAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, common.NewRequestId())
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{ForceFormat: true}}}
	event := `{"type":"response.completed","response":{"id":"resp_usage","object":"response","created_at":1710000000,"status":"completed","model":"gpt-test","output":[],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cached_tokens":40},"output_tokens_details":{"reasoning_tokens":5}}}}`
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: " + event + "\n\ndata: [DONE]\n\n")), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}

	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Contains(t, recorder.Body.String(), `"prompt_tokens":100`)
	assert.Contains(t, recorder.Body.String(), `"completion_tokens":20`)
	assert.Contains(t, recorder.Body.String(), `"prompt_tokens_details":{"cached_tokens":40`)
	assert.Contains(t, recorder.Body.String(), `"completion_tokens_details":{"reasoning_tokens":5}`)
}

func TestConvertedUsageIncludesBothDetailFamilies(t *testing.T) {
	chatUsage := relayconvert.UsageFromChatUsage(&dto.Usage{
		PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120,
		PromptTokensDetails:    dto.InputTokenDetails{CachedTokens: 40},
		CompletionTokenDetails: dto.OutputTokenDetails{ReasoningTokens: 5},
	})
	require.NotNil(t, chatUsage.InputTokensDetails)
	require.NotNil(t, chatUsage.OutputTokensDetails)
	assert.Equal(t, 40, chatUsage.InputTokensDetails.CachedTokens)
	assert.Equal(t, 5, chatUsage.OutputTokensDetails.ReasoningTokens)
	assert.Equal(t, 120, chatUsage.TotalTokens)

	responsesUsage := relayconvert.UsageFromResponsesUsage(&dto.Usage{
		InputTokens: 100, OutputTokens: 20, TotalTokens: 120,
		InputTokensDetails:  &dto.InputTokenDetails{CachedTokens: 40},
		OutputTokensDetails: &dto.OutputTokenDetails{ReasoningTokens: 5},
	})
	assert.Equal(t, 100, responsesUsage.PromptTokens)
	assert.Equal(t, 20, responsesUsage.CompletionTokens)
	assert.Equal(t, 40, responsesUsage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 5, responsesUsage.CompletionTokenDetails.ReasoningTokens)
	assert.Equal(t, 120, responsesUsage.TotalTokens)
	zero := 0
	pointerOnlyDetails := relayconvert.UsageFromChatUsage(&dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{CachedTokensDetails: &dto.CachedTokenDetails{TextTokens: &zero}},
	})
	require.NotNil(t, pointerOnlyDetails.InputTokensDetails)
	require.NotNil(t, pointerOnlyDetails.InputTokensDetails.CachedTokensDetails)
	require.NotNil(t, pointerOnlyDetails.InputTokensDetails.CachedTokensDetails.TextTokens)
	assert.Equal(t, 0, *pointerOnlyDetails.InputTokensDetails.CachedTokensDetails.TextTokens)

	alreadyReported := &dto.Usage{PromptTokens: 100, InputTokens: 90, CompletionTokens: 20, OutputTokens: 19}
	alreadyReported.FillOpenAIUsageAliases(types.RelayFormatOpenAI)
	assert.Equal(t, 90, alreadyReported.InputTokens)
	assert.Equal(t, 19, alreadyReported.OutputTokens)
}

func TestOpenaiHandlerAddsTopLevelCachedTokensWithoutForceFormat(t *testing.T) {
	body := `{"id":"chatcmpl-cache","object":"chat.completion","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":86}}}`
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, false)
	info.ChannelType = constant.ChannelTypeOpenAI
	info.ChannelSetting.ForceFormat = false

	usage, apiErr := OpenaiHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	assert.Equal(t, 86, usage.CachedTokens)
	assert.Equal(t, int64(86), gjson.GetBytes(recorder.Body.Bytes(), "usage.cached_tokens").Int())
	assert.Equal(t, int64(86), gjson.GetBytes(recorder.Body.Bytes(), "usage.prompt_tokens_details.cached_tokens").Int())
}

func TestOaiStreamHandlerAddsTopLevelCachedTokensWithoutForceFormat(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-cache","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-cache","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":86}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, true)
	info.ChannelType = constant.ChannelTypeOpenAI
	info.ChannelSetting.ForceFormat = false

	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 86, usage.CachedTokens)

	foundUsage := false
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if !gjson.Get(payload, "usage").Exists() {
			continue
		}
		foundUsage = true
		assert.Equal(t, int64(86), gjson.Get(payload, "usage.cached_tokens").Int())
		assert.Equal(t, int64(86), gjson.Get(payload, "usage.prompt_tokens_details.cached_tokens").Int())
	}
	assert.True(t, foundUsage)
}
