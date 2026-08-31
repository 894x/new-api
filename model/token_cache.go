package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var ErrTokenQuotaCacheStale = errors.New("token quota cache snapshot is stale")

func getTokenCacheKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

func getTokenCacheFenceKey(key string) string {
	return fmt.Sprintf("token:fence:%s", common.GenerateHMAC(key))
}

func tokenCacheMutationPending(key string) (bool, error) {
	if !common.RedisEnabled || key == "" {
		return false, nil
	}
	count, err := common.RDB.Exists(context.Background(), getTokenCacheFenceKey(key)).Result()
	return count > 0, err
}

func tokenCacheTTLSeconds() int {
	ttl := common.RedisKeyCacheSeconds()
	if ttl <= 0 {
		return 60
	}
	return ttl
}

// tokenCacheFenceSeconds must outlive a token mutation's database write plus
// any in-flight reader's DB-read-to-cache-init gap. The fence is not deleted
// after commit; it expires naturally so a reader holding a pre-mutation
// snapshot cannot publish it right after the mutation cleared the cache.
// While the fence exists readers simply serve the database without caching.
const tokenCacheFenceSeconds = 10

// invalidateTokenCacheForMutation is called before a token metadata mutation
// writes to the database: it raises the fence and drops the cached hash so no
// reader can act on (or re-publish) the pre-mutation state.
func invalidateTokenCacheForMutation(key string) error {
	if !common.RedisEnabled || key == "" {
		return nil
	}
	ctx := context.Background()
	err := common.RDB.Set(ctx, getTokenCacheFenceKey(key), 1, time.Duration(tokenCacheFenceSeconds)*time.Second).Err()
	if err != nil {
		return err
	}
	return common.RDB.Del(ctx, getTokenCacheKey(key)).Err()
}

func prepareTokenQuotaCacheMutation(key string, operation string) {
	if !common.RedisEnabled {
		return
	}
	if err := invalidateTokenCacheForMutation(key); err != nil {
		common.SysError(fmt.Sprintf("starting %s without a token quota cache fence: %v", operation, err))
	}
}

func finalizeTokenQuotaCacheMutation(key string, operation string) {
	if !common.RedisEnabled {
		return
	}
	if err := invalidateTokenCacheForMutation(key); err != nil {
		common.SysError(fmt.Sprintf("committed %s but failed to finalize token quota cache fence: %v", operation, err))
	}
}

func discardTokenQuotaCacheSnapshot(key string, quotaVersion int64) error {
	if !common.RedisEnabled {
		return nil
	}
	const script = `
if redis.call('HEXISTS', KEYS[1], 'QuotaVersion') == 1
  and tonumber(redis.call('HGET', KEYS[1], 'QuotaVersion')) == tonumber(ARGV[1]) then
  redis.call('SET', KEYS[2], '1', 'EX', ARGV[2])
  redis.call('DEL', KEYS[1])
  return 1
end
return 0`
	return common.RDB.Eval(
		context.Background(),
		script,
		[]string{getTokenCacheKey(key), getTokenCacheFenceKey(key)},
		quotaVersion,
		tokenCacheFenceSeconds,
	).Err()
}

