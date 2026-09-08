package common

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type StreamEndReason string

const (
	StreamEndReasonNone        StreamEndReason = ""
	StreamEndReasonDone        StreamEndReason = "done"
	StreamEndReasonTimeout     StreamEndReason = "timeout"
	StreamEndReasonClientGone  StreamEndReason = "client_gone"
	StreamEndReasonScannerErr  StreamEndReason = "scanner_error"
	StreamEndReasonHandlerStop StreamEndReason = "handler_stop"
	StreamEndReasonEOF         StreamEndReason = "eof"
	StreamEndReasonPanic       StreamEndReason = "panic"
	StreamEndReasonPingFail    StreamEndReason = "ping_fail"
)

const maxStreamErrorEntries = 20

type StreamErrorEntry struct {
	Message   string
	Timestamp time.Time
}

type StreamStatus struct {
	EndReason StreamEndReason
	EndError  error
	endOnce   sync.Once
	endMu     sync.Mutex

	mu         sync.Mutex
	Errors     []StreamErrorEntry
	ErrorCount int
}

func NewStreamStatus() *StreamStatus {
	return &StreamStatus{}
}

func (s *StreamStatus) SetEndReason(reason StreamEndReason, err error) {
	if s == nil {
		return
	}
	s.endMu.Lock()
	defer s.endMu.Unlock()
	s.endOnce.Do(func() {
		s.EndReason = reason
		s.EndError = err
	})
}

// SetClientGone records an explicit downstream cancellation even when a
// scanner error won the shutdown race first. Once the request context is gone,
// that attempt says nothing about upstream health and must not become hard.
func (s *StreamStatus) SetClientGone(err error) {
	if s == nil {
		return
	}
	s.endMu.Lock()
	defer s.endMu.Unlock()
	s.endOnce.Do(func() {})
	s.EndReason = StreamEndReasonClientGone
	s.EndError = err
}

func (s *StreamStatus) End() (StreamEndReason, error) {
	if s == nil {
		return StreamEndReasonNone, nil
	}
	s.endMu.Lock()
	defer s.endMu.Unlock()
	return s.EndReason, s.EndError
}

func (s *StreamStatus) RecordError(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
	if len(s.Errors) < maxStreamErrorEntries {
		s.Errors = append(s.Errors, StreamErrorEntry{
			Message:   msg,
			Timestamp: time.Now(),
		})
	}
}

func (s *StreamStatus) HasErrors() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount > 0
}

func (s *StreamStatus) TotalErrorCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount
}

func (s *StreamStatus) IsNormalEnd() bool {
	if s == nil {
		return true
	}
	reason, _ := s.End()
	return reason == StreamEndReasonDone ||
		reason == StreamEndReasonEOF ||
		reason == StreamEndReasonHandlerStop
}

func (s *StreamStatus) Summary() string {
	if s == nil {
		return "StreamStatus<nil>"
	}
	reason, endErr := s.End()
	b := &strings.Builder{}
	fmt.Fprintf(b, "reason=%s", reason)
	if endErr != nil {
		fmt.Fprintf(b, " end_error=%q", endErr.Error())
	}
	s.mu.Lock()
	if s.ErrorCount > 0 {
		fmt.Fprintf(b, " soft_errors=%d", s.ErrorCount)
	}
	s.mu.Unlock()
	return b.String()
}
