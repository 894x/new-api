package perfmetrics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type ChannelAnalyticsQueryParams struct {
	Context       context.Context
	ChannelId     int
	StartTs       int64
	EndTs         int64
	BucketSeconds int64
}

type ChannelAnalyticsPercentiles struct {
	P50Ms       int64 `json:"p50_ms"`
	P95Ms       int64 `json:"p95_ms"`
	P99Ms       int64 `json:"p99_ms"`
	SampleCount int64 `json:"sample_count"`
}

type ChannelAnalyticsLatency struct {
	AcquireMs                ChannelAnalyticsPercentiles `json:"acquire_ms"`
	WriteMs                  ChannelAnalyticsPercentiles `json:"write_ms"`
	TotalMs                  ChannelAnalyticsPercentiles `json:"total_ms"`
	BodyReadMs               ChannelAnalyticsPercentiles `json:"body_read_ms"`
	UpstreamQueueMs          ChannelAnalyticsPercentiles `json:"upstream_queue_ms"`
	UploadMs                 ChannelAnalyticsPercentiles `json:"upload_ms"`
	ProviderWaitMs           ChannelAnalyticsPercentiles `json:"provider_wait_ms"`
	HeadersToFirstResponseMs ChannelAnalyticsPercentiles `json:"headers_to_first_response_ms"`
	DownstreamMs             ChannelAnalyticsPercentiles `json:"downstream_ms"`
}

type ChannelAnalyticsConcurrency struct {
	Average float64 `json:"average"`
	Maximum int64   `json:"maximum"`
}

type ChannelAnalyticsPoint struct {
	Ts                int64                       `json:"ts"`
	RequestCount      int64                       `json:"request_count"`
	SuccessRate       float64                     `json:"success_rate"`
	ActiveConcurrency ChannelAnalyticsConcurrency `json:"active_concurrency"`
	Latency           ChannelAnalyticsLatency     `json:"latency"`
}

type ChannelAnalyticsResult struct {
	ScannedLogs      int                     `json:"scanned_logs"`
	Truncated        bool                    `json:"truncated"`
	TransportGroups  []ChannelTransportGroup `json:"transport_groups"`
	ChannelId        int                     `json:"channel_id"`
	EffectiveStartTs int64                   `json:"effective_start_timestamp"`
	EffectiveEndTs   int64                   `json:"effective_end_timestamp"`
	Summary          ChannelAnalyticsPoint   `json:"summary"`
	Series           []ChannelAnalyticsPoint `json:"series"`
}

type channelTimingSample struct {
	attempts []common.UpstreamTransportSnapshot
	startMs  int64
	endMs    int64
	success  bool
	latency  channelLatencyAccumulator
}

type ChannelTransportGroup struct {
	ChannelID      int                         `json:"channel_id"`
	Protocol       string                      `json:"protocol"`
	BodyClass      string                      `json:"body_class"`
	Reason         string                      `json:"reason"`
	ThresholdBytes int64                       `json:"threshold_bytes"`
	Count          int64                       `json:"count"`
	Errors         int64                       `json:"errors"`
	Canceled       int64                       `json:"canceled"`
	Timeouts       int64                       `json:"timeouts"`
	Reused         int64                       `json:"reused"`
	AcquireMs      ChannelAnalyticsPercentiles `json:"acquire_ms"`
	WriteMs        ChannelAnalyticsPercentiles `json:"write_ms"`
}

type channelTransportAccumulator struct {
	group   ChannelTransportGroup
	acquire histogram
	write   histogram
}

