package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newModelRequestTPMTestContext() *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx
}

func useModelRequestTPMRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	previousRedisClient := common.RDB
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	require.NoError(t, redisClient.Ping(context.Background()).Err())

	common.RedisEnabled = true
	common.RDB = redisClient
	t.Cleanup(func() {
		_ = redisClient.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRedisClient
	})

	return redisServer
}

func useModelRequestTPMSettings(t *testing.T, enabled bool, globalTPM int, groups string) {
	t.Helper()

	previousEnabled := setting.ModelRequestRateLimitEnabled
	previousTPM := setting.ModelRequestRateLimitTPM
	previousGroups := setting.ModelRequestRateLimitGroup2JSONString()
	setting.ModelRequestRateLimitEnabled = enabled
	setting.ModelRequestRateLimitTPM = globalTPM
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(groups))
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled = previousEnabled
		setting.ModelRequestRateLimitTPM = previousTPM
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(previousGroups))
	})
}

func useModelRequestTPMMemory(t *testing.T) {
	t.Helper()

	previousRedisEnabled := common.RedisEnabled
	modelRequestTPMMemoryStore.Lock()
	previousWindows := modelRequestTPMMemoryStore.windows
	previousCleanupAt := modelRequestTPMMemoryStore.lastCleanupAt
	modelRequestTPMMemoryStore.windows = make(map[string]modelRequestTPMMemoryWindow)
	modelRequestTPMMemoryStore.lastCleanupAt = time.Time{}
	modelRequestTPMMemoryStore.Unlock()
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		modelRequestTPMMemoryStore.Lock()
		modelRequestTPMMemoryStore.windows = previousWindows
		modelRequestTPMMemoryStore.lastCleanupAt = previousCleanupAt
		modelRequestTPMMemoryStore.Unlock()
	})
}

func TestResolveModelRequestTPMLimitUsesTokenGroupOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMSettings(t, true, 1000, `{"vip":[0,1000,60000]}`)
	ctx := newModelRequestTPMTestContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "vip")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	assert.Equal(t, 60000, ResolveModelRequestTPMLimit(ctx))
}

func TestResolveModelRequestTPMLimitFallsBackToGlobalLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMSettings(t, true, 1000, `{}`)
	ctx := newModelRequestTPMTestContext()
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	assert.Equal(t, 1000, ResolveModelRequestTPMLimit(ctx))
}

func TestResolveModelRequestTPMLimitIsUnlimitedWhenRateLimitingIsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMSettings(t, false, 1000, `{"vip":[0,1000,60000]}`)
	ctx := newModelRequestTPMTestContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "vip")

	assert.Zero(t, ResolveModelRequestTPMLimit(ctx))
}

