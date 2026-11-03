package model

import (
	"maps"
	"math/bits"
	"slices"
)

// ChannelMonitorTimings retains a mergeable distribution, not a median of
// per-flush medians. Integer milliseconds up to 128 are exact; larger values
// round upward by less than 1/64. The number of bins is bounded independently
// of request volume. Total duration and TTFT never share a distribution.
type ChannelMonitorTimings struct {
	Duration map[int64]int `json:"duration"`
	TTFT     map[int64]int `json:"ttft"`
}

func (timings *ChannelMonitorTimings) Add(record ChannelMonitorRecord) {
	if record.State != "healthy" && record.State != "slow" && record.State != "very_slow" {
		return
	}
	if timings.Duration == nil {
		timings.Duration = make(map[int64]int)
	}
	if timings.TTFT == nil {
		timings.TTFT = make(map[int64]int)
	}
	timings.Duration[channelMonitorTimingBound(record.LatencyMs)]++
	if record.TTFTMs != nil {
		timings.TTFT[channelMonitorTimingBound(*record.TTFTMs)]++
	}
}

func (timings ChannelMonitorTimings) Clone() ChannelMonitorTimings {
	return ChannelMonitorTimings{Duration: maps.Clone(timings.Duration), TTFT: maps.Clone(timings.TTFT)}
}

func (timings *ChannelMonitorTimings) Merge(other ChannelMonitorTimings) {
	if timings.Duration == nil {
		timings.Duration = make(map[int64]int)
		timings.TTFT = make(map[int64]int)
	}
	for bound, count := range other.Duration {
		timings.Duration[bound] += count
	}
	for bound, count := range other.TTFT {
		timings.TTFT[bound] += count
	}
}

type ChannelMonitorPerformance struct {
	DurationP50Ms *int64 `json:"duration_p50_ms"`
	TTFTP50Ms     *int64 `json:"ttft_p50_ms"`
	DurationCount int    `json:"duration_count"`
	TTFTCount     int    `json:"ttft_count"`
}

func (timings ChannelMonitorTimings) Performance() ChannelMonitorPerformance {
	duration, durationCount := channelMonitorTimingP50(timings.Duration)
	ttft, ttftCount := channelMonitorTimingP50(timings.TTFT)
	return ChannelMonitorPerformance{DurationP50Ms: duration, TTFTP50Ms: ttft, DurationCount: durationCount, TTFTCount: ttftCount}
}

func channelMonitorTimingBound(ms int64) int64 {
	ms = max(0, ms)
	width := int64(1) << max(0, bits.Len64(uint64(ms))-7)
	// Saturate the final bin rather than overflowing a signed millisecond value.
	const maximum = int64(1<<63 - 1)
	if ms > maximum-width+1 {
		return maximum
	}
	bound := (ms + width - 1) / width * width
	// Preserve the exact color boundaries even when a percentile is rounded.
	if ms < 5000 {
		return min(bound, 4999)
	}
	if ms < 10000 {
		return min(bound, 9999)
	}
	return bound
}

// ChannelMonitorLatencyState matches the usage log's first-token thresholds.
// A slow successful request remains distinct from an upstream failure.
func ChannelMonitorLatencyState(ms int64) string {
	if ms < 5000 {
		return "healthy"
	}
	if ms < 10000 {
		return "slow"
	}
	return "very_slow"
}

func channelMonitorTimingP50(histogram map[int64]int) (*int64, int) {
	bounds := make([]int64, 0, len(histogram))
	count := 0
	for bound, n := range histogram {
		if bound < 0 || n <= 0 {
			continue
		}
		bounds = append(bounds, bound)
		count += n
	}
	if count == 0 {
		return nil, 0
	}
	slices.Sort(bounds)
	rank := count/2 + count%2
	for _, bound := range bounds {
		rank -= histogram[bound]
		if rank <= 0 {
			return &bound, count
		}
	}
	return nil, count
}
