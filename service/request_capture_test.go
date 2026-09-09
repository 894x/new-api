package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func captureFixture(t *testing.T) (*RequestCaptureStore, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.RequestCapturePolicy{}, &model.RequestCapture{}, &model.Option{}))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	previousDB, previousStore := model.DB, requestCaptureStore
	model.DB = db
	store, err := newRequestCaptureStore(t.TempDir())
	require.NoError(t, err)
	requestCaptureStore = store
	t.Cleanup(func() { model.DB, requestCaptureStore = previousDB, previousStore; _ = sqlDB.Close() })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", 7)
	c.Set(common.RequestIdKey, "capture-test-request")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"client-model","messages":[]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	return store, c, w
}

func TestRequestCaptureDisabledByDefaultAndPerUser(t *testing.T) {
	store, c, _ := captureFixture(t)
	original := c.Request.Body
	assert.Nil(t, BeginRequestCapture(c))
	assert.Equal(t, original, c.Request.Body)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 8, Enabled: true}))
	assert.Nil(t, BeginRequestCapture(c))
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	s := BeginRequestCapture(c)
	require.NotNil(t, s)
	s.Finish()
	assert.Same(t, s, <-store.queue)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: false}))
	assert.Nil(t, BeginRequestCapture(c))
}

func TestRequestCapturePersistsFourBoundariesRetriesAndSSEWithoutChangingBytes(t *testing.T) {
	store, c, w := captureFixture(t)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	s := BeginRequestCapture(c)
	require.NotNil(t, s)
	s.CaptureClientRequest(c)
	in, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"client-model","messages":[]}`, string(in))
	finish := s.CaptureClientResponse(c)
	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "http://upstream/v1/chat/completions", strings.NewReader(`{"model":"mapped-model","api_key":"private"}`))
		req.Header.Set("Content-Type", "application/json")
		capture := CaptureUpstreamExchange(c, req, 10+attempt, "mapped-model")
		out, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.Contains(t, string(out), "private", "capture must not alter the outbound request")
		data := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"prompt_tokens_details\":{\"cached_tokens\":80}}}\n\ndata: [DONE]\n\n"
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}, "X-Request-Id": {"upstream-id"}}, Body: io.NopCloser(strings.NewReader(data)), ContentLength: -1}
		capture(resp, nil)
		actual, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, data, string(actual))
		if attempt == 1 {
			c.Writer.Header().Set("Content-Type", "text/event-stream")
			_, err = c.Writer.Write(actual[:15])
			require.NoError(t, err)
			c.Writer.Flush()
			assert.True(t, w.Flushed)
			assert.Equal(t, data[:15], w.Body.String())
			_, err = c.Writer.WriteString(string(actual[15:]))
			require.NoError(t, err)
			assert.Equal(t, data, w.Body.String())
		}
	}
	finish(true)
	s.Finish()
	store.persist(<-store.queue)
	record, err := model.FindRequestCapture(context.Background(), "capture-test-request")
	require.NoError(t, err)
	assert.Equal(t, "ready", record.Status)
	assert.Positive(t, record.StoredBytes)
	parts, err := ReadRequestCapture(record)
	require.NoError(t, err)
	require.Len(t, parts, 6)
	assert.Equal(t, "client_request", parts[0].Stage)
	assert.Equal(t, "client_response", parts[1].Stage)
	assert.Equal(t, 1, parts[2].Attempt)
	assert.Equal(t, 2, parts[4].Attempt)
	assert.NotContains(t, parts[2].Body, "private")
	assert.True(t, parts[2].Redacted)
	assert.Contains(t, parts[5].Body, `"cached_tokens":80`)
	assert.Equal(t, "upstream-id", parts[5].UpstreamRequestID)
}

func TestRequestCaptureTruncationRetainsUsageTailAndMarksIncomplete(t *testing.T) {
	store, c, _ := captureFixture(t)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	s := BeginRequestCapture(c)
	s.remaining = 20
	p := s.NewPart("upstream_response")
	p.ContentType = "text/event-stream"
	s.append(p, []byte("data: {\"delta\":\""+strings.Repeat("x", 100)+"\"}\n\ndata: {\"usage\":{\"cached_tokens\":80}}\n\n"))
	s.Finish()
	store.persist(<-store.queue)
	record, err := model.FindRequestCapture(context.Background(), "capture-test-request")
	require.NoError(t, err)
	assert.Equal(t, "partial", record.Status)
	parts, err := ReadRequestCapture(record)
	require.NoError(t, err)
	assert.True(t, parts[0].Truncated)
	assert.False(t, parts[0].Complete)
	assert.Contains(t, parts[0].Tail, `"cached_tokens":80`)
}

func TestRequestCaptureCapacityFailureIsIndexedWithoutWrappingBody(t *testing.T) {
	store, c, _ := captureFixture(t)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	for i := 0; i < cap(store.slots); i++ {
		store.slots <- struct{}{}
	}
	original := c.Request.Body
	assert.Nil(t, BeginRequestCapture(c))
	assert.Equal(t, original, c.Request.Body)
	record, err := model.FindRequestCapture(context.Background(), "capture-test-request")
	require.NoError(t, err)
	assert.Equal(t, "capacity_exceeded", record.Reason)
}

func TestRequestCaptureStorageFailureAndRetention(t *testing.T) {
	store, c, _ := captureFixture(t)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	s := BeginRequestCapture(c)
	s.Finish()
	store.persist(<-store.queue)
	record, err := model.FindRequestCapture(context.Background(), "capture-test-request")
	require.NoError(t, err)
	path, err := store.path(record)
	require.NoError(t, err)
	require.FileExists(t, path)
	record.ExpiresAt = time.Now().Add(-time.Hour).Unix()
	require.NoError(t, model.SaveRequestCapture(context.Background(), record))
	require.NoError(t, store.cleanup(0))
	assert.NoFileExists(t, path)
	record, err = model.FindRequestCapture(context.Background(), "capture-test-request")
	require.NoError(t, err)
	assert.Equal(t, "expired", record.Status)
	assert.Zero(t, record.StoredBytes)
	// A file where a directory is required simulates a non-writable volume.
	blocked := store.root + "/blocked"
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0600))
	store.root = blocked
	c.Set(common.RequestIdKey, "capture-write-failure")
	s = BeginRequestCapture(c)
	s.Finish()
	store.persist(<-store.queue)
	record, err = model.FindRequestCapture(context.Background(), "capture-write-failure")
	require.NoError(t, err)
	assert.Equal(t, "failed", record.Status)
	assert.Equal(t, "storage_write_failed", record.Reason)
	malicious := *record
	malicious.ID = "../../outside"
	_, err = store.path(&malicious)
	assert.Error(t, err)
}

func TestRequestCaptureRedactionAndInvalidBodies(t *testing.T) {
	for _, test := range []struct {
		name, body, contentType string
		omitted                 bool
	}{
		{"json", `{"messages":[{"content":"keep prompt"}],"apiKey":"secret-value"}`, "application/json", false},
		{"sse", "data: {\"access_token\":\"secret-value\"}\n\n", "text/event-stream", false},
		{"truncated", `{"apiKey":"secret-value`, "application/json", true},
		{"binary", "secret-value", "application/octet-stream", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, _, omitted := sanitizeCaptureBody([]byte(test.body), test.contentType, false)
			assert.NotContains(t, body, "secret-value")
			assert.Equal(t, test.omitted, omitted)
		})
	}
}

