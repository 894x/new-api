package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const cacheHitPolicyContextKey = "user_cache_hit_policy"

var cacheHitPolicyDayZone = time.FixedZone("Asia/Shanghai", 8*60*60)

type CacheHitPolicyDailyUsage struct {
	Day             string `json:"day"`
	InputTokens     int64  `json:"input_tokens"`
	RealCacheTokens int64  `json:"real_cache_tokens"`
	BillCacheTokens int64  `json:"bill_cache_tokens"`
}

type CacheHitPolicyDecision struct {
	CacheHitPolicyDailyUsage
	RequestInputTokens int `json:"request_input_tokens"`
	RealCachedTokens   int `json:"real_cached_tokens"`
	BilledCachedTokens int `json:"billed_cached_tokens"`
	GapTokens          int `json:"gap_tokens"`
	TargetBPS          int `json:"target_bps"`
}

type cacheHitPolicyState struct {
	mu       sync.Mutex
	userID   int
	model    string
	request  string
	day      string
	policy   model.UserCacheHitPolicy
	decision *CacheHitPolicyDecision
	failure  string
	settled  bool
}

// Lua serializes the weighted daily cursor across replicas and deduplicates
// repeated usage events. Revised final input facts replace this request's
// reservation while retaining its original random target.
// Real low-cache requests retain their real usage; the range only samples a
// target ceiling and never manufactures cache hits or rewrites past invoices.
var cacheHitPolicyAllocate = redis.NewScript(`
local saved = redis.call('GET', KEYS[2])
local p = tonumber(ARGV[1])
local r = tonumber(ARGV[2])
local lower = tonumber(ARGV[3])
local target = tonumber(ARGV[4])
local dp = tonumber(redis.call('HGET', KEYS[1], 'input') or '0')
local dr = tonumber(redis.call('HGET', KEYS[1], 'real') or '0')
local db = tonumber(redis.call('HGET', KEYS[1], 'billed') or '0')
if saved then
  local previous = cjson.decode(saved)
  if previous.released then return redis.error_reply('cache hit reservation already released') end
  if previous.request_input == p and previous.request_real == r then return saved end
  dp = dp - previous.request_input
  dr = dr - previous.request_real
  db = db - previous.request_billed
  target = previous.target
end
if dp < 0 or dr < 0 or db < 0 or dr > dp or db > dr or dp + p > 1000000000000 then
  return redis.error_reply('invalid cache hit daily cursor')
end
local billed = r
if r / p >= lower / 10000 and (dr + r) / (dp + p) >= lower / 10000 then
  billed = math.min(r, math.max(0, math.floor((dp + p) * (target / 10000)) - db))
end
dp = dp + p
dr = dr + r
db = db + billed
local result = cjson.encode({input=dp, real=dr, billed=db, request_input=p, request_real=r, request_billed=billed, target=target})
redis.call('HSET', KEYS[1], 'input', dp, 'real', dr, 'billed', db)
redis.call('EXPIREAT', KEYS[1], ARGV[5])
redis.call('SET', KEYS[2], result)
redis.call('EXPIREAT', KEYS[2], ARGV[5])
return result
`)

var cacheHitPolicyRelease = redis.NewScript(`
local saved = redis.call('GET', KEYS[2])
if not saved then return 0 end
local result = cjson.decode(saved)
if result.released then return 0 end
if redis.call('EXISTS', KEYS[1]) == 1 then
  redis.call('HINCRBY', KEYS[1], 'input', -result.request_input)
  redis.call('HINCRBY', KEYS[1], 'real', -result.request_real)
  redis.call('HINCRBY', KEYS[1], 'billed', -result.request_billed)
end
local ttl = redis.call('TTL', KEYS[2])
result.released = true
redis.call('SET', KEYS[2], cjson.encode(result))
if ttl > 0 then redis.call('EXPIRE', KEYS[2], ttl) end
return 1
`)

// Failed attempts that never reach settlement do not consume the daily budget.
// Partial usage billed after downstream disconnects remains in the ledger.
func FinishUserCacheHitPolicy(c *gin.Context) {
	s, ok := common.GetContextKeyType[*cacheHitPolicyState](c, cacheHitPolicyContextKey)
	if !ok || common.RDB == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled || s.decision == nil {
		return
	}
	key := cacheHitPolicyDailyKey(s.userID, s.model, s.day)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := cacheHitPolicyRelease.Run(ctx, common.RDB, []string{key, key + ":request:" + fmt.Sprintf("%x", sha256.Sum256([]byte(s.request)))}).Err(); err != nil {
		common.SysError("cache hit policy release failed: " + err.Error())
	}
}

func markUserCacheHitPolicySettled(c *gin.Context) {
	if s, ok := common.GetContextKeyType[*cacheHitPolicyState](c, cacheHitPolicyContextKey); ok {
		s.mu.Lock()
		s.settled = true
		s.mu.Unlock()
	}
}

