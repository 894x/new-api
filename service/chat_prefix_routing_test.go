package service

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chatPrefixTestBody(t testing.TB, count int, changes map[int]string, extra map[string]any) []byte {
	t.Helper()
	messages := make([]map[string]any, count)
	for i := range count {
		content := fmt.Sprintf("message %d", i)
		if replacement, exists := changes[i]; exists {
			content = replacement
		}
		messages[i] = map[string]any{"role": "user", "content": content}
	}
	body := map[string]any{"model": "prefix-model", "stream": true, "messages": messages}
	for key, value := range extra {
		body[key] = value
	}
	encoded, err := common.Marshal(body)
	require.NoError(t, err)
	return encoded
}

func chatPrefixTestParam(body []byte) *RetryParam {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUserId, 42)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyDynamicRoutingEligible, true)
	return &RetryParam{Ctx: c, TokenGroup: "default", ModelName: "prefix-model", RequestPath: c.Request.URL.Path, RequestBody: body, DynamicRoutingEligible: true,
		routingPools: map[string]*model.ModelRoutingPool{"default": nil}}
}

func isolateChatPrefixCache(t *testing.T) {
	t.Helper()
	original := chatPrefixRoutingCache
	chatPrefixRoutingCache = newChatPrefixCache(32, time.Minute)
	t.Cleanup(func() { chatPrefixRoutingCache = original })
}

func TestChatPrefixContinuationUsesCheckpointsAndAdvancesHistory(t *testing.T) {
	cache := newChatPrefixCache(4, time.Minute)
	scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
	channel := chatPrefixChannel{ID: 1}
	now := time.Now()
	old := buildChatPrefixRequest(bytes.Repeat([]byte("a"), 100))
	// The unsampled tail may change. Checkpoint matching is deliberately an
	// estimate; a complete historical-tail comparison must not become a requirement.
	continuedBody := bytes.Repeat([]byte("a"), 150)
	copy(continuedBody[90:100], "editedtail")
	continued := buildChatPrefixRequest(continuedBody)
	require.NotNil(t, old)
	require.NotNil(t, continued)
	cache.record(scope, channel, old, now)
	match := cache.lookup(scope, continued, now.Add(time.Second))[channel]
	assert.Equal(t, old.Bytes, match.MatchedBytes)
	assert.Equal(t, continued.Bytes-old.Bytes, match.NewBytes)

	cache.record(scope, channel, continued, now.Add(2*time.Second))
	assert.Equal(t, 1, cache.entries.Len(), "a continuation replaces the previous history")
	assert.Equal(t, 150, cache.lookup(scope, continued, now.Add(3*time.Second))[channel].MatchedBytes)

	unsampledBody := bytes.Clone(continuedBody)
	unsampledBody[50] = 'b'
	assert.Equal(t, 150, cache.lookup(scope, buildChatPrefixRequest(unsampledBody), now.Add(4*time.Second))[channel].MatchedBytes,
		"changes between sampled positions are intentionally outside the estimate")
	// A changed sampled byte invalidates every later sampled-string prefix.
	branchedBody := bytes.Clone(continuedBody)
	branchedBody[55] = 'b'
	branched := buildChatPrefixRequest(branchedBody)
	require.NotNil(t, branched)
	assert.Equal(t, 35, cache.lookup(scope, branched, now.Add(4*time.Second))[channel].MatchedBytes)
	cache.record(scope, channel, branched, now.Add(5*time.Second))
	assert.Equal(t, 2, cache.entries.Len(), "divergent histories remain independently reusable")
	cache = newChatPrefixCache(2, time.Minute)
	sameKeyLong := buildChatPrefixRequest(bytes.Repeat([]byte("a"), 110))
	sameKeyShort := buildChatPrefixRequest(bytes.Repeat([]byte("a"), 100))
	cache.record(scope, channel, sameKeyLong, now)
	cache.record(scope, chatPrefixChannel{ID: 2}, sameKeyLong, now.Add(time.Second))
	cache.record(scope, channel, sameKeyShort, now.Add(59*time.Second))
	cache.record(scope, chatPrefixChannel{ID: 3}, sameKeyLong, now.Add(60*time.Second))
	refreshed := cache.lookup(scope, sameKeyLong, now.Add(60*time.Second))
	assert.Equal(t, 110, refreshed[channel].MatchedBytes, "successful same-key reuse refreshes expiry without shrinking the estimate")
	assert.Equal(t, now.Add(59*time.Second), refreshed[channel].ObservedAt)
	assert.NotContains(t, refreshed, chatPrefixChannel{ID: 2}, "same-key reuse also refreshes the eviction order")
}

func TestChatPrefixSamplesOriginalBodyBytes(t *testing.T) {
	body := []byte(" {\n\"model\":\"prefix-model\", \"stream\":true, \"messages\":[{\"role\":\"user\",\"content\":\"中文内容\"}]} ")
	request := buildChatPrefixRequest(body)
	require.NotNil(t, request)
	assert.Equal(t, len(body), request.Bytes)
	expected := make([]byte, 0, len(request.Checkpoints))
	for _, checkpoint := range request.Checkpoints {
		expected = append(expected, body[checkpoint.Bytes-1])
		assert.Equal(t, string(expected), checkpoint.Sample, "keys must concatenate sampled wire bytes directly")
	}
	assert.Nil(t, buildChatPrefixRequest(nil))
}

