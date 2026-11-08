package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/chat_early_chunk_setting"
)

const (
	earlyChunkHistogramSize  = 64
	earlyChunkTimeSlices     = 3
	earlyChunkMaxBuckets     = 4096
	earlyChunkMaxCellBytes   = 8192
	earlyChunkMaxSlotSamples = 1_000_000
	earlyChunkRedisPrefix    = "new-api:{chat-early-chunk:v3}:"
	earlyChunkUnknownTokens  = 42
	earlyChunkOverflowTokens = 41
	earlyChunkUnknownCache   = 20
)

type EarlyChunkBand struct {
	Index   int  `json:"index"`
	Min     int  `json:"min"`
	Max     *int `json:"max,omitempty"`
	Unknown bool `json:"unknown,omitempty"`
}

func earlyChunkTokenBands() []EarlyChunkBand {
	bands := make([]EarlyChunkBand, 0, earlyChunkUnknownTokens+1)
	minimum := 0
	for _, maximum := range []int{1024, 2 * 1024, 4 * 1024, 8 * 1024, 16 * 1024} {
		bands = append(bands, EarlyChunkBand{Index: len(bands), Min: minimum, Max: common.GetPointer(maximum)})
		minimum = maximum
	}
	for _, segment := range []struct{ end, step int }{{64 * 1024, 4 * 1024}, {128 * 1024, 8 * 1024}, {256 * 1024, 16 * 1024}, {512 * 1024, 32 * 1024}} {
		for minimum < segment.end {
			maximum := minimum + segment.step
			bands = append(bands, EarlyChunkBand{Index: len(bands), Min: minimum, Max: common.GetPointer(maximum)})
			minimum = maximum
		}
	}
	bands = append(bands, EarlyChunkBand{Index: earlyChunkOverflowTokens, Min: 512 * 1024}, EarlyChunkBand{Index: earlyChunkUnknownTokens, Unknown: true})
	return bands
}

// Basis points keep the high-reuse ranges precise without float bucket edges.
var earlyChunkCacheEdges = [...]int{0, 1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 8500, 9000, 9200, 9400, 9500, 9600, 9700, 9800, 9900, 9950, 9990, 10000}

func earlyChunkCacheBands() []EarlyChunkBand {
	bands := make([]EarlyChunkBand, 0, 21)
	for i := range len(earlyChunkCacheEdges) - 1 {
		bands = append(bands, EarlyChunkBand{Index: i, Min: earlyChunkCacheEdges[i], Max: common.GetPointer(earlyChunkCacheEdges[i+1])})
	}
	return append(bands, EarlyChunkBand{Index: earlyChunkUnknownCache, Unknown: true})
}

func earlyChunkTokenBand(tokens int) int {
	if tokens <= 0 {
		return earlyChunkUnknownTokens
	}
	for _, band := range earlyChunkTokenRanges[:earlyChunkOverflowTokens] {
		if tokens <= *band.Max {
			return band.Index
		}
	}
	return earlyChunkOverflowTokens
}

var earlyChunkTokenRanges = earlyChunkTokenBands()

// Cache indices refer to the finest display columns. Short inputs combine
// those columns into progressively coarser ranges, without duplicating samples.
func earlyChunkCacheBandsByToken() [][]EarlyChunkBand {
	ranges := make([][]EarlyChunkBand, len(earlyChunkTokenRanges))
	for token, band := range earlyChunkTokenRanges {
		edges := earlyChunkCacheEdges[:]
		if band.Max != nil {
			switch {
			case *band.Max <= 2*1024:
				edges = []int{0, 10000}
			case *band.Max <= 4*1024:
				edges = []int{0, 5000, 10000}
			case *band.Max <= 8*1024:
				edges = []int{0, 5000, 9000, 10000}
			case *band.Max <= 16*1024:
				edges = []int{0, 5000, 8000, 9500, 10000}
			case *band.Max <= 64*1024:
				edges = []int{0, 2000, 4000, 6000, 8000, 9500, 10000}
			case *band.Max <= 128*1024:
				edges = []int{0, 2000, 4000, 6000, 8000, 9000, 9500, 9900, 10000}
			case *band.Max <= 256*1024:
				edges = []int{0, 1000, 2000, 4000, 6000, 8000, 9000, 9400, 9600, 9800, 9900, 9950, 10000}
			}
		}
		for i, minimum := range edges[:len(edges)-1] {
			ranges[token] = append(ranges[token], EarlyChunkBand{Index: slices.Index(earlyChunkCacheEdges[:], minimum), Min: minimum, Max: common.GetPointer(edges[i+1])})
		}
		ranges[token] = append(ranges[token], EarlyChunkBand{Index: earlyChunkUnknownCache, Unknown: true})
	}
	return ranges
}