func cacheHitPolicyDailyKey(userID int, modelName, day string) string {
	return fmt.Sprintf("cache-hit-policy:{%d:%x:%s}", userID, sha256.Sum256([]byte(modelName)), day)
}

// The request's Beijing day is frozen at admission, even for streams crossing
// midnight. Unconfigured users only incur a primary-option lookup, no Redis IO.
func BeginUserCacheHitPolicy(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil || c.Request == nil || c.Request.Method != "POST" {
		return
	}
	switch c.Request.URL.Path {
	case "/v1/chat/completions", "/v1/completions", "/v1/messages", "/v1/responses", "/v1/responses/compact":
	default:
		return
	}
	policies, _, err := model.GetUserCacheHitPolicies(info.UserId)
	if err != nil {
		common.SysError("cache hit policy lookup failed: " + err.Error())
		return
	}
	policy, ok := policies[info.OriginModelName]
	if !ok || !policy.Enabled {
		return
	}
	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		requestID = common.NewRequestId()
	}
	admittedAt := info.StartTime
	if admittedAt.IsZero() {
		admittedAt = time.Now()
	}
	c.Set(cacheHitPolicyContextKey, &cacheHitPolicyState{
		userID: info.UserId, model: info.OriginModelName, request: requestID,
		day: admittedAt.In(cacheHitPolicyDayZone).Format("2006-01-02"), policy: policy,
	})
}

func (s *cacheHitPolicyState) allocate(inputTokens, realCached int) *CacheHitPolicyDecision {
	if s.decision != nil && (s.settled || (s.decision.RequestInputTokens == inputTokens && s.decision.RealCachedTokens == realCached)) {
		return s.decision
	}
	if s.failure != "" {
		return s.decision
	}
	if inputTokens <= 0 || realCached < 0 || realCached > inputTokens || inputTokens > 2147483647 {
		return nil
	}
	if !common.RedisEnabled || common.RDB == nil {
		s.failure = "redis_unavailable"
		return s.decision
	}
	target, err := rand.Int(rand.Reader, big.NewInt(int64(s.policy.MaxBPS-s.policy.MinBPS+1)))
	if err != nil {
		s.failure = "random_target_unavailable"
		return nil
	}
	day, err := time.ParseInLocation("2006-01-02", s.day, cacheHitPolicyDayZone)
	if err != nil {
		s.failure = "invalid_day"
		return nil
	}
	key := cacheHitPolicyDailyKey(s.userID, s.model, s.day)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := cacheHitPolicyAllocate.Run(ctx, common.RDB, []string{key, key + ":request:" + fmt.Sprintf("%x", sha256.Sum256([]byte(s.request)))},
		inputTokens, realCached, s.policy.MinBPS, s.policy.MinBPS+int(target.Int64()), day.AddDate(0, 0, 3).Unix()).Text()
	if err != nil {
		s.failure = "daily_counter_unavailable"
		common.SysError("cache hit policy allocation failed: " + err.Error())
		return s.decision
	}
	var cursor struct {
		Input         int64 `json:"input"`
		Real          int64 `json:"real"`
		Billed        int64 `json:"billed"`
		RequestInput  int   `json:"request_input"`
		RequestReal   int   `json:"request_real"`
		RequestBilled int   `json:"request_billed"`
		Target        int   `json:"target"`
	}
	if err := common.UnmarshalJsonStr(result, &cursor); err != nil || cursor.RequestInput != inputTokens || cursor.RequestReal != realCached || cursor.RequestBilled < 0 || cursor.RequestBilled > realCached {
		s.failure = "invalid_daily_decision"
		return nil
	}
	s.decision = &CacheHitPolicyDecision{
		CacheHitPolicyDailyUsage: CacheHitPolicyDailyUsage{Day: s.day, InputTokens: cursor.Input, RealCacheTokens: cursor.Real, BillCacheTokens: cursor.Billed},
		RequestInputTokens:       inputTokens, RealCachedTokens: realCached, BilledCachedTokens: cursor.RequestBilled,
		GapTokens: realCached - cursor.RequestBilled, TargetBPS: cursor.Target,
	}
	return s.decision
}

