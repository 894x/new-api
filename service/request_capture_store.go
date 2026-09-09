package service

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var requestCaptureStore *RequestCaptureStore
var captureIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type RequestCaptureStore struct {
	root      string
	id        string
	retention time.Duration
	maxBytes  int64
	slots     chan struct{}
	queue     chan *RequestCaptureSession
	stop      chan struct{}
	done      chan struct{}
	stopOnce  sync.Once
}

// StartRequestCaptureStorage uses a persistent volume. An installation marker
// prevents a different node's local files from being mistaken for this store.
func StartRequestCaptureStorage() {
	root := os.Getenv("REQUEST_CAPTURE_DIR")
	if root == "" {
		root = filepath.Join("data", "request-captures")
	}
	days, maxGB := 3, 10
	for key, target := range map[string]*int{"REQUEST_CAPTURE_RETENTION_DAYS": &days, "REQUEST_CAPTURE_MAX_GB": &maxGB} {
		if raw := os.Getenv(key); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 365 {
				common.SysError("invalid request capture storage limits; capture unavailable")
				return
			}
			*target = value
		}
	}
	store, err := newRequestCaptureStore(root, time.Duration(days)*24*time.Hour, int64(maxGB)<<30)
	if err != nil {
		common.SysError("request capture storage initialization failed; capture unavailable")
		return
	}
	if err := store.recoverInterrupted(); err != nil {
		common.SysError("request capture recovery failed; capture unavailable")
		return
	}
	requestCaptureStore = store
	go store.run()
}

// One running process owns a local capture volume. Before serving requests,
// reconcile interrupted writes and remove orphan artifacts left by a crash.
func (store *RequestCaptureStore) recoverInterrupted() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	records, err := model.ListStoredRequestCaptures(ctx, store.id)
	if err != nil {
		return err
	}
	keep := make(map[string]bool, len(records))
	for i := range records {
		record := &records[i]
		path, err := store.path(record)
		if err != nil {
			return err
		}
		if record.Status == "ready" || record.Status == "partial" {
			keep[path] = true
			continue
		}
		if record.Status == "recording" {
			record.Status, record.Reason, record.StoredBytes = "failed", "process_interrupted", 0
			if err := model.SaveRequestCapture(ctx, record); err != nil {
				return err
			}
		}
	}
	return filepath.WalkDir(store.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".tmp"), ".jsonl.gz")
		if !captureIDPattern.MatchString(name) || keep[path] {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl.gz") && !strings.HasSuffix(entry.Name(), ".jsonl.gz.tmp") {
			return nil
		}
		return os.Remove(path)
	})
}