func TestChatPrefixHistoryHandlesConcurrentRoundsExpiryAndEviction(t *testing.T) {
	scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
	channel := chatPrefixChannel{ID: 1}
	short := buildChatPrefixRequest(bytes.Repeat([]byte("a"), 40))
	long := buildChatPrefixRequest(bytes.Repeat([]byte("a"), 150))
	require.NotNil(t, short)
	require.NotNil(t, long)
	cache := newChatPrefixCache(2, time.Minute)
	now := time.Now()
	var wg sync.WaitGroup
	wg.Go(func() { cache.record(scope, channel, short, now) })
	wg.Go(func() { cache.record(scope, channel, long, now) })
	wg.Wait()
	assert.Equal(t, 150, cache.lookup(scope, long, now)[channel].MatchedBytes)
	assert.Empty(t, cache.lookup(scope, long, now.Add(time.Minute)), "expired input must not be treated as warm")
	cache = newChatPrefixCache(4, time.Minute)
	for i, request := range []*chatPrefixRequest{long, short, long, short} {
		cache.record(scope, channel, request, now.Add(time.Duration(i)*time.Second))
		assert.Equal(t, min(i+1, 2), cache.entries.Len(), "a shorter key stays independent and repeated writes do not duplicate it")
	}
	assert.Equal(t, 150, cache.lookup(scope, long, now.Add(4*time.Second))[channel].MatchedBytes)
	assert.Equal(t, 40, cache.lookup(scope, short, now.Add(4*time.Second))[channel].MatchedBytes)

	cache = newChatPrefixCache(2, time.Minute)
	cache.record(scope, channel, short, now)
	second := chatPrefixChannel{ID: 2}
	third := chatPrefixChannel{ID: 3}
	cache.record(scope, second, short, now.Add(time.Second))
	cache.record(scope, third, short, now.Add(2*time.Second))
	matches := cache.lookup(scope, short, now.Add(3*time.Second))
	assert.NotContains(t, matches, channel)
	assert.Contains(t, matches, second)
	assert.Contains(t, matches, third)
	assert.Len(t, matches, 2, "the process-local history has a bounded capacity")
}

func TestChatPrefixCacheAppliesChangedLimits(t *testing.T) {
	scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
	request := buildChatPrefixRequest([]byte("sampled request"))
	now := time.Now()
	cache := newChatPrefixCache(4, 5*time.Minute)
	for i := range 3 {
		cache.record(scope, chatPrefixChannel{ID: i + 1}, request, now.Add(time.Duration(i)*time.Second))
	}
	cache.configure(2, 5*time.Minute, now.Add(3*time.Second))
	matches := cache.lookup(scope, request, now.Add(3*time.Second))
	assert.Len(t, matches, 2)
	assert.NotContains(t, matches, chatPrefixChannel{ID: 1}, "shrinking evicts the oldest success")
	cache.configure(2, time.Second, now.Add(3*time.Second))
	assert.Empty(t, cache.lookup(scope, request, now.Add(3*time.Second)), "shorter TTL applies to existing records")
	cache.record(scope, chatPrefixChannel{ID: 1}, request, now.Add(4*time.Second))
	cache.configure(0, time.Minute, now.Add(4*time.Second))
	cache.record(scope, chatPrefixChannel{ID: 2}, request, now.Add(4*time.Second))
	assert.Empty(t, cache.lookup(scope, request, now.Add(4*time.Second)), "disabling clears history and stops recording")
	cache.configure(10, time.Minute, now.Add(4*time.Second))
	cache.record(scope, chatPrefixChannel{ID: 3}, request, now.Add(4*time.Second))
	assert.Contains(t, cache.lookup(scope, request, now.Add(4*time.Second)), chatPrefixChannel{ID: 3})
}

func TestChatPrefixRequestUsesSavedSystemLimits(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	isolateChatPrefixCache(t)
	before := model.CurrentRequestPolicy().Options
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = maps.Clone(before)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		err := model.UpdateRequestPolicyOptions(before)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, err)
	})
	const capacity = "chat_prefix_routing_setting.max_entries"
	const ttl = "chat_prefix_routing_setting.ttl_seconds"
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{capacity: "16384", ttl: "300"}))
	param := chatPrefixTestParam([]byte("raw request"))
	param.ChatPrefixMatches("default", nil)
	assert.Equal(t, 16384, chatPrefixRoutingCache.capacity)
	assert.Equal(t, 5*time.Minute, chatPrefixRoutingCache.ttl)
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{capacity: "0"}))
	assert.Empty(t, param.ChatPrefixMatches("default", nil))
	assert.Zero(t, chatPrefixRoutingCache.capacity, "disabling takes effect even after this request was sampled")
	require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{capacity: "2", ttl: "60"}))
	param.ChatPrefixMatches("default", nil)
	assert.Equal(t, 2, chatPrefixRoutingCache.capacity)
	assert.Equal(t, time.Minute, chatPrefixRoutingCache.ttl)
}

