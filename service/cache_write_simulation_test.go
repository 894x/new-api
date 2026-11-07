package service_test

import (
	"fmt"
	"math"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCacheWriteSimulationBoundariesAndUsageConservation(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		prompt, cached, threshold int
		percentage                float64
		existing, want            int
		anthropic                 bool
	}{
		{name: "threshold equality skips", prompt: 1000, threshold: 1000, percentage: 100},
		{name: "threshold plus one activates", prompt: 1001, threshold: 1000, percentage: 100, want: 1001},
		{name: "fraction rounds down", prompt: 101, percentage: 33.3, want: 33},
		{name: "read cache excluded", prompt: 1000, cached: 200, percentage: 50, want: 400},
		{name: "Claude uses full context threshold", prompt: 800, cached: 200, threshold: 999, percentage: 50, want: 400, anthropic: true},
		{name: "existing write skipped", prompt: 1000, percentage: 100, existing: 10},
		{name: "all cache reads skipped", prompt: 1000, cached: 1000, percentage: 100},
		{name: "zero percentage", prompt: 1000, percentage: 0},
		{name: "negative percentage", prompt: 1000, percentage: -1},
		{name: "excessive percentage", prompt: 1000, percentage: 101},
		{name: "NaN percentage", prompt: 1000, percentage: math.NaN()},
		{name: "infinite percentage", prompt: 1000, percentage: math.Inf(1)},
		{name: "maximum safe input", prompt: math.MaxInt32, percentage: 100, want: math.MaxInt32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := &dto.Usage{PromptTokens: tc.prompt, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: tc.cached, CacheWriteTokens: tc.existing}}
			before, err := common.Marshal(usage)
			require.NoError(t, err)
			adjusted, tokens := common.ApplyCacheWriteSimulation(usage, &dto.CacheWriteSimulationSettings{Enabled: true, MinInputTokens: tc.threshold, Percentage: tc.percentage}, tc.anthropic)
			assert.Equal(t, tc.want, tokens)
			after, err := common.Marshal(usage)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "upstream evidence must remain unchanged")
			if tokens > 0 {
				params := service.BuildTieredTokenParams(adjusted, tc.anthropic, map[string]bool{"cr": true, "cc": true, "cc1h": true})
				context := float64(tc.prompt)
				if tc.anthropic {
					context += float64(tc.cached)
				}
				assert.Equal(t, context, params.Len)
				assert.Equal(t, context, params.P+params.CR+params.CC+params.CC1h)
				assert.Equal(t, float64(tokens), params.CC, "missing TTL defaults to 5m")
			}
		})
	}
	for _, policy := range []*dto.CacheWriteSimulationSettings{
		{Enabled: true, MinInputTokens: -1, Percentage: 50},
		{Enabled: true, MinInputTokens: math.MaxInt32 + 1, Percentage: 50},
		{Enabled: true, Percentage: -1}, {Enabled: true, Percentage: 101},
		{Enabled: true, Percentage: 50, TTL: "2h"},
	} {
		channel := &model.Channel{}
		channel.SetOtherSettings(dto.ChannelOtherSettings{CacheWriteSimulation: policy})
		assert.Error(t, channel.ValidateSettings())
	}
}

