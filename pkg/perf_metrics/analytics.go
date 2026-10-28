package perfmetrics

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

const (
	metricTtft             = "ttft"
	metricTpot             = "tpot"
	metricTtftSum          = "ttft_sum"
	metricTpotSum          = "tpot_sum"
	metricTtftAverageCount = "ttft_n"
	metricTpotAverageCount = "tpot_n"
	metricTpotTokens       = "tpot_tok"
)

var histogramUpperBoundsMs = [...]int64{
	5, 10, 20, 40, 80, 100, 160, 250, 500, 750, 1000, 1500, 2000,
	3000, 5000, 7500, 10000, 15000, 20000, 30000, 60000, 120000,
	300000, 600000,
}

type AnalyticsQueryParams struct {
	Context context.Context
	Model   string
	UserId  int
	TokenId int
	StartTs int64
	EndTs   int64
}

type AnalyticsPercentiles struct {
	AvgMs       *float64 `json:"avg_ms"`
	P50Ms       int64    `json:"p50_ms"`
	P90Ms       int64    `json:"p90_ms"`
	P99Ms       int64    `json:"p99_ms"`
	SampleCount int64    `json:"sample_count"`
}

type AnalyticsSummary struct {
	AnalyticsUsage
	RequestCount int64                `json:"request_count"`
	ErrorCount   int64                `json:"error_count"`
	SuccessRate  float64              `json:"success_rate"`
	Rpm          float64              `json:"rpm"`
	Tpm          float64              `json:"tpm"`
	CacheHitRate float64              `json:"cache_hit_rate"`
	Ttft         AnalyticsPercentiles `json:"ttft"`
	Tpot         AnalyticsPercentiles `json:"tpot"`
}

type AnalyticsPoint struct {
	AnalyticsUsage
	Ts           int64                `json:"ts"`
	RequestCount int64                `json:"request_count"`
	ErrorCount   int64                `json:"error_count"`
	SuccessRate  float64              `json:"success_rate"`
	Rpm          float64              `json:"rpm"`
	Tpm          float64              `json:"tpm"`
	CacheHitRate float64              `json:"cache_hit_rate"`
	Ttft         AnalyticsPercentiles `json:"ttft"`
	Tpot         AnalyticsPercentiles `json:"tpot"`
}

type AnalyticsUsage struct {
	RequestBodySamples  int64    `json:"request_body_samples"`
	RequestBodyBytes    int64    `json:"request_body_bytes"`
	RequestBodyMaxBytes int64    `json:"request_body_max_bytes"`
	CacheReadSamples    int64    `json:"cache_read_samples"`
	CacheWriteTokens    int64    `json:"cache_write_tokens"`
	CacheWriteSamples   int64    `json:"cache_write_samples"`
	CacheWrite5mTokens  int64    `json:"cache_write_5m_tokens"`
	CacheWrite1hTokens  int64    `json:"cache_write_1h_tokens"`
	CacheReadTokens     int64    `json:"cache_read_tokens"`
	RequestBodyAvgBytes *float64 `json:"request_body_avg_bytes"`
}

type AnalyticsResult struct {
	ErrorsUnavailable bool                `json:"errors_unavailable"`
	ModelName         string              `json:"model_name"`
	ScannedLogs       int                 `json:"scanned_logs"`
	Truncated         bool                `json:"truncated"`
	ErrorGroups       []ChannelErrorGroup `json:"error_groups"`
	EffectiveStartTs  int64               `json:"effective_start_timestamp"`
	EffectiveEndTs    int64               `json:"effective_end_timestamp"`
	Summary           AnalyticsSummary    `json:"summary"`
	Series            []AnalyticsPoint    `json:"series"`
}

type detailBucketKey struct {
	model    string
	userId   int
	tokenId  int
	bucketTs int64
}

type detailCounters struct {
	tpotGenerationTokens int64
	tpotAverageCount     int64
	ttftSumMs            int64
	tpotGenerationMs     int64
	requestCount         int64
	successCount         int64
	inputTokens          int64
	outputTokens         int64
	totalTokens          int64
	cacheReadTokens      int64
	requestBodySamples   int64
	requestBodyBytes     int64
	requestBodyMaxBytes  int64
	cacheReadSamples     int64
	cacheWriteTokens     int64
	cacheWriteSamples    int64
	cacheWrite5mTokens   int64
	cacheWrite1hTokens   int64

	ttftCount   int64
	tpotCount   int64
	ttftBuckets [len(histogramUpperBoundsMs)]int64
	tpotBuckets [len(histogramUpperBoundsMs)]int64
}

