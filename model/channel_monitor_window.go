package model

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

const channelWindowSeconds = int64(15 * 60)

// Revisions are checked in the database, rather than invalidated only in this
// process: another gateway can flush or retry an older cumulative traffic bucket.
const channelWindowBucketSQL = "created_at - (created_at % 900)"

type channelWindowKey struct {
	db       *gorm.DB
	model    string
	channels string
}

type channelWindowEntry[T any] struct {
	mu   sync.Mutex
	used uint64
	data T
}

// Keep only a few actively viewed windows. Each domain additionally bounds the
// retained payload; no credentials or complete consume-log bodies are cached.
type channelWindowCache[T any] struct {
	mu      sync.Mutex
	clock   uint64
	entries map[channelWindowKey]*channelWindowEntry[T]
}

func (cache *channelWindowCache[T]) entry(key channelWindowKey) *channelWindowEntry[T] {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.clock++
	if entry := cache.entries[key]; entry != nil {
		entry.used = cache.clock
		return entry
	}
	if cache.entries == nil {
		cache.entries = make(map[channelWindowKey]*channelWindowEntry[T])
	}
	if len(cache.entries) >= 4 {
		var oldest channelWindowKey
		var age uint64 = ^uint64(0)
		for candidate, entry := range cache.entries {
			if entry.used < age {
				oldest, age = candidate, entry.used
			}
		}
		delete(cache.entries, oldest)
	}
	entry := &channelWindowEntry[T]{used: cache.clock}
	cache.entries[key] = entry
	return entry
}

type channelWindowStamp struct {
	BucketAt int64
	Records  int64
	LastID   int64
	Revision int64
	Samples  int64
	Observed int64
}

type channelMonitorInterval struct {
	stamp   channelWindowStamp
	records []ChannelMonitorRecord
	stats   map[int]*channelMonitorContribution
	bytes   int
}

type channelMonitorContribution struct {
	registered  bool
	latest      []ChannelMonitorRecord
	buckets     map[int64]*channelMonitorIntervalStats
	bucketOrder []int64
}

type channelMonitorIntervalStats struct {
	bucket     ChannelMonitorBucket
	timings    ChannelMonitorTimings
	incomplete bool
	measured   int
}

type channelMonitorWindow struct {
	policy    string
	intervals map[int64]*channelMonitorInterval
}

type channelMonitorWindowCache struct {
	channelWindowCache[channelMonitorWindow]
}

var channelMonitorWindows channelMonitorWindowCache

func (cache *channelMonitorWindowCache) summaries(ctx context.Context, channels []*Channel, modelName string, enabled bool, interval time.Duration, excluded []operation_setting.StatusCodeRange, now time.Time) ([]ChannelMonitorSummary, error) {
	ids := make([]int, 0, len(channels))
	byID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
		byID[channel.Id] = channel
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	entry := cache.entry(channelWindowKey{DB, modelName, fmt.Sprint(ids)})
	entry.mu.Lock()
	defer entry.mu.Unlock()
	window := &entry.data
	end := now.Unix()
	start := end/channelWindowSeconds*channelWindowSeconds + channelWindowSeconds - 24*60*60
	query := DB.WithContext(ctx).Model(&ChannelMonitorRecord{}).
		Where("channel_id IN ? AND model = ? AND created_at >= ? AND created_at <= ?", ids, modelName, start, end)
	var stamps []channelWindowStamp
	if len(ids) > 0 {
		if err := query.Session(&gorm.Session{}).
			Select(channelWindowBucketSQL + " AS bucket_at, COUNT(*) AS records, MAX(id) AS last_id, COALESCE(SUM(window_revision), 0) AS revision, COALESCE(SUM(count), 0) AS samples, COALESCE(SUM(observed_at_ms), 0) AS observed").
			Group(channelWindowBucketSQL).Scan(&stamps).Error; err != nil {
			return nil, err
		}
	}
	var total int64
	live := make(map[int64]bool, len(stamps))
	changed := make([]int64, 0, len(stamps))
	for _, stamp := range stamps {
		total += stamp.Records
		live[stamp.BucketAt] = true
		cached := window.intervals[stamp.BucketAt]
		if cached == nil || cached.stamp != stamp {
			changed = append(changed, stamp.BucketAt)
		}
	}
	// The original limit selects the newest records across the whole query, not
	// per channel or bucket. Preserve that selection and its truncation contract.
	if total > 100000 {
		entry.data = channelMonitorWindow{}
		return channelMonitorSummariesUncached(ctx, channels, modelName, enabled, interval, excluded, now)
	}
	if window.intervals == nil {
		window.intervals = make(map[int64]*channelMonitorInterval)
	}
	for at := range window.intervals {
		if !live[at] {
			delete(window.intervals, at)
		}
	}
	if len(changed) > 0 {
		var records []ChannelMonitorRecord
		if err := query.Session(&gorm.Session{}).Where(channelWindowBucketSQL+" IN ?", changed).
			Order("created_at DESC, observed_at_ms DESC, id DESC").Limit(100001).Find(&records).Error; err != nil {
			return nil, err
		}
		if len(records) > 100000 {
			entry.data = channelMonitorWindow{}
			return channelMonitorSummariesUncached(ctx, channels, modelName, enabled, interval, excluded, now)
		}
		for _, stamp := range stamps {
			if slices.Contains(changed, stamp.BucketAt) {
				window.intervals[stamp.BucketAt] = &channelMonitorInterval{stamp: stamp}
			}
		}
		for _, record := range records {
			at := record.CreatedAt / channelWindowSeconds * channelWindowSeconds
			window.intervals[at].records = append(window.intervals[at].records, record)
			window.intervals[at].bytes += 384 + len(record.TimingHistogram)
		}
	}
	// Runtime rules remain separate from raw facts. Changing exclusions or probe
	// registration rebuilds contributions from retained records, without SQL rereads.
	type registration struct {
		ID, Type       int
		Models, Config string
		TestModel      *string
	}
	registrations := make([]registration, 0, len(ids))
	for _, id := range ids {
		channel := byID[id]
		registrations = append(registrations, registration{id, channel.Type, channel.Models, channel.MonitorConfig, channel.TestModel})
	}
	policy, err := common.Marshal(struct {
		Channels []registration
		Excluded []operation_setting.StatusCodeRange
	}{registrations, excluded})
	if err != nil {
		return nil, err
	}
	var retained int
	for _, part := range window.intervals {
		if part.stats == nil || window.policy != string(policy) {
			part.stats = compileChannelMonitorInterval(part.records, byID, modelName, excluded)
		}
		retained += part.bytes
	}
	window.policy = string(policy)
	var gaps int64
	if err := DB.WithContext(ctx).Model(&ChannelMonitorRecord{}).
		Where("source = ? AND created_at >= ? AND created_at <= ?", "gap", start, end).Limit(1).Count(&gaps).Error; err != nil {
		return nil, err
	}
	summaries := window.summaries(channels, modelName, enabled, interval, start, end, gaps > 0)
	if retained > 4<<20 {
		entry.data = channelMonitorWindow{}
	}
	return summaries, nil
}

