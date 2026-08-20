package channel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type attemptBoundaryAdaptor struct {
	Adaptor
	requestURL string
	urlErr     error
	headerErr  error
	headerHook func()
}

func (a *attemptBoundaryAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.requestURL, a.urlErr
}

func (a *attemptBoundaryAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	if a.headerHook != nil {
		a.headerHook()
	}
	return a.headerErr
}

func newAttemptBoundaryFixture(requestURL string) (*gin.Context, *relaycommon.RelayInfo) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "upstream-model",
		},
	}
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), "public-model", true)
	return c, info
}

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestDoApiRequestLocalConstructionFailuresStayPreUpstream(t *testing.T) {
	tests := []struct {
		name       string
		requestURL string
		urlErr     error
		headerErr  error
		proxy      string
	}{
		{name: "request URL", urlErr: errors.New("URL construction failed")},
		{name: "invalid URL", requestURL: "://invalid"},
		{name: "request headers", requestURL: "https://example.test/v1", headerErr: errors.New("header construction failed")},
		{name: "proxy client", requestURL: "https://example.test/v1", proxy: "http://["},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, info := newAttemptBoundaryFixture(tt.requestURL)
			info.ChannelSetting.Proxy = tt.proxy
			adaptor := &attemptBoundaryAdaptor{
				requestURL: tt.requestURL,
				urlErr:     tt.urlErr,
				headerErr:  tt.headerErr,
			}

			_, err := DoApiRequest(adaptor, c, info, nil)

			require.Error(t, err)
			apiErr := relaytypes.NewOpenAIError(err, relaytypes.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
			_, observed := info.FinishDynamicRoutingAttempt(apiErr)
			assert.False(t, observed)
		})
	}
}

func TestDoApiRequestMarksAttemptAtPhysicalDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	c, info := newAttemptBoundaryFixture(server.URL)

	resp, err := DoApiRequest(&attemptBoundaryAdaptor{requestURL: server.URL}, c, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Cleanup(func() { _ = resp.Body.Close() })
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)

	sample, observed := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, observed)
	assert.False(t, sample.UpstreamStartedAt.IsZero())
}

func TestDoApiRequestInvalidatesTimingWhenPreHeaderPingIsStillWriting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	c, info := newAttemptBoundaryFixture(server.URL)

	settings := operation_setting.GetGeneralSetting()
	oldEnabled := settings.PingIntervalEnabled
	settings.PingIntervalEnabled = true
	t.Cleanup(func() { settings.PingIntervalEnabled = oldEnabled })

	originalStarter := startPingKeepAliveForRequest
	interrupted := make(chan struct{})
	startPingKeepAliveForRequest = func(*gin.Context, time.Duration) *pingKeepAlive {
		done := make(chan struct{})
		var once sync.Once
		pinger := &pingKeepAlive{
			done: done,
			interruptWrite: func() {
				once.Do(func() {
					close(interrupted)
					close(done)
				})
			},
		}
		pinger.stop = func() {}
		pinger.writing = true
		return pinger
	}
	t.Cleanup(func() { startPingKeepAliveForRequest = originalStarter })

	resp, err := DoApiRequest(&attemptBoundaryAdaptor{requestURL: server.URL}, c, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.True(t, info.DynamicRoutingAttemptBackpressured())
	select {
	case <-interrupted:
	default:
		t.Fatal("blocked pre-header ping write was not actively interrupted")
	}
}

func TestDoApiRequestCancellationDuringHeaderConstructionStaysPreUpstream(t *testing.T) {
	c, info := newAttemptBoundaryFixture("https://example.test/v1")
	requestContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestContext)
	adaptor := &attemptBoundaryAdaptor{
		requestURL: "https://example.test/v1",
		headerHook: cancel,
		headerErr:  context.Canceled,
	}

	_, err := DoApiRequest(adaptor, c, info, nil)

	require.ErrorIs(t, err, context.Canceled)
	_, observed := info.FinishDynamicRoutingAttempt(
		relaytypes.NewOpenAIError(err, relaytypes.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
	)
	assert.False(t, observed)
}
