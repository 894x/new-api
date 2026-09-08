package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayChannelCapacitySharesUpstreamLimitsAndSpillsBeforeDispatch(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprintf("dynamic=%t", dynamic), func(t *testing.T) {
			testRelayChannelCapacitySpillover(t, dynamic)
		})
	}
}

func testRelayChannelCapacitySpillover(t *testing.T, dynamic bool) {
	previous := dynamic_routing_setting.GetSetting()
	configured := previous
	configured.Enabled = dynamic
	require.NoError(t, dynamic_routing_setting.ReplaceAndSync(configured))
	t.Cleanup(func() { require.NoError(t, dynamic_routing_setting.ReplaceAndSync(previous)) })
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.UserSubscription{}))
	oldCache, oldRetry, oldRatio, oldCount := common.MemoryCacheEnabled, common.RetryTimes, ratio_setting.ModelRatio2JSONString(), constant.CountToken
	server := miniredis.RunT(t)
	oldRedis, oldRDB := common.RedisEnabled, common.RDB
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() { require.NoError(t, common.RDB.Close()); common.RDB = oldRDB; common.RedisEnabled = oldRedis })
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0
	constant.CountToken = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldCache
		common.RetryTimes = oldRetry
		constant.CountToken = oldCount
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
	})
	const publicModel = "capacity-http-regression"
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"capacity-http-regression":0}`))
	var firstHits, secondHits atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits.Add(1)
		assert.Equal(t, "organization-first", r.Header.Get("OpenAI-Organization"))
		if dynamic {
			w.Header().Set("Content-Type", "text/event-stream")
			_, err := w.Write([]byte("data: {\"id\":\"upstream-stream\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
			assert.NoError(t, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"upstream-1","object":"chat.completion","model":"mapped-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		assert.NoError(t, err)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits.Add(1)
		assert.Empty(t, r.Header.Get("OpenAI-Organization"))
		if dynamic {
			w.Header().Set("Content-Type", "text/event-stream")
			_, err := w.Write([]byte("data: {\"id\":\"upstream-stream\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
			assert.NoError(t, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"upstream-2","object":"chat.completion","model":"mapped-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		assert.NoError(t, err)
	}))
	defer second.Close()
	var channels []*model.Channel
	for i, url := range []string{first.URL, second.URL} {
		channel := &model.Channel{Id: 8801 + i, Type: constant.ChannelTypeOpenAI, Key: "local-test-key", Status: common.ChannelStatusEnabled, Name: fmt.Sprint(i), Models: publicModel, Group: "default", BaseURL: &url, Priority: common.GetPointer(int64(100 - i)), RPM: common.GetPointer(int64(1)), ModelMapping: common.GetPointer(`{"capacity-http-regression":"mapped-model"}`)}
		channel.TPM = common.GetPointer(int64(100000))
		if i == 0 {
			channel.OpenAIOrganization = common.GetPointer("organization-first")
		}
		require.NoError(t, channel.Insert())
		channels = append(channels, channel)
	}
	for i := range 3 {
		require.NoError(t, db.Create(&model.User{Id: 8801 + i, Username: fmt.Sprintf("capacity-user-%d", i), AffCode: fmt.Sprintf("cp%d", i), Group: "default", Quota: 1000000}).Error)
	}
	model.InitChannelCache()
	for i := range 3 {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"capacity-http-regression","messages":[{"role":"user","content":"hi"}],"max_tokens":1,"stream":%t}`, dynamic)))
		c.Request.Header.Set("Content-Type", "application/json")
		common.SetContextKey(c, constant.ContextKeyDynamicRoutingEligible, dynamic)
		common.SetContextKey(c, constant.ContextKeyUserId, 8801+i)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenId, 9901+i)
		require.Nil(t, middleware.SetupContextForSelectedChannel(c, channels[0], publicModel))
		Relay(c, types.RelayFormatOpenAI)
		assert.Zero(t, common.GetContextKeyInt(c, constant.ContextKeyPromptTokens), "capacity-only estimation must not write billing prompt metadata")
		if i < 2 {
			assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		} else {
			assert.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), "channel_model_capacity_exhausted")
			assert.NotEmpty(t, recorder.Header().Get("Retry-After"))
		}
	}
	assert.Equal(t, int32(1), firstHits.Load())
	assert.Equal(t, int32(1), secondHits.Load())
}
