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
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func routingServiceTypeParam(userID int, name string) *service.RetryParam {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserId, userID)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	return &service.RetryParam{Ctx: ctx, ModelName: name, TokenGroup: "default"}
}

func TestRoutingServiceTypeDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade_%t", dialect, upgrade), func(t *testing.T) {
				if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
					t.Skip("isolated database DSN not configured")
				}
				t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
				db := setupManageUserTestDB(t)
				cache, groups, ratios := common.MemoryCacheEnabled, setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
				t.Cleanup(func() {
					common.MemoryCacheEnabled = cache
					assert.NoError(t, setting.UpdateUserUsableGroupsByJSONString(groups))
					assert.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(ratios))
				})
				common.MemoryCacheEnabled = false
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Standard","control":"Control"}`))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"control":1}`))
				require.NoError(t, db.Migrator().DropTable(&model.UserRoutingServiceType{}, &model.RoutingServiceTypePool{}, &model.RoutingServiceType{}))
				require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
				for _, id := range []int{9811, 9812, 9813} {
					channel := model.Channel{Id: id, Type: 1, Name: fmt.Sprint(id), Models: "service-model,unconfigured", Group: "default,control", Status: common.ChannelStatusEnabled}
					require.NoError(t, db.Create(&channel).Error)
					require.NoError(t, channel.AddAbilities(nil))
				}
				for _, id := range []int{8011, 8012} {
					require.NoError(t, db.Create(&model.User{Id: id, Username: fmt.Sprint(id), AffCode: fmt.Sprint(id), Role: common.RoleCommonUser, Group: "default"}).Error)
				}
				legacy := model.ModelRoutingPool{RoutingGroup: "default", Model: "service-model", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9811, Weight: 100, Enabled: true}}}
				if upgrade {
					require.NoError(t, model.SaveModelRoutingPool(&legacy, ""))
					require.NoError(t, db.Create(&model.UserGroupChannelRoutingOverride{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: 8011, Model: "service-model", ChannelId: 9813}, RoutingGroup: "default"}).Error)
				}
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.RoutingServiceType{}, &model.RoutingServiceTypePool{}, &model.UserRoutingServiceType{}, &model.ModelRoutingPool{}, &model.UserGroupChannelRoutingOverride{}))
				}
				if upgrade {
					loaded, err := model.GetModelRoutingPool("default", "service-model")
					require.NoError(t, err)
					require.NotNil(t, loaded)
					assert.Equal(t, legacy.Revision, loaded.Revision)
					assert.Equal(t, legacy.Channels, loaded.Channels)
					rows, err := model.ListUserGroupChannelRoutingOverrides(8011, "service-model")
					require.NoError(t, err)
					require.Len(t, rows, 1)
					assert.Equal(t, 9813, rows[0].ChannelId)
					require.NoError(t, db.Where("user_id = ?", 8011).Delete(&model.UserGroupChannelRoutingOverride{}).Error)
				} else {
					require.NoError(t, model.SaveModelRoutingPool(&legacy, ""))
				}
				types := []model.RoutingServiceType{{Name: "Economy"}, {Name: "Quality"}}
				for i := range types {
					require.NoError(t, model.SaveRoutingServiceType(&types[i], ""))
				}
				assert.Error(t, model.SaveRoutingServiceType(&model.RoutingServiceType{Name: "Economy"}, ""), "names remain unique")
				shared := model.ModelRoutingPool{ServiceTypeID: types[0].ID, RoutingGroup: "default", Model: "service-model", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9812, Weight: 0, Priority: 0, Enabled: true}}}
				quality := model.ModelRoutingPool{ServiceTypeID: types[1].ID, RoutingGroup: "default", Model: "service-model", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9813, Weight: 100, Enabled: true}}}
				for _, pool := range []*model.ModelRoutingPool{&shared, &quality} {
					body, err := common.Marshal(pool)
					require.NoError(t, err)
					response := userRoutingRequestAtURL(t, 8011, http.MethodPut, "/api/channel/routing-pool", string(body), SaveModelRoutingPool)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					loaded, err := model.GetModelRoutingPoolForServiceType(pool.RoutingGroup, pool.Model, pool.ServiceTypeID)
					require.NoError(t, err)
					*pool = *loaded
				}
				fallback := legacy
				fallback.Model, fallback.Revision = "unconfigured", ""
				require.NoError(t, model.SaveModelRoutingPool(&fallback, ""))
				for _, id := range []int{8011, 8012} {
					binding := model.UserRoutingServiceType{UserID: id, ServiceTypeID: types[0].ID}
					require.NoError(t, model.SaveUserRoutingServiceType(&binding, "", false))
					param := routingServiceTypeParam(id, "service-model")
					channel, _, routeErr := service.SelectChannelForRequest(param.Ctx, param.ModelName, param)
					require.Nil(t, routeErr)
					require.NotNil(t, channel)
					assert.Equal(t, 9812, channel.Id)
					assert.Error(t, service.ValidateSelectedChannelGroupPolicy(param.Ctx, 9811, param.ModelName))
				}
				fallbackPool, err := routingServiceTypeParam(8012, "unconfigured").ModelRoutingPool("default")
				require.NoError(t, err)
				require.NotNil(t, fallbackPool)
				assert.Zero(t, fallbackPool.ServiceTypeID, "missing service entries inherit default pools")
				strategy := model.UserRoutingStrategy{UserId: 8012, Model: "service-model", RoutingGroup: "default", Mode: model.RoutingModeSmart}
				require.NoError(t, model.SaveUserRoutingStrategy(&strategy, ""))
				paramWithStrategy := routingServiceTypeParam(8012, "service-model")
				effectiveStrategy, err := paramWithStrategy.RoutingStrategy("default")
				require.NoError(t, err)
				assert.Equal(t, model.RoutingModeSmart, effectiveStrategy.Mode)
				unchangedPool, err := paramWithStrategy.ModelRoutingPool("default")
				require.NoError(t, err)
				assert.Equal(t, shared.Channels, unchangedPool.Channels, "strategy changes do not change service membership")
				strategy.Mode = "inherit"
				require.NoError(t, model.SaveUserRoutingStrategy(&strategy, strategy.Revision))
				snapshot := routingServiceTypeParam(8011, "service-model")
				oldPool, err := snapshot.ModelRoutingPool("default")
				require.NoError(t, err)
				shared.Channels = []model.ModelRoutingPoolChannel{{ChannelID: 9811, Weight: 100, Enabled: true}}
				require.NoError(t, model.SaveModelRoutingPool(&shared, shared.Revision))
				assert.Equal(t, 9812, oldPool.Channels[0].ChannelID, "in-flight requests retain their snapshot")
				for _, id := range []int{8011, 8012} {
					pool, err := routingServiceTypeParam(id, "service-model").ModelRoutingPool("default")
					require.NoError(t, err)
					assert.Equal(t, types[0].ID, pool.ServiceTypeID)
					assert.Equal(t, 9811, pool.Channels[0].ChannelID)
				}
				exceptions := []model.UserRoutingServiceType{
					{UserID: 8011, RoutingGroup: "default", ServiceTypeID: types[1].ID},
					{UserID: 8011, Model: "service-model", ServiceTypeID: types[0].ID},
					{UserID: 8011, Model: "service-model", RoutingGroup: "default", ServiceTypeID: types[1].ID},
				}
				for i := range exceptions {
					require.NoError(t, model.SaveUserRoutingServiceType(&exceptions[i], "", false))
					pool, err := routingServiceTypeParam(8011, "service-model").ModelRoutingPool("default")
					require.NoError(t, err)
					assert.Equal(t, exceptions[i].ServiceTypeID, pool.ServiceTypeID)
				}
				exact := &exceptions[2]
				assert.ErrorIs(t, model.SaveUserRoutingServiceType(exact, "stale", false), model.ErrRoutingPoolConflict)
				exact.ServiceTypeID = 0
				require.NoError(t, model.SaveUserRoutingServiceType(exact, exact.Revision, false))
				pool, err := routingServiceTypeParam(8011, "service-model").ModelRoutingPool("default")
				require.NoError(t, err)
				assert.Zero(t, pool.ServiceTypeID, "explicit default bypasses the account service type")
				require.NoError(t, model.SaveUserRoutingServiceType(exact, exact.Revision, true))
				pool, err = routingServiceTypeParam(8011, "service-model").ModelRoutingPool("default")
				require.NoError(t, err)
				assert.Equal(t, types[0].ID, pool.ServiceTypeID)
				rows, err := model.ListUserGroupChannelRoutingOverrides(8011, "service-model")
				require.NoError(t, err)
				members := make([]model.UserChannelRoutingOverride, 0, len(rows))
				for _, row := range rows {
					members = append(members, row.UserChannelRoutingOverride)
				}
				revision, err := model.UserChannelRoutingRevision(members)
				require.NoError(t, err)
				require.NoError(t, model.PatchUserChannelRoutingForGroup(8011, "service-model", "default", revision, []model.UserChannelRoutingPatch{{ChannelId: 9813, Enabled: common.GetPointer(true)}}))
				param := routingServiceTypeParam(8011, "service-model")
				channel, _, routeErr := service.SelectChannelForRequest(param.Ctx, param.ModelName, param)
				require.Nil(t, routeErr)
				assert.Equal(t, 9813, channel.Id, "dedicated pools retain precedence")
				for _, enabled := range []bool{false, true} {
					_, err := model.SetChannelsAndRoutingPoolsEnabled(context.Background(), []int{9811}, common.RoleRootUser, enabled)
					require.NoError(t, err)
					loaded, err := model.GetModelRoutingPoolForServiceType("default", "service-model", shared.ServiceTypeID)
					require.NoError(t, err)
					require.Len(t, loaded.Channels, 1)
					assert.Equal(t, enabled, loaded.Channels[0].Enabled, "global status changes include service pools")
					shared = *loaded
				}
				impact, err := model.LoadChannelStatusRoutingPools(context.Background(), []int{9811}, common.RoleRootUser)
				require.NoError(t, err)
				assert.Contains(t, impact, model.ChannelStatusRoutingPool{Source: "service_type", ServiceTypeID: types[0].ID, ServiceTypeName: "Economy", ChannelID: 9811, Model: "service-model", RoutingGroup: "default", Priority: common.GetPointer(int64(0)), Enabled: true})
				shared.Channels = []model.ModelRoutingPoolChannel{}
				require.NoError(t, model.SaveModelRoutingPool(&shared, shared.Revision))
				param = routingServiceTypeParam(8012, "service-model")
				channel, _, routeErr = service.SelectChannelForRequest(param.Ctx, param.ModelName, param)
				assert.Nil(t, channel, "explicit empty service pool never falls through to defaults")
				require.NotNil(t, routeErr)
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.RoutingServiceType{}, &model.RoutingServiceTypePool{}, &model.UserRoutingServiceType{}))
				}
				stored, err := model.GetModelRoutingPoolForServiceType("default", "service-model", shared.ServiceTypeID)
				require.NoError(t, err)
				assert.Equal(t, shared.Revision, stored.Revision)
				assert.Empty(t, stored.Channels)
				currentTypes, err := model.ListRoutingServiceTypes()
				require.NoError(t, err)
				for _, row := range currentTypes {
					assert.ErrorIs(t, model.DeleteRoutingServiceType(row.ID, row.Revision), model.ErrRoutingServiceTypeInUse)
				}
				response := userRoutingRequestAtURL(t, 8012, http.MethodGet, fmt.Sprintf("/api/channel/routing-pools?service_type_id=%d", shared.ServiceTypeID), "", ListModelRoutingPools)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), fmt.Sprintf(`"service_type_id":%d`, shared.ServiceTypeID))
				assert.NotContains(t, response.Body.String(), `"config_json"`)
				response = userRoutingRequestAtURL(t, 8012, http.MethodGet, "/api/user/8012/routing-service-types", "", GetUserRoutingServiceTypes)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var assignments struct {
					Data struct {
						Bindings []model.UserRoutingServiceType `json:"bindings"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &assignments))
				require.Len(t, assignments.Data.Bindings, 1)
				assignment := assignments.Data.Bindings[0]
				assignment.ServiceTypeID = quality.ServiceTypeID
				body, err := common.Marshal(assignment)
				require.NoError(t, err)
				response = userRoutingRequestAtURL(t, 8012, http.MethodPut, "/api/user/8012/routing-service-types", string(body), SaveUserRoutingServiceType)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				pool, err = routingServiceTypeParam(8012, "service-model").ModelRoutingPool("default")
				require.NoError(t, err)
				assert.Equal(t, quality.ServiceTypeID, pool.ServiceTypeID)
				response = userRoutingRequestAtURL(t, 8012, http.MethodGet, "/api/user/8012/channel-routing-overrides?model=service-model&routing_group=default&view=pool", "", GetUserChannelRouting)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var preview struct {
					Data userChannelRoutingResponse `json:"data"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &preview))
				assert.Equal(t, "service_type", preview.Data.Source)
				assert.Equal(t, "Quality", preview.Data.ServiceTypeName)
				for _, id := range []int{8011, 8012} {
					bindings, err := model.ListUserRoutingServiceTypes(id)
					require.NoError(t, err)
					for _, binding := range bindings {
						require.NoError(t, model.SaveUserRoutingServiceType(&binding, binding.Revision, true))
					}
				}
				currentTypes, err = model.ListRoutingServiceTypes()
				require.NoError(t, err)
				for _, row := range currentTypes {
					if row.ID != shared.ServiceTypeID {
						continue
					}
					row.Name = "Economy renamed"
					body, err := common.Marshal(row)
					require.NoError(t, err)
					response = userRoutingRequestAtURL(t, 8012, http.MethodPut, "/api/channel/routing-service-types", string(body), SaveRoutingServiceType)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					var renamed struct {
						Data model.RoutingServiceType `json:"data"`
					}
					require.NoError(t, common.Unmarshal(response.Body.Bytes(), &renamed))
					body, err = common.Marshal(renamed.Data)
					require.NoError(t, err)
					response = userRoutingRequestAtURL(t, 8012, http.MethodDelete, "/api/channel/routing-service-types", string(body), DeleteRoutingServiceType)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				}
				deleted, err := model.GetModelRoutingPoolForServiceType("default", "service-model", shared.ServiceTypeID)
				require.NoError(t, err)
				assert.Nil(t, deleted, "deleting an unassigned type also removes its pools")
			})
		}
	}
}
