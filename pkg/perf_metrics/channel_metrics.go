package perfmetrics

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Channel filters use the existing bounded usage-log reader: the historical
// performance buckets do not contain a channel dimension. Missing facts stay
// missing; upstream transport bytes are not application request-body bytes.
type ChannelMetricsResult struct {
	AnalyticsResult
	Source  string                `json:"source"`
	Summary ChannelMetricsSummary `json:"summary"`
	Series  []ChannelMetricsPoint `json:"series"`
}

type ChannelMetricsSummary struct {
	MetricPercentiles map[string]ObservedValuePercentiles `json:"metric_percentiles"`
	AnalyticsSummary
	Ttft ObservedLatencyPercentiles `json:"ttft"`
	Tpot ObservedLatencyPercentiles `json:"tpot"`
}

type ChannelMetricsPoint struct {
	MetricPercentiles map[string]ObservedValuePercentiles `json:"metric_percentiles"`
	AnalyticsPoint
	Ttft ObservedLatencyPercentiles `json:"ttft"`
	Tpot ObservedLatencyPercentiles `json:"tpot"`
}

type ObservedLatencyPercentiles struct {
	AvgMs       *float64 `json:"avg_ms"`
	P50Ms       float64  `json:"p50_ms"`
	P90Ms       float64  `json:"p90_ms"`
	P95Ms       float64  `json:"p95_ms"`
	P99Ms       float64  `json:"p99_ms"`
	MaxMs       *float64 `json:"max_ms"`
	SampleCount int      `json:"sample_count"`
}

type channelMetricsBucket struct {
	cacheHitRates    []float64
	cacheWriteValues []float64
	generationMs     int64
	generationTokens int64
	usage            *analyticsAccumulator
	ttft             []float64
	tpot             []float64
}

func QueryChannelMetrics(channelID int, params AnalyticsQueryParams) (ChannelMetricsResult, error) {
	if channelID <= 0 || params.Model == "" || params.StartTs <= 0 || params.EndTs <= params.StartTs || params.EndTs-params.StartTs > 30*24*60*60 {
		return ChannelMetricsResult{}, errors.New("invalid channel metrics query")
	}
	if model.LOG_DB == nil {
		return ChannelMetricsResult{}, errors.New("log database unavailable")
	}
	ctx := params.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	bucketSeconds := analyticsBucketSeconds(params.StartTs, params.EndTs, 300)
	buckets := make(map[int64]*channelMetricsBucket)
	summary := &channelMetricsBucket{usage: newAnalyticsAccumulator()}
	groups := make(map[string]*ChannelErrorGroup)
	scanned, truncated, err := model.VisitPerfAnalyticsChannelLogs(ctx, channelID, params.UserId, params.Model, params.TokenId, params.StartTs, params.EndTs, func(row model.PerfChannelLog) {
		// Match the query's exact interval, including its partially filled buckets.
		if row.CreatedAt >= params.EndTs {
			return
		}
		ts := bucketStartAt(row.CreatedAt, bucketSeconds)
		bucket := buckets[ts]
		if bucket == nil {
			bucket = &channelMetricsBucket{usage: newAnalyticsAccumulator()}
			buckets[ts] = bucket
		}
		bucket.usage.requestCount++
		if row.Type == model.LogTypeError {
			bucket.usage.errorCount++
			accumulateChannelError(groups, row)
			return
		}
		bucket.usage.successCount++
		bucket.usage.inputTokens += max(row.PromptTokens, 0)
		bucket.usage.totalTokens += max(row.PromptTokens, 0) + max(row.CompletionTokens, 0)

		var other struct {
			Frt          *float64 `json:"frt"`
			CacheRead    *int64   `json:"cache_tokens"`
			CacheWrite   *int64   `json:"cache_creation_tokens"`
			CacheWrite5m *int64   `json:"cache_creation_tokens_5m"`
			CacheWrite1h *int64   `json:"cache_creation_tokens_1h"`
			AdminInfo    struct {
				Timing common.RequestTimingSnapshot `json:"request_timing"`
			} `json:"admin_info"`
		}
		if common.Unmarshal([]byte(row.Other), &other) != nil {
			return
		}
		if other.CacheRead != nil && *other.CacheRead >= 0 {
			bucket.usage.cacheReadSamples++
			bucket.usage.cacheReadTokens += *other.CacheRead
			if row.PromptTokens > 0 {
				bucket.cacheHitRates = append(bucket.cacheHitRates, float64(min(*other.CacheRead, row.PromptTokens))*100/float64(row.PromptTokens))
			}
		}
		if other.CacheWrite != nil && *other.CacheWrite >= 0 {
			bucket.usage.cacheWriteSamples++
			bucket.usage.cacheWriteTokens += *other.CacheWrite
			bucket.cacheWriteValues = append(bucket.cacheWriteValues, float64(*other.CacheWrite))
		} else if (other.CacheWrite5m != nil && *other.CacheWrite5m >= 0) || (other.CacheWrite1h != nil && *other.CacheWrite1h >= 0) {
			bucket.usage.cacheWriteSamples++
			var writeTokens int64
			if other.CacheWrite5m != nil {
				writeTokens += max(*other.CacheWrite5m, 0)
			}
			if other.CacheWrite1h != nil {
				writeTokens += max(*other.CacheWrite1h, 0)
			}
			bucket.usage.cacheWriteTokens += writeTokens
			bucket.cacheWriteValues = append(bucket.cacheWriteValues, float64(writeTokens))
		}
		if other.CacheWrite5m != nil {
			bucket.usage.cacheWrite5mTokens += max(*other.CacheWrite5m, 0)
		}
		if other.CacheWrite1h != nil {
			bucket.usage.cacheWrite1hTokens += max(*other.CacheWrite1h, 0)
		}
		if !row.IsStream {
			return
		}
		if other.Frt != nil && *other.Frt >= 0 {
			bucket.ttft = append(bucket.ttft, *other.Frt)
		}
		timing := other.AdminInfo.Timing
		if row.CompletionTokens > 1 && timing.FirstResponseAtMs > 0 && timing.RequestCompletedAtMs >= timing.FirstResponseAtMs {
			// Keep fractional milliseconds; integer use_time cannot reconstruct TPOT.
			bucket.generationMs += timing.RequestCompletedAtMs - timing.FirstResponseAtMs
			bucket.generationTokens += row.CompletionTokens - 1
			bucket.tpot = append(bucket.tpot, float64(timing.RequestCompletedAtMs-timing.FirstResponseAtMs)/float64(row.CompletionTokens-1))
		}
	})
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return ChannelMetricsResult{}, err
	}

	timestamps := make([]int64, 0, len(buckets))
	for ts := range buckets {
		timestamps = append(timestamps, ts)
	}
	slices.Sort(timestamps)
	points := make([]ChannelMetricsPoint, 0, len(timestamps))
	for _, ts := range timestamps {
		bucket := buckets[ts]
		mergeAnalyticsAccumulator(summary.usage, bucket.usage)
		summary.ttft = append(summary.ttft, bucket.ttft...)
		summary.tpot = append(summary.tpot, bucket.tpot...)
		summary.cacheHitRates = append(summary.cacheHitRates, bucket.cacheHitRates...)
		summary.cacheWriteValues = append(summary.cacheWriteValues, bucket.cacheWriteValues...)
		summary.generationMs += bucket.generationMs
		summary.generationTokens += bucket.generationTokens
		seconds := min(ts+bucketSeconds, params.EndTs) - max(ts, params.StartTs)
		points = append(points, ChannelMetricsPoint{
			AnalyticsPoint:    buildAnalyticsPoint(max(ts, params.StartTs), seconds, bucket.usage),
			MetricPercentiles: bucket.metricPercentiles(),
			Ttft:              observedLatencyPercentiles(bucket.ttft), Tpot: bucket.tpotPercentiles(),
		})
	}
	return ChannelMetricsResult{
		AnalyticsResult: AnalyticsResult{
			ModelName: params.Model, ScannedLogs: scanned, Truncated: truncated,
			ErrorGroups: channelErrorResults(groups), EffectiveStartTs: params.StartTs, EffectiveEndTs: params.EndTs,
		},
		Source: "usage_logs",
		Summary: ChannelMetricsSummary{
			AnalyticsSummary:  buildAnalyticsSummary(params.EndTs-params.StartTs, summary.usage),
			MetricPercentiles: summary.metricPercentiles(),
			Ttft:              observedLatencyPercentiles(summary.ttft), Tpot: summary.tpotPercentiles(),
		},
		Series: points,
	}, nil
}

