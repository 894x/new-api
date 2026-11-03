package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// Active targets schedule synthetic requests. Enabling monitoring also listens
// to ordinary traffic for its routed public models and HTTP protocols.
func ChannelTrafficMonitoringEndpoint(channel *model.Channel, info *relaycommon.RelayInfo, publicModel string) string {
	if channel == nil || info == nil || info.IsChannelTest || publicModel == "" || len(publicModel) > 128 ||
		model.CurrentRequestPolicy().Options["monitor_setting.channel_monitoring_enabled"] != "true" {
		return ""
	}
	enabled, _, err := channel.MonitorTargets()
	if err != nil || !enabled {
		return ""
	}
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		return string(constant.EndpointTypeOpenAI)
	case types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		return string(constant.EndpointTypeOpenAIResponse)
	case types.RelayFormatClaude:
		return string(constant.EndpointTypeAnthropic)
	case types.RelayFormatGemini:
		return string(constant.EndpointTypeGemini)
	case types.RelayFormatEmbedding:
		return string(constant.EndpointTypeEmbeddings)
	case types.RelayFormatRerank:
		return string(constant.EndpointTypeJinaRerank)
	default:
		return ""
	}
}

// ChannelTrafficObservation classifies raw upstream facts before retry and
// client-facing status mappings. HTTP exclusions are applied later at query
// time, using exactly the same policy as active probes.
func ChannelTrafficObservation(ctx context.Context, info *relaycommon.RelayInfo, endpoint string, apiErr *types.NewAPIError) (model.ChannelMonitorRecord, bool) {
	if info == nil {
		return model.ChannelMonitorRecord{}, false
	}
	a, reached := info.ChannelMonitoringSnapshot()
	stream := info.StreamStatus.OutcomeSnapshot()
	if !reached || endpoint == "" || (ctx != nil && ctx.Err() != nil) || (apiErr != nil && errors.Is(apiErr, context.Canceled)) ||
		stream.EndReason == relaycommon.StreamEndReasonClientGone || stream.EndReason == relaycommon.StreamEndReasonPingFail ||
		stream.Response == relaycommon.ResponseOutcomeCancelled || info.PerformanceBusinessRejection {
		return model.ChannelMonitorRecord{}, false
	}
	record := model.ChannelMonitorRecord{
		Source: "traffic", Count: 1, ChannelID: a.ChannelID, Model: a.Model,
		EndpointType: endpoint, CreatedAt: a.FinishedAt.Unix(),
		ObservedAtMs: a.FinishedAt.UnixMilli(),
		BucketAt:     a.FinishedAt.Unix() / 900 * 900,
		State:        "healthy", StatusCode: a.HTTPStatus,
		LatencyMs: max(0, a.FinishedAt.Sub(a.StartedAt).Milliseconds()),
	}
	failed := apiErr != nil || a.HTTPStatus >= 400 || stream.Response == relaycommon.ResponseOutcomeFailed || stream.HasErrors
	if stream.ErrorStatus > 0 {
		record.StatusCode = stream.ErrorStatus
		failed = failed || stream.ErrorStatus >= 400
	}
	if stream.Response == relaycommon.ResponseOutcomeIncomplete && stream.IncompleteReason != "max_output_tokens" && stream.IncompleteReason != "max_tokens" {
		failed = true
	}
	switch stream.EndReason {
	case relaycommon.StreamEndReasonTimeout, relaycommon.StreamEndReasonScannerErr, relaycommon.StreamEndReasonPanic:
		failed = true
	}
	if info.IsStream && (stream.ExpectsTerminal && stream.Response == relaycommon.ResponseOutcomeUnknown && stream.EndReason != relaycommon.StreamEndReasonDone || a.FirstResponseAt.IsZero()) {
		failed = true
	}
	if failed {
		record.State, record.ErrorCode = "unavailable", "upstream_request_failed"
		return record, true
	}
	latency := record.LatencyMs
	if info.IsStream && !a.FirstContentAt.IsZero() && !a.TTFTInvalid {
		ttft := max(0, a.FirstContentAt.Sub(a.StartedAt).Milliseconds())
		record.TTFTMs, latency = &ttft, ttft
	}
	if info.IsStream && !a.TPOTInvalid && a.CompletionTokens > 1 && a.LastContentAt.After(a.FirstContentAt) && !a.FirstContentAt.IsZero() {
		speed := float64(a.CompletionTokens-1) / a.LastContentAt.Sub(a.FirstContentAt).Seconds()
		record.TokensPerSecond = &speed
	}
	record.State = model.ChannelMonitorLatencyState(latency)
	return record, true
}

type channelTrafficKey struct {
	ChannelID              int
	Model, Endpoint, State string
	Status                 int
	Bucket                 int64
}

