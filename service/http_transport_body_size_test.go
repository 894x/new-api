package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyRoutingThroughConnectProxy(t *testing.T) {
	withRelayHTTPTransportSettings(t)
	upstream := startHTTP2TLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(w, r.Body) }))
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", 400)
			return
		}
		// Test proxy is deliberately restricted to this local test upstream.
		if r.Host != strings.TrimPrefix(upstream.URL, "https://") {
			http.Error(w, "unexpected target", 400)
			return
		}
		target, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, "dial failed", 502)
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = target.Close()
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		go func() { _, _ = io.Copy(target, conn); _ = target.Close() }()
		_, _ = io.Copy(conn, target)
		_ = conn.Close()
	}))
	t.Cleanup(proxyServer.Close)
	proxyURL, err := url.Parse(proxyServer.URL)
	require.NoError(t, err)
	normal, err := newHTTPClientFromPolicy(HTTPTransportPolicy{Protocol: "auto", Shards: 1}, proxyURL, testTLSClientConfig(t, upstream))
	require.NoError(t, err)
	large, err := newHTTPClientFromPolicy(HTTPTransportPolicy{Protocol: "http1", Shards: 1}, proxyURL, testTLSClientConfig(t, upstream))
	require.NoError(t, err)
	client := &http.Client{Transport: &bodySizeRoundTripper{normal: normal.Transport, large: large.Transport, threshold: 8}}
	t.Cleanup(client.CloseIdleConnections)
	for _, tc := range []struct {
		body     string
		protocol int
	}{{"small", 2}, {"large body", 1}} {
		resp, err := client.Post(upstream.URL, "text/plain", strings.NewReader(tc.body))
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		assert.Equal(t, tc.protocol, resp.ProtoMajor)
		assert.Equal(t, tc.body, string(body))
	}
}

func TestBodySizeRoutingTLSBoundariesAndReuse(t *testing.T) {
	withRelayHTTPTransportSettings(t)
	server := startHTTP2TLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", 400)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	normal := newHTTPClientWithPolicyAndTLS(HTTPTransportPolicy{Protocol: "auto", Shards: 2}, testTLSClientConfig(t, server))
	large := newHTTPClientWithPolicyAndTLS(HTTPTransportPolicy{Protocol: "http1", Shards: 1}, testTLSClientConfig(t, server))
	client := &http.Client{Transport: &bodySizeRoundTripper{normal: normal.Transport, large: large.Transport, threshold: 16}}
	t.Cleanup(client.CloseIdleConnections)
	for _, tc := range []struct {
		name, body, reason string
		unknown            bool
		proto              int
		reused             bool
	}{
		{"small", strings.Repeat("a", 15), "small_body", false, 2, false},
		{"boundary", strings.Repeat("b", 16), "large_body", false, 1, false},
		{"large reused", strings.Repeat("c", 17), "large_body", false, 1, true},
		{"unknown", strings.Repeat("d", 32), "unknown_length", true, 2, false},
		{"empty", "", "small_body", false, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(tc.body))
			require.NoError(t, err)
			if tc.unknown {
				req.Body = io.NopCloser(strings.NewReader(tc.body))
				req.ContentLength = -1
				req.GetBody = nil
			}
			timing := common.NewRequestTiming(time.Now())
			req, finish := TraceUpstreamTransport(req, timing, 15, dto.ChannelSettings{})
			resp, err := client.Do(req)
			finish(resp, err)
			require.NoError(t, err)
			assert.Equal(t, tc.proto, resp.ProtoMajor)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, tc.body, string(body), "routing must not change the payload")
			snapshots := timing.UpstreamTransports()
			require.Len(t, snapshots, 1)
			s := snapshots[0]
			assert.Equal(t, tc.reason, s.Reason)
			assert.Equal(t, resp.Proto, s.Protocol)
			assert.Equal(t, tc.reused, s.Reused)
			assert.Equal(t, "complete", s.Outcome)
			assert.NotNil(t, s.AcquireMs)
			assert.NotNil(t, s.WriteMs)
			if tc.unknown {
				assert.Equal(t, int64(-1), s.BodyBytes)
			} else {
				assert.Equal(t, int64(len(tc.body)), s.BodyBytes)
			}
		})
	}
}

