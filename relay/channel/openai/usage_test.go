package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

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
