package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

func TestGroupRPMIsInheritedAndCountedPerUserAndModel(t *testing.T) {
	for _, useRedis := range []bool{false, true} {
		t.Run(fmt.Sprint(useRedis), func(t *testing.T) {
			useGroupModelRateLimitSettings(t, useRedis, `{"vip":[0,1,1000]}`)
			router := groupModelRateLimitRouter()
			for _, request := range []struct {
				user, model string
				want        int
			}{
				{"601", "model-a", 204}, {"601", "model-a", 429}, {"601", "model-b", 204}, {"602", "model-a", 204},
			} {
				response := performGroupModelRequest(router, request.user, request.model)
				assert.Equal(t, request.want, response.Code, "%s/%s", request.user, request.model)
			}
		})
	}
}

func TestModelRPMOverrideCanRaiseLimitAndExplicitlyDisableIt(t *testing.T) {
	for _, useRedis := range []bool{false, true} {
		t.Run(fmt.Sprint(useRedis), func(t *testing.T) {
			useGroupModelRateLimitSettings(t, useRedis, `{"vip":{"limits":[0,1,1000],"models":{"raised":{"rpm":2},"unlimited":{"rpm":0},"tpm-only":{"tpm":100}}}}`)
			router := groupModelRateLimitRouter()
			for _, request := range []struct {
				model string
				want  int
			}{
				{"raised", 204}, {"raised", 204}, {"raised", 429},
				{"unlimited", 204}, {"unlimited", 204},
				{"tpm-only", 204}, {"tpm-only", 429},
			} {
				response := performGroupModelRequest(router, "603", request.model)
				assert.Equal(t, request.want, response.Code, request.model)
			}
		})
	}
}

func useGroupModelRateLimitSettings(t *testing.T, useRedis bool, groups string) {
	t.Helper()
	previousMemoryLimiter := modelRequestMemoryRateLimiter
	modelRequestMemoryRateLimiter = &common.InMemoryRateLimiter{}
	modelRequestMemoryRateLimiter.Init(0)
	oldEnabled, oldRedis := setting.ModelRequestRateLimitEnabled, common.RedisEnabled
	oldDuration := setting.ModelRequestRateLimitDurationMinutes
	oldGroups := setting.ModelRequestRateLimitGroup2JSONString()
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	common.RedisEnabled = false
	if useRedis {
		useRateLimitMiniRedis(t)
	}
	t.Cleanup(func() {
		modelRequestMemoryRateLimiter = previousMemoryLimiter
		setting.ModelRequestRateLimitEnabled = oldEnabled
		setting.ModelRequestRateLimitDurationMinutes = oldDuration
		common.RedisEnabled = oldRedis
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(oldGroups))
	})
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(groups))
}

func groupModelRateLimitRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		var userID int
		_, _ = fmt.Sscan(c.GetHeader("Test-User"), &userID)
		c.Set("id", userID)
		c.Set("role", common.RoleCommonUser)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "vip")
	}, ModelRequestRateLimit(), func(c *gin.Context) {
		// The limiter must leave the request body reusable for the relay.
		var body struct {
			Model string `json:"model"`
		}
		if err := common.UnmarshalBodyReusable(c, &body); err != nil || body.Model == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusNoContent)
	})
	return router
}

func performGroupModelRequest(router *gin.Engine, user, model string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q}`, model)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Test-User", user)
	router.ServeHTTP(recorder, request)
	return recorder
}
