package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestK3EmptyReasoningUsagePreservesSettlement(t *testing.T) {
	for _, tc := range []struct {
		name       string
		model      string
		message    string
		details    string
		incomplete bool
		wantChat   int64
		wantOutput int64
	}{
		{name: "chat details only", details: `"completion_tokens_details":{"reasoning_tokens":1}`},
		{name: "both detail families", details: `"completion_tokens_details":{"reasoning_tokens":1},"output_tokens_details":{"reasoning_tokens":1}`},
		{name: "output details only", details: `"output_tokens_details":{"reasoning_tokens":1}`},
		{name: "empty fields", message: `,"reasoning_content":"","reasoning":null`, details: `"completion_tokens_details":{"reasoning_tokens":1}`},
		{name: "actual reasoning", message: `,"reasoning_content":"think"`, details: `"completion_tokens_details":{"reasoning_tokens":1}`, wantChat: 1, wantOutput: 1},
		{name: "reasoning alias", message: `,"reasoning":"think"`, details: `"completion_tokens_details":{"reasoning_tokens":1}`, wantChat: 1, wantOutput: 1},
		{name: "nonempty alias with empty canonical field", message: `,"reasoning_content":"","reasoning":"think"`, details: `"completion_tokens_details":{"reasoning_tokens":1}`, wantChat: 1, wantOutput: 1},
		{name: "larger count", details: `"completion_tokens_details":{"reasoning_tokens":5}`, wantChat: 5, wantOutput: 5},
		{name: "other model", model: "gpt-test", details: `"completion_tokens_details":{"reasoning_tokens":1}`, wantChat: 1, wantOutput: 1},
		{name: "k3 model variant", model: "kimi-k3-pro", details: `"completion_tokens_details":{"reasoning_tokens":1}`},
		{name: "conflicting positive aliases", details: `"completion_tokens_details":{"reasoning_tokens":1},"output_tokens_details":{"reasoning_tokens":5}`, wantChat: 1, wantOutput: 5},
		{name: "conflicting zero alias", details: `"completion_tokens_details":{"reasoning_tokens":1},"output_tokens_details":{"reasoning_tokens":0}`, wantChat: 1},
		{name: "incomplete response", incomplete: true, details: `"completion_tokens_details":{"reasoning_tokens":1}`, wantChat: 1, wantOutput: 1},
	} {
		for _, stream := range []bool{false, true} {
			for _, forceFormat := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/format=%t", tc.name, stream, forceFormat), func(t *testing.T) {
					usageJSON := `{"prompt_tokens":100,"completion_tokens":20,"input_tokens":100,"output_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":40,"cache_write_tokens":25},` + tc.details + `}`
					body := `{"id":"chatcmpl-reasoning","object":"chat.completion","model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"` + tc.message + `},"finish_reason":"stop"}],"usage":` + usageJSON + `}`
					if stream {
						body = `data: {"id":"chatcmpl-reasoning","object":"chat.completion.chunk","model":"kimi-k3","choices":[{"index":0,"delta":{"content":"ok"` + tc.message + `},"finish_reason":"stop"}]}` + "\n\n" +
							`data: {"id":"chatcmpl-reasoning","object":"chat.completion.chunk","model":"kimi-k3","choices":[],"usage":` + usageJSON + `}` + "\n\ndata: [DONE]\n\n"
					}
					if tc.incomplete {
						body = strings.ReplaceAll(body, `"finish_reason":"stop"`, `"finish_reason":""`)
						body = strings.ReplaceAll(body, "data: [DONE]\n\n", "")
					}
					c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, stream)
					info.ChannelSetting.ForceFormat = forceFormat
					info.ChannelType = constant.ChannelTypeOpenAI
					if tc.model != "" {
						info.UpstreamModelName = tc.model
					}
					handler := OpenaiHandler
					if stream {
						handler = OaiStreamHandler
					}
					usage, apiErr := handler(c, info, resp)
					require.Nil(t, apiErr)
					require.NotNil(t, usage)
					assert.Equal(t, 100, usage.PromptTokens)
					assert.Equal(t, 20, usage.CompletionTokens)
					assert.Equal(t, 120, usage.TotalTokens)
					assert.Equal(t, 40, usage.PromptTokensDetails.CachedTokens)
					assert.Equal(t, 25, usage.PromptTokensDetails.CacheWriteTokens)
					assert.Equal(t, int(gjson.Get(usageJSON, "completion_tokens_details.reasoning_tokens").Int()), usage.CompletionTokenDetails.ReasoningTokens)
					if !stream && gjson.Get(usageJSON, "output_tokens_details").Exists() {
						require.NotNil(t, usage.OutputTokensDetails)
						assert.Equal(t, int(gjson.Get(usageJSON, "output_tokens_details.reasoning_tokens").Int()), usage.OutputTokensDetails.ReasoningTokens)
					}
					payload := recorder.Body.String()
					if stream {
						payload = ""
						for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
							data, ok := strings.CutPrefix(line, "data: ")
							if ok && gjson.Get(data, "usage.prompt_tokens").Int() > 0 {
								require.Empty(t, payload, "usage must not be duplicated")
								payload = data
							}
						}
					}
					require.NotEmpty(t, payload)
					for path, want := range map[string]int64{"prompt_tokens": 100, "completion_tokens": 20, "input_tokens": 100, "output_tokens": 20, "total_tokens": 120} {
						assert.Equal(t, want, gjson.Get(payload, "usage."+path).Int(), path)
					}
					assert.Equal(t, tc.wantChat, gjson.Get(payload, "usage.completion_tokens_details.reasoning_tokens").Int())
					output := gjson.Get(payload, "usage.output_tokens_details.reasoning_tokens")
					if forceFormat || gjson.Get(usageJSON, "output_tokens_details").Exists() {
						require.True(t, output.Exists())
						assert.Equal(t, tc.wantOutput, output.Int())
					} else {
						assert.False(t, output.Exists())
					}
				})
			}
		}
	}
}

