package xai

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

func TestXAIStreamUpstreamErrorsAreClassified(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	tests := []struct {
		name       string
		payload    string
		wantStatus int
		hard       bool
		nilHandler bool
	}{
		{name: "malformed event", payload: `{not-json`, hard: true, nilHandler: true},
		{name: "null event", payload: `null`, hard: true, nilHandler: true},
		{name: "unknown error envelope", payload: `{"error":{"message":"failed","code":"provider"}}`, wantStatus: http.StatusBadGateway, hard: true},
		{name: "client error envelope", payload: `{"error":{"message":"bad request","status_code":400}}`, wantStatus: http.StatusBadRequest},
		{name: "rate limit envelope", payload: `{"error":{"message":"limited","code":"429"}}`, wantStatus: http.StatusTooManyRequests, hard: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(fmt.Sprintf("data: %s\n\n", tt.payload)))}
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok"}}
			info.BeginDynamicRoutingAttempt(20, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(resp.StatusCode)

			_, handlerErr := xAIStreamHandler(c, info, resp)
			if tt.nilHandler {
				require.Nil(t, handlerErr)
			} else {
				require.NotNil(t, handlerErr)
				assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			}
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}
