package service

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type transportAttemptKey struct{}

// TraceUpstreamTransport instruments one relay attempt, not the customer's entire
// request. It preserves any existing trace and never changes retry behavior.
func TraceUpstreamTransport(req *http.Request, timing *common.RequestTiming, channelID int, settings dto.ChannelSettings) (*http.Request, func(*http.Response, error)) {
	if timing == nil {
		return req, func(*http.Response, error) {}
	}
	a := &common.UpstreamTransportAttempt{}
	bodyBytes := req.ContentLength
	if req.Body == nil || req.Body == http.NoBody {
		bodyBytes = 0
	} else if bodyBytes <= 0 {
		bodyBytes = -1
	}
	policy := NormalizeHTTPTransportPolicy(settings).Protocol
	a.Update(func(s *common.UpstreamTransportSnapshot) {
		s.ChannelID, s.BodyBytes, s.Policy, s.Outcome = channelID, bodyBytes, policy, "pending"
		s.Reason = "disabled"
		if policy == dto.HTTPProtocolHTTP1 {
			s.Reason = "forced_http1"
		}
	})
	timing.AddUpstreamTransport(a)
	// All callback state is protected by the attempt's mutex. net/http may invoke
	// callbacks concurrently and may acquire/write more than once internally.
	var acquired, acquiring, dnsStart, tlsStart, written time.Time
	connectStarts := make(map[string]time.Time)
	trace := &httptrace.ClientTrace{
		ConnectStart: func(network, addr string) {
			a.Update(func(*common.UpstreamTransportSnapshot) {
				if len(connectStarts) < 16 {
					connectStarts[network+addr] = time.Now()
				}
			})
		},
		ConnectDone: func(network, addr string, err error) {
			a.Update(func(s *common.UpstreamTransportSnapshot) {
				key := network + addr
				if start, ok := connectStarts[key]; ok {
					if err == nil {
						v := float64(time.Since(start)) / float64(time.Millisecond)
						s.TCPMs = &v
					}
					delete(connectStarts, key)
				}
			})
		},
		GetConn: func(string) {
			a.Update(func(s *common.UpstreamTransportSnapshot) {
				acquiring, acquired, written = time.Now(), time.Time{}, time.Time{}
				s.ConnectionAcquisitions++
				s.AcquireMs, s.WriteMs, s.FirstByteMs = nil, nil, nil
				s.DNSMs, s.TCPMs, s.TLSMs = nil, nil, nil
				s.Reused = false
			})
		},
		GotConn: func(info httptrace.GotConnInfo) {
			a.Update(func(s *common.UpstreamTransportSnapshot) {
				acquired = time.Now()
				s.Reused = info.Reused
				if !acquiring.IsZero() {
					v := float64(acquired.Sub(acquiring)) / float64(time.Millisecond)
					s.AcquireMs = &v
				}
			})
		},
		DNSStart: func(httptrace.DNSStartInfo) {
			a.Update(func(*common.UpstreamTransportSnapshot) { dnsStart = time.Now() })
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			a.Update(func(s *common.UpstreamTransportSnapshot) {
				if !dnsStart.IsZero() {
					v := float64(time.Since(dnsStart)) / float64(time.Millisecond)
					s.DNSMs = &v
				}
			})
		},
		TLSHandshakeStart: func() { a.Update(func(*common.UpstreamTransportSnapshot) { tlsStart = time.Now() }) },
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			a.Update(func(s *common.UpstreamTransportSnapshot) {
				if !tlsStart.IsZero() {
					v := float64(time.Since(tlsStart)) / float64(time.Millisecond)
					s.TLSMs = &v
				}
			})
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			a.Update(func(s *common.UpstreamTransportSnapshot) {
				s.WriteCallbacks++
				if info.Err == nil {
					written = time.Now()
					if !acquired.IsZero() {
						v := float64(written.Sub(acquired)) / float64(time.Millisecond)
						s.WriteMs = &v
					}
				}
			})
		},
		GotFirstResponseByte: func() {
			a.Update(func(s *common.UpstreamTransportSnapshot) {
				if !written.IsZero() {
					v := float64(time.Since(written)) / float64(time.Millisecond)
					s.FirstByteMs = &v
				}
			})
		},
	}
	ctx := context.WithValue(req.Context(), transportAttemptKey{}, a)
	return req.WithContext(httptrace.WithClientTrace(ctx, trace)), func(resp *http.Response, err error) {
		a.Update(func(s *common.UpstreamTransportSnapshot) {
			if s.Outcome == "pending" {
				s.Outcome = "headers"
			}
			if resp != nil {
				s.Protocol, s.StatusCode = resp.Proto, resp.StatusCode
			}
			if err != nil {
				s.Outcome = transportErrorClass(err)
			}
		})
	}
}

func transportErrorClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return "timeout"
	}
	return "error"
}