func TestK3StreamReasoningUsageWaitsForCompleteResponse(t *testing.T) {
	usageJSON := `{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"completion_tokens_details":{"reasoning_tokens":1}}`
	for _, tc := range []struct {
		name       string
		later      string
		terminal   string
		want       int64
		wantError  bool
		conversion bool
	}{
		{name: "penultimate usage", terminal: "data: [DONE]\n\n"},
		{name: "later reasoning on another choice", later: `{"choices":[{"index":1,"delta":{"reasoning_content":"think"}}]}`, terminal: "data: [DONE]\n\n", want: 1},
		{name: "reasoning converted to content", later: `{"choices":[{"index":0,"delta":{"reasoning_content":"think"}}]}`, terminal: "data: [DONE]\n\n", want: 1, conversion: true},
		{name: "missing terminal", want: 1},
		{name: "upstream error", later: `{"error":{"message":"upstream failed","type":"server_error","code":"upstream_error"}}`, want: 1, wantError: true},
	} {
		for _, forceFormat := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/format=%t", tc.name, forceFormat), func(t *testing.T) {
				// Usage shares a frame with text, followed by an empty event. Text
				// must be delivered once; the usage waits for later reasoning/errors.
				body := `data: {"id":"chatcmpl-early","model":"kimi-k3","choices":[{"index":0,"delta":{"content":"ok"}}],"usage":` + usageJSON + `}` + "\n\n" +
					`data: {"id":"chatcmpl-early","model":"kimi-k3","choices":[]}` + "\n\n"
				if tc.later != "" {
					body += "data: " + tc.later + "\n\n"
				}
				body += tc.terminal
				c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, true)
				info.ChannelSetting.ForceFormat = forceFormat
				info.ChannelSetting.ThinkingToContent = tc.conversion
				info.ThinkingContentInfo.IsFirstThinkingContent = true
				usage, apiErr := OaiStreamHandler(c, info, resp)
				if tc.wantError {
					require.NotNil(t, apiErr)
				} else {
					require.Nil(t, apiErr)
					require.NotNil(t, usage)
					assert.Equal(t, 20, usage.CompletionTokens)
					assert.Equal(t, 120, usage.TotalTokens)
					assert.Equal(t, 1, usage.CompletionTokenDetails.ReasoningTokens)
				}
				assert.Equal(t, 1, strings.Count(recorder.Body.String(), `"content":"ok"`))
				found := 0
				for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
					payload, ok := strings.CutPrefix(line, "data: ")
					if !ok || gjson.Get(payload, "usage.prompt_tokens").Int() == 0 {
						continue
					}
					found++
					assert.Equal(t, tc.want, gjson.Get(payload, "usage.completion_tokens_details.reasoning_tokens").Int())
					assert.Equal(t, int64(20), gjson.Get(payload, "usage.completion_tokens").Int())
					assert.Equal(t, int64(120), gjson.Get(payload, "usage.total_tokens").Int())
				}
				assert.Equal(t, 1, found)
			})
		}
	}
}