var earlyChunkCacheRanges = earlyChunkCacheBandsByToken()

func earlyChunkCacheBand(token, matched, total int, known bool) int {
	if !known || total <= 0 {
		return earlyChunkUnknownCache
	}
	ratio := int64(max(0, matched)) * 10000 / int64(total)
	bands := earlyChunkCacheRanges[token]
	for _, band := range bands[:len(bands)-1] {
		if ratio < int64(*band.Max) {
			return band.Index
		}
	}
	return bands[len(bands)-2].Index
}

func earlyChunkCellID(tokens, cache int) int { return tokens*21 + cache }

// Millisecond upper bounds: 250ms around the pre-emission window, coarser
// thereafter. The final bin is overflow and never claims a precise quantile.
func earlyChunkLatencyBounds() [earlyChunkHistogramSize]int64 {
	var bounds [earlyChunkHistogramSize]int64
	i := 0
	for upper := int64(250); upper <= 8000; upper += 250 {
		bounds[i] = upper
		i++
	}
	for upper := int64(8500); upper <= 16000; upper += 500 {
		bounds[i] = upper
		i++
	}
	for upper := int64(18000); upper <= 40000; upper += 2000 {
		bounds[i] = upper
		i++
	}
	for _, upper := range []int64{60000, 120000, 300000, math.MaxInt32} {
		bounds[i] = upper
		i++
	}
	return bounds
}

var earlyChunkBounds = earlyChunkLatencyBounds()

func earlyChunkLatencyBin(ms int64) int {
	for i, upper := range earlyChunkBounds {
		if ms <= upper {
			return i
		}
	}
	return earlyChunkHistogramSize - 1
}

type earlyChunkSlice struct {
	Epoch      int64                           `json:"e"`
	Content    [earlyChunkHistogramSize]uint32 `json:"c"`
	Failure    [earlyChunkHistogramSize]uint32 `json:"f"`
	Cancel     [earlyChunkHistogramSize]uint32 `json:"x"`
	SumMS      float64                         `json:"s"`
	Completed  uint32                          `json:"ok"`
	Failed     uint32                          `json:"bad"`
	Cancelled  uint32                          `json:"cancel"`
	Decisions  uint32                          `json:"d"`
	Sent       uint32                          `json:"sent"`
	Met        uint32                          `json:"met"`
	SentMet    uint32                          `json:"sm"`
	SentFailed uint32                          `json:"sf"`
	GapCount   uint32                          `json:"gn"`
	GapSumMS   float64                         `json:"gs"`
	UpdatedMS  int64                           `json:"u"`
}

type earlyChunkCell struct {
	Version int                                   `json:"v"`
	SpanMS  int64                                 `json:"span"`
	Slots   [earlyChunkTimeSlices]earlyChunkSlice `json:"slots"`
}

type earlyChunkRecord struct {
	At        time.Time
	LatencyMS int64
	Kind      string // content, failure, cancel; terminal outcome is independent
	Outcome   string // completed, failed, cancelled
	Decision  bool
	Sent      bool
	Met       bool
	GapMS     *int64
}

type earlyChunkStoredKey struct {
	Scope string
	Cell  int
}

var chatEarlyChunkHistory = struct {
	sync.Mutex
	entries map[earlyChunkStoredKey]*earlyChunkCell
}{entries: make(map[earlyChunkStoredKey]*earlyChunkCell)}

var earlyChunkReadErrors atomic.Uint64
var earlyChunkWriteErrors atomic.Uint64
var earlyChunkEvictions atomic.Uint64

func earlyChunkScopeKey(name string, channel *model.Channel, age int) string {
	return earlyChunkChannelScopeKey(name, chatPrefixChannelIdentity(channel), age)
}

