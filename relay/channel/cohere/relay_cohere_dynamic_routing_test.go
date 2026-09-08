package cohere

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	return r.closed
}

type cohereAttemptObserverSpy struct {
	firstResponseCalls int
	visibleCalls       int
}

type cohereBlockingFailWriter struct {
	*closeNotifyRecorder
	blocked chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *cohereBlockingFailWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("token_0")) {
		w.once.Do(func() { close(w.blocked) })
		<-w.release
		return 0, errors.New("downstream write failed")
	}
	return w.closeNotifyRecorder.Write(data)
}

func (s *cohereAttemptObserverSpy) SetFirstResponseTime() {
	s.firstResponseCalls++
}

func (s *cohereAttemptObserverSpy) RecordAttemptVisibleText(string) {
	s.visibleCalls++
}

func TestObserveCohereStreamAttemptRecordsEveryVisibleEvent(t *testing.T) {
	observer := &cohereAttemptObserverSpy{}
	firstResponseRecorded := false
	for _, data := range []string{
		`{"event_type":"stream-start","is_finished":false}`,
		`{"event_type":"text-generation","is_finished":false,"text":"hel"}`,
		`{"event_type":"text-generation","is_finished":false,"text":"lo"}`,
		`{"event_type":"stream-end","is_finished":true,"text":"ignored"}`,
	} {
		var err error
		firstResponseRecorded, err = observeCohereStreamAttempt(observer, data, firstResponseRecorded)
		require.NoError(t, err)
	}

	assert.True(t, firstResponseRecorded)
	assert.Equal(t, 1, observer.firstResponseCalls)
	assert.Equal(t, 2, observer.visibleCalls)
}