func TestK3StreamPreservesUsageSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		metadata             string
		lastCount            int
		excludeTerminalUsage bool
		want                 []int64
	}{
		{name: "all snapshots normalized", lastCount: 1, want: []int64{0, 0}},
		{name: "later larger count", lastCount: 5, want: []int64{1, 5}},
		{name: "larger terminal usage excluded by client", lastCount: 5, excludeTerminalUsage: true, want: []int64{1}},
		// A large provider extension exercises the bounded-buffer fallback,
		// rather than silently discarding usage or buffering unbounded data.
		{name: "oversized provider metadata", metadata: strings.Repeat("x", 65<<10), lastCount: 1, want: []int64{1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `data: {"id":"chatcmpl-snapshots","choices":[{"index":0,"delta":{"content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"completion_tokens_details":{"reasoning_tokens":1}}}` + "\n\n" +
				fmt.Sprintf(`data: {"id":"chatcmpl-snapshots","choices":[],"provider_metadata":"%s","usage":{"prompt_tokens":100,"completion_tokens":21,"total_tokens":121,"completion_tokens_details":{"reasoning_tokens":%d}}}`, tc.metadata, tc.lastCount) + "\n\n" +
				`data: {"id":"chatcmpl-snapshots","choices":[]}` + "\n\ndata: [DONE]\n\n"
			c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, true)
			info.ChannelSetting.ForceFormat = false
			if tc.excludeTerminalUsage {
				body = strings.Replace(body, `data: {"id":"chatcmpl-snapshots","choices":[]}`+"\n\n", "", 1)
				resp.Body = io.NopCloser(strings.NewReader(body))
				info.ShouldIncludeUsage = false
			}
			usage, apiErr := OaiStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 21, usage.CompletionTokens)
			assert.Equal(t, 121, usage.TotalTokens)
			assert.Equal(t, tc.lastCount, usage.CompletionTokenDetails.ReasoningTokens)
			var snapshots []gjson.Result
			for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
				payload, ok := strings.CutPrefix(line, "data: ")
				if ok && gjson.Get(payload, "usage").IsObject() {
					snapshots = append(snapshots, gjson.Get(payload, "usage"))
				}
			}
			require.Len(t, snapshots, len(tc.want))
			for i, snapshot := range snapshots {
				assert.Equal(t, tc.want[i], snapshot.Get("completion_tokens_details.reasoning_tokens").Int())
				assert.Equal(t, int64(20+i), snapshot.Get("completion_tokens").Int())
				assert.Equal(t, int64(120+i), snapshot.Get("total_tokens").Int())
			}
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), `"content":"ok"`))
		})
	}
}

type reasoningUsageNotifyingWriter struct {
	*httptest.ResponseRecorder
	content chan struct{}
}

func (w *reasoningUsageNotifyingWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(data)
	if strings.Contains(string(data), `"content":"ok"`) {
		select {
		case w.content <- struct{}{}:
		default:
		}
	}
	return n, err
}