func TestChatPrefixPoolEnableOverrides(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	createChannelSelectAutoGroupsChannel(t, db, 1, "default", "prefix-model")
	model.InitChannelCache()
	channel, err := model.CacheGetChannel(1)
	require.NoError(t, err)
	isolateChatPrefixCache(t)
	before := model.CurrentRequestPolicy().Options
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = maps.Clone(before)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		err := model.UpdateRequestPolicyOptions(before)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, err)
	})
	for _, tc := range []struct {
		name     string
		global   bool
		override *bool
		want     bool
	}{
		{"inherit_on", true, nil, true}, {"inherit_off", false, nil, false},
		{"force_on", false, common.GetPointer(true), true}, {"force_off", true, common.GetPointer(false), false},
	} {
		for _, mode := range []string{model.RoutingModePriority, model.RoutingModeSmart, model.RoutingModeDiscount} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				require.NoError(t, model.UpdateRequestPolicyOptions(map[string]string{"chat_prefix_routing_setting.enabled": fmt.Sprint(tc.global)}))
				param := chatPrefixTestParam(chatPrefixTestBody(t, 4, nil, nil))
				param.routingPools["default"] = &model.ModelRoutingPool{Mode: mode, Tuning: &dynamic_routing_setting.Overrides{ChatPrefixEnabled: tc.override}}
				request := buildChatPrefixRequest(param.RequestBody)
				chatPrefixRoutingCache = newChatPrefixCache(32, time.Minute)
				chatPrefixRoutingCache.record(chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}, chatPrefixChannelIdentity(channel), request, time.Now())
				assert.Equal(t, tc.want, len(param.ChatPrefixMatches("default", []model.Ability{{ChannelId: 1}})) > 0)
				chatPrefixRoutingCache = newChatPrefixCache(32, time.Minute)
				BindChatPrefixChannel(param.Ctx, channel)
				RequestPolicy(param.Ctx).Successful = true
				RecordChatPrefixSuccess(param, 1)
				assert.Equal(t, tc.want, chatPrefixRoutingCache.entries.Len() > 0, "a disabled pool must not learn history")
			})
		}
	}
	assert.Error(t, model.UpdateRequestPolicyOptions(map[string]string{"chat_prefix_routing_setting.enabled": "invalid"}))
}

func TestChatPrefixRedisSharesAtomicContinuationAndLimits(t *testing.T) {
	server := miniredis.RunT(t)
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	server.SetTime(now)
	first := redis.NewClient(&redis.Options{Addr: server.Addr()})
	second := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, first.Close()); require.NoError(t, second.Close()) })
	writer := chatPrefixRedisCache{client: first, capacity: 2, ttl: 5 * time.Minute}
	reader := chatPrefixRedisCache{client: second, capacity: 2, ttl: 5 * time.Minute}
	ctx := context.Background()
	scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
	channel := chatPrefixChannel{ID: 1}
	short := buildChatPrefixRequest(bytes.Repeat([]byte{0xff}, 100))
	long := buildChatPrefixRequest(bytes.Repeat([]byte{0xff}, 150))
	require.NoError(t, writer.record(ctx, scope, channel, short))
	matches, err := reader.lookup(ctx, scope, long, []int{1})
	require.NoError(t, err)
	assert.Equal(t, 100, matches[channel].MatchedBytes, "a second instance can read non-UTF-8 sampled bytes")
	assert.Equal(t, 5*time.Minute, server.TTL(chatPrefixRedisKeys[0]))
	require.NoError(t, writer.record(ctx, scope, channel, long))
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	wg.Go(func() { errors <- writer.record(ctx, scope, channel, short) })
	wg.Go(func() { errors <- reader.record(ctx, scope, channel, long) })
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	matches, err = reader.lookup(ctx, scope, long, []int{1})
	require.NoError(t, err)
	assert.Equal(t, 150, matches[channel].MatchedBytes)
	assert.EqualValues(t, 2, first.HLen(ctx, chatPrefixRedisKeys[0]).Val(), "a shorter completion keeps its own record alongside the longest history")
	for _, request := range []*chatPrefixRequest{short, long, short} {
		require.NoError(t, writer.record(ctx, scope, channel, request))
		assert.EqualValues(t, 2, first.HLen(ctx, chatPrefixRedisKeys[0]).Val(), "each exact sampled key remains independently updateable")
	}
	for _, isolated := range []chatPrefixScope{
		{UserID: 43, Group: scope.Group, Model: scope.Model},
		{UserID: scope.UserID, Group: "other", Model: scope.Model},
		{UserID: scope.UserID, Group: scope.Group, Model: "other"},
	} {
		matches, err = reader.lookup(ctx, isolated, long, []int{1})
		require.NoError(t, err)
		assert.Empty(t, matches)
	}
	server.SetTime(now.Add(time.Second))
	require.NoError(t, writer.record(ctx, scope, chatPrefixChannel{ID: 2}, short))
	server.SetTime(now.Add(2 * time.Second))
	require.NoError(t, writer.record(ctx, scope, chatPrefixChannel{ID: 3}, short))
	matches, err = reader.lookup(ctx, scope, short, []int{1, 2, 3})
	require.NoError(t, err)
	assert.Len(t, matches, 2)
	assert.NotContains(t, matches, channel, "capacity is shared across clients")
	reader.capacity = 1
	matches, err = reader.lookup(ctx, scope, short, []int{1, 2, 3})
	require.NoError(t, err)
	assert.Len(t, matches, 1, "reducing capacity trims the shared records")
	reader.ttl = time.Second
	server.SetTime(now.Add(3 * time.Second))
	matches, err = reader.lookup(ctx, scope, short, []int{1, 2, 3})
	require.NoError(t, err)
	assert.Empty(t, matches, "reduced TTL applies to previously written entries")
	require.NoError(t, writer.record(ctx, scope, channel, short))
	reader.capacity = 0
	_, err = reader.lookup(ctx, scope, short, []int{1})
	require.NoError(t, err)
	assert.Empty(t, server.Keys(), "disabling releases every shared index")
	require.NoError(t, writer.record(ctx, scope, channel, short))
	require.NoError(t, first.Del(ctx, chatPrefixRedisKeys[3]).Err())
	reader.capacity = 2
	matches, err = reader.lookup(ctx, scope, short, []int{1})
	require.NoError(t, err)
	assert.Empty(t, matches, "a partially evicted Redis index must start cold")
	require.NoError(t, writer.record(ctx, scope, channel, short))
	server.FastForward(5 * time.Minute)
	assert.Empty(t, server.Keys(), "idle cache expires without a cleanup worker")
	server.SetTime(now)
	reader.ttl = writer.ttl
	sameKeyLong := buildChatPrefixRequest(bytes.Repeat([]byte{0xff}, 110))
	require.NoError(t, writer.record(ctx, scope, channel, sameKeyLong))
	server.FastForward(250 * time.Second)
	server.SetTime(now.Add(250 * time.Second))
	require.NoError(t, writer.record(ctx, scope, chatPrefixChannel{ID: 2}, sameKeyLong))
	server.FastForward(49 * time.Second)
	server.SetTime(now.Add(299 * time.Second))
	require.NoError(t, writer.record(ctx, scope, channel, short))
	assert.Equal(t, 5*time.Minute, server.TTL(chatPrefixRedisKeys[0]), "same-key successful updates reset the shared TTL")
	server.FastForward(2 * time.Second)
	server.SetTime(now.Add(301 * time.Second))
	require.NoError(t, writer.record(ctx, scope, chatPrefixChannel{ID: 3}, sameKeyLong))
	matches, err = reader.lookup(ctx, scope, sameKeyLong, []int{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, 110, matches[channel].MatchedBytes)
	assert.Equal(t, now.Add(299*time.Second), matches[channel].ObservedAt.UTC())
	assert.NotContains(t, matches, chatPrefixChannel{ID: 2})
}