func TestCacheWriteSimulationPublicJSONAndSSE(t *testing.T) {
	for _, ttl := range []string{"5m", "1h"} {
		for _, tc := range []struct {
			name, path, body, usagePath, detailPath string
			wantInput                               int
		}{
			{"chat", "/v1/chat/completions", `{"choices":[],"extension":"keep","usage":{"prompt_tokens":1000,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":200,"vendor":true}}}`, "usage", "prompt_tokens_details.cache_write_tokens", 1000},
			{"responses", "/v1/responses", `{"object":"response","extension":"keep","usage":{"input_tokens":1000,"output_tokens":20,"input_tokens_details":{"cached_tokens":200,"vendor":true}}}`, "usage", "input_tokens_details.cache_write_tokens", 1000},
			{"responses stream", "/v1/responses", `{"type":"response.completed","extension":"keep","response":{"usage":{"input_tokens":1000,"output_tokens":20,"input_tokens_details":{"cached_tokens":200}}}}`, "response.usage", "input_tokens_details.cache_write_tokens", 1000},
			{"Claude", "/v1/messages", `{"type":"message","extension":"keep","usage":{"input_tokens":800,"output_tokens":20,"cache_read_input_tokens":200,"cache_creation_input_tokens":0}}`, "usage", "cache_creation_input_tokens", 400},
			{"Claude start", "/v1/messages", `{"type":"message_start","extension":"keep","message":{"usage":{"input_tokens":800,"output_tokens":0,"cache_read_input_tokens":200,"cache_creation_input_tokens":0}}}`, "message.usage", "cache_creation_input_tokens", 400},
		} {
			t.Run(ttl+"/"+tc.name, func(t *testing.T) {
				for _, stream := range []bool{false, true} {
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					ctx.Request = httptest.NewRequest("POST", tc.path, nil)
					common.SetContextKey(ctx, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{CacheWriteSimulation: &dto.CacheWriteSimulationSettings{Enabled: true, Percentage: 50, MinInputTokens: 999, TTL: ttl}})
					if stream {
						require.NoError(t, helper.StringData(ctx, tc.body))
					} else {
						service.IOCopyBytesGracefully(ctx, nil, []byte(tc.body))
					}
					body := strings.TrimSpace(strings.TrimPrefix(recorder.Body.String(), "data: "))
					assert.Equal(t, int64(400), gjson.Get(body, tc.usagePath+"."+tc.detailPath).Int())
					bucket := "ephemeral_5m_input_tokens"
					if ttl == "1h" {
						bucket = "ephemeral_1h_input_tokens"
					}
					assert.Equal(t, int64(400), gjson.Get(body, tc.usagePath+".cache_creation."+bucket).Int())
					assert.Equal(t, "keep", gjson.Get(body, "extension").String())
					assert.NotContains(t, body, "simulation")
					if tc.wantInput == 400 {
						assert.Equal(t, int64(400), gjson.Get(body, tc.usagePath+".input_tokens").Int())
					}
				}
			})
		}
	}
}

func TestCacheWriteSimulationReservesTTLPriceBeforeSubmission(t *testing.T) {
	previous := config.GlobalConfig.ExportAllConfigs()
	previousRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(previous))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatios))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"simulation-legacy":1}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":         `{"simulation-tiered":"tiered_expr","simulation-cheap-cache":"tiered_expr","simulation-fixed":"tiered_expr"}`,
		"billing_setting.billing_expr":         `{"simulation-tiered":"tier(\"base\", p * 2 + cc * 2 + cc1h * 4)","simulation-cheap-cache":"tier(\"base\", p * 2 + cc * 0 + cc1h * 0)","simulation-fixed":"tier(\"request\", fixed(0.01))"}`,
		"group_ratio_setting.group_ratio":      `{"default":1}`,
		"quota_setting.pre_consume_multiplier": "2",
	}))
	for _, tc := range []struct {
		model, ttl string
		want       int
	}{
		{"simulation-legacy", "5m", 2250}, {"simulation-legacy", "1h", 3000},
		{"simulation-tiered", "5m", 2000}, {"simulation-tiered", "1h", 3000},
		{"simulation-cheap-cache", "1h", 2000}, {"simulation-fixed", "1h", 5000},
	} {
		t.Run(tc.model+"/"+tc.ttl, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{OriginModelName: tc.model, UserGroup: "default", UsingGroup: "default", BillingRequestInput: &billingexpr.RequestInput{}, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{CacheWriteSimulation: &dto.CacheWriteSimulationSettings{Enabled: true, Percentage: 50, TTL: tc.ttl}}}}
			price, err := helper.ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, price.QuotaToPreConsume)
		})
	}
}

