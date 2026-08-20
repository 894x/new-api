package helper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cancelingErrorReadCloser struct {
	cancel context.CancelFunc
	once   sync.Once
}

func (r *cancelingErrorReadCloser) Read([]byte) (int, error) {
	r.once.Do(r.cancel)
	return 0, errors.New("scanner read failed after cancellation")
}

func (*cancelingErrorReadCloser) Close() error { return nil }

type visibleContentRecorderSpy struct {
	calls int
	texts []string
}

func (s *visibleContentRecorderSpy) RecordAttemptVisibleText(text string) {
	s.calls++
	s.texts = append(s.texts, text)
}

type attemptTPOTInvalidatorSpy struct {
	invalidated chan struct{}
	once        sync.Once
}

func (s *attemptTPOTInvalidatorSpy) MarkDynamicRoutingAttemptBackpressure() {
	s.once.Do(func() { close(s.invalidated) })
}

func init() {
	gin.SetMode(gin.TestMode)
	if constant.StreamingTimeout == 0 {
		constant.StreamingTimeout = 30
	}
}

func setupStreamTest(t *testing.T, body io.Reader) (*gin.Context, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{
		Body: io.NopCloser(body),
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	return c, resp, info
}

func buildSSEBody(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "data: {\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}\n", i, i)
	}
	b.WriteString("data: [DONE]\n")
	return b.String()
}

func TestStreamChunkHasVisibleText(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		visible bool
	}{
		{name: "metadata only", data: `{"id":"chatcmpl_1","choices":[{"delta":{"role":"assistant"}}]}`},
		{name: "openai content", data: `{"choices":[{"delta":{"content":"hello"}}]}`, visible: true},
		{name: "openai refusal", data: `{"choices":[{"delta":{"refusal":"I cannot help with that"}}]}`, visible: true},
		{name: "openai empty content", data: `{"choices":[{"delta":{"content":""}}]}`},
		{name: "openai completions text", data: `{"choices":[{"text":"hello"}]}`, visible: true},
		{name: "openai completions empty text", data: `{"choices":[{"text":""}]}`},
		{name: "responses lifecycle", data: `{"type":"response.created","response":{"id":"resp_1"}}`},
		{name: "responses output text", data: `{"type":"response.output_text.delta","delta":"hello"}`, visible: true},
		{name: "responses refusal", data: `{"type":"response.refusal.delta","delta":"I cannot help with that"}`, visible: true},
		{name: "responses reasoning text", data: `{"type":"response.reasoning_text.delta","delta":"hidden"}`},
		{name: "anthropic message metadata", data: `{"type":"message_start","message":{"id":"msg_1"}}`},
		{name: "anthropic text delta", data: `{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`, visible: true},
		{name: "anthropic thinking delta", data: `{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hidden"}}`},
		{name: "gemini text", data: `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`, visible: true},
		{name: "gemini thought text", data: `{"candidates":[{"content":{"parts":[{"thought":true,"text":"hidden"}]}}]}`},
		{name: "gemini usage metadata", data: `{"usageMetadata":{"promptTokenCount":5}}`},
		{name: "invalid json", data: `not-json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.visible, StreamChunkHasVisibleText(tt.data))
		})
	}
}

func TestStreamChunkVisibleTextExcludesNonVisibleModalities(t *testing.T) {
	tests := []struct {
		name string
		data string
		text string
	}{
		{
			name: "chat content excludes reasoning and tools",
			data: `{"choices":[{"delta":{"reasoning_content":"hidden","content":"visible","tool_calls":[{"id":"call_1"}]}}]}`,
			text: "visible",
		},
		{
			name: "chat content array includes only visible text parts",
			data: `{"choices":[{"delta":{"content":[{"type":"reasoning","text":"hidden-r"},{"type":"thinking","text":"hidden-t"},{"type":"analysis","text":"hidden-a"},{"type":"tool_call","text":"hidden-tool"},{"type":"audio","text":"hidden-audio"},{"type":"vendor_unknown","text":"hidden-unknown"},{"type":"text","text":"visible"},{"type":"output_text","text":" output"},{"type":"refusal","text":" refusal"},{"text":" plain"}]}}]}`,
			text: "visible output refusal plain",
		},
		{name: "chat refusal", data: `{"choices":[{"delta":{"refusal":"chat refusal"}}]}`, text: "chat refusal"},
		{name: "completions text", data: `{"choices":[{"text":"completion"}]}`, text: "completion"},
		{name: "responses text", data: `{"type":"response.output_text.delta","delta":"response"}`, text: "response"},
		{name: "responses refusal", data: `{"type":"response.refusal.delta","delta":"responses refusal"}`, text: "responses refusal"},
		{name: "anthropic text", data: `{"type":"content_block_delta","delta":{"type":"text_delta","text":"anthropic"}}`, text: "anthropic"},
		{
			name: "gemini excludes thought and media",
			data: `{"candidates":[{"content":{"parts":[{"thought":true,"text":"hidden"},{"inlineData":{"mimeType":"audio/wav"}},{"text":"gemini"}]}}]}`,
			text: "gemini",
		},
		{name: "tool only", data: `{"choices":[{"delta":{"tool_calls":[{"id":"call_1"}]}}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.text, StreamChunkVisibleText(tt.data))
		})
	}
}

