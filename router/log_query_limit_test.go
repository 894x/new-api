package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLogQueryE2E(t *testing.T) (*managedUserE2EFixture, *miniredis.Miniredis) {
	t.Helper()
	previousEnable, previousNum, previousDuration := common.LogQueryRateLimitEnable, common.LogQueryRateLimitNum, common.LogQueryRateLimitDuration
	previousGlobal, previousCritical := common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable
	previousGlobalNum, previousGlobalDuration := common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration
	previousCriticalNum, previousCriticalDuration := common.CriticalRateLimitNum, common.CriticalRateLimitDuration
	previousRDB := common.RDB
	common.LogQueryRateLimitEnable, common.LogQueryRateLimitNum, common.LogQueryRateLimitDuration = true, 2, 60
	common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = true, 100, 180
	common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration = true, 20, 1200
	t.Cleanup(func() {
		common.LogQueryRateLimitEnable, common.LogQueryRateLimitNum, common.LogQueryRateLimitDuration = previousEnable, previousNum, previousDuration
		common.GlobalApiRateLimitEnable, common.CriticalRateLimitEnable = previousGlobal, previousCritical
		common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration = previousGlobalNum, previousGlobalDuration
		common.CriticalRateLimitNum, common.CriticalRateLimitDuration = previousCriticalNum, previousCriticalDuration
		common.RDB = previousRDB
	})
	f := setupManagedUserE2E(t)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	common.RedisEnabled, common.RDB = true, client
	f.server.Close()
	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	SetApiRouter(engine)
	f.server = httptest.NewServer(engine)
	return f, redisServer
}

func TestAdminLogQueryLimitPersistenceAndPermissions(t *testing.T) {
	f, _ := setupLogQueryE2E(t)
	target := f.managerA
	payload := map[string]any{"id": target.Id, "username": target.Username, "display_name": target.DisplayName, "group": target.Group}
	for _, limit := range []int{300, 0, 60} {
		payload["log_query_rate_limit"] = limit
		status, result := f.request(t, http.MethodPut, "/api/user/", f.root.Id, payload)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, result["success"], result)
		status, result = f.request(t, http.MethodGet, "/api/user/"+strconv.Itoa(target.Id), f.root.Id, nil)
		require.Equal(t, http.StatusOK, status)
		data := result["data"].(map[string]any)
		assert.EqualValues(t, limit, data["log_query_rate_limit"])
		policy := data["log_query_rate_limit_policy"].(map[string]any)
		assert.EqualValues(t, 2, policy["default_limit"])
		assert.EqualValues(t, 60, policy["window_seconds"])
	}
	delete(payload, "log_query_rate_limit")
	_, result := f.request(t, http.MethodPut, "/api/user/", f.root.Id, payload)
	require.Equal(t, true, result["success"])
	limit, err := model.GetUserLogQueryRateLimit(target.Id)
	require.NoError(t, err)
	assert.Equal(t, 60, limit, "legacy updates must preserve an existing override")
	for _, invalid := range []any{-1, 60001, 1.5, "300"} {
		payload["log_query_rate_limit"] = invalid
		_, result = f.request(t, http.MethodPut, "/api/user/", f.root.Id, payload)
		assert.Equal(t, false, result["success"], result)
	}
	payload["log_query_rate_limit"] = 300
	status, _ := f.request(t, http.MethodPut, "/api/user/", target.Id, payload)
	assert.Equal(t, http.StatusForbidden, status)
	status, _ = f.request(t, http.MethodPut, "/api/user/self", target.Id, map[string]any{"display_name": "changed", "log_query_rate_limit": 300})
	assert.Equal(t, http.StatusForbidden, status)
	_, result = f.request(t, http.MethodPut, "/api/user/self", target.Id, map[string]any{"display_name": "updated"})
	require.Equal(t, true, result["success"])
	limit, err = model.GetUserLogQueryRateLimit(target.Id)
	require.NoError(t, err)
	assert.Equal(t, 60, limit)
	var stored model.User
	require.NoError(t, model.DB.First(&stored, target.Id).Error)
	assert.Equal(t, target.AuthVersion, stored.AuthVersion, "changing query limits must not revoke login sessions")
	assert.Equal(t, target.Quota, stored.Quota)
}