// cacheInitToken publishes a database snapshot only when no mutation fence is
// active. A newer quota version replaces an older live hash atomically; an
// equal version only refreshes TTL so an in-flight same-generation cache delta
// cannot be overwritten.
// 返回值：0=被 fence 拦截，1=完成初始化或版本升级，2=相同版本仅刷新 TTL。
func cacheInitToken(token Token) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	var persisted struct {
		QuotaVersion int64
	}
	if err := DB.Model(&Token{}).
		Select("COALESCE(quota_version, 0) AS quota_version").
		Where("id = ? AND "+commonKeyCol+" = ?", token.Id, token.Key).
		Take(&persisted).Error; err != nil {
		return 0, err
	}
	if persisted.QuotaVersion != token.QuotaVersion {
		return 0, ErrTokenQuotaCacheStale
	}
	runQuotaCacheRaceHook(tokenQuotaCacheAfterVersionCheckHook())
	allowIps := ""
	if token.AllowIps != nil {
		allowIps = *token.AllowIps
	}
	const script = `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return 0
end
if redis.call('EXISTS', KEYS[1]) == 1 then
  if redis.call('HEXISTS', KEYS[1], 'QuotaVersion') == 0 then
    redis.call('DEL', KEYS[1])
  else
    local currentQuotaVersion = tonumber(redis.call('HGET', KEYS[1], 'QuotaVersion'))
    local incomingQuotaVersion = tonumber(ARGV[17])
    if currentQuotaVersion > incomingQuotaVersion then
      return 3
    end
    if currentQuotaVersion == incomingQuotaVersion then
      redis.call('EXPIRE', KEYS[1], ARGV[18])
      return 2
    end
  end
end
redis.call('HSET', KEYS[1],
  'Id', ARGV[1], 'UserId', ARGV[2], 'Status', ARGV[3], 'Name', ARGV[4],
  'CreatedTime', ARGV[5], 'AccessedTime', ARGV[6], 'ExpiredTime', ARGV[7],
  'UnlimitedQuota', ARGV[8], 'ModelLimitsEnabled', ARGV[9], 'ModelLimits', ARGV[10],
  'AllowIps', ARGV[11], 'Group', ARGV[12], 'CrossGroupRetry', ARGV[13],
  'AutoGroups', ARGV[14], 'RemainQuota', ARGV[15], 'UsedQuota', ARGV[16],
  'QuotaVersion', ARGV[17])
redis.call('EXPIRE', KEYS[1], ARGV[18])
return 1`

	result, err := common.RDB.Eval(context.Background(), script, []string{
		getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key),
	},
		token.Id, token.UserId, token.Status, token.Name,
		token.CreatedTime, token.AccessedTime, token.ExpiredTime,
		strconv.FormatBool(token.UnlimitedQuota), strconv.FormatBool(token.ModelLimitsEnabled),
		token.ModelLimits, allowIps, token.Group, strconv.FormatBool(token.CrossGroupRetry),
		token.AutoGroups, token.RemainQuota, token.UsedQuota, token.QuotaVersion,
		tokenCacheTTLSeconds(),
	).Int()
	if err != nil {
		return 0, err
	}
	if result == 3 {
		return 0, ErrTokenQuotaCacheStale
	}
	if result == 0 {
		return 0, nil
	}
	var current struct {
		QuotaVersion int64
	}
	postResult := DB.Model(&Token{}).
		Select("COALESCE(quota_version, 0) AS quota_version").
		Where("id = ? AND "+commonKeyCol+" = ?", token.Id, token.Key).
		Take(&current)
	if postResult.Error != nil || current.QuotaVersion != token.QuotaVersion {
		if discardErr := discardTokenQuotaCacheSnapshot(token.Key, token.QuotaVersion); discardErr != nil {
			return 0, discardErr
		}
		if postResult.Error != nil {
			return 0, postResult.Error
		}
		return 0, ErrTokenQuotaCacheStale
	}
	return result, nil
}

// cacheGetTokenByKey 从缓存读取 token；不完整的哈希（如仅有配额字段）会被拒绝。
func cacheGetTokenByKey(key string) (*Token, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	if err := common.RedisHGetObj(getTokenCacheKey(key), &token); err != nil {
		return nil, err
	}
	if token.Id <= 0 {
		return nil, fmt.Errorf("token cache is incomplete")
	}
	versionPresent, err := common.RDB.HExists(context.Background(), getTokenCacheKey(key), "QuotaVersion").Result()
	if err != nil {
		return nil, err
	}
	if !versionPresent {
		return nil, fmt.Errorf("token cache quota version is missing")
	}
	token.Key = key
	var persisted struct {
		QuotaVersion int64
	}
	if err := DB.Model(&Token{}).
		Select("COALESCE(quota_version, 0) AS quota_version").
		Where("id = ? AND "+commonKeyCol+" = ?", token.Id, key).
		Take(&persisted).Error; err != nil {
		return nil, err
	}
	if persisted.QuotaVersion != token.QuotaVersion {
		if err := discardTokenQuotaCacheSnapshot(key, token.QuotaVersion); err != nil {
			return nil, errors.Join(ErrTokenQuotaCacheStale, err)
		}
		return nil, ErrTokenQuotaCacheStale
	}
	return &token, nil
}
