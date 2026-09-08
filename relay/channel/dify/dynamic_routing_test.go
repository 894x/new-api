package dify

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDifyStreamVisibleTextUsesOnlyPublicMessageEvents(t *testing.T) {
	extractVisibleText := newDifyStreamVisibleTextExtractor()
	assert.Equal(t, "hello", extractVisibleText(`{"event":"message","answer":"hello"}`))
	assert.Equal(t, "agent", extractVisibleText(`{"event":"agent_message","answer":"agent"}`))
	assert.Empty(t, extractVisibleText(`{"event":"node_finished","answer":"hidden"}`))
	assert.Empty(t, extractVisibleText(`{"event":"message_end","metadata":{"usage":{"total_tokens":5}}}`))
}

func TestDifyStreamVisibleTextExcludesSplitThinkingBlock(t *testing.T) {
	extractVisibleText := newDifyStreamVisibleTextExtractor()
	event := func(answer string) string {
		return fmt.Sprintf(`{"event":"message","answer":%q}`, answer)
	}

	require.Empty(t, extractVisibleText(event(`<details style="color:gray;back`)))
	require.Empty(t, extractVisibleText(event(`ground-color: #f8f8f8;padding: 8px;border-radius: 4px;" open> <summary> Thinking... </summary>`+"\n")))
	require.Empty(t, extractVisibleText(event(`private reasoning`)))
	require.Empty(t, extractVisibleText(event(`</det`)))
	require.Empty(t, extractVisibleText(event(`ails>`)))
	require.Equal(t, "public answer", extractVisibleText(event(`public answer`)))
}

func TestDifyUpstreamErrorEventDynamicRoutingClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })

	tests := []struct {
		name       string
		statusJSON string
		hard       bool
	}{
		{name: "client bad request is neutral", statusJSON: `,"status":400`, hard: false},
		{name: "upstream authentication is hard", statusJSON: `,"status":401`, hard: true},
		{name: "rate limit is hard", statusJSON: `,"status":429`, hard: true},
		{name: "server failure is hard", statusJSON: `,"status":500`, hard: true},
		{name: "missing status is malformed and hard", hard: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			payload := fmt.Sprintf(`{"event":"error","code":"upstream_failed","message":"failed"%s}`, tt.statusJSON)
			resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\n"))}
			info := &relaycommon.RelayInfo{
				IsStream:        true,
				DisablePing:     true,
				OriginModelName: "dify-public-model",
				ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "dify-upstream-model"},
			}
			info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()

			_, handlerErr := difyStreamHandler(c, info, resp)
			require.Nil(t, handlerErr)
			sample, ok := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, ok)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}