func TestCacheWriteSimulationConvertedModalitiesKeepResponseAndBillingAligned(t *testing.T) {
	policy := &dto.CacheWriteSimulationSettings{Enabled: true, Percentage: 50}
	usage := dto.Usage{PromptTokens: 1000, CompletionTokens: 20, TotalTokens: 1020,
		PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 200, ImageTokens: 300, AudioTokens: 100}}
	usage.BillingUsage = dto.NewOpenAIChatBillingUsage(&usage)
	converted := service.ResponseOpenAI2Claude(&dto.OpenAITextResponse{Usage: usage}, &relaycommon.RelayInfo{})
	body, err := common.Marshal(converted)
	require.NoError(t, err)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	common.SetContextKey(ctx, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{CacheWriteSimulation: policy})
	public, err := helper.SanitizeResponseBody(body, types.RelayFormatClaude, ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(200), gjson.GetBytes(public, "usage.cache_creation_input_tokens").Int())
	assert.Equal(t, int64(600), gjson.GetBytes(public, "usage.input_tokens").Int())
	assert.False(t, gjson.GetBytes(public, "usage.billing_usage").Exists())
	second, err := common.RewriteResponseCacheWrite(ctx, public)
	require.NoError(t, err)
	assert.Equal(t, string(public), string(second), "serialization must not apply the policy twice")
	_, count := common.ApplyCacheWriteSimulation(&usage, policy, false)
	assert.Equal(t, 200, count, "response and settlement must both exclude image/audio input")
}

