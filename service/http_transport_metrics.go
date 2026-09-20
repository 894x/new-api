package service

import (
	"context"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type httpTransportChannelKey struct{}

func WithHTTPTransportChannelID(req *http.Request, channelID int) *http.Request {
	if req == nil || channelID <= 0 {
		return req
	}
	return req.WithContext(context.WithValue(req.Context(), httpTransportChannelKey{}, channelID))
}

type relayHTTPTransportTracker struct {
	id              uint64
	mu              sync.Mutex
	activeRequests  int64
	connections     int64
	lastUsed        time.Time
	lastProtocol    string
	activeByChannel map[int]int64
	protocol        string
	shard           int
	shards          int
}

type HTTPTransportMetric struct {
	PoolID          uint64        `json:"pool_id"`
	Protocol        string        `json:"protocol"`
	Shard           int           `json:"shard"`
	Shards          int           `json:"shards"`
	ActiveRequests  int64         `json:"active_requests"`
	Connections     int64         `json:"connections"`
	LastProtocol    string        `json:"last_protocol,omitempty"`
	ActiveByChannel map[int]int64 `json:"active_by_channel,omitempty"`
}

type trackedConn struct {
	net.Conn
	tracker *relayHTTPTransportTracker
	once    sync.Once
}

func (conn *trackedConn) Close() error {
	conn.once.Do(func() { conn.tracker.mu.Lock(); conn.tracker.connections--; conn.tracker.mu.Unlock() })
	return conn.Conn.Close()
}

// responseLifetimeBody preserves streaming: neither Close nor Read drains it.
type responseLifetimeBody struct {
	io.ReadCloser
	finish func(error)
	stop   func() bool
}

func (b *responseLifetimeBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.stop()
		b.finish(err)
	}
	return n, err
}

func (b *responseLifetimeBody) Close() error {
	err := b.ReadCloser.Close()
	b.stop()
	b.finish(err)
	return err
}

type trackedRoundTripper struct {
	inner   http.RoundTripper
	tracker *relayHTTPTransportTracker
}

func (rt *trackedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	t := rt.tracker
	channelID, _ := req.Context().Value(httpTransportChannelKey{}).(int)
	t.mu.Lock()
	t.activeRequests++
	t.lastUsed = time.Now()
	if channelID > 0 {
		t.activeByChannel[channelID]++
	}
	httpTransportTrackers.Store(t.id, t)
	t.mu.Unlock()
	var once sync.Once
	finish := func(err error) {
		once.Do(func() {
			t.mu.Lock()
			t.activeRequests--
			t.lastUsed = time.Now()
			if channelID > 0 {
				t.activeByChannel[channelID]--
				if t.activeByChannel[channelID] == 0 {
					delete(t.activeByChannel, channelID)
				}
			}
			t.mu.Unlock()
			if a, ok := req.Context().Value(transportAttemptKey{}).(*common.UpstreamTransportAttempt); ok {
				a.Update(func(s *common.UpstreamTransportSnapshot) {
					s.Outcome = "closed"
					if err == io.EOF {
						s.Outcome = "complete"
					} else if err != nil {
						s.Outcome = transportErrorClass(err)
					}
				})
			}
		})
	}
	stop := context.AfterFunc(req.Context(), func() { finish(req.Context().Err()) })
	resp, err := rt.inner.RoundTrip(req)
	if resp != nil {
		t.mu.Lock()
		t.lastProtocol = resp.Proto
		t.mu.Unlock()
	}
	if err != nil || resp == nil || resp.Body == nil {
		stop()
		finish(err)
	} else {
		resp.Body = &responseLifetimeBody{ReadCloser: resp.Body, finish: finish, stop: stop}
	}
	return resp, err
}

func (rt *trackedRoundTripper) CloseIdleConnections() { closeIdleConnections(rt.inner) }

var httpTransportTrackerID atomic.Uint64
var httpTransportTrackers sync.Map // ID -> counters only; never retain transports/clients.

// Called exactly once per never-used transport, after its policy/proxy is set.
func configureHTTPTransportTracker(transport *http.Transport, protocol string, shard, shards int) *relayHTTPTransportTracker {
	t := &relayHTTPTransportTracker{id: httpTransportTrackerID.Add(1), protocol: protocol, shard: shard, shards: shards, lastUsed: time.Now(), activeByChannel: make(map[int]int64)}
	dial := transport.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		t.mu.Lock()
		t.connections++
		t.lastUsed = time.Now()
		httpTransportTrackers.Store(t.id, t)
		t.mu.Unlock()
		return &trackedConn{Conn: conn, tracker: t}, nil
	}
	httpTransportTrackers.Store(t.id, t)
	return t
}

// Connections count pool-wide sockets, NOT channel-owned HTTP/2 streams.
// The registry is swept on reads; idle entries can register again on reuse.
func GetHTTPTransportMetrics() []HTTPTransportMetric {
	metrics := make([]HTTPTransportMetric, 0)
	httpTransportTrackers.Range(func(key, value any) bool {
		t := value.(*relayHTTPTransportTracker)
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.connections == 0 && t.activeRequests == 0 && time.Since(t.lastUsed) > 10*time.Minute {
			httpTransportTrackers.Delete(key)
			return true
		}
		channels := make(map[int]int64, len(t.activeByChannel))
		for id, count := range t.activeByChannel {
			channels[id] = count
		}
		metrics = append(metrics, HTTPTransportMetric{PoolID: t.id, Protocol: t.protocol, Shard: t.shard, Shards: t.shards, ActiveRequests: t.activeRequests, Connections: t.connections, LastProtocol: t.lastProtocol, ActiveByChannel: channels})
		return true
	})
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].PoolID < metrics[j].PoolID })
	return metrics
}
