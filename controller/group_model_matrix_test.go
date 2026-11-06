package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGroupModelMatrixDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
				t.Skip("isolated database DSN not configured")
			}
			t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
			db := setupManageUserTestDB(t)
			common.OptionMapRWMutex.Lock()
			previousOptions := common.OptionMap
			common.OptionMap = make(map[string]string)
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
			})
			require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			previousCache := common.MemoryCacheEnabled
			previousGroups := setting.UserUsableGroups2JSONString()
			previousRatios := ratio_setting.GroupRatio2JSONString()
			previousDynamic := dynamic_routing_setting.GetSetting()
			t.Cleanup(func() {
				_, revision, err := model.GetGroupModelAccess()
				require.NoError(t, err)
				_, err = model.SaveGroupModelAccess(model.GroupModelAccess{}, revision)
				require.NoError(t, err)
				common.MemoryCacheEnabled = previousCache
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
				require.NoError(t, dynamic_routing_setting.ReplaceAndSync(previousDynamic))
			})
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Standard","vip":"Premium"}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
			channel := model.Channel{Id: 9901, Type: 1, Name: "Shared", Models: "model-a,model-b", Group: "default,vip", Status: common.ChannelStatusEnabled, Key: "private-matrix-key"}
			require.NoError(t, db.Create(&channel).Error)
			require.NoError(t, channel.AddAbilities(nil))
			pool := model.ModelRoutingPool{RoutingGroup: "default", Model: "model-b", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: channel.Id, Enabled: true}}}
			require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
			config, revision, err := model.GetGroupModelAccess()
			require.NoError(t, err)
			assert.Empty(t, config)
			assert.True(t, model.IsGroupModelAllowed("default", "model-b"), "old installations remain unrestricted")
			newRevision, err := model.SaveGroupModelAccess(model.GroupModelAccess{"default": {"model-a"}}, revision)
			require.NoError(t, err)
			_, err = model.SaveGroupModelAccess(model.GroupModelAccess{}, revision)
			assert.ErrorIs(t, err, model.ErrGroupModelAccessConflict)
			assert.False(t, model.IsGroupModelAllowed("default", "model-b"))
			assert.True(t, model.IsGroupModelAllowed("vip", "model-b"))
			assert.True(t, model.IsGroupModelAllowed("default", "model-a@effort:high"))
			assert.False(t, model.IsGroupModelAllowed("default", "model-b@effort:high"))
			assert.False(t, model.IsGroupModelAllowed("default", "new-model"), "new models are opt-in for restricted groups")
			writeFailure := errors.New("matrix persistence unavailable")
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("matrix_test_write_failure", func(tx *gorm.DB) {
				tx.AddError(writeFailure)
			}))
			_, err = model.SaveGroupModelAccess(model.GroupModelAccess{}, newRevision)
			require.NoError(t, db.Callback().Update().Remove("matrix_test_write_failure"))
			assert.ErrorIs(t, err, writeFailure)
			assert.False(t, model.IsGroupModelAllowed("default", "model-b"), "failed persistence must not open access")

			// Reloading and running existing schema setup again preserves both the
			// option and pre-existing channel/pool data; no ability rows are deleted.
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.Option{}))
				stored, loadedRevision, err := model.GetGroupModelAccess()
				require.NoError(t, err)
				assert.Equal(t, model.GroupModelAccess{"default": {"model-a"}}, stored)
				assert.Equal(t, newRevision, loadedRevision)
			}
			var count int64
			require.NoError(t, db.Model(&model.Ability{}).Count(&count).Error)
			assert.EqualValues(t, 4, count)

			for _, cached := range []bool{false, true} {
				for _, dynamic := range []bool{false, true} {
					t.Run(fmt.Sprintf("cache=%t/dynamic=%t", cached, dynamic), func(t *testing.T) {
						common.MemoryCacheEnabled = cached
						model.InitChannelCache()
						settings := previousDynamic
						settings.Enabled = dynamic
						require.NoError(t, dynamic_routing_setting.ReplaceAndSync(settings))
						for _, name := range []string{"model-b", "model-b@effort:high"} {
							ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
							common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
							common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
							param := &service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: name, DynamicRoutingEligible: dynamic}
							selected, _, err := service.CacheGetRandomSatisfiedChannel(param)
							require.NoError(t, err)
							assert.Nil(t, selected, "explicit pools cannot bypass the matrix")
							assert.ErrorIs(t, service.ValidateSelectedChannelGroupPolicy(ctx, channel.Id, name), service.ErrGroupModelChannelDenied)
							assert.False(t, model.IsChannelEnabledForGroupModel("default", name, channel.Id))
							common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")
							common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
							param = &service.RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: name, DynamicRoutingEligible: dynamic}
							selected, group, err := service.CacheGetRandomSatisfiedChannel(param)
							require.NoError(t, err)
							require.NotNil(t, selected)
							assert.Equal(t, "vip", group)
						}
						assert.Equal(t, []string{"model-a"}, model.GetGroupEnabledModels("default"))
					})
				}
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/group-model-matrix", nil)
			GetGroupModelMatrix(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "model-b", "denied models remain editable")
			assert.NotContains(t, recorder.Body.String(), channel.Key)
			t.Run("only_public_groups", func(t *testing.T) {
				publicGroups := setting.UserUsableGroups2JSONString()
				groupRatios := ratio_setting.GroupRatio2JSONString()
				originalConfig, _, err := model.GetGroupModelAccess()
				require.NoError(t, err)
				t.Cleanup(func() {
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(publicGroups))
					require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatios))
					_, currentRevision, err := model.GetGroupModelAccess()
					require.NoError(t, err)
					_, err = model.SaveGroupModelAccess(originalConfig, currentRevision)
					require.NoError(t, err)
				})
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"vip":"Premium","empty":"No channels","stale":"No ratio","auto":"Automatic"}`))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2,"empty":1,"ratio-only":1,"auto":1}`))
				hiddenConfig := model.GroupModelAccess{"default": {"model-a"}, "config-only": {"model-a"}}
				_, err = model.SaveGroupModelAccess(hiddenConfig, newRevision)
				require.NoError(t, err)

				for _, public := range []bool{true, false} {
					if !public {
						require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{}`))
					}
					recorder := httptest.NewRecorder()
					ctx, _ := gin.CreateTestContext(recorder)
					GetGroupModelMatrix(ctx)
					require.Equal(t, http.StatusOK, recorder.Code)
					var response struct {
						Success bool
						Data    struct {
							Groups []string
							Config model.GroupModelAccess
						}
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
					require.True(t, response.Success)
					if public {
						assert.Equal(t, []string{"empty", "vip"}, response.Data.Groups)
					} else {
						assert.Empty(t, response.Data.Groups, "no public groups must not fall back to internal groups")
					}
					assert.Equal(t, hiddenConfig, response.Data.Config, "hidden policies must survive matrix saves")
				}
			})
			for _, request := range []struct {
				body   string
				status int
			}{
				{fmt.Sprintf(`{"config":{},"revision":%q}`, revision), http.StatusConflict},
				{fmt.Sprintf(`{"config":{"default":null},"revision":%q}`, newRevision), http.StatusBadRequest},
				{fmt.Sprintf(`{"config":{"default":["model-a"]},"revision":%q}`, newRevision), http.StatusOK},
			} {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/group-model-matrix", strings.NewReader(request.body))
				ctx.Request.Header.Set("Content-Type", "application/json")
				SaveGroupModelMatrix(ctx)
				assert.Equal(t, request.status, recorder.Code, recorder.Body.String())
			}
			recorder = httptest.NewRecorder()
			ctx, _ = gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/routing-pools?page=1&page_size=12", nil)
			ListModelRoutingPools(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"total":0`)
			_, err = model.SaveGroupModelAccess(model.GroupModelAccess{"default": {}}, newRevision)
			require.NoError(t, err)
			assert.False(t, model.IsGroupModelAllowed("default", "model-a"), "empty whitelist denies all")
			_, currentRevision, err := model.GetGroupModelAccess()
			require.NoError(t, err)
			_, err = model.SaveGroupModelAccess(model.GroupModelAccess{}, currentRevision)
			require.NoError(t, err)
			assert.True(t, model.IsGroupModelAllowed("default", "model-b"))
			preservedPool, err := model.GetModelRoutingPool("default", "model-b")
			require.NoError(t, err)
			require.NotNil(t, preservedPool)
			assert.Equal(t, pool.Revision, preservedPool.Revision)
			for _, invalid := range []string{`null`, `{"auto":[]}`, `{"default":null}`, `{"default":[""]}`, `{"default":[" model-a"]}`} {
				_, err := model.ParseGroupModelAccess(invalid)
				assert.Error(t, err)
			}
		})
	}
}
