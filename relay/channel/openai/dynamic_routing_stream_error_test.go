package openai

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type panicFlushResponseWriter struct {
	gin.ResponseWriter
}

func (w *panicFlushResponseWriter) Flush() { panic("downstream flush failed") }

type failingWriteResponseWriter struct {
	gin.ResponseWriter
}

func (w *failingWriteResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("downstream write failed")
}

func prepareDynamicRoutingStreamTest(t *testing.T) {
	t.Helper()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
}

func beginDynamicRoutingStreamAttempt(info *relaycommon.RelayInfo, respStatus int) {
	info.IsStream = true
	info.OriginModelName = "public-model"
	info.BeginDynamicRoutingAttempt(12, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(respStatus)
}

func TestResponsesStreamErrorEnvelopeClassification(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	tests := []struct {
		name       string
		statusJSON string
		wantStatus int
		hard       bool
	}{
		{name: "unknown status is hard", wantStatus: http.StatusBadGateway, hard: true},
		{name: "explicit bad request is soft", statusJSON: `,"status_code":400`, wantStatus: http.StatusBadRequest},
		{name: "authentication is hard", statusJSON: `,"status":401`, wantStatus: http.StatusUnauthorized, hard: true},
		{name: "rate limit is hard", statusJSON: `,"status_code":429`, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "server failure is hard", statusJSON: `,"status":500`, wantStatus: http.StatusInternalServerError, hard: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := fmt.Sprintf(`{"type":"response.failed","response":{"error":{"message":"failed","type":"upstream_error","code":"failed"}}%s}`, tt.statusJSON)
			c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
			beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

			_, handlerErr := OaiResponsesStreamHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestResponsesFailedPreservesNestedModelNotFound(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	payload := `{"type":"response.failed","response":{"error":{"message":"model missing","type":"invalid_request_error","code":"model_not_found","status":404}}}`
	tests := []struct {
		name    string
		handler func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{name: "native responses", handler: OaiResponsesStreamHandler},
		{name: "responses converted to chat", handler: OaiResponsesToChatStreamHandler},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
			beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

			_, handlerErr := tt.handler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, http.StatusNotFound, handlerErr.StatusCode)
			assert.Equal(t, types.ErrorCodeModelNotFound, handlerErr.GetErrorCode())
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.True(t, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestResponsesToChatStreamTopLevelErrorEnvelopeClassification(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	tests := []struct {
		name       string
		statusJSON string
		wantStatus int
		hard       bool
	}{
		{name: "unknown status is hard", wantStatus: http.StatusBadGateway, hard: true},
		{name: "explicit bad request is soft", statusJSON: `,"status_code":400`, wantStatus: http.StatusBadRequest},
		{name: "authentication is hard", statusJSON: `,"status":401`, wantStatus: http.StatusUnauthorized, hard: true},
		{name: "timeout is hard", statusJSON: `,"status_code":408`, wantStatus: http.StatusRequestTimeout, hard: true},
		{name: "rate limit is hard", statusJSON: `,"status_code":429`, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "server failure is hard", statusJSON: `,"status":500`, wantStatus: http.StatusInternalServerError, hard: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := fmt.Sprintf(`{"error":{"message":"failed","type":"upstream_error","code":"failed"}%s}`, tt.statusJSON)
			c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
			beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

			_, handlerErr := OaiResponsesToChatStreamHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestResponsesStreamMalformedEventIsHardWhenHandlerReturnsNil(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	c, _, resp, info := newHTTP200ErrorTestContext(t, "data: {not-json}\n\n", "text/event-stream", true)
	beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

	_, handlerErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, handlerErr)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestChatToResponsesStreamErrorEnvelopeClassification(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	tests := []struct {
		name       string
		statusJSON string
		wantStatus int
		hard       bool
	}{
		{name: "unknown status is hard", wantStatus: http.StatusBadGateway, hard: true},
		{name: "explicit bad request is soft", statusJSON: `,"status_code":400`, wantStatus: http.StatusBadRequest},
		{name: "authentication is hard", statusJSON: `,"status":403`, wantStatus: http.StatusForbidden, hard: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := fmt.Sprintf(`{"error":{"message":"failed","type":"upstream_error","code":"failed"}%s}`, tt.statusJSON)
			c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
			info.RelayFormat = types.RelayFormatOpenAIResponses
			beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

			_, handlerErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestConvertedResponsesStreamsTreatMalformedUpstreamEventsAsHard(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	tests := []struct {
		name    string
		handler func(*relaycommon.RelayInfo) (*types.NewAPIError, relaycommon.DynamicRoutingAttemptSample, bool)
	}{
		{
			name: "chat to responses",
			handler: func(_ *relaycommon.RelayInfo) (*types.NewAPIError, relaycommon.DynamicRoutingAttemptSample, bool) {
				c, _, resp, info := newHTTP200ErrorTestContext(t, "data: {not-json}\n\n", "text/event-stream", true)
				info.RelayFormat = types.RelayFormatOpenAIResponses
				beginDynamicRoutingStreamAttempt(info, resp.StatusCode)
				_, handlerErr := OaiChatToResponsesStreamHandler(c, info, resp)
				sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)
				return handlerErr, sample, observed
			},
		},
		{
			name: "responses to chat",
			handler: func(_ *relaycommon.RelayInfo) (*types.NewAPIError, relaycommon.DynamicRoutingAttemptSample, bool) {
				c, _, resp, info := newHTTP200ErrorTestContext(t, "data: {not-json}\n\n", "text/event-stream", true)
				beginDynamicRoutingStreamAttempt(info, resp.StatusCode)
				_, handlerErr := OaiResponsesToChatStreamHandler(c, info, resp)
				sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)
				return handlerErr, sample, observed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerErr, sample, observed := tt.handler(nil)
			require.Nil(t, handlerErr)
			require.True(t, observed)
			assert.True(t, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestDirectOpenAIStreamMalformedEventIsHard(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	c, _, resp, info := newHTTP200ErrorTestContext(t, "data: {not-json}\n\n", "text/event-stream", true)
	beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

	_, handlerErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, handlerErr)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestDirectOpenAIStreamModelNotFoundAt404IsChannelModelHard(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	payload := `{"error":{"message":"model missing","type":"invalid_request_error","code":"model_not_found"},"status_code":404}`
	c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
	beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

	_, handlerErr := OaiStreamHandler(c, info, resp)
	require.NotNil(t, handlerErr)
	assert.Equal(t, http.StatusNotFound, handlerErr.StatusCode)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestDirectAzureStreamDeploymentNotFoundAt404IsChannelModelHard(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	tests := []string{"DeploymentNotFound", "ModelNotFound"}
	for _, code := range tests {
		t.Run(code, func(t *testing.T) {
			payload := fmt.Sprintf(`{"error":{"message":"deployment missing","type":"invalid_request_error","code":%q},"status_code":404}`, code)
			c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
			info.ChannelMeta.ChannelType = constant.ChannelTypeAzure
			beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

			_, handlerErr := OaiStreamHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, http.StatusNotFound, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.True(t, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestDirectAzureStreamOrdinaryResourceNotFoundAt404RemainsSoft(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	payload := `{"error":{"message":"resource missing","type":"invalid_request_error","code":"resource_not_found"},"status_code":404}`
	c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
	info.ChannelMeta.ChannelType = constant.ChannelTypeAzure
	beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

	_, handlerErr := OaiStreamHandler(c, info, resp)
	require.NotNil(t, handlerErr)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.False(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestDirectOpenAIStreamNullEventIsHardAfterVisibleContent(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		`data: null`,
	}, "\n\n")
	c, _, resp, info := newHTTP200ErrorTestContext(t, body, "text/event-stream", true)
	beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

	_, handlerErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, handlerErr)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestResponsesStreamDownstreamWriterFailureIsClientGone(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	payload := `{"type":"response.output_text.delta","delta":"hello"}`
	c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
	c.Writer = &panicFlushResponseWriter{ResponseWriter: c.Writer}
	beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

	_, handlerErr := OaiResponsesStreamHandler(c, info, resp)
	require.NotNil(t, handlerErr)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	_, observed := info.FinishDynamicRoutingAttempt(handlerErr)
	assert.False(t, observed)
}

func TestResponsesStreamDownstreamWriteErrorIsClientGone(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	payload := `{"type":"response.output_text.delta","delta":"hello"}`
	c, _, resp, info := newHTTP200ErrorTestContext(t, "data: "+payload+"\n\n", "text/event-stream", true)
	c.Writer = &failingWriteResponseWriter{ResponseWriter: c.Writer}
	beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

	_, handlerErr := OaiResponsesStreamHandler(c, info, resp)
	require.NotNil(t, handlerErr)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	_, observed := info.FinishDynamicRoutingAttempt(handlerErr)
	assert.False(t, observed)
}

func TestBufferedResponsesFallbackPublishesSyntheticStreamHealthAndVisibleTTFT(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	tests := []struct {
		name        string
		hasVisible  bool
		bodyContent string
	}{
		{
			name:        "visible output",
			hasVisible:  true,
			bodyContent: `"content":[{"type":"output_text","text":"hello"}]`,
		},
		{
			name:        "tool-only output is health-only",
			bodyContent: `"call_id":"call_1","name":"lookup","arguments":"{}"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputType := "message"
			if !tt.hasVisible {
				outputType = "function_call"
			}
			payload := fmt.Sprintf(`{"id":"resp_1","object":"response","status":"completed","model":"gpt-test","output":[{"type":%q,%s}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`, outputType, tt.bodyContent)
			c, _, resp, info := newHTTP200ErrorTestContext(t, payload, "application/json", true)
			beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

			_, handlerErr := OaiResponsesToChatHandler(c, info, resp)
			require.Nil(t, handlerErr)
			visibleText := info.DynamicRoutingAttemptVisibleText()
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.True(t, sample.Success)
			assert.Equal(t, tt.hasVisible, !sample.FirstContentAt.IsZero(), "visible=%q sample=%+v", visibleText, sample)
			if !tt.hasVisible {
				assert.False(t, sample.HasTTFT)
			}
			assert.False(t, sample.HasTPOT)
		})
	}
}

func TestBufferedResponsesFallbackErrorEnvelopeClassification(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	tests := []struct {
		name       string
		payload    string
		wantStatus int
		hard       bool
	}{
		{
			name:       "unknown upstream error is protocol hard",
			payload:    `{"error":{"message":"failed","type":"upstream_error","code":"failed"}}`,
			wantStatus: http.StatusBadGateway,
			hard:       true,
		},
		{
			name:       "status-bearing client error is soft",
			payload:    `{"error":{"message":"bad request","type":"invalid_request_error","code":400,"status":400}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "model not found is channel-model hard",
			payload:    `{"error":{"message":"model missing","type":"invalid_request_error","code":"model_not_found","status":404}}`,
			wantStatus: http.StatusNotFound,
			hard:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, resp, info := newHTTP200ErrorTestContext(t, tt.payload, "application/json", true)
			beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

			_, handlerErr := OaiResponsesToChatHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestBufferedResponsesFallbackDownstreamWriteErrorIsClientGone(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)
	payload := `{"id":"resp_1","object":"response","status":"completed","model":"gpt-test","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}`
	c, _, resp, info := newHTTP200ErrorTestContext(t, payload, "application/json", true)
	c.Writer = &failingWriteResponseWriter{ResponseWriter: c.Writer}
	beginDynamicRoutingStreamAttempt(info, resp.StatusCode)

	_, handlerErr := OaiResponsesToChatHandler(c, info, resp)
	require.NotNil(t, handlerErr)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	_, observed := info.FinishDynamicRoutingAttempt(handlerErr)
	assert.False(t, observed)
}