func TestChatPrefixRoutingUsesRedisAcrossInstances(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	createChannelSelectAutoGroupsChannel(t, db, 1, "default", "prefix-model")
	model.InitChannelCache()
	isolateChatPrefixCache(t)
	server := miniredis.RunT(t)
	address := server.Addr()
	if realAddress := os.Getenv("CHAT_PREFIX_TEST_REDIS_ADDR"); realAddress != "" {
		address = realAddress
		originalKeys := chatPrefixRedisKeys
		prefix := "test:{chat-prefix:" + common.GetUUID() + "}:"
		chatPrefixRedisKeys = []string{
			prefix + "records", prefix + "order", prefix + "index", prefix + "sequence",
		}
		t.Cleanup(func() { chatPrefixRedisKeys = originalKeys })
	}
	first := redis.NewClient(&redis.Options{Addr: address, MaxRetries: -1})
	second := redis.NewClient(&redis.Options{Addr: address, MaxRetries: -1})
	previousClient, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = first, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousClient, previousEnabled
		require.NoError(t, first.Del(context.Background(), chatPrefixRedisKeys...).Err())
		require.NoError(t, first.Close())
		require.NoError(t, second.Close())
	})
	channel, err := model.CacheGetChannel(1)
	require.NoError(t, err)
	body := chatPrefixTestBody(t, 4, nil, nil)
	param := chatPrefixTestParam(body)
	BindChatPrefixChannel(param.Ctx, channel)
	RequestPolicy(param.Ctx).Successful = true
	RecordChatPrefixSuccess(param, 1)
	assert.Zero(t, chatPrefixRoutingCache.entries.Len(), "Redis writes must not create a process-local copy")
	common.RDB = second
	match := chatPrefixTestParam(body).ChatPrefixMatches("default", []model.Ability{{ChannelId: 1}})
	assert.Equal(t, len(body), match[1].MatchedBytes, "another client selects using the shared history")
	assert.Zero(t, chatPrefixRoutingCache.entries.Len())
	for i, size := range []int{150, 100, 150, 100, 110, 100} {
		param := chatPrefixTestParam(bytes.Repeat([]byte{0xff}, size))
		BindChatPrefixChannel(param.Ctx, channel)
		RequestPolicy(param.Ctx).Successful = true
		RecordChatPrefixSuccess(param, 1)
		assert.EqualValues(t, min(i+2, 3), first.HLen(context.Background(), chatPrefixRedisKeys[0]).Val(), "short and long histories coexist without accumulating duplicates")
	}
	long := chatPrefixTestParam(bytes.Repeat([]byte{0xff}, 150)).ChatPrefixMatches("default", []model.Ability{{ChannelId: 1}})
	assert.Equal(t, 150, long[1].MatchedBytes, "inserting a short history must preserve the longer match")
	// A Redis failure skips this routing preference instead of using stale
	// local history that would diverge between instances.
	chatPrefixRoutingCache.record(chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}, chatPrefixChannelIdentity(channel), buildChatPrefixRequest(body), time.Now())
	unavailable := redis.NewClient(&redis.Options{Addr: address, MaxRetries: -1})
	require.NoError(t, unavailable.Close())
	common.RDB = unavailable
	assert.Empty(t, chatPrefixTestParam(body).ChatPrefixMatches("default", []model.Ability{{ChannelId: 1}}))
}

