package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapacity"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCapacitySpilloverPreservesAutoGroupAndParameterFilters(t *testing.T) {
	for _, tc := range []struct{ cache, dynamic bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		t.Run(fmt.Sprintf("cache=%t/dynamic=%t", tc.cache, tc.dynamic), func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			configureDynamicRoutingForTest(t, tc.dynamic)
			previousRedis, previousLimiter, previousClock := common.RedisEnabled, channelCapacityMemoryLimiter, channelCapacityNow
			common.RedisEnabled = false
			channelCapacityMemoryLimiter = channelcapacity.NewMemoryLimiter()
			channelCapacityNow = func() time.Time { return time.Unix(120, 0) }
			t.Cleanup(func() {
				common.RedisEnabled = previousRedis
				channelCapacityMemoryLimiter = previousLimiter
				channelCapacityNow = previousClock
			})
			for _, id := range []int{8201, 8202, 8203, 8204} {
				group := "vip"
				if id == 8204 {
					group = "default"
				}
				createChannelSelectAutoGroupsChannel(t, db, id, group, "capacity-model")
				var channel model.Channel
				require.NoError(t, db.First(&channel, id).Error)
				channel.RPM = common.GetPointer(int64(1))
				channel.Priority = common.GetPointer(int64(100 - id%100))
				require.NoError(t, channel.Update())
			}
			setChannelSelectResolutionParameters(t, db, 8202, "720p")
			setChannelSelectResolutionParameters(t, db, 8204, "720p")
			model.InitChannelCache()
			common.MemoryCacheEnabled = tc.cache
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
			common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
			param := &RetryParam{DynamicRoutingEligible: true, Ctx: c, TokenGroup: "auto", ModelName: "capacity-model", RequestPath: c.Request.URL.Path, RequestBody: []byte(`{"metadata":{"resolution":"1080p"}}`)}
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions, OriginModelName: param.ModelName, ChannelMeta: &relaycommon.ChannelMeta{}}
			needsTokens, err := ConfigureChannelModelCapacity(param, info)
			require.NoError(t, err)
			assert.False(t, needsTokens)
			first, group, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Equal(t, 8201, first.Id)
			assert.Equal(t, "vip", group)
			info.ChannelId = first.Id
			require.NoError(t, AdmitFinalChannelModelCapacity(c, info, strings.NewReader(`{}`)))
			var denied *ChannelModelCapacityError
			require.ErrorAs(t, AdmitFinalChannelModelCapacity(c, info, strings.NewReader(`{}`)), &denied)
			param.RetryAfterCapacityDenial()
			param.IncreaseRetry()
			second, group, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.Equal(t, 8203, second.Id, "skip incompatible channel, retain the current group even with RetryTimes=0")
			assert.Equal(t, "vip", group)
			info.ChannelId = second.Id
			require.NoError(t, AdmitFinalChannelModelCapacity(c, info, strings.NewReader(`{}`)))
			require.ErrorAs(t, AdmitFinalChannelModelCapacity(c, info, strings.NewReader(`{}`)), &denied)
			param.RetryAfterCapacityDenial()
			param.IncreaseRetry()
			selected, _, err := CacheGetRandomSatisfiedChannel(param)
			assert.Nil(t, selected)
			require.ErrorAs(t, err, &denied, "a later incompatible group cannot mask capacity exhaustion")
			assert.Equal(t, time.Minute, denied.RetryAfter)
		})
	}
}

func TestFinalChannelCapacityReservesOutputAndPreservesExplicitZero(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	createChannelSelectAutoGroupsChannel(t, db, 8301, "default", "capacity-model")
	var channel model.Channel
	require.NoError(t, db.First(&channel, 8301).Error)
	channel.TPM = common.GetPointer(int64(100))
	require.NoError(t, channel.Update())
	model.InitChannelCache()
	previousRedis, previousCount, previousLimiter := common.RedisEnabled, constant.CountToken, channelCapacityMemoryLimiter
	common.RedisEnabled = false
	constant.CountToken = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedis
		constant.CountToken = previousCount
		channelCapacityMemoryLimiter = previousLimiter
	})
	for _, tc := range []struct {
		name, body string
		allowed    bool
	}{
		{"explicit zero", `{"messages":[{"role":"user","content":"hi"}],"max_tokens":0}`, true},
		{"missing output", `{"messages":[{"role":"user","content":"hi"}]}`, false},
		{"explicit maximum", `{"messages":[],"max_tokens":101}`, false},
		{"multiple choices", `{"messages":[],"max_tokens":60,"n":2}`, false},
		{"transformed prompt", `{"messages":[{"role":"system","content":"` + strings.Repeat("hello ", 120) + `"}],"max_tokens":0}`, false},
		{"unsigned overflow", `{"messages":[],"max_tokens":18446744073709551615}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channelCapacityMemoryLimiter = channelcapacity.NewMemoryLimiter()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			c.Set("specific_channel_id", 8301)
			common.SetContextKey(c, constant.ContextKeyChannelId, 8301)
			param := &RetryParam{Ctx: c, TokenGroup: "default", ModelName: "capacity-model", RequestPath: c.Request.URL.Path}
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions, OriginModelName: param.ModelName, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 8301}}
			needed, err := ConfigureChannelModelCapacity(param, info)
			require.NoError(t, err)
			assert.True(t, needed)
			body, closer, err := relaycommon.NewOutboundJSONBody([]byte(tc.body))
			require.NoError(t, err)
			defer closer.Close()
			err = AdmitFinalChannelModelCapacity(c, info, body)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				var denied *ChannelModelCapacityError
				require.ErrorAs(t, err, &denied)
			}
			bytes, readErr := body.NewReader()
			require.NoError(t, readErr)
			require.NoError(t, bytes.Close())
		})
	}
}
