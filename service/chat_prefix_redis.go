package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

var chatPrefixRedisKeys = []string{
	"new-api:{chat-prefix:v1}:records",
	"new-api:{chat-prefix:v1}:order",
	"new-api:{chat-prefix:v1}:index",
	"new-api:{chat-prefix:v1}:sequence",
}

// One script owns both the records and their checkpoint index. Redis serializes
// it across instances, including continuation replacement and capacity eviction.
// TIME avoids making expiry or ordering depend on application clock differences.
var chatPrefixRedisScript = redis.NewScript(`
local capacity, ttl = tonumber(ARGV[2]), tonumber(ARGV[3])
if capacity == 0 then
  redis.call('DEL', unpack(KEYS))
  return {}
end
-- A Redis eviction can remove one structure independently. Start cold if the
-- set is incomplete so resetting the sequence cannot reuse a live record ID.
local structures = redis.call('EXISTS', unpack(KEYS))
if structures > 0 and structures < #KEYS then redis.call('DEL', unpack(KEYS)) end
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local cutoff = now - ttl
local function field(scope, sample, channel)
  return scope .. ':' .. sample .. ':' .. tostring(channel)
end
local function remove(id)
  local raw = redis.call('HGET', KEYS[1], id)
  if raw then
    local entry = cjson.decode(raw)
    for i = 2, #entry.k, 2 do
      local key = field(entry.s, string.sub(entry.k, 1, i), entry.c)
      if redis.call('HGET', KEYS[3], key) == id then
        redis.call('HDEL', KEYS[3], key)
      end
    end
    redis.call('HDEL', KEYS[1], id)
  end
  redis.call('ZREM', KEYS[2], id)
end
-- Bound routine expiry work; reads also check timestamps for entries left in
-- this batch. Whole-key expiry reclaims everything when traffic becomes idle.
for _, id in ipairs(redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', cutoff, 'LIMIT', 0, 128)) do
  remove(id)
end
local excess = redis.call('ZCARD', KEYS[2]) - capacity
if excess > 0 then
  for _, id in ipairs(redis.call('ZRANGE', KEYS[2], 0, excess - 1)) do remove(id) end
end
local scope, sample = ARGV[4], ARGV[5]
if ARGV[1] == 'lookup' then
  local found, result = {}, {}
  for _, channel in ipairs(cjson.decode(ARGV[7])) do
    for i = #sample, 2, -2 do
      local id = redis.call('HGET', KEYS[3], field(scope, string.sub(sample, 1, i), channel))
      if id and not found[id] then
        found[id] = true
        local raw = redis.call('HGET', KEYS[1], id)
        if raw and cjson.decode(raw).t > cutoff then table.insert(result, raw) end
      end
    end
  end
  return result
end
local channel, size, version = tonumber(ARGV[7]), tonumber(ARGV[6]), ARGV[8]
local previous, previousSize = nil, -1
for i = #sample, 2, -2 do
  local id = redis.call('HGET', KEYS[3], field(scope, string.sub(sample, 1, i), channel))
  if id then
    local raw = redis.call('HGET', KEYS[1], id)
    if raw then
      local entry = cjson.decode(raw)
      local length = math.min(#sample, #entry.k)
      if entry.t > cutoff and entry.v == version and string.sub(sample, 1, length) == string.sub(entry.k, 1, length) then
        -- Equal complete keys keep the longer byte estimate. A distinct
        -- shorter key is inserted alongside the longer history.
        if entry.b > size and #entry.k == #sample then
          entry.t = now
          redis.call('HSET', KEYS[1], id, cjson.encode(entry))
          redis.call('ZADD', KEYS[2], now, id)
          for _, key in ipairs(KEYS) do redis.call('PEXPIRE', key, ttl) end
          return {}
        end
        if entry.b <= size and entry.b > previousSize then previous, previousSize = id, entry.b end
      end
    end
  end
end
if previous then remove(previous) end
if redis.call('ZCARD', KEYS[2]) >= capacity then
  local oldest = redis.call('ZRANGE', KEYS[2], 0, 0)
  if oldest[1] then remove(oldest[1]) end
end
local id = tostring(redis.call('INCR', KEYS[4]))
local entry = cjson.encode({s=scope, c=channel, v=version, k=sample, b=size, t=now})
redis.call('HSET', KEYS[1], id, entry)
redis.call('ZADD', KEYS[2], now, id)
for i = 2, #sample, 2 do
  local key = field(scope, string.sub(sample, 1, i), channel)
  local keepExact = false
  if i < #sample then
    local indexed = redis.call('HGET', KEYS[3], key)
    if indexed then
      local raw = redis.call('HGET', KEYS[1], indexed)
      if raw then
        local stored = cjson.decode(raw)
        keepExact = stored.t > cutoff and stored.v == version and #stored.k == i
      end
    end
  end
  if not keepExact then redis.call('HSET', KEYS[3], key, id) end
end
for _, key in ipairs(KEYS) do redis.call('PEXPIRE', key, ttl) end
return {}
`)