func TestChatPrefixRecordsOnlySuccessfulStreamingChatRequests(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	createChannelSelectAutoGroupsChannel(t, db, 1, "default", "prefix-model")
	model.InitChannelCache()
	isolateChatPrefixCache(t)
	channel, err := model.CacheGetChannel(1)
	require.NoError(t, err)
	body := chatPrefixTestBody(t, 4, nil, nil)
	for _, tc := range []struct {
		name         string
		path         string
		method       string
		body         []byte
		userID       int
		success      bool
		off          bool
		nonStreaming bool
		unbound      bool
		want         bool
	}{
		{name: "successful_chat", userID: 42, success: true, want: true},
		{name: "failed_chat", userID: 42},
		{name: "responses", path: "/v1/responses", userID: 42, success: true},
		{name: "messages", path: "/v1/messages", userID: 42, success: true},
		{name: "non_streaming", body: chatPrefixTestBody(t, 4, nil, map[string]any{"stream": false}), userID: 42, success: true, nonStreaming: true},
		{name: "empty_body", body: []byte{}, userID: 42, success: true},
		{name: "no_user", success: true},
		{name: "wrong_method", method: http.MethodGet, userID: 42, success: true},
		{name: "session_off", userID: 42, success: true, off: true},
		{name: "missing_dispatch_identity", userID: 42, success: true, unbound: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chatPrefixRoutingCache = newChatPrefixCache(32, time.Minute)
			param := chatPrefixTestParam(body)
			if tc.body != nil {
				param.RequestBody = tc.body
			}
			if tc.path != "" {
				param.Ctx.Request.URL.Path = tc.path
			}
			if tc.method != "" {
				param.Ctx.Request.Method = tc.method
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyUserId, tc.userID)
			RequestPolicy(param.Ctx).Successful = tc.success
			if tc.off {
				RequestPolicy(param.Ctx).SessionMode = "off"
			}
			if tc.nonStreaming {
				param.DynamicRoutingEligible = false
			}
			if !tc.unbound {
				BindChatPrefixChannel(param.Ctx, channel)
			}
			RecordChatPrefixSuccess(param, 1)
			request := buildChatPrefixRequest(body)
			require.NotNil(t, request)
			matches := chatPrefixRoutingCache.lookup(chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}, request, time.Now())
			assert.Equal(t, tc.want, len(matches) > 0)
		})
	}
}

func TestChatPrefixIsolationAndRawInput(t *testing.T) {
	cache := newChatPrefixCache(4, time.Minute)
	scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
	channel := chatPrefixChannel{ID: 1}
	now := time.Now()
	request := buildChatPrefixRequest(chatPrefixTestBody(t, 4, nil, nil))
	require.NotNil(t, request)
	cache.record(scope, channel, request, now)
	for _, other := range []chatPrefixScope{
		{UserID: 43, Group: scope.Group, Model: scope.Model},
		{UserID: scope.UserID, Group: "vip", Model: scope.Model},
		{UserID: scope.UserID, Group: scope.Group, Model: "other-model"},
	} {
		assert.Empty(t, cache.lookup(other, request, now))
	}
	// The estimator does not normalize JSON formatting or field order.
	changedRawBody := append([]byte(" "), chatPrefixTestBody(t, 4, nil, nil)...)
	formatted := buildChatPrefixRequest(changedRawBody)
	require.NotNil(t, formatted)
	assert.Empty(t, cache.lookup(scope, formatted, now), "different leading wire bytes must not match")
}

