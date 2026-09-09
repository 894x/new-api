package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const requestCaptureKey = "request_capture_session"
const captureBudget = 4 << 20
const captureTailLimit = 64 << 10
const captureAttemptLimit = 8

// CapturePart retains exact application-body bytes unless credential redaction
// is necessary. Tail is separate evidence after truncation, never concatenated
// to Body as if the intervening bytes had been captured.
type CapturePart struct {
	Stage             string `json:"stage"`
	Attempt           int    `json:"attempt,omitempty"`
	ChannelID         int    `json:"channel_id,omitempty"`
	Model             string `json:"model,omitempty"`
	ContentType       string `json:"content_type,omitempty"`
	StatusCode        int    `json:"status_code,omitempty"`
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`
	Bytes             int64  `json:"bytes"`
	Complete          bool   `json:"complete"`
	Truncated         bool   `json:"truncated"`
	Redacted          bool   `json:"redacted"`
	Omitted           bool   `json:"omitted"`
	Body              string `json:"body"`
	Tail              string `json:"tail,omitempty"`
	head              []byte
	tail              []byte
}

type RequestCaptureSession struct {
	mu        sync.Mutex
	store     *RequestCaptureStore
	Record    model.RequestCapture
	Parts     []*CapturePart
	remaining int
	finished  bool
	attempts  int
}

// BeginRequestCapture performs no body work for users without an enabled policy.
// Policy reads use the primary DB so disabling does not wait for a cache TTL.
func BeginRequestCapture(c *gin.Context) *RequestCaptureSession {
	store := requestCaptureStore
	if store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	policy, err := model.GetRequestCapturePolicy(ctx, c.GetInt("id"))
	if err != nil {
		common.SysError("request capture policy lookup failed")
		return nil
	}
	if !policy.Enabled {
		return nil
	}
	id := common.GetUUID()
	id = strings.ReplaceAll(id, "-", "")
	now := time.Now()
	retention := time.Duration(model.GetRequestCaptureStorageSettings().RetentionDays) * 24 * time.Hour
	s := &RequestCaptureSession{store: store, remaining: captureBudget, Record: model.RequestCapture{
		ID: id, RequestID: c.GetString(common.RequestIdKey), UserID: policy.UserID,
		StoreID: store.id, CreatedAt: now.Unix(), ExpiresAt: now.Add(retention).Unix(), Status: "recording",
	}}
	select {
	case store.slots <- struct{}{}:
	default:
		s.Record.Status, s.Record.Reason = "failed", "capacity_exceeded"
		if model.SaveRequestCapture(ctx, &s.Record) != nil {
			common.SysError("request capture failure index write failed")
		}
		return nil
	}
	if err := model.SaveRequestCapture(ctx, &s.Record); err != nil {
		<-store.slots
		common.SysError("request capture index write failed")
		return nil
	}
	c.Set(requestCaptureKey, s)
	return s
}

func (s *RequestCaptureSession) NewPart(stage string) *CapturePart {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := &CapturePart{Stage: stage}
	s.Parts = append(s.Parts, p)
	return p
}

func (s *RequestCaptureSession) append(p *CapturePart, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	p.Bytes += int64(len(data))
	n := min(len(data), s.remaining)
	p.head = append(p.head, data[:n]...)
	s.remaining -= n
	if n != len(data) {
		p.Truncated = true
	}
	// Only response tails are retained: usage often arrives in the final SSE frame.
	if strings.HasSuffix(p.Stage, "response") {
		if len(data) >= captureTailLimit {
			p.tail = append(p.tail[:0], data[len(data)-captureTailLimit:]...)
		} else {
			if extra := len(p.tail) + len(data) - captureTailLimit; extra > 0 {
				p.tail = p.tail[extra:]
			}
			p.tail = append(p.tail, data...)
		}
	}
}

type captureReadCloser struct {
	io.ReadCloser
	session  *RequestCaptureSession
	part     *CapturePart
	expected int64
}

func (r *captureReadCloser) Read(b []byte) (int, error) {
	n, err := r.ReadCloser.Read(b)
	r.session.append(r.part, b[:n])
	r.session.mu.Lock()
	if !r.session.finished && (err == io.EOF || (r.expected >= 0 && r.part.Bytes == r.expected)) {
		r.part.Complete = true
	}
	r.session.mu.Unlock()
	return n, err
}

func (s *RequestCaptureSession) CaptureClientRequest(c *gin.Context) {
	p := s.NewPart("client_request")
	p.ContentType = c.ContentType()
	if c.Request.Body != nil {
		c.Request.Body = &captureReadCloser{ReadCloser: c.Request.Body, session: s, part: p, expected: c.Request.ContentLength}
	} else {
		p.Complete = true
	}
}

// CaptureUpstreamExchange observes the shared HTTP boundary after all adaptor
// conversions. It does not replace GetBody or change transport retry behavior.
func CaptureUpstreamExchange(c *gin.Context, req *http.Request, channelID int, modelName string) func(*http.Response, error) {
	v, ok := c.Get(requestCaptureKey)
	if !ok {
		return func(*http.Response, error) {}
	}
	s := v.(*RequestCaptureSession)
	s.mu.Lock()
	if s.attempts >= captureAttemptLimit {
		s.Record.Reason = "attempt_limit"
		s.mu.Unlock()
		return func(*http.Response, error) {}
	}
	s.attempts++
	attempt := s.attempts
	s.mu.Unlock()
	out := s.NewPart("upstream_request")
	out.Attempt, out.ChannelID, out.Model = attempt, channelID, modelName
	out.ContentType = req.Header.Get("Content-Type")
	if req.Body != nil {
		req.Body = &captureReadCloser{ReadCloser: req.Body, session: s, part: out, expected: req.ContentLength}
	} else {
		out.Complete = true
	}
	return func(resp *http.Response, err error) {
		in := s.NewPart("upstream_response")
		in.Attempt, in.ChannelID, in.Model = attempt, channelID, modelName
		if err != nil || resp == nil {
			return
		}
		in.StatusCode, in.ContentType = resp.StatusCode, resp.Header.Get("Content-Type")
		for _, key := range []string{"x-request-id", "request-id", "x-amzn-requestid"} {
			if value := resp.Header.Get(key); value != "" {
				in.UpstreamRequestID = common.LimitUpstreamRequestIdentifier(value)
				break
			}
		}
		if resp.Body != nil {
			resp.Body = &captureReadCloser{ReadCloser: resp.Body, session: s, part: in, expected: resp.ContentLength}
		} else {
			in.Complete = true
		}
	}
}

type captureResponseWriter struct {
	gin.ResponseWriter
	session     *RequestCaptureSession
	part        *CapturePart
	writeFailed bool
}

func (w *captureResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.session.append(w.part, b[:n])
	w.session.mu.Lock()
	if err != nil {
		w.writeFailed = true
	}
	w.session.mu.Unlock()
	return n, err
}

func (w *captureResponseWriter) WriteString(value string) (int, error) { return w.Write([]byte(value)) }

func (s *RequestCaptureSession) CaptureClientResponse(c *gin.Context) func(bool) {
	p := s.NewPart("client_response")
	w := &captureResponseWriter{ResponseWriter: c.Writer, session: s, part: p}
	c.Writer = w
	return func(completed bool) {
		s.mu.Lock()
		p.ContentType = w.Header().Get("Content-Type")
		p.StatusCode = w.Status()
		p.Complete = completed && !w.writeFailed && c.Request.Context().Err() == nil
		s.Record.StatusCode = p.StatusCode
		s.mu.Unlock()
	}
}

func (s *RequestCaptureSession) Finish() {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.mu.Unlock()
	// A slot is held through persistence, so this bounded queue cannot fill.
	s.store.queue <- s
}

func sanitizeCaptureJSON(data []byte) ([]byte, bool, bool) {
	var value any
	if common.Unmarshal(data, &value) != nil {
		return nil, false, false
	}
	changed := redactCaptureValue(value)
	if !changed {
		return data, false, true
	}
	encoded, err := common.Marshal(value)
	return encoded, true, err == nil
}

func redactCaptureValue(value any) bool {
	changed := false
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			switch normalized {
			case "authorization", "apikey", "xapikey", "accesskey", "accesskeyid", "secretaccesskey", "token", "accesstoken", "refreshtoken", "password", "secret", "clientsecret", "cookie", "setcookie":
				v[key], changed = "[REDACTED]", true
			default:
				if text, ok := child.(string); ok && (strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "http://")) {
					if parsed, err := url.Parse(text); err == nil && (parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "") {
						parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
						v[key], changed = parsed.String(), true
					}
				}
				changed = redactCaptureValue(child) || changed
			}
		}
	case []any:
		for _, child := range v {
			changed = redactCaptureValue(child) || changed
		}
	}
	return changed
}

func sanitizeCaptureBody(data []byte, contentType string, partial bool) (string, bool, bool) {
	if len(data) == 0 {
		return "", false, false
	}
	if clean, redacted, ok := sanitizeCaptureJSON(data); ok {
		return string(clean), redacted, false
	}
	if !strings.Contains(contentType, "text/event-stream") {
		return "", false, true
	}
	var out bytes.Buffer
	redacted, omitted := false, false
	lines := bytes.SplitAfter(data, []byte("\n"))
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("data:")) {
			payload := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
			if bytes.Equal(payload, []byte("[DONE]")) {
				out.Write(line)
				continue
			}
			clean, changed, ok := sanitizeCaptureJSON(payload)
			if !ok {
				omitted = true
				continue
			}
			if changed {
				out.WriteString("data: ")
				out.Write(clean)
				out.WriteByte('\n')
				redacted = true
			} else {
				out.Write(line)
			}
		} else if len(trimmed) == 0 || bytes.HasPrefix(trimmed, []byte("event:")) {
			out.Write(line)
		} else {
			// Comments, partial frames and unsupported SSE fields may contain
			// credentials or unparseable content; do not persist them silently.
			omitted = true
		}
	}
	return out.String(), redacted, omitted || partial
}
