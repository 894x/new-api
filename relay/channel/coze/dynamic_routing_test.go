package coze

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCozeStreamAttemptObservesMessageDeltaOnly(t *testing.T) {
	body := strings.Join([]string{
		"event: conversation.message.delta",
		`data: {"content":"hel"}`,
		"",
		"event: conversation.message.delta",
		`data: {"content":"lo"}`,
		"",
		"event: conversation.chat.completed",
		`data: {"usage":{"input_count":2,"output_count":3,"token_count":5}}`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "coze"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()

	usage, apiErr := cozeChatStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(body))})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, "hello", info.DynamicRoutingAttemptVisibleText())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestCozeStreamAttemptExcludesToolDeltaUntilVisibleAnswer(t *testing.T) {
	body := strings.Join([]string{
		"event: conversation.message.delta",
		`data: {"role":"assistant","type":"function_call","content_type":"text","content":"hidden tool call"}`,
		"",
		"event: conversation.message.delta",
		`data: {"role":"assistant","type":"answer","content_type":"text","content":"visible answer"}`,
		"",
		"event: conversation.chat.completed",
		`data: {"usage":{"input_count":2,"output_count":2,"token_count":4}}`,
		"",
	}, "\n")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "coze"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()

	_, apiErr := cozeChatStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(body))})

	require.Nil(t, apiErr)
	assert.Equal(t, "visible answer", info.DynamicRoutingAttemptVisibleText())
	assert.NotContains(t, recorder.Body.String(), "hidden tool call")
	assert.Contains(t, recorder.Body.String(), "visible answer")
}

func TestCozeToolOnlyStreamHasNoPerformanceTiming(t *testing.T) {
	body := strings.Join([]string{
		"event: conversation.message.delta",
		`data: {"role":"assistant","type":"tool_response","content_type":"text","content":"hidden tool result"}`,
		"",
		"event: conversation.chat.completed",
		`data: {"usage":{"input_count":2,"output_count":0,"token_count":2}}`,
		"",
	}, "\n")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "coze"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(http.StatusOK)

	_, apiErr := cozeChatStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, apiErr)
	sample, observed := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, observed)
	assert.True(t, sample.Success)
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
	assert.Empty(t, info.DynamicRoutingAttemptVisibleText())
}

func TestCozeCompletedOnlyStreamIsProtocolFailure(t *testing.T) {
	body := strings.Join([]string{
		"event: conversation.chat.completed",
		`data: {"usage":{"input_count":2,"output_count":0,"token_count":2}}`,
		"",
	}, "\n")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "coze"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(http.StatusOK)

	_, apiErr := cozeChatStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, apiErr)
	sample, observed := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}

func TestCozeStreamErrorEnvelopeClassification(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantStatus int
		hard       bool
	}{
		{name: "bad request is soft", payload: `{"code":400,"message":"bad"}`, wantStatus: http.StatusBadRequest},
		{name: "not found is soft", payload: `{"code":404,"message":"missing"}`, wantStatus: http.StatusNotFound},
		{name: "authentication is hard", payload: `{"code":401,"message":"auth"}`, wantStatus: http.StatusUnauthorized, hard: true},
		{name: "timeout is hard", payload: `{"code":408,"message":"timeout"}`, wantStatus: http.StatusRequestTimeout, hard: true},
		{name: "rate limit is hard", payload: `{"code":429,"message":"rate"}`, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "server failure is hard", payload: `{"code":500,"message":"failed"}`, wantStatus: http.StatusInternalServerError, hard: true},
		{name: "unknown application code is hard", payload: `{"code":7001,"message":"failed"}`, wantStatus: http.StatusBadGateway, hard: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "event: error\n" + "data: " + tt.payload + "\n\n"
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "coze"}}
			info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)

			_, handlerErr := cozeChatStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestCozeFailedChatEventClassification(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantStatus int
		hard       bool
	}{
		{name: "bad request is soft", payload: `{"last_error":{"code":400,"msg":"bad"}}`, wantStatus: http.StatusBadRequest},
		{name: "server failure is hard", payload: `{"last_error":{"code":500,"msg":"failed"}}`, wantStatus: http.StatusInternalServerError, hard: true},
		{name: "unknown application code is hard", payload: `{"last_error":{"code":7001,"msg":"failed"}}`, wantStatus: http.StatusBadGateway, hard: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "event: conversation.chat.failed\n" + "data: " + tt.payload + "\n\n"
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "coze"}}
			info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)

			_, handlerErr := cozeChatStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}