func TestK3StreamForwardsTextBeforeTerminalAndPreservesCancelledUsage(t *testing.T) {
	for _, cancelStream := range []bool{false, true} {
		for _, forceFormat := range []bool{false, true} {
			t.Run(fmt.Sprintf("cancel=%t/format=%t", cancelStream, forceFormat), func(t *testing.T) {
				c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, "", true)
				writer := &reasoningUsageNotifyingWriter{ResponseRecorder: recorder, content: make(chan struct{}, 1)}
				wrapped, _ := gin.CreateTestContext(writer)
				wrapped.Request, wrapped.Keys = c.Request, c.Keys
				ctx, cancel := context.WithCancel(wrapped.Request.Context())
				t.Cleanup(cancel)
				wrapped.Request = wrapped.Request.WithContext(ctx)
				reader, upstream := io.Pipe()
				t.Cleanup(func() { _ = reader.Close(); _ = upstream.Close() })
				resp.Body = reader
				info.ChannelSetting.ForceFormat = forceFormat
				result := make(chan struct {
					usage *dto.Usage
					err   *types.NewAPIError
				}, 1)
				go func() {
					usage, apiErr := OaiStreamHandler(wrapped, info, resp)
					result <- struct {
						usage *dto.Usage
						err   *types.NewAPIError
					}{usage, apiErr}
				}()
				_, err := io.WriteString(upstream, `data: {"id":"chatcmpl-live","choices":[{"index":0,"delta":{"content":"ok"}}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"completion_tokens_details":{"reasoning_tokens":1}}}`+"\n\n"+
					`data: {"id":"chatcmpl-live","choices":[]}`+"\n\n")
				require.NoError(t, err)
				select {
				case <-writer.content:
					// The upstream is still open and has not sent its terminal.
				case <-time.After(5 * time.Second):
					t.Fatal("text was held until stream completion")
				}
				if cancelStream {
					cancel()
				} else {
					_, err = io.WriteString(upstream, "data: [DONE]\n\n")
					require.NoError(t, err)
				}
				select {
				case response := <-result:
					require.NotNil(t, response.usage)
					assert.Equal(t, 20, response.usage.CompletionTokens)
					assert.Equal(t, 120, response.usage.TotalTokens)
					assert.Equal(t, 1, response.usage.CompletionTokenDetails.ReasoningTokens)
					if cancelStream {
						require.NotNil(t, response.err)
						assert.Equal(t, types.ErrorCodeClientGone, response.err.GetErrorCode())
						assert.NotContains(t, recorder.Body.String(), `"reasoning_tokens":0`)
					} else {
						require.Nil(t, response.err)
						assert.Contains(t, recorder.Body.String(), `"reasoning_tokens":0`)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("stream handler did not finish")
				}
			})
		}
	}
}

func TestForceFormattedChatUsageIncludesResponsesAliases(t *testing.T) {
	body := `{"id":"chatcmpl-usage","object":"chat.completion","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":40,"cache_write_tokens":25},"completion_tokens_details":{"reasoning_tokens":5}}}`
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, false)
	info.ChannelType = constant.ChannelTypeOpenAI

	usage, apiErr := OpenaiHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.InputTokens)
	assert.Nil(t, usage.OutputTokensDetails)
	assert.Equal(t, 25, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Zero(t, usage.PromptTokensDetails.CachedCreationTokens)
	assert.Equal(t, int64(100), gjson.GetBytes(recorder.Body.Bytes(), "usage.input_tokens").Int())
	assert.Equal(t, int64(20), gjson.GetBytes(recorder.Body.Bytes(), "usage.output_tokens").Int())
	assert.Equal(t, int64(40), gjson.GetBytes(recorder.Body.Bytes(), "usage.input_tokens_details.cached_tokens").Int())
	assert.Equal(t, int64(25), gjson.GetBytes(recorder.Body.Bytes(), "usage.prompt_tokens_details.cached_creation_tokens").Int())
	assert.Equal(t, int64(25), gjson.GetBytes(recorder.Body.Bytes(), "usage.input_tokens_details.cached_creation_tokens").Int())
	assert.Equal(t, int64(5), gjson.GetBytes(recorder.Body.Bytes(), "usage.output_tokens_details.reasoning_tokens").Int())
	assert.Equal(t, int64(120), gjson.GetBytes(recorder.Body.Bytes(), "usage.total_tokens").Int())
}

