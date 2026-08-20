package palm

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPalmStreamAttemptObservesCandidateContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "palm2"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`{"candidates":[{"author":"assistant","content":"hello"}]}`))}

	apiErr, responseText := palmStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	assert.Equal(t, "hello", responseText)
	assert.Equal(t, "hello", info.DynamicRoutingAttemptVisibleText())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestPalmStreamErrorEnvelopeClassification(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		wantStatus int
		hard       bool
	}{
		{name: "client error is soft", code: http.StatusBadRequest, wantStatus: http.StatusBadRequest},
		{name: "cancelled upstream is hard", code: 1, wantStatus: http.StatusBadGateway, hard: true},
		{name: "unknown upstream failure is hard", code: 2, wantStatus: http.StatusBadGateway, hard: true},
		{name: "invalid argument is soft", code: 3, wantStatus: http.StatusBadRequest},
		{name: "deadline exceeded is hard", code: 4, wantStatus: http.StatusGatewayTimeout, hard: true},
		{name: "model endpoint not found is hard", code: 5, wantStatus: http.StatusBadGateway, hard: true},
		{name: "already exists is soft", code: 6, wantStatus: http.StatusConflict},
		{name: "permission denied is hard", code: 7, wantStatus: http.StatusForbidden, hard: true},
		{name: "resource exhausted is hard", code: 8, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "failed precondition is soft", code: 9, wantStatus: http.StatusPreconditionFailed},
		{name: "aborted upstream is hard", code: 10, wantStatus: http.StatusServiceUnavailable, hard: true},
		{name: "out of range is soft", code: 11, wantStatus: http.StatusBadRequest},
		{name: "unimplemented is hard", code: 12, wantStatus: http.StatusNotImplemented, hard: true},
		{name: "internal failure is hard", code: 13, wantStatus: http.StatusInternalServerError, hard: true},
		{name: "unavailable is hard", code: 14, wantStatus: http.StatusServiceUnavailable, hard: true},
		{name: "data loss is hard", code: 15, wantStatus: http.StatusInternalServerError, hard: true},
		{name: "unauthenticated is hard", code: 16, wantStatus: http.StatusUnauthorized, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "palm2"}}
			info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)
			body := fmt.Sprintf(`{"error":{"code":%d,"message":"failed","status":"ERROR"}}`, tt.code)

			handlerErr, _ := palmStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			if tt.code == 5 {
				assert.Equal(t, types.ErrorCodeModelNotFound, handlerErr.GetErrorCode())
			}
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}
