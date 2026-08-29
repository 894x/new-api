package perfmetrics

import (
	"errors"
	"math"
	"sort"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

const (
	metricTtft = "ttft"
	metricTpot = "tpot"
)

var histogramUpperBoundsMs = [...]int64{
	5, 10, 20, 40, 80, 100, 160, 250, 500, 750, 1000, 1500, 2000,
	3000, 5000, 7500, 10000, 15000, 20000, 30000, 60000, 120000,
	300000, 600000,
}

type AnalyticsQueryParams struct {
	Model   string
	UserId  int
	TokenId int
	StartTs int64
	EndTs   int64
}

type AnalyticsPercentiles struct {
	P50Ms       int64 `json:"p50_ms"`
	P90Ms       int64 `json:"p90_ms"`
	P99Ms       int64 `json:"p99_ms"`
	SampleCount int64 `json:"sample_count"`
}

type AnalyticsSummary struct {
	RequestCount int64                `json:"request_count"`
	SuccessRate  float64              `json:"success_rate"`
	Rpm          float64              `json:"rpm"`
	Tpm          float64              `json:"tpm"`
	CacheHitRate float64              `json:"cache_hit_rate"`
	Ttft         AnalyticsPercentiles `json:"ttft"`
	Tpot         AnalyticsPercentiles `json:"tpot"`
}

type AnalyticsPoint struct {
	Ts           int64                `json:"ts"`
	RequestCount int64                `json:"request_count"`
	SuccessRate  float64              `json:"success_rate"`
	Rpm          float64              `json:"rpm"`
	Tpm          float64              `json:"tpm"`
	CacheHitRate float64              `json:"cache_hit_rate"`
	Ttft         AnalyticsPercentiles `json:"ttft"`
	Tpot         AnalyticsPercentiles `json:"tpot"`
}

type AnalyticsResult struct {
	ModelName        string           `json:"model_name"`
	EffectiveStartTs int64            `json:"effective_start_timestamp"`
	EffectiveEndTs   int64            `json:"effective_end_timestamp"`
	Summary          AnalyticsSummary `json:"summary"`
	Series           []AnalyticsPoint `json:"series"`
}

type detailBucketKey struct {
	model    string
	userId   int
	tokenId  int
	bucketTs int64
}

type detailCounters struct {
	requestCount    int64
	successCount    int64
	inputTokens     int64
	outputTokens    int64
	totalTokens     int64
	cacheReadTokens int64
	ttftCount       int64
	tpotCount       int64
	ttftBuckets     [len(histogramUpperBoundsMs)]int64
	tpotBuckets     [len(histogramUpperBoundsMs)]int64
}

type analyticsAccumulator struct {
	requestCount    int64
	successCount    int64
	inputTokens     int64
	totalTokens     int64
	cacheReadTokens int64
	ttft            map[int64]int64
	tpot            map[int64]int64
}

func QueryAnalytics(params AnalyticsQueryParams) (AnalyticsResult, error) {
	if params.Model == "" || params.StartTs <= 0 || params.EndTs <= params.StartTs {
		return AnalyticsResult{}, errors.New("invalid performance analytics query")
	}
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
	details, histograms, err := model.GetPerfAnalyticsBuckets(filter)
	if err != nil {
		return AnalyticsResult{}, err
	}

	series := make(map[int64]*analyticsAccumulator)
	for _, detail := range details {
		point := analyticsPointAccumulator(series, detail.BucketTs)
		point.requestCount += detail.RequestCount
		point.successCount += detail.SuccessCount
		point.inputTokens += detail.InputTokens
		point.totalTokens += detail.TotalTokens
		point.cacheReadTokens += detail.CacheReadTokens
	}
	for _, histogram := range histograms {
		point := analyticsPointAccumulator(series, histogram.BucketTs)
		addHistogramCount(point, histogram.Metric, histogram.UpperBoundMs, histogram.Count)
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
		ModelName:        params.Model,
		EffectiveStartTs: params.StartTs,
		EffectiveEndTs:   effectiveEndTs,
		Summary:          buildAnalyticsSummary(effectiveDurationSeconds, summary),
		Series:           points,
	}, nil
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
	}
}

func mergeAnalyticsAccumulator(target *analyticsAccumulator, source *analyticsAccumulator) {
	target.requestCount += source.requestCount
	target.successCount += source.successCount
	target.inputTokens += source.inputTokens
	target.totalTokens += source.totalTokens
	target.cacheReadTokens += source.cacheReadTokens
	for upperBoundMs, count := range source.ttft {
		target.ttft[upperBoundMs] += count
	}
	for upperBoundMs, count := range source.tpot {
		target.tpot[upperBoundMs] += count
	}
}

func buildAnalyticsPoint(ts int64, bucketSeconds int64, accumulator *analyticsAccumulator) AnalyticsPoint {
	return AnalyticsPoint{
		Ts:           ts,
		RequestCount: accumulator.requestCount,
		SuccessRate:  analyticsSuccessRate(accumulator),
		Rpm:          analyticsPerMinute(accumulator.requestCount, bucketSeconds),
		Tpm:          analyticsPerMinute(accumulator.totalTokens, bucketSeconds),
		CacheHitRate: analyticsCacheHitRate(accumulator),
		Ttft:         histogramPercentiles(accumulator.ttft),
		Tpot:         histogramPercentiles(accumulator.tpot),
	}
}

func buildAnalyticsSummary(durationSeconds int64, accumulator *analyticsAccumulator) AnalyticsSummary {
	return AnalyticsSummary{
		RequestCount: accumulator.requestCount,
		SuccessRate:  analyticsSuccessRate(accumulator),
		Rpm:          analyticsPerMinute(accumulator.requestCount, durationSeconds),
		Tpm:          analyticsPerMinute(accumulator.totalTokens, durationSeconds),
		CacheHitRate: analyticsCacheHitRate(accumulator),
		Ttft:         histogramPercentiles(accumulator.ttft),
		Tpot:         histogramPercentiles(accumulator.tpot),
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
