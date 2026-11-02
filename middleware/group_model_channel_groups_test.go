package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributeChannelPoolIntersectionRechecksAffinityAndPinnedChannels(t *testing.T) {
	require.NoError(t, i18n.Init())
	originalDB, originalCache, originalRedis := model.DB, common.MemoryCacheEnabled, common.RedisEnabled
	originalPath, originalMaster := common.SQLitePath, common.IsMasterNode
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	affinity := operation_setting.GetChannelAffinitySetting()
	originalAffinity := *affinity
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	common.SQLitePath, common.IsMasterNode = filepath.Join(t.TempDir(), "routing.db"), false
	require.NoError(t, model.InitDB())
	db := model.DB
	common.SQLitePath, common.IsMasterNode = originalPath, originalMaster
	common.MemoryCacheEnabled, common.RedisEnabled = true, false
	t.Cleanup(func() {
		*affinity = originalAffinity
		model.DB, common.MemoryCacheEnabled, common.RedisEnabled = originalDB, originalCache, originalRedis
		common.SetDatabaseTypes(originalMainType, originalLogType)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}))
	for i, groups := range []string{"vip,pool", "vip", "pool"} {
		id := i + 1
		priority := int64(i * 100)
		channel := model.Channel{Id: id, Name: fmt.Sprintf("policy-channel-%d", id), Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "test-channel-key", Models: "model-a", Group: groups, Priority: &priority}
		require.NoError(t, db.Create(&channel).Error)
		for _, group := range strings.Split(groups, ",") {
			require.NoError(t, db.Create(&model.Ability{Group: group, Model: "model-a", ChannelId: id, Enabled: true, Priority: &priority}).Error)
		}
	}
	model.InitChannelCache()
	*affinity = operation_setting.ChannelAffinitySetting{Enabled: true, DefaultTTLSeconds: 60, SwitchOnSuccess: true, Rules: []operation_setting.ChannelAffinityRule{{
		Name: t.Name(), ModelRegex: []string{"^model-a$"}, IncludeRuleName: true, IncludeModelName: true, IncludeUsingGroup: true,
		KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}},
	}}}
	gin.SetMode(gin.TestMode)
	affinityContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	affinityContext.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	affinityContext.Request.Header.Set("X-Session", "pool-policy-test")
	service.GetPreferredChannelByAffinity(affinityContext, "model-a", "vip")
	service.ClearCurrentChannelAffinityCache(affinityContext)
	t.Cleanup(func() { service.ClearCurrentChannelAffinityCache(affinityContext) })
	router := gin.New()
	dispatched := 0
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "vip")
		if forced := c.Query("channel"); forced != "" {
			id, err := strconv.Atoi(forced)
			require.NoError(t, err)
			service.GetChannelConstraints(c).AddPin(dto.ChannelPin{
				ChannelId: id, Source: dto.PinSourceToken, Rank: dto.PinRankToken, RetryMode: dto.PinRetrySingleAttempt,
			})
		}
		c.Next()
	}, Distribute(), func(c *gin.Context) {
		dispatched++
		c.JSON(http.StatusOK, gin.H{"channel": c.GetInt("channel_id")})
	})

	for _, test := range []struct {
		name, query     string
		members         []model.ModelRoutingPoolChannel
		configured      bool
		status, channel int
	}{
		{name: "seed affinity without policy", status: 200, channel: 2},
		{name: "shared X replaces denied affinity", query: "", members: []model.ModelRoutingPoolChannel{{ChannelID: 1, Weight: 100, Enabled: true}}, configured: true, status: 200, channel: 1},
		{name: "shared X may be pinned", query: "?channel=1", members: []model.ModelRoutingPoolChannel{{ChannelID: 1, Weight: 100, Enabled: true}}, configured: true, status: 200, channel: 1},
		{name: "token-only pinned channel denied", query: "?channel=2", members: []model.ModelRoutingPoolChannel{{ChannelID: 1, Weight: 100, Enabled: true}}, configured: true, status: 403, channel: 0},
		{name: "pool-only pinned channel denied", query: "?channel=3", members: []model.ModelRoutingPoolChannel{{ChannelID: 1, Weight: 100, Enabled: true}}, configured: true, status: 403, channel: 0},
		{name: "empty pool cannot reuse affinity", query: "", configured: true, status: 503, channel: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "shared X replaces denied affinity" {
				channel, found := service.GetPreferredChannelByAffinity(affinityContext, "model-a", "vip")
				require.True(t, found)
				require.Equal(t, 2, channel)
			}
			if test.configured {
				pool, err := model.GetModelRoutingPool("vip", "model-a")
				require.NoError(t, err)
				revision := ""
				if pool != nil {
					revision = pool.Revision
				}
				require.NoError(t, model.SaveModelRoutingPool(&model.ModelRoutingPool{RoutingGroup: "vip", Model: "model-a", Mode: model.RoutingModePriority, Channels: test.members}, revision))
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions"+test.query, strings.NewReader(`{"model":"model-a","messages":[{"role":"user","content":"hello"}]}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Session", "pool-policy-test")
			recorder := httptest.NewRecorder()
			before := dispatched
			router.ServeHTTP(recorder, request)
			require.Equal(t, test.status, recorder.Code, recorder.Body.String())
			if test.channel == 0 {
				assert.Equal(t, before, dispatched)
				return
			}
			assert.JSONEq(t, fmt.Sprintf(`{"channel":%d}`, test.channel), recorder.Body.String())
		})
	}
}