func GetUserCacheHitDailyUsage(userID int, modelName string) (CacheHitPolicyDailyUsage, error) {
	usage := CacheHitPolicyDailyUsage{Day: time.Now().In(cacheHitPolicyDayZone).Format("2006-01-02")}
	if !common.RedisEnabled || common.RDB == nil {
		return usage, errors.New("Redis is required for daily cache hit policy statistics")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cursor, err := common.RDB.HGetAll(ctx, cacheHitPolicyDailyKey(userID, modelName, usage.Day)).Result()
	if err != nil {
		return usage, err
	}
	for key, dst := range map[string]*int64{"input": &usage.InputTokens, "real": &usage.RealCacheTokens, "billed": &usage.BillCacheTokens} {
		if value := cursor[key]; value != "" {
			*dst, err = strconv.ParseInt(value, 10, 64)
			if err != nil {
				return usage, err
			}
		}
	}
	if usage.InputTokens < 0 || usage.RealCacheTokens < 0 || usage.BillCacheTokens < 0 || usage.RealCacheTokens > usage.InputTokens || usage.BillCacheTokens > usage.RealCacheTokens {
		return usage, errors.New("invalid cache hit daily statistics")
	}
	return usage, nil
}

// Keep raw canonical accounting facts intact for channel metrics and audits.
// Only the separate settlement copy loses the discount; writes and total
// context length are preserved for both inclusive and Anthropic semantics.
func cacheHitPolicyInputTokens(info *relaycommon.RelayInfo, usage *dto.Usage) (int, bool) {
	exclusive := usageSemanticFromUsage(info, usage) == dto.BillingUsageSemanticAnthropic
	if info != nil && info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeOpenRouter {
		exclusive = false // OpenRouter reports inclusive prompt counts.
	}
	input := usage.PromptTokens
	if exclusive {
		input += usage.PromptTokensDetails.CachedTokens + usage.PromptTokensDetails.CacheCreationTokensTotal()
	}
	return input, exclusive
}

func applyUserCacheHitBilling(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) *dto.Usage {
	s, ok := common.GetContextKeyType[*cacheHitPolicyState](c, cacheHitPolicyContextKey)
	if !ok || usage == nil {
		return usage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if details := usage.PromptTokensDetails.CachedTokensDetails; details != nil && ((details.ImageTokens != nil && *details.ImageTokens > 0) || (details.AudioTokens != nil && *details.AudioTokens > 0)) && s.decision == nil {
		s.failure = "multimodal_cache_breakdown"
		return usage
	}
	inputTokens, exclusive := cacheHitPolicyInputTokens(info, usage)
	if s.allocate(inputTokens, usage.PromptTokensDetails.CachedTokens) == nil {
		return usage
	}
	adjusted := *usage
	realCached := usage.PromptTokensDetails.CachedTokens
	gap := min(s.decision.GapTokens, max(realCached, 0))
	adjusted.PromptTokensDetails.CachedTokens = realCached - gap
	adjusted.CachedTokens = adjusted.PromptTokensDetails.CachedTokens
	adjusted.PromptCacheHitTokens = adjusted.PromptTokensDetails.CachedTokens
	if usage.PromptTokensDetails.CachedTokensDetails != nil {
		details := *usage.PromptTokensDetails.CachedTokensDetails
		if details.TextTokens != nil {
			details.TextTokens = common.GetPointer(adjusted.PromptTokensDetails.CachedTokens)
		}
		adjusted.PromptTokensDetails.CachedTokensDetails = &details
	}
	if usage.InputTokensDetails != nil {
		details := *usage.InputTokensDetails
		details.CachedTokens = adjusted.PromptTokensDetails.CachedTokens
		adjusted.InputTokensDetails = &details
	}
	if exclusive {
		adjusted.PromptTokens += gap
		adjusted.TotalTokens += gap // this legacy field excludes separately reported cache
	}
	return &adjusted
}

func appendUserCacheHitPolicyLog(c *gin.Context, info *relaycommon.RelayInfo, other *model.LogOther, realUsage *dto.Usage) {
	s, ok := common.GetContextKeyType[*cacheHitPolicyState](c, cacheHitPolicyContextKey)
	if !ok || other == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data := map[string]any{"day": s.day, "min_bps": s.policy.MinBPS, "max_bps": s.policy.MaxBPS}
	if s.decision != nil {
		encoded, err := common.Marshal(s.decision)
		if err == nil {
			_ = common.Unmarshal(encoded, &data)
		}
	}
	if realUsage != nil && s.decision != nil {
		input, _ := cacheHitPolicyInputTokens(info, realUsage)
		real := realUsage.PromptTokensDetails.CachedTokens
		gap := min(s.decision.GapTokens, max(real, 0))
		data["request_input_tokens"], data["real_cached_tokens"], data["billed_cached_tokens"], data["gap_tokens"] = input, real, real-gap, gap
	}
	if s.failure != "" {
		data["fallback_reason"] = s.failure
	} else if s.decision == nil {
		data["fallback_reason"] = "no_supported_client_usage"
	}
	if realUsage != nil {
		if s.decision == nil {
			data["request_input_tokens"], _ = cacheHitPolicyInputTokens(info, realUsage)
			data["real_cached_tokens"] = realUsage.PromptTokensDetails.CachedTokens
			data["billed_cached_tokens"] = realUsage.PromptTokensDetails.CachedTokens
			data["gap_tokens"] = 0
		}
		data["real_cache_write_tokens"] = realUsage.PromptTokensDetails.CacheCreationTokensTotal()
		data["real_cache_write_tokens_5m"] = realUsage.ClaudeCacheCreation5mTokens
		data["real_cache_write_tokens_1h"] = realUsage.ClaudeCacheCreation1hTokens
	}
	other.SetAdmin("cache_hit_policy", data)
}
