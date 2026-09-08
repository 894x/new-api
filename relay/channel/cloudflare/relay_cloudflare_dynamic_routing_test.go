package cloudflare

import (
	"bytes"
	"context"
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

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cloudflareInspectingResponseWriter struct {
	*httptest.ResponseRecorder
	onVisibleWrite func()
	once           sync.Once
}

type cloudflareBlockingReadCloser struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

type cloudflareBlockingResponseWriter struct {
	*httptest.ResponseRecorder
	blocked chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *cloudflareBlockingResponseWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("token_0")) {
		w.once.Do(func() { close(w.blocked) })
		<-w.release
	}
	return w.ResponseRecorder.Write(data)
}

func (r *cloudflareBlockingReadCloser) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.closed
	return 0, errors.New("upstream body closed")
}

func (r *cloudflareBlockingReadCloser) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

func (w *cloudflareInspectingResponseWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("hello")) {
		w.once.Do(w.onVisibleWrite)
	}
	return w.ResponseRecorder.Write(data)
}

func TestCloudflareStreamAttemptUsesVisibleContentNotMetadata(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		hasContent bool
	}{
		{
			name: "metadata then visible content",
			body: strings.Join([]string{
				`data: {"id":"chunk-1","choices":[{"delta":{"role":"assistant"}}]}`,
				`data: {"id":"chunk-2","choices":[{"delta":{"content":"hello"}}]}`,
				`data: [DONE]`,
			}, "\n"),
			hasContent: true,
		},
		{
			name: "metadata only",
			body: strings.Join([]string{
				`data: {"id":"chunk-1","choices":[{"delta":{"role":"assistant"}}]}`,
				`data: [DONE]`,
			}, "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.body))}
			info := &relaycommon.RelayInfo{
				IsStream: true,
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "gpt-3.5-turbo",
				},
			}
			info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
			info.MarkAttemptUpstreamStarted()

			apiErr, usage := cfStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			sample, ok := info.FinishDynamicRoutingAttempt(nil)

			require.True(t, ok)
			assert.True(t, sample.Success)
			assert.Equal(t, tt.hasContent, !sample.FirstContentAt.IsZero())
		})
	}
}

func TestCloudflareStreamRecordsVisibleContentBeforeDownstreamWrite(t *testing.T) {
	var sample relaycommon.DynamicRoutingAttemptSample
	var observed bool
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-3.5-turbo",
		},
	}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	writer := &cloudflareInspectingResponseWriter{ResponseRecorder: httptest.NewRecorder()}
	writer.onVisibleWrite = func() {
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
		sample, observed = info.FinishDynamicRoutingAttempt(nil)
	}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"id":"chunk-1","choices":[{"delta":{"content":"hello"}}]}`,
			`data: [DONE]`,
		}, "\n"))),
	}

	apiErr, usage := cfStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.True(t, observed)
	assert.False(t, sample.FirstContentAt.IsZero())
	assert.False(t, sample.TTFTInvalidated)
	assert.False(t, sample.TPOTInvalidated)
}

func TestCloudflareStreamCancellationClosesSilentUpstreamAndReturns(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	body := &cloudflareBlockingReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestContext)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "model"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	handlerDone := make(chan struct{})
	go func() {
		_, _ = cfStreamHandler(c, info, &http.Response{Body: body})
		close(handlerDone)
	}()

	select {
	case <-body.started:
	case <-time.After(time.Second):
		t.Fatal("stream reader did not start")
	}
	cancel()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("stream handler did not return after client cancellation")
	}

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
}

func TestCloudflareSilentUpstreamTriggersDynamicRoutingTimeout(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	body := &cloudflareBlockingReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "model"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	type result struct {
		err *types.NewAPIError
	}
	resultChan := make(chan result, 1)
	go func() {
		handlerErr, _ := cfStreamHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: body})
		resultChan <- result{err: handlerErr}
	}()

	select {
	case <-body.started:
	case <-time.After(time.Second):
		t.Fatal("stream reader did not start")
	}
	var handlerResult result
	select {
	case handlerResult = <-resultChan:
	case <-time.After(2 * time.Second):
		_ = body.Close()
		t.Fatal("silent upstream did not trigger streaming timeout")
	}
	sample, observed := info.FinishDynamicRoutingAttempt(handlerResult.err)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason)
}

func TestCloudflareSlowDownstreamInvalidatesTPOTAfterBoundedIngressFills(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&body, "data: {\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}\n", i)
	}
	body.WriteString("data: [DONE]\n")
	writer := &cloudflareBlockingResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		blocked:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "model"}}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	handlerDone := make(chan struct{})
	go func() {
		_, _ = cfStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(body.String()))})
		close(handlerDone)
	}()

	select {
	case <-writer.blocked:
	case <-time.After(time.Second):
		t.Fatal("downstream writer did not block on first visible chunk")
	}
	backpressured := make(chan struct{})
	stopPolling := make(chan struct{})
	defer close(stopPolling)
	go func() {
		for {
			if info.DynamicRoutingAttemptBackpressured() {
				close(backpressured)
				return
			}
			select {
			case <-stopPolling:
				return
			default:
				runtime.Gosched()
			}
		}
	}()
	select {
	case <-backpressured:
	case <-time.After(time.Second):
		t.Fatal("bounded ingress did not mark downstream backpressure")
	}
	close(writer.release)
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after downstream resumed")
	}

	sample, observed := info.FinishDynamicRoutingAttempt(nil)
	require.True(t, observed)
	assert.False(t, sample.TTFTInvalidated)
	assert.True(t, sample.TPOTInvalidated)
}

func TestCloudflareStreamErrorEnvelopeClassification(t *testing.T) {
	tests := []struct {
		name       string
		payload    string
		wantStatus int
		hard       bool
	}{
		{name: "client error is soft", payload: `{"success":false,"errors":[{"code":400,"message":"bad"}]}`, wantStatus: http.StatusBadRequest},
		{name: "rate limit is hard", payload: `{"success":false,"errors":[{"code":429,"message":"limited"}]}`, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "unknown error is hard", payload: `{"success":false,"errors":[{"code":1001,"message":"failed"}]}`, wantStatus: http.StatusBadGateway, hard: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "workers-ai"}}
			info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), info.OriginModelName, true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: " + tt.payload + "\n"))}

			handlerErr, _ := cfStreamHandler(c, info, resp)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestCloudflareStreamNullEventIsHardAfterVisibleContent(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "public-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "workers-ai"}}
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), info.OriginModelName, true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(http.StatusOK)
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		`data: null`,
	}, "\n")
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}

	handlerErr, _ := cfStreamHandler(c, info, resp)
	require.Nil(t, handlerErr)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}