func compileChannelMonitorInterval(records []ChannelMonitorRecord, channels map[int]*Channel, modelName string, excluded []operation_setting.StatusCodeRange) map[int]*channelMonitorContribution {
	result := make(map[int]*channelMonitorContribution)
	protocols := make(map[int]map[string]bool)
	seen := make(map[int]map[string]bool)
	for id, channel := range channels {
		_, targets, _ := channel.MonitorTargets()
		protocols[id] = make(map[string]bool)
		seen[id] = make(map[string]bool)
		for _, target := range targets {
			if target.Model == modelName {
				protocols[id][channel.MonitoringEndpoint(target)] = true
			}
		}
	}
	for _, record := range records {
		channel := channels[record.ChannelID]
		if record.Source == "" {
			record.Source = "probe"
		}
		if record.Source == "probe" {
			record.EndpointType = channel.MonitoringEndpoint(ChannelMonitorTarget{Model: record.Model, EndpointType: record.EndpointType})
			if !protocols[record.ChannelID][record.EndpointType] {
				continue
			}
		} else if record.Source != "traffic" {
			continue
		}
		part := result[record.ChannelID]
		if part == nil {
			part = &channelMonitorContribution{buckets: make(map[int64]*channelMonitorIntervalStats)}
			result[record.ChannelID] = part
		}
		part.registered = part.registered || record.Source == "traffic"
		count := max(1, record.Count)
		record.Count = count
		if slices.ContainsFunc(excluded, func(code operation_setting.StatusCodeRange) bool {
			return record.StatusCode >= code.Start && record.StatusCode <= code.End
		}) {
			record.State, record.ErrorCode = "excluded", "excluded_http_status"
			record.TTFTMs, record.TokensPerSecond = nil, nil
		}
		if record.State == "healthy" || record.State == "slow" || record.State == "very_slow" {
			latency := record.LatencyMs
			if record.TTFTMs != nil {
				latency = *record.TTFTMs
			}
			record.State = ChannelMonitorLatencyState(latency)
		}
		if !seen[record.ChannelID][record.EndpointType] {
			part.latest = append(part.latest, record)
			seen[record.ChannelID][record.EndpointType] = true
		}
		at := record.CreatedAt / channelWindowSeconds * channelWindowSeconds
		if record.Source == "traffic" {
			at = record.BucketAt
		}
		stats := part.buckets[at]
		if stats == nil {
			stats = &channelMonitorIntervalStats{bucket: ChannelMonitorBucket{Timestamp: at, State: "empty", StateCounts: make(map[string]int), SourceCounts: make(map[string]int)}}
			part.buckets[at] = stats
			part.bucketOrder = append(part.bucketOrder, at)
		}
		bucket := &stats.bucket
		if bucket.Count == 0 || channelMonitorStateRank(record.State) > channelMonitorStateRank(bucket.State) {
			bucket.State, bucket.ErrorCode = record.State, record.ErrorCode
		}
		bucket.Count += count
		bucket.StateCounts[record.State] += count
		bucket.SourceCounts[record.Source] += count
		if record.State == "healthy" || record.State == "slow" || record.State == "very_slow" {
			var timing ChannelMonitorTimings
			if record.TimingHistogram != "" {
				if common.UnmarshalJsonStr(record.TimingHistogram, &timing) != nil || timing.Performance().DurationCount != count {
					stats.incomplete = true
				} else {
					stats.timings.Merge(timing)
				}
			} else if count == 1 {
				stats.timings.Add(record)
			} else {
				stats.incomplete = true
			}
			bucket.SuccessCount += count
			stats.measured += count
			if bucket.LatencyMs == nil {
				latency := record.LatencyMs
				bucket.LatencyMs = &latency
			}
		} else if record.State == "unavailable" {
			stats.measured += count
		}
	}
	return result
}

