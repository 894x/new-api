package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInMemoryRateLimiterHonorsReducedLimitAfterOldRequestsExpire(t *testing.T) {
	limiter := InMemoryRateLimiter{}
	limiter.Init(0)
	now := time.Now().Unix()
	// An administrator reduced the limit from 4 to 2. Three old requests
	// expired, but one recent request still consumes the new allowance.
	entry := limiter.touchEntry("customer", time.Unix(now, 0))
	for _, timestamp := range []int64{now - 120, now - 120, now - 120, now} {
		entry.requests.append(timestamp)
	}
	assert.True(t, limiter.Request("customer", 2, 60))
	assert.False(t, limiter.Request("customer", 2, 60))
}