type ObservedValuePercentiles struct {
	P50         float64  `json:"p50"`
	P90         float64  `json:"p90"`
	P95         float64  `json:"p95"`
	P99         float64  `json:"p99"`
	Max         *float64 `json:"max"`
	SampleCount int      `json:"sample_count"`
}

func observedValuePercentiles(values []float64) ObservedValuePercentiles {
	if len(values) == 0 {
		return ObservedValuePercentiles{}
	}
	slices.Sort(values)
	maximum := values[len(values)-1]
	return ObservedValuePercentiles{
		P50: values[int(math.Ceil(float64(len(values))*.50))-1],
		P90: values[int(math.Ceil(float64(len(values))*.90))-1],
		P95: values[int(math.Ceil(float64(len(values))*.95))-1],
		P99: values[int(math.Ceil(float64(len(values))*.99))-1],
		Max: &maximum, SampleCount: len(values),
	}
}

func observedLatencyPercentiles(values []float64) ObservedLatencyPercentiles {
	percentiles := observedValuePercentiles(values)
	result := ObservedLatencyPercentiles{P50Ms: percentiles.P50, P90Ms: percentiles.P90, P95Ms: percentiles.P95, P99Ms: percentiles.P99, MaxMs: percentiles.Max, SampleCount: percentiles.SampleCount}
	if len(values) > 0 {
		var sum float64
		for _, value := range values {
			sum += value
		}
		average := sum / float64(len(values))
		result.AvgMs = &average
	}
	return result
}

func (bucket *channelMetricsBucket) metricPercentiles() map[string]ObservedValuePercentiles {
	return map[string]ObservedValuePercentiles{
		"cache_hit_rate":     observedValuePercentiles(bucket.cacheHitRates),
		"cache_write_tokens": observedValuePercentiles(bucket.cacheWriteValues),
	}
}

func (bucket *channelMetricsBucket) tpotPercentiles() ObservedLatencyPercentiles {
	result := observedLatencyPercentiles(bucket.tpot)
	if bucket.generationTokens > 0 {
		average := float64(bucket.generationMs) / float64(bucket.generationTokens)
		result.AvgMs = &average
	}
	return result
}