func (window *channelMonitorWindow) summaries(channels []*Channel, modelName string, enabled bool, interval time.Duration, start, end int64, truncated bool) []ChannelMonitorSummary {
	order := make([]int64, 0, len(window.intervals))
	for at := range window.intervals {
		order = append(order, at)
	}
	slices.Sort(order)
	slices.Reverse(order)
	result := make([]ChannelMonitorSummary, 0, len(channels))
	for _, channel := range channels {
		summary := ChannelMonitorSummary{ChannelID: channel.Id, State: "pending", Buckets: make([]ChannelMonitorBucket, 96), Truncated: truncated, SourceCounts: make(map[string]int)}
		monitor, targets, err := channel.MonitorTargets()
		summary.Enabled = monitor
		for _, target := range targets {
			summary.Registered = summary.Registered || target.Model == modelName
		}
		for i := range summary.Buckets {
			summary.Buckets[i] = ChannelMonitorBucket{Timestamp: start + int64(i)*channelWindowSeconds, State: "empty", StateCounts: make(map[string]int), SourceCounts: make(map[string]int)}
		}
		timings := make([]ChannelMonitorTimings, 96)
		incomplete := make([]bool, 96)
		protocols := make(map[string]bool)
		freshProtocol, rank := false, -1
		measured, succeeded := 0, 0
		for _, at := range order {
			part := window.intervals[at].stats[channel.Id]
			if part == nil {
				continue
			}
			summary.Registered = summary.Registered || part.registered
			for _, record := range part.latest {
				if protocols[record.EndpointType] {
					continue
				}
				protocols[record.EndpointType] = true
				if summary.Latest == nil {
					copy := record
					summary.Latest = &copy
				}
				fresh := end-record.CreatedAt <= int64(interval.Seconds()*2)+60
				freshProtocol = freshProtocol || fresh
				if next := channelMonitorStateRank(record.State); fresh && next > rank {
					rank, summary.State = next, record.State
					copy := record
					summary.Latest = &copy
				}
			}
			for _, bucketAt := range part.bucketOrder {
				stats := part.buckets[bucketAt]
				if bucketAt < start || bucketAt > end {
					continue
				}
				i := min(95, int((bucketAt-start)/channelWindowSeconds))
				source, bucket := &stats.bucket, &summary.Buckets[i]
				if bucket.Count == 0 || channelMonitorStateRank(source.State) > channelMonitorStateRank(bucket.State) {
					bucket.State, bucket.ErrorCode = source.State, source.ErrorCode
				}
				bucket.Count += source.Count
				bucket.SuccessCount += source.SuccessCount
				for state, count := range source.StateCounts {
					bucket.StateCounts[state] += count
				}
				for origin, count := range source.SourceCounts {
					bucket.SourceCounts[origin] += count
					summary.SourceCounts[origin] += count
				}
				if bucket.LatencyMs == nil && source.LatencyMs != nil {
					latency := *source.LatencyMs
					bucket.LatencyMs = &latency
				}
				timings[i].Merge(stats.timings)
				incomplete[i] = incomplete[i] || stats.incomplete
				summary.SampleCount += source.Count
				measured += stats.measured
				succeeded += source.SuccessCount
			}
		}
		for i := range summary.Buckets {
			if !truncated && !incomplete[i] {
				performance := timings[i].Performance()
				summary.Buckets[i].Performance = &performance
			}
		}
		if measured > 0 && !truncated {
			rate := float64(succeeded) * 100 / float64(measured)
			summary.SuccessRate = &rate
		}
		switch {
		case err != nil:
			summary.State = "configuration"
		case !monitor:
			summary.State = "disabled"
		case !summary.Registered:
			summary.State = "unregistered"
		case !enabled || channel.Status == common.ChannelStatusManuallyDisabled:
			summary.State = "paused"
		case summary.Latest != nil && !freshProtocol:
			summary.State = "stale"
		case summary.Latest == nil:
			summary.State = "pending"
		}
		// All maps and percentile values above belong to this response; callers
		// may attach cache statistics or serialize concurrently with the next poll.
		if summary.Latest != nil {
			if summary.Latest.TTFTMs != nil {
				ttft := *summary.Latest.TTFTMs
				summary.Latest.TTFTMs = &ttft
			}
			if summary.Latest.TokensPerSecond != nil {
				speed := *summary.Latest.TokensPerSecond
				summary.Latest.TokensPerSecond = &speed
			}
		}
		result = append(result, summary)
	}
	return result
}
