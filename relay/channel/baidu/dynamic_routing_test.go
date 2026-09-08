package baidu

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaiduStreamVisibleTextUsesResultOnly(t *testing.T) {
	assert.Equal(t, "hello", baiduStreamVisibleText(`{"result":"hello","usage":{"total_tokens":5}}`))
	assert.Empty(t, baiduStreamVisibleText(`{"usage":{"total_tokens":5}}`))
}

func TestBaiduAccessTokenFailurePublishesPreDispatchHealthFailure(t *testing.T) {
	info := &relaycommon.RelayInfo{
		IsStream:        true,
		OriginModelName: "ERNIE-4.0",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "invalid-key-without-secret",
			ChannelBaseUrl:    "https://example.invalid",
			UpstreamModelName: "ERNIE-4.0",
		},
	}
	info.BeginDynamicRoutingAttempt(2, info.GetChannelType(), info.OriginModelName, true)

	_, err := (&Adaptor{}).GetRequestURL(info)
	require.Error(t, err)
	apiErr, ok := err.(*relaytypes.NewAPIError)
	require.True(t, ok)
	sample, observed := info.FinishDynamicRoutingAttempt(apiErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.True(t, sample.UpstreamStartedAt.IsZero())
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}

func TestBaiduTransientTokenPreflightIsDynamicHardWithoutAutoDisable(t *testing.T) {
	originalProvider := baiduAccessTokenProvider
	baiduAccessTokenProvider = func(string) (string, error) {
		return "", errors.New("transient token endpoint failure")
	}
	t.Cleanup(func() { baiduAccessTokenProvider = originalProvider })
	originalAutoDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = originalAutoDisable })

	info := &relaycommon.RelayInfo{
		IsStream:        true,
		OriginModelName: "ERNIE-4.0",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "client-id|client-secret",
			ChannelBaseUrl:    "https://example.invalid",
			UpstreamModelName: "ERNIE-4.0",
		},
	}
	info.BeginDynamicRoutingAttempt(2, info.GetChannelType(), info.OriginModelName, true)

	_, err := (&Adaptor{}).GetRequestURL(info)
	require.Error(t, err)
	apiErr, ok := err.(*relaytypes.NewAPIError)
	require.True(t, ok)
	assert.False(t, relaytypes.IsChannelError(apiErr))
	assert.False(t, service.ShouldDisableChannel(apiErr))
	sample, observed := info.FinishDynamicRoutingAttempt(apiErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.True(t, sample.UpstreamStartedAt.IsZero())
}

func TestBaiduStreamErrorEnvelopeClassification(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	tests := []struct {
		name       string
		code       int
		wantStatus int
		hard       bool
	}{
		{name: "client error is soft", code: http.StatusBadRequest, wantStatus: http.StatusBadRequest},
		{name: "invalid argument is soft", code: 336001, wantStatus: http.StatusBadRequest},
		{name: "invalid json is soft", code: 336002, wantStatus: http.StatusBadRequest},
		{name: "base64 decode failure is soft", code: 336003, wantStatus: http.StatusBadRequest},
		{name: "invalid input size is soft", code: 336004, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "image decode failure is soft", code: 336005, wantStatus: http.StatusBadRequest},
		{name: "missing required parameter is soft", code: 336006, wantStatus: http.StatusBadRequest},
		{name: "question too long is soft", code: 336007, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "prompt too long is soft", code: 336103, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "daily request limit is hard", code: 17, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "qps limit is hard", code: 18, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "quota exhausted is hard", code: 19, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "invalid access token is hard", code: 110, wantStatus: http.StatusUnauthorized, hard: true},
		{name: "expired access token is hard", code: 111, wantStatus: http.StatusUnauthorized, hard: true},
		{name: "rate limit is hard", code: http.StatusTooManyRequests, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "provider internal error is hard", code: 336000, wantStatus: http.StatusInternalServerError, hard: true},
		{name: "model temporarily unavailable is hard", code: 336100, wantStatus: http.StatusServiceUnavailable, hard: true},
		{name: "unknown provider code is hard", code: 339999, wantStatus: http.StatusBadGateway, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "ernie"}}
			info.BeginDynamicRoutingAttempt(2, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)
			payload := fmt.Sprintf(`{"error_code":%d,"error_msg":"failed"}`, tt.code)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\n"))}

			handlerErr, _ := baiduStreamHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
			assert.Empty(t, info.DynamicRoutingAttemptVisibleText())
		})
	}
}
