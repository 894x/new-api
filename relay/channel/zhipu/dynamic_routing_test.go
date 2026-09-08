package zhipu

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

type zhipuCloseNotifyRecorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

func (r *zhipuCloseNotifyRecorder) CloseNotify() <-chan bool { return r.closed }

func TestZhipuStreamAttemptObservesDataButNotMeta(t *testing.T) {
	body := strings.Join([]string{
		"data:hel",
		"data:lo",
		`meta:{"request_id":"req","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
	}, "\n")
	recorder := &zhipuCloseNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "chatglm"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()

	usage, apiErr := zhipuStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(body))})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, "hello", info.DynamicRoutingAttemptVisibleText())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestZhipuStreamFailureEventsAreClassified(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		hard       bool
	}{
		{name: "failed meta is hard", body: `meta:{"task_status":"FAIL"}`, wantStatus: http.StatusBadGateway, hard: true},
		{name: "event error client status is soft", body: "event:error\ndata:{\"code\":400,\"msg\":\"bad\"}", wantStatus: http.StatusBadRequest},
		{name: "event error without status is hard", body: "event:error\ndata:{\"code\":1001,\"msg\":\"failed\"}", wantStatus: http.StatusBadGateway, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &zhipuCloseNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "chatglm"}}
			info.BeginDynamicRoutingAttempt(5, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)

			_, handlerErr := zhipuStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.body + "\n"))})
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

func TestZhipuStructuredNullMetaIsHardButRawNullDataRemainsText(t *testing.T) {
	t.Run("structured meta null is hard", func(t *testing.T) {
		body := "data:visible\nmeta:null\n"
		recorder := &zhipuCloseNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "chatglm"}}
		info.BeginDynamicRoutingAttempt(5, info.GetChannelType(), info.OriginModelName, true)
		info.MarkAttemptUpstreamStarted()
		info.SetAttemptHTTPStatus(http.StatusOK)

		_, handlerErr := zhipuStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
		require.Nil(t, handlerErr)
		sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

		require.True(t, observed)
		assert.True(t, sample.HardFailure)
		assert.False(t, sample.Success)
	})

	t.Run("raw data null remains visible text", func(t *testing.T) {
		body := "data:null\n" + `meta:{"request_id":"req","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}` + "\n"
		recorder := &zhipuCloseNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "chatglm"}}
		info.BeginDynamicRoutingAttempt(5, info.GetChannelType(), info.OriginModelName, true)
		info.MarkAttemptUpstreamStarted()
		info.SetAttemptHTTPStatus(http.StatusOK)

		_, handlerErr := zhipuStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
		require.Nil(t, handlerErr)
		assert.Equal(t, "null", info.DynamicRoutingAttemptVisibleText())
		sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

		require.True(t, observed)
		assert.True(t, sample.Success)
		assert.False(t, sample.HardFailure)
	})
}

func TestZhipuTerminalOrEventOnlyStreamIsProtocolFailure(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "terminal meta only",
			body: `meta:{"request_id":"req","usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`,
		},
		{
			name: "event label only",
			body: "event:add",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &zhipuCloseNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "chatglm"}}
			info.BeginDynamicRoutingAttempt(5, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)

			_, handlerErr := zhipuStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.body + "\n"))})
			require.Nil(t, handlerErr)
			sample, observed := info.FinishDynamicRoutingAttempt(nil)

			require.True(t, observed)
			assert.True(t, sample.HardFailure)
			assert.False(t, sample.Success)
			assert.False(t, sample.HasTTFT)
			assert.False(t, sample.HasTPOT)
		})
	}
}
