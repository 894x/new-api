package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	InitialScannerBufferSize    = 64 << 10  // 64KB (64*1024)
	DefaultMaxScannerBufferSize = 128 << 20 // 128 MiB default maximum SSE event size
	DefaultPingInterval         = 10 * time.Second
	// streamWriteTimeout bounds a single blocked write to a slow client so the
	// unconditional wg.Wait() in cleanup can always finish. Without it, a slow
	// but connected client (full TCP buffer, no server WriteTimeout) could hang
	// the handler forever.
	streamWriteTimeout = 30 * time.Second
)

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

func NewStreamScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	return scanner
}

// StreamChunkVisibleText extracts only well-known user-visible text deltas.
// Unknown provider payloads stay unmeasured instead of treating metadata,
// tools, media, or reasoning traces as user output.
func StreamChunkVisibleText(data string) string {
	if !gjson.Valid(data) {
		return ""
	}

	var visible strings.Builder
	for _, choice := range gjson.Get(data, "choices").Array() {
		text := choice.Get("text")
		if text.Type == gjson.String && text.String() != "" {
			visible.WriteString(text.String())
		}
		content := choice.Get("delta.content")
		if content.Type == gjson.String && content.String() != "" {
			visible.WriteString(content.String())
		}
		if content.IsArray() {
			for _, part := range content.Array() {
				partType := strings.ToLower(strings.TrimSpace(part.Get("type").String()))
				if partType != "" && partType != "text" && partType != "output_text" && partType != "refusal" {
					continue
				}
				if text := part.Get("text"); text.Type == gjson.String && text.String() != "" {
					visible.WriteString(text.String())
				}
			}
		}
		refusal := choice.Get("delta.refusal")
		if refusal.Type == gjson.String && refusal.String() != "" {
			visible.WriteString(refusal.String())
		}
	}
	if visible.Len() > 0 {
		return visible.String()
	}

	eventType := gjson.Get(data, "type").String()
	if eventType == "response.output_text.delta" || eventType == "response.refusal.delta" {
		delta := gjson.Get(data, "delta")
		if delta.Type == gjson.String {
			return delta.String()
		}
		return ""
	}
	if eventType == "content_block_delta" && gjson.Get(data, "delta.type").String() == "text_delta" {
		text := gjson.Get(data, "delta.text")
		if text.Type == gjson.String {
			return text.String()
		}
		return ""
	}

	for _, candidate := range gjson.Get(data, "candidates").Array() {
		for _, part := range candidate.Get("content.parts").Array() {
			if part.Get("thought").Bool() {
				continue
			}
			text := part.Get("text")
			if text.Type == gjson.String && text.String() != "" {
				visible.WriteString(text.String())
			}
		}
	}

	return visible.String()
}

func StreamChunkHasVisibleText(data string) bool {
	return StreamChunkVisibleText(data) != ""
}

type attemptVisibleContentRecorder interface {
	RecordAttemptVisibleText(string)
}

type AttemptBackpressureMarker interface {
	MarkDynamicRoutingAttemptBackpressure()
}

func recordAttemptVisibleStreamChunk(recorder attemptVisibleContentRecorder, data string) {
	if text := StreamChunkVisibleText(data); text != "" {
		recorder.RecordAttemptVisibleText(text)
	}
}

// EnqueueStreamDataWithBackpressure preserves the bounded stream queue while
// marking timing as unreliable before waiting for a slow downstream consumer.
func EnqueueStreamItemWithBackpressure[T any](ctx context.Context, dataChan chan<- T, data T, marker AttemptBackpressureMarker) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	select {
	case dataChan <- data:
		return true
	default:
		marker.MarkDynamicRoutingAttemptBackpressure()
	}

	select {
	case dataChan <- data:
		return true
	case <-ctx.Done():
		return false
	}
}