func accumulateChannelTransport(groups map[string]*channelTransportAccumulator, attempt common.UpstreamTransportSnapshot) {
	bodyClass := "unknown"
	switch {
	case attempt.BodyBytes < 0:
	case attempt.BodyBytes < 256<<10:
		bodyClass = "<256 KiB"
	case attempt.BodyBytes < 1<<20:
		bodyClass = "256 KiB–1 MiB"
	case attempt.BodyBytes < 4<<20:
		bodyClass = "1–4 MiB"
	default:
		bodyClass = ">=4 MiB"
	}
	key := fmt.Sprintf("%d/%s/%s/%s/%d", attempt.ChannelID, attempt.Protocol, bodyClass, attempt.Reason, attempt.ThresholdBytes)
	group := groups[key]
	if group == nil {
		if len(groups) >= 256 {
			key = "overflow"
			group = groups[key]
		}
		if group == nil {
			group = &channelTransportAccumulator{group: ChannelTransportGroup{ChannelID: attempt.ChannelID, Protocol: attempt.Protocol, BodyClass: bodyClass, Reason: attempt.Reason, ThresholdBytes: attempt.ThresholdBytes}}
			if key == "overflow" {
				group.group = ChannelTransportGroup{BodyClass: "overflow", Reason: "overflow"}
			}
			groups[key] = group
		}
	}
	group.group.Count++
	if attempt.Reused {
		group.group.Reused++
	}
	if attempt.StatusCode >= 400 || attempt.Outcome == "error" || attempt.Outcome == "timeout" || attempt.Outcome == "canceled" {
		group.group.Errors++
	}
	if attempt.Outcome == "timeout" {
		group.group.Timeouts++
	}
	if attempt.Outcome == "canceled" {
		group.group.Canceled++
	}
	if attempt.AcquireMs != nil {
		group.acquire.add(int64(math.Ceil(*attempt.AcquireMs)))
	}
	if attempt.WriteMs != nil {
		group.write.add(int64(math.Ceil(*attempt.WriteMs)))
	}
}

func channelTransportResults(groups map[string]*channelTransportAccumulator) []ChannelTransportGroup {
	result := make([]ChannelTransportGroup, 0, len(groups))
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := groups[key]
		value.group.AcquireMs = value.acquire.percentiles()
		value.group.WriteMs = value.write.percentiles()
		result = append(result, value.group)
	}
	return result
}

// Sweep each event once, instead of copying long-running spans into every bucket.
func distributeChannelConcurrency(events []concurrencyEvent, buckets map[int64]*channelBucket, params ChannelAnalyticsQueryParams) {
	sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })
	index, active := 0, int64(0)
	for ts := params.StartTs; ts < params.EndTs; ts += params.BucketSeconds {
		bucket := buckets[ts]
		end := min(ts+params.BucketSeconds, params.EndTs) * 1000
		if active > 0 {
			bucket.concurrency.events = append(bucket.concurrency.events, concurrencyEvent{at: ts * 1000, delta: active})
		}
		for index < len(events) && events[index].at < end {
			event := events[index]
			bucket.concurrency.events = append(bucket.concurrency.events, event)
			active += event.delta
			index++
		}
		if active > 0 {
			bucket.concurrency.events = append(bucket.concurrency.events, concurrencyEvent{at: end, delta: -active})
		}
	}
}

type channelLatencyAccumulator struct {
	acquireMs                histogram
	writeMs                  histogram
	totalMs                  histogram
	bodyReadMs               histogram
	upstreamQueueMs          histogram
	uploadMs                 histogram
	providerWaitMs           histogram
	headersToFirstResponseMs histogram
	downstreamMs             histogram
}

type channelBucket struct {
	requestCount int64
	successCount int64
	latency      channelLatencyAccumulator
	concurrency  concurrencyAccumulator
}

type histogram struct {
	counts  [len(histogramUpperBoundsMs)]int64
	maximum int64
}

type concurrencyEvent struct {
	at    int64
	delta int64
}

type concurrencyAccumulator struct {
	events []concurrencyEvent
}

