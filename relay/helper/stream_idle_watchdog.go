package helper

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// StreamIdleWatchdog tracks time since the last upstream stream event. A nil
// channel disables the watchdog, preserving STREAMING_TIMEOUT=0 semantics.
type StreamIdleWatchdog struct {
	mu           sync.Mutex
	timeout      time.Duration
	lastActivity time.Time
	activity     chan struct{}
	ticks        chan time.Time
	stop         chan struct{}
	done         chan struct{}
	stopOnce     sync.Once
	stopped      bool
}

func NewStreamIdleWatchdog(timeout time.Duration) *StreamIdleWatchdog {
	watchdog := &StreamIdleWatchdog{timeout: timeout}
	if timeout <= 0 {
		return watchdog
	}
	watchdog.lastActivity = time.Now()
	watchdog.activity = make(chan struct{}, 1)
	watchdog.ticks = make(chan time.Time, 1)
	watchdog.stop = make(chan struct{})
	watchdog.done = make(chan struct{})
	go watchdog.run()
	return watchdog
}

func NewConfiguredStreamIdleWatchdog() *StreamIdleWatchdog {
	return NewStreamIdleWatchdog(time.Duration(constant.StreamingTimeout) * time.Second)
}

func (w *StreamIdleWatchdog) C() <-chan time.Time {
	if w == nil {
		return nil
	}
	return w.ticks
}

// Reset records one upstream event, including metadata and terminal events.
func (w *StreamIdleWatchdog) Reset() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.activity == nil || w.stopped {
		w.mu.Unlock()
		return
	}
	w.lastActivity = time.Now()
	w.mu.Unlock()
	select {
	case w.activity <- struct{}{}:
	default:
	}
}

func (w *StreamIdleWatchdog) Stop() {
	if w == nil || w.stop == nil {
		return
	}
	w.stopOnce.Do(func() {
		w.mu.Lock()
		w.stopped = true
		w.mu.Unlock()
		close(w.stop)
	})
	<-w.done
}

func (w *StreamIdleWatchdog) WrapReadCloser(upstream io.ReadCloser) io.ReadCloser {
	return &streamIdleActivityReadCloser{ReadCloser: upstream, watchdog: w}
}

type streamIdleActivityReadCloser struct {
	io.ReadCloser
	watchdog *StreamIdleWatchdog
}

func (r *streamIdleActivityReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.watchdog.Reset()
	}
	return n, err
}

func (w *StreamIdleWatchdog) run() {
	defer close(w.done)
	timer := time.NewTimer(w.timeout)
	defer timer.Stop()
	for {
		select {
		case <-w.activity:
			w.resetOwnedTimer(timer)
		case tick := <-timer.C:
			w.mu.Lock()
			remaining := w.timeout - time.Since(w.lastActivity)
			w.mu.Unlock()
			if remaining > 0 {
				timer.Reset(remaining)
				continue
			}
			w.ticks <- tick
			return
		case <-w.stop:
			return
		}
	}
}

func (w *StreamIdleWatchdog) resetOwnedTimer(timer *time.Timer) {
	w.mu.Lock()
	remaining := w.timeout - time.Since(w.lastActivity)
	w.mu.Unlock()
	if remaining <= 0 {
		remaining = time.Nanosecond
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(remaining)
}

// CloseUpstreamOnIdleTimeout interrupts a blocking upstream read when the
// configured idle interval elapses. The producer's terminal error cannot
// overwrite Timeout because StreamStatus keeps the first terminal reason.
func CloseUpstreamOnIdleTimeout(ctx context.Context, info *relaycommon.RelayInfo, upstream io.Closer, completed <-chan struct{}, watchdog *StreamIdleWatchdog) <-chan struct{} {
	done := make(chan struct{})
	if watchdog == nil || watchdog.C() == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
		case <-completed:
		case <-watchdog.C():
			if requestErr := ctx.Err(); requestErr != nil {
				info.StreamStatus.SetClientGone(requestErr)
			} else {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, context.DeadlineExceeded)
			}
			_ = upstream.Close()
		}
	}()
	return done
}