func TestRedisModelRequestTPMSettlementReplacesEstimateWithActualUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMRedis(t)

	first := newModelRequestTPMTestContext()
	allowed, _, err := ReserveModelRequestTPM(first, 101, 100, 60)
	require.NoError(t, err)
	require.True(t, allowed)

	blocked := newModelRequestTPMTestContext()
	allowed, retryAfter, err := ReserveModelRequestTPM(blocked, 101, 100, 50)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Positive(t, retryAfter)

	require.NoError(t, SettleModelRequestTPM(first, 30, 10))
	afterSettlement := newModelRequestTPMTestContext()
	allowed, _, err = ReserveModelRequestTPM(afterSettlement, 101, 100, 50)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRedisModelRequestTPMSettlementCarriesOutputDebt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMRedis(t)

	first := newModelRequestTPMTestContext()
	allowed, _, err := ReserveModelRequestTPM(first, 102, 100, 20)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, SettleModelRequestTPM(first, 20, 60))

	next := newModelRequestTPMTestContext()
	allowed, _, err = ReserveModelRequestTPM(next, 102, 100, 21)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRedisModelRequestTPMSettlementSurvivesClientCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMRedis(t)

	first := newModelRequestTPMTestContext()
	allowed, _, err := ReserveModelRequestTPM(first, 105, 100, 60)
	require.NoError(t, err)
	require.True(t, allowed)

	requestContext, cancel := context.WithCancel(first.Request.Context())
	cancel()
	first.Request = first.Request.WithContext(requestContext)
	require.NoError(t, SettleModelRequestTPM(first, 30, 10))

	afterSettlement := newModelRequestTPMTestContext()
	allowed, _, err = ReserveModelRequestTPM(afterSettlement, 105, 100, 50)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRedisModelRequestTPMSettlementStartsWithActualUsageAfterWindowExpires(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer := useModelRequestTPMRedis(t)

	first := newModelRequestTPMTestContext()
	allowed, _, err := ReserveModelRequestTPM(first, 106, 100, 60)
	require.NoError(t, err)
	require.True(t, allowed)

	redisServer.FastForward(time.Minute)
	require.NoError(t, SettleModelRequestTPM(first, 30, 10))

	afterSettlement := newModelRequestTPMTestContext()
	allowed, _, err = ReserveModelRequestTPM(afterSettlement, 106, 100, 70)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRedisModelRequestTPMReservationIsAtomicUnderConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMRedis(t)
	const requestCount = 20

	var allowedCount atomic.Int64
	errorsFound := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestCount)
	for range requestCount {
		go func() {
			defer waitGroup.Done()
			allowed, _, err := ReserveModelRequestTPM(newModelRequestTPMTestContext(), 103, 70, 10)
			if err != nil {
				errorsFound <- err
				return
			}
			if allowed {
				allowedCount.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	close(errorsFound)
	for err := range errorsFound {
		require.NoError(t, err)
	}

	assert.Equal(t, int64(7), allowedCount.Load())
}

func TestMemoryModelRequestTPMUsesTheSameReservationContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMMemory(t)

	first := newModelRequestTPMTestContext()
	allowed, _, err := ReserveModelRequestTPM(first, 104, 100, 60)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, SettleModelRequestTPM(first, 30, 10))

	second := newModelRequestTPMTestContext()
	allowed, _, err = ReserveModelRequestTPM(second, 104, 100, 60)
	require.NoError(t, err)
	assert.True(t, allowed)

	third := newModelRequestTPMTestContext()
	allowed, _, err = ReserveModelRequestTPM(third, 104, 100, 1)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestMemoryModelRequestTPMSettlementStartsWithActualUsageAfterWindowExpires(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useModelRequestTPMMemory(t)

	first := newModelRequestTPMTestContext()
	allowed, _, err := ReserveModelRequestTPM(first, 107, 100, 60)
	require.NoError(t, err)
	require.True(t, allowed)

	key := fmt.Sprintf("rateLimit:v2:user:%s:%d", modelRequestTPMRedisMark, 107)
	modelRequestTPMMemoryStore.Lock()
	window := modelRequestTPMMemoryStore.windows[key]
	window.expiresAt = time.Now().Add(-time.Second)
	modelRequestTPMMemoryStore.windows[key] = window
	modelRequestTPMMemoryStore.Unlock()
	require.NoError(t, SettleModelRequestTPM(first, 30, 10))

	afterSettlement := newModelRequestTPMTestContext()
	allowed, _, err = ReserveModelRequestTPM(afterSettlement, 107, 100, 70)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestEstimateRequestTokenRunsWhenExplicitlyCalledForTPM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousCountToken := constant.CountToken
	constant.CountToken = false
	t.Cleanup(func() {
		constant.CountToken = previousCountToken
	})
	ctx := newModelRequestTPMTestContext()
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "test-model")
	meta := &types.TokenCountMeta{
		TokenType:   types.TokenTypeTextNumber,
		CombineText: "hello",
	}
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}

	tokens, err := EstimateRequestToken(ctx, meta, info)

	require.NoError(t, err)
	assert.Equal(t, 5, tokens)
}
