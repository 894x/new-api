package channelcapacity

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmissionIsAtomicAndDeniedRequestsDoNotConsumeCapacity(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "redis"}[shared], func(t *testing.T) {
			now := time.Unix(120, 0)
			limiter := NewMemoryLimiter()
			if shared {
				server := miniredis.RunT(t)
				server.SetTime(now)
				client := redis.NewClient(&redis.Options{Addr: server.Addr()})
				t.Cleanup(func() { require.NoError(t, client.Close()) })
				limiter = NewRedisLimiter(client)
			}
			ctx := context.Background()
			key := Key{ChannelID: 1, Model: "public-model"}
			decision, err := limiter.Acquire(ctx, key, Limits{RPM: 2, TPM: 10}, 11, now)
			require.NoError(t, err)
			assert.False(t, decision.Allowed)
			var admitted atomic.Int32
			var wg sync.WaitGroup
			for range 4 {
				wg.Go(func() {
					result, acquireErr := limiter.Acquire(ctx, key, Limits{RPM: 2, TPM: 10}, 5, now)
					assert.NoError(t, acquireErr)
					if result.Allowed {
						admitted.Add(1)
					}
				})
			}
			wg.Wait()
			assert.Equal(t, int32(2), admitted.Load())
			decision, err = limiter.Acquire(ctx, key, Limits{RPM: 2, TPM: 10}, 1, now)
			require.NoError(t, err)
			assert.False(t, decision.Allowed)
			assert.Equal(t, time.Minute, decision.RetryAfter)
			decision, err = limiter.Acquire(ctx, Key{ChannelID: 2, Model: key.Model}, Limits{RPM: 2, TPM: 10}, 5, now)
			require.NoError(t, err)
			assert.True(t, decision.Allowed)
		})
	}
}

func TestRedisCapacitySharesServerTimeAndResetsAtMinuteBoundary(t *testing.T) {
	server := miniredis.RunT(t)
	server.SetTime(time.Unix(179, 0))
	first := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer first.Close()
	second := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer second.Close()
	key := Key{ChannelID: 1, Model: "public-model"}
	decision, err := NewRedisLimiter(first).Acquire(context.Background(), key, Limits{RPM: 1}, 0, time.Unix(0, 0))
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
	decision, err = NewRedisLimiter(second).Acquire(context.Background(), key, Limits{RPM: 1}, 0, time.Unix(9999, 0))
	require.NoError(t, err)
	assert.False(t, decision.Allowed)
	assert.Equal(t, time.Second, decision.RetryAfter)
	server.SetTime(time.Unix(180, 0))
	decision, err = NewRedisLimiter(second).Acquire(context.Background(), key, Limits{RPM: 1}, 0, time.Unix(0, 0))
	require.NoError(t, err)
	assert.True(t, decision.Allowed)
}
