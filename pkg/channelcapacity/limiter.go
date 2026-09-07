// Package channelcapacity admits upstream attempts in fixed, epoch-aligned
// minute windows. A rejected attempt never consumes either counter.
package channelcapacity

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

const MaxLimit int64 = 1<<53 - 1

type Key struct {
	ChannelID int
	Model     string
}
type Limits struct{ RPM, TPM int64 }
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}
type usage struct{ window, rpm, tpm int64 }

type Limiter struct {
	mu     sync.Mutex
	usage  map[Key]usage
	window int64
	redis  redis.UniversalClient
}

func NewMemoryLimiter() *Limiter                            { return &Limiter{usage: make(map[Key]usage)} }
func NewRedisLimiter(client redis.UniversalClient) *Limiter { return &Limiter{redis: client} }

func (l *Limiter) Acquire(ctx context.Context, key Key, limits Limits, tokens int64, now time.Time) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	if key.ChannelID <= 0 || strings.TrimSpace(key.Model) == "" || limits.RPM < 0 || limits.TPM < 0 || limits.RPM > MaxLimit || limits.TPM > MaxLimit || tokens < 0 || tokens > MaxLimit+1 {
		return Decision{}, errors.New("invalid channel capacity reservation")
	}
	if limits.RPM == 0 && limits.TPM == 0 {
		return Decision{Allowed: true}, nil
	}
	if l.redis != nil {
		name := fmt.Sprintf("channelModelCapacity:v1:%d:%s", key.ChannelID, base64.RawURLEncoding.EncodeToString([]byte(key.Model)))
		values, err := acquireScript.Run(ctx, l.redis, []string{name}, limits.RPM, limits.TPM, tokens).Int64Slice()
		if err != nil {
			return Decision{}, err
		}
		if len(values) != 2 {
			return Decision{}, errors.New("invalid Redis capacity response")
		}
		return Decision{Allowed: values[0] == 1, RetryAfter: time.Duration(values[1]) * time.Millisecond}, nil
	}
	if l.usage == nil {
		return Decision{}, errors.New("channel capacity backend is unavailable")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	window := now.Unix() / 60
	if window > l.window {
		for key, value := range l.usage {
			if value.window < window {
				delete(l.usage, key)
			}
		}
		l.window = window
	}
	value := l.usage[key]
	if value.window != window {
		value = usage{window: window}
	}
	retry := time.Unix((window+1)*60, 0).Sub(now)
	if limits.RPM > 0 && value.rpm >= limits.RPM || limits.TPM > 0 && (tokens > limits.TPM || value.tpm > limits.TPM-tokens) {
		return Decision{RetryAfter: retry}, nil
	}
	if limits.RPM > 0 {
		value.rpm++
	}
	if limits.TPM > 0 {
		value.tpm += tokens
	}
	l.usage[key] = value
	return Decision{Allowed: true, RetryAfter: retry}, nil
}

// Redis server time gives replicas a shared window even when local clocks differ.
var acquireScript = redis.NewScript(`
redis.replicate_commands()
local clock = redis.call('TIME')
local seconds = tonumber(clock[1])
local window = math.floor(seconds / 60)
local retry = 60000 - (seconds % 60) * 1000 - math.floor(tonumber(clock[2]) / 1000)
if tonumber(redis.call('HGET', KEYS[1], 'window') or '-1') ~= window then
  redis.call('HSET', KEYS[1], 'window', window, 'rpm', 0, 'tpm', 0)
end
local rpm = tonumber(redis.call('HGET', KEYS[1], 'rpm') or '0')
local tpm = tonumber(redis.call('HGET', KEYS[1], 'tpm') or '0')
local rpm_limit, tpm_limit, tokens = tonumber(ARGV[1]), tonumber(ARGV[2]), tonumber(ARGV[3])
redis.call('PEXPIRE', KEYS[1], retry + 60000)
if (rpm_limit > 0 and rpm >= rpm_limit) or (tpm_limit > 0 and (tokens > tpm_limit or tpm > tpm_limit - tokens)) then
  return {0, retry}
end
if rpm_limit > 0 then redis.call('HINCRBY', KEYS[1], 'rpm', 1) end
if tpm_limit > 0 and tokens > 0 then redis.call('HINCRBY', KEYS[1], 'tpm', tokens) end
return {1, retry}
`)
