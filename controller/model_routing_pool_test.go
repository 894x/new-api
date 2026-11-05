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