func EnqueueStreamDataWithBackpressure(ctx context.Context, dataChan chan<- string, data string, marker AttemptBackpressureMarker) bool {
	return EnqueueStreamItemWithBackpressure(ctx, dataChan, data, marker)
}

// CloseUpstreamOnContext actively interrupts a provider read when the client
// context ends. completed must close when the producer exits, which also
// stops this watcher without touching a normally completed upstream.
func CloseUpstreamOnContext(ctx context.Context, upstream io.Closer, completed <-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = upstream.Close()
		case <-completed:
		}
	}()
	return done
}

func copyCodexSSEHeaders(c *gin.Context, resp *http.Response) {
	if c == nil || c.Writer == nil || resp == nil {
		return
	}
	// codex
	for _, name := range []string{"X-Reasoning-Included", "X-Codex-Turn-State"} {
		values := resp.Header.Values(name)
		if !service.ShouldCopyUpstreamHeader(c, name, values) {
			continue
		}
		for _, value := range values {
			if value != "" {
				c.Writer.Header().Add(name, value)
			}
		}
	}
}

// ExtendWriteDeadline pushes the connection write deadline forward before each
// stream write. Best-effort: writers that don't support deadlines (e.g.
// httptest recorders) are silently ignored.
func ExtendWriteDeadline(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(streamWriteTimeout))
}

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string, sr *StreamResult)) {
	StreamScannerHandlerWithVisibleText(c, resp, info, StreamChunkVisibleText, dataHandler)
}