func newRequestCaptureStore(root string, retention time.Duration, maxBytes int64) (*RequestCaptureStore, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	marker := filepath.Join(abs, ".store-id")
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, writeErr := f.WriteString(strings.ReplaceAll(common.GetUUID(), "-", ""))
		closeErr := f.Close()
		if writeErr != nil {
			return nil, writeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	id, err := os.ReadFile(marker)
	if err != nil || !captureIDPattern.Match(id) {
		return nil, errors.New("invalid capture store identity")
	}
	return &RequestCaptureStore{root: abs, id: string(id), retention: retention, maxBytes: maxBytes,
		slots: make(chan struct{}, 8), queue: make(chan *RequestCaptureSession, 8), stop: make(chan struct{}), done: make(chan struct{})}, nil
}

func (store *RequestCaptureStore) run() {
	defer close(store.done)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	if err := store.cleanup(0); err != nil {
		common.SysError("request capture cleanup failed")
	}
	for {
		select {
		case <-store.stop:
			for len(store.queue) > 0 {
				store.persist(<-store.queue)
				<-store.slots
			}
			return
		case session := <-store.queue:
			store.persist(session)
			<-store.slots
		case <-ticker.C:
			if err := store.cleanup(0); err != nil {
				common.SysError("request capture cleanup failed")
			}
		}
	}
}

// StopRequestCaptureStorage is called after HTTP handlers have drained.
func StopRequestCaptureStorage(ctx context.Context) {
	store := requestCaptureStore
	if store == nil {
		return
	}
	store.stopOnce.Do(func() { close(store.stop) })
	select {
	case <-store.done:
	case <-ctx.Done():
		common.SysError("request capture shutdown flush timed out")
	}
}

func (store *RequestCaptureStore) path(record *model.RequestCapture) (string, error) {
	if !captureIDPattern.MatchString(record.ID) || record.StoreID != store.id {
		return "", errors.New("capture belongs to another storage volume")
	}
	date := time.Unix(record.CreatedAt, 0).UTC().Format("2006/01/02/15")
	return filepath.Join(store.root, filepath.FromSlash(date), record.ID+".jsonl.gz"), nil
}

func (store *RequestCaptureStore) persist(session *RequestCaptureSession) {
	record := &session.Record
	record.Status = "ready"
	if record.Reason != "" {
		record.Status = "partial"
	}
	for _, p := range session.Parts {
		record.RawBytes += p.Bytes
		p.Body, p.Redacted, p.Omitted = sanitizeCaptureBody(p.head, p.ContentType, p.Truncated)
		if p.Truncated && len(p.tail) > 0 {
			var redacted, omitted bool
			p.Tail, redacted, omitted = sanitizeCaptureBody(p.tail, p.ContentType, true)
			p.Redacted = p.Redacted || redacted
			p.Omitted = p.Omitted || omitted
		}
		p.head, p.tail = nil, nil
		if !p.Complete || p.Truncated || p.Omitted {
			record.Status = "partial"
		}
	}
	// SDK/custom transports bypassing the shared HTTP boundary are explicit.
	if session.attempts == 0 {
		record.Status, record.Reason = "partial", "upstream_not_observed"
	}
	path, err := store.path(record)
	if err == nil {
		err = store.write(path, record, session.Parts)
	}
	if err != nil {
		record.Status, record.Reason, record.StoredBytes = "failed", "storage_write_failed", 0
		common.SysError("request capture write failed: request_id=" + record.RequestID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := model.SaveRequestCapture(ctx, record); err != nil {
		// Avoid an unindexed raw-body file when the final index write fails.
		if path != "" {
			_ = os.Remove(path)
		}
		common.SysError("request capture final index write failed: request_id=" + record.RequestID)
	}
}

func (store *RequestCaptureStore) write(path string, record *model.RequestCapture, parts []*CapturePart) (err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(path + ".tmp") }()
	z := gzip.NewWriter(f)
	values := []any{map[string]any{"format": "request-capture-v1", "request_id": record.RequestID, "created_at": record.CreatedAt}}
	for _, p := range parts {
		values = append(values, p)
	}
	for _, value := range values {
		line, marshalErr := common.Marshal(value)
		if marshalErr != nil {
			return marshalErr
		}
		if _, err = z.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	if err = z.Close(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	total, sizeErr := model.RequestCaptureStoredBytes(ctx, store.id)
	cancel()
	if sizeErr != nil {
		return sizeErr
	}
	if total+stat.Size() > store.maxBytes {
		err = store.cleanup(stat.Size())
	}
	if err != nil {
		return err
	}
	if err = os.Rename(path+".tmp", path); err != nil {
		return err
	}
	record.StoredBytes = stat.Size()
	return nil
}

// cleanup evicts oldest files before admitting new bytes and retains small
// expired indexes for seven additional days so the UI can explain their absence.
func (store *RequestCaptureStore) cleanup(incoming int64) error {
	if incoming > store.maxBytes {
		return errors.New("capture exceeds storage limit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	records, err := model.ListStoredRequestCaptures(ctx, store.id)
	if err != nil {
		return err
	}
	var total int64
	for _, record := range records {
		total += record.StoredBytes
	}
	now := time.Now().Unix()
	for i := range records {
		record := &records[i]
		expired := record.ExpiresAt <= now
		if !expired && (total+incoming <= store.maxBytes || record.StoredBytes == 0) {
			continue
		}
		path, err := store.path(record)
		if err != nil {
			return err
		}
		if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		total -= record.StoredBytes
		record.StoredBytes = 0
		record.Status, record.Reason = "expired", "retention_expired"
		if !expired {
			record.Reason = "storage_limit"
		}
		if record.ExpiresAt < now-int64(7*24*time.Hour/time.Second) {
			if err := model.DeleteRequestCapture(ctx, record.ID); err != nil {
				return err
			}
		} else if err := model.SaveRequestCapture(ctx, record); err != nil {
			return err
		}
	}
	if total+incoming > store.maxBytes {
		return errors.New("capture storage full")
	}
	return nil
}

func RequestCaptureStorageAvailable() bool { return requestCaptureStore != nil }

func ReadRequestCapture(record *model.RequestCapture) ([]CapturePart, error) {
	store := requestCaptureStore
	if store == nil {
		return nil, errors.New("capture storage unavailable")
	}
	path, err := store.path(record)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	// The writer's maximum retained body size is bounded. Also bound reads of
	// damaged/replaced files, including decompression and JSON string escaping.
	scanner := bufio.NewScanner(io.LimitReader(z, 40<<20))
	scanner.Buffer(make([]byte, 4096), 32<<20)
	if !scanner.Scan() {
		return nil, errors.New("capture header missing")
	}
	var header struct {
		Format    string `json:"format"`
		RequestID string `json:"request_id"`
	}
	if common.Unmarshal(scanner.Bytes(), &header) != nil || header.Format != "request-capture-v1" || header.RequestID != record.RequestID {
		return nil, errors.New("invalid capture header")
	}
	parts := make([]CapturePart, 0, 4)
	for scanner.Scan() {
		if len(parts) >= 2+2*captureAttemptLimit {
			return nil, errors.New("too many capture parts")
		}
		var part CapturePart
		if err := common.Unmarshal(scanner.Bytes(), &part); err != nil {
			return nil, fmt.Errorf("invalid capture part")
		}
		parts = append(parts, part)
	}
	return parts, scanner.Err()
}
