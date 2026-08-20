package channelcapacity

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryLimiterRejectsWhenEitherMinuteBudgetWouldBeExceeded(t *testing.T) {
	t.Parallel()

	limiter := NewMemoryLimiter()
	key := Key{ChannelID: 7, Model: "gpt-test"}
	limits := Limits{RPM: 2, TPM: 100}
	now := time.Unix(120, 0)

	first, err := limiter.Acquire(context.Background(), key, limits, 60, now)
	require.NoError(t, err)
	assert.True(t, first.Allowed)
	assert.Equal(t, int64(1), first.UsedRPM)
	assert.Equal(t, int64(60), first.UsedTPM)

	tooManyTokens, err := limiter.Acquire(context.Background(), key, limits, 50, now.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, tooManyTokens.Allowed)
	assert.Equal(t, LimitTPM, tooManyTokens.LimitedBy)
	assert.Equal(t, int64(1), tooManyTokens.UsedRPM, "a rejected request must not consume RPM")
	assert.Equal(t, int64(60), tooManyTokens.UsedTPM, "a rejected request must not consume TPM")

	second, err := limiter.Acquire(context.Background(), key, limits, 40, now.Add(2*time.Second))
	require.NoError(t, err)
	assert.True(t, second.Allowed)
	assert.Equal(t, int64(2), second.UsedRPM)
	assert.Equal(t, int64(100), second.UsedTPM)

	rpmExhausted, err := limiter.Acquire(context.Background(), key, limits, 0, now.Add(3*time.Second))
	require.NoError(t, err)
	assert.False(t, rpmExhausted.Allowed)
	assert.Equal(t, LimitRPM, rpmExhausted.LimitedBy)
}

func TestMemoryLimiterUsesIndependentChannelModelWindowsAndResetsAtMinuteBoundary(t *testing.T) {
	t.Parallel()

	limiter := NewMemoryLimiter()
	limits := Limits{RPM: 1, TPM: 10}
	windowStart := time.Unix(120, 0)

	first, err := limiter.Acquire(context.Background(), Key{ChannelID: 1, Model: "model-a"}, limits, 10, windowStart)
	require.NoError(t, err)
	require.True(t, first.Allowed)

	otherModel, err := limiter.Acquire(context.Background(), Key{ChannelID: 1, Model: "model-b"}, limits, 10, windowStart.Add(59*time.Second))
	require.NoError(t, err)
	assert.True(t, otherModel.Allowed)

	otherChannel, err := limiter.Acquire(context.Background(), Key{ChannelID: 2, Model: "model-a"}, limits, 10, windowStart.Add(59*time.Second))
	require.NoError(t, err)
	assert.True(t, otherChannel.Allowed)

	reset, err := limiter.Acquire(context.Background(), Key{ChannelID: 1, Model: "model-a"}, limits, 10, windowStart.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, reset.Allowed)
	assert.Equal(t, int64(1), reset.UsedRPM)
	assert.Equal(t, int64(10), reset.UsedTPM)
}

func TestMemoryLimiterNeverOversubscribesConcurrentRPM(t *testing.T) {
	t.Parallel()

	limiter := NewMemoryLimiter()
	key := Key{ChannelID: 9, Model: "concurrent"}
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

func TestMemoryLimiterDropsInactiveKeysAfterTheirMinuteExpires(t *testing.T) {
	t.Parallel()

	limiter := NewMemoryLimiter()
	limits := Limits{RPM: 1}
	firstWindow := time.Unix(120, 0)

	_, err := limiter.Acquire(context.Background(), Key{ChannelID: 1, Model: "old"}, limits, 0, firstWindow)
	require.NoError(t, err)
	_, err = limiter.Acquire(context.Background(), Key{ChannelID: 2, Model: "current"}, limits, 0, firstWindow.Add(time.Minute))
	require.NoError(t, err)

	assert.NotContains(t, limiter.usage, Key{ChannelID: 1, Model: "old"})
	assert.Contains(t, limiter.usage, Key{ChannelID: 2, Model: "current"})
}