// Counts are cumulative within a fixed quarter-hour and process run. Retrying
// an ambiguous database write replaces the same count instead of adding it.
// Memory is bounded; overflow is recorded as a gap, disabling the 24h rate.
type channelTrafficCollector struct {
	mu      sync.Mutex
	flushMu sync.Mutex
	runID   string
	limit   int
	records map[channelTrafficKey]model.ChannelMonitorRecord
	timings map[channelTrafficKey]model.ChannelMonitorTimings
	gapAt   int64
	save    func(context.Context, []model.ChannelMonitorRecord) error
}

var channelTraffic = channelTrafficCollector{
	runID: rand.Text(), limit: 8192,
	records: make(map[channelTrafficKey]model.ChannelMonitorRecord),
	save:    model.SaveChannelTrafficRecords,
}

func (collector *channelTrafficCollector) observe(record model.ChannelMonitorRecord) {
	key := channelTrafficKey{record.ChannelID, record.Model, record.EndpointType, record.State, record.StatusCode, record.BucketAt}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	previous, exists := collector.records[key]
	if !exists && len(collector.records) >= collector.limit {
		collector.gapAt = max(collector.gapAt, record.CreatedAt)
		return
	}
	if collector.timings == nil {
		collector.timings = make(map[channelTrafficKey]model.ChannelMonitorTimings)
	}
	timing := collector.timings[key]
	timing.Add(record)
	collector.timings[key] = timing
	if exists && previous.ObservedAtMs > record.ObservedAtMs {
		previous.Count++
		collector.records[key] = previous
		return
	}
	record.SampleKey = previous.SampleKey
	if !exists {
		record.SampleKey = fmt.Sprintf("%x", sha256.Sum256([]byte(collector.runID+rand.Text())))
	}
	record.Count = previous.Count + 1
	collector.records[key] = record
}

func ObserveChannelTraffic(ctx context.Context, info *relaycommon.RelayInfo, endpoint string, apiErr *types.NewAPIError) {
	if record, ok := ChannelTrafficObservation(ctx, info, endpoint, apiErr); ok {
		channelTraffic.observe(record)
	}
}

func (collector *channelTrafficCollector) flush(ctx context.Context, now time.Time) error {
	collector.flushMu.Lock()
	defer collector.flushMu.Unlock()
	collector.mu.Lock()
	records := make([]model.ChannelMonitorRecord, 0, len(collector.records)+1)
	timings := make([]model.ChannelMonitorTimings, 0, len(collector.records))
	for key, record := range collector.records {
		records = append(records, record)
		timings = append(timings, collector.timings[key].Clone())
	}
	if collector.gapAt > 0 {
		records = append(records, model.ChannelMonitorRecord{SampleKey: fmt.Sprintf("%x", sha256.Sum256([]byte(collector.runID+"gap"))), Source: "gap", CreatedAt: collector.gapAt, State: "collection_gap"})
	}
	collector.mu.Unlock()
	for i, timing := range timings {
		encoded, err := common.Marshal(timing)
		if err != nil {
			return err
		}
		records[i].TimingHistogram = string(encoded)
	}
	if err := collector.save(ctx, records); err != nil {
		return err
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	for _, record := range records {
		key := channelTrafficKey{record.ChannelID, record.Model, record.EndpointType, record.State, record.StatusCode, record.BucketAt}
		if current, ok := collector.records[key]; ok && current.Count == record.Count && record.BucketAt < now.Unix()/900*900 {
			delete(collector.records, key)
			delete(collector.timings, key)
		}
	}
	return nil
}

var channelTrafficStop = make(chan struct{})
var channelTrafficStopped = make(chan struct{})

func StartChannelTrafficMonitoring() {
	go func() {
		defer close(channelTrafficStopped)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		var prunedAt time.Time
		for {
			select {
			case <-channelTrafficStop:
				return
			case now := <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := channelTraffic.flush(ctx, now); err != nil {
					common.SysError("channel traffic monitoring flush failed: " + err.Error())
				}
				if now.Sub(prunedAt) >= time.Minute {
					if err := model.PruneChannelMonitorRecords(ctx, now); err != nil {
						common.SysError("channel monitoring retention failed: " + err.Error())
					}
					prunedAt = now
				}
				cancel()
			}
		}
	}()
}

func StopChannelTrafficMonitoring(ctx context.Context) {
	close(channelTrafficStop)
	select {
	case <-channelTrafficStopped:
	case <-ctx.Done():
		return
	}
	if err := channelTraffic.flush(ctx, time.Now()); err != nil {
		common.SysError("channel traffic monitoring final flush failed: " + err.Error())
	}
}
