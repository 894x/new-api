package channelcapacity

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

const redisAcquireScript = `
redis.replicate_commands()
local current_time = redis.call('TIME')
local now_seconds = tonumber(current_time[1])
local now_microseconds = tonumber(current_time[2])
local current_window = math.floor(now_seconds / 60)
local stored_window = tonumber(redis.call('HGET', KEYS[1], 'window') or '-1')
if stored_window ~= current_window then
  redis.call('HSET', KEYS[1], 'window', current_window, 'rpm', 0, 'tpm', 0)
end

local used_rpm = tonumber(redis.call('HGET', KEYS[1], 'rpm') or '0')
local used_tpm = tonumber(redis.call('HGET', KEYS[1], 'tpm') or '0')
local rpm_limit = tonumber(ARGV[1])
local tpm_limit = tonumber(ARGV[2])
local requested_tokens = tonumber(ARGV[3])
local rpm_exceeded = rpm_limit > 0 and used_rpm > rpm_limit - 1
local tpm_exceeded = tpm_limit > 0 and (requested_tokens > tpm_limit or used_tpm > tpm_limit - requested_tokens)
local elapsed_ms = (now_seconds % 60) * 1000 + math.floor(now_microseconds / 1000)
local retry_after_ms = 60000 - elapsed_ms
redis.call('PEXPIRE', KEYS[1], retry_after_ms + 60000)

if rpm_exceeded or tpm_exceeded then
  local limited_by = 0
  if rpm_exceeded then limited_by = limited_by + 1 end
  if tpm_exceeded then limited_by = limited_by + 2 end
  return {0, used_rpm, used_tpm, limited_by, retry_after_ms}
end

if rpm_limit > 0 then
  used_rpm = redis.call('HINCRBY', KEYS[1], 'rpm', 1)
end
if tpm_limit > 0 and requested_tokens > 0 then
  used_tpm = redis.call('HINCRBY', KEYS[1], 'tpm', requested_tokens)
end
return {1, used_rpm, used_tpm, 0, retry_after_ms}
`

var redisAcquire = redis.NewScript(redisAcquireScript)

type RedisLimiter struct {
	client redis.UniversalClient
	prefix string
}

func NewRedisLimiter(client redis.UniversalClient, prefix string) *RedisLimiter {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "channelModelCapacity:v1"
	}
	return &RedisLimiter{client: client, prefix: prefix}
}

func (l *RedisLimiter) Acquire(ctx context.Context, key Key, limits Limits, tokens int64, _ time.Time) (Decision, error) {
	if err := validateAcquire(key, limits, tokens); err != nil {
		return Decision{}, err
	}
	if limits.RPM == 0 && limits.TPM == 0 {
		return Decision{Allowed: true}, nil
	}
	if l == nil || l.client == nil {
		return Decision{}, errors.New("Redis client is not initialized")
	}

	values, err := redisAcquire.Run(
		ctx,
		l.client,
		[]string{l.redisKey(key)},
		limits.RPM,
		limits.TPM,
		tokens,
	).Slice()
	if err != nil {
		return Decision{}, err
	}
	if len(values) != 5 {
		return Decision{}, fmt.Errorf("unexpected Redis capacity reply length %d", len(values))
	}

	allowed, err := redisInteger(values[0])
	if err != nil {
		return Decision{}, err
	}
	usedRPM, err := redisInteger(values[1])
	if err != nil {
		return Decision{}, err
	}
	usedTPM, err := redisInteger(values[2])
	if err != nil {
		return Decision{}, err
	}
	limitedCode, err := redisInteger(values[3])
	if err != nil {
		return Decision{}, err
	}
	retryAfterMilliseconds, err := redisInteger(values[4])
	if err != nil {
		return Decision{}, err
	}

	return Decision{
		Allowed:    allowed == 1,
		LimitedBy:  redisLimitedBy(limitedCode),
		UsedRPM:    usedRPM,
		UsedTPM:    usedTPM,
		RetryAfter: time.Duration(retryAfterMilliseconds) * time.Millisecond,
	}, nil
}

func (l *RedisLimiter) redisKey(key Key) string {
	model := base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(key.Model)))
	return fmt.Sprintf("%s:%d:%s", l.prefix, key.ChannelID, model)
}

func redisInteger(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis integer reply type %T", value)
	}
}

func redisLimitedBy(code int64) LimitKind {
	switch code {
	case 1:
		return LimitRPM
	case 2:
		return LimitTPM
	case 3:
		return LimitBoth
	default:
		return LimitNone
	}
}
