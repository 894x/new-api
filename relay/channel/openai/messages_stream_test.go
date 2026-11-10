package openai

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messagesTerminalFailingWriter struct {
	gin.ResponseWriter
}

func (w *messagesTerminalFailingWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "event: message_delta") {
		return 0, errors.New("downstream terminal write failed")
	}
	return w.ResponseWriter.Write(data)
}

func TestOaiStreamHandlerMessagesLifecycle(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	role := `{"id":"chat_test","model":"kimi-k3.1-pro","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`
	text := `{"id":"chat_test","model":"kimi-k3.1-pro","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`
	finish := `{"id":"chat_test","model":"kimi-k3.1-pro","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
	usageOnly := `{"id":"chat_test","model":"kimi-k3.1-pro","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	usageChoice := `{"id":"chat_test","model":"kimi-k3.1-pro","choices":[{"index":0,"delta":{},"finish_reason":null}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	combined := `{"id":"chat_test","model":"kimi-k3.1-pro","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	tool := `{"id":"chat_test","model":"kimi-k3.1-pro","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":null}]}`
	tests := []struct {
		name       string
		frames     []string
		done       bool
		readError  bool
		writeError bool
		wantTool   bool
		wantStop   string
		wantEnd    relaycommon.StreamEndReason
	}{
		{name: "usage-only tail", frames: []string{role, text, finish, usageOnly}, done: true, wantStop: "end_turn", wantEnd: relaycommon.StreamEndReasonDone},
		{name: "usage tail with empty choice", frames: []string{role, text, finish, usageChoice}, done: true, wantStop: "end_turn", wantEnd: relaycommon.StreamEndReasonDone},
		{name: "single combined frame", frames: []string{combined}, done: true, wantStop: "end_turn", wantEnd: relaycommon.StreamEndReasonDone},
		{name: "role and combined final frame", frames: []string{role, combined}, done: true, wantStop: "end_turn", wantEnd: relaycommon.StreamEndReasonDone},
		{name: "finish without usage", frames: []string{role, text, finish}, done: true, wantStop: "end_turn", wantEnd: relaycommon.StreamEndReasonDone},
		{name: "done without finish reason", frames: []string{role, text}, done: true, wantStop: "end_turn", wantEnd: relaycommon.StreamEndReasonDone},
		{name: "finished stream at EOF", frames: []string{role, text, finish, usageChoice}, wantStop: "end_turn", wantEnd: relaycommon.StreamEndReasonEOF},
		{name: "token limit with usage tail", frames: []string{role, text, strings.ReplaceAll(finish, `"stop"`, `"length"`), usageChoice}, done: true, wantStop: "max_tokens", wantEnd: relaycommon.StreamEndReasonDone},
		{name: "pending tool with usage tail", frames: []string{role, tool, strings.ReplaceAll(finish, `"stop"`, `"tool_calls"`), usageChoice}, done: true, wantTool: true, wantStop: "tool_use", wantEnd: relaycommon.StreamEndReasonDone},
		{name: "unfinished EOF", frames: []string{role, text}, wantEnd: relaycommon.StreamEndReasonEOF},
		{name: "upstream read failure", frames: []string{role, text}, readError: true, wantEnd: relaycommon.StreamEndReasonScannerErr},
		{name: "downstream terminal write failure", frames: []string{role, text, finish, usageChoice}, done: true, writeError: true, wantEnd: relaycommon.StreamEndReasonClientGone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body strings.Builder
			for _, frame := range tt.frames {
				body.WriteString("data: " + frame + "\n\n")
			}
			if tt.done {
				body.WriteString("data: [DONE]\n\n")
			}
			var reader io.Reader = strings.NewReader(body.String())
			if tt.readError {
				reader = io.MultiReader(reader, iotest.ErrReader(io.ErrUnexpectedEOF))
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Set(common.RequestIdKey, common.NewRequestId())
			if tt.writeError {
				c.Writer = &messagesTerminalFailingWriter{ResponseWriter: c.Writer}
			}
			info, err := relaycommon.GenRelayInfo(c, types.RelayFormatClaude, nil, nil)
			require.NoError(t, err)
			info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "kimi-k3.1-pro"}
			info.IsStream = true
			info.DisablePing = true
			info.SetEstimatePromptTokens(2)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}

			usage, apiErr := OaiStreamHandler(c, info, resp)
			if tt.writeError {
				require.NotNil(t, apiErr)
				assert.Equal(t, types.ErrorCodeClientGone, apiErr.GetErrorCode())
			} else {
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
			}
			assert.Equal(t, tt.wantEnd, info.StreamStatus.EndReason)
			if tt.writeError {
				assert.Contains(t, recorder.Body.String(), "event: content_block_stop\n")
				assert.NotContains(t, recorder.Body.String(), "event: message_stop\n")
				return
			}
			var events []dto.ClaudeResponse
			for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
				if data, ok := strings.CutPrefix(line, "data: "); ok {
					var event dto.ClaudeResponse
					require.NoError(t, common.UnmarshalJsonStr(data, &event))
					events = append(events, event)
				}
			}
			require.NotEmpty(t, events)
			assert.Equal(t, "message_start", events[0].Type)
			counts := make(map[string]int)
			var textOutput strings.Builder
			var toolArguments strings.Builder
			var toolBlock *dto.ClaudeMediaMessage
			var terminal *dto.ClaudeResponse
			for i := range events {
				event := &events[i]
				counts[event.Type]++
				if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Text != nil {
					textOutput.WriteString(*event.Delta.Text)
				}
				if event.Type == "message_delta" {
					terminal = event
				}
				if event.Type == "content_block_start" && event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
					toolBlock = event.ContentBlock
				}
				if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.PartialJson != nil {
					toolArguments.WriteString(*event.Delta.PartialJson)
				}
			}
			assert.Equal(t, 1, counts["message_start"])
			if tt.wantTool {
				require.NotNil(t, toolBlock)
				assert.NotEmpty(t, toolBlock.Id)
				assert.Equal(t, "lookup", toolBlock.Name)
				assert.JSONEq(t, `{"q":"x"}`, toolArguments.String())
			} else {
				assert.Equal(t, "ok", textOutput.String())
			}
			if tt.wantStop == "" {
				assert.Zero(t, counts["message_stop"])
				assert.Nil(t, terminal)
				return
			}
			assert.Equal(t, 1, counts["content_block_start"])
			assert.Equal(t, 1, counts["content_block_stop"])
			assert.Equal(t, 1, counts["message_delta"])
			assert.Equal(t, 1, counts["message_stop"])
			assert.Equal(t, "message_stop", events[len(events)-1].Type)
			require.NotNil(t, terminal)
			require.NotNil(t, terminal.Delta)
			require.NotNil(t, terminal.Delta.StopReason)
			assert.Equal(t, tt.wantStop, *terminal.Delta.StopReason)
			require.NotNil(t, terminal.Usage)
			assert.Equal(t, 2, terminal.Usage.InputTokens)
			assert.Equal(t, usage.CompletionTokens, terminal.Usage.OutputTokens)
		})
	}
}
