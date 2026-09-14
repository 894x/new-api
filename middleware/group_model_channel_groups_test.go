package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
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
	originalPolicy := setting.GroupModelChannelGroupsJSON()
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
		require.NoError(t, setting.UpdateGroupModelChannelGroups(originalPolicy))
		model.DB, common.MemoryCacheEnabled, common.RedisEnabled = originalDB, originalCache, originalRedis
		common.SetDatabaseTypes(originalMainType, originalLogType)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}))
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
			common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, forced)
		}
		c.Next()
	}, Distribute(), func(c *gin.Context) {
		dispatched++
		c.JSON(http.StatusOK, gin.H{"channel": c.GetInt("channel_id")})
	})

	for _, test := range []struct {
		name, policy, query string
		status, channel     int
	}{
		{"seed affinity without policy", `{}`, "", 200, 2},
		{"shared X replaces denied affinity", `{"default":{"model-a":["pool"]}}`, "", 200, 1},
		{"shared X may be pinned", `{"default":{"model-a":["pool"]}}`, "?channel=1", 200, 1},
		{"token-only pinned channel denied", `{"default":{"model-a":["pool"]}}`, "?channel=2", 403, 0},
		{"pool-only pinned channel denied", `{"default":{"model-a":["pool"]}}`, "?channel=3", 403, 0},
		{"empty pool cannot reuse affinity", `{"default":{"model-a":[]}}`, "", 503, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "shared X replaces denied affinity" {
				channel, found := service.GetPreferredChannelByAffinity(affinityContext, "model-a", "vip")
				require.True(t, found)
				require.Equal(t, 2, channel)
			}
			require.NoError(t, setting.UpdateGroupModelChannelGroups(test.policy))
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
