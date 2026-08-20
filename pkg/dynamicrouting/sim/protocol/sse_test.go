package protocol_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting/sim"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting/sim/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type manualClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

func TestMeasureUsesRealSSEEventsForTTFTAndTPOT(t *testing.T) {
	clock := &manualClock{now: time.Unix(0, 0)}
	ack := make(chan struct{})
	server := httptest.NewServer(protocol.NewHandler(protocol.StreamScript{
		FirstEventDelay:        40 * time.Millisecond,
		MetadataOnlyFirstEvent: true,
		FirstContentAfterEvent: 60 * time.Millisecond,
		PerTokenDelay:          25 * time.Millisecond,
		Tokens:                 []string{"one", "two", "three"},
	}, protocol.HandlerHooks{
		Wait:       clock.Advance,
		AfterFlush: func() { <-ack },
	}))
	t.Cleanup(server.Close)

	metrics, err := protocol.Measure(context.Background(), server.Client(), server.URL, protocol.MeasureOptions{
		Now:        clock.Now,
		AfterEvent: func() { ack <- struct{}{} },
	})
	require.NoError(t, err)
	assert.Equal(t, 40*time.Millisecond, metrics.TTFTAnyEvent)
	assert.Equal(t, 100*time.Millisecond, metrics.TTFTContent)
	assert.Equal(t, 25*time.Millisecond, metrics.TPOT)
	assert.Equal(t, 3, metrics.ContentEvents)
	assert.Equal(t, http.StatusOK, metrics.StatusCode)
	assert.True(t, metrics.Completed)
	assert.NotEqual(t, metrics.TTFTAnyEvent, metrics.TTFTContent, "metadata-only SSE frames are not visible first-token latency")
}

func TestMeasureCapturesHTTPFaultLatencyAndIncompleteStreams(t *testing.T) {
	for _, statusCode := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			clock := &manualClock{now: time.Unix(0, 0)}
			server := httptest.NewServer(protocol.NewHandler(protocol.StreamScript{
				StatusCode: statusCode, ResponseDelay: 80 * time.Millisecond,
			}, protocol.HandlerHooks{Wait: clock.Advance}))
			t.Cleanup(server.Close)

			metrics, err := protocol.Measure(context.Background(), server.Client(), server.URL, protocol.MeasureOptions{Now: clock.Now})
			require.NoError(t, err)
			assert.Equal(t, statusCode, metrics.StatusCode)
			assert.Equal(t, 80*time.Millisecond, metrics.TTFTAnyEvent)
			assert.False(t, metrics.Completed)
		})
	}

	clock := &manualClock{now: time.Unix(0, 0)}
	ack := make(chan struct{})
	server := httptest.NewServer(protocol.NewHandler(protocol.StreamScript{
		FirstEventDelay: 100 * time.Millisecond, PerTokenDelay: 20 * time.Millisecond,
		Tokens: []string{"one", "two"}, CloseBeforeDone: true,
	}, protocol.HandlerHooks{Wait: clock.Advance, AfterFlush: func() { <-ack }}))
	t.Cleanup(server.Close)

	metrics, err := protocol.Measure(context.Background(), server.Client(), server.URL, protocol.MeasureOptions{
		Now: clock.Now, AfterEvent: func() { ack <- struct{}{} },
	})
	assert.ErrorIs(t, err, protocol.ErrIncompleteStream)
	assert.Equal(t, 100*time.Millisecond, metrics.TTFTContent)
	assert.Equal(t, 20*time.Millisecond, metrics.TPOT)
	assert.False(t, metrics.Completed)
}

func TestMeasuredSSEFeedbackMovesTrafficFromBadAToGoodB(t *testing.T) {
	clock := &manualClock{now: time.Unix(0, 0)}
	ack := make(chan struct{})
	hooks := protocol.HandlerHooks{Wait: clock.Advance, AfterFlush: func() { <-ack }}
	goodA := protocol.NewHandler(protocol.StreamScript{
		FirstEventDelay: 80 * time.Millisecond, PerTokenDelay: 10 * time.Millisecond,
		Tokens: []string{"one", "two", "three"},
	}, hooks)
	badA := protocol.NewHandler(protocol.StreamScript{
		FirstEventDelay: 1200 * time.Millisecond, PerTokenDelay: 80 * time.Millisecond,
		Tokens: []string{"one", "two", "three"},
	}, hooks)
	goodB := protocol.NewHandler(protocol.StreamScript{
		FirstEventDelay: 120 * time.Millisecond, PerTokenDelay: 15 * time.Millisecond,
		Tokens: []string{"one", "two", "three"},
	}, hooks)
	var degraded atomic.Bool
	serverA := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if degraded.Load() {
			badA.ServeHTTP(writer, request)
			return
		}
		goodA.ServeHTTP(writer, request)
	}))
	t.Cleanup(serverA.Close)
	serverB := httptest.NewServer(goodB)
	t.Cleanup(serverB.Close)

	policy, err := sim.NewDynamicPolicy(dynamicrouting.Config{
		Enabled: true, MaxSamples: 30, MaxAge: time.Minute, MinSamples: 3,
		ProbeFraction: 0.2, DegradationThreshold: 1.3, RecoveryThreshold: 1.1,
		CriticalThreshold: 2, CandidateAdvantage: 1.1, Aggressiveness: 0.9,
		RecoveryStep: 0.02,
	}, 1, dynamicrouting.RouteKey{Group: "protocol", Model: "stream-model"})
	require.NoError(t, err)
	candidates := []sim.Candidate{
		{ID: "a", ChannelID: 1, Priority: 100, Weight: 100},
		{ID: "b", ChannelID: 2, Priority: 50, Weight: 100},
	}
	startedAt := clock.Now()
	preFaultB := 0
	postFaultA := 0
	firstBDominantRequest := -1
	for requestIndex := 0; requestIndex < 30; requestIndex++ {
		if requestIndex == 15 {
			degraded.Store(true)
		}
		now := clock.Now().Sub(startedAt)
		decision := policy.Select(now, candidates)
		endpoint := serverA.URL
		if decision.CandidateID == "b" {
			endpoint = serverB.URL
			if requestIndex < 15 {
				preFaultB++
			}
		} else if requestIndex >= 15 {
			postFaultA++
		}
		if requestIndex >= 15 && firstBDominantRequest < 0 && decision.DominantCandidate == "b" {
			firstBDominantRequest = requestIndex - 15
		}
		metrics, measureErr := protocol.Measure(context.Background(), http.DefaultClient, endpoint, protocol.MeasureOptions{
			Now: clock.Now, AfterEvent: func() { ack <- struct{}{} },
		})
		require.NoError(t, measureErr)
		completedAt := clock.Now().Sub(startedAt)
		policy.Observe(sim.Sample{
			CandidateID: decision.CandidateID, ArrivedAt: now, CompletedAt: completedAt,
			TTFT: metrics.TTFTContent, TPOT: metrics.TPOT, Success: metrics.Completed,
		})
	}

	assert.GreaterOrEqual(t, preFaultB, 3)
	assert.LessOrEqual(t, firstBDominantRequest, 5)
	assert.LessOrEqual(t, postFaultA, 5)
}
