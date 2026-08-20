package protocol

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var ErrIncompleteStream = errors.New("SSE stream ended before [DONE]")

type StreamScript struct {
	StatusCode             int
	ResponseDelay          time.Duration
	FirstEventDelay        time.Duration
	MetadataOnlyFirstEvent bool
	FirstContentAfterEvent time.Duration
	PerTokenDelay          time.Duration
	Tokens                 []string
	CloseBeforeDone        bool
}

type HandlerHooks struct {
	Wait       func(time.Duration)
	AfterFlush func()
}

type MeasureOptions struct {
	Now        func() time.Time
	AfterEvent func()
}

type StreamMetrics struct {
	StatusCode    int
	TTFTAnyEvent  time.Duration
	TTFTContent   time.Duration
	TPOT          time.Duration
	ContentEvents int
	Completed     bool
}

type chunk struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Delta delta `json:"delta"`
}

type delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

func NewHandler(script StreamScript, hooks HandlerHooks) http.Handler {
	wait := hooks.Wait
	if wait == nil {
		wait = time.Sleep
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		wait(script.ResponseDelay)
		statusCode := script.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		if statusCode != http.StatusOK {
			writer.WriteHeader(statusCode)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		writeChunk := func(value any) bool {
			payload, err := common.Marshal(value)
			if err != nil {
				return false
			}
			if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
				return false
			}
			flusher.Flush()
			if hooks.AfterFlush != nil {
				hooks.AfterFlush()
			}
			return true
		}

		wait(script.FirstEventDelay)
		if script.MetadataOnlyFirstEvent {
			if !writeChunk(chunk{Choices: []choice{{Delta: delta{Role: "assistant"}}}}) {
				return
			}
			wait(script.FirstContentAfterEvent)
		}
		for index, token := range script.Tokens {
			if index > 0 {
				wait(script.PerTokenDelay)
			}
			if !writeChunk(chunk{Choices: []choice{{Delta: delta{Content: token}}}}) {
				return
			}
		}
		if script.CloseBeforeDone {
			return
		}
		if _, err := fmt.Fprint(writer, "data: [DONE]\n\n"); err != nil {
			return
		}
		flusher.Flush()
		if hooks.AfterFlush != nil {
			hooks.AfterFlush()
		}
	})
}

func Measure(ctx context.Context, client *http.Client, endpoint string, options MeasureOptions) (StreamMetrics, error) {
	if client == nil {
		return StreamMetrics{}, errors.New("HTTP client is required")
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(`{"stream":true}`))
	if err != nil {
		return StreamMetrics{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	startedAt := now()
	response, err := client.Do(request)
	if err != nil {
		return StreamMetrics{}, err
	}
	defer response.Body.Close()

	metrics := StreamMetrics{StatusCode: response.StatusCode}
	if response.StatusCode != http.StatusOK {
		metrics.TTFTAnyEvent = now().Sub(startedAt)
		return metrics, nil
	}

	var firstEventAt time.Time
	var firstContentAt time.Time
	var lastContentAt time.Time
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		observedAt := now()
		if firstEventAt.IsZero() {
			firstEventAt = observedAt
			metrics.TTFTAnyEvent = observedAt.Sub(startedAt)
		}
		if data == "[DONE]" {
			metrics.Completed = true
			if options.AfterEvent != nil {
				options.AfterEvent()
			}
			break
		}
		var event chunk
		if err := common.Unmarshal([]byte(data), &event); err != nil {
			return metrics, fmt.Errorf("decode SSE event: %w", err)
		}
		visible := false
		for _, candidate := range event.Choices {
			if candidate.Delta.Content != "" {
				visible = true
				break
			}
		}
		if visible {
			metrics.ContentEvents++
			if firstContentAt.IsZero() {
				firstContentAt = observedAt
				metrics.TTFTContent = observedAt.Sub(startedAt)
			}
			lastContentAt = observedAt
		}
		if options.AfterEvent != nil {
			options.AfterEvent()
		}
	}
	if err := scanner.Err(); err != nil {
		return metrics, err
	}
	if metrics.ContentEvents > 1 {
		metrics.TPOT = lastContentAt.Sub(firstContentAt) / time.Duration(metrics.ContentEvents-1)
	}
	if !metrics.Completed {
		return metrics, ErrIncompleteStream
	}
	return metrics, nil
}
