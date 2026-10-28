package perfmetrics

import (
	"math"
	"math/bits"
)

const (
	metricBody       = "body"
	metricCacheWrite = "cwrite"
	metricCacheHit   = "chit"
)

// Request distributions share the existing histogram store.
type detailHistogramKey struct {
	metric     string
	upperBound int64
}

type histogramObservation struct {
	count      int64
	maxValue   int64
	maxSamples int64
}

func (bucket *atomicDetailBucket) observe(metric string, value int64) {
	upperBound := distributionUpperBound(value)
	if metric == metricTtft || metric == metricTpot {
		for _, bound := range histogramUpperBoundsMs {
			if value <= bound {
				upperBound = bound
				break
			}
		}
	}
	key := detailHistogramKey{metric: metric, upperBound: upperBound}
	observation := bucket.histograms[key]
	observation.count++
	observation.maxSamples++
	observation.maxValue = max(observation.maxValue, value)
	bucket.histograms[key] = observation
}

// Four significant bits bound histogram cardinality, including long tails.
// Maxima are retained separately and never replaced with bucket boundaries.
func distributionUpperBound(value int64) int64 {
	if value < 16 {
		return value
	}
	step := int64(1) << (bits.Len64(uint64(value)) - 4)
	if value > math.MaxInt64-step+1 {
		return math.MaxInt64
	}
	return ((value + step - 1) / step) * step
}

func mergeHistogramObservations(target map[detailHistogramKey]histogramObservation, source map[detailHistogramKey]histogramObservation) {
	for key, value := range source {
		observation := target[key]
		observation.count += value.count
		observation.maxSamples += value.maxSamples
		observation.maxValue = max(observation.maxValue, value.maxValue)
		target[key] = observation
	}
}

type AnalyticsValuePercentiles struct {
	P50         float64  `json:"p50"`
	P90         float64  `json:"p90"`
	P95         float64  `json:"p95"`
	P99         float64  `json:"p99"`
	Max         *float64 `json:"max"`
	SampleCount int64    `json:"sample_count"`
}

func valuePercentiles(histogram map[int64]histogramObservation, scale float64) AnalyticsValuePercentiles {
	counts := make(map[int64]int64, len(histogram))
	var count, maxSamples, maximum int64
	for bound, observation := range histogram {
		counts[bound] = observation.count
		count += observation.count
		maxSamples += observation.maxSamples
		maximum = max(maximum, observation.maxValue)
	}
	result := AnalyticsValuePercentiles{SampleCount: count}
	if count == 0 {
		return result
	}
	if maxSamples == count {
		value := float64(maximum) / scale
		result.Max = &value
	}
	values := []*float64{&result.P50, &result.P90, &result.P95, &result.P99}
	for i, percentile := range []float64{0.50, 0.90, 0.95, 0.99} {
		value := float64(histogramPercentile(counts, count, percentile)) / scale
		if result.Max != nil {
			value = min(value, *result.Max)
		}
		*values[i] = value
	}
	return result
}

func buildMetricPercentiles(a *analyticsAccumulator) map[string]AnalyticsValuePercentiles {
	return map[string]AnalyticsValuePercentiles{
		"request_body_avg_bytes": valuePercentiles(a.histograms[metricBody], 1),
		"cache_write_tokens":     valuePercentiles(a.histograms[metricCacheWrite], 1),
		"cache_hit_rate":         valuePercentiles(a.histograms[metricCacheHit], 100),
	}
}
