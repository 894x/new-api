package common

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const requestTimingKey = "request_timing"

type RequestTimingMilestone uint8

const (
	RequestTimingReceived RequestTimingMilestone = iota
	RequestTimingBodyRead
	RequestTimingUpstreamRequestStarted
	RequestTimingUpstreamRequestWritten
	RequestTimingUpstreamResponseHeaders
	RequestTimingFirstResponse
	RequestTimingCompleted
)

type RequestTimingSnapshot struct {
	RequestReceivedAtMs         int64 `json:"request_received_at_ms,omitempty"`
	RequestBodyReadAtMs         int64 `json:"request_body_read_at_ms,omitempty"`
	UpstreamRequestStartedAtMs  int64 `json:"upstream_request_started_at_ms,omitempty"`
	UpstreamRequestWrittenAtMs  int64 `json:"upstream_request_written_at_ms,omitempty"`
	UpstreamResponseHeadersAtMs int64 `json:"upstream_response_headers_at_ms,omitempty"`
	FirstResponseAtMs           int64 `json:"first_response_at_ms,omitempty"`
	RequestCompletedAtMs        int64 `json:"request_completed_at_ms,omitempty"`
}

type RequestTiming struct {
	mu         sync.RWMutex
	milestones map[RequestTimingMilestone]time.Time
}

func NewRequestTiming(receivedAt time.Time) *RequestTiming {
	timing := &RequestTiming{
		milestones: make(map[RequestTimingMilestone]time.Time, 7),
	}
	timing.Mark(RequestTimingReceived, receivedAt)
	return timing
}

// Mark records the first observation for a milestone. Some providers invoke
// response callbacks more than once, so later observations must not move the
// original boundary.
func (timing *RequestTiming) Mark(milestone RequestTimingMilestone, at time.Time) {
	if timing == nil || at.IsZero() {
		return
	}
	timing.mu.Lock()
	defer timing.mu.Unlock()
	if _, exists := timing.milestones[milestone]; exists {
		return
	}
	timing.milestones[milestone] = at
}

func (timing *RequestTiming) Snapshot() RequestTimingSnapshot {
	if timing == nil {
		return RequestTimingSnapshot{}
	}
	timing.mu.RLock()
	defer timing.mu.RUnlock()

	millis := func(milestone RequestTimingMilestone) int64 {
		at := timing.milestones[milestone]
		if at.IsZero() {
			return 0
		}
		return at.UnixMilli()
	}
	return RequestTimingSnapshot{
		RequestReceivedAtMs:         millis(RequestTimingReceived),
		RequestBodyReadAtMs:         millis(RequestTimingBodyRead),
		UpstreamRequestStartedAtMs:  millis(RequestTimingUpstreamRequestStarted),
		UpstreamRequestWrittenAtMs:  millis(RequestTimingUpstreamRequestWritten),
		UpstreamResponseHeadersAtMs: millis(RequestTimingUpstreamResponseHeaders),
		FirstResponseAtMs:           millis(RequestTimingFirstResponse),
		RequestCompletedAtMs:        millis(RequestTimingCompleted),
	}
}

func StartRequestTiming(c *gin.Context, receivedAt time.Time) *RequestTiming {
	if c == nil {
		return nil
	}
	timing := NewRequestTiming(receivedAt)
	c.Set(requestTimingKey, timing)
	return timing
}

func GetRequestTiming(c *gin.Context) *RequestTiming {
	if c == nil {
		return nil
	}
	timing, ok := GetContextKeyType[*RequestTiming](c, requestTimingKey)
	if !ok {
		return nil
	}
	return timing
}

func MarkRequestTiming(c *gin.Context, milestone RequestTimingMilestone) {
	if timing := GetRequestTiming(c); timing != nil {
		timing.Mark(milestone, time.Now())
	}
}
