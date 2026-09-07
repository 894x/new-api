package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// Asset timelines contain allow-listed identifiers and measurements, never URLs,
// credentials, provider bodies or arbitrary error messages. They are admin-only.
type AssetLibraryTiming struct {
	Version       int                        `json:"version"`
	RequestID     string                     `json:"request_id"`
	Action        string                     `json:"action"`
	AssetID       string                     `json:"asset_id,omitempty"`
	StartedAtMS   int64                      `json:"started_at_ms"`
	CompletedAtMS int64                      `json:"completed_at_ms"`
	DurationMS    int64                      `json:"duration_ms"`
	Outcome       string                     `json:"outcome"`
	Stages        []*AssetLibraryTimingStage `json:"stages"`
	Replicas      []AssetLibraryReadiness    `json:"replicas,omitempty"`
	DroppedStages int                        `json:"dropped_stages,omitempty"`
}

type AssetLibraryTimingStage struct {
	ID                int    `json:"id"`
	HeldMS            int64  `json:"held_ms,omitempty"`
	Backend           string `json:"backend,omitempty"`
	Stage             string `json:"stage"`
	Operation         string `json:"operation,omitempty"`
	AssetID           string `json:"asset_id,omitempty"`
	ChannelID         int    `json:"channel_id,omitempty"`
	StartedAtMS       int64  `json:"started_at_ms"`
	DurationMS        int64  `json:"duration_ms"`
	Outcome           string `json:"outcome"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`
	ErrorCode         string `json:"error_code,omitempty"`
	ErrorKind         string `json:"error_kind,omitempty"`
	Bytes             int64  `json:"bytes,omitempty"`
}

type AssetLibraryReadiness struct {
	AssetID            string `json:"asset_id"`
	ChannelID          int    `json:"channel_id"`
	Status             string `json:"status"`
	UploadStartedAtMS  int64  `json:"upload_started_at_ms,omitempty"`
	SubmittedAtMS      int64  `json:"submitted_at_ms,omitempty"`
	FirstActiveAtMS    int64  `json:"first_active_at_ms,omitempty"`
	LastPolledAtMS     int64  `json:"last_polled_at_ms,omitempty"`
	LastProcessingAtMS int64  `json:"last_processing_at_ms,omitempty"`
	PollCount          int64  `json:"poll_count"`
}

type assetLibraryTraceKey struct{}
type assetLibraryResourceKey struct{}
type assetLibraryUploadStartKey struct{}
type assetLibraryTrace struct {
	failed                   bool
	mu                       sync.Mutex
	timing                   AssetLibraryTiming
	started                  time.Time
	ip, content, auditAction string
	params                   map[string]interface{}
}

// BeginAssetLibraryOperation reuses a parent operation, so automatic imports and
// nested replication produce one request timeline instead of duplicate audits.
func BeginAssetLibraryOperation(ctx context.Context, userID int, action, assetID string) (context.Context, func(error)) {
	if assetTrace(ctx) != nil {
		return ctx, func(error) {}
	}
	requestID, _ := ctx.Value(common.RequestIdKey).(string)
	requestID = assetTimingIdentifier(requestID)
	if requestID == "" || len(requestID) > 64 {
		requestID = common.GetUUID()
	}
	ctx = context.WithValue(ctx, common.RequestIdKey, requestID)
	trace := &assetLibraryTrace{started: time.Now()}
	trace.timing = AssetLibraryTiming{Version: 1, RequestID: requestID, Action: action, AssetID: assetID, StartedAtMS: trace.started.UnixMilli(), Stages: make([]*AssetLibraryTimingStage, 0)}
	ctx = context.WithValue(ctx, assetLibraryTraceKey{}, trace)
	return ctx, func(err error) {
		trace.timing.CompletedAtMS = time.Now().UnixMilli()
		trace.timing.DurationMS = time.Since(trace.started).Milliseconds()
		trace.timing.Outcome = "succeeded"
		if err != nil || trace.failed {
			trace.timing.Outcome = "failed"
		}
		for _, stage := range trace.timing.Stages {
			if stage.Outcome == "failed" {
				trace.timing.Outcome = "failed"
			}
		}
		if trace.auditAction == "" {
			trace.auditAction = "asset_library.request"
			trace.params = map[string]interface{}{"action": action, "id": trace.timing.AssetID}
			trace.content = "Asset library request: " + action + " (" + trace.timing.AssetID + ")"
		}
		// The log store is initialized in production; service-only callers may
		// intentionally run without an audit database (e.g. command-line tools).
		if model.LOG_DB != nil {
			model.RecordOperationAuditLog(userID, trace.content, trace.ip, trace.auditAction, trace.params,
				map[string]interface{}{"asset_timing": trace.timing}, nil, requestID)
		}
		data, marshalErr := common.Marshal(trace.timing)
		if marshalErr == nil {
			logger.LogInfo(ctx, "asset_library.request_completed "+string(data))
		}
	}
}