type analyticsAccumulator struct {
	tpotGenerationTokens int64
	ttftSumMs            int64
	tpotGenerationMs     int64
	ttftAverageCount     int64
	tpotAverageCount     int64
	requestCount         int64
	errorCount           int64
	successCount         int64
	inputTokens          int64
	totalTokens          int64
	cacheReadTokens      int64
	requestBodySamples   int64
	requestBodyBytes     int64
	requestBodyMaxBytes  int64
	cacheReadSamples     int64
	cacheWriteTokens     int64
	cacheWriteSamples    int64
	cacheWrite5mTokens   int64
	cacheWrite1hTokens   int64

	ttft map[int64]int64
	tpot map[int64]int64
}

func QueryAnalytics(params AnalyticsQueryParams) (AnalyticsResult, error) {
	if params.Model == "" || params.StartTs <= 0 || params.EndTs <= params.StartTs {
		return AnalyticsResult{}, errors.New("invalid performance analytics query")
	}
	requestedStartTs := params.StartTs
	params.StartTs = bucketStart(params.StartTs)
	storageBucketSeconds := int64(perf_metrics_setting.GetBucketSeconds())
	effectiveEndTs := bucketStart(params.EndTs) + storageBucketSeconds - 1
	bucketSeconds := analyticsBucketSeconds(
		params.StartTs,
		params.EndTs,
		storageBucketSeconds,
	)

	filter := model.PerfMetricDetailFilter{
		ModelName:     params.Model,
		UserId:        params.UserId,
		TokenId:       params.TokenId,
		StartTs:       params.StartTs,
		EndTs:         params.EndTs,
		BucketSeconds: bucketSeconds,
	}
	ctx := params.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	filter.Context = ctx
	var series map[int64]*analyticsAccumulator
	for {
		if err := ctx.Err(); err != nil {
			return AnalyticsResult{}, err
		}
		detailSnapshotMu.RLock()
		generation, done := detailFlushGeneration, detailFlushDone
		detailSnapshotMu.RUnlock()
		if generation%2 != 0 {
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return AnalyticsResult{}, ctx.Err()
			}
		}
		details, histograms, err := model.GetPerfAnalyticsBuckets(filter)
		if err != nil {
			if ctx.Err() != nil {
				return AnalyticsResult{}, ctx.Err()
			}
			return AnalyticsResult{}, err
		}
		series = make(map[int64]*analyticsAccumulator)
		for _, detail := range details {
			point := analyticsPointAccumulator(series, detail.BucketTs)
			point.requestCount += detail.RequestCount
			point.successCount += detail.SuccessCount
			point.inputTokens += detail.InputTokens
			point.totalTokens += detail.TotalTokens
			point.cacheReadTokens += detail.CacheReadTokens
			if detail.CacheReadSamples == 0 && detail.CacheReadTokens > 0 {
				point.cacheReadSamples += detail.RequestCount
			}
			point.requestBodySamples += detail.RequestBodySamples
			point.requestBodyBytes += detail.RequestBodyBytes
			point.requestBodyMaxBytes = max(point.requestBodyMaxBytes, detail.RequestBodyMaxBytes)
			point.cacheReadSamples += detail.CacheReadSamples
			point.cacheWriteTokens += detail.CacheWriteTokens
			point.cacheWriteSamples += detail.CacheWriteSamples
			point.cacheWrite5mTokens += detail.CacheWrite5mTokens
			point.cacheWrite1hTokens += detail.CacheWrite1hTokens

		}
		for _, histogram := range histograms {
			addHistogramCount(analyticsPointAccumulator(series, histogram.BucketTs), histogram.Metric, histogram.UpperBoundMs, histogram.Count)
		}
		detailSnapshotMu.RLock()
		if generation != detailFlushGeneration {
			detailSnapshotMu.RUnlock()
			continue
		}
		mergeActiveDetailBuckets(series, params, bucketSeconds)
		detailSnapshotMu.RUnlock()
		break
	}

	errorParams := params
	errorParams.Context = ctx
	errorParams.StartTs = requestedStartTs
	scannedLogs, truncated, errorGroups, err := queryAnalyticsErrors(errorParams, series, bucketSeconds)
	if err != nil {
		if params.Context != nil && params.Context.Err() != nil {
			return AnalyticsResult{}, params.Context.Err()
		}
		common.SysError("performance error breakdown unavailable: " + err.Error())
		truncated = true
	}

	timestamps := make([]int64, 0, len(series))
	for ts := range series {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	summary := newAnalyticsAccumulator()
	points := make([]AnalyticsPoint, 0, len(timestamps))
	for _, ts := range timestamps {
		accumulator := series[ts]
		mergeAnalyticsAccumulator(summary, accumulator)
		points = append(points, buildAnalyticsPoint(ts, bucketSeconds, accumulator))
	}
	effectiveDurationSeconds := effectiveEndTs - params.StartTs + 1

	return AnalyticsResult{
		ErrorsUnavailable: err != nil,
		ModelName:         params.Model,
		ScannedLogs:       scannedLogs,
		Truncated:         truncated,
		ErrorGroups:       errorGroups,
		EffectiveStartTs:  params.StartTs,
		EffectiveEndTs:    effectiveEndTs,
		Summary:           buildAnalyticsSummary(effectiveDurationSeconds, summary),
		Series:            points,
	}, nil
}