func requestTokenLogs(t *testing.T, serverURL, key string) (int, http.Header, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, serverURL+"/api/log/token", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer sk-"+key)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, response.Header, body
}

func TestTokenLogRouteSharesUserLimitAndPreservesOtherBoundaries(t *testing.T) {
	f, redisServer := setupLogQueryE2E(t)
	tokens := []model.Token{
		{UserId: f.managerA.Id, Key: "limitkeya", Status: common.TokenStatusEnabled},
		{UserId: f.managerA.Id, Key: "limitkeyb", Status: common.TokenStatusExpired},
		{UserId: f.managerB.Id, Key: "limitkeyc", Status: common.TokenStatusExhausted},
		{UserId: f.managerA.Id, Key: "limitkeydisabled", Status: common.TokenStatusDisabled},
	}
	require.NoError(t, model.DB.Create(&tokens).Error)
	for i, token := range tokens[:3] {
		require.NoError(t, model.DB.Create(&model.Log{UserId: token.UserId, TokenId: token.Id, Type: model.LogTypeConsume, ModelName: "only-token-" + strconv.Itoa(i)}).Error)
	}
	// Exhausting the sensitive-operation bucket must no longer block log reads.
	redisServer.Set("rateLimit:v2:ip:CT:127.0.0.1", "20")
	redisServer.SetTTL("rateLimit:v2:ip:CT:127.0.0.1", 1200*time.Second)
	for i, key := range []string{"limitkeya", "limitkeyb", "limitkeyc"} {
		status, _, body := requestTokenLogs(t, f.server.URL, key)
		require.Equal(t, http.StatusOK, status, string(body))
		var response struct {
			Success bool        `json:"success"`
			Data    []model.Log `json:"data"`
		}
		require.NoError(t, common.Unmarshal(body, &response))
		require.True(t, response.Success)
		require.Len(t, response.Data, 1)
		assert.Equal(t, "only-token-"+strconv.Itoa(i), response.Data[0].ModelName)
	}
	status, headers, _ := requestTokenLogs(t, f.server.URL, "limitkeya")
	assert.Equal(t, http.StatusTooManyRequests, status)
	assert.Equal(t, "60", headers.Get("Retry-After"))
	for _, key := range []string{"missing", "limitkeydisabled"} {
		status, _, _ = requestTokenLogs(t, f.server.URL, key)
		assert.Equal(t, http.StatusUnauthorized, status)
	}
	// A new server instance must use the same user's Redis bucket.
	engine := gin.New()
	SetApiRouter(engine)
	second := httptest.NewServer(engine)
	defer second.Close()
	status, _, _ = requestTokenLogs(t, second.URL, "limitkeyb")
	assert.Equal(t, http.StatusTooManyRequests, status)
	// Admin edits take effect even with the previous auth snapshot cached.
	_, result := f.request(t, http.MethodPut, "/api/user/", f.root.Id, map[string]any{
		"id": f.managerA.Id, "username": f.managerA.Username, "display_name": f.managerA.DisplayName, "group": f.managerA.Group, "log_query_rate_limit": 10,
	})
	require.Equal(t, true, result["success"], result)
	status, _, _ = requestTokenLogs(t, second.URL, "limitkeya")
	assert.Equal(t, http.StatusOK, status)
	_, result = f.request(t, http.MethodPut, "/api/user/", f.root.Id, map[string]any{
		"id": f.managerA.Id, "username": f.managerA.Username, "display_name": f.managerA.DisplayName, "group": f.managerA.Group, "log_query_rate_limit": 0,
	})
	require.Equal(t, true, result["success"], result)
	status, _, _ = requestTokenLogs(t, second.URL, "limitkeya")
	assert.Equal(t, http.StatusTooManyRequests, status)
	redisServer.FastForward(time.Minute)
	status, _, _ = requestTokenLogs(t, f.server.URL, "limitkeya")
	assert.Equal(t, http.StatusOK, status)
	// The outer IP ceiling remains effective even with available user quota.
	redisServer.Set("rateLimit:v2:ip:GA:127.0.0.1", "100")
	redisServer.SetTTL("rateLimit:v2:ip:GA:127.0.0.1", 180*time.Second)
	status, headers, _ = requestTokenLogs(t, f.server.URL, "limitkeyc")
	assert.Equal(t, http.StatusTooManyRequests, status)
	assert.Equal(t, "180", headers.Get("Retry-After"))
}