func TestChatPrefixRoutingRespectsSelectionBoundaries(t *testing.T) {
	for _, cacheEnabled := range []bool{false, true} {
		for _, tc := range []struct {
			name     string
			dynamic  bool
			priority int64
			allowed  map[int]struct{}
			blocked  bool
			pinned   bool
			pooled   bool
			failed   bool
			want     int
		}{
			{name: "static_prefix_hit", want: 9702},
			{name: "dynamic_prefix_hit", dynamic: true, want: 9702},
			{name: "higher_priority_wins", priority: 100, want: 9701},
			{name: "allowed_candidates_only", allowed: map[int]struct{}{9701: {}}, want: 9701},
			{name: "capacity_denial", blocked: true, want: 9701},
			{name: "pinned_channel", pinned: true, want: 9701},
			{name: "explicit_pool", pooled: true, want: 9701},
			{name: "dynamic_failure", dynamic: true, failed: true, want: 9701},
		} {
			t.Run(fmt.Sprintf("cache_%t/%s", cacheEnabled, tc.name), func(t *testing.T) {
				db := setupChannelSelectAutoGroupsTest(t)
				require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}, &model.UserRoutingStrategy{}))
				common.MemoryCacheEnabled = cacheEnabled
				configureDynamicRoutingForTest(t, tc.dynamic)
				isolateChatPrefixCache(t)
				createPrioritizedChannelSelectFixture(t, db, 9701, "default", "prefix-model", tc.priority)
				createPrioritizedChannelSelectFixture(t, db, 9702, "default", "prefix-model", 0)
				model.InitChannelCache()
				warmChannel, err := model.CacheGetChannel(9702)
				require.NoError(t, err)
				shared := map[int]string{0: strings.Repeat("shared input prefix ", 256)}
				seed := chatPrefixTestParam(chatPrefixTestBody(t, 9, shared, nil))
				BindChatPrefixChannel(seed.Ctx, warmChannel)
				RequestPolicy(seed.Ctx).Successful = true
				RecordChatPrefixSuccess(seed, warmChannel.Id)

				param := chatPrefixTestParam(chatPrefixTestBody(t, 11, shared, nil))
				param.AllowedChannelIds = tc.allowed
				if tc.blocked {
					param.Capacity = &ChannelCapacityState{eligible: map[int]struct{}{}, blocked: map[int]struct{}{9702: {}}}
				}
				if tc.pinned {
					GetChannelConstraints(param.Ctx).AddPin(hostdto.ChannelPin{ChannelId: 9701, Source: hostdto.PinSourceOriginTask})
				}
				if tc.pooled {
					param.routingPools = map[string]*model.ModelRoutingPool{"default": {
						RoutingGroup: "default", Model: param.ModelName, Mode: model.RoutingModePriority,
						Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9701, Enabled: true}},
					}}
				}
				if tc.failed {
					now := time.Now()
					for i := range 4 {
						ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 9701, Model: "prefix-model"}, dynamicrouting.Sample{
							ObservedAt: now.Add(time.Duration(i) * time.Millisecond), TTFT: 100 * time.Millisecond,
							TPOT: time.Millisecond, HasTTFT: true, HasTPOT: true, Success: true,
						})
					}
					ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 9702, Model: "prefix-model"}, dynamicrouting.Sample{
						ObservedAt: now, HardFailure: true,
					})
				}
				selected, group, selectErr := SelectChannelForRequest(param.Ctx, param.ModelName, param)
				require.Nil(t, selectErr)
				require.NotNil(t, selected)
				assert.Equal(t, tc.want, selected.Id)
				if !tc.pinned {
					assert.Equal(t, "default", group)
				}
			})
		}
	}
}

func TestChatPrefixRoutingDropsChangedUpstreamsAndAttemptedChannels(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	isolateChatPrefixCache(t)
	createChannelSelectAutoGroupsChannel(t, db, 9801, "default", "prefix-model")
	model.InitChannelCache()
	channel, err := model.CacheGetChannel(9801)
	require.NoError(t, err)
	body := chatPrefixTestBody(t, 4, nil, nil)
	seed := chatPrefixTestParam(body)
	BindChatPrefixChannel(seed.Ctx, channel)
	RequestPolicy(seed.Ctx).Successful = true
	RecordChatPrefixSuccess(seed, channel.Id)
	candidates := []model.Ability{{ChannelId: channel.Id}}
	param := chatPrefixTestParam(body)
	assert.Contains(t, param.ChatPrefixMatches("default", candidates), channel.Id)
	unrelated := chatPrefixTestParam(chatPrefixTestBody(t, 4, map[int]string{0: "unrelated request"}, nil))
	assert.Empty(t, unrelated.ChatPrefixMatches("default", candidates), "a shared JSON envelope alone must not force channel affinity")
	param.MarkAttempted(channel.Id)
	assert.Empty(t, param.ChatPrefixMatches("default", candidates))

	inflight := chatPrefixTestParam(body)
	BindChatPrefixChannel(inflight.Ctx, channel)
	RequestPolicy(inflight.Ctx).Successful = true
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_mapping", `{"prefix-model":"new-upstream"}`).Error)
	model.InitChannelCache()
	RecordChatPrefixSuccess(inflight, channel.Id)
	assert.Empty(t, chatPrefixTestParam(body).ChatPrefixMatches("default", candidates))
	updated, err := model.CacheGetChannel(channel.Id)
	require.NoError(t, err)
	BindChatPrefixChannel(inflight.Ctx, updated)
	RecordChatPrefixSuccess(inflight, channel.Id)
	assert.Contains(t, chatPrefixTestParam(body).ChatPrefixMatches("default", candidates), channel.Id, "a new attempt binds the updated configuration")
}

func TestChatPrefixDynamicPreferencePreservesProbesAndDiscountOrder(t *testing.T) {
	config := dynamic_routing_setting.GetSnapshot().Config
	config.Enabled, config.ProbeFraction = true, 0.25
	controller, err := dynamicrouting.NewController(config)
	require.NoError(t, err)
	now := time.Now()
	candidates := []dynamicrouting.Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100},
		{ChannelID: 2, Priority: 100, Weight: 100, PrefixMatchedBytes: 100},
		{ChannelID: 3, Priority: 10, Weight: 100, PrefixMatchedBytes: 1000},
	}
	for i, want := range []int{2, 2, 3, 2} {
		decision := controller.SelectAvoiding(dynamicrouting.RouteKey{Group: "default", Model: "prefix-model"}, candidates, nil, now.Add(time.Duration(i)*time.Millisecond))
		require.True(t, decision.HasSelection)
		assert.Equal(t, want, decision.SelectedChannelID, "prefix preference must preserve the regular probe turn")
	}
	controller, err = dynamicrouting.NewController(config)
	require.NoError(t, err)
	cheap, expensive := 0.1, 0.5
	decision := controller.SelectDiscountAvoiding(dynamicrouting.RouteKey{Group: "default", Model: "prefix-model"}, []dynamicrouting.Candidate{
		{ChannelID: 1, Priority: 100, Weight: 100, CostDiscount: &cheap},
		{ChannelID: 2, Priority: 100, Weight: 100, CostDiscount: &expensive, PrefixMatchedBytes: 1000},
	}, nil, dynamicrouting.DiscountLimits{MaxTTFT: time.Second, MaxTPOT: time.Second}, now)
	require.True(t, decision.HasSelection)
	assert.Equal(t, 1, decision.SelectedChannelID, "a warm prefix must not bypass the discount tier")
}

