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
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaClaudeModeRequestContract(t *testing.T) {
	for _, tc := range []struct {
		name, setting, path string
		native              bool
	}{
		{"legacy configuration", `{}`, "/api/chat", false},
		{"explicit conversion", `{"ollama_native_claude":false}`, "/api/chat", false},
		{"native opt in", `{"ollama_native_claude":true}`, "/v1/messages", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			ctx.Request.Header.Set("anthropic-beta", "test-beta")
			ctx.Request.Header.Set("anthropic-version", "2023-06-01")
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, RelayMode: relayconstant.RelayModeChatCompletions,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOllama, ChannelBaseUrl: "https://ollama.example", ApiKey: "test-key"}}
			require.NoError(t, common.UnmarshalJsonStr(tc.setting, &info.ChannelSetting))
			var request dto.ClaudeRequest
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"llama3","max_tokens":128,"messages":[{"role":"user","content":"hello"}],"stream":false}`, &request))
			adaptor := &Adaptor{}
			converted, err := adaptor.ConvertClaudeRequest(ctx, info, &request)
			require.NoError(t, err)
			if tc.native {
				assert.Same(t, &request, converted)
			} else {
				chat, ok := converted.(*OllamaChatRequest)
				require.True(t, ok)
				assert.Equal(t, "llama3", chat.Model)
				require.Len(t, chat.Messages, 1)
				assert.Equal(t, "hello", chat.Messages[0].Content)
			}
			url, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://ollama.example"+tc.path, url)
			headers := make(http.Header)
			require.NoError(t, adaptor.SetupRequestHeader(ctx, &headers, info))
			assert.Equal(t, "Bearer test-key", headers.Get("Authorization"))
			assert.Empty(t, headers.Get("x-api-key"))
			if tc.native {
				assert.Equal(t, "2023-06-01", headers.Get("anthropic-version"))
				assert.Equal(t, "test-beta", headers.Get("anthropic-beta"))
			} else {
				assert.Empty(t, headers.Get("anthropic-version"))
				assert.Empty(t, headers.Get("anthropic-beta"))
			}
		})
	}
}

func TestOllamaClaudeModeResponseDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, body, responseKey string
		native                  bool
	}{
		{"conversion", `{"model":"llama3","message":{"role":"assistant","content":"hello"},"done":true,"prompt_eval_count":3,"eval_count":2}`, "choices", false},
		{"native", `{"id":"msg_test","type":"message","role":"assistant","model":"llama3","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`, "content", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, RelayMode: relayconstant.RelayModeChatCompletions,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOllama, UpstreamModelName: "llama3", ChannelSetting: dto.ChannelSettings{OllamaNativeClaude: tc.native}}}
			response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
			usage, apiErr := (&Adaptor{}).DoResponse(ctx, response, info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 5, usage.(*dto.Usage).TotalTokens)
			var result map[string]any
			require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &result))
			assert.Contains(t, result, tc.responseKey)
			assert.Contains(t, writer.Body.String(), "hello")
		})
	}
}

func TestOllamaClaudeConversionUsesOpenAIChatWhenEnabled(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOllama, ChannelBaseUrl: "https://ollama.example",
			ChannelOtherSettings: dto.ChannelOtherSettings{OllamaOpenAIChat: true}}}
	var request dto.ClaudeRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"llama3","max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`, &request))
	converted, err := (&Adaptor{}).ConvertClaudeRequest(c, info, &request)
	require.NoError(t, err)
	chat, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok, "OpenAI endpoint must receive an OpenAI request, not Ollama options")
	assert.Equal(t, "llama3", chat.Model)
	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://ollama.example/v1/chat/completions", url)
}

func TestOllamaOpenAIEndpointsRemainIndependentOfClaudeMode(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, tc := range []struct {
			mode int
			path string
		}{
			{relayconstant.RelayModeChatCompletions, "/api/chat"},
			{relayconstant.RelayModeCompletions, "/api/generate"},
			{relayconstant.RelayModeEmbeddings, "/api/embed"},
			{relayconstant.RelayModeResponses, "/v1/responses"},
			{relayconstant.RelayModeResponsesCompact, "/v1/responses/compact"},
		} {
			info := &relaycommon.RelayInfo{RelayMode: tc.mode, RelayFormat: types.RelayFormatOpenAI,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://ollama.example", ChannelSetting: dto.ChannelSettings{OllamaNativeClaude: enabled}}}
			url, err := (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://ollama.example"+tc.path, url)
		}
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	request := dto.OpenAIResponsesRequest{Model: "llama3"}
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(ctx, info, request)
	assert.NoError(t, err)
	assert.NotNil(t, converted)
}

func TestOllamaClaudeStreamingModeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name, body, terminal string
		native               bool
	}{
		{"conversion", "{\"model\":\"llama3\",\"message\":{\"role\":\"assistant\",\"content\":\"hello\"},\"done\":false}\n{\"model\":\"llama3\",\"done\":true,\"prompt_eval_count\":3,\"eval_count\":2}\n", "[DONE]", false},
		{"native", "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"model\":\"llama3\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", "message_stop", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatClaude, RelayMode: relayconstant.RelayModeChatCompletions,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOllama, UpstreamModelName: "llama3", ChannelSetting: dto.ChannelSettings{OllamaNativeClaude: tc.native}}}
			response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
			usage, apiErr := (&Adaptor{}).DoResponse(ctx, response, info)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 5, usage.(*dto.Usage).TotalTokens)
			assert.Contains(t, writer.Body.String(), "hello")
			assert.Contains(t, writer.Body.String(), tc.terminal)
		})
	}
}