func TestRequestCaptureRecoveryCleansInterruptedFilesAndMarksFailure(t *testing.T) {
	store, c, _ := captureFixture(t)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	s := BeginRequestCapture(c)
	path, err := store.path(&s.Record)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path+".tmp", []byte("private unfinished data"), 0600))
	require.NoError(t, os.WriteFile(path, []byte("uncommitted final file"), 0600))
	require.NoError(t, store.recoverInterrupted())
	assert.NoFileExists(t, path)
	assert.NoFileExists(t, path+".tmp")
	record, err := model.FindRequestCapture(context.Background(), s.Record.RequestID)
	require.NoError(t, err)
	assert.Equal(t, "process_interrupted", record.Reason)
	assert.Equal(t, "failed", record.Status)
}

func TestRequestCaptureCapacityEvictsFileBeforeNewArchive(t *testing.T) {
	store, c, _ := captureFixture(t)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	s := BeginRequestCapture(c)
	s.Finish()
	store.persist(<-store.queue)
	record, err := model.FindRequestCapture(context.Background(), s.Record.RequestID)
	require.NoError(t, err)
	path, err := store.path(record)
	require.NoError(t, err)
	record.StoredBytes = 1 << 30
	require.NoError(t, model.SaveRequestCapture(context.Background(), record))
	require.NoError(t, model.UpdateOption(model.RequestCaptureStorageOptionKey, `{"retention_days":3,"max_gib":1}`))
	require.NoError(t, store.cleanup(1))
	assert.NoFileExists(t, path)
	record, err = model.FindRequestCapture(context.Background(), s.Record.RequestID)
	require.NoError(t, err)
	assert.Equal(t, "storage_limit", record.Reason)
}