func TestCacheWriteSimulationAccountingAcrossDatabases(t *testing.T) {
	for _, engine := range []struct {
		name common.DatabaseType
		env  string
	}{
		{common.DatabaseTypeSQLite, ""},
		{common.DatabaseTypeMySQL, "TEST_SIMULATION_MYSQL_DSN"},
		{common.DatabaseTypePostgreSQL, "TEST_SIMULATION_POSTGRES_DSN"},
	} {
		t.Run(string(engine.name), func(t *testing.T) {
			var driver gorm.Dialector = sqlite.Open(":memory:")
			if engine.env != "" {
				dsn := os.Getenv(engine.env)
				if dsn == "" {
					t.Skip(engine.env + " is not configured")
				}
				if engine.name == common.DatabaseTypeMySQL {
					driver = mysql.Open(dsn)
				} else {
					driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
				}
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			var logDriver gorm.Dialector = sqlite.Open(":memory:")
			if engine.env != "" {
				logEnv := strings.Replace(engine.env, "_DSN", "_LOG_DSN", 1)
				logDSN := os.Getenv(logEnv)
				require.NotEmpty(t, logDSN, logEnv+" is required to verify a separate log database")
				if engine.name == common.DatabaseTypeMySQL {
					logDriver = mysql.Open(logDSN)
				} else {
					logDriver = postgres.New(postgres.Config{DSN: logDSN, PreferSimpleProtocol: true})
				}
			}
			logDB, err := gorm.Open(logDriver, &gorm.Config{})
			require.NoError(t, err)
			logSQL, err := logDB.DB()
			require.NoError(t, err)
			logSQL.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, logSQL.Close()) })
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
			model.DB, model.LOG_DB = db, logDB
			common.SetDatabaseTypes(engine.name, engine.name)
			t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLogDB; common.SetDatabaseTypes(oldMain, oldLog) })
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.ChannelModelOverride{}, &model.BillingAdmissionReserveOperation{}, &model.BillingRefundOperation{}))
				require.NoError(t, logDB.AutoMigrate(&model.Log{}))
				require.NoError(t, model.MigrateGrossProfit(db))
			}
			versionQuery := "select version()"
			if engine.name == common.DatabaseTypeSQLite {
				versionQuery = "select sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database: %s", version)
			require.NoError(t, logDB.Raw(versionQuery).Scan(&version).Error)
			t.Logf("log database: %s", version)
			for _, semantic := range []string{"openai", "anthropic", "openrouter"} {
				for _, ttl := range []string{"5m", "1h"} {
					for _, expression := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/%s/expression=%t", semantic, ttl, expression), func(t *testing.T) {
							const initial = 20000
							user := model.User{Username: fmt.Sprintf("sim_%s_%s_%t", semantic, ttl, expression), Quota: initial, Status: common.UserStatusEnabled}
							user.AffCode = user.Username
							require.NoError(t, db.Create(&user).Error)
							token := model.Token{UserId: user.Id, Key: user.Username, Name: "simulation-test", RemainQuota: initial, Status: common.TokenStatusEnabled}
							require.NoError(t, db.Create(&token).Error)
							channel := model.Channel{Name: "simulation", Key: "unused", Status: common.ChannelStatusEnabled}
							channel.SetOtherSettings(dto.ChannelOtherSettings{CacheWriteSimulation: &dto.CacheWriteSimulationSettings{Enabled: true, MinInputTokens: 999, Percentage: 50, TTL: ttl}})
							require.NoError(t, channel.ValidateSettings())
							require.NoError(t, db.Create(&channel).Error)
							t.Cleanup(func() {
								require.NoError(t, logDB.Where("user_id = ?", user.Id).Delete(&model.Log{}).Error)
								require.NoError(t, db.Unscoped().Delete(&token).Error)
								require.NoError(t, db.Unscoped().Delete(&channel).Error)
								require.NoError(t, db.Unscoped().Delete(&user).Error)
							})
							var loaded model.Channel
							require.NoError(t, db.First(&loaded, channel.Id).Error)
							assert.Equal(t, channel.GetOtherSettings(), loaded.GetOtherSettings())
							info := &relaycommon.RelayInfo{RequestId: user.Username, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, ChannelOtherSettings: loaded.GetOtherSettings()}, OriginModelName: "simulation-test", UsingGroup: "default", UserGroup: "default", UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, ForcePreConsume: true, StartTime: time.Now(), RelayFormat: types.RelayFormatOpenAI,
								PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 1, CacheRatio: 0.1, CacheCreationRatio: 1, CacheCreation5mRatio: 1, CacheCreation1hRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
							usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 20, TotalTokens: 1020, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 200}}
							usage.BillingUsage = dto.NewOpenAIChatBillingUsage(usage)
							path := "/v1/chat/completions"
							if semantic == "anthropic" {
								path = "/v1/messages"
								info.RelayFormat, info.FinalRequestRelayFormat = types.RelayFormatClaude, types.RelayFormatClaude
								usage.BillingUsage = dto.NewClaudeMessagesBillingUsage(&dto.ClaudeUsage{InputTokens: 800, OutputTokens: 20, CacheReadInputTokens: 200})
							}
							if semantic == "openrouter" {
								info.ChannelType = constant.ChannelTypeOpenRouter
								info.FinalRequestRelayFormat = types.RelayFormatClaude
								usage.BillingUsage = nil
							}
							if expression {
								const expr = `tier("base", p * 2 + c * 2 + cr * 0.2 + cc * 2 + cc1h * 4)`
								info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), GroupRatio: 1, QuotaPerUnit: common.QuotaPerUnit, EstimatedQuotaBeforeGroup: 1000, EstimatedQuotaAfterGroup: 1000}
							}
							ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
							ctx.Request = httptest.NewRequest("POST", path, nil)
							common.SetContextKey(ctx, constant.ContextKeyChannelOtherSetting, loaded.GetOtherSettings())
							require.Nil(t, service.PreConsumeBilling(ctx, 1000, info))
							service.PostTextConsumeQuota(ctx, info, usage, nil)
							want := 840
							if ttl == "1h" {
								want = 1240
							}
							require.NoError(t, info.Billing.Settle(want), "settlement replay must not charge twice")
							require.NoError(t, db.First(&user, user.Id).Error)
							require.NoError(t, db.First(&token, token.Id).Error)
							assert.Equal(t, initial-want, user.Quota)
							assert.Equal(t, initial-want, token.RemainQuota)
							var log model.Log
							require.NoError(t, logDB.Where("user_id = ?", user.Id).Take(&log).Error)
							assert.Equal(t, want, log.Quota)
							assert.Equal(t, int64(400), gjson.Get(log.Other, "cache_creation_tokens_"+ttl).Int())
							assert.Equal(t, int64(400), gjson.Get(log.Other, "admin_info.cache_write_simulation.tokens").Int())
							assert.Equal(t, int64(400), info.PerformanceCacheWriteTokens)
							original := usage
							if usage.BillingUsage != nil {
								var ok bool
								original, ok = usage.BillingUsage.CanonicalUsage()
								require.True(t, ok)
							}
							writes, _, _ := original.CacheCreationTokenCounts()
							assert.Zero(t, writes)
						})
					}
				}
			}
		})
	}
}
