package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type channelCacheSample struct {
	id, at      int64
	channel     int
	bytes       int
	input, read int64
	valid       bool
}

type channelCacheTokens struct {
	input, read float64
	valid       bool
}

type channelCacheInterval struct {
	stamp  channelWindowStamp
	totals map[int]channelCacheTokens
	exact  bool
}

type channelCacheWindow struct {
	initialized  bool
	samples      []channelCacheSample // Original descending-ID prefix, without bodies.
	guard        *channelCacheSample  // First excluded log proves truncation.
	intervals    map[int64]*channelCacheInterval
	stamps       map[int64]channelWindowStamp
	cursor       int64
	minAt, maxAt int64
}

type channelCacheWindowCache struct {
	channelWindowCache[channelCacheWindow]
}

var channelCacheWindows channelCacheWindowCache

// Below 2^53 all positive integer token sums are exact in float64, independent
// of merge order. Larger totals retain the original ordered calculation.
const channelCacheExactTokens = float64(1 << 53)

func (window *channelCacheWindow) replace(samples []channelCacheSample, guard *channelCacheSample) {
	window.minAt, window.maxAt = 0, 0
	if len(samples) > 0 {
		window.minAt, window.maxAt = samples[0].at, samples[0].at
	}
	stamps := make(map[int64]channelWindowStamp)
	for _, sample := range samples {
		window.minAt = min(window.minAt, sample.at)
		window.maxAt = max(window.maxAt, sample.at)
		at := sample.at / channelWindowSeconds * channelWindowSeconds
		stamp := stamps[at]
		stamp.BucketAt, stamp.Records = at, stamp.Records+1
		stamp.LastID = max(stamp.LastID, sample.id)
		stamps[at] = stamp
	}
	parts := make(map[int64]*channelCacheInterval, len(stamps))
	for at, stamp := range stamps {
		if part := window.intervals[at]; part != nil && part.stamp == stamp {
			parts[at] = part
		} else {
			parts[at] = &channelCacheInterval{stamp: stamp, totals: make(map[int]channelCacheTokens), exact: true}
		}
	}
	for _, sample := range samples {
		at := sample.at / channelWindowSeconds * channelWindowSeconds
		part := parts[at]
		if part == window.intervals[at] {
			continue
		}
		total := part.totals[sample.channel]
		if sample.input > 1<<53 || total.input > channelCacheExactTokens-float64(sample.input) {
			part.exact = false
		}
		total.input += float64(sample.input)
		total.read += float64(sample.read)
		total.valid = total.valid || sample.valid
		part.totals[sample.channel] = total
	}
	if guard != nil {
		at := guard.at / channelWindowSeconds * channelWindowSeconds
		stamp := stamps[at]
		stamp.BucketAt, stamp.Records = at, stamp.Records+1
		stamp.LastID = max(stamp.LastID, guard.id)
		stamps[at] = stamp
	}
	window.initialized, window.samples, window.guard = true, samples, guard
	window.intervals, window.stamps, window.cursor = parts, stamps, 0
	if len(samples) > 0 {
		window.cursor = samples[0].id
	} else if guard != nil {
		window.cursor = guard.id
	}
}

// A read retains exactly the same 50,000-row / 32 MiB prefix as visitPerfLogs.
// The excluded sentinel is metadata only; its tokens never enter the totals.
func readChannelCacheWindow(ctx context.Context, ids []int, modelName string, start, end, after int64) ([]channelCacheSample, *channelCacheSample, error) {
	var samples []channelCacheSample
	var guard *channelCacheSample
	_, _, err := visitPerfLogs(ctx, perfLogFilter{
		ChannelIds: ids, ModelName: modelName, ConsumeOnly: true,
		StartTs: start, EndTs: end, AfterID: after,
		OnTruncated: func(row PerfChannelLog) {
			guard = &channelCacheSample{id: row.Id, at: row.CreatedAt, bytes: len(row.Other)}
		},
	}, func(row PerfChannelLog) {
		sample := channelCacheSample{id: row.Id, at: row.CreatedAt, channel: row.ChannelId, bytes: len(row.Other)}
		if row.PromptTokens > 0 {
			sample.input = row.PromptTokens
			var other struct {
				CacheRead *int64 `json:"cache_tokens"`
			}
			if common.UnmarshalJsonStr(row.Other, &other) == nil && other.CacheRead != nil && *other.CacheRead >= 0 {
				sample.valid = true
				sample.read = min(*other.CacheRead, row.PromptTokens)
			}
		}
		samples = append(samples, sample)
	})
	return samples, guard, err
}