type bodyRoutingTestTransport func(*http.Request) (*http.Response, error)

func (f bodyRoutingTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBodyRoutingNeverRetriesAfterWriteFailure(t *testing.T) {
	normalCalls, largeCalls := 0, 0
	failure := errors.New("write failed")
	router := &bodySizeRoundTripper{threshold: 1,
		normal: bodyRoutingTestTransport(func(*http.Request) (*http.Response, error) { normalCalls++; return nil, failure }),
		large: bodyRoutingTestTransport(func(req *http.Request) (*http.Response, error) {
			largeCalls++
			_, _ = io.Copy(io.Discard, req.Body)
			_ = req.Body.Close()
			return nil, failure
		}),
	}
	req := httptest.NewRequest(http.MethodPost, "https://example.test", strings.NewReader("payload"))
	_, err := router.RoundTrip(req)
	assert.ErrorIs(t, err, failure)
	assert.Equal(t, 1, largeCalls)
	assert.Zero(t, normalCalls)
}

func TestBodyRoutingUsesCachedPoolsAndDisabledOriginalClient(t *testing.T) {
	initDefaultHTTPClientFixture(t)
	base, err := GetHttpClientWithProxySettings("", dto.ChannelSettings{})
	require.NoError(t, err)
	disabled, err := GetHttpClientWithProxySettings("", dto.ChannelSettings{HTTP1LargeBodyThresholdBytes: 4096})
	require.NoError(t, err)
	assert.Same(t, base, disabled)
	for _, proxyURL := range []string{"", "http://127.0.0.1:9998", "socks5://127.0.0.1:9999"} {
		settings := dto.ChannelSettings{HTTP1LargeBodyEnabled: true, HTTP1LargeBodyThresholdBytes: 1024}
		a, err := GetHttpClientWithProxySettings(proxyURL, settings)
		require.NoError(t, err)
		settings.HTTP1LargeBodyThresholdBytes = 8192
		b, err := GetHttpClientWithProxySettings(proxyURL, settings)
		require.NoError(t, err)
		x, y := a.Transport.(*bodySizeRoundTripper), b.Transport.(*bodySizeRoundTripper)
		assert.Same(t, x.normal, y.normal)
		assert.Same(t, x.large, y.large)
		assert.NotSame(t, x.normal, x.large)
	}
}

func TestTrackedTransportKeepsSSEActiveUntilCloseOrCancel(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "close", true: "cancel"}[cancelRequest], func(t *testing.T) {
			reader, writer := io.Pipe()
			defer writer.Close()
			tracker := configureHTTPTransportTracker(&http.Transport{}, "auto", 0, 1)
			transport := &trackedRoundTripper{tracker: tracker, inner: bodyRoutingTestTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{Proto: "HTTP/2.0", Body: reader}, nil
			})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := WithHTTPTransportChannelID(httptest.NewRequest(http.MethodGet, "https://example.test", nil).WithContext(ctx), 15)
			resp, err := transport.RoundTrip(req)
			require.NoError(t, err)
			tracker.mu.Lock()
			active := tracker.activeRequests
			byChannel := tracker.activeByChannel[15]
			tracker.mu.Unlock()
			assert.Equal(t, int64(1), active)
			assert.Equal(t, int64(1), byChannel)
			if cancelRequest {
				cancel()
				require.Eventually(t, func() bool { tracker.mu.Lock(); defer tracker.mu.Unlock(); return tracker.activeRequests == 0 }, time.Second, time.Millisecond)
			}
			require.NoError(t, resp.Body.Close())
			require.NoError(t, resp.Body.Close())
			tracker.mu.Lock()
			defer tracker.mu.Unlock()
			assert.Zero(t, tracker.activeRequests)
			assert.Empty(t, tracker.activeByChannel)
		})
	}
}
