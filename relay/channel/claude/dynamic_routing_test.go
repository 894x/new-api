package claude

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeHTTP200StreamErrorClassification(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	tests := []struct {
		name       string
		errorType  string
		wantStatus int
		hard       bool
	}{
		{name: "missing subtype", wantStatus: http.StatusBadGateway, hard: true},
		{name: "invalid request", errorType: "invalid_request_error", wantStatus: http.StatusBadRequest},
		{name: "authentication", errorType: "authentication_error", wantStatus: http.StatusUnauthorized, hard: true},
		{name: "permission", errorType: "permission_error", wantStatus: http.StatusForbidden, hard: true},
		{name: "model not found is channel model hard", errorType: "not_found_error", wantStatus: http.StatusNotFound, hard: true},
		{name: "request too large", errorType: "request_too_large", wantStatus: http.StatusRequestEntityTooLarge},
		{name: "rate limit", errorType: "rate_limit_error", wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "api error", errorType: "api_error", wantStatus: http.StatusInternalServerError, hard: true},
		{name: "overloaded", errorType: "overloaded_error", wantStatus: 529, hard: true},
		{name: "unknown subtype", errorType: "unknown_error", wantStatus: http.StatusBadGateway, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			payload := fmt.Sprintf(`{"type":"error","error":{"type":%q,"message":"failed"}}`, tt.errorType)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\n"))}
			info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, OriginModelName: "public-model", RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude"}}
			info.BeginDynamicRoutingAttempt(30, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(resp.StatusCode)

			_, handlerErr := ClaudeStreamHandler(c, resp, info)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestClaudeMalformedStreamEventIsHard(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: {not-json}\n\n"))}
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, OriginModelName: "public-model", RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude"}}
	info.BeginDynamicRoutingAttempt(30, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(resp.StatusCode)

	_, handlerErr := ClaudeStreamHandler(c, resp, info)
	require.NotNil(t, handlerErr)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}