func earlyChunkChannelScopeKey(name string, channel chatPrefixChannel, age int) string {
	encoded, _ := common.Marshal([]any{"grid-v3", name, channel.ID, channel, age})
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func earlyChunkSpan(age int) int64 { return max(1, (int64(age)*1000+2)/3) }

func (cell *earlyChunkCell) record(record earlyChunkRecord, now time.Time, age int) bool {
	span := earlyChunkSpan(age)
	if cell.Version != 2 || cell.SpanMS != span {
		*cell = earlyChunkCell{Version: 2, SpanMS: span}
	}
	epoch := record.At.UnixMilli() / span
	current := now.UnixMilli() / span
	if epoch > current || epoch < current-2 {
		return false
	}
	slot := &cell.Slots[epoch%3]
	if slot.Epoch != epoch {
		*slot = earlyChunkSlice{Epoch: epoch}
	}
	var count uint64
	for i := range earlyChunkHistogramSize {
		count += uint64(slot.Content[i]) + uint64(slot.Failure[i]) + uint64(slot.Cancel[i])
	}
	if count >= earlyChunkMaxSlotSamples {
		return false
	}
	bin := earlyChunkLatencyBin(record.LatencyMS)
	switch record.Kind {
	case "content":
		slot.Content[bin]++
		slot.SumMS += float64(record.LatencyMS)
	case "failure":
		slot.Failure[bin]++
	case "cancel":
		slot.Cancel[bin]++
	default:
		return false
	}
	switch record.Outcome {
	case "completed":
		slot.Completed++
	case "failed":
		slot.Failed++
	case "cancelled":
		slot.Cancelled++
	}
	if record.Decision {
		slot.Decisions++
	}
	if record.Sent {
		slot.Sent++
	}
	if record.Met {
		slot.Met++
	}
	if record.Sent && record.Met {
		slot.SentMet++
	}
	if record.Sent && record.Outcome == "failed" {
		slot.SentFailed++
	}
	if record.GapMS != nil && *record.GapMS >= 0 {
		slot.GapCount++
		slot.GapSumMS += float64(*record.GapMS)
	}
	slot.UpdatedMS = max(slot.UpdatedMS, record.At.UnixMilli())
	return true
}

type earlyChunkAggregate struct {
	Content    [earlyChunkHistogramSize]uint64
	Failure    [earlyChunkHistogramSize]uint64
	Cancel     [earlyChunkHistogramSize]uint64
	SumMS      float64
	Completed  uint64
	Failed     uint64
	Cancelled  uint64
	Decisions  uint64
	Sent       uint64
	Met        uint64
	SentMet    uint64
	SentFailed uint64
	GapCount   uint64
	GapSumMS   float64
	UpdatedMS  int64
}

func (total *earlyChunkAggregate) add(cell *earlyChunkCell, now time.Time, age int) {
	if cell == nil || cell.Version != 2 || cell.SpanMS != earlyChunkSpan(age) {
		return
	}
	epoch := now.UnixMilli() / cell.SpanMS
	for _, slot := range cell.Slots {
		if slot.Epoch > epoch || slot.Epoch < epoch-2 {
			continue
		}
		for i := range earlyChunkHistogramSize {
			total.Content[i] += uint64(slot.Content[i])
			total.Failure[i] += uint64(slot.Failure[i])
			total.Cancel[i] += uint64(slot.Cancel[i])
		}
		total.SumMS += slot.SumMS
		total.Completed += uint64(slot.Completed)
		total.Failed += uint64(slot.Failed)
		total.Cancelled += uint64(slot.Cancelled)
		total.Decisions += uint64(slot.Decisions)
		total.Sent += uint64(slot.Sent)
		total.Met += uint64(slot.Met)
		total.SentMet += uint64(slot.SentMet)
		total.SentFailed += uint64(slot.SentFailed)
		total.GapCount += uint64(slot.GapCount)
		total.GapSumMS += slot.GapSumMS
		total.UpdatedMS = max(total.UpdatedMS, slot.UpdatedMS)
	}
}

type EarlyChunkPrediction struct {
	Pending         uint64  `json:"pending"`
	Soon            uint64  `json:"soon"`
	Probability     float64 `json:"probability"`
	MergedCells     int     `json:"merged_cells"`
	Eligible        bool    `json:"eligible"`
	AtMS            int64   `json:"at_ms"`
	EstimatedTTFTMS *int64  `json:"estimated_ttft_ms"`
	RemainingP50MS  *int64  `json:"remaining_p50_ms"`
	RemainingP90MS  *int64  `json:"remaining_p90_ms"`
	LeadMS          *int64  `json:"lead_ms"`
	CandidateMS     *int64  `json:"candidate_ms"`
	Reason          string  `json:"reason"`
}

// Quantiles use only content observations that can still be pending at elapsed.
// A straddling bin remains in the risk set; overflow cannot give a finite ETA.
func (total *earlyChunkAggregate) contentQuantile(elapsed int64, percentile uint64) *int64 {
	var samples uint64
	for bin, upper := range earlyChunkBounds {
		if upper > elapsed {
			samples += total.Content[bin]
		}
	}
	if samples == 0 {
		return nil
	}
	threshold := (samples*percentile + 99) / 100
	var count uint64
	for bin, upper := range earlyChunkBounds {
		if upper <= elapsed {
			continue
		}
		count += total.Content[bin]
		if count >= threshold {
			if bin == earlyChunkHistogramSize-1 {
				return nil
			}
			return common.GetPointer(upper)
		}
	}
	return nil
}

func (total *earlyChunkAggregate) prediction(elapsed int64, config chat_early_chunk_setting.Config) EarlyChunkPrediction {
	result := EarlyChunkPrediction{AtMS: elapsed, Reason: "insufficient_samples"}
	result.EstimatedTTFTMS = total.contentQuantile(elapsed, 50)
	if result.EstimatedTTFTMS != nil {
		result.RemainingP50MS = common.GetPointer(*result.EstimatedTTFTMS - elapsed)
		lead := min(int64(config.MaxLeadMS), max(int64(config.MinLeadMS), int64(math.Round(float64(*result.EstimatedTTFTMS)*config.LeadRatio))))
		result.LeadMS = &lead
		result.CandidateMS = common.GetPointer(max(int64(config.EarliestMS), *result.EstimatedTTFTMS-lead))
	}
	if p90 := total.contentQuantile(elapsed, 90); p90 != nil {
		result.RemainingP90MS = common.GetPointer(*p90 - elapsed)
	}
	minimum := int64(0)
	for bin, upper := range earlyChunkBounds {
		if upper > elapsed {
			result.Pending += total.Content[bin] + total.Failure[bin] + total.Cancel[bin]
		}
		if result.LeadMS != nil && minimum > elapsed && upper <= elapsed+*result.LeadMS {
			result.Soon += total.Content[bin]
		}
		minimum = upper + 1
	}
	if result.Pending > 0 {
		result.Probability = float64(result.Soon) / float64(result.Pending)
	}
	if result.Pending < uint64(config.MinSamples) {
		return result
	}
	if result.CandidateMS == nil {
		result.Reason = "no_content_estimate"
		return result
	}
	if elapsed < *result.CandidateMS {
		result.Reason = "before_candidate"
		return result
	}
	if result.Probability < config.MinProbability {
		result.Reason = "low_probability"
		return result
	}
	result.Eligible = true
	result.Reason = "eligible"
	return result
}

func earlyChunkNeighborIDs(token, cache int) []int {
	ids := []int{earlyChunkCellID(token, cache)}
	if token >= earlyChunkOverflowTokens || cache == earlyChunkUnknownCache {
		return ids
	}
	bands := earlyChunkCacheRanges[token]
	position := slices.IndexFunc(bands, func(band EarlyChunkBand) bool { return band.Index == cache })
	if position < 0 {
		return ids
	}
	seen := map[int]bool{ids[0]: true}
	// Start with the exact reuse interval, then widen only when it cannot supply
	// enough pending samples. A neighboring token row may have a different
	// partition: include only whole buckets inside the current reuse interval.
	for radius := range len(bands) - 1 {
		minimum := bands[max(0, position-radius)].Min
		maximum := *bands[min(len(bands)-2, position+radius)].Max
		for _, delta := range []int{0, -1, 1, -2, 2} {
			neighbor := token + delta
			if neighbor < 0 || neighbor >= earlyChunkOverflowTokens {
				continue
			}
			for _, band := range earlyChunkCacheRanges[neighbor] {
				if band.Unknown || band.Min < minimum || *band.Max > maximum {
					continue
				}
				id := earlyChunkCellID(neighbor, band.Index)
				if !seen[id] {
					ids = append(ids, id)
					seen[id] = true
				}
			}
		}
	}
	return ids
}

func earlyChunkPredict(cells map[int]*earlyChunkCell, token, cache int, now time.Time, elapsed int64, config chat_early_chunk_setting.Config) EarlyChunkPrediction {
	if elapsed < 0 {
		// Preview the same candidate jumps as the runtime timer. Each jump removes
		// elapsed histogram mass; the fixed bound also protects monitor requests.
		result := earlyChunkPredict(cells, token, cache, now, 0, config)
		for range earlyChunkHistogramSize {
			if result.CandidateMS == nil || *result.CandidateMS <= result.AtMS {
				return result
			}
			result = earlyChunkPredict(cells, token, cache, now, *result.CandidateMS, config)
		}
		return result
	}
	var combined earlyChunkAggregate
	result := EarlyChunkPrediction{AtMS: elapsed, Reason: "insufficient_samples"}
	included := 0
	for _, id := range earlyChunkNeighborIDs(token, cache) {
		cell := cells[id]
		if cell == nil {
			continue
		}
		combined.add(cell, now, config.MaxAgeSeconds)
		included++
		result = combined.prediction(elapsed, config)
		result.MergedCells = included
		if result.Pending >= uint64(config.MinSamples) {
			break
		}
	}
	if token == earlyChunkUnknownTokens || cache == earlyChunkUnknownCache {
		result.Eligible = false
		result.Reason = "unknown_estimate"
	}
	return result
}

func decodeEarlyChunkCell(value string, age int) (*earlyChunkCell, error) {
	if len(value) > earlyChunkMaxCellBytes {
		return nil, fmt.Errorf("early chunk cell exceeds size limit")
	}
	var cell earlyChunkCell
	if err := common.UnmarshalJsonStr(value, &cell); err != nil {
		return nil, fmt.Errorf("invalid early chunk cell: %w", err)
	}
	if cell.Version != 2 || cell.SpanMS != earlyChunkSpan(age) {
		return nil, fmt.Errorf("incompatible early chunk cell")
	}
	for _, slot := range cell.Slots {
		var n, content uint64
		for i := range earlyChunkHistogramSize {
			content += uint64(slot.Content[i])
			n += uint64(slot.Content[i]) + uint64(slot.Failure[i]) + uint64(slot.Cancel[i])
		}
		if n > earlyChunkMaxSlotSamples || uint64(slot.Completed)+uint64(slot.Failed)+uint64(slot.Cancelled) != n || uint64(slot.Decisions) > n || slot.Sent > slot.Decisions || slot.Met > slot.Decisions || slot.SentMet > slot.Sent || slot.SentFailed > slot.Sent || uint64(slot.GapCount) > content || math.IsNaN(slot.SumMS) || slot.SumMS < 0 || slot.SumMS > float64(content)*math.MaxInt32 || math.IsNaN(slot.GapSumMS) || slot.GapSumMS < 0 || slot.GapSumMS > float64(slot.GapCount)*math.MaxInt32 {
			return nil, fmt.Errorf("invalid early chunk cell counters")
		}
	}
	return &cell, nil
}

// One compact hash field per populated cell. Updates and cross-node eviction
// are atomic; no per-request samples, prompts or user identifiers are retained.
const earlyChunkRecordScript = `
local t=redis.call('TIME')
local now=tonumber(t[1])*1000+math.floor(tonumber(t[2])/1000)
local span=tonumber(ARGV[2])
local epoch=math.floor((now-tonumber(ARGV[3]))/span)
if epoch<math.floor(now/span)-2 then return {0,0} end
local raw=redis.call('HGET',KEYS[1],ARGV[1])
local c
local function empty(e)
 local s={e=e,c={},f={},x={},s=0,ok=0,bad=0,cancel=0,d=0,sent=0,met=0,sm=0,sf=0,gn=0,gs=0,u=0}
 for i=1,64 do s.c[i]=0;s.f[i]=0;s.x[i]=0 end
 return s
end
if raw then
 if #raw>8192 then return redis.error_reply('oversized early chunk cell') end
 c=cjson.decode(raw)
 if c.v~=2 or c.span~=span then return redis.error_reply('incompatible early chunk cell') end
else c={v=2,span=span,slots={empty(0),empty(0),empty(0)}} end
local index=epoch%3+1
local s=c.slots[index]
if s.e~=epoch then s=empty(epoch);c.slots[index]=s end
local n=0
for i=1,64 do n=n+s.c[i]+s.f[i]+s.x[i] end
if n>=1000000 then return {0,0} end
local bin=tonumber(ARGV[4])+1
local kind=ARGV[5]
if kind=='content' then s.c[bin]=s.c[bin]+1;s.s=s.s+tonumber(ARGV[6])
elseif kind=='failure' then s.f[bin]=s.f[bin]+1
elseif kind=='cancel' then s.x[bin]=s.x[bin]+1
else return redis.error_reply('invalid early chunk event') end
local outcome=ARGV[7]
if outcome=='completed' then s.ok=s.ok+1 elseif outcome=='failed' then s.bad=s.bad+1 else s.cancel=s.cancel+1 end
s.d=s.d+tonumber(ARGV[8]);s.sent=s.sent+tonumber(ARGV[9]);s.met=s.met+tonumber(ARGV[10])
if tonumber(ARGV[9])==1 and tonumber(ARGV[10])==1 then s.sm=s.sm+1 end
if tonumber(ARGV[9])==1 and outcome=='failed' then s.sf=s.sf+1 end
local gap=tonumber(ARGV[11]);if gap>=0 then s.gn=s.gn+1;s.gs=s.gs+gap end
s.u=math.max(s.u,now-tonumber(ARGV[3]))
redis.call('HSET',KEYS[1],ARGV[1],cjson.encode(c))
redis.call('PEXPIRE',KEYS[1],tonumber(ARGV[12]))
local latest=0
for _,slot in ipairs(c.slots) do latest=math.max(latest,slot.u) end
redis.call('ZADD',KEYS[2],latest,KEYS[1]..'|'..ARGV[1])
-- Keep the index longer than every allowed cell TTL across config generations.
redis.call('PEXPIRE',KEYS[2],86400000)
local removed=0
local function remove(member)
 local key,field=string.match(member,'^(.*)|([0-9]+)$')
 if key and string.sub(key,1,string.len(ARGV[13]))==ARGV[13] then
  redis.call('HDEL',key,field)
  if redis.call('HLEN',key)==0 then redis.call('DEL',key) end
 end
 redis.call('ZREM',KEYS[2],member);removed=removed+1
end
for _,member in ipairs(redis.call('ZRANGEBYSCORE',KEYS[2],'-inf',now-tonumber(ARGV[12]),'LIMIT',0,128)) do remove(member) end
local extra=redis.call('ZCARD',KEYS[2])-4096
if extra>0 then for _,member in ipairs(redis.call('ZRANGE',KEYS[2],0,extra-1)) do remove(member) end end
return {1,removed}`

func earlyChunkReadCells(ctx context.Context, scope string, ids []int, now time.Time, age int) (map[int]*earlyChunkCell, int64, int, error) {
	result := make(map[int]*earlyChunkCell)
	var payload int64
	if common.RedisEnabled {
		if common.RDB == nil {
			return nil, 0, 0, fmt.Errorf("early chunk Redis is unavailable")
		}
		ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		client := common.RDB.WithTimeout(100 * time.Millisecond)
		key := earlyChunkRedisPrefix + "data:" + scope
		values := make(map[string]string)
		if ids == nil {
			n, err := client.HLen(ctx, key).Result()
			if err != nil {
				return nil, 0, 0, err
			}
			if n > (earlyChunkUnknownTokens+1)*21 {
				return nil, 0, 0, fmt.Errorf("early chunk table exceeds cell limit")
			}
			values, err = client.HGetAll(ctx, key).Result()
			if err != nil {
				return nil, 0, 0, err
			}
		} else {
			fields := make([]string, len(ids))
			for i, id := range ids {
				fields[i] = strconv.Itoa(id)
			}
			loaded, err := client.HMGet(ctx, key, fields...).Result()
			if err != nil {
				return nil, 0, 0, err
			}
			for i, value := range loaded {
				if value != nil {
					text, ok := value.(string)
					if !ok {
						return nil, 0, 0, fmt.Errorf("invalid early chunk Redis value")
					}
					values[fields[i]] = text
				}
			}
		}
		for field, value := range values {
			id, err := strconv.Atoi(field)
			if err != nil || id < 0 || id >= (earlyChunkUnknownTokens+1)*21 {
				return nil, 0, 0, fmt.Errorf("invalid early chunk cell id")
			}
			cell, err := decodeEarlyChunkCell(value, age)
			if err != nil {
				return nil, 0, 0, err
			}
			var sum earlyChunkAggregate
			sum.add(cell, now, age)
			if sum.UpdatedMS == 0 {
				continue
			}
			result[id] = cell
			payload += int64(len(value))
		}
		if ids != nil {
			return result, 0, 0, nil
		}
		n, err := client.ZCount(ctx, earlyChunkRedisPrefix+"index", strconv.FormatInt(now.UnixMilli()-int64(age)*1000, 10), "+inf").Result()
		if err != nil {
			return nil, 0, 0, err
		}
		return result, payload, int(n), nil
	}
	chatEarlyChunkHistory.Lock()
	defer chatEarlyChunkHistory.Unlock()
	if ids != nil {
		// Prediction touches a bounded neighborhood; global scans belong to the
		// low-frequency monitor, never the request path.
		for _, id := range ids {
			key := earlyChunkStoredKey{Scope: scope, Cell: id}
			cell := chatEarlyChunkHistory.entries[key]
			var aggregate earlyChunkAggregate
			aggregate.add(cell, now, age)
			if aggregate.UpdatedMS == 0 {
				delete(chatEarlyChunkHistory.entries, key)
				continue
			}
			copy := *cell
			result[id] = &copy
		}
		return result, 0, len(chatEarlyChunkHistory.entries), nil
	}
	for key, cell := range chatEarlyChunkHistory.entries {
		var aggregate earlyChunkAggregate
		aggregate.add(cell, now, age)
		if aggregate.UpdatedMS == 0 {
			delete(chatEarlyChunkHistory.entries, key)
			continue
		}
		if key.Scope != scope {
			continue
		}
		copy := *cell
		result[key.Cell] = &copy
		encoded, _ := common.Marshal(cell)
		payload += int64(len(encoded))
	}
	return result, payload, len(chatEarlyChunkHistory.entries), nil
}

func earlyChunkRecordCell(ctx context.Context, scope string, id int, record earlyChunkRecord, now time.Time, age int) error {
	if id < 0 || id >= (earlyChunkUnknownTokens+1)*21 || record.LatencyMS < 0 || record.LatencyMS > math.MaxInt32 || record.GapMS != nil && (*record.GapMS < 0 || *record.GapMS > math.MaxInt32) {
		return fmt.Errorf("invalid early chunk observation")
	}
	if !slices.Contains([]string{"content", "failure", "cancel"}, record.Kind) || !slices.Contains([]string{"completed", "failed", "cancelled"}, record.Outcome) || record.Kind != "content" && (record.Outcome == "completed" || record.GapMS != nil || record.Met) || record.Sent && !record.Decision || record.Met && !record.Decision {
		return fmt.Errorf("inconsistent early chunk observation")
	}
	epoch, current := record.At.UnixMilli()/earlyChunkSpan(age), now.UnixMilli()/earlyChunkSpan(age)
	if epoch > current || epoch < current-2 {
		return nil
	}
	if common.RedisEnabled {
		if common.RDB == nil {
			return fmt.Errorf("early chunk Redis is unavailable")
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Millisecond)
		defer cancel()
		flag := func(value bool) int {
			if value {
				return 1
			}
			return 0
		}
		gap := int64(-1)
		if record.GapMS != nil {
			gap = *record.GapMS
		}
		lag := max(int64(0), now.Sub(record.At).Milliseconds())
		result, err := common.RDB.WithTimeout(100*time.Millisecond).Eval(ctx, earlyChunkRecordScript, []string{earlyChunkRedisPrefix + "data:" + scope, earlyChunkRedisPrefix + "index"}, strconv.Itoa(id), earlyChunkSpan(age), lag, earlyChunkLatencyBin(record.LatencyMS), record.Kind, record.LatencyMS, record.Outcome, flag(record.Decision), flag(record.Sent), flag(record.Met), gap, int64(age)*1000, earlyChunkRedisPrefix+"data:").Int64Slice()
		if err != nil {
			return err
		}
		if len(result) != 2 {
			return fmt.Errorf("invalid early chunk update result")
		}
		earlyChunkEvictions.Add(uint64(max(int64(0), result[1])))
		return nil
	}
	chatEarlyChunkHistory.Lock()
	defer chatEarlyChunkHistory.Unlock()
	key := earlyChunkStoredKey{Scope: scope, Cell: id}
	cell := chatEarlyChunkHistory.entries[key]
	if cell == nil {
		if len(chatEarlyChunkHistory.entries) >= earlyChunkMaxBuckets {
			var oldestKey earlyChunkStoredKey
			oldest := int64(math.MaxInt64)
			for k, value := range chatEarlyChunkHistory.entries {
				latest := int64(0)
				for _, slot := range value.Slots {
					latest = max(latest, slot.UpdatedMS)
				}
				if latest < oldest {
					oldestKey, oldest = k, latest
				}
			}
			delete(chatEarlyChunkHistory.entries, oldestKey)
			earlyChunkEvictions.Add(1)
		}
		cell = &earlyChunkCell{}
		chatEarlyChunkHistory.entries[key] = cell
	}
	cell.record(record, now, age)
	return nil
}

type EarlyChunkCellMetrics struct {
	Samples        uint64   `json:"samples"`
	ContentSamples uint64   `json:"content_samples"`
	Completed      uint64   `json:"completed"`
	Failed         uint64   `json:"failed"`
	Cancelled      uint64   `json:"cancelled"`
	AverageMS      *float64 `json:"average_ms"`
	P50MS          *int64   `json:"p50_ms"`
	P90MS          *int64   `json:"p90_ms"`
	P95MS          *int64   `json:"p95_ms"`
	Overflow       uint64   `json:"overflow"`
	Decisions      uint64   `json:"decisions"`
	Sent           uint64   `json:"sent"`
	Met            uint64   `json:"met"`
	SentMet        uint64   `json:"sent_met"`
	SentFailed     uint64   `json:"sent_failed"`
	AverageGapMS   *float64 `json:"average_gap_ms"`
	UpdatedAtMS    int64    `json:"updated_at_ms"`
}

func (total *earlyChunkAggregate) metrics() EarlyChunkCellMetrics {
	result := EarlyChunkCellMetrics{Completed: total.Completed, Failed: total.Failed, Cancelled: total.Cancelled, Decisions: total.Decisions, Sent: total.Sent, Met: total.Met, SentMet: total.SentMet, SentFailed: total.SentFailed, UpdatedAtMS: total.UpdatedMS, Overflow: total.Content[earlyChunkHistogramSize-1]}
	for i := range earlyChunkHistogramSize {
		result.ContentSamples += total.Content[i]
		result.Samples += total.Content[i] + total.Failure[i] + total.Cancel[i]
	}
	if result.ContentSamples > 0 {
		result.AverageMS = common.GetPointer(total.SumMS / float64(result.ContentSamples))
	}
	if total.GapCount > 0 {
		result.AverageGapMS = common.GetPointer(total.GapSumMS / float64(total.GapCount))
	}
	result.P50MS = total.contentQuantile(-1, 50)
	result.P90MS = total.contentQuantile(-1, 90)
	result.P95MS = total.contentQuantile(-1, 95)
	return result
}

type EarlyChunkCellView struct {
	ID         int                   `json:"id"`
	TokenBand  int                   `json:"token_band"`
	CacheBand  int                   `json:"cache_band"`
	Metrics    EarlyChunkCellMetrics `json:"metrics"`
	Prediction EarlyChunkPrediction  `json:"prediction"`
	Histogram  []uint64              `json:"histogram"`
}

type ChatEarlyChunkGrid struct {
	Model             string                          `json:"model"`
	ChannelID         int                             `json:"channel_id"`
	ChannelName       string                          `json:"channel_name"`
	Scope             string                          `json:"scope"`
	Backend           string                          `json:"backend"`
	TokenBands        []EarlyChunkBand                `json:"token_bands"`
	CacheBands        []EarlyChunkBand                `json:"cache_bands"`
	CacheBandsByToken [][]EarlyChunkBand              `json:"cache_bands_by_token"`
	LatencyBoundsMS   []int64                         `json:"latency_bounds_ms"`
	RegularBuckets    int                             `json:"regular_buckets"`
	TrackedBuckets    int                             `json:"tracked_buckets"`
	MaxBuckets        int                             `json:"max_buckets"`
	PayloadBytes      int64                           `json:"payload_bytes"`
	ReadErrors        uint64                          `json:"node_read_errors"`
	WriteErrors       uint64                          `json:"node_write_errors"`
	Evictions         uint64                          `json:"node_evictions"`
	ProbeMS           int64                           `json:"probe_ms"`
	Config            chat_early_chunk_setting.Config `json:"config"`
	Summary           EarlyChunkCellMetrics           `json:"summary"`
	Cells             []EarlyChunkCellView            `json:"cells"`
	GeneratedAtMS     int64                           `json:"generated_at_ms"`
}

func GetChatEarlyChunkGrid(ctx context.Context, name string, channel *model.Channel, probeMS int64) (*ChatEarlyChunkGrid, error) {
	config := model.CurrentRequestPolicy().ChatEarlyChunk
	if config.Validate() != nil {
		return nil, fmt.Errorf("invalid early chunk configuration")
	}
	now := time.Now()
	cells, payload, tracked, err := earlyChunkReadCells(ctx, earlyChunkScopeKey(name, channel, config.MaxAgeSeconds), nil, now, config.MaxAgeSeconds)
	if err != nil {
		earlyChunkReadErrors.Add(1)
		return nil, err
	}
	regularBuckets := 0
	for _, bands := range earlyChunkCacheRanges[:earlyChunkOverflowTokens] {
		regularBuckets += len(bands) - 1
	}
	result := &ChatEarlyChunkGrid{Model: name, ChannelID: channel.Id, ChannelName: channel.Name, Scope: "model_channel", Backend: "memory", TokenBands: earlyChunkTokenRanges, CacheBands: earlyChunkCacheBands(), CacheBandsByToken: earlyChunkCacheRanges, LatencyBoundsMS: earlyChunkBounds[:earlyChunkHistogramSize-1], RegularBuckets: regularBuckets, TrackedBuckets: tracked, MaxBuckets: earlyChunkMaxBuckets, PayloadBytes: payload, ReadErrors: earlyChunkReadErrors.Load(), WriteErrors: earlyChunkWriteErrors.Load(), Evictions: earlyChunkEvictions.Load(), ProbeMS: probeMS, Config: config, GeneratedAtMS: now.UnixMilli(), Cells: make([]EarlyChunkCellView, 0, len(cells))}
	if common.RedisEnabled {
		result.Backend = "redis"
	}
	var all earlyChunkAggregate
	for id, cell := range cells {
		var total earlyChunkAggregate
		total.add(cell, now, config.MaxAgeSeconds)
		all.add(cell, now, config.MaxAgeSeconds)
		result.Cells = append(result.Cells, EarlyChunkCellView{ID: id, TokenBand: id / 21, CacheBand: id % 21, Metrics: total.metrics(), Prediction: earlyChunkPredict(cells, id/21, id%21, now, probeMS, config), Histogram: total.Content[:]})
	}
	slices.SortFunc(result.Cells, func(a, b EarlyChunkCellView) int { return a.ID - b.ID })
	result.Summary = all.metrics()
	return result, nil
}