func QueryChannelAnalytics(params ChannelAnalyticsQueryParams) (ChannelAnalyticsResult, error) {
	if params.EndTs <= params.StartTs {
		return ChannelAnalyticsResult{}, nil
	}
	if params.BucketSeconds < 1 {
		params.BucketSeconds = 60
	}
	if params.BucketSeconds > 3600 {
		params.BucketSeconds = 3600
	}

	ctx := params.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	buckets := make(map[int64]*channelBucket)
	for ts := params.StartTs; ts < params.EndTs; ts += params.BucketSeconds {
		buckets[ts] = &channelBucket{}
	}
	summary := &channelBucket{}
	groups := make(map[string]*channelTransportAccumulator)
	count, truncated, err := model.VisitPerfChannelLogs(ctx, params.ChannelId, params.StartTs-3600, params.EndTs, func(row model.PerfChannelLog) {
		sample, ok := decodeChannelTiming(row)
		if !ok || sample.endMs <= params.StartTs*1000 || sample.endMs > params.EndTs*1000 || sample.startMs >= params.EndTs*1000 {
			return
		}
		// Completed requests are assigned to the bucket in which they ended.
		bucketTs := params.StartTs + (min(sample.endMs/1000, params.EndTs-1)-params.StartTs)/params.BucketSeconds*params.BucketSeconds
		bucket := buckets[bucketTs]
		if bucket == nil {
			bucket = &channelBucket{}
			buckets[bucketTs] = bucket
		}
		bucket.requestCount++
		if sample.success {
			bucket.successCount++
		}
		mergeChannelLatency(&bucket.latency, &sample.latency)
		mergeChannelLatency(&summary.latency, &sample.latency)
		addConcurrencySpan(summary, sample.startMs, sample.endMs, params.StartTs, params.EndTs-params.StartTs)
		for _, attempt := range sample.attempts {
			accumulateChannelTransport(groups, attempt)
		}
		summary.requestCount++
		if sample.success {
			summary.successCount++
		}
	})
	if err != nil {
		return ChannelAnalyticsResult{}, err
	}
	distributeChannelConcurrency(summary.concurrency.events, buckets, params)

	series := make([]ChannelAnalyticsPoint, 0, len(buckets))
	for bucketTs, bucket := range buckets {
		series = append(series, buildChannelPoint(bucketTs, bucket, min(params.BucketSeconds, params.EndTs-bucketTs)))
	}
	sort.Slice(series, func(i, j int) bool { return series[i].Ts < series[j].Ts })

	return ChannelAnalyticsResult{
		ScannedLogs: count, Truncated: truncated, TransportGroups: channelTransportResults(groups),
		ChannelId:        params.ChannelId,
		EffectiveStartTs: params.StartTs,
		EffectiveEndTs:   params.EndTs,
		Summary:          buildChannelPoint(params.StartTs, summary, params.EndTs-params.StartTs),
		Series:           series,
	}, nil
}

