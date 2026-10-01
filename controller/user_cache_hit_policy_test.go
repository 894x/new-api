package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUserCacheHitPolicyConfigurationAndResponse(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	// Repeat startup on existing storage; this feature introduces no new table.
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	user := model.User{Username: "cache-policy-user", Status: common.UserStatusEnabled, Group: "default", AffCode: "cache-policy-user"}
	require.NoError(t, db.Create(&user).Error)
	previousRDB := common.RDB
	rdb := redis.NewClient(&redis.Options{Addr: os.Getenv("TEST_CACHE_POLICY_REDIS_ADDR")})
	if os.Getenv("TEST_CACHE_POLICY_REDIS_ADDR") == "" {
		require.NoError(t, rdb.Close())
		server := miniredis.RunT(t)
		rdb = redis.NewClient(&redis.Options{Addr: server.Addr()})
	}
	t.Cleanup(func() { require.NoError(t, rdb.Close()); common.RDB = previousRDB })
	common.RDB, common.RedisEnabled = rdb, true
	_, revision, err := model.GetUserCacheHitPolicies(user.Id)
	require.NoError(t, err)
	request := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("PUT", "/api/user/1/cache-hit-policy", strings.NewReader(body))
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(user.Id)}}
		PutUserCacheHitPolicy(c)
		return recorder
	}
	response := request(fmt.Sprintf(`{"model":"policy-model","revision":%q,"enabled":true,"min_bps":5000,"max_bps":5000}`, revision))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, http.StatusConflict, request(fmt.Sprintf(`{"model":"other-model","revision":%q,"enabled":false,"min_bps":0,"max_bps":0}`, revision)).Code)
	assert.Equal(t, http.StatusBadRequest, request(`{"model":"policy-model","revision":"x","enabled":true,"max_bps":5000}`).Code)
	policies, current, err := model.GetUserCacheHitPolicies(user.Id)
	require.NoError(t, err)
	assert.True(t, policies["policy-model"].Enabled)
	assert.NotEqual(t, revision, current)
	require.NoError(t, model.UpdateUserCacheHitPolicy(user.Id, "other-model", current, model.UserCacheHitPolicy{}))
	policies, _, err = model.GetUserCacheHitPolicies(user.Id)
	require.NoError(t, err)
	assert.Len(t, policies, 2, "saving another model preserves existing rules")
	require.NoError(t, db.Create(&model.Option{Key: "CachePolicyUnrelatedTest", Value: "preserved"}).Error)
	for range 2 {
		require.NoError(t, db.AutoMigrate(&model.Option{}))
	}
	policies, _, err = model.GetUserCacheHitPolicies(user.Id)
	require.NoError(t, err)
	assert.Len(t, policies, 2, "repeated startup preserves saved policies")
	var unrelated model.Option
	require.NoError(t, db.Where(&model.Option{Key: "CachePolicyUnrelatedTest"}).First(&unrelated).Error)
	assert.Equal(t, "preserved", unrelated.Value)
	assert.Error(t, model.UpdateOption(model.UserCacheHitPolicyOptionPrefix+fmt.Sprint(user.Id), "{}"), "generic options cannot bypass revision checks")

	info := &relaycommon.RelayInfo{UserId: user.Id, OriginModelName: "policy-model"}
	for _, tc := range []struct {
		name, path, body, cachePath string
		cached, ordinary            int64
	}{
		{"openai", "/v1/chat/completions", `{"usage":{"prompt_tokens":1000,"completion_tokens":10,"total_tokens":1010,"prompt_tokens_details":{"cached_tokens":900}},"choices":[{"message":{"content":"unchanged"}}]}`, "usage.prompt_tokens_details.cached_tokens", 500, 0},
		{"low real", "/v1/chat/completions", `{"usage":{"prompt_tokens":1000,"prompt_tokens_details":{"cached_tokens":100}}}`, "usage.prompt_tokens_details.cached_tokens", 100, 0},
		{"claude", "/v1/messages", `{"type":"message_start","message":{"usage":{"input_tokens":50,"cache_read_input_tokens":900,"cache_creation_input_tokens":50,"cache_creation":{"ephemeral_1h_input_tokens":50}}}}`, "message.usage.cache_read_input_tokens", 500, 450},
		{"claude split writes only", "/v1/messages", `{"type":"message_start","message":{"usage":{"input_tokens":50,"cache_read_input_tokens":900,"cache_creation":{"ephemeral_5m_input_tokens":25,"ephemeral_1h_input_tokens":25}}}}`, "message.usage.cache_read_input_tokens", 500, 0},
		{"responses", "/v1/responses", `{"type":"response.completed","response":{"usage":{"input_tokens":1000,"input_tokens_details":{"cached_tokens":900},"output_tokens":10}}}`, "response.usage.input_tokens_details.cached_tokens", 500, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", tc.path, nil)
			c.Set(common.RequestIdKey, "cache-policy-test-"+common.NewRequestId())
			service.BeginUserCacheHitPolicy(c, info)
			updated := service.TransformUserCacheHitResponse(c, []byte(tc.body))
			assert.Equal(t, tc.cached, gjson.GetBytes(updated, tc.cachePath).Int())
			assert.Equal(t, updated, service.TransformUserCacheHitResponse(c, []byte(tc.body)), "repeated usage shares one decision")
			if tc.ordinary > 0 {
				assert.Equal(t, tc.ordinary, gjson.GetBytes(updated, "message.usage.input_tokens").Int())
				assert.Equal(t, int64(50), gjson.GetBytes(updated, "message.usage.cache_creation.ephemeral_1h_input_tokens").Int())
			}
			if tc.name == "openai" {
				assert.Contains(t, string(updated), "unchanged")
				source := &http.Response{StatusCode: 200, Header: http.Header{"Content-Length": []string{fmt.Sprint(len(tc.body))}, "ETag": []string{"upstream-hash"}}}
				service.IOCopyBytesGracefully(c, source, []byte(tc.body))
				assert.Equal(t, fmt.Sprint(recorder.Body.Len()), recorder.Header().Get("Content-Length"))
				assert.Empty(t, recorder.Header().Get("ETag"))
				assert.Equal(t, int64(500), gjson.Get(recorder.Body.String(), tc.cachePath).Int())
			}
			if tc.name == "responses" {
				require.NoError(t, helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.completed"}, tc.body))
				assert.Contains(t, recorder.Body.String(), "event: response.completed")
			}
			service.FinishUserCacheHitPolicy(c)
		})
	}
	daily, err := service.GetUserCacheHitDailyUsage(user.Id, info.OriginModelName)
	require.NoError(t, err)
	assert.Zero(t, daily.InputTokens, "unsettled responses release their daily allocation")
	_, current, err = model.GetUserCacheHitPolicies(user.Id)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, request(fmt.Sprintf(`{"model":"policy-model","revision":%q,"enabled":true,"min_bps":0,"max_bps":0}`, current)).Code)
	cZero, _ := gin.CreateTestContext(httptest.NewRecorder())
	cZero.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	service.BeginUserCacheHitPolicy(cZero, info)
	zeroBody := service.TransformUserCacheHitResponse(cZero, []byte(`{"usage":{"prompt_tokens":1000,"prompt_tokens_details":{"cached_tokens":900}}}`))
	assert.Zero(t, gjson.GetBytes(zeroBody, "usage.prompt_tokens_details.cached_tokens").Int())
	service.FinishUserCacheHitPolicy(cZero)
	_, current, err = model.GetUserCacheHitPolicies(user.Id)
	require.NoError(t, err)
	common.RedisEnabled = false
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	service.BeginUserCacheHitPolicy(c, info)
	body := []byte(`{"usage":{"prompt_tokens":1000,"prompt_tokens_details":{"cached_tokens":900}}}`)
	assert.Equal(t, body, service.TransformUserCacheHitResponse(c, body))
	assert.Equal(t, http.StatusServiceUnavailable, request(fmt.Sprintf(`{"model":"policy-model","revision":%q,"enabled":true,"min_bps":0,"max_bps":0}`, current)).Code)
	_, err = rdb.Ping(context.Background()).Result()
	require.NoError(t, err)
}