func mergeActiveDetailBuckets(series map[int64]*analyticsAccumulator, params AnalyticsQueryParams, bucketSeconds int64) {
	detailHotBuckets.Range(func(key, value any) bool {
		bucketKey := key.(detailBucketKey)
		if bucketKey.model != params.Model || bucketKey.bucketTs < params.StartTs || bucketKey.bucketTs > params.EndTs {
			return true
		}
		if params.UserId > 0 && bucketKey.userId != params.UserId {
			return true
		}
		if params.TokenId > 0 && bucketKey.tokenId != params.TokenId {
			return true
		}
		bucketTs := bucketKey.bucketTs - bucketKey.bucketTs%bucketSeconds
		addDetailCounters(analyticsPointAccumulator(series, bucketTs), value.(*atomicDetailBucket).snapshot())
		return true
	})
}

func addDetailCounters(target *analyticsAccumulator, source detailCounters) {
	target.ttftSumMs += source.ttftSumMs
	target.tpotGenerationMs += source.tpotGenerationMs
	target.ttftAverageCount += source.ttftCount
	target.tpotAverageCount += source.tpotAverageCount
	target.tpotGenerationTokens += source.tpotGenerationTokens
	target.requestCount += source.requestCount
	target.successCount += source.successCount
	target.inputTokens += source.inputTokens
	target.totalTokens += source.totalTokens
	target.cacheReadTokens += source.cacheReadTokens
	target.requestBodySamples += source.requestBodySamples
	target.requestBodyBytes += source.requestBodyBytes
	target.requestBodyMaxBytes = max(target.requestBodyMaxBytes, source.requestBodyMaxBytes)
	target.cacheReadSamples += source.cacheReadSamples
	target.cacheWriteTokens += source.cacheWriteTokens
	target.cacheWriteSamples += source.cacheWriteSamples
	target.cacheWrite5mTokens += source.cacheWrite5mTokens
	target.cacheWrite1hTokens += source.cacheWrite1hTokens

	for i, count := range source.ttftBuckets {
		if count > 0 {
			target.ttft[histogramUpperBoundsMs[i]] += count
		}
	}
	for i, count := range source.tpotBuckets {
		if count > 0 {
			target.tpot[histogramUpperBoundsMs[i]] += count
		}
	}
}

func queryAnalyticsErrors(params AnalyticsQueryParams, series map[int64]*analyticsAccumulator, bucketSeconds int64) (int, bool, []ChannelErrorGroup, error) {
	if params.UserId <= 0 || model.LOG_DB == nil {
		return 0, false, nil, nil
	}
	ctx := params.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	groups := make(map[string]*ChannelErrorGroup)
	scanned, truncated, err := model.VisitPerfUserLogs(ctx, params.UserId, params.Model, params.TokenId, params.StartTs, params.EndTs, func(row model.PerfChannelLog) {
		if row.Type != model.LogTypeError {
			return
		}
		bucketTs := row.CreatedAt - row.CreatedAt%bucketSeconds
		point := analyticsPointAccumulator(series, bucketTs)
		point.errorCount++
		accumulateChannelError(groups, row)
	})
	if err != nil {
		return scanned, truncated, nil, err
	}
	return scanned, truncated, channelErrorResults(groups), nil
}