func decodeChannelTiming(row model.PerfChannelLog) (channelTimingSample, bool) {
	completedMs := row.CreatedAt * 1000
	var other struct {
		AdminInfo struct {
			RequestTiming common.RequestTimingSnapshot       `json:"request_timing"`
			Transport     []common.UpstreamTransportSnapshot `json:"upstream_transport"`
		} `json:"admin_info"`
	}
	if row.Other != "" {
		_ = common.Unmarshal([]byte(row.Other), &other)
	}
	timing := other.AdminInfo.RequestTiming
	if timing.RequestCompletedAtMs > 0 {
		completedMs = timing.RequestCompletedAtMs
	}
	startMs := timing.RequestReceivedAtMs
	if startMs <= 0 && row.UseTime > 0 {
		startMs = completedMs - int64(row.UseTime)*1000
	}
	if startMs <= 0 || completedMs <= startMs {
		return channelTimingSample{}, false
	}

	sample := channelTimingSample{
		attempts: other.AdminInfo.Transport,
		startMs:  startMs,
		endMs:    completedMs,
		success:  row.Type == model.LogTypeConsume,
	}
	// Error and consume logs represent individual relay attempts; earlier snapshots
	// remain available in request details but must not be counted again here.
	if len(sample.attempts) > 1 {
		sample.attempts = sample.attempts[len(sample.attempts)-1:]
	}
	for _, attempt := range sample.attempts {
		if attempt.AcquireMs != nil {
			sample.latency.acquireMs.add(int64(math.Ceil(*attempt.AcquireMs)))
		}
		if attempt.WriteMs != nil {
			sample.latency.writeMs.add(int64(math.Ceil(*attempt.WriteMs)))
		}
	}
	if timing.RequestBodyReadAtMs > 0 {
		sample.latency.bodyReadMs.add(maxDurationMs(timing.RequestBodyReadAtMs - startMs))
	}
	if timing.UpstreamRequestStartedAtMs > 0 && timing.RequestBodyReadAtMs > 0 {
		sample.latency.upstreamQueueMs.add(maxDurationMs(timing.UpstreamRequestStartedAtMs - timing.RequestBodyReadAtMs))
	}
	if timing.UpstreamRequestWrittenAtMs > 0 && timing.UpstreamRequestStartedAtMs > 0 {
		sample.latency.uploadMs.add(maxDurationMs(timing.UpstreamRequestWrittenAtMs - timing.UpstreamRequestStartedAtMs))
	}
	if timing.UpstreamResponseHeadersAtMs > 0 && timing.UpstreamRequestWrittenAtMs > 0 {
		sample.latency.providerWaitMs.add(maxDurationMs(timing.UpstreamResponseHeadersAtMs - timing.UpstreamRequestWrittenAtMs))
	}
	if timing.FirstResponseAtMs > 0 && timing.UpstreamResponseHeadersAtMs > 0 {
		sample.latency.headersToFirstResponseMs.add(maxDurationMs(timing.FirstResponseAtMs - timing.UpstreamResponseHeadersAtMs))
	}
	if timing.RequestCompletedAtMs > 0 && timing.FirstResponseAtMs > 0 {
		sample.latency.downstreamMs.add(maxDurationMs(timing.RequestCompletedAtMs - timing.FirstResponseAtMs))
	}
	sample.latency.totalMs.add(completedMs - startMs)
	return sample, true
}

func maxDurationMs(value int64) int64 {
	if value < 0 {
		return -1
	}
	return value
}

func bucketStartAt(ts int64, bucketSeconds int64) int64 {
	return ts - ts%bucketSeconds
}

func (h *histogram) add(valueMs int64) {
	if valueMs < 0 {
		return
	}
	h.maximum = max(h.maximum, valueMs)
	for i, upperBoundMs := range histogramUpperBoundsMs {
		if valueMs <= upperBoundMs {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.counts)-1]++
}

func (h histogram) percentiles() ChannelAnalyticsPercentiles {
	var total int64
	for _, count := range h.counts {
		total += count
	}
	return ChannelAnalyticsPercentiles{
		P50Ms:       h.percentile(total, 0.50),
		P95Ms:       h.percentile(total, 0.95),
		P99Ms:       h.percentile(total, 0.99),
		SampleCount: total,
	}
}

func (h histogram) percentile(total int64, percentile float64) int64 {
	if total <= 0 {
		return 0
	}
	rank := int64(math.Ceil(float64(total) * percentile))
	var cumulative int64
	for i, count := range h.counts {
		cumulative += count
		if cumulative >= rank {
			if i == len(h.counts)-1 {
				return max(h.maximum, histogramUpperBoundsMs[i])
			}
			return histogramUpperBoundsMs[i]
		}
	}
	return histogramUpperBoundsMs[len(histogramUpperBoundsMs)-1]
}