func TestRecordAttemptVisibleStreamChunkRecordsEveryVisibleEvent(t *testing.T) {
	recorder := &visibleContentRecorderSpy{}
	for _, data := range []string{
		`{"choices":[{"delta":{"role":"assistant"}}]}`,
		`{"choices":[{"delta":{"content":"hel"}}]}`,
		`{"choices":[{"delta":{"content":"lo"}}]}`,
	} {
		recordAttemptVisibleStreamChunk(recorder, data)
	}

	assert.Equal(t, 2, recorder.calls)
}

func TestRecordAttemptVisibleStreamChunkWaitsForVisibleContentPart(t *testing.T) {
	recorder := &visibleContentRecorderSpy{}
	recordAttemptVisibleStreamChunk(recorder, `{"choices":[{"delta":{"content":[{"type":"reasoning","text":"hidden"}]}}]}`)
	assert.Zero(t, recorder.calls)

	recordAttemptVisibleStreamChunk(recorder, `{"choices":[{"delta":{"content":[{"type":"text","text":"visible"}]}}]}`)

	assert.Equal(t, 1, recorder.calls)
	assert.Equal(t, []string{"visible"}, recorder.texts)
}

func TestRefusalFirstFrameSetsVisibleTimingAndDenominatorText(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		text    string
	}{
		{
			name:    "chat refusal",
			payload: `{"choices":[{"delta":{"refusal":"chat refusal"}}]}`,
			text:    "chat refusal",
		},
		{
			name:    "responses refusal",
			payload: `{"type":"response.refusal.delta","delta":"responses refusal"}`,
			text:    "responses refusal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{IsStream: true}
			info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), "public-model", true)
			info.MarkAttemptUpstreamStarted()

			recordAttemptVisibleStreamChunk(info, tt.payload)

			assert.Equal(t, tt.text, info.DynamicRoutingAttemptVisibleText())
			info.StreamStatus = relaycommon.NewStreamStatus()
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			sample, observed := info.FinishDynamicRoutingAttempt(nil)
			require.True(t, observed)
			assert.True(t, sample.Success)
			assert.False(t, sample.FirstContentAt.IsZero())
		})
	}
}

