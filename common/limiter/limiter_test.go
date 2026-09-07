package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestBucketExpiresAfterRefillAndRecoversAfterScriptFlush(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := context.Background()
	limit := New(ctx, client)
	allowed, err := limit.Allow(ctx, "user-model", WithCapacity(120), WithRate(2), WithRequested(60))
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.GreaterOrEqual(t, server.TTL("user-model"), time.Minute)
	assert.LessOrEqual(t, server.TTL("user-model"), 2*time.Minute)
	require.NoError(t, client.ScriptFlush(ctx).Err())
	allowed, err = limit.Allow(ctx, "user-model", WithCapacity(120), WithRate(2), WithRequested(60))
	require.NoError(t, err)
	assert.True(t, allowed)
	allowed, err = limit.Allow(ctx, "user-model", WithCapacity(120), WithRate(2), WithRequested(60))
	require.NoError(t, err)
	assert.False(t, allowed)
	server.FastForward(2 * time.Minute)
	assert.False(t, server.Exists("user-model"))
}

func TestRequestLimiterUsesTheSuppliedRedisClient(t *testing.T) {
	ctx := context.Background()
	for range 2 {
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		allowed, err := New(ctx, client).Allow(ctx, "same-user-model", WithCapacity(1), WithRate(1))
		require.NoError(t, err)
		assert.True(t, allowed, "a different Redis client must use its own bucket")
		require.NoError(t, client.Close())
	}
}