func (cache *channelCacheWindowCache) stats(ctx context.Context, channelIDs []int, modelName string, startTs, endTs int64) (map[int]*ChannelCacheStats, error) {
	result := make(map[int]*ChannelCacheStats, len(channelIDs))
	for _, id := range channelIDs {
		result[id] = &ChannelCacheStats{}
	}
	if len(channelIDs) == 0 {
		return result, nil
	}
	if LOG_DB == nil {
		return nil, errors.New("log database unavailable")
	}
	// Preserve the existing ClickHouse log identity/storage path and arbitrary
	// historical reports. The incremental cache serves the live <=24h window.
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) || endTs < startTs || endTs-startTs > 86400 {
		return queryChannelCacheStatsUncached(ctx, channelIDs, modelName, startTs, endTs)
	}
	ids := slices.Clone(channelIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	entry := cache.entry(channelWindowKey{LOG_DB, modelName, fmt.Sprint(ids)})
	entry.mu.Lock()
	defer entry.mu.Unlock()
	window := &entry.data
	rebuild := !window.initialized
	if !rebuild {
		// Preserve exact inclusive seconds at the edge, rather than dropping an
		// entire quarter-hour. A truncated prefix needs refilling when it expires.
		if len(window.samples) > 0 && (window.minAt < startTs || window.maxAt > endTs) {
			if window.guard != nil {
				rebuild = true
			} else {
				kept := make([]channelCacheSample, 0, len(window.samples))
				for _, sample := range window.samples {
					if sample.at >= startTs && sample.at <= endTs {
						kept = append(kept, sample)
					}
				}
				window.replace(kept, nil)
			}
		}
		if window.guard != nil && (window.guard.at < startTs || window.guard.at > endTs) {
			rebuild = true
		}
	}
	if !rebuild {
		query := LOG_DB.WithContext(ctx).Model(&Log{}).
			Where("channel_id IN ? AND type = ? AND created_at >= ? AND created_at <= ?", ids, LogTypeConsume, startTs, endTs)
		if modelName != "" {
			query = query.Where("model_name = ?", modelName)
		}
		// Older IDs cannot affect an already truncated prefix. Include its
		// sentinel so deletion/expiry of the first excluded row causes a refill.
		if window.guard != nil {
			query = query.Where("id >= ?", window.guard.id)
		}
		var stamps []channelWindowStamp
		if err := query.Session(&gorm.Session{}).
			Select(channelWindowBucketSQL + " AS bucket_at, COUNT(*) AS records, MAX(id) AS last_id").
			Group(channelWindowBucketSQL).Scan(&stamps).Error; err != nil {
			return nil, err
		}
		current := make(map[int64]channelWindowStamp, len(stamps))
		for _, stamp := range stamps {
			current[stamp.BucketAt] = stamp
		}
		unchanged := len(current) == len(window.stamps)
		for at, stamp := range current {
			unchanged = unchanged && window.stamps[at] == stamp
		}
		if !unchanged {
			added, guard, err := readChannelCacheWindow(ctx, ids, modelName, startTs, endTs, window.cursor)
			if err != nil {
				entry.data = channelCacheWindow{}
				return nil, err
			}
			if guard != nil {
				// Newer logs alone fill the prefix; no older log can contribute.
				window.replace(added, guard)
			} else {
				expected := make(map[int64]channelWindowStamp, len(window.stamps))
				for at, stamp := range window.stamps {
					expected[at] = stamp
				}
				for _, sample := range added {
					at := sample.at / channelWindowSeconds * channelWindowSeconds
					stamp := expected[at]
					stamp.BucketAt, stamp.Records = at, stamp.Records+1
					stamp.LastID = max(stamp.LastID, sample.id)
					expected[at] = stamp
				}
				appendOnly := len(current) == len(expected)
				for at, stamp := range current {
					appendOnly = appendOnly && expected[at] == stamp
				}
				if !appendOnly {
					// Includes deletions and a lower-ID transaction committing late.
					rebuild = true
				} else {
					combined := append(added, window.samples...)
					bytes, count := 0, 0
					for _, sample := range combined {
						if count >= 50000 || bytes+sample.bytes > 32<<20 {
							break
						}
						bytes += sample.bytes
						count++
					}
					guard := window.guard
					if count < len(combined) {
						excluded := combined[count]
						guard = &excluded
					}
					window.replace(combined[:count], guard)
				}
			}
		}
	}
	if rebuild {
		samples, guard, err := readChannelCacheWindow(ctx, ids, modelName, startTs, endTs, 0)
		if err != nil {
			entry.data = channelCacheWindow{}
			return nil, err
		}
		window.replace(samples, guard)
	}
	totals := make(map[int]channelCacheTokens, len(ids))
	for _, part := range window.intervals {
		if !part.exact {
			entry.data = channelCacheWindow{}
			return queryChannelCacheStatsUncached(ctx, channelIDs, modelName, startTs, endTs)
		}
		for channel, tokens := range part.totals {
			total := totals[channel]
			if total.input > channelCacheExactTokens-tokens.input {
				entry.data = channelCacheWindow{}
				return queryChannelCacheStatsUncached(ctx, channelIDs, modelName, startTs, endTs)
			}
			total.input += tokens.input
			total.read += tokens.read
			total.valid = total.valid || tokens.valid
			totals[channel] = total
		}
	}
	for id, stat := range result {
		stat.Truncated = window.guard != nil
		if total := totals[id]; total.valid && total.input > 0 {
			rate := math.Round(total.read/total.input*10000) / 100
			stat.HitRate = &rate
		}
	}
	return result, nil
}