func analyticsBucketSeconds(startTs int64, endTs int64, storageBucketSeconds int64) int64 {
	durationSeconds := endTs - startTs
	desiredBucketSeconds := int64(5 * 60)
	if durationSeconds > 14*24*60*60 {
		desiredBucketSeconds = 6 * 60 * 60
	} else if durationSeconds > 2*24*60*60 {
		desiredBucketSeconds = 60 * 60
	}
	if storageBucketSeconds > desiredBucketSeconds {
		return storageBucketSeconds
	}
	return desiredBucketSeconds
}

func analyticsPointAccumulator(series map[int64]*analyticsAccumulator, ts int64) *analyticsAccumulator {
	point, ok := series[ts]
	if !ok {
		point = newAnalyticsAccumulator()
		series[ts] = point
	}
	return point
}

func newAnalyticsAccumulator() *analyticsAccumulator {
	return &analyticsAccumulator{
		ttft: make(map[int64]int64),
		tpot: make(map[int64]int64),
	}
}

func addHistogramCount(accumulator *analyticsAccumulator, metric string, upperBoundMs int64, count int64) {
	if count <= 0 {
		return
	}
	switch metric {
	case metricTtft:
		accumulator.ttft[upperBoundMs] += count
	case metricTpot:
		accumulator.tpot[upperBoundMs] += count
	case metricTtftSum:
		accumulator.ttftSumMs += count
	case metricTpotTokens:
		accumulator.tpotGenerationTokens += count
	case metricTpotSum:
		accumulator.tpotGenerationMs += count
	case metricTtftAverageCount:
		accumulator.ttftAverageCount += count
	case metricTpotAverageCount:
		accumulator.tpotAverageCount += count
	}
}

func mergeAnalyticsAccumulator(target *analyticsAccumulator, source *analyticsAccumulator) {
	target.ttftSumMs += source.ttftSumMs
	target.tpotGenerationMs += source.tpotGenerationMs
	target.ttftAverageCount += source.ttftAverageCount
	target.tpotAverageCount += source.tpotAverageCount
	target.tpotGenerationTokens += source.tpotGenerationTokens
	target.requestCount += source.requestCount
	target.errorCount += source.errorCount
	target.successCount += source.successCount
	target.inputTokens += source.inputTokens
	target.totalTokens += source.totalTokens
	target.cacheReadTokens += source.cacheReadTokens
	target.requestBodySamples += source.requestBodySamples
	target.requestBodyBytes += source.requestBodyBytes
	target.requestBodyMaxBytes = max(target.requestBodyMaxBytes, source.requestBodyMaxBytes)
	target.cacheReadSamples += source.cacheReadSamples
	target.cacheWriteTokens += source.cacheWriteTokens
	target.cacheWriteSamples += source.cacheWriteSamples
	target.cacheWrite5mTokens += source.cacheWrite5mTokens
	target.cacheWrite1hTokens += source.cacheWrite1hTokens

	for upperBoundMs, count := range source.ttft {
		target.ttft[upperBoundMs] += count
	}
	for upperBoundMs, count := range source.tpot {
		target.tpot[upperBoundMs] += count
	}
}

func buildAnalyticsPoint(ts int64, bucketSeconds int64, accumulator *analyticsAccumulator) AnalyticsPoint {
	return AnalyticsPoint{
		Ts:             ts,
		AnalyticsUsage: buildAnalyticsUsage(accumulator),
		RequestCount:   accumulator.requestCount,
		ErrorCount:     accumulator.errorCount,
		SuccessRate:    analyticsSuccessRate(accumulator),
		Rpm:            analyticsPerMinute(accumulator.requestCount, bucketSeconds),
		Tpm:            analyticsPerMinute(accumulator.totalTokens, bucketSeconds),
		CacheHitRate:   analyticsCacheHitRate(accumulator),
		Ttft:           analyticsLatencyPercentiles(accumulator.ttft, accumulator.ttftSumMs, accumulator.ttftAverageCount, accumulator.ttftAverageCount),
		Tpot:           analyticsLatencyPercentiles(accumulator.tpot, accumulator.tpotGenerationMs, accumulator.tpotAverageCount, accumulator.tpotGenerationTokens),
	}
}