func assetTrace(ctx context.Context) *assetLibraryTrace {
	trace, _ := ctx.Value(assetLibraryTraceKey{}).(*assetLibraryTrace)
	return trace
}

// SetAssetLibraryAudit enriches the existing mutation audit when the request
// finishes, including when replication fails after the logical asset was saved.
func SetAssetLibraryAudit(ctx context.Context, content, ip, action string, params map[string]interface{}) bool {
	trace := assetTrace(ctx)
	if trace == nil {
		return false
	}
	trace.content, trace.ip, trace.auditAction, trace.params = content, ip, action, params
	if id, ok := params["id"].(string); ok {
		trace.timing.AssetID = id
	}
	return true
}

type assetLibrarySpan struct {
	ctx     context.Context
	trace   *assetLibraryTrace
	stage   *AssetLibraryTimingStage
	started time.Time
}

func startAssetLibraryStage(ctx context.Context, stage, operation, assetID string, channelID int) *assetLibrarySpan {
	span := &assetLibrarySpan{ctx: ctx, trace: assetTrace(ctx), started: time.Now()}
	if assetID == "" {
		assetID, _ = ctx.Value(assetLibraryResourceKey{}).(string)
	}
	if assetID == "" && span.trace != nil {
		assetID = span.trace.timing.AssetID
	}
	span.stage = &AssetLibraryTimingStage{Stage: stage, Operation: operation, AssetID: assetID, ChannelID: channelID, StartedAtMS: span.started.UnixMilli(), Outcome: "running"}
	if span.trace == nil {
		return span
	}
	span.trace.mu.Lock()
	span.stage.ID = len(span.trace.timing.Stages) + span.trace.timing.DroppedStages + 1
	if len(span.trace.timing.Stages) < 256 {
		span.trace.timing.Stages = append(span.trace.timing.Stages, span.stage)
	} else {
		span.trace.timing.DroppedStages++
	}
	span.trace.mu.Unlock()
	data, err := common.Marshal(span.stage)
	if err == nil {
		logger.LogInfo(ctx, "asset_library.stage_started "+string(data))
	}
	return span
}

func (span *assetLibrarySpan) finish(err error) {
	span.stage.DurationMS = time.Since(span.started).Milliseconds()
	span.stage.Outcome = "succeeded"
	if err != nil {
		span.stage.Outcome = "failed"
		if span.trace != nil {
			span.trace.failed = true
		}
		span.stage.ErrorKind = "operation_failed"
		if errors.Is(err, context.Canceled) {
			span.stage.ErrorKind = "canceled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			span.stage.ErrorKind = "timeout"
		}
		var upstreamErr *AssetLibraryUpstreamError
		if errors.As(err, &upstreamErr) {
			span.stage.HTTPStatus = upstreamErr.StatusCode
			span.stage.ErrorCode = assetTimingIdentifier(upstreamErr.Code)
		}
	}
	if span.trace == nil {
		return
	}
	data, marshalErr := common.Marshal(span.stage)
	if marshalErr == nil {
		logger.LogInfo(span.ctx, "asset_library.stage_completed "+string(data))
	}
}