func mergeChannelLatency(target *channelLatencyAccumulator, source *channelLatencyAccumulator) {
	mergeHistogram(&target.acquireMs, &source.acquireMs)
	mergeHistogram(&target.writeMs, &source.writeMs)
	mergeHistogram(&target.totalMs, &source.totalMs)
	mergeHistogram(&target.bodyReadMs, &source.bodyReadMs)
	mergeHistogram(&target.upstreamQueueMs, &source.upstreamQueueMs)
	mergeHistogram(&target.uploadMs, &source.uploadMs)
	mergeHistogram(&target.providerWaitMs, &source.providerWaitMs)
	mergeHistogram(&target.headersToFirstResponseMs, &source.headersToFirstResponseMs)
	mergeHistogram(&target.downstreamMs, &source.downstreamMs)
}

func mergeHistogram(target *histogram, source *histogram) {
	target.maximum = max(target.maximum, source.maximum)
	for i, count := range source.counts {
		target.counts[i] += count
	}
}

func addConcurrencySpan(bucket *channelBucket, startMs int64, endMs int64, bucketTs int64, bucketSeconds int64) {
	if bucketSeconds <= 0 {
		return
	}
	startBoundary := bucketTs * 1000
	endBoundary := (bucketTs + bucketSeconds) * 1000
	start := maxInt64(startMs, startBoundary)
	end := minInt64(endMs, endBoundary)
	if end <= start {
		return
	}
	bucket.concurrency.events = append(bucket.concurrency.events,
		concurrencyEvent{at: start, delta: 1},
		concurrencyEvent{at: end, delta: -1},
	)
}

func buildChannelPoint(ts int64, bucket *channelBucket, bucketSeconds int64) ChannelAnalyticsPoint {
	average, maximum := bucket.concurrency.summary(ts*1000, (ts+bucketSeconds)*1000)
	return ChannelAnalyticsPoint{
		Ts:                ts,
		RequestCount:      bucket.requestCount,
		SuccessRate:       channelSuccessRate(bucket),
		ActiveConcurrency: ChannelAnalyticsConcurrency{Average: average, Maximum: maximum},
		Latency:           channelLatencyResult(bucket.latency),
	}
}

func (c concurrencyAccumulator) summary(startMs int64, endMs int64) (float64, int64) {
	if endMs <= startMs || len(c.events) == 0 {
		return 0, 0
	}
	events := append([]concurrencyEvent(nil), c.events...)
	sort.Slice(events, func(i, j int) bool {
		if events[i].at == events[j].at {
			return events[i].delta < events[j].delta
		}
		return events[i].at < events[j].at
	})
	current := int64(0)
	maximum := int64(0)
	areaMs := int64(0)
	previous := startMs
	for _, event := range events {
		at := minInt64(maxInt64(event.at, startMs), endMs)
		if at > previous {
			areaMs += current * (at - previous)
			previous = at
		}
		current += event.delta
		if current > maximum {
			maximum = current
		}
	}
	if previous < endMs {
		areaMs += current * (endMs - previous)
	}
	return math.Round(float64(areaMs)/float64(endMs-startMs)*100) / 100, maximum
}

func channelSuccessRate(bucket *channelBucket) float64 {
	if bucket.requestCount <= 0 {
		return 0
	}
	return math.Round(float64(bucket.successCount)/float64(bucket.requestCount)*10000) / 100
}

func channelLatencyResult(value channelLatencyAccumulator) ChannelAnalyticsLatency {
	return ChannelAnalyticsLatency{
		AcquireMs: value.acquireMs.percentiles(), WriteMs: value.writeMs.percentiles(),
		TotalMs: value.totalMs.percentiles(), BodyReadMs: value.bodyReadMs.percentiles(),
		UpstreamQueueMs: value.upstreamQueueMs.percentiles(), UploadMs: value.uploadMs.percentiles(),
		ProviderWaitMs: value.providerWaitMs.percentiles(), HeadersToFirstResponseMs: value.headersToFirstResponseMs.percentiles(),
		DownstreamMs: value.downstreamMs.percentiles(),
	}
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