func buildAnalyticsSummary(durationSeconds int64, accumulator *analyticsAccumulator) AnalyticsSummary {
	return AnalyticsSummary{
		AnalyticsUsage: buildAnalyticsUsage(accumulator),
		RequestCount:   accumulator.requestCount,
		ErrorCount:     accumulator.errorCount,
		SuccessRate:    analyticsSuccessRate(accumulator),
		Rpm:            analyticsPerMinute(accumulator.requestCount, durationSeconds),
		Tpm:            analyticsPerMinute(accumulator.totalTokens, durationSeconds),
		CacheHitRate:   analyticsCacheHitRate(accumulator),
		Ttft:           analyticsLatencyPercentiles(accumulator.ttft, accumulator.ttftSumMs, accumulator.ttftAverageCount, accumulator.ttftAverageCount),
		Tpot:           analyticsLatencyPercentiles(accumulator.tpot, accumulator.tpotGenerationMs, accumulator.tpotAverageCount, accumulator.tpotGenerationTokens),
	}
}

func analyticsPerMinute(count int64, durationSeconds int64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return math.Round(float64(count)*60/float64(durationSeconds)*100) / 100
}

func analyticsCacheHitRate(accumulator *analyticsAccumulator) float64 {
	if accumulator.inputTokens <= 0 || accumulator.cacheReadTokens <= 0 {
		return 0
	}
	cacheReadTokens := min(accumulator.cacheReadTokens, accumulator.inputTokens)
	return math.Round(float64(cacheReadTokens)/float64(accumulator.inputTokens)*10000) / 100
}

func analyticsSuccessRate(accumulator *analyticsAccumulator) float64 {
	if accumulator.requestCount == 0 {
		return 0
	}
	return math.Round(float64(accumulator.successCount)/float64(accumulator.requestCount)*10000) / 100
}

func histogramPercentiles(histogram map[int64]int64) AnalyticsPercentiles {
	var sampleCount int64
	for _, count := range histogram {
		sampleCount += count
	}
	return AnalyticsPercentiles{
		P50Ms:       histogramPercentile(histogram, sampleCount, 0.50),
		P90Ms:       histogramPercentile(histogram, sampleCount, 0.90),
		P99Ms:       histogramPercentile(histogram, sampleCount, 0.99),
		SampleCount: sampleCount,
	}
}

func histogramPercentile(histogram map[int64]int64, sampleCount int64, percentile float64) int64 {
	if sampleCount == 0 {
		return 0
	}
	upperBounds := make([]int64, 0, len(histogram))
	for upperBoundMs := range histogram {
		upperBounds = append(upperBounds, upperBoundMs)
	}
	sort.Slice(upperBounds, func(i, j int) bool { return upperBounds[i] < upperBounds[j] })
	rank := int64(math.Ceil(float64(sampleCount) * percentile))
	var cumulative int64
	for _, upperBoundMs := range upperBounds {
		cumulative += histogram[upperBoundMs]
		if cumulative >= rank {
			return upperBoundMs
		}
	}
	return upperBounds[len(upperBounds)-1]
}

func buildAnalyticsUsage(a *analyticsAccumulator) AnalyticsUsage {
	result := AnalyticsUsage{
		CacheReadTokens:     a.cacheReadTokens,
		RequestBodySamples:  a.requestBodySamples,
		RequestBodyBytes:    a.requestBodyBytes,
		RequestBodyMaxBytes: a.requestBodyMaxBytes,
		CacheReadSamples:    a.cacheReadSamples,
		CacheWriteTokens:    a.cacheWriteTokens,
		CacheWriteSamples:   a.cacheWriteSamples,
		CacheWrite5mTokens:  a.cacheWrite5mTokens,
		CacheWrite1hTokens:  a.cacheWrite1hTokens,
	}
	if a.requestBodySamples > 0 {
		average := float64(a.requestBodyBytes) / float64(a.requestBodySamples)
		result.RequestBodyAvgBytes = &average
	}
	return result
}

// Legacy buckets have percentile counts but no recorded sums. Only expose an
// average when every sample in the requested bucket has a measured sum.
func analyticsLatencyPercentiles(histogram map[int64]int64, sumMs, averageCount, weight int64) AnalyticsPercentiles {
	result := histogramPercentiles(histogram)
	if weight > 0 && averageCount > 0 && averageCount == result.SampleCount {
		average := float64(sumMs) / float64(weight)
		result.AvgMs = &average
	}
	return result
}
