package xunfei

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type xunfeiDialerFunc func(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error)

func (f xunfeiDialerFunc) DialContext(ctx context.Context, url string, header http.Header) (*websocket.Conn, *http.Response, error) {
	return f(ctx, url, header)
}

func TestXunfeiResponseVisibleTextUsesOnlyEmittedChoiceText(t *testing.T) {
	response := XunfeiChatResponse{}
	response.Payload.Choices.Text = []XunfeiChatResponseTextItem{
		{Content: "visible"},
		{Content: "not-emitted-by-current-adapter"},
	}
	assert.Equal(t, "visible", xunfeiResponseVisibleText(&response))

	response.Payload.Choices.Text = nil
	assert.Empty(t, xunfeiResponseVisibleText(&response))
}

func TestXunfeiStreamErrorClassification(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		wantStatus int
		hard       bool
	}{
		{name: "invalid parameter is soft", code: 10003, wantStatus: http.StatusBadRequest},
		{name: "misplaced system message is soft", code: 10049, wantStatus: http.StatusBadRequest},
		{name: "engine schema validation is soft", code: 10163, wantStatus: http.StatusUnprocessableEntity},
		{name: "token limit is soft", code: 10907, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "content policy is soft", code: 10013, wantStatus: http.StatusUnprocessableEntity},
		{name: "rate limit is hard", code: 10007, wantStatus: http.StatusTooManyRequests, hard: true},
		{name: "authentication is hard", code: 11200, wantStatus: http.StatusForbidden, hard: true},
		{name: "capacity is hard", code: 10008, wantStatus: http.StatusServiceUnavailable, hard: true},
		{name: "unknown upstream error is hard", code: 19999, wantStatus: http.StatusBadGateway, hard: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &XunfeiChatResponse{}
			response.Header.Code = tt.code
			response.Header.Message = "failed"
			handlerErr := newXunfeiStreamError(response)
			require.NotNil(t, handlerErr)
			assert.Equal(t, tt.wantStatus, handlerErr.StatusCode)

			info := &relaycommon.RelayInfo{IsStream: true}
			info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "public-model", true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)
			info.StreamStatus = relaycommon.NewStreamStatus()
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, handlerErr)
			sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

			require.True(t, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
		})
	}
}

func TestXunfeiInvalidChannelKeyPublishesPreDialHealthFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		IsStream:    true,
		ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "invalid-key"},
	}
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "public-model", true)
	adaptor := &Adaptor{request: &dto.GeneralOpenAIRequest{}}

	usage, handlerErr := adaptor.DoResponse(c, &http.Response{}, info)
	require.Nil(t, usage)
	require.NotNil(t, handlerErr)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.True(t, sample.UpstreamStartedAt.IsZero())
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}

func TestDialXunfeiUpstreamMarksPhysicalAttemptImmediatelyBeforeDial(t *testing.T) {
	dialErr := errors.New("dial failed")
	dialCalled := false
	info := &relaycommon.RelayInfo{}
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "public-model", true)

	conn, err := dialXunfeiUpstream(context.Background(), info, xunfeiDialerFunc(func(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error) {
		dialCalled = true
		return nil, nil, dialErr
	}), "wss://example.invalid/chat")

	assert.Nil(t, conn)
	require.ErrorIs(t, err, dialErr)
	assert.True(t, dialCalled)
	sample, observed := info.FinishDynamicRoutingAttempt(nil)
	require.True(t, observed)
	assert.False(t, sample.UpstreamStartedAt.IsZero())
}

func TestXunfeiSilentWebsocketCancellationClosesConnectionAndProducer(t *testing.T) {
	serverDone := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				close(serverDone)
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	info := &relaycommon.RelayInfo{}
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "public-model", true)
	info.StreamStatus = relaycommon.NewStreamStatus()
	stream, err := xunfeiMakeRequest(ctx, info, dto.GeneralOpenAIRequest{}, "general", strings.Replace(server.URL, "http://", "ws://", 1), "app")
	require.NoError(t, err)
	t.Cleanup(func() {
		stream.idleWatchdog.Stop()
		<-stream.idleWatcherDone
	})
	watcherDone := helper.CloseUpstreamOnContext(ctx, stream.conn, stream.done)

	cancel()
	select {
	case <-stream.done:
	case <-time.After(time.Second):
		t.Fatal("xunfei producer did not stop after client cancellation")
	}
	select {
	case <-watcherDone:
	case <-time.After(time.Second):
		t.Fatal("xunfei cancellation watcher did not exit")
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("xunfei websocket server did not observe client close")
	}

	reason, endErr := info.StreamStatus.End()
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, reason)
	require.ErrorIs(t, endErr, context.Canceled)
}

func TestXunfeiTerminalOnlyStreamIsProtocolFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
		terminal := XunfeiChatResponse{}
		terminal.Payload.Choices.Status = 2
		_ = conn.WriteJSON(terminal)
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	info := &relaycommon.RelayInfo{IsStream: true}
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "public-model", true)
	info.StreamStatus = relaycommon.NewStreamStatus()
	stream, err := xunfeiMakeRequest(ctx, info, dto.GeneralOpenAIRequest{}, "general", strings.Replace(server.URL, "http://", "ws://", 1), "app")
	require.NoError(t, err)
	for range stream.responses {
	}
	stream.idleWatchdog.Stop()
	<-stream.idleWatcherDone

	sample, observed := info.FinishDynamicRoutingAttempt(nil)
	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}