func TestRequestCaptureSettingsApplyWithoutRestartAndKeepExistingExpiry(t *testing.T) {
	store, c, _ := captureFixture(t)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	first := BeginRequestCapture(c)
	require.NotNil(t, first)
	assert.Equal(t, int64(3*24*60*60), first.Record.ExpiresAt-first.Record.CreatedAt)
	first.Finish()
	store.persist(<-store.queue)
	require.NoError(t, model.UpdateOption(model.RequestCaptureStorageOptionKey, `{"retention_days":7,"max_gib":2}`))
	c.Set(common.RequestIdKey, "capture-after-settings-change")
	second := BeginRequestCapture(c)
	require.NotNil(t, second)
	assert.Equal(t, int64(7*24*60*60), second.Record.ExpiresAt-second.Record.CreatedAt)
	previous, err := model.FindRequestCapture(context.Background(), first.Record.RequestID)
	require.NoError(t, err)
	assert.Equal(t, first.Record.ExpiresAt, previous.ExpiresAt)
	assert.Equal(t, 2, model.GetRequestCaptureStorageSettings().MaxGiB)
}

func TestRequestCaptureHTTPErrorThenStreamingRetryRetainsActualExchanges(t *testing.T) {
	store, c, _ := captureFixture(t)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 7, Enabled: true}))
	s := BeginRequestCapture(c)
	s.CaptureClientRequest(c)
	_, err := io.ReadAll(c.Request.Body)
	require.NoError(t, err)
	finishClient := s.CaptureClientResponse(c)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !bytes.Equal(body, []byte(`{"model":"mapped"}`)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/error" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"retry this channel"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("data: {\"usage\":{\"cached_tokens\":30}}\n\ndata: [DONE]\n\n"))
	}))
	defer upstream.Close()
	for _, path := range []string{"/error", "/success"} {
		req, err := http.NewRequest(http.MethodPost, upstream.URL+path, strings.NewReader(`{"model":"mapped"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		finish := CaptureUpstreamExchange(c, req, 12, "mapped")
		resp, err := upstream.Client().Do(req)
		finish(resp, err)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		if path == "/error" {
			assert.Equal(t, 429, resp.StatusCode)
		} else {
			assert.Equal(t, 200, resp.StatusCode)
			c.Writer.Header().Set("Content-Type", "text/event-stream")
			_, err = c.Writer.Write(body)
			require.NoError(t, err)
		}
	}
	finishClient(true)
	s.Finish()
	store.persist(<-store.queue)
	record, err := model.FindRequestCapture(context.Background(), s.Record.RequestID)
	require.NoError(t, err)
	parts, err := ReadRequestCapture(record)
	require.NoError(t, err)
	require.Len(t, parts, 6)
	assert.Equal(t, 429, parts[3].StatusCode)
	assert.Contains(t, parts[3].Body, "retry this channel")
	assert.Equal(t, 2, parts[5].Attempt)
	assert.Contains(t, parts[5].Body, `"cached_tokens":30`)
	assert.Equal(t, "ready", record.Status)
}