func assetTimingIdentifier(value string) string {
	if len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return ""
	}
	return value
}

func observeAssetLibraryReplica(ctx context.Context, replica *model.UserAssetReplica) {
	trace := assetTrace(ctx)
	if trace == nil {
		return
	}
	trace.failed = trace.failed || replica.State == model.AssetReplicaStateFailed
	item := AssetLibraryReadiness{AssetID: replica.AssetId, ChannelID: replica.ChannelId, Status: replica.State,
		UploadStartedAtMS: replica.UploadStartedAtMS, SubmittedAtMS: replica.SubmittedAtMS, FirstActiveAtMS: replica.FirstActiveAtMS,
		LastPolledAtMS: replica.LastPolledAtMS, LastProcessingAtMS: replica.LastProcessingAtMS, PollCount: replica.PollCount}
	for i, previous := range trace.timing.Replicas {
		if previous.AssetID == item.AssetID && previous.ChannelID == item.ChannelID {
			trace.timing.Replicas[i] = item
			return
		}
	}
	if len(trace.timing.Replicas) < 256 {
		trace.timing.Replicas = append(trace.timing.Replicas, item)
	}
}

// CreateAssetLibraryRecord measures logical persistence before replication.
func CreateAssetLibraryRecord(ctx context.Context, asset *model.UserAsset) error {
	span := startAssetLibraryStage(ctx, "local_persistence", "CreateAsset", asset.Id, 0)
	err := model.CreateUserAsset(asset)
	span.finish(err)
	return err
}

func persistAssetLibraryReplica(ctx context.Context, replica *model.UserAssetReplica) error {
	span := startAssetLibraryStage(ctx, "local_persistence", "SaveReplica", replica.AssetId, replica.ChannelId)
	err := model.SaveUserAssetReplica(replica)
	span.finish(err)
	observeAssetLibraryReplica(ctx, replica)
	return err
}

// BeginAssetLibraryUpload marks the start of one asset attempt, independently
// of other assets in a batch. The ID can correlate failed validation too.
func BeginAssetLibraryUpload(ctx context.Context, assetID string) context.Context {
	ctx = context.WithValue(ctx, assetLibraryResourceKey{}, assetID)
	ctx = context.WithValue(ctx, assetLibraryUploadStartKey{}, time.Now().UnixMilli())
	if trace := assetTrace(ctx); trace != nil && trace.timing.Action == "CreateAsset" {
		trace.timing.AssetID = assetID
	}
	return ctx
}

// acquireAssetLibraryChannel measures queueing separately from time spent
// holding the shared channel lock. Hold time overlaps the recorded work stages
// and therefore is diagnostic metadata, not another stacked timeline segment.
func acquireAssetLibraryChannel(ctx context.Context, channelID int) *assetLibraryChannelGuard {
	span := startAssetLibraryStage(ctx, "channel_lock", "", "", channelID)
	lock := getAssetLibraryChannelLock(channelID)
	lock.Lock()
	span.finish(nil)
	return &assetLibraryChannelGuard{lock: lock, span: span, acquired: time.Now()}
}

type assetLibraryChannelGuard struct {
	lock     *sync.Mutex
	span     *assetLibrarySpan
	acquired time.Time
}

func (guard *assetLibraryChannelGuard) Unlock() {
	guard.span.stage.HeldMS = time.Since(guard.acquired).Milliseconds()
	guard.lock.Unlock()
	if guard.span.trace != nil {
		data, err := common.Marshal(guard.span.stage)
		if err == nil {
			logger.LogInfo(guard.span.ctx, "asset_library.channel_released "+string(data))
		}
	}
}
