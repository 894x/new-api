package common

import "sync"

// Durations are optional: missing callbacks must not become zero-millisecond samples.
// BodyBytes=-1 denotes unknown length; no payload, URL or credentials are retained.
type UpstreamTransportSnapshot struct {
	Attempt                int      `json:"attempt"`
	ChannelID              int      `json:"channel_id"`
	BodyBytes              int64    `json:"body_bytes"`
	ThresholdBytes         int64    `json:"threshold_bytes,omitempty"`
	Reason                 string   `json:"reason"`
	Policy                 string   `json:"policy"`
	Protocol               string   `json:"protocol,omitempty"`
	Reused                 bool     `json:"reused"`
	ConnectionAcquisitions int      `json:"connection_acquisitions"`
	WriteCallbacks         int      `json:"write_callbacks"`
	AcquireMs              *float64 `json:"acquire_ms,omitempty"`
	DNSMs                  *float64 `json:"dns_ms,omitempty"`
	TLSMs                  *float64 `json:"tls_ms,omitempty"`
	TCPMs                  *float64 `json:"tcp_ms,omitempty"`
	WriteMs                *float64 `json:"write_ms,omitempty"`
	FirstByteMs            *float64 `json:"first_byte_ms,omitempty"`
	Outcome                string   `json:"outcome"`
	StatusCode             int      `json:"status_code,omitempty"`
}

type UpstreamTransportAttempt struct {
	mu   sync.Mutex
	data UpstreamTransportSnapshot
}

func (a *UpstreamTransportAttempt) Update(update func(*UpstreamTransportSnapshot)) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	update(&a.data)
}

func (a *UpstreamTransportAttempt) Snapshot() UpstreamTransportSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.data
}
