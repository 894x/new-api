package channelcapacity

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisLimiterAtomicallyAppliesRPMAndTPMWithoutChargingDeniedRequests(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	limiter := NewRedisLimiter(client, "test-capacity")
	key := Key{ChannelID: 7, Model: "gpt/test:model"}
	limits := Limits{RPM: 2, TPM: 100}
	now := time.Unix(120, 0)

	first, err := limiter.Acquire(context.Background(), key, limits, 60, now)
	require.NoError(t, err)
	require.True(t, first.Allowed)

	denied, err := limiter.Acquire(context.Background(), key, limits, 50, now.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, denied.Allowed)
	assert.Equal(t, LimitTPM, denied.LimitedBy)
	assert.Equal(t, int64(1), denied.UsedRPM)
	assert.Equal(t, int64(60), denied.UsedTPM)

	second, err := limiter.Acquire(context.Background(), key, limits, 40, now.Add(2*time.Second))
	require.NoError(t, err)
	assert.True(t, second.Allowed)
	assert.Equal(t, int64(2), second.UsedRPM)
	assert.Equal(t, int64(100), second.UsedTPM)
}

func TestRedisLimiterNeverOversubscribesConcurrentRPM(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	limiter := NewRedisLimiter(client, "test-concurrent")
	key := Key{ChannelID: 8, Model: "concurrent"}
	limits := Limits{RPM: 10}
	now := time.Unix(120, 0)

	var allowed atomic.Int64
	var wg sync.WaitGroup
	errors := make(chan error, 100)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, err := limiter.Acquire(context.Background(), key, limits, 0, now)
			if err != nil {
				errors <- err
				return
			}
			if decision.Allowed {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	assert.Equal(t, int64(10), allowed.Load())
}

func TestRedisLimiterSharesOneMinuteWindowAcrossGatewayInstances(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	firstClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	secondClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, firstClient.Close())
		require.NoError(t, secondClient.Close())
	})
	firstInstance := NewRedisLimiter(firstClient, "test-shared")
	secondInstance := NewRedisLimiter(secondClient, "test-shared")
	key := Key{ChannelID: 10, Model: "shared-model"}
	limits := Limits{RPM: 1, TPM: 100}
	now := time.Unix(120, 0)

	first, err := firstInstance.Acquire(context.Background(), key, limits, 60, now)
	require.NoError(t, err)
	require.True(t, first.Allowed)

	second, err := secondInstance.Acquire(context.Background(), key, limits, 40, now.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, second.Allowed)
	assert.Equal(t, LimitRPM, second.LimitedBy)
	assert.Equal(t, int64(1), second.UsedRPM)
	assert.Equal(t, int64(60), second.UsedTPM)
}

func TestRedisLimiterUsesRedisTimeAcrossSkewedGatewayClocks(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	server.SetTime(time.Unix(179, 0))
	firstClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	secondClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, firstClient.Close())
		require.NoError(t, secondClient.Close())
	})
	firstInstance := NewRedisLimiter(firstClient, "test-clock-skew")
	secondInstance := NewRedisLimiter(secondClient, "test-clock-skew")
	key := Key{ChannelID: 12, Model: "clock-skew-model"}
	limits := Limits{RPM: 1}

	first, err := firstInstance.Acquire(context.Background(), key, limits, 0, time.Unix(120, 0))
	require.NoError(t, err)
	require.True(t, first.Allowed)

	second, err := secondInstance.Acquire(context.Background(), key, limits, 0, time.Unix(180, 0))
	require.NoError(t, err)
	assert.False(t, second.Allowed, "gateway clocks must not split the shared Redis window")

	server.SetTime(time.Unix(180, 0))
	reset, err := firstInstance.Acquire(context.Background(), key, limits, 0, time.Unix(120, 0))
	require.NoError(t, err)
	assert.True(t, reset.Allowed, "the Redis clock owns the shared minute boundary")
}

func TestRedisLimiterPreservesSafeIntegerBoundaryAndResetsAtNextMinute(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	limiter := NewRedisLimiter(client, "test-safe-integer")
	key := Key{ChannelID: 11, Model: "large-token-window"}
	const maxSafeInteger int64 = 1<<53 - 1
	limits := Limits{TPM: maxSafeInteger}
	now := time.Unix(120, 0)
	server.SetTime(now)

	first, err := limiter.Acquire(context.Background(), key, limits, maxSafeInteger-1, now)
	require.NoError(t, err)
	require.True(t, first.Allowed)

	denied, err := limiter.Acquire(context.Background(), key, limits, 2, now.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, denied.Allowed)
	assert.Equal(t, LimitTPM, denied.LimitedBy)
	assert.Equal(t, maxSafeInteger-1, denied.UsedTPM)

	lastToken, err := limiter.Acquire(context.Background(), key, limits, 1, now.Add(2*time.Second))
	require.NoError(t, err)
	assert.True(t, lastToken.Allowed)
	assert.Equal(t, maxSafeInteger, lastToken.UsedTPM)

	server.SetTime(now.Add(time.Minute))
	reset, err := limiter.Acquire(context.Background(), key, limits, maxSafeInteger, now.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, reset.Allowed)
	assert.Equal(t, maxSafeInteger, reset.UsedTPM)
}
