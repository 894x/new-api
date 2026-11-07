package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pre-discount schema is the deployed routing-pool contract. The latest
// upstream release predates routing pools entirely (covered by the fresh case).
type preDiscountRoutingPool struct {
	RoutingGroup string `gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model        string `gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	Mode         string `gorm:"type:varchar(16);not null"`
	Revision     string `gorm:"type:varchar(64);not null"`
	ChannelsJSON string `gorm:"type:text;not null"`
}

func (preDiscountRoutingPool) TableName() string { return "model_routing_pools" }

// The pool schema before tuning already contains discount latency limits.
type preTuningRoutingPool struct {
	RoutingGroup string `gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model        string `gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	Mode         string `gorm:"type:varchar(16);not null"`
	Revision     string `gorm:"type:varchar(64);not null"`
	ChannelsJSON string `gorm:"type:text;not null"`
	MaxTTFTMS    int64  `gorm:"column:max_ttft_ms"`
	MaxTPOTMS    int64  `gorm:"column:max_tpot_ms"`
}

func (preTuningRoutingPool) TableName() string { return "model_routing_pools" }

func TestRoutingPoolTuningDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade_%t", dialect, upgrade), func(t *testing.T) {
				if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
					t.Skip("isolated database DSN not configured")
				}
				t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
				db := setupManageUserTestDB(t)
				originalGroups, originalRatios, originalSetting := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString(), dynamic_routing_setting.GetSetting()
				originalCache := common.MemoryCacheEnabled
				t.Cleanup(func() {
					common.MemoryCacheEnabled = originalCache
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalGroups))
					require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
					require.NoError(t, dynamic_routing_setting.ReplaceAndSync(originalSetting))
				})
				common.MemoryCacheEnabled = false
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Standard","control":"Control"}`))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"control":1}`))
				require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}, &model.Option{}))
				if upgrade {
					require.NoError(t, db.AutoMigrate(&preTuningRoutingPool{}))
					require.NoError(t, db.Create(&preTuningRoutingPool{RoutingGroup: "legacy", Model: "legacy-model", Mode: model.RoutingModeDiscount, Revision: "legacy-revision", ChannelsJSON: "[]", MaxTTFTMS: 4000, MaxTPOTMS: 100}).Error)
				}
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.ModelRoutingPool{}))
				}
				if upgrade {
					legacy, err := model.GetModelRoutingPool("legacy", "legacy-model")
					require.NoError(t, err)
					require.NotNil(t, legacy)
					assert.Nil(t, legacy.Tuning)
					assert.EqualValues(t, 4000, legacy.MaxTTFTMS)
					assert.Equal(t, "legacy-revision", legacy.Revision)
					assert.Error(t, db.Create(&preTuningRoutingPool{RoutingGroup: "legacy", Model: "legacy-model", Mode: model.RoutingModeSmart, ChannelsJSON: "[]"}).Error, "migration retains composite uniqueness")
				}
				name := fmt.Sprintf("tuning-%s-%t", dialect, upgrade)
				for _, id := range []int{9701, 9702} {
					channel := model.Channel{Id: id, Type: 1, Name: fmt.Sprint(id), Models: name, Group: "default,control", Status: common.ChannelStatusEnabled}
					require.NoError(t, db.Create(&channel).Error)
					require.NoError(t, channel.AddAbilities(nil))
				}
				pool := model.ModelRoutingPool{RoutingGroup: "default", Model: name, Mode: model.RoutingModeSmart, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9701, Priority: 100, Weight: 100, Enabled: true}, {ChannelID: 9702, Weight: 100, Enabled: true}}, Tuning: &dynamic_routing_setting.Overrides{MaxSamples: common.GetPointer(5), HardFailureCooldownSeconds: common.GetPointer(10), CooldownSeconds: common.GetPointer(0), ProbeFraction: common.GetPointer(0.4)}}
				require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.ModelRoutingPool{}))
				}
				loaded, err := model.GetModelRoutingPool("default", name)
				require.NoError(t, err)
				require.NotNil(t, loaded.Tuning)
				assert.Equal(t, pool.Tuning, loaded.Tuning)
				assert.Contains(t, loaded.TuningJSON, `"cooldown_seconds":0`)
				assert.NotContains(t, loaded.TuningJSON, "recovery_observation_seconds", "inherited defaults must not be persisted")
				invalid := *loaded
				invalid.Tuning = &dynamic_routing_setting.Overrides{HardFailureCooldownSeconds: common.GetPointer(100), HardFailureMaxCooldownSeconds: common.GetPointer(50)}
				assert.ErrorIs(t, model.SaveModelRoutingPool(&invalid, loaded.Revision), model.ErrInvalidRoutingPool)
				global := originalSetting
				global.MinSamples = 6
				before := dynamic_routing_setting.GetSnapshot()
				assert.ErrorContains(t, model.UpdateDynamicRoutingOptions(global), "invalid tuning for pool")
				assert.Equal(t, before, dynamic_routing_setting.GetSnapshot(), "incompatible global changes must not be published")
				// An old membership editor must preserve newly configured tuning.
				body, err := common.Marshal(loaded)
				require.NoError(t, err)
				var request map[string]any
				require.NoError(t, common.Unmarshal(body, &request))
				delete(request, "tuning")
				body, err = common.Marshal(request)
				require.NoError(t, err)
				response := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(response)
				ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/routing-pool", strings.NewReader(string(body)))
				ctx.Request.Header.Set("Content-Type", "application/json")
				SaveModelRoutingPool(ctx)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), `"hard_failure_cooldown_seconds":10`)
				control := model.ModelRoutingPool{RoutingGroup: "control", Model: name, Mode: model.RoutingModeSmart, Channels: pool.Channels}
				require.NoError(t, model.SaveModelRoutingPool(&control, ""))
				now := time.Now()
				for _, age := range []int{20, 19, 18} {
					for _, id := range []int{9701, 9702} {
						service.ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: id, Model: name}, dynamicrouting.Sample{ObservedAt: now.Add(-time.Duration(age) * time.Second), Success: true, HasTTFT: true, TTFT: 100 * time.Millisecond}, true)
					}
				}
				service.ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 9701, Model: name}, dynamicrouting.Sample{ObservedAt: now.Add(-15 * time.Second), HardFailure: true}, true)
				probed := false
				for range 4 {
					for _, group := range []string{"default", "control"} {
						ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
						channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: ctx, TokenGroup: group, ModelName: name, DynamicRoutingEligible: true})
						require.NoError(t, err)
						require.NotNil(t, channel)
						if group == "default" {
							probed = probed || channel.Id == 9701
						} else {
							assert.Equal(t, 9702, channel.Id, "the other pool keeps the global 30-second cooldown")
						}
					}
				}
				assert.True(t, probed, "the configured 10-second pool must already allow recovery probes")
				loaded.Tuning = &dynamic_routing_setting.Overrides{}
				require.NoError(t, model.SaveModelRoutingPool(loaded, loaded.Revision))
				cleared, err := model.GetModelRoutingPool("default", name)
				require.NoError(t, err)
				assert.Nil(t, cleared.Tuning)
				assert.Empty(t, cleared.TuningJSON)
				response = httptest.NewRecorder()
				ctx, _ = gin.CreateTestContext(response)
				ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-pools", nil)
				ListModelRoutingPools(ctx)
				require.Equal(t, http.StatusOK, response.Code)
				assert.Contains(t, response.Body.String(), `"tuning_defaults"`)
			})
		}
	}
}

func TestDiscountRoutingPoolDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade_%t", dialect, upgrade), func(t *testing.T) {
				if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
					t.Skip("isolated database DSN not configured")
				}
				t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
				db := setupManageUserTestDB(t)
				originalCache := common.MemoryCacheEnabled
				originalGroups := setting.UserUsableGroups2JSONString()
				originalRatios := ratio_setting.GroupRatio2JSONString()
				t.Cleanup(func() {
					common.MemoryCacheEnabled = originalCache
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalGroups))
					require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
				})
				common.MemoryCacheEnabled = false
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Standard"}`))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
				require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
				if upgrade {
					require.NoError(t, db.AutoMigrate(&preDiscountRoutingPool{}))
					require.NoError(t, db.Create(&preDiscountRoutingPool{RoutingGroup: "legacy", Model: "legacy-model", Mode: model.RoutingModeSmart, Revision: "legacy-revision", ChannelsJSON: `[{"channel_id":1,"priority":0,"weight":0,"enabled":true}]`}).Error)
				}
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.ModelRoutingPool{}))
				}
				if upgrade {
					legacy, err := model.GetModelRoutingPool("legacy", "legacy-model")
					require.NoError(t, err)
					require.NotNil(t, legacy)
					assert.Equal(t, model.RoutingModeSmart, legacy.Mode)
					assert.Equal(t, "legacy-revision", legacy.Revision)
					assert.Zero(t, legacy.MaxTTFTMS)
					assert.Zero(t, legacy.MaxTPOTMS)
					assert.Equal(t, 1, legacy.Channels[0].ChannelID)
					assert.Error(t, db.Create(&preDiscountRoutingPool{RoutingGroup: "legacy", Model: "legacy-model", Mode: model.RoutingModeSmart}).Error, "migration retains composite uniqueness")
				}
				channels := []model.Channel{
					{Id: 9901, Type: 1, Name: "Cheap", Models: "discount-model", Group: "default", Status: common.ChannelStatusEnabled, CostDiscount: common.GetPointer(0.5)},
					{Id: 9902, Type: 1, Name: "Expensive", Models: "discount-model", Group: "default", Status: common.ChannelStatusEnabled, CostDiscount: common.GetPointer(0.8)},
				}
				for i := range channels {
					require.NoError(t, db.Create(&channels[i]).Error)
					require.NoError(t, channels[i].AddAbilities(nil))
				}
				pool := model.ModelRoutingPool{RoutingGroup: "default", Model: "discount-model", Mode: model.RoutingModeDiscount, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9901, Enabled: true}, {ChannelID: 9902, Priority: 100, Enabled: true}}}
				require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
				assert.Equal(t, model.DefaultDiscountMaxTTFTMS, pool.MaxTTFTMS)
				assert.Equal(t, model.DefaultDiscountMaxTPOTMS, pool.MaxTPOTMS)
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.ModelRoutingPool{}))
				}
				loaded, err := model.GetModelRoutingPool("default", "discount-model")
				require.NoError(t, err)
				require.NotNil(t, loaded)
				assert.Equal(t, pool.Revision, loaded.Revision)
				assert.Equal(t, pool.MaxTTFTMS, loaded.MaxTTFTMS)
				oldRevision := pool.Revision
				pool.MaxTTFTMS = 7000
				require.NoError(t, model.SaveModelRoutingPool(&pool, oldRevision))
				assert.NotEqual(t, oldRevision, pool.Revision)
				assert.ErrorIs(t, model.SaveModelRoutingPool(&pool, oldRevision), model.ErrRoutingPoolConflict)
				pool.MaxTTFTMS = 9000
				body, err := common.Marshal(pool)
				require.NoError(t, err)
				response := httptest.NewRecorder()
				apiCtx, _ := gin.CreateTestContext(response)
				apiCtx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/routing-pool", strings.NewReader(string(body)))
				apiCtx.Request.Header.Set("Content-Type", "application/json")
				SaveModelRoutingPool(apiCtx)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var saved struct {
					Success bool                   `json:"success"`
					Data    model.ModelRoutingPool `json:"data"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &saved))
				require.True(t, saved.Success)
				assert.EqualValues(t, 9000, saved.Data.MaxTTFTMS)
				pool = saved.Data
				response = httptest.NewRecorder()
				apiCtx, _ = gin.CreateTestContext(response)
				apiCtx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-pools?mode=discount", nil)
				ListModelRoutingPools(apiCtx)
				require.Equal(t, http.StatusOK, response.Code)
				assert.Contains(t, response.Body.String(), `"mode":"discount"`)
				assert.NotContains(t, response.Body.String(), "legacy-model")
				for _, cached := range []bool{false, true} {
					common.MemoryCacheEnabled = cached
					model.InitChannelCache()
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					param := &service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "discount-model@effort:high", DynamicRoutingEligible: true}
					selected, _, err := service.CacheGetRandomSatisfiedChannel(param)
					require.NoError(t, err)
					require.NotNil(t, selected)
					assert.Equal(t, 9901, selected.Id)
					assert.True(t, service.RequestDynamicRoutingEnabled(ctx))
					param.AttemptedChannelIds = map[int]struct{}{9901: {}}
					selected, _, err = service.CacheGetRandomSatisfiedChannel(param)
					require.NoError(t, err)
					require.NotNil(t, selected)
					assert.Equal(t, 9902, selected.Id, "retries must exclude the failed cheap channel")
					// Per-model zero overrides must beat both the channel discount
					// and cached routing metadata, including normalized model names.
					require.NoError(t, db.Create(&model.ChannelModelOverride{ChannelId: 9902, Model: "discount-model", CostDiscount: common.GetPointer(0.0)}).Error)
					param = &service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "discount-model@effort:high", DynamicRoutingEligible: true}
					selected, _, err = service.CacheGetRandomSatisfiedChannel(param)
					require.NoError(t, err)
					require.NotNil(t, selected)
					assert.Equal(t, 9902, selected.Id)
					require.NoError(t, db.Where("channel_id = ?", 9902).Delete(&model.ChannelModelOverride{}).Error)
					// Media/non-streaming retains the explicit priority policy.
					param = &service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "discount-model"}
					selected, _, err = service.CacheGetRandomSatisfiedChannel(param)
					require.NoError(t, err)
					require.NotNil(t, selected)
					assert.Equal(t, 9902, selected.Id)
				}
				invalid := pool
				invalid.MaxTPOTMS = -1
				assert.ErrorIs(t, model.SaveModelRoutingPool(&invalid, pool.Revision), model.ErrInvalidRoutingPool)

				modifierPool := model.ModelRoutingPool{RoutingGroup: "default", Model: "discount-model@effort:high", Mode: model.RoutingModePriority, Channels: pool.Channels}
				for _, stage := range []struct {
					name          string
					effectiveMode *string
					exactPool     bool
				}{
					{name: "inherits_base", effectiveMode: common.GetPointer(model.RoutingModeDiscount)},
					{name: "exact_override", effectiveMode: common.GetPointer(model.RoutingModePriority), exactPool: true},
					{name: "deleted_override", effectiveMode: common.GetPointer(model.RoutingModeDiscount)},
					{name: "deleted_base"},
				} {
					switch stage.name {
					case "exact_override":
						require.NoError(t, model.SaveModelRoutingPool(&modifierPool, ""))
					case "deleted_override":
						require.NoError(t, model.DeleteModelRoutingPool(modifierPool.RoutingGroup, modifierPool.Model, modifierPool.Revision))
					case "deleted_base":
						require.NoError(t, model.DeleteModelRoutingPool(pool.RoutingGroup, pool.Model, pool.Revision))
					}
					response = httptest.NewRecorder()
					apiCtx, _ = gin.CreateTestContext(response)
					apiCtx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-pool?routing_group=default&model="+modifierPool.Model, nil)
					GetModelRoutingPool(apiCtx)
					require.Equal(t, http.StatusOK, response.Code, "%s: %s", stage.name, response.Body.String())
					var metadata struct {
						Success bool `json:"success"`
						Data    struct {
							Pool          *model.ModelRoutingPool `json:"pool"`
							EffectiveMode *string                 `json:"effective_mode"`
						} `json:"data"`
					}
					require.NoError(t, common.Unmarshal(response.Body.Bytes(), &metadata))
					require.True(t, metadata.Success, "%s", stage.name)
					assert.Equal(t, stage.effectiveMode, metadata.Data.EffectiveMode, "%s", stage.name)
					if stage.exactPool {
						require.NotNil(t, metadata.Data.Pool)
						assert.Equal(t, modifierPool.Model, metadata.Data.Pool.Model)
						assert.Equal(t, modifierPool.Revision, metadata.Data.Pool.Revision)
					} else {
						assert.Nil(t, metadata.Data.Pool, "%s: inherited mode must not become an editable exact override", stage.name)
					}
				}
			})
		}
	}
}

func TestRoutingPoolChannelMetricsDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
				t.Skip("isolated database DSN not configured")
			}
			t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
			t.Setenv("TEST_MANAGE_USER_SEPARATE_LOG_DB", "1")
			db := setupManageUserTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ChannelMonitorRecord{}))
			channels := []model.Channel{
				{Id: 9801, Name: "Inherited", Key: "private-metrics-key", Models: "metrics-model", Group: "default", Status: common.ChannelStatusEnabled, CostDiscount: common.GetPointer(0.3)},
				{Id: 9802, Name: "Override", Models: "metrics-model", Group: "default", Status: common.ChannelStatusEnabled, CostDiscount: common.GetPointer(0.5)},
				{Id: 9803, Name: "Unknown", Models: "metrics-model", Group: "default", Status: common.ChannelStatusEnabled},
			}
			for i := range channels {
				require.NoError(t, db.Create(&channels[i]).Error)
				require.NoError(t, channels[i].AddAbilities(nil))
			}
			require.NoError(t, db.Create(&model.ChannelModelOverride{ChannelId: 9802, Model: "metrics-model", CostDiscount: common.GetPointer(0.0)}).Error)
			candidates, err := model.ListModelRoutingPoolCandidates("default", "metrics-model@effort:high")
			require.NoError(t, err)
			require.Len(t, candidates, 3)
			assert.Equal(t, common.GetPointer(0.3), candidates[0].EffectiveCostDiscount)
			assert.Equal(t, common.GetPointer(0.0), candidates[1].EffectiveCostDiscount)
			assert.Nil(t, candidates[2].EffectiveCostDiscount)

			now := time.Now().Unix()
			logs := []model.Log{
				{ChannelId: 9801, ModelName: "metrics-model", Type: model.LogTypeConsume, CreatedAt: now - 10, PromptTokens: 100, Other: `{"cache_tokens":100}`},
				{ChannelId: 9801, ModelName: "metrics-model", Type: model.LogTypeConsume, CreatedAt: now - 20, PromptTokens: 900, Other: `{"cache_tokens":0}`},
				{ChannelId: 9802, ModelName: "metrics-model", Type: model.LogTypeConsume, CreatedAt: now - 10, PromptTokens: 100, Other: `{"cache_tokens":0}`},
				{ChannelId: 9803, ModelName: "metrics-model", Type: model.LogTypeConsume, CreatedAt: now - 10, PromptTokens: 100, Other: `{}`},
				{ChannelId: 9801, ModelName: "another-model", Type: model.LogTypeConsume, CreatedAt: now - 10, PromptTokens: 100, Other: `{"cache_tokens":100}`},
				{ChannelId: 9801, ModelName: "metrics-model", Type: model.LogTypeError, CreatedAt: now - 10, PromptTokens: 100, Other: `{"cache_tokens":100}`},
				{ChannelId: 9801, ModelName: "metrics-model", Type: model.LogTypeConsume, CreatedAt: now - 86401, PromptTokens: 100, Other: `{"cache_tokens":100}`},
				{ChannelId: 9804, ModelName: "metrics-model", Type: model.LogTypeConsume, CreatedAt: now - 10, PromptTokens: 100, Other: `{"cache_tokens":100}`},
			}
			require.NoError(t, model.LOG_DB.Create(&logs).Error)
			previousSettings := *operation_setting.GetMonitorSetting()
			t.Cleanup(func() { *operation_setting.GetMonitorSetting() = previousSettings })
			operation_setting.GetMonitorSetting().ChannelMonitoringEnabled = false
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/monitoring?model=metrics-model&channel_ids=9801,9802,9803", nil)
			GetChannelMonitoring(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Data []model.ChannelMonitorSummary `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Len(t, response.Data, 3)
			for i, expected := range []*float64{common.GetPointer(10.0), common.GetPointer(0.0), nil} {
				require.NotNil(t, response.Data[i].Cache)
				assert.Equal(t, expected, response.Data[i].Cache.HitRate)
				assert.False(t, response.Data[i].Cache.Truncated)
			}
			assert.NotContains(t, recorder.Body.String(), "private-metrics-key")

			// Invalid/missing cache measurements stay distinct from a measured zero.
			require.NoError(t, model.LOG_DB.Create(&[]model.Log{
				{ChannelId: 9803, ModelName: "metrics-model", Type: model.LogTypeConsume, CreatedAt: now - 10, PromptTokens: 100, Other: `{"cache_tokens":-1}`},
				{ChannelId: 9803, ModelName: "metrics-model", Type: model.LogTypeConsume, CreatedAt: now - 10, PromptTokens: 100, Other: `not json`},
			}).Error)
			stats, err := model.QueryChannelCacheStats(context.Background(), []int{9803}, "metrics-model", now-86400, now)
			require.NoError(t, err)
			assert.Nil(t, stats[9803].HitRate)
			// An unavailable log database must not hide the channel's probe status.
			logDB := model.LOG_DB
			model.LOG_DB = nil
			recorder = httptest.NewRecorder()
			ctx, _ = gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/monitoring?model=metrics-model&channel_ids=9801", nil)
			GetChannelMonitoring(ctx)
			model.LOG_DB = logDB
			require.Equal(t, http.StatusOK, recorder.Code)
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Len(t, response.Data, 1)
			assert.Nil(t, response.Data[0].Cache)
		})
	}
}

func TestModelRoutingPoolDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
				t.Skip("isolated database DSN not configured")
			}
			t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
			db := setupManageUserTestDB(t)
			previousCache := common.MemoryCacheEnabled
			previousGroups := setting.UserUsableGroups2JSONString()
			previousRatios := ratio_setting.GroupRatio2JSONString()
			previousDynamic := dynamic_routing_setting.GetSetting()
			t.Cleanup(func() {
				common.MemoryCacheEnabled = previousCache
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
				require.NoError(t, dynamic_routing_setting.ReplaceAndSync(previousDynamic))
			})
			common.MemoryCacheEnabled = false
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Standard","vip":"Fast"}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
			dynamic := previousDynamic
			dynamic.Enabled = false
			require.NoError(t, dynamic_routing_setting.ReplaceAndSync(dynamic))
			// Representative existing routing data is created before the new table.
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			channels := []model.Channel{
				{Id: 9701, Type: 1, Name: "Primary", Key: "private-pool-key", Models: "pool-model,other-model", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(100)), Weight: common.GetPointer(uint(10))},
				{Id: 9702, Type: 1, Name: "Backup", Key: "private-pool-key", Models: "pool-model,other-model", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(50)), Weight: common.GetPointer(uint(20))},
				{Id: 9703, Type: 1, Name: "Other group", Models: "pool-model", Group: "internal", Status: common.ChannelStatusEnabled},
			}
			for i := range channels {
				require.NoError(t, db.Create(&channels[i]).Error)
				require.NoError(t, channels[i].AddAbilities(nil))
			}
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.ModelRoutingPool{}))
			}
			pool := model.ModelRoutingPool{RoutingGroup: "default", Model: "pool-model", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{
				{ChannelID: 9702, Priority: 100, Weight: 0, Enabled: true},
				{ChannelID: 9701, Priority: 0, Weight: 5, Enabled: true},
			}}
			require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
			assert.ErrorIs(t, model.SaveModelRoutingPool(&pool, ""), model.ErrRoutingPoolConflict)
			oldRevision := pool.Revision
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.ModelRoutingPool{}))
			}
			reloaded, err := model.GetModelRoutingPool("default", "pool-model")
			require.NoError(t, err)
			require.NotNil(t, reloaded)
			assert.Equal(t, pool.Channels, reloaded.Channels)
			var unchanged model.Channel
			require.NoError(t, db.First(&unchanged, 9701).Error)
			assert.EqualValues(t, 100, unchanged.GetPriority())
			for _, cached := range []bool{false, true} {
				t.Run(fmt.Sprintf("cache_%t", cached), func(t *testing.T) {
					common.MemoryCacheEnabled = cached
					if cached {
						model.InitChannelCache()
					}
					for _, tc := range []struct {
						group, modelName string
						want             int
					}{{"default", "pool-model", 9702}, {"default", "pool-model@effort:high", 9702}, {"vip", "pool-model", 9701}, {"default", "other-model", 9701}} {
						ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
						common.SetContextKey(ctx, constant.ContextKeyUsingGroup, tc.group)
						param := &service.RetryParam{Ctx: ctx, TokenGroup: tc.group, ModelName: tc.modelName}
						selected, group, err := service.CacheGetRandomSatisfiedChannel(param)
						require.NoError(t, err)
						require.NotNil(t, selected)
						assert.Equal(t, tc.group, group)
						assert.Equal(t, tc.want, selected.Id)
						if tc.group == "default" && tc.modelName == "pool-model" {
							filters, err := param.SelectionFilters(tc.group)
							require.NoError(t, err)
							candidates, err := model.ListChannelSelectionCandidates(tc.group, tc.modelName, filters)
							require.NoError(t, err)
							for _, candidate := range candidates {
								if candidate.ChannelId == 9702 {
									assert.Zero(t, candidate.Weight)
								}
							}
							param.SetRetry(1)
							selected, _, err = service.CacheGetRandomSatisfiedChannel(param)
							require.NoError(t, err)
							require.NotNil(t, selected)
							assert.Equal(t, 9701, selected.Id)
						}
					}
				})
			}
			common.MemoryCacheEnabled = false
			invalid := pool
			invalid.Channels = []model.ModelRoutingPoolChannel{{ChannelID: 9703, Enabled: true}}
			assert.ErrorIs(t, model.SaveModelRoutingPool(&invalid, oldRevision), model.ErrInvalidRoutingPool)
			// An empty explicit pool blocks selection and cannot be bypassed by pins.
			pool.Channels = []model.ModelRoutingPoolChannel{}
			require.NoError(t, model.SaveModelRoutingPool(&pool, oldRevision))
			assert.ErrorIs(t, model.DeleteModelRoutingPool(pool.RoutingGroup, pool.Model, oldRevision), model.ErrRoutingPoolConflict)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
			selected, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "pool-model"})
			require.NoError(t, err)
			assert.Nil(t, selected)
			assert.ErrorIs(t, service.ValidateSelectedChannelGroupPolicy(ctx, 9701, "pool-model"), service.ErrGroupModelChannelDenied)
			selected, _, err = service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "pool-model@effort:high"})
			require.NoError(t, err)
			assert.Nil(t, selected)
			assert.ErrorIs(t, service.ValidateSelectedChannelGroupPolicy(ctx, 9701, "pool-model@effort:high"), service.ErrGroupModelChannelDenied)
			// Per-pool smart mode works while the legacy global switch is off.
			revision := pool.Revision
			pool.Mode, pool.Channels = model.RoutingModeSmart, reloaded.Channels
			require.NoError(t, model.SaveModelRoutingPool(&pool, revision))
			selected, _, err = service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "pool-model", DynamicRoutingEligible: true})
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.True(t, service.RequestDynamicRoutingEnabled(ctx))
			assert.False(t, service.DynamicRoutingEnabled())
			// A complete personal pool supersedes the default ordering, while the
			// administrator's strategy still applies to the personalized pool.
			require.NoError(t, db.Create(&[]model.UserGroupChannelRoutingOverride{
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: 9710, Model: "pool-model", ChannelId: 9701, Priority: common.GetPointer(int64(200))}, RoutingGroup: "default"},
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: 9710, Model: "pool-model", ChannelId: 9702}, RoutingGroup: "default"},
			}).Error)
			personalContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(personalContext, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(personalContext, constant.ContextKeyUserId, 9710)
			personalParam := &service.RetryParam{Ctx: personalContext, TokenGroup: "default", ModelName: "pool-model", DynamicRoutingEligible: true}
			personalFilters, err := personalParam.SelectionFilters("default")
			require.NoError(t, err)
			assert.Nil(t, personalFilters.RoutingPoolChannels)
			require.Contains(t, personalFilters.UserRoutingOverrides, 9701)
			assert.EqualValues(t, 200, *personalFilters.UserRoutingOverrides[9701].Priority)
			selected, _, err = service.CacheGetRandomSatisfiedChannel(personalParam)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.True(t, service.RequestDynamicRoutingEnabled(personalContext))
			// A real failed-attempt observation suppresses the failed channel
			// even when its personal priority is highest.
			for range dynamic.HardFailureThreshold {
				service.ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 9701, Model: "pool-model"}, dynamicrouting.Sample{ObservedAt: time.Now(), HardFailure: true}, true)
			}
			selected, _, err = service.CacheGetRandomSatisfiedChannel(personalParam)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 9702, selected.Id)
			// Overview/detail are redacted and model/group scopes remain independent.
			response := httptest.NewRecorder()
			apiCtx, _ := gin.CreateTestContext(response)
			apiCtx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-pools", nil)
			ListModelRoutingPools(apiCtx)
			assert.Equal(t, http.StatusOK, response.Code)
			assert.NotContains(t, response.Body.String(), "private-pool-key")
			assert.Contains(t, response.Body.String(), "Backup")
			assert.NotContains(t, response.Body.String(), "Other group")
			require.NoError(t, model.DeleteModelRoutingPool(pool.RoutingGroup, pool.Model, pool.Revision))
			selected, _, err = service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "pool-model"})
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 9701, selected.Id)
		})
	}
}

func TestRoutingPoolUserOverviewDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
				t.Skip("isolated database DSN not configured")
			}
			t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
			db := setupManageUserTestDB(t)
			previousGroups, previousRatios := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
			previousCache := common.MemoryCacheEnabled
			t.Cleanup(func() {
				common.MemoryCacheEnabled = previousCache
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
			})
			common.MemoryCacheEnabled = false
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Standard","vip":"Fast"}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`))
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.UserGroupChannelRoutingOverride{}))
			for _, id := range []int{9801, 9802} {
				priority := int64(100)
				if id == 9801 {
					priority = 200
				}
				channel := model.Channel{Id: id, Name: fmt.Sprint(id), Key: "secret-channel-key", Type: 1, Models: "overview-model,other-model", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: &priority}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(nil))
			}
			for _, pool := range []model.ModelRoutingPool{
				{Model: "overview-model", RoutingGroup: "default", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9801, Enabled: true}}},
				{Model: "other-model", RoutingGroup: "vip", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9802, Enabled: true}}},
			} {
				require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
			}
			users := []model.User{
				{Username: "overview-alpha", Role: common.RoleCommonUser, Group: "default", AffCode: "ov1"},
				{Username: "overview-beta", Role: common.RoleCommonUser, Group: "default", AffCode: "ov2"},
				{Username: "overview-strategy", Role: common.RoleCommonUser, Group: "default", AffCode: "ov3"},
				{Username: "overview-inherited", Role: common.RoleCommonUser, Group: "default", AffCode: "ov4"},
				{Username: "overview-deleted", Role: common.RoleCommonUser, Group: "default", AffCode: "ov5"},
			}
			require.NoError(t, db.Create(&users).Error)
			for _, row := range []model.UserGroupChannelRoutingOverride{
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[0].Id, Model: "overview-model", ChannelId: 9801, Disabled: true}, RoutingGroup: "default"},
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[1].Id, Model: "overview-model", ChannelId: 9802}, RoutingGroup: "default"},
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[4].Id, Model: "overview-model", ChannelId: 9801}, RoutingGroup: "default"},
			} {
				require.NoError(t, db.Create(&row).Error)
			}
			require.NoError(t, db.Delete(&users[4]).Error)
			for _, userID := range []int{users[0].Id, users[2].Id} {
				require.NoError(t, model.SaveUserRoutingStrategy(&model.UserRoutingStrategy{UserId: userID, Model: "overview-model", RoutingGroup: "default", Mode: model.RoutingModeSmart}, ""))
			}
			for _, tc := range []struct {
				name, query string
				total, size int
				userIDs     []int
			}{
				{"default_only", "", 2, 2, []int{0, 0}},
				{"all_explicit_pools", "?scope=all", 5, 5, []int{0, 0, users[0].Id, users[1].Id, users[2].Id}},
				{"model_channel_group", "?scope=all&model=overview&channel_id=9801&routing_group=default", 3, 3, []int{0, users[0].Id, users[2].Id}},
				{"strategy_only", "?scope=all&mode=smart", 2, 2, []int{users[0].Id, users[2].Id}},
				{"user_membership_replaces_defaults", "?scope=all&model=overview&channel_id=9802", 1, 1, []int{users[1].Id}},
				{"pagination_after_filtering", "?scope=all&model=overview&channel_id=9801&page_size=2&page=2", 3, 1, []int{users[2].Id}},
				{"no_match", "?scope=all&channel_id=9999", 0, 0, []int{}},
				{"primary_channel", "?scope=all&model=overview&channel_id=9801&channel_role=primary", 2, 2, []int{0, users[2].Id}},
				{"disabled_member_is_not_primary", "?scope=all&model=overview&channel_id=9801&channel_role=non_primary", 1, 1, []int{users[0].Id}},
				{"member_absent_is_not_non_primary", "?scope=all&model=overview&channel_id=9802&channel_role=non_primary", 0, 0, []int{}},
				{"primary_pagination", "?scope=all&model=overview&channel_id=9801&channel_role=primary&page_size=1&page=2", 2, 1, []int{users[2].Id}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					response := userRoutingRequestAtURL(t, 1, http.MethodGet, "/api/channel/routing-pools"+tc.query, "", ListModelRoutingPools)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					var body struct {
						Success bool `json:"success"`
						Data    struct {
							Items []modelRoutingPoolView `json:"items"`
							Total int                    `json:"total"`
						} `json:"data"`
					}
					require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
					require.True(t, body.Success, response.Body.String())
					assert.Equal(t, tc.total, body.Data.Total)
					require.Len(t, body.Data.Items, tc.size)
					ids := make([]int, 0, len(body.Data.Items))
					for _, item := range body.Data.Items {
						ids = append(ids, item.UserID)
					}
					assert.Equal(t, tc.userIDs, ids)
					assert.NotContains(t, response.Body.String(), "secret-channel-key")
					assert.NotContains(t, response.Body.String(), "overview-inherited")
					assert.NotContains(t, response.Body.String(), "overview-deleted")
				})
			}
			require.NoError(t, model.SaveUserRoutingStrategy(&model.UserRoutingStrategy{UserId: users[3].Id, Model: "overview-model@effort:high", RoutingGroup: "default", Mode: model.RoutingModeSmart}, ""))
			for _, tc := range []struct{ channelID, total int }{{9801, 1}, {9802, 0}} {
				response := userRoutingRequestAtURL(t, 1, http.MethodGet, fmt.Sprintf("/api/channel/routing-pools?scope=all&model=overview-model%%40effort%%3Ahigh&channel_id=%d", tc.channelID), "", ListModelRoutingPools)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var body struct {
					Data struct {
						Total int `json:"total"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
				assert.Equal(t, tc.total, body.Data.Total, "model modifiers inherit the base model's pool membership")
			}
			pool, err := model.GetModelRoutingPool("default", "overview-model")
			require.NoError(t, err)
			require.NotNil(t, pool)
			pool.Channels = []model.ModelRoutingPoolChannel{{ChannelID: 9801, Priority: 500, Enabled: false}, {ChannelID: 9802, Enabled: true}}
			require.NoError(t, model.SaveModelRoutingPool(pool, pool.Revision))
			for _, row := range []model.UserGroupChannelRoutingOverride{
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[0].Id, Model: "overview-model", ChannelId: 9802, Priority: common.GetPointer(int64(0))}, RoutingGroup: "default"},
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[1].Id, Model: "overview-model", ChannelId: 9801, Priority: common.GetPointer(int64(0))}, RoutingGroup: "default"},
			} {
				require.NoError(t, db.Create(&row).Error)
			}
			for _, query := range []string{"channel_id=9802&channel_role=primary", "channel_id=9801&channel_role=non_primary"} {
				response := userRoutingRequestAtURL(t, 1, http.MethodGet, "/api/channel/routing-pools?scope=all&model=overview&"+query, "", ListModelRoutingPools)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var body struct {
					Data struct {
						Total int `json:"total"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
				assert.Equal(t, 5, body.Data.Total, "skip disabled members, preserve explicit zero and inherit nil priorities")
			}
			require.NoError(t, db.Model(&model.UserGroupChannelRoutingOverride{}).Where("user_id = ? AND model = ? AND channel_id = ?", users[1].Id, "overview-model", 9801).Update("priority", 100).Error)
			response := userRoutingRequestAtURL(t, 1, http.MethodGet, "/api/channel/routing-pools?scope=all&model=overview&channel_id=9801&channel_role=primary", "", ListModelRoutingPools)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var primaryBody struct {
				Data struct {
					Items []modelRoutingPoolView `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &primaryBody))
			require.Len(t, primaryBody.Data.Items, 1, "equal user priorities follow channel ID order")
			assert.Equal(t, users[1].Id, primaryBody.Data.Items[0].UserID)
			for _, query := range []string{"?channel_role=primary", "?channel_id=9801&channel_role=invalid"} {
				response := userRoutingRequestAtURL(t, 1, http.MethodGet, "/api/channel/routing-pools"+query, "", ListModelRoutingPools)
				assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			}
			require.NoError(t, authz.Init(db))
			administrator := model.User{Username: "overview-admin", Role: common.RoleAdminUser, Group: "default", AffCode: "ov6"}
			require.NoError(t, db.Create(&administrator).Error)
			response = userRoutingRequestAtURL(t, administrator.Id, http.MethodGet, "/api/channel/routing-pools?scope=all", "", func(c *gin.Context) {
				c.Set("id", administrator.Id)
				c.Set("role", administrator.Role)
				ListModelRoutingPools(c)
			})
			assert.Equal(t, http.StatusForbidden, response.Code)
			require.NoError(t, authz.SetUserPermissions(administrator.Id, authz.PermissionsMap{authz.ResourceUserGroupConfig: {authz.ActionRead: true}}))
			require.NoError(t, db.Create(&model.UserGroupChannelRoutingOverride{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: administrator.Id, Model: "overview-model", ChannelId: 9801}, RoutingGroup: "default"}).Error)
			response = userRoutingRequestAtURL(t, administrator.Id, http.MethodGet, "/api/channel/routing-pools?scope=all", "", func(c *gin.Context) {
				c.Set("id", administrator.Id)
				c.Set("role", administrator.Role)
				ListModelRoutingPools(c)
			})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.NotContains(t, response.Body.String(), "overview-admin", "same-role administrators cannot inspect each other's configuration")
		})
	}
}
