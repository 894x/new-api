package service

import (
	"container/list"
	"context"
	"crypto/sha256"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const chatPrefixRequestContextKey = "chat_prefix_request"
const chatPrefixChannelContextKey = "chat_prefix_channel"

// This is a process-local estimate, not a copy of the provider's KV cache.
// Entries contain sampled strings and lengths, and expire from their last success.
var chatPrefixRoutingCache = newChatPrefixCache(40960, 5*time.Minute)

type chatPrefixCheckpoint struct {
	Sample string
	Bytes  int
}

type chatPrefixRequest struct {
	Checkpoints []chatPrefixCheckpoint
	Bytes       int
}

type chatPrefixScope struct {
	UserID int
	Group  string
	Model  string
}

type chatPrefixChannel struct {
	ID      int
	Version [sha256.Size]byte
}

// ChatPrefixMatch describes the estimated reusable input on one channel.
// Bytes are raw HTTP body bytes, not billable or provider-cached tokens.
type ChatPrefixMatch struct {
	MatchedBytes int
	HistoryBytes int
	NewBytes     int
	ObservedAt   time.Time
}

type chatPrefixEntry struct {
	Scope   chatPrefixScope
	Channel chatPrefixChannel
	Request *chatPrefixRequest
	SeenAt  time.Time
}

type chatPrefixIndexKey struct {
	Scope  chatPrefixScope
	Sample string
}

type chatPrefixCache struct {
	mu       sync.Mutex
	entries  *list.List
	index    map[chatPrefixIndexKey]map[int]*list.Element
	capacity int
	ttl      time.Duration
}

func newChatPrefixCache(capacity int, ttl time.Duration) *chatPrefixCache {
	return &chatPrefixCache{
		entries: list.New(), index: make(map[chatPrefixIndexKey]map[int]*list.Element),
		capacity: capacity, ttl: ttl,
	}
}

// Configuration is applied on the next eligible request. Reducing limits also
// removes old records; disabling drops the index so it can be reclaimed by GC.
func (cache *chatPrefixCache) configure(capacity int, ttl time.Duration, now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.capacity == capacity && cache.ttl == ttl {
		return
	}
	cache.capacity, cache.ttl = capacity, ttl
	if capacity == 0 {
		cache.entries.Init()
		cache.index = make(map[chatPrefixIndexKey]map[int]*list.Element)
		return
	}
	cache.expire(now)
	for cache.entries.Len() > capacity {
		cache.remove(cache.entries.Back())
	}
}

func (p *RetryParam) chatPrefixRequest() *chatPrefixRequest {
	c := p.Ctx
	if c == nil || c.Request == nil || c.Request.URL == nil || c.Request.Method != http.MethodPost ||
		c.Request.URL.Path != "/v1/chat/completions" ||
		!p.DynamicRoutingEligible || common.GetContextKeyInt(c, constant.ContextKeyUserId) <= 0 || RequestPolicy(c).SessionMode == "off" {
		return nil
	}
	// The distributor has already resolved stream eligibility from the request.
	// Sampling uses its original body bytes without decoding messages or JSON.
	if cached, exists := c.Get(chatPrefixRequestContextKey); exists {
		request, _ := cached.(*chatPrefixRequest)
		return request
	}
	request := buildChatPrefixRequest(p.RequestBody)
	c.Set(chatPrefixRequestContextKey, request)
	return request
}

func buildChatPrefixRequest(body []byte) *chatPrefixRequest {
	if len(body) == 0 {
		return nil
	}
	request := &chatPrefixRequest{Bytes: len(body)}
	samples := make([]byte, 0, 32)
	next, previous := 0, 1
	for next < len(body) {
		samples = append(samples, body[next])
		request.Checkpoints = append(request.Checkpoints, chatPrefixCheckpoint{Bytes: next + 1})
		if next == 0 {
			next = 1
		} else if previous > len(body)-1-next {
			break
		} else {
			next, previous = next+previous, next
		}
	}
	// Copy only the sampled bytes, then share this small string across the
	// checkpoint keys. No request body or cumulative prefix hash is retained.
	key := string(samples)
	for i := range request.Checkpoints {
		request.Checkpoints[i].Sample = key[:i+1]
	}
	return request
}

func chatPrefixChannelIdentity(channel *model.Channel) chatPrefixChannel {
	// Configuration changes must not inherit a former upstream's warm prefixes.
	encoded, _ := common.Marshal([]any{
		channel.Type, channel.BaseURL, channel.Key, channel.OpenAIOrganization,
		channel.ModelMapping, channel.ParamOverride, channel.HeaderOverride, channel.Setting, channel.OtherSettings,
	})
	return chatPrefixChannel{ID: channel.Id, Version: sha256.Sum256(encoded)}
}

// BindChatPrefixChannel captures the configuration actually used by this
// attempt. The channel may be edited while a long-running stream is in flight.
func BindChatPrefixChannel(c *gin.Context, channel *model.Channel) {
	if c == nil || channel == nil || c.Request == nil || c.Request.URL == nil ||
		c.Request.Method != http.MethodPost || c.Request.URL.Path != "/v1/chat/completions" ||
		!common.GetContextKeyBool(c, constant.ContextKeyDynamicRoutingEligible) {
		return
	}
	c.Set(chatPrefixChannelContextKey, chatPrefixChannelIdentity(channel))
}

func (entry *chatPrefixEntry) match(request *chatPrefixRequest) ChatPrefixMatch {
	match := ChatPrefixMatch{HistoryBytes: entry.Request.Bytes, ObservedAt: entry.SeenAt}
	matched := 0
	for i := range min(len(entry.Request.Checkpoints), len(request.Checkpoints)) {
		old, current := entry.Request.Checkpoints[i], request.Checkpoints[i]
		if old.Sample != current.Sample {
			break
		}
		matched++
		match.MatchedBytes = current.Bytes
	}
	if matched == min(len(entry.Request.Checkpoints), len(request.Checkpoints)) {
		// All stored checkpoints matching is intentionally sufficient to treat
		// the request as a continuation. Do not add a full-tail comparison.
		match.MatchedBytes = min(entry.Request.Bytes, request.Bytes)
	}
	match.NewBytes = request.Bytes - match.MatchedBytes
	return match
}

// Caller holds mu. Entries are ordered by their last successful observation.
func (cache *chatPrefixCache) expire(now time.Time) {
	for element := cache.entries.Back(); element != nil; element = cache.entries.Back() {
		if now.Before(element.Value.(*chatPrefixEntry).SeenAt.Add(cache.ttl)) {
			break
		}
		cache.remove(element)
	}
}

func (cache *chatPrefixCache) remove(element *list.Element) {
	entry := element.Value.(*chatPrefixEntry)
	for _, checkpoint := range entry.Request.Checkpoints {
		key := chatPrefixIndexKey{Scope: entry.Scope, Sample: checkpoint.Sample}
		if cache.index[key][entry.Channel.ID] == element {
			delete(cache.index[key], entry.Channel.ID)
		}
		if len(cache.index[key]) == 0 {
			delete(cache.index, key)
		}
	}
	cache.entries.Remove(element)
}

func (cache *chatPrefixCache) lookup(scope chatPrefixScope, request *chatPrefixRequest, now time.Time) map[chatPrefixChannel]ChatPrefixMatch {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.expire(now)
	matches := make(map[chatPrefixChannel]ChatPrefixMatch)
	seen := make(map[*list.Element]bool)
	for _, checkpoint := range request.Checkpoints {
		for _, element := range cache.index[chatPrefixIndexKey{Scope: scope, Sample: checkpoint.Sample}] {
			if seen[element] {
				continue
			}
			seen[element] = true
			entry := element.Value.(*chatPrefixEntry)
			match := entry.match(request)
			old := matches[entry.Channel]
			if match.MatchedBytes > old.MatchedBytes || match.MatchedBytes == old.MatchedBytes && match.ObservedAt.After(old.ObservedAt) {
				matches[entry.Channel] = match
			}
		}
	}
	return matches
}

func (cache *chatPrefixCache) record(scope chatPrefixScope, channel chatPrefixChannel, request *chatPrefixRequest, now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.expire(now)
	if cache.capacity <= 0 || len(request.Checkpoints) == 0 {
		return
	}
	if newest := cache.entries.Front(); newest != nil && newest.Value.(*chatPrefixEntry).SeenAt.After(now) {
		// Concurrent successes can enter the lock in a different order from
		// their timestamps. Keep expiry order monotonic without a background scan.
		now = newest.Value.(*chatPrefixEntry).SeenAt
	}
	var previous *list.Element
	for _, checkpoint := range request.Checkpoints {
		element := cache.index[chatPrefixIndexKey{Scope: scope, Sample: checkpoint.Sample}][channel.ID]
		if element == nil {
			continue
		}
		entry := element.Value.(*chatPrefixEntry)
		if entry.Channel != channel {
			continue
		}
		match := entry.match(request)
		if match.MatchedBytes != min(request.Bytes, entry.Request.Bytes) {
			continue
		}
		if entry.Request.Bytes > request.Bytes {
			// The same complete sampled key keeps its longer byte estimate.
			// A distinct shorter key is stored alongside the longer history.
			if len(entry.Request.Checkpoints) == len(request.Checkpoints) {
				entry.SeenAt = now
				cache.entries.MoveToFront(element)
				return
			}
			continue
		}
		if previous == nil || entry.Request.Bytes > previous.Value.(*chatPrefixEntry).Request.Bytes {
			previous = element
		}
	}
	if previous != nil {
		cache.remove(previous)
	}
	for cache.entries.Len() >= cache.capacity {
		cache.remove(cache.entries.Back())
	}
	// Namespace strings can originate as slices of a parsed request body.
	// Own the small names so the cache cannot retain the original JSON buffer.
	scope.Group, scope.Model = strings.Clone(scope.Group), strings.Clone(scope.Model)
	entry := &chatPrefixEntry{Scope: scope, Channel: channel, Request: request, SeenAt: now}
	element := cache.entries.PushFront(entry)
	for _, checkpoint := range request.Checkpoints {
		key := chatPrefixIndexKey{Scope: scope, Sample: checkpoint.Sample}
		if cache.index[key] == nil {
			cache.index[key] = make(map[int]*list.Element)
		}
		if indexed := cache.index[key][channel.ID]; indexed != nil && len(checkpoint.Sample) < len(request.Checkpoints) {
			stored := indexed.Value.(*chatPrefixEntry)
			if stored.Channel == channel && len(stored.Request.Checkpoints) == len(checkpoint.Sample) {
				// Keep an exact shorter key addressable when a longer history
				// updates their shared prefix, so later short writes deduplicate.
				continue
			}
		}
		cache.index[key][channel.ID] = element
	}
}

// ChatPrefixMatches only scores candidates already admitted by routing policy.
func (p *RetryParam) ChatPrefixMatches(group string, candidates []model.Ability) map[int]ChatPrefixMatch {
	request := p.chatPrefixRequest()
	if request == nil {
		return nil
	}
	scope := chatPrefixScope{UserID: common.GetContextKeyInt(p.Ctx, constant.ContextKeyUserId), Group: group, Model: p.ModelName}
	setting := model.CurrentRequestPolicy()
	ttl := time.Duration(setting.ChatPrefixTTLSeconds) * time.Second
	var stored map[chatPrefixChannel]ChatPrefixMatch
	if common.RedisEnabled && common.RDB != nil {
		channels := make([]int, 0, len(candidates))
		for _, candidate := range candidates {
			channels = append(channels, candidate.ChannelId)
		}
		ctx, cancel := context.WithTimeout(p.Ctx.Request.Context(), 100*time.Millisecond)
		defer cancel()
		cache := chatPrefixRedisCache{client: common.RDB.WithTimeout(100 * time.Millisecond), capacity: setting.ChatPrefixMaxEntries, ttl: ttl}
		var err error
		stored, err = cache.lookup(ctx, scope, request, channels)
		if err != nil {
			common.SysError("chat prefix cache lookup failed: " + err.Error())
			return nil
		}
	} else {
		chatPrefixRoutingCache.configure(setting.ChatPrefixMaxEntries, ttl, time.Now())
		stored = chatPrefixRoutingCache.lookup(scope, request, time.Now())
	}
	if len(stored) == 0 {
		return nil
	}
	matches := make(map[int]ChatPrefixMatch)
	for _, candidate := range candidates {
		if p.HasAttempted(candidate.ChannelId) {
			continue
		}
		if p.Capacity != nil {
			if _, blocked := p.Capacity.blocked[candidate.ChannelId]; blocked {
				continue
			}
		}
		channel, err := model.CacheGetChannel(candidate.ChannelId)
		if err != nil || channel == nil {
			continue
		}
		if match, exists := stored[chatPrefixChannelIdentity(channel)]; exists &&
			match.MatchedBytes >= min(1024, match.HistoryBytes, request.Bytes) {
			// Shared JSON punctuation and key names alone must not make all
			// unrelated traffic sticky. Short complete sampled matches still count.
			matches[channel.Id] = match
		}
	}
	return matches
}

func (p *RetryParam) recordChatPrefixSelection(group string, channelID int, matches map[int]ChatPrefixMatch) {
	if match := matches[channelID]; match.MatchedBytes > 0 {
		RequestPolicy(p.Ctx).AddEvent(PolicyEvent{ChannelID: channelID, Group: group,
			Decision: PolicyDecision{Action: "match", Reason: "chat_prefix_match", Source: "prefix_cache"}})
	}
}

func RecordChatPrefixSuccess(param *RetryParam, channelID int) {
	if param == nil || channelID <= 0 || !RequestPolicy(param.Ctx).Successful {
		return
	}
	request := param.chatPrefixRequest()
	if request == nil {
		return
	}
	group := param.TokenGroup
	if group == "auto" {
		group = common.GetContextKeyString(param.Ctx, constant.ContextKeyAutoGroup)
	}
	if group == "" || group == "auto" {
		return
	}
	// Use the immutable identity captured when middleware bound this attempt,
	// not a configuration that may have changed while the stream was running.
	value, exists := param.Ctx.Get(chatPrefixChannelContextKey)
	channel, ok := value.(chatPrefixChannel)
	if !exists || !ok || channel.ID != channelID {
		return
	}
	scope := chatPrefixScope{UserID: common.GetContextKeyInt(param.Ctx, constant.ContextKeyUserId), Group: group, Model: param.ModelName}
	setting := model.CurrentRequestPolicy()
	ttl := time.Duration(setting.ChatPrefixTTLSeconds) * time.Second
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(param.Ctx.Request.Context(), 100*time.Millisecond)
		defer cancel()
		cache := chatPrefixRedisCache{client: common.RDB.WithTimeout(100 * time.Millisecond), capacity: setting.ChatPrefixMaxEntries, ttl: ttl}
		if err := cache.record(ctx, scope, channel, request); err != nil {
			common.SysError("chat prefix cache write failed: " + err.Error())
		}
		return
	}
	chatPrefixRoutingCache.configure(setting.ChatPrefixMaxEntries, ttl, time.Now())
	chatPrefixRoutingCache.record(scope, channel, request, time.Now())
}
