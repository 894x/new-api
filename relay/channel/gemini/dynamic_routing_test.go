package gemini

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestGeminiMalformedUpstreamEventIsHardFailureWhenHandlerReturnsNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: {not-json}\n\n"))}
	info := &relaycommon.RelayInfo{
		IsStream:        true,
		DisablePing:     true,
		OriginModelName: "gemini-public-model",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gemini-upstream-model"},
	}
	info.BeginDynamicRoutingAttempt(8, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()

	_, handlerErr := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool { return true })
	require.Nil(t, handlerErr)
	sample, ok := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, ok)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestGeminiErrorEnvelopeClassification(t *testing.T) {
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })
	tests := []struct {
		name       string
		code       int
		wantStatus int
		hard       bool
	}{
		{name: "invalid argument is soft", code: http.StatusBadRequest, wantStatus: http.StatusBadRequest},
		{name: "authentication is hard", code: http.StatusUnauthorized, wantStatus: http.StatusUnauthorized, hard: true},
		{name: "rate limit is hard", code: http.StatusTooManyRequests, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "server error is hard", code: http.StatusInternalServerError, wantStatus: http.StatusInternalServerError, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			payload := fmt.Sprintf(`{"error":{"code":%d,"message":"failed","status":"ERROR"}}`, tt.code)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\n"))}
			info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini"}}
			info.BeginDynamicRoutingAttempt(8, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(resp.StatusCode)

			_, handlerErr := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool { return true })
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestGeminiModelResourceNotFoundIsChannelModelHard(t *testing.T) {
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	payload := `{"error":{"code":404,"message":"models/gemini-missing is not found","status":"NOT_FOUND"}}`
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\n"))}
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-missing"}}
	info.BeginDynamicRoutingAttempt(8, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(resp.StatusCode)

	_, handlerErr := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool { return true })
	require.NotNil(t, handlerErr)
	assert.Equal(t, http.StatusNotFound, handlerErr.StatusCode)
	assert.Equal(t, types.ErrorCodeModelNotFound, handlerErr.GetErrorCode())
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}