func chatPrefixBenchmarkBody(b testing.TB, messages, messageBytes, branch int) []byte {
	body, _ := chatPrefixBenchmarkPayload(b, messages, messageBytes, branch)
	return body
}

func chatPrefixBenchmarkPayload(b testing.TB, messages, messageBytes, branch int) ([]byte, []int) {
	b.Helper()
	text := strings.Repeat("x", messageBytes)
	changes := make(map[int]string, messages)
	for i := range messages {
		changes[i] = text
	}
	changes[0] = fmt.Sprintf("branch-%08d-%s", branch, text)
	body := chatPrefixTestBody(b, messages, changes, nil)
	// Distinct-history benchmarks need distinct sampled strings. Vary four
	// sampled content bytes deliberately, rather than letting digit changes
	// between sample positions silently collapse all histories into one entry.
	positions := make([]int, 0, 4)
	for _, checkpoint := range buildChatPrefixRequest(body).Checkpoints {
		position := checkpoint.Bytes - 1
		if body[position] == 'x' {
			positions = append(positions, position)
			if len(positions) == 4 {
				break
			}
		}
	}
	require.Len(b, positions, 4)
	value := branch
	for _, position := range positions {
		body[position] = "0123456789abcdef"[value&15]
		value >>= 4
	}
	return body, positions
}

func BenchmarkChatPrefixRequest(b *testing.B) {
	for _, tc := range []struct{ kib, messages int }{{16, 16}, {64, 64}, {256, 128}, {1024, 256}} {
		b.Run(fmt.Sprintf("%dKiB", tc.kib), func(b *testing.B) {
			body := chatPrefixBenchmarkBody(b, tc.messages, tc.kib*1024/tc.messages, 0)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				request := buildChatPrefixRequest(body)
				if request == nil {
					b.Fatal("missing request profile")
				}
				runtime.KeepAlive(request)
			}
		})
	}
}

func BenchmarkChatPrefixLookup(b *testing.B) {
	for _, count := range []int{1, 256, 4096} {
		b.Run(fmt.Sprintf("%dhistories", count), func(b *testing.B) {
			cache := newChatPrefixCache(4096, time.Hour)
			scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
			channel := chatPrefixChannel{ID: 1}
			now := time.Now()
			matchedBytes := 0
			for i := range count {
				request := buildChatPrefixRequest(chatPrefixBenchmarkBody(b, 16, 128, i))
				matchedBytes = request.Bytes
				cache.record(scope, channel, request, now)
			}
			require.Equal(b, count, cache.entries.Len(), "distinct-history setup must actually populate every record")
			request := buildChatPrefixRequest(chatPrefixBenchmarkBody(b, 18, 128, count-1))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				matches := cache.lookup(scope, request, now)
				if matches[channel].MatchedBytes != matchedBytes {
					b.Fatal("wrong prefix match")
				}
			}
		})
	}
}

func BenchmarkChatPrefixRoutingSelection(b *testing.B) {
	db := setupChannelSelectAutoGroupsTest(b)
	for id := 1; id <= 8; id++ {
		createChannelSelectAutoGroupsChannel(b, db, id, "default", "prefix-model")
	}
	model.InitChannelCache()
	original := chatPrefixRoutingCache
	b.Cleanup(func() { chatPrefixRoutingCache = original })
	for _, tc := range []struct{ kib, messages int }{{16, 16}, {64, 64}, {256, 128}, {1024, 256}} {
		body := chatPrefixBenchmarkBody(b, tc.messages, tc.kib*1024/tc.messages, 0)
		for _, mode := range []string{"baseline", "cold", "hit"} {
			b.Run(fmt.Sprintf("%dKiB/%s", tc.kib, mode), func(b *testing.B) {
				chatPrefixRoutingCache = newChatPrefixCache(4096, time.Hour)
				if mode == "hit" {
					channel, err := model.CacheGetChannel(2)
					require.NoError(b, err)
					seed := chatPrefixTestParam(body)
					BindChatPrefixChannel(seed.Ctx, channel)
					RequestPolicy(seed.Ctx).Successful = true
					RecordChatPrefixSuccess(seed, channel.Id)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					param := chatPrefixTestParam(body)
					// The distributor's already-resolved policy is identical in
					// both cases; database reads and upstream calls are excluded.
					param.userRoutingLoaded = true
					param.routingPools = map[string]*model.ModelRoutingPool{"default": nil}
					if mode == "baseline" {
						RequestPolicy(param.Ctx).SessionMode = "off"
					}
					selected, err := selectChannelWithCapacity(param, "default", 0)
					if err != nil || selected == nil || mode == "hit" && selected.Id != 2 {
						b.Fatalf("unexpected routing result: %v, %v", selected, err)
					}
				}
			})
		}
	}
}