func TestForceFormattedChatStreamUsageIncludesResponsesAliases(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-usage","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":40,"cache_write_tokens":25},"completion_tokens_details":{"reasoning_tokens":5}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, true)
	info.ChannelType = constant.ChannelTypeOpenAI

	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 0, usage.InputTokens)
	assert.Equal(t, 25, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Zero(t, usage.PromptTokensDetails.CachedCreationTokens)
	assert.Contains(t, recorder.Body.String(), `"input_tokens":100`)
	assert.Contains(t, recorder.Body.String(), `"output_tokens":20`)
	assert.Contains(t, recorder.Body.String(), `"input_tokens_details":{"cached_tokens":40`)
	assert.Contains(t, recorder.Body.String(), `"output_tokens_details":{"text_tokens":0,"audio_tokens":0,"image_tokens":0,"reasoning_tokens":5}`)
	foundUsage := false
	for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || gjson.Get(payload, "usage.prompt_tokens").Int() == 0 {
			continue
		}
		foundUsage = true
		assert.Equal(t, int64(25), gjson.Get(payload, "usage.prompt_tokens_details.cached_creation_tokens").Int())
		assert.Equal(t, int64(25), gjson.Get(payload, "usage.input_tokens_details.cached_creation_tokens").Int())
	}
	assert.True(t, foundUsage)
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
	body := `{"id":"resp_usage","object":"response","created_at":1710000000,"status":"completed","model":"gpt-test","output":[],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":25,"provider_tokens":3},"output_tokens_details":{"reasoning_tokens":5},"provider_tokens":3}}`
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}

	usage, apiErr := OaiResponsesHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 25, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Zero(t, usage.PromptTokensDetails.CachedCreationTokens)
	require.NotNil(t, usage.InputTokensDetails)
	assert.Zero(t, usage.InputTokensDetails.CachedCreationTokens)
	assert.Equal(t, int64(100), gjson.GetBytes(recorder.Body.Bytes(), "usage.prompt_tokens").Int())
	assert.Equal(t, int64(20), gjson.GetBytes(recorder.Body.Bytes(), "usage.completion_tokens").Int())
	assert.Equal(t, int64(40), gjson.GetBytes(recorder.Body.Bytes(), "usage.prompt_tokens_details.cached_tokens").Int())
	for _, detailName := range []string{"input_tokens_details", "prompt_tokens_details"} {
		assert.Equal(t, int64(25), gjson.GetBytes(recorder.Body.Bytes(), "usage."+detailName+".cached_creation_tokens").Int())
		assert.Equal(t, int64(3), gjson.GetBytes(recorder.Body.Bytes(), "usage."+detailName+".provider_tokens").Int())
	}
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
	event := `{"type":"response.completed","response":{"id":"resp_usage","object":"response","created_at":1710000000,"status":"completed","model":"gpt-test","output":[],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":25,"provider_tokens":3},"output_tokens_details":{"reasoning_tokens":5}}}}`
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: " + event + "\n\ndata: [DONE]\n\n")), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}

	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 25, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Zero(t, usage.PromptTokensDetails.CachedCreationTokens)
	require.NotNil(t, usage.InputTokensDetails)
	assert.Zero(t, usage.InputTokensDetails.CachedCreationTokens)
	assert.Contains(t, recorder.Body.String(), `"prompt_tokens":100`)
	assert.Contains(t, recorder.Body.String(), `"completion_tokens":20`)
	assert.Contains(t, recorder.Body.String(), `"prompt_tokens_details":{"cached_tokens":40`)
	assert.Contains(t, recorder.Body.String(), `"completion_tokens_details":{"reasoning_tokens":5}`)
	for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || !gjson.Get(payload, "response.usage").Exists() {
			continue
		}
		for _, detailName := range []string{"input_tokens_details", "prompt_tokens_details"} {
			assert.Equal(t, int64(25), gjson.Get(payload, "response.usage."+detailName+".cached_creation_tokens").Int())
			assert.Equal(t, int64(3), gjson.Get(payload, "response.usage."+detailName+".provider_tokens").Int())
		}
	}
}

func TestCacheCreationCompatibilityAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
		for _, tc := range []struct {
			name            string
			details         string
			forceFormat     bool
			wantCreation    int64
			creationPresent bool
		}{
			{name: "native cache write", details: `{"cache_write_tokens":5888}`, forceFormat: true, wantCreation: 5888, creationPresent: true},
			{name: "reported compatibility value", details: `{"cache_write_tokens":5888,"cached_creation_tokens":123}`, forceFormat: true, wantCreation: 123, creationPresent: true},
			{name: "compatibility value only", details: `{"cached_creation_tokens":123}`, forceFormat: true, wantCreation: 123, creationPresent: true},
			{name: "no cache write", details: `{}`, forceFormat: true},
			{name: "zero cache write", details: `{"cache_write_tokens":0}`, forceFormat: true},
			{name: "negative cache write", details: `{"cache_write_tokens":-1}`, forceFormat: true},
			{name: "force format disabled", details: `{"cache_write_tokens":5888}`},
		} {
			t.Run(string(format)+"/"+tc.name, func(t *testing.T) {
				// Both families are already reported, so each must gain its alias
				// without modifying the shared input-details used for settlement.
				usageJSON := `{"prompt_tokens":6011,"completion_tokens":5,"input_tokens":6011,"output_tokens":5,"total_tokens":6016,"prompt_tokens_details":` + tc.details + `,"input_tokens_details":` + tc.details + `}`
				body := `{"id":"chatcmpl-cache-write","object":"chat.completion","model":"kimi-k3","choices":[],"usage":` + usageJSON + `}`
				if format == types.RelayFormatOpenAIResponses {
					body = `{"id":"resp_cache_write","object":"response","status":"completed","model":"kimi-k3","output":[],"usage":` + usageJSON + `}`
				}
				c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, false)
				info.RelayFormat = format
				info.ChannelSetting.ForceFormat = tc.forceFormat

				handler := OpenaiHandler
				if format == types.RelayFormatOpenAIResponses {
					handler = OaiResponsesHandler
				}
				usage, apiErr := handler(c, info, resp)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				assert.Equal(t, int(gjson.Get(tc.details, "cached_creation_tokens").Int()), usage.PromptTokensDetails.CachedCreationTokens)
				wantCacheWrite := int(gjson.Get(tc.details, "cache_write_tokens").Int())
				if format == types.RelayFormatOpenAIResponses {
					wantCacheWrite = max(wantCacheWrite, 0)
				}
				assert.Equal(t, wantCacheWrite, usage.PromptTokensDetails.CacheWriteTokens)
				if usage.InputTokensDetails != nil {
					assert.Equal(t, int(gjson.Get(tc.details, "cached_creation_tokens").Int()), usage.InputTokensDetails.CachedCreationTokens)
					assert.Equal(t, int(gjson.Get(tc.details, "cache_write_tokens").Int()), usage.InputTokensDetails.CacheWriteTokens)
				}

				for _, detailName := range []string{"prompt_tokens_details", "input_tokens_details"} {
					creation := gjson.GetBytes(recorder.Body.Bytes(), "usage."+detailName+".cached_creation_tokens")
					assert.Equal(t, tc.creationPresent, creation.Exists())
					assert.Equal(t, tc.wantCreation, creation.Int())
					assert.Equal(t, gjson.Get(tc.details, "cache_write_tokens").Int(), gjson.GetBytes(recorder.Body.Bytes(), "usage."+detailName+".cache_write_tokens").Int())
				}
				assert.Equal(t, int64(6011), gjson.GetBytes(recorder.Body.Bytes(), "usage.prompt_tokens").Int())
				assert.Equal(t, int64(5), gjson.GetBytes(recorder.Body.Bytes(), "usage.completion_tokens").Int())
				assert.Equal(t, int64(6016), gjson.GetBytes(recorder.Body.Bytes(), "usage.total_tokens").Int())
				if tc.name == "native cache write" {
					t.Logf("client usage: %s", gjson.GetBytes(recorder.Body.Bytes(), "usage").Raw)
				}
			})
		}
	}
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
