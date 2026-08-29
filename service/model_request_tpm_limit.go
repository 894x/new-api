package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

const (
	modelRequestTPMWindowSeconds = int64(60)
	modelRequestTPMRedisMark     = "MRTPM"
	modelRequestTPMRedisTimeout  = 5 * time.Second
)

// TPM uses the same fixed-window semantics as the existing request limiter.
// Reservations are atomic, then reconciled with the upstream-reported usage.
const modelRequestTPMReserveScript = `
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
local requested = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local duration = tonumber(ARGV[3])
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  ttl = duration
end
if current + requested > limit then
  return {0, current, ttl}
end
local next = current
if requested > 0 then
  next = redis.call('INCRBY', KEYS[1], requested)
  local stored_ttl = redis.call('TTL', KEYS[1])
  if stored_ttl < 0 then
    redis.call('EXPIRE', KEYS[1], duration)
    stored_ttl = duration
  end
  ttl = stored_ttl
end
return {1, next, ttl}
`

const modelRequestTPMSettleScript = `
local actual = tonumber(ARGV[1])
local estimated = tonumber(ARGV[2])
local duration = tonumber(ARGV[3])
if redis.call('EXISTS', KEYS[1]) == 0 then
  if actual > 0 then
    redis.call('SET', KEYS[1], actual, 'EX', duration)
  end
  return actual
end
local delta = actual - estimated
local next = redis.call('INCRBY', KEYS[1], delta)
if next < 0 then
  redis.call('SET', KEYS[1], 0)
  next = 0
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  redis.call('EXPIRE', KEYS[1], duration)
end
return next
`

type modelRequestTPMReservation struct {
	mutex           sync.Mutex
	key             string
	estimatedTokens int64
	useRedis        bool
	settled         bool
}

type modelRequestTPMMemoryWindow struct {
	tokens    int64
	expiresAt time.Time
}

var modelRequestTPMMemoryStore = struct {
	sync.Mutex
	windows       map[string]modelRequestTPMMemoryWindow
	lastCleanupAt time.Time
}{
	windows: make(map[string]modelRequestTPMMemoryWindow),
}

func ResolveModelRequestTPMLimit(c *gin.Context) int {
	if !setting.ModelRequestRateLimitEnabled {
		return 0
	}

	limit := setting.ModelRequestRateLimitTPM
	group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	_, _, groupTPM, found := setting.GetGroupRateLimit(group)
	if found {
		limit = groupTPM
	}
	if limit < 0 {
		return 0
	}
	return limit
}

func ReserveModelRequestTPM(c *gin.Context, userID, limit, estimatedTokens int) (bool, int64, error) {
	if limit <= 0 {
		return true, 0, nil
	}
	if userID <= 0 {
		return false, 0, errors.New("model request TPM user ID must be positive")
	}
	if estimatedTokens < 0 {
		estimatedTokens = 0
	}

	key := fmt.Sprintf("rateLimit:v2:user:%s:%d", modelRequestTPMRedisMark, userID)
	requested := int64(estimatedTokens)
	if common.RedisEnabled {
		allowed, retryAfter, err := reserveRedisModelRequestTPM(c, key, int64(limit), requested)
		if err != nil || !allowed {
			return allowed, retryAfter, err
		}
		common.SetContextKey(c, constant.ContextKeyModelRequestTPMReservation, &modelRequestTPMReservation{
			key:             key,
			estimatedTokens: requested,
			useRedis:        true,
		})
		return true, retryAfter, nil
	}

	allowed, retryAfter := reserveMemoryModelRequestTPM(key, int64(limit), requested)
	if !allowed {
		return false, retryAfter, nil
	}
	common.SetContextKey(c, constant.ContextKeyModelRequestTPMReservation, &modelRequestTPMReservation{
		key:             key,
		estimatedTokens: requested,
	})
	return true, retryAfter, nil
}

func SettleModelRequestTPM(c *gin.Context, promptTokens, completionTokens int) error {
	reservation, ok := common.GetContextKeyType[*modelRequestTPMReservation](c, constant.ContextKeyModelRequestTPMReservation)
	if !ok || reservation == nil {
		return nil
	}

	reservation.mutex.Lock()
	defer reservation.mutex.Unlock()
	if reservation.settled {
		return nil
	}

	actualTokens := modelRequestTPMTokenTotal(promptTokens, completionTokens)
	if reservation.useRedis {
		if err := settleRedisModelRequestTPM(reservation.key, reservation.estimatedTokens, actualTokens); err != nil {
			return err
		}
	} else {
		settleMemoryModelRequestTPM(reservation.key, reservation.estimatedTokens, actualTokens)
	}
	reservation.settled = true
	return nil
}