func TestCohereMalformedUpstreamEventIsHardFailureWhenHandlerReturnsNil(t *testing.T) {
	recorder := &closeNotifyRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closed:           make(chan bool),
	}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{not-json}\n"))}
	info := &relaycommon.RelayInfo{
		IsStream:        true,
		OriginModelName: "cohere-public-model",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "command-r"},
	}
	info.BeginDynamicRoutingAttempt(9, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()

	_, handlerErr := cohereStreamHandler(c, info, resp)
	require.Nil(t, handlerErr)
	sample, ok := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, ok)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestCohereEmptyOrTerminalOnlyStreamIsProtocolHard(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty body"},
		{name: "terminal only", body: `{"event_type":"stream-end","finish_reason":"COMPLETE"}` + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.body))}
			info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "command-r"}}
			info.BeginDynamicRoutingAttempt(9, info.GetChannelType(), "public-model", true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)

			_, handlerErr := cohereStreamHandler(c, info, resp)
			require.Nil(t, handlerErr)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.True(t, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestCohereStreamAttemptUsesVisibleContentNotMetadata(t *testing.T) {
	tests := []struct {
		name       string
		lines      []string
		hasContent bool
	}{
		{
			name: "metadata then multiple visible content chunks",
			lines: []string{
				`{"event_type":"stream-start","is_finished":false}`,
				`{"event_type":"text-generation","is_finished":false,"text":"hel"}`,
				`{"event_type":"text-generation","is_finished":false,"text":"lo"}`,
				`{"event_type":"stream-end","finish_reason":"COMPLETE","response":{"meta":{"billed_units":{"input_tokens":1,"output_tokens":3}}}}`,
			},
			hasContent: true,
		},
		{
			name: "metadata only",
			lines: []string{
				`{"event_type":"stream-start","is_finished":false}`,
				`{"event_type":"stream-end","finish_reason":"COMPLETE","response":{"meta":{"billed_units":{"input_tokens":1,"output_tokens":0}}}}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &closeNotifyRecorder{
				ResponseRecorder: httptest.NewRecorder(),
				closed:           make(chan bool),
			}
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			body := strings.Join(tt.lines, "\n") + "\n"
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
			info := &relaycommon.RelayInfo{
				IsStream: true,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "command-r",
				},
			}
			info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
			info.MarkAttemptUpstreamStarted()

			usage, apiErr := cohereStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			info.SetAttemptCompletionTokens(usage.CompletionTokens)
			sample, ok := info.FinishDynamicRoutingAttempt(nil)

			require.True(t, ok)
			assert.True(t, sample.Success)
			assert.Equal(t, tt.hasContent, !sample.FirstContentAt.IsZero())
			if tt.hasContent {
				assert.Equal(t, 3, usage.CompletionTokens)
			}
		})
	}
}

func TestCohereStreamErrorEnvelopeClassification(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantStatus int
		hard       bool
	}{
		{name: "client error is soft", payload: `{"event_type":"stream-error","status_code":400,"message":"bad"}`, wantStatus: http.StatusBadRequest},
		{name: "authentication is hard", payload: `{"event_type":"stream-error","status":401,"message":"auth"}`, wantStatus: http.StatusUnauthorized, hard: true},
		{name: "Cohere invalid token is hard", payload: `{"event_type":"stream-error","status":498,"message":"invalid token"}`, wantStatus: 498, hard: true},
		{name: "unknown error is hard", payload: `{"event_type":"stream-error","message":"failed"}`, wantStatus: http.StatusBadGateway, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "command-r"}}
			info.BeginDynamicRoutingAttempt(9, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.payload + "\n"))}

			_, handlerErr := cohereStreamHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestCohereFailedFinishReasonIsHardFailure(t *testing.T) {
	tests := []struct {
		finishReason string
		wantStatus   int
		hard         bool
	}{
		{finishReason: "ERROR", wantStatus: http.StatusBadGateway, hard: true},
		{finishReason: "ERROR_TOXIC", wantStatus: http.StatusUnprocessableEntity},
		{finishReason: "TIMEOUT", wantStatus: http.StatusRequestTimeout, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.finishReason, func(t *testing.T) {
			recorder := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "command-r"}}
			info.BeginDynamicRoutingAttempt(9, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)
			payload := `{"event_type":"stream-end","finish_reason":"` + tt.finishReason + `"}`
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload + "\n"))}

			_, handlerErr := cohereStreamHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestCohereDownstreamFailureCancelsProducerBlockedByFullIngress(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&body, "{\"event_type\":\"text-generation\",\"is_finished\":false,\"text\":\"token_%d\"}\n", i)
	}
	body.WriteString("{\"event_type\":\"stream-end\",\"is_finished\":true,\"finish_reason\":\"COMPLETE\"}\n")
	baseWriter := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
	writer := &cohereBlockingFailWriter{
		closeNotifyRecorder: baseWriter,
		blocked:             make(chan struct{}),
		release:             make(chan struct{}),
	}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "command-r"}}
	info.BeginDynamicRoutingAttempt(9, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	type result struct {
		err *types.NewAPIError
	}
	resultChan := make(chan result, 1)
	go func() {
		_, handlerErr := cohereStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body.String()))})
		resultChan <- result{err: handlerErr}
	}()

	select {
	case <-writer.blocked:
	case <-time.After(time.Second):
		t.Fatal("downstream writer did not block")
	}
	backpressured := make(chan struct{})
	go func() {
		for !info.DynamicRoutingAttemptBackpressured() {
			runtime.Gosched()
		}
		close(backpressured)
	}()
	select {
	case <-backpressured:
	case <-time.After(time.Second):
		close(writer.release)
		t.Fatal("producer did not block on the bounded ingress")
	}
	close(writer.release)
	var handlerResult result
	select {
	case handlerResult = <-resultChan:
	case <-time.After(time.Second):
		t.Fatal("handler did not cancel and join the blocked producer")
	}

	require.NotNil(t, handlerResult.err)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	_, observed := info.FinishDynamicRoutingAttempt(handlerResult.err)
	assert.False(t, observed)
}