func TestEnqueueStreamDataInvalidatesTPOTBeforeWaitingForBackpressure(t *testing.T) {
	const queueSize = 10
	dataChan := make(chan string, queueSize)
	for i := 0; i < queueSize; i++ {
		dataChan <- fmt.Sprintf("queued-%d", i)
	}

	invalidator := &attemptTPOTInvalidatorSpy{invalidated: make(chan struct{})}
	returned := make(chan bool, 1)
	go func() {
		returned <- EnqueueStreamDataWithBackpressure(context.Background(), dataChan, "blocked-eleventh-chunk", invalidator)
	}()

	select {
	case <-invalidator.invalidated:
	case <-time.After(time.Second):
		t.Fatal("TPOT was not invalidated when the downstream queue filled")
	}
	select {
	case <-returned:
		t.Fatal("enqueue returned before the blocked downstream queue made room")
	default:
	}

	<-dataChan
	select {
	case enqueued := <-returned:
		assert.True(t, enqueued)
	case <-time.After(time.Second):
		t.Fatal("enqueue did not resume after the downstream queue made room")
	}
	for i := 1; i < queueSize; i++ {
		assert.Equal(t, fmt.Sprintf("queued-%d", i), <-dataChan)
	}
	assert.Equal(t, "blocked-eleventh-chunk", <-dataChan)
}

func TestCloseUpstreamOnContextUnblocksSilentReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	producerDone := make(chan struct{})
	watcherDone := CloseUpstreamOnContext(ctx, reader, producerDone)
	readDone := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 1))
		readDone <- err
	}()

	cancel()
	select {
	case <-watcherDone:
	case <-time.After(time.Second):
		t.Fatal("context watcher did not close the silent upstream")
	}
	select {
	case err := <-readDone:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("silent upstream read remained blocked after cancellation")
	}
}

// ---------- Basic correctness ----------

func TestStreamScannerHandler_NilInputs(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	StreamScannerHandler(c, nil, info, func(data string, sr *StreamResult) {})
	StreamScannerHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(""))}, info, nil)
}

func TestNewStreamScanner_AllowsLargeStreamLine(t *testing.T) {
	oldBufferMB := constant.StreamScannerMaxBufferMB
	constant.StreamScannerMaxBufferMB = 1
	t.Cleanup(func() {
		constant.StreamScannerMaxBufferMB = oldBufferMB
	})

	payload := strings.Repeat("x", 128<<10)
	scanner := NewStreamScanner(strings.NewReader("data: " + payload + "\n"))
	scanner.Split(bufio.ScanLines)

	require.True(t, scanner.Scan())
	assert.Equal(t, "data: "+payload, scanner.Text())
	require.NoError(t, scanner.Err())
}

func TestStreamScannerHandler_EmptyBody(t *testing.T) {
	t.Parallel()

	c, resp, info := setupStreamTest(t, strings.NewReader(""))

	var called atomic.Bool
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		called.Store(true)
	})

	assert.False(t, called.Load(), "handler should not be called for empty body")
}

func TestStreamScannerHandler_1000Chunks(t *testing.T) {
	t.Parallel()

	const numChunks = 1000
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(numChunks), count.Load())
	assert.Equal(t, numChunks, info.ReceivedResponseCount)
}

func TestStreamScannerHandler_OrderPreserved(t *testing.T) {
	t.Parallel()

	const numChunks = 500
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var mu sync.Mutex
	received := make([]string, 0, numChunks)

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		mu.Lock()
		received = append(received, data)
		mu.Unlock()
	})

	require.Equal(t, numChunks, len(received))
	for i := 0; i < numChunks; i++ {
		expected := fmt.Sprintf("{\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}", i, i)
		assert.Equal(t, expected, received[i], "chunk %d out of order", i)
	}
}