type chatPrefixRedisCache struct {
	client   *redis.Client
	capacity int
	ttl      time.Duration
}

// Sample is hex encoded because selected wire bytes need not form valid UTF-8.
// This is reversible encoding, not a hash of the original request body.
type chatPrefixRedisRecord struct {
	Scope     string `json:"s"`
	ChannelID int    `json:"c"`
	Version   string `json:"v"`
	Sample    string `json:"k"`
	Bytes     int    `json:"b"`
	SeenAt    int64  `json:"t"`
}

func (cache chatPrefixRedisCache) lookup(ctx context.Context, scope chatPrefixScope, request *chatPrefixRequest, channels []int) (map[chatPrefixChannel]ChatPrefixMatch, error) {
	scopeJSON, err := common.Marshal(scope)
	if err != nil {
		return nil, err
	}
	if channels == nil {
		channels = []int{}
	}
	channelJSON, err := common.Marshal(channels)
	if err != nil {
		return nil, err
	}
	sample := request.Checkpoints[len(request.Checkpoints)-1].Sample
	rows, err := chatPrefixRedisScript.Run(ctx, cache.client, chatPrefixRedisKeys,
		"lookup", cache.capacity, cache.ttl.Milliseconds(), string(scopeJSON), hex.EncodeToString([]byte(sample)), request.Bytes, string(channelJSON)).Slice()
	if err != nil {
		return nil, err
	}
	matches := make(map[chatPrefixChannel]ChatPrefixMatch)
	for _, row := range rows {
		raw, ok := row.(string)
		if !ok {
			return nil, fmt.Errorf("invalid prefix cache record")
		}
		var record chatPrefixRedisRecord
		if err := common.Unmarshal([]byte(raw), &record); err != nil {
			return nil, err
		}
		version, err := hex.DecodeString(record.Version)
		if err != nil || len(version) != len(chatPrefixChannel{}.Version) {
			return nil, fmt.Errorf("invalid prefix cache channel version")
		}
		samples, err := hex.DecodeString(record.Sample)
		if err != nil || len(samples) == 0 || record.Bytes <= 0 || record.Scope != string(scopeJSON) {
			return nil, fmt.Errorf("invalid prefix cache sample")
		}
		key := string(samples)
		stored := &chatPrefixRequest{Bytes: record.Bytes}
		next, previous := 0, 1
		for i := range len(samples) {
			if next >= record.Bytes {
				return nil, fmt.Errorf("invalid prefix cache sample length")
			}
			stored.Checkpoints = append(stored.Checkpoints, chatPrefixCheckpoint{Sample: key[:i+1], Bytes: next + 1})
			if next == 0 {
				next = 1
			} else {
				next, previous = next+previous, next
			}
		}
		channel := chatPrefixChannel{ID: record.ChannelID}
		copy(channel.Version[:], version)
		entry := chatPrefixEntry{Request: stored, SeenAt: time.UnixMilli(record.SeenAt)}
		match, old := entry.match(request), matches[channel]
		if match.MatchedBytes > old.MatchedBytes || match.MatchedBytes == old.MatchedBytes && match.ObservedAt.After(old.ObservedAt) {
			matches[channel] = match
		}
	}
	return matches, nil
}

func (cache chatPrefixRedisCache) record(ctx context.Context, scope chatPrefixScope, channel chatPrefixChannel, request *chatPrefixRequest) error {
	scopeJSON, err := common.Marshal(scope)
	if err != nil {
		return err
	}
	sample := request.Checkpoints[len(request.Checkpoints)-1].Sample
	return chatPrefixRedisScript.Run(ctx, cache.client, chatPrefixRedisKeys,
		"record", cache.capacity, cache.ttl.Milliseconds(), string(scopeJSON), hex.EncodeToString([]byte(sample)), request.Bytes, channel.ID, hex.EncodeToString(channel.Version[:])).Err()
}
