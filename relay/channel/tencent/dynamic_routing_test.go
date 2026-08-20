package tencent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTencentStreamAttemptObservesVisibleContentAndDone(t *testing.T) {
	body := strings.Join([]string{
		`data:{"Choices":[{"Delta":{"Content":"hel"}}]}`,
		`data:{"Choices":[{"Delta":{"Content":"lo"},"FinishReason":"stop"}]}`,
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "hunyuan"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()

	usage, apiErr := tencentStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(body))})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, "hello", info.DynamicRoutingAttemptVisibleText())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestTencentMalformedChannelCredentialsPublishPreDispatchHealthFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(string(constant.ContextKeyChannelKey), "malformed-tencent-key")
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "hunyuan-pro"}
	info.BeginDynamicRoutingAttempt(3, info.GetChannelType(), info.OriginModelName, true)

	_, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{Model: info.OriginModelName})
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

func TestTencentTerminalOnlyStreamIsProtocolFailure(t *testing.T) {
	body := `data:{"Choices":[{"Delta":{},"FinishReason":"stop"}]}` + "\n"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "hunyuan"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(http.StatusOK)

	_, apiErr := tencentStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, apiErr)
	sample, observed := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}

func TestTencentStreamErrorEnvelopeClassification(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		rawJSON    bool
		wantStatus int
		hard       bool
	}{
		{name: "invalid parameter is soft", payload: `{"ErrorMsg":{"Code":"InvalidParameter","Message":"bad temperature"}}`, wantStatus: http.StatusBadRequest},
		{name: "missing parameter is soft", payload: `{"ErrorMsg":{"Code":"MissingParameter.Messages","Message":"missing"}}`, wantStatus: http.StatusBadRequest},
		{name: "unknown parameter is soft", payload: `{"ErrorMsg":{"Code":"UnknownParameter.Legacy","Message":"unknown"}}`, wantStatus: http.StatusBadRequest},
		{name: "invalid action is soft", payload: `{"ErrorMsg":{"Code":"InvalidAction","Message":"bad action"}}`, wantStatus: http.StatusBadRequest},
		{name: "unsupported api version is soft", payload: `{"ErrorMsg":{"Code":"NoSuchVersion","Message":"bad version"}}`, wantStatus: http.StatusBadRequest},
		{name: "unsupported operation is soft", payload: `{"ErrorMsg":{"Code":"UnsupportedOperation.Model","Message":"unsupported"}}`, wantStatus: http.StatusBadRequest},
		{name: "failed prompt operation is soft", payload: `{"ErrorMsg":{"Code":"FailedOperation.SensitiveContent","Message":"blocked"}}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "text policy denial is soft", payload: `{"ErrorMsg":{"Code":"OperationDenied.TextIllegalDetected","Message":"blocked"}}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "engine request timeout is hard", payload: `{"ErrorMsg":{"Code":"FailedOperation.EngineRequestTimeout","Message":"timeout"}}`, wantStatus: http.StatusRequestTimeout, hard: true},
		{name: "engine server error is hard", payload: `{"ErrorMsg":{"Code":"FailedOperation.EngineServerError","Message":"failed"}}`, wantStatus: http.StatusInternalServerError, hard: true},
		{name: "engine rate limit is hard", payload: `{"ErrorMsg":{"Code":"FailedOperation.EngineServerLimitExceeded","Message":"limited"}}`, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "console server error is hard", payload: `{"ErrorMsg":{"Code":"FailedOperation.ConsoleServerError","Message":"failed"}}`, wantStatus: http.StatusInternalServerError, hard: true},
		{name: "resource package exhausted is hard", payload: `{"ErrorMsg":{"Code":"FailedOperation.ResourcePackExhausted","Message":"exhausted"}}`, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "service stopped is hard", payload: `{"ErrorMsg":{"Code":"FailedOperation.ServiceStop","Message":"stopped"}}`, wantStatus: http.StatusServiceUnavailable, hard: true},
		{name: "unknown failed operation is hard", payload: `{"ErrorMsg":{"Code":"FailedOperation.Unknown","Message":"failed"}}`, wantStatus: http.StatusBadGateway, hard: true},
		{name: "authentication is hard", payload: `{"ErrorMsg":{"Code":"AuthFailure.SignatureFailure","Message":"auth"}}`, wantStatus: http.StatusUnauthorized, hard: true},
		{name: "unauthorized operation is hard", payload: `{"ErrorMsg":{"Code":"UnauthorizedOperation","Message":"denied"}}`, wantStatus: http.StatusForbidden, hard: true},
		{name: "request limit is hard", payload: `{"ErrorMsg":{"Code":"RequestLimitExceeded","Message":"limited"}}`, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "internal error is hard", payload: `{"ErrorMsg":{"Code":"InternalError","Message":"failed"}}`, wantStatus: http.StatusInternalServerError, hard: true},
		{name: "resource unavailable is hard", payload: `{"ErrorMsg":{"Code":"ResourceUnavailable.Model","Message":"busy"}}`, wantStatus: http.StatusServiceUnavailable, hard: true},
		{name: "resource insufficient is hard", payload: `{"ErrorMsg":{"Code":"ResourceInsufficient.Quota","Message":"capacity"}}`, wantStatus: http.StatusServiceUnavailable, hard: true},
		{name: "model resource not found is hard", payload: `{"ErrorMsg":{"Code":"ResourceNotFound.Model","Message":"missing model"}}`, wantStatus: http.StatusServiceUnavailable, hard: true},
		{name: "legacy numeric error remains compatible", payload: `{"Error":{"Code":400,"Message":"bad"}}`, wantStatus: http.StatusBadRequest},
		{name: "unknown provider error is hard", payload: `{"ErrorMsg":{"Code":"Vendor.Unknown","Message":"failed"}}`, wantStatus: http.StatusBadGateway, hard: true},
		{name: "ordinary json response invalid parameter is soft", payload: `{"Response":{"Error":{"Code":"InvalidParameter","Message":"bad temperature"}}}`, rawJSON: true, wantStatus: http.StatusBadRequest},
		{name: "ordinary json response authentication is hard", payload: `{"Response":{"ErrorMsg":{"Code":"AuthFailure.SignatureFailure","Message":"auth"}}}`, rawJSON: true, wantStatus: http.StatusUnauthorized, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "hunyuan"}}
			info.BeginDynamicRoutingAttempt(3, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)
			body := "data:" + tt.payload + "\n"
			if tt.rawJSON {
				body = tt.payload + "\n"
			}
			_, handlerErr := tencentStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}