// StreamScannerHandlerWithVisibleText lets provider-specific SSE dialects
// identify their public text while retaining the shared bounded ingress,
// cancellation, and stream lifecycle handling.
func StreamScannerHandlerWithVisibleText(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, visibleText func(string) string, dataHandler func(data string, sr *StreamResult)) {

	if resp == nil || dataHandler == nil {
		return
	}

	// 无条件新建 StreamStatus
	info.StreamStatus = relaycommon.NewStreamStatus()

	ctx, cancel := context.WithCancel(context.Background())

	idleWatchdog := NewConfiguredStreamIdleWatchdog()

	var (
		stopChan    = make(chan bool, 3) // 增加缓冲区避免阻塞
		scanner     = NewStreamScanner(resp.Body)
		pingTicker  *time.Ticker
		writeMutex  sync.Mutex     // Mutex to protect concurrent writes
		wg          sync.WaitGroup // 用于等待所有 goroutine 退出
		cleanupOnce sync.Once
		stopOnce    sync.Once
		scannerEnd  = relaycommon.StreamEndReasonEOF
		scannerErr  error
	)

	stop := func() {
		stopOnce.Do(func() {
			close(stopChan)
		})
	}

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	logger.LogDebug(c, "relay timeout seconds: %d", common.RelayTimeout)
	logger.LogDebug(c, "relay max idle conns: %d", common.RelayMaxIdleConns)
	logger.LogDebug(c, "relay max idle conns per host: %d", common.RelayMaxIdleConnsPerHost)
	logger.LogDebug(c, "streaming timeout seconds: %d", constant.StreamingTimeout)
	logger.LogDebug(c, "ping interval seconds: %d", int64(pingInterval.Seconds()))

	cleanup := func() {
		cleanupOnce.Do(func() {
			cancel()
			stop()
			if resp.Body != nil {
				_ = resp.Body.Close()
			}

			idleWatchdog.Stop()
			if pingTicker != nil {
				pingTicker.Stop()
			}

			wg.Wait()
		})
	}
	// Ensure gin.Context is not returned to Gin's pool while any stream goroutine can still use it.
	defer cleanup()

	scanner.Split(bufio.ScanLines)
	copyCodexSSEHeaders(c, resp)
	SetEventStreamHeaders(c)

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	// Handle ping data sending with improved error handling
	if pingEnabled && pingTicker != nil {
		wg.Add(1)
		gopool.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					logger.LogError(c, fmt.Sprintf("ping goroutine panic: %v", r))
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("ping panic: %v", r))
					stop()
				}
				logger.LogDebug(c, "ping goroutine exited")
				wg.Done()
			}()

			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute // 最大 ping 持续时间
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()

			for {
				select {
				case <-pingTicker.C:
					var err error
					func() {
						writeMutex.Lock()
						defer writeMutex.Unlock()
						ExtendWriteDeadline(c)
						err = PingData(c)
					}()
					if err != nil {
						logger.LogError(c, "ping data error: "+err.Error())
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
						return
					}
					logger.LogDebug(c, "ping data sent")
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					// 监听客户端断开连接
					return
				case <-pingTimeout.C:
					logger.LogError(c, "ping goroutine max duration reached")
					return
				}
			}
		})
	}

	dataChan := make(chan string, 10)

	wg.Add(1)
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("data handler goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("handler panic: %v", r))
			}
			stop()
			wg.Done()
		}()
		sr := newStreamResult(info.StreamStatus)
		for data := range dataChan {
			sr.reset()
			func() {
				writeMutex.Lock()
				defer writeMutex.Unlock()
				ExtendWriteDeadline(c)
				dataHandler(data, sr)
			}()
			if sr.IsStopped() {
				return
			}
		}
		// Apply the transport terminal only after every queued upstream event
		// has been handled. Otherwise a fast scanner can publish EOF/[DONE]
		// before a buffered event reports a protocol or decode failure.
		info.StreamStatus.SetEndReason(scannerEnd, scannerErr)
	})

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("scanner panic: %v", r))
			}
			close(dataChan)
			stop()
			logger.LogDebug(c, "scanner goroutine exited")
			wg.Done()
		}()

		firstResponseRecorded := false
		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			default:
			}

			idleWatchdog.Reset()
			data := scanner.Text()
			logger.LogDebug(c, "stream scanner data: %s", data)

			if len(data) < 6 {
				continue
			}
			if data[:5] != "data:" && data[:6] != "[DONE]" {
				continue
			}
			data = data[5:]
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}
			if !strings.HasPrefix(data, "[DONE]") {
				if IsNullJSONStreamEvent(data) {
					scannerEnd = relaycommon.StreamEndReasonScannerErr
					scannerErr = ErrNullJSONStreamEvent
					return
				}
				if !firstResponseRecorded {
					info.SetFirstResponseTime()
					firstResponseRecorded = true
				}
				if visibleText != nil {
					if text := visibleText(data); text != "" {
						info.RecordAttemptVisibleText(text)
					}
				}
				info.ReceivedResponseCount++

				if !EnqueueStreamDataWithBackpressure(ctx, dataChan, data, info) {
					return
				}
			} else {
				scannerEnd = relaycommon.StreamEndReasonDone
				logger.LogDebug(c, "received [DONE], stopping scanner")
				return
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, "scanner error: "+err.Error())
				scannerEnd = relaycommon.StreamEndReasonScannerErr
				scannerErr = err
			}
		}
	})

	// 主循环等待完成或超时
	select {
	case <-idleWatchdog.C():
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, nil)
	case <-stopChan:
		// EndReason already set by the goroutine that triggered stopChan
	case <-c.Request.Context().Done():
		// 客户端断开：立即 cleanup 关闭上游 resp.Body，解除 scanner 阻塞并让上游停止生成，
		// 避免为已放弃的请求继续消费上游 token。
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, c.Request.Context().Err())
	}

	cleanup()
	if requestErr := c.Request.Context().Err(); requestErr != nil {
		info.StreamStatus.SetClientGone(requestErr)
	}
	if info.StreamStatus.IsNormalEnd() && !info.StreamStatus.HasErrors() {
		logger.LogInfo(c, fmt.Sprintf("stream ended: %s", info.StreamStatus.Summary()))
	} else {
		logger.LogError(c, fmt.Sprintf("stream ended: %s, received=%d", info.StreamStatus.Summary(), info.ReceivedResponseCount))
	}
}