func TestStreamScannerHandlerRecordsEveryVisibleChunkBeforeDelivery(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chunk-1","choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"id":"chunk-2","choices":[{"delta":{"content":"hel"}}]}`,
		`data: {"id":"chunk-3","choices":[{"delta":{"content":"lo"}}]}`,
		`data: [DONE]`,
	}, "\n")
	c, resp, info := setupStreamTest(t, strings.NewReader(body))
	info.IsStream = true
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	var sample relaycommon.DynamicRoutingAttemptSample
	var observed bool

	StreamScannerHandler(c, resp, info, func(data string, _ *StreamResult) {
		if strings.Contains(data, `"content":"lo"`) {
			info.SetAttemptCompletionTokens(3)
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			sample, observed = info.FinishDynamicRoutingAttempt(nil)
		}
	})

	require.True(t, observed)
	assert.False(t, sample.FirstContentAt.IsZero(), "visible content must be recorded before its downstream callback")
}

func TestStreamScannerHandler_DoneStopsScanner(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(50) + "data: should_not_appear\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(50), count.Load(), "data after [DONE] must not be processed")
}

func TestStreamScannerHandler_StopStopsStream(t *testing.T) {
	t.Parallel()

	const numChunks = 200
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	const stopAt int64 = 50
	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= stopAt {
			sr.Stop(fmt.Errorf("fatal at %d", n))
		}
	})

	assert.Equal(t, stopAt, count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
}

func TestStreamScannerHandler_SkipsNonDataLines(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString(": comment line\n")
	b.WriteString("event: message\n")
	b.WriteString("id: 12345\n")
	b.WriteString("retry: 5000\n")
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "data: payload_%d\n", i)
		b.WriteString(": interleaved comment\n")
	}
	b.WriteString("data: [DONE]\n")

	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(100), count.Load())
}

func TestStreamScannerHandler_DataWithExtraSpaces(t *testing.T) {
	t.Parallel()

	body := "data:   {\"trimmed\":true}  \ndata: [DONE]\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var got string
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		got = data
	})

	assert.Equal(t, "{\"trimmed\":true}", got)
}

// TestStreamScannerHandler_ClientCancelAbortsUpstreamAndReturns pins the
// disconnect contract: when the client goes away, the handler must return
// promptly (all goroutines joined, so the gin.Context can never leak into a
// pooled reuse), the upstream body must be closed to stop token generation,
// and no data received after the disconnect may be processed or written.
func TestStreamScannerHandler_ClientCancelAbortsUpstreamAndReturns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	firstHandled := make(chan struct{})
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
			_ = StringData(c, data)
			if data == "first" {
				close(firstHandled)
			}
		})
		close(done)
	}()

	_, err := fmt.Fprint(pw, "data: first\n")
	require.NoError(t, err)

	select {
	case <-firstHandled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first chunk")
	}

	cancel()

	// The handler must return without any further upstream input: cleanup
	// closes resp.Body, which unblocks the scanner goroutine.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client disconnect")
	}

	// Upstream read side must be closed so the provider stops generating
	// (and billing) for a request nobody is listening to.
	_, err = fmt.Fprint(pw, "data: second\n")
	require.ErrorIs(t, err, io.ErrClosedPipe, "upstream body should be closed after client disconnect")

	assert.Equal(t, int64(1), count.Load(), "no chunk after disconnect should be processed")
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)

	body := recorder.Body.String()
	assert.Contains(t, body, "first")
	assert.NotContains(t, body, "second")
}

func TestStreamScannerHandlerClientCancellationWinsScannerErrorRace(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestContext)
	resp := &http.Response{Body: &cancelingErrorReadCloser{cancel: cancel}}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	StreamScannerHandler(c, resp, info, func(string, *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	reason, endErr := info.StreamStatus.End()
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, reason)
	assert.ErrorIs(t, endErr, context.Canceled)
}

// ---------- Ping tests ----------

func TestStreamScannerHandler_PingSentDuringSlowUpstream(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < 4; i++ {
			fmt.Fprintf(pw, "data: chunk_%d\n", i)
			time.Sleep(400 * time.Millisecond)
		}
		fmt.Fprint(pw, "data: [DONE]\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream to finish")
	}

	assert.Equal(t, int64(4), count.Load())

	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	assert.GreaterOrEqual(t, pingCount, 1,
		"expected at least 1 ping during slow stream with 1s interval; got %d", pingCount)
}

func TestStreamScannerHandler_PingDisabledByRelayInfo(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(buildSSEBody(5)))}
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	assert.Equal(t, int64(5), count.Load())

	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	assert.Equal(t, 0, pingCount, "pings should be disabled when DisablePing=true")
}

// ---------- StreamStatus integration ----------

func TestStreamScannerHandler_StreamStatus_DoneReason(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Nil(t, info.StreamStatus.EndError)
	assert.True(t, info.StreamStatus.IsNormalEnd())
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandlerEmptyTerminalDoesNotPublishHealthSuccess(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		success bool
		hard    bool
	}{
		{name: "empty body", hard: true},
		{name: "DONE only", body: "data: [DONE]\n", hard: true},
		{
			name:    "metadata then DONE",
			body:    "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\ndata: [DONE]\n",
			success: true,
		},
		{
			name:    "tool call then DONE",
			body:    "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"lookup\"}}]}}]}\ndata: [DONE]\n",
			success: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, resp, info := setupStreamTest(t, strings.NewReader(tt.body))
			info.IsStream = true
			info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), "public-model", true)
			info.MarkAttemptUpstreamStarted()
			info.SetAttemptHTTPStatus(http.StatusOK)

			StreamScannerHandler(c, resp, info, func(string, *StreamResult) {})
			sample, observed := info.FinishDynamicRoutingAttempt(nil)

			require.True(t, observed)
			assert.Equal(t, tt.success, sample.Success)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.HasTTFT)
			assert.False(t, sample.HasTPOT)
		})
	}
}

func TestStreamScannerHandler_StreamStatus_EOFWithoutDone(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.IsNormalEnd())
}

func TestStreamScannerHandler_StreamStatus_HandlerStop(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(100)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 10 {
			sr.Stop(fmt.Errorf("stop at 10"))
		}
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_HandlerDone(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(20)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 5 {
			sr.Done()
		}
	})

	assert.Equal(t, int64(5), count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.False(t, info.StreamStatus.HasErrors())
}

func TestStreamScannerHandler_StreamStatus_Timeout(t *testing.T) {
	// Not parallel: modifies global constant.StreamingTimeout
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	pr, pw := io.Pipe()
	go func() {
		fmt.Fprint(pw, "data: {\"id\":1}\n")
		time.Sleep(2 * time.Second)
		pw.Close()
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stream timeout")
	}

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason)
	assert.False(t, info.StreamStatus.IsNormalEnd())
}

func TestStreamScannerHandler_ZeroStreamingTimeoutDisablesIdleWatchdog(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 0
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, resp, info := setupStreamTest(t, strings.NewReader("data: [DONE]\n"))

	StreamScannerHandler(c, resp, info, func(string, *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestStreamScannerHandler_StreamStatus_SoftErrors(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("soft error for chunk"))
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.True(t, info.StreamStatus.HasErrors())
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_MultipleErrorsPerChunk(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("error A"))
		sr.Error(fmt.Errorf("error B"))
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_ErrorThenStop(t *testing.T) {
	t.Parallel()

	// Use a large body without [DONE] to avoid race between scanner's [DONE]
	// and handler's Stop on the sync.Once EndReason.
	var b strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
		sr.Error(fmt.Errorf("soft error"))
		sr.Stop(fmt.Errorf("fatal"))
	})

	assert.Equal(t, int64(1), count.Load())
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason)
	assert.Equal(t, 2, info.StreamStatus.TotalErrorCount())
}

func TestStreamScannerHandler_StreamStatus_InitializedIfNil(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(1)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	assert.Nil(t, info.StreamStatus)

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.NotNil(t, info.StreamStatus)
}

func TestStreamScannerHandler_StreamStatus_ReplacesPreInitialized(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RecordError("pre-existing error")

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 0, info.StreamStatus.TotalErrorCount())
}
