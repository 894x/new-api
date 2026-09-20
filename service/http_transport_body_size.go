package service

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
)

// bodySizeRoundTripper chooses a pool once, using the final request metadata.
// It never reads/buffers the body or replays a failed request on another pool.
type bodySizeRoundTripper struct {
	normal    http.RoundTripper
	large     http.RoundTripper
	threshold int64
}

func (t *bodySizeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := t.normal
	reason, policy := "small_body", "auto"
	if req.Body != nil && req.Body != http.NoBody && req.ContentLength <= 0 {
		reason = "unknown_length"
	}
	if req.Body != nil && req.Body != http.NoBody && req.ContentLength > 0 && req.ContentLength >= t.threshold {
		transport = t.large
		reason, policy = "large_body", "http1"
	}
	if attempt, ok := req.Context().Value(transportAttemptKey{}).(*common.UpstreamTransportAttempt); ok {
		attempt.Update(func(s *common.UpstreamTransportSnapshot) {
			s.Reason, s.Policy, s.ThresholdBytes = reason, policy, t.threshold
		})
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(req)
}

func (t *bodySizeRoundTripper) CloseIdleConnections() {
	closeIdleConnections(t.normal)
	closeIdleConnections(t.large)
}
