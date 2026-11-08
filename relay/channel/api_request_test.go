package channel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type earlyChunkFlushRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
	once    sync.Once
}

func (recorder *earlyChunkFlushRecorder) Flush() {
	recorder.ResponseRecorder.Flush()
	recorder.once.Do(func() { close(recorder.flushed) })
}

func TestDoRequestPreEmitsBeforeUpstreamHeaders(t *testing.T) {
	for _, variant := range []string{"success", "http_error", "connection_closed", "client_cancel"} {
		t.Run(variant, func(t *testing.T) {
			service.InitHttpClient()
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			previousDB, previousRedis, previousCache := model.DB, rootcommon.RedisEnabled, rootcommon.MemoryCacheEnabled
			previousOptions := rootcommon.OptionMap
			model.DB, rootcommon.RedisEnabled, rootcommon.MemoryCacheEnabled = db, false, false
			rootcommon.OptionMap = make(map[string]string)
			t.Cleanup(func() {
				model.DB, rootcommon.RedisEnabled, rootcommon.MemoryCacheEnabled = previousDB, previousRedis, previousCache
				rootcommon.OptionMap = previousOptions
				sqlDB, _ := db.DB()
				_ = sqlDB.Close()
			})
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.ModelRoutingPool{}, &model.UserRoutingStrategy{}, &model.UserGroupChannelRoutingOverride{}, &model.Option{}))
			oldConfig := model.CurrentRequestPolicy().ChatEarlyChunk
			require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{"chat_early_chunk_setting.earliest_ms": "100"}))
			t.Cleanup(func() {
				require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{"chat_early_chunk_setting.earliest_ms": fmt.Sprint(oldConfig.EarliestMS)}))
			})
			name := "early-headers-" + variant
			channel := model.Channel{Id: 9001, Type: 1, Models: name, Group: "default"}
			require.NoError(t, db.Create(&channel).Error)
			require.NoError(t, db.Create(&model.User{Id: 42, Username: name, Role: 1, AffCode: name}).Error)
			require.NoError(t, db.Create(&model.ModelRoutingPool{RoutingGroup: "default", Model: name, Mode: "priority", ChannelsJSON: "[]", TuningJSON: `{"chat_early_chunk":{"mode":"enabled"}}`}).Error)
			body := `{"model":"` + name + `","stream":true,"messages":[{"role":"user","content":"` + strings.Repeat("stable prefix ", 500) + `"}]}`
			recorder := &earlyChunkFlushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{})}
			c, _ := gin.CreateTestContext(recorder)
			requestContext, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)).WithContext(requestContext)
			rootcommon.SetContextKey(c, constant.ContextKeyUserId, 42)
			rootcommon.SetContextKey(c, constant.ContextKeyDynamicRoutingEligible, true)
			rootcommon.SetContextKey(c, constant.ContextKeyResponseId, "public-id")
			p := &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: name, RequestBody: []byte(body), DynamicRoutingEligible: true}
			service.RequestPolicy(c).Successful = true
			service.BindChatPrefixChannel(c, &channel)
			service.RecordChatPrefixSuccess(p, channel.Id)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, UpstreamModelName: "upstream"}, IsStream: true, DisablePing: true, RelayFormat: relaytypes.RelayFormatOpenAI, UsingGroup: "default", UserId: 42, ResponseId: "public-id", StartTime: time.Now().Add(-600 * time.Millisecond)}
			info.StreamStatus = relaycommon.NewStreamStatus()
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			for range 100 {
				now := time.Now()
				info.StartTime = now.Add(-600 * time.Millisecond)
				service.BeginChatEarlyChunkAttempt(p, info, &channel)
				service.ChatEarlyChunk(c).Load(c, info.UpstreamModelName, now)
				service.ObserveChatEarlyChunkData(c, `{"choices":[{"delta":{"content":"history"}}]}`, now)
				service.FinishChatEarlyChunkAttempt(c, info, nil)
			}
			releaseHeaders := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(releaseHeaders) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				select {
				case <-releaseHeaders:
				case <-r.Context().Done():
					return
				}
				if variant == "connection_closed" {
					connection, _, hijackErr := w.(http.Hijacker).Hijack()
					if assert.NoError(t, hijackErr) {
						_ = connection.Close()
					}
					return
				}
				if variant == "http_error" {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = io.WriteString(w, `{"error":{"message":"unavailable","type":"server_error"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			t.Cleanup(server.Close)
			t.Cleanup(release)
			info.StartTime, info.StreamStatus = time.Now(), nil
			info.RequestTiming = rootcommon.NewRequestTiming(info.StartTime)
			service.BeginChatEarlyChunkAttempt(p, info, &channel)
			req, err := http.NewRequestWithContext(requestContext, http.MethodPost, server.URL, strings.NewReader(body))
			require.NoError(t, err)
			type result struct {
				response *http.Response
				err      error
			}
			returned := make(chan result, 1)
			go func() { response, requestErr := DoRequest(c, req, info); returned <- result{response, requestErr} }()
			select {
			case <-recorder.flushed:
			case <-time.After(2 * time.Second):
				t.Fatal("empty chunk was not sent while upstream headers were blocked")
			}
			writeMutex := helper.StreamWriteMutex(c)
			writeMutex.Lock()
			assert.Contains(t, recorder.Body.String(), `"content":""`)
			assert.NotContains(t, recorder.Body.String(), `"usage"`)
			assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			writeMutex.Unlock()
			assert.Zero(t, info.RequestTiming.Snapshot().UpstreamResponseHeadersAtMs)
			assert.Zero(t, info.ReceivedResponseCount)
			select {
			case <-returned:
				t.Fatal("upstream headers returned before they were released")
			default:
			}
			if variant == "client_cancel" {
				cancel()
			} else {
				release()
			}
			var got result
			select {
			case got = <-returned:
			case <-time.After(2 * time.Second):
				t.Fatal("pending upstream request did not terminate")
			}
			if variant == "client_cancel" {
				require.Error(t, got.err)
				assert.Nil(t, got.response)
				return
			}
			if variant == "success" {
				require.NoError(t, got.err)
				actual, readErr := io.ReadAll(got.response.Body)
				require.NoError(t, readErr, "stopping the pre-header sender must not cancel the response body")
				assert.Contains(t, string(actual), "[DONE]")
				require.NoError(t, got.response.Body.Close())
				return
			}
			var apiErr *relaytypes.NewAPIError
			if variant == "http_error" {
				require.NoError(t, got.err)
				apiErr = service.RelayErrorHandler(c.Request.Context(), got.response, false)
			} else {
				require.Error(t, got.err)
				apiErr = relaytypes.NewOpenAIError(got.err, relaytypes.ErrorCodeDoRequestFailed, http.StatusBadGateway)
			}
			assert.Equal(t, "response_committed", service.DecideRelayRetry(c, apiErr, 1).Reason)
			require.NoError(t, helper.WriteChatEarlyChunkError(c, apiErr))
			require.NoError(t, helper.WriteChatEarlyChunkError(c, apiErr))
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), `"error":`))
			assert.Equal(t, 1, strings.Count(recorder.Body.String(), "[DONE]"))
		})
	}
}

func TestDoRequestRecordsGenericUpstreamTransportMilestones(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		assert.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"ok":true}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`))
	timing := rootcommon.NewRequestTiming(time.Now())
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		RequestTiming: timing,
	}
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"model":"test"}`))
	require.NoError(t, err)

	resp, err := DoRequest(ctx, req, info)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	snapshot := timing.Snapshot()
	require.Positive(t, snapshot.UpstreamRequestStartedAtMs)
	require.Positive(t, snapshot.UpstreamRequestWrittenAtMs)
	require.Positive(t, snapshot.UpstreamResponseHeadersAtMs)
	assert.LessOrEqual(t, snapshot.UpstreamRequestStartedAtMs, snapshot.UpstreamRequestWrittenAtMs)
	assert.LessOrEqual(t, snapshot.UpstreamRequestWrittenAtMs, snapshot.UpstreamResponseHeadersAtMs)
}

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

func TestNewTaskAPIRequestInheritsClientCancellation(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)

	upstream, err := newTaskAPIRequest(c, "https://provider.example/tasks", nil)
	require.NoError(t, err)
	cancel()

	require.ErrorIs(t, upstream.Context().Err(), context.Canceled)
}

func TestRelayRequestClientCancellationClosesUpstream(t *testing.T) {
	service.InitHttpClient()
	for _, dispatch := range []struct {
		name string
		send func(Adaptor, *gin.Context, *relaycommon.RelayInfo, io.Reader) (*http.Response, error)
	}{
		{name: "JSON", send: DoApiRequest},
		{name: "form", send: DoFormRequest},
	} {
		for _, stage := range []string{"before headers", "after headers"} {
			t.Run(dispatch.name+"/"+stage, func(t *testing.T) {
				started := make(chan struct{})
				upstreamCanceled := make(chan struct{})
				release := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, err := io.Copy(io.Discard, r.Body)
					assert.NoError(t, err)
					if stage == "after headers" {
						w.Header().Set("Content-Type", "text/event-stream")
						w.WriteHeader(http.StatusOK)
						w.(http.Flusher).Flush()
					}
					close(started)
					select {
					case <-r.Context().Done():
						close(upstreamCanceled)
					case <-release:
					}
				}))
				t.Cleanup(server.Close)
				t.Cleanup(func() { close(release) })
				c, info := newAttemptBoundaryFixture(server.URL)
				requestContext, cancel := context.WithCancel(c.Request.Context())
				t.Cleanup(cancel)
				c.Request = c.Request.WithContext(requestContext)
				type result struct {
					response *http.Response
					err      error
				}
				completed := make(chan result, 1)
				go func() {
					resp, err := dispatch.send(&attemptBoundaryAdaptor{requestURL: server.URL}, c, info, strings.NewReader(`{}`))
					completed <- result{response: resp, err: err}
				}()
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("upstream request did not arrive")
				}
				var response *http.Response
				if stage == "after headers" {
					select {
					case outcome := <-completed:
						require.NoError(t, outcome.err)
						require.NotNil(t, outcome.response)
						response = outcome.response
						t.Cleanup(func() { _ = response.Body.Close() })
					case <-time.After(3 * time.Second):
						t.Fatal("relay did not receive upstream headers")
					}
				}
				cancel()
				select {
				case <-upstreamCanceled:
				case <-time.After(3 * time.Second):
					t.Fatal("client cancellation did not close the upstream request")
				}
				if response != nil {
					_, err := io.ReadAll(response.Body)
					assert.ErrorIs(t, err, context.Canceled)
					return
				}
				select {
				case outcome := <-completed:
					require.ErrorIs(t, outcome.err, context.Canceled)
					apiErr := relaytypes.NewOpenAIError(outcome.err, relaytypes.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
					assert.Equal(t, relaytypes.ErrorCodeClientGone, apiErr.GetErrorCode())
					assert.Equal(t, 499, apiErr.StatusCode)
					assert.True(t, relaytypes.IsSkipRetryError(apiErr))
				case <-time.After(3 * time.Second):
					t.Fatal("relay request did not return after client cancellation")
				}
			})
		}
	}
}

func TestDoRequestKeepsUpstreamCancellationSeparateFromClientGone(t *testing.T) {
	service.InitHttpClient()
	c, info := newAttemptBoundaryFixture("https://provider.invalid/v1/chat/completions")
	upstreamContext, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(upstreamContext, http.MethodPost, "https://provider.invalid/v1/chat/completions", strings.NewReader(`{}`))
	require.NoError(t, err)

	_, err = DoRequest(c, req, info)

	require.Error(t, err)
	require.NoError(t, c.Request.Context().Err())
	var apiErr *relaytypes.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, relaytypes.ErrorCodeDoRequestFailed, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
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

func TestProcessHeaderOverrideSessionIDClientPrecedence(t *testing.T) {
	tests := []struct {
		name            string
		clientSessionID string
		headerName      string
		headerOverrides map[string]any
		useRuntime      bool
		channelTest     bool
		wantSessionID   string
	}{
		{name: "client wins", clientSessionID: "client-session", wantSessionID: "client-session"},
		{name: "missing client uses channel", wantSessionID: "channel-session"},
		{name: "empty client uses channel", headerName: "X-Session-Id", wantSessionID: "channel-session"},
		{name: "blank client uses channel", clientSessionID: " \t ", wantSessionID: "channel-session"},
		{name: "case insensitive header", headerName: "x-SeSsIoN-iD", clientSessionID: "client-session", wantSessionID: "client-session"},
		{name: "client value is not interpolated", clientSessionID: "client-{api_key}", wantSessionID: "client-{api_key}"},
		{name: "unconfigured channel forwards client", clientSessionID: "client-session", headerOverrides: map[string]any{}, wantSessionID: "client-session"},
		{name: "wildcard keeps client priority", clientSessionID: "client-session", headerOverrides: map[string]any{"*": "", "X-Session-ID": "channel-session"}, wantSessionID: "client-session"},
		{name: "runtime override keeps client priority", clientSessionID: "client-session", useRuntime: true, wantSessionID: "client-session"},
		{name: "missing client uses runtime", useRuntime: true, wantSessionID: "runtime-session"},
		{name: "channel test uses channel", clientSessionID: "client-session", channelTest: true, wantSessionID: "channel-session"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			headerName := tt.headerName
			if headerName == "" {
				headerName = "X-Session-Id"
			}
			if tt.clientSessionID != "" || tt.headerName != "" {
				ctx.Request.Header.Set(headerName, tt.clientSessionID)
			}
			ctx.Request.Header.Set("X-Trace-Id", "client-trace")
			headerOverrides := tt.headerOverrides
			if headerOverrides == nil {
				headerOverrides = map[string]any{"X-Session-ID": "channel-session"}
			}
			headerOverrides["X-Trace-Id"] = "channel-trace"
			info := &relaycommon.RelayInfo{
				IsChannelTest:             tt.channelTest,
				UseRuntimeHeadersOverride: tt.useRuntime,
				RuntimeHeadersOverride: map[string]any{
					"x-session-id": "runtime-session",
					"x-trace-id":   "channel-trace",
				},
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiKey:          "channel-secret",
					HeadersOverride: headerOverrides,
				},
			}

			headers, err := processHeaderOverride(info, ctx)
			require.NoError(t, err)
			upstream := httptest.NewRequest(http.MethodPost, "https://example.test/v1/chat/completions", nil)
			applyHeaderOverrideToRequest(upstream, headers)

			assert.Equal(t, tt.wantSessionID, upstream.Header.Get("X-Session-Id"))
			assert.Equal(t, []string{tt.wantSessionID}, upstream.Header.Values("X-Session-Id"))
			assert.Equal(t, "channel-trace", upstream.Header.Get("X-Trace-Id"))
			assert.Equal(t, tt.clientSessionID, ctx.Request.Header.Get("X-Session-Id"))
		})
	}
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

func TestRoutingMonitorDotsRecordPhysicalDispatchOnceAndKeepRetryChannels(t *testing.T) {
	controller, err := dynamicrouting.NewController(dynamicrouting.Config{Enabled: true, MaxSamples: 6, MaxAge: time.Minute, MinSamples: 1, ProbeFraction: .015})
	require.NoError(t, err)
	key := dynamicrouting.RouteKey{Group: "default", Model: "public-model", Policy: "user:7"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(server.Close)
	c, info := newAttemptBoundaryFixture(server.URL)
	for index, channelID := range []int{17, 23} {
		decision := controller.SelectAvoiding(key, []dynamicrouting.Candidate{{ChannelID: channelID}}, nil, time.Now())
		require.True(t, decision.HasSelection)
		info.BeginDynamicRoutingAttempt(channelID, info.GetChannelType(), key.Model, true)
		info.OnDynamicRoutingUpstreamStarted(func(at time.Time) { controller.RecordDispatch(key, false, channelID, at) })
		_, err := DoApiRequest(&attemptBoundaryAdaptor{urlErr: errors.New("invalid configuration")}, c, info, nil)
		require.Error(t, err)
		before := controller.MonitorSnapshot(time.Now())
		require.Len(t, before, 1)
		assert.Len(t, before[0].Requests, index, "local construction failures must not produce a dot")
		resp, err := DoApiRequest(&attemptBoundaryAdaptor{requestURL: server.URL}, c, info, strings.NewReader(`{}`))
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		info.MarkAttemptUpstreamStarted()
	}
	snapshot := controller.MonitorSnapshot(time.Now())
	require.Len(t, snapshot, 1)
	require.Len(t, snapshot[0].Requests, 2)
	assert.Equal(t, 17, snapshot[0].Requests[0].ChannelID)
	assert.Equal(t, 23, snapshot[0].Requests[1].ChannelID)
	assert.NotEqual(t, snapshot[0].Requests[0].ID, snapshot[0].Requests[1].ID)
	assert.Equal(t, "user:7", snapshot[0].Policy)
	assert.Empty(t, controller.MonitorSnapshot(time.Now().Add(16 * time.Second))[0].Requests)
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

func TestToWebSocketURL(t *testing.T) {
	for input, want := range map[string]string{
		"https://api.openai.com/v1/responses":             "wss://api.openai.com/v1/responses",
		"http://127.0.0.1:3000/v1/responses":              "ws://127.0.0.1:3000/v1/responses",
		"wss://chatgpt.com/backend-api/codex/responses":   "wss://chatgpt.com/backend-api/codex/responses",
		"ws://127.0.0.1:3000/backend-api/codex/responses": "ws://127.0.0.1:3000/backend-api/codex/responses",
	} {
		assert.Equal(t, want, toWebSocketURL(input), input)
	}
}