func BenchmarkChatPrefixConcurrent(b *testing.B) {
	cache := newChatPrefixCache(4096, time.Hour)
	scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
	channel := chatPrefixChannel{ID: 1}
	matchedBytes := 0
	for i := range 4096 {
		request := buildChatPrefixRequest(chatPrefixBenchmarkBody(b, 16, 128, i))
		matchedBytes = request.Bytes
		cache.record(scope, channel, request, time.Now())
	}
	body := chatPrefixBenchmarkBody(b, 18, 128, 4095)
	b.ReportAllocs()
	b.SetParallelism(8)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			request := buildChatPrefixRequest(body)
			if cache.lookup(scope, request, time.Now())[channel].MatchedBytes != matchedBytes {
				b.Error("wrong concurrent prefix match")
			}
		}
	})
}

func BenchmarkChatPrefixRecord(b *testing.B) {
	for _, kib := range []int{16, 64, 1024} {
		b.Run(fmt.Sprintf("%dKiB", kib), func(b *testing.B) {
			cache := newChatPrefixCache(4096, time.Hour)
			scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
			channel := chatPrefixChannel{ID: 1}
			request := buildChatPrefixRequest(chatPrefixBenchmarkBody(b, 16, kib*1024/16, 0))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				cache.record(scope, channel, request, time.Now())
			}
		})
	}
}

// Run only against an explicitly supplied Redis address. All benchmark data is
// isolated under unique keys; 15,000 histories model 3,000 RPM over five minutes.
func BenchmarkChatPrefixRedis(b *testing.B) {
	address := os.Getenv("CHAT_PREFIX_TEST_REDIS_ADDR")
	if address == "" {
		b.Skip("CHAT_PREFIX_TEST_REDIS_ADDR not configured")
	}
	client := redis.NewClient(&redis.Options{Addr: address, MaxRetries: -1})
	originalKeys := chatPrefixRedisKeys
	prefix := "benchmark:{chat-prefix:" + common.GetUUID() + "}:"
	chatPrefixRedisKeys = []string{prefix + "records", prefix + "order", prefix + "index", prefix + "sequence"}
	ctx := context.Background()
	b.Cleanup(func() {
		require.NoError(b, client.Del(ctx, chatPrefixRedisKeys...).Err())
		chatPrefixRedisKeys = originalKeys
		require.NoError(b, client.Close())
	})
	cache := chatPrefixRedisCache{client: client, capacity: 40960, ttl: 5 * time.Minute}
	body, positions := chatPrefixBenchmarkPayload(b, 16, 64*1024/16, 0)
	scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
	channel := chatPrefixChannel{ID: 1}
	var request *chatPrefixRequest
	for i := range 15000 {
		value := i
		for _, position := range positions {
			body[position] = "0123456789abcdef"[value&15]
			value >>= 4
		}
		request = buildChatPrefixRequest(body)
		require.NoError(b, cache.record(ctx, scope, channel, request))
	}
	require.EqualValues(b, 15000, client.HLen(ctx, chatPrefixRedisKeys[0]).Val())
	var memory int64
	for _, key := range chatPrefixRedisKeys {
		used, err := client.MemoryUsage(ctx, key, 0).Result()
		require.NoError(b, err)
		memory += used
	}
	b.Run("lookup_8channels", func(b *testing.B) {
		b.ReportAllocs()
		b.ReportMetric(float64(memory)/(1024*1024), "Redis-MiB")
		for range b.N {
			matches, err := cache.lookup(ctx, scope, request, []int{1, 2, 3, 4, 5, 6, 7, 8})
			require.NoError(b, err)
			require.Equal(b, request.Bytes, matches[channel].MatchedBytes)
		}
	})
	b.Run("record", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			require.NoError(b, cache.record(ctx, scope, channel, request))
		}
	})
}

func BenchmarkChatPrefixMemory(b *testing.B) {
	for _, tc := range []struct{ histories, kib int }{{1000, 16}, {4096, 16}, {4096, 64}, {4096, 1024}, {10000, 64}} {
		b.Run(fmt.Sprintf("%dhistories_%dKiB", tc.histories, tc.kib), func(b *testing.B) {
			// Run with -benchtime=1x: this measures the retained cache heap
			// after temporary request bodies and parser allocations are GC'd.
			body, positions := chatPrefixBenchmarkPayload(b, 16, tc.kib*1024/16, 0)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			cache := newChatPrefixCache(4096, time.Hour)
			scope := chatPrefixScope{UserID: 42, Group: "default", Model: "prefix-model"}
			channel := chatPrefixChannel{ID: 1}
			for i := range tc.histories {
				value := i
				for _, position := range positions {
					body[position] = "0123456789abcdef"[value&15]
					value >>= 4
				}
				cache.record(scope, channel, buildChatPrefixRequest(body), time.Now())
			}
			require.Equal(b, min(tc.histories, 4096), cache.entries.Len(), "memory simulation must not measure collapsed sample keys")
			runtime.GC()
			runtime.ReadMemStats(&after)
			retained := max(int64(after.HeapAlloc)-int64(before.HeapAlloc), 0)
			b.ReportMetric(float64(retained)/1048576, "retained-MiB")
			b.ReportMetric(float64(retained)/float64(cache.entries.Len()), "retained-B/entry")
			b.ReportMetric(float64(cache.entries.Len()), "records")
			runtime.KeepAlive(body)
			runtime.KeepAlive(cache)
		})
	}
}