func reserveRedisModelRequestTPM(c *gin.Context, key string, limit, requested int64) (bool, int64, error) {
	if common.RDB == nil {
		return false, 0, errors.New("Redis client is not initialized")
	}

	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	values, err := common.RDB.Eval(
		ctx,
		modelRequestTPMReserveScript,
		[]string{key},
		requested,
		limit,
		modelRequestTPMWindowSeconds,
	).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(values) != 3 {
		return false, 0, fmt.Errorf("unexpected Redis TPM rate limit reply length %d", len(values))
	}

	allowedValue, err := modelRequestTPMRedisInteger(values[0])
	if err != nil {
		return false, 0, err
	}
	retryAfter, err := modelRequestTPMRedisInteger(values[2])
	if err != nil {
		return false, 0, err
	}
	return allowedValue == 1, retryAfter, nil
}

func settleRedisModelRequestTPM(key string, estimated, actual int64) error {
	if common.RDB == nil {
		return errors.New("Redis client is not initialized")
	}
	// Settlement is accounting work and must survive a disconnected client.
	ctx, cancel := context.WithTimeout(context.Background(), modelRequestTPMRedisTimeout)
	defer cancel()
	return common.RDB.Eval(
		ctx,
		modelRequestTPMSettleScript,
		[]string{key},
		actual,
		estimated,
		modelRequestTPMWindowSeconds,
	).Err()
}

func modelRequestTPMRedisInteger(value any) (int64, error) {
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

func reserveMemoryModelRequestTPM(key string, limit, requested int64) (bool, int64) {
	now := time.Now()
	modelRequestTPMMemoryStore.Lock()
	defer modelRequestTPMMemoryStore.Unlock()

	cleanupModelRequestTPMMemoryWindows(now)
	window, found := modelRequestTPMMemoryStore.windows[key]
	if !found || !now.Before(window.expiresAt) {
		window = modelRequestTPMMemoryWindow{expiresAt: now.Add(time.Duration(modelRequestTPMWindowSeconds) * time.Second)}
	}
	retryAfter := int64((time.Until(window.expiresAt) + time.Second - 1) / time.Second)
	if retryAfter < 1 {
		retryAfter = 1
	}
	if window.tokens+requested > limit {
		return false, retryAfter
	}
	window.tokens += requested
	modelRequestTPMMemoryStore.windows[key] = window
	return true, retryAfter
}

func settleMemoryModelRequestTPM(key string, estimated, actual int64) {
	now := time.Now()
	modelRequestTPMMemoryStore.Lock()
	defer modelRequestTPMMemoryStore.Unlock()

	window, found := modelRequestTPMMemoryStore.windows[key]
	if !found || !now.Before(window.expiresAt) {
		window = modelRequestTPMMemoryWindow{
			tokens:    actual,
			expiresAt: now.Add(time.Duration(modelRequestTPMWindowSeconds) * time.Second),
		}
		modelRequestTPMMemoryStore.windows[key] = window
		return
	}
	window.tokens += actual - estimated
	if window.tokens < 0 {
		window.tokens = 0
	}
	modelRequestTPMMemoryStore.windows[key] = window
}

func cleanupModelRequestTPMMemoryWindows(now time.Time) {
	if now.Sub(modelRequestTPMMemoryStore.lastCleanupAt) < time.Minute {
		return
	}
	for key, window := range modelRequestTPMMemoryStore.windows {
		if !now.Before(window.expiresAt) {
			delete(modelRequestTPMMemoryStore.windows, key)
		}
	}
	modelRequestTPMMemoryStore.lastCleanupAt = now
}

func modelRequestTPMTokenTotal(promptTokens, completionTokens int) int64 {
	prompt := int64(promptTokens)
	if prompt < 0 {
		prompt = 0
	}
	completion := int64(completionTokens)
	if completion < 0 {
		completion = 0
	}
	if prompt > int64(common.MaxQuota)-completion {
		return int64(common.MaxQuota)
	}
	return prompt + completion
}
