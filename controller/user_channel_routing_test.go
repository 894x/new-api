package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelDisableImpactDatabaseMatrix(t *testing.T) {
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
			t.Cleanup(func() {
				common.MemoryCacheEnabled = previousCache
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
			})
			common.MemoryCacheEnabled = false
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.Token{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			channels := []model.Channel{
				{Id: 9201, Name: "Target", Key: "channel-secret-must-not-leak", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5", Group: "default", Priority: common.GetPointer(int64(100)), Tag: common.GetPointer("impact-batch")},
				{Id: 9202, Name: "Alternative", Key: "alternative-secret", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5", Group: "default", Priority: common.GetPointer(int64(10)), Tag: common.GetPointer("impact-batch")},
			}
			for i := range channels {
				require.NoError(t, db.Create(&channels[i]).Error)
				require.NoError(t, channels[i].AddAbilities(nil))
			}
			require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{{ChannelId: 9201, Model: "gpt-5", Priority: common.GetPointer(int64(20))}}))
			users := []model.User{
				{Username: "inherited", Group: "default", Status: common.UserStatusEnabled, AffCode: "impact0"},
				{Username: "scoped-zero", Group: "default", Status: common.UserStatusEnabled, AffCode: "impact1"},
				{Username: "denied", Group: "denied", Status: common.UserStatusEnabled, AffCode: "impact2"},
				{Username: "disabled", Group: "default", Status: common.UserStatusDisabled, AffCode: "impact3"},
				{Username: "expired-key", Group: "default", Status: common.UserStatusEnabled, AffCode: "impact4"},
				{Username: "model-limited", Group: "default", Status: common.UserStatusEnabled, AffCode: "impact5"},
				{Username: "auto-key", Group: "default", Status: common.UserStatusEnabled, AffCode: "impact6"},
				{Username: "route-disabled", Group: "default", Status: common.UserStatusEnabled, AffCode: "impact7"},
				{Username: "legacy-inherited", Group: "default", Status: common.UserStatusEnabled, AffCode: "impact8"},
			}
			require.NoError(t, db.Create(&users).Error)
			for i, user := range users {
				token := model.Token{UserId: user.Id, Key: fmt.Sprintf("impact-token-%d", i), Status: common.TokenStatusEnabled, ExpiredTime: -1, Group: "default"}
				switch i {
				case 0:
					token.Group = ""
				case 4:
					token.ExpiredTime = time.Now().Unix() - 100
				case 5:
					token.ModelLimitsEnabled, token.ModelLimits = true, "other-model"
				case 6:
					token.Group, token.AutoGroups = "auto", `["default"]`
				}
				require.NoError(t, db.Create(&token).Error)
			}
			// A second key must not duplicate the same customer/model/group row.
			require.NoError(t, db.Create(&model.Token{UserId: users[0].Id, Key: "impact-second-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, Group: "default"}).Error)
			require.NoError(t, db.Create(&[]model.UserChannelRoutingOverride{
				{UserId: users[1].Id, Model: "gpt-5", ChannelId: 9201, Priority: common.GetPointer(int64(30))},
				{UserId: users[8].Id, Model: "gpt-5", ChannelId: 9201, Priority: common.GetPointer(int64(40)), Disabled: true},
			}).Error)
			require.NoError(t, db.Create(&[]model.UserGroupChannelRoutingOverride{
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[1].Id, Model: "gpt-5", ChannelId: 9201, Priority: common.GetPointer(int64(0))}, RoutingGroup: "default"},
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[7].Id, Model: "gpt-5", ChannelId: 9201, Disabled: true}, RoutingGroup: "default"},
				{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[8].Id, Model: "gpt-5", ChannelId: 9201, Priority: common.GetPointer(int64(40))}, RoutingGroup: "default"},
			}).Error)
			for _, cached := range []bool{false, true} {
				t.Run(fmt.Sprintf("cache_%t", cached), func(t *testing.T) {
					common.MemoryCacheEnabled = cached
					if cached {
						model.InitChannelCache()
					}
					response := userRoutingRequestAtURL(t, 1, http.MethodPost, "/api/channel/disable-impact", `{"ids":[9201]}`, GetChannelDisableImpact)
					require.Equal(t, http.StatusOK, response.Code)
					var body struct {
						Success bool                         `json:"success"`
						Data    service.ChannelDisableImpact `json:"data"`
					}
					require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
					require.True(t, body.Success, response.Body.String())
					assert.Equal(t, 5, body.Data.CustomerCount)
					require.Len(t, body.Data.Customers, 5)
					byName := make(map[string]service.ChannelDisableImpactCustomer)
					for _, row := range body.Data.Customers {
						byName[row.Username] = row
						assert.Equal(t, "gpt-5", row.Model)
						assert.Equal(t, "default", row.KeyGroup)
						wantAlternatives := 1
						if row.Username == "scoped-zero" || row.Username == "legacy-inherited" {
							wantAlternatives = 0
						}
						assert.Equal(t, wantAlternatives, row.AlternativeChannels)
					}
					assert.EqualValues(t, 20, byName["inherited"].EffectivePriority)
					assert.True(t, byName["inherited"].Primary)
					assert.EqualValues(t, 0, byName["scoped-zero"].EffectivePriority)
					assert.True(t, byName["scoped-zero"].Primary)
					assert.EqualValues(t, 40, byName["legacy-inherited"].EffectivePriority)
					assert.True(t, byName["auto-key"].Primary)
					assert.NotContains(t, response.Body.String(), "secret")
					assert.NotContains(t, response.Body.String(), "impact-token")
					batch := userRoutingRequestAtURL(t, 1, http.MethodPost, "/api/channel/disable-impact", `{"tag":"impact-batch"}`, GetChannelDisableImpact)
					require.NoError(t, common.Unmarshal(batch.Body.Bytes(), &body))
					require.True(t, body.Success, batch.Body.String())
					require.Len(t, body.Data.Channels, 2)
					for _, row := range body.Data.Customers {
						assert.Zero(t, row.AlternativeChannels, "channels disabled in the same batch are not alternatives")
					}
				})
			}
			pool := model.ModelRoutingPool{RoutingGroup: "default", Model: "gpt-5", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9202, Priority: 100, Weight: 10, Enabled: true}}}
			require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
			impact, err := service.GetChannelDisableImpact(context.Background(), []int{9201}, nil, common.RoleRootUser)
			require.NoError(t, err)
			assert.Equal(t, 2, impact.CustomerCount, "only explicit user overrides still use the channel excluded by the default pool")
			for _, row := range impact.Customers {
				assert.Contains(t, []string{"scoped-zero", "legacy-inherited"}, row.Username)
			}
			require.NoError(t, model.DeleteModelRoutingPool(pool.RoutingGroup, pool.Model, pool.Revision))
			for _, invalid := range []string{`{}`, `{"ids":[-1]}`, `{"ids":[9201],"tag":"impact-batch"}`, `{"tag":" "}`} {
				response := userRoutingRequestAtURL(t, 1, http.MethodPost, "/api/channel/disable-impact", invalid, GetChannelDisableImpact)
				assert.Equal(t, http.StatusBadRequest, response.Code, invalid)
			}
			missing := userRoutingRequestAtURL(t, 1, http.MethodPost, "/api/channel/disable-impact", `{"ids":[999999]}`, GetChannelDisableImpact)
			var empty struct {
				Success bool                         `json:"success"`
				Data    service.ChannelDisableImpact `json:"data"`
			}
			require.NoError(t, common.Unmarshal(missing.Body.Bytes(), &empty))
			assert.True(t, empty.Success)
			assert.Empty(t, empty.Data.Channels)
			assert.Empty(t, empty.Data.Customers)
			var unchanged model.Channel
			require.NoError(t, db.First(&unchanged, 9201).Error)
			assert.Equal(t, common.ChannelStatusEnabled, unchanged.Status, "preflight never disables channels")
			privileged := model.User{Username: "root-routing", Group: "default", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "impactroot"}
			require.NoError(t, db.Create(&privileged).Error)
			require.NoError(t, db.Create(&model.Token{UserId: privileged.Id, Key: "impact-root-key", Group: "default", Status: common.TokenStatusEnabled, ExpiredTime: -1}).Error)
			visible, err := service.GetChannelDisableImpact(context.Background(), []int{9201}, nil, common.RoleAdminUser)
			require.NoError(t, err)
			for _, row := range visible.Customers {
				assert.NotEqual(t, privileged.Id, row.UserID, "operators cannot inspect routing of equal or higher roles")
			}
		})
	}
}

func TestChannelDisableImpactRequiresCustomerRoutingPermission(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 0)
	c.Set("role", common.RoleCommonUser)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/disable-impact", strings.NewReader(`{"ids":[9201]}`))
	GetChannelDisableImpact(c)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "customers")
}

func userRoutingRequest(t *testing.T, userID int, method, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	return userRoutingRequestAtURL(t, userID, method, "/api/user/1/channel-routing-overrides?model=gpt-5&routing_group=default", body, handler)
}

func userRoutingRequestAtURL(t *testing.T, userID int, method, requestURL, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	c.Set("username", "routing-admin")
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(userID)}}
	c.Request = httptest.NewRequest(method, requestURL, strings.NewReader(body))
	handler(c)
	return recorder
}

func readUserRouting(t *testing.T, userID int) userChannelRoutingResponse {
	t.Helper()
	recorder := userRoutingRequest(t, userID, http.MethodGet, "", GetUserChannelRouting)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool                       `json:"success"`
		Data    userChannelRoutingResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	return response.Data
}

func TestUserChannelRoutingDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
				t.Skip("isolated database DSN not configured")
			}
			t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
			db := setupManageUserTestDB(t)
			previousCache := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = false
			t.Cleanup(func() {
				common.MemoryCacheEnabled = previousCache
			})
			// These pre-existing schemas and records model an upgrade from the
			// current release; no existing schema changes are needed by this feature.
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}))
			users := []model.User{{Username: "routing-a", Group: "default", AffCode: "routinga"}, {Username: "routing-b", Group: "default", AffCode: "routingb"}}
			require.NoError(t, db.Create(&users).Error)
			for _, channel := range []*model.Channel{
				{Id: 9101, Name: "Global first", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5", Group: "default,pool-a", Priority: common.GetPointer(int64(100))},
				{Id: 9102, Name: "User first", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5", Group: "default,pool-b", Priority: common.GetPointer(int64(10))},
				{Id: 9103, Name: "Other pool", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5", Group: "private", Priority: common.GetPointer(int64(999))},
			} {
				require.NoError(t, db.Create(channel).Error)
				require.NoError(t, channel.AddAbilities(nil))
			}
			require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{{ChannelId: 9102, Model: "gpt-5", Priority: common.GetPointer(int64(20))}}))
			require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			initial := readUserRouting(t, users[0].Id)
			require.Len(t, initial.Channels, 2)
			assert.EqualValues(t, 20, initial.Channels[1].InheritedPriority)
			payload := fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"overrides":[{"channel_id":9101,"priority_override":0,"enabled":true},{"channel_id":9102,"priority_override":200,"enabled":true}]}`, initial.Revision)
			saved := userRoutingRequest(t, users[0].Id, http.MethodPatch, payload, PatchUserChannelRouting)
			require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
			assert.Equal(t, http.StatusConflict, userRoutingRequest(t, users[0].Id, http.MethodPatch, payload, PatchUserChannelRouting).Code)
			// Restart/migration preserves overrides, global data and composite uniqueness.
			require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			configured := readUserRouting(t, users[0].Id)
			require.Len(t, configured.Channels, 2)
			assert.Equal(t, 9102, configured.Channels[0].ChannelID)
			assert.EqualValues(t, 0, *configured.Channels[1].PriorityOverride)
			assert.Error(t, db.Create(&model.UserGroupChannelRoutingOverride{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: users[0].Id, Model: "gpt-5", ChannelId: 9102, Priority: common.GetPointer(int64(1))}, RoutingGroup: "default"}).Error)
			for _, cache := range []bool{false, true} {
				t.Run(fmt.Sprintf("cache_%t", cache), func(t *testing.T) {
					common.MemoryCacheEnabled = cache
					if cache {
						model.InitChannelCache()
					}
					for _, test := range []struct{ user, retry, channel int }{{users[0].Id, 0, 9102}, {users[0].Id, 1, 9101}, {users[1].Id, 0, 9101}} {
						ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
						common.SetContextKey(ctx, constant.ContextKeyUserId, test.user)
						common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
						channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "gpt-5", Retry: &test.retry})
						require.NoError(t, err)
						require.NotNil(t, channel)
						assert.Equal(t, test.channel, channel.Id)
					}
				})
			}
			common.MemoryCacheEnabled = false
			// Switch off/on retains the priority and never changes the shared channel.
			for _, enabled := range []bool{false, true} {
				current := readUserRouting(t, users[0].Id)
				body := fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"overrides":[{"channel_id":9101,"priority_override":0,"enabled":true},{"channel_id":9102,"priority_override":200,"enabled":%t}]}`, current.Revision, enabled)
				require.Equal(t, http.StatusOK, userRoutingRequest(t, users[0].Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
				next := readUserRouting(t, users[0].Id)
				assert.Equal(t, enabled, next.Channels[0].Enabled)
				assert.EqualValues(t, 200, *next.Channels[0].PriorityOverride)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				common.SetContextKey(ctx, constant.ContextKeyUserId, users[0].Id)
				common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
				channel, _, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "gpt-5"})
				require.NoError(t, err)
				require.NotNil(t, channel)
				want := 9101
				if enabled {
					want = 9102
				}
				assert.Equal(t, want, channel.Id)
			}
			current := readUserRouting(t, users[0].Id)
			// Mixed valid/invalid batch rolls back the valid part too.
			err := model.PatchUserChannelRoutingForGroup(users[0].Id, "gpt-5", "default", current.Revision, []model.UserChannelRoutingPatch{
				{ChannelId: 9102, Enabled: common.GetPointer(true)},
				{ChannelId: 999999, Priority: common.GetPointer(int64(1)), Enabled: common.GetPointer(true)},
			})
			require.Error(t, err)
			assert.Equal(t, current.Revision, readUserRouting(t, users[0].Id).Revision)
			for _, invalid := range []string{
				`{"channel_id":9102,"priority_override":-1,"enabled":true}`,
				`{"channel_id":9102,"priority_override":2147483648,"enabled":true}`,
				`{"channel_id":9102,"priority_override":1.5,"enabled":true}`,
				`{"channel_id":9102,"priority_override":1}`,
			} {
				body := fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"overrides":[%s]}`, current.Revision, invalid)
				assert.Equal(t, http.StatusBadRequest, userRoutingRequest(t, users[0].Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
			}
			// Null restores model-level inheritance; the second user's row is separate.
			empty, err := model.UserChannelRoutingRevision([]model.UserChannelRoutingOverride{})
			require.NoError(t, err)
			require.NoError(t, model.PatchUserChannelRoutingForGroup(users[1].Id, "gpt-5", "default", empty, []model.UserChannelRoutingPatch{{ChannelId: 9102, Enabled: common.GetPointer(false)}}))
			require.NoError(t, model.PatchUserChannelRoutingForGroup(users[0].Id, "gpt-5", "default", current.Revision, []model.UserChannelRoutingPatch{
				{ChannelId: 9101, Enabled: common.GetPointer(true)}, {ChannelId: 9102, Enabled: common.GetPointer(true)},
			}))
			assert.NotEqual(t, initial.Revision, readUserRouting(t, users[0].Id).Revision)
			otherRows, err := model.ListUserGroupChannelRoutingOverrides(users[1].Id, "gpt-5")
			require.NoError(t, err)
			require.Len(t, otherRows, 1)
			assert.True(t, otherRows[0].Disabled)
			var channel model.Channel
			require.NoError(t, db.First(&channel, 9102).Error)
			assert.EqualValues(t, 10, *channel.Priority)
			assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
		})
	}
}

func TestUserChannelRoutingGroupIsolationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade_%t", dialect, upgrade), func(t *testing.T) {
				if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
					t.Skip("isolated database DSN not configured")
				}
				t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
				db := setupManageUserTestDB(t)
				previousUsable, previousRatios := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
				previousAutoLimit, previousRetry := setting.GetMaxTokenAutoGroups(), common.RetryTimes
				t.Cleanup(func() {
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
					require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
					require.NoError(t, setting.UpdateMaxTokenAutoGroups(strconv.Itoa(previousAutoLimit)))
					common.RetryTimes = previousRetry
				})
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"pool-a":"A","pool-b":"B"}`))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"pool-a":1,"pool-b":1}`))
				require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
				common.RetryTimes = 0
				require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.UserChannelRoutingOverride{}))
				user := model.User{Username: "group-routing", Group: "default", AffCode: "group-routing"}
				require.NoError(t, db.Create(&user).Error)
				for _, channel := range []*model.Channel{
					{Id: 9201, Name: "Shared", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5,gpt-6", Group: "pool-a,pool-b", Priority: common.GetPointer(int64(100))},
					{Id: 9202, Name: "Alternative", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5,gpt-6", Group: "pool-a,pool-b", Priority: common.GetPointer(int64(50))},
					{Id: 9203, Name: "A only", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5", Group: "pool-a", Priority: common.GetPointer(int64(1))},
				} {
					require.NoError(t, db.Create(channel).Error)
					require.NoError(t, channel.AddAbilities(nil))
				}
				if upgrade {
					require.NoError(t, db.Create(&model.UserChannelRoutingOverride{UserId: user.Id, Model: "gpt-5", ChannelId: 9201, Priority: common.GetPointer(int64(25))}).Error)
				}
				// The old primary key, rows and indexes survive an additive upgrade.
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
				}
				read := func(group string) userChannelRoutingResponse {
					requestURL := "/api/user/1/channel-routing-overrides?model=gpt-5&routing_group=" + url.QueryEscape(group)
					recorder := userRoutingRequestAtURL(t, user.Id, http.MethodGet, requestURL, "", GetUserChannelRouting)
					require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
					var response struct {
						Data userChannelRoutingResponse `json:"data"`
					}
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
					return response.Data
				}
				patch := func(group, revision string, priority int64, enabled bool) *httptest.ResponseRecorder {
					body := fmt.Sprintf(`{"model":"gpt-5","routing_group":%q,"revision":%q,"overrides":[{"channel_id":9201,"priority_override":%d,"enabled":%t}]}`, group, revision, priority, enabled)
					return userRoutingRequest(t, user.Id, http.MethodPatch, body, PatchUserChannelRouting)
				}
				a, b := read("pool-a"), read("pool-b")
				require.Len(t, a.Channels, 3)
				require.Len(t, b.Channels, 2)
				require.Equal(t, http.StatusOK, patch("pool-a", a.Revision, 200, true).Code)
				// Editing A must not invalidate B's revision or change B's default.
				assert.Equal(t, b.Revision, read("pool-b").Revision)
				require.Equal(t, http.StatusOK, patch("pool-b", b.Revision, 0, true).Code)
				assert.Equal(t, http.StatusConflict, patch("pool-a", a.Revision, 999, true).Code)
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.UserGroupChannelRoutingOverride{}))
				}
				assert.EqualValues(t, 200, *read("pool-a").Channels[0].PriorityOverride)
				assert.EqualValues(t, 0, *read("pool-b").Channels[1].PriorityOverride)
				rows, err := model.ListUserGroupChannelRoutingOverrides(user.Id, "gpt-5")
				require.NoError(t, err)
				require.Len(t, rows, 2)
				assert.Error(t, db.Create(&rows[0]).Error)
				previousCache := common.MemoryCacheEnabled
				t.Cleanup(func() { common.MemoryCacheEnabled = previousCache })
				for _, cache := range []bool{false, true} {
					common.MemoryCacheEnabled = cache
					if cache {
						model.InitChannelCache()
					}
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
					param := &service.RetryParam{Ctx: ctx, TokenGroup: "pool-a", ModelName: "gpt-5"}
					for _, test := range []struct {
						group, model string
						channel      int
					}{
						{"pool-a", "gpt-5", 9201}, {"pool-b", "gpt-5", 9201}, {"pool-a", "gpt-5", 9201},
					} {
						param.TokenGroup = test.group
						channel, group, err := service.CacheGetRandomSatisfiedChannel(param)
						require.NoError(t, err)
						require.NotNil(t, channel)
						assert.Equal(t, test.group, group)
						assert.Equal(t, test.channel, channel.Id)
					}
					// A different model and user retain the shared channel defaults.
					for _, other := range []*service.RetryParam{
						{Ctx: ctx, TokenGroup: "pool-b", ModelName: "gpt-6"},
						{Ctx: func() *gin.Context { c, _ := gin.CreateTestContext(httptest.NewRecorder()); return c }(), TokenGroup: "pool-b", ModelName: "gpt-5"},
					} {
						channel, _, err := service.CacheGetRandomSatisfiedChannel(other)
						require.NoError(t, err)
						require.NotNil(t, channel)
						assert.Equal(t, 9201, channel.Id)
					}
					common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"pool-a", "pool-b"})
					common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
					auto := &service.RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "gpt-5"}
					for _, expected := range []struct {
						group   string
						channel int
					}{{"pool-a", 9201}, {"pool-b", 9201}} {
						channel, group, err := service.CacheGetRandomSatisfiedChannel(auto)
						require.NoError(t, err)
						require.NotNil(t, channel)
						assert.Equal(t, expected.group, group)
						assert.Equal(t, expected.channel, channel.Id)
						auto.IncreaseRetry()
					}
				}
				a = read("pool-a")
				require.Equal(t, http.StatusOK, patch("pool-a", a.Revision, 200, false).Code)
				assert.True(t, read("pool-b").Channels[1].Enabled)
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "pool-b")
				require.NoError(t, service.ValidateSelectedChannelGroupPolicy(ctx, 9201, "gpt-5"))
				common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "pool-a")
				assert.ErrorIs(t, service.ValidateSelectedChannelGroupPolicy(ctx, 9201, "gpt-5"), service.ErrGroupModelChannelDenied)
				service.GetChannelConstraints(ctx).AddPin(dto.ChannelPin{ChannelId: 9201, Source: dto.PinSourceOriginTask})
				pinned, _, pinErr := service.SelectChannelForRequest(ctx, "gpt-5", &service.RetryParam{Ctx: ctx, TokenGroup: "pool-a", ModelName: "gpt-5"})
				assert.Nil(t, pinned)
				require.NotNil(t, pinErr)
				assert.Equal(t, "user_channel_disabled", pinErr.Message)
				common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "pool-b")
				pinned, _, pinErr = service.SelectChannelForRequest(ctx, "gpt-5", &service.RetryParam{Ctx: ctx, TokenGroup: "pool-b", ModelName: "gpt-5"})
				require.Nil(t, pinErr)
				require.NotNil(t, pinned)
				assert.Equal(t, 9201, pinned.Id)
				// Reject a channel outside B without changing B's configured values.
				b = read("pool-b")
				body := fmt.Sprintf(`{"model":"gpt-5","routing_group":"pool-b","revision":%q,"overrides":[{"channel_id":9201,"priority_override":999,"enabled":true},{"channel_id":9203,"priority_override":1,"enabled":true}]}`, b.Revision)
				assert.Equal(t, http.StatusBadRequest, userRoutingRequest(t, user.Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
				assert.Equal(t, b.Revision, read("pool-b").Revision)
				a = read("pool-a")
				body = fmt.Sprintf(`{"model":"gpt-5","routing_group":"pool-a","revision":%q,"overrides":[{"channel_id":9201,"priority_override":null,"enabled":true}]}`, a.Revision)
				require.Equal(t, http.StatusOK, userRoutingRequest(t, user.Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
				assert.Equal(t, b.Revision, read("pool-b").Revision)
				for _, row := range read("pool-a").Channels {
					if row.ChannelID == 9201 {
						assert.Nil(t, row.PriorityOverride)
						assert.True(t, row.Enabled)
						expected := int64(100)
						assert.Equal(t, expected, row.EffectivePriority)
					}
				}
				var channel model.Channel
				require.NoError(t, db.First(&channel, 9201).Error)
				assert.EqualValues(t, 100, *channel.Priority)
				legacy, err := model.ListUserChannelRoutingOverrides(user.Id, "gpt-5")
				require.NoError(t, err)
				if upgrade {
					require.Len(t, legacy, 1)
					assert.EqualValues(t, 25, *legacy[0].Priority)
				} else {
					assert.Empty(t, legacy)
				}
			})
		}
	}
}

func TestUserChannelRoutingCatalogSeparatesPublicKeyGroupsFromResourcePools(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
	previousUsable, previousRatios := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
	previousSpecial := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Clear()
		ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.AddAll(previousSpecial)
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"standard":"Standard","vip":"VIP","removed":"Removed","stale":"Stale","auto":"Auto"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"standard":1,"vip":1,"removed":1,"resource-pool":1,"customer":1}`))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Clear()
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.AddAll(map[string]map[string]string{"customer": {"-:removed": "", "+:resource-pool": "Internal"}})
	user := model.User{Username: "key-group-catalog", Group: "customer", AffCode: "key-group-catalog"}
	require.NoError(t, db.Create(&user).Error)
	channel := model.Channel{Id: 9301, Name: "Shared resource", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-5", Group: "standard,vip,resource-pool", Priority: common.GetPointer(int64(100))}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, db.Create(&[]model.UserGroupChannelRoutingOverride{
		{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: user.Id, Model: "gpt-5", ChannelId: channel.Id, Priority: common.GetPointer(int64(200))}, RoutingGroup: "vip"},
		{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: user.Id, Model: "gpt-5", ChannelId: channel.Id, Priority: common.GetPointer(int64(100))}, RoutingGroup: "standard"},
		{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: user.Id, Model: "internal-model", ChannelId: channel.Id, Priority: common.GetPointer(int64(300))}, RoutingGroup: "resource-pool"},
	}).Error)
	recorder := userRoutingRequestAtURL(t, user.Id, http.MethodGet, "/api/user/1/channel-routing-overrides", "", GetUserChannelRouting)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			KeyGroups           map[string]string   `json:"key_groups"`
			ConfiguredModels    []string            `json:"configured_models"`
			ConfiguredKeyGroups map[string][]string `json:"configured_key_groups"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, map[string]string{"standard": "Standard", "vip": "VIP"}, response.Data.KeyGroups)
	assert.Equal(t, []string{"gpt-5"}, response.Data.ConfiguredModels)
	assert.Equal(t, map[string][]string{"gpt-5": {"standard", "vip"}}, response.Data.ConfiguredKeyGroups)
}

func TestUserRoutingPoolEditorDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, upgrade := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upgrade_%t", dialect, upgrade), func(t *testing.T) {
				if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
					t.Skip("isolated database DSN not configured")
				}
				t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
				db := setupManageUserTestDB(t)
				previousCache, previousGroups, previousRatios := common.MemoryCacheEnabled, setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString()
				t.Cleanup(func() {
					common.MemoryCacheEnabled = previousCache
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
					require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
				})
				common.MemoryCacheEnabled = false
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
				require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}))
				user := model.User{Username: "pool-editor", Group: "default", AffCode: "pooleditor"}
				require.NoError(t, db.Create(&user).Error)
				channel := model.Channel{Id: 9801, Type: 1, Name: "Public", Models: "gpt-5", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(10)), Weight: common.GetPointer(uint(20))}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(nil))
				if upgrade {
					type previousOverride struct {
						UserId    int    `gorm:"primaryKey;autoIncrement:false"`
						Model     string `gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
						ChannelId int    `gorm:"primaryKey;autoIncrement:false"`
						Priority  *int64 `gorm:"bigint"`
						Disabled  bool
					}
					type previousScopedOverride struct {
						Override     previousOverride `gorm:"embedded"`
						RoutingGroup string           `gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
					}
					require.NoError(t, db.Table("user_channel_routing_overrides").AutoMigrate(&previousOverride{}))
					require.NoError(t, db.Table("user_group_channel_routing_overrides").AutoMigrate(&previousScopedOverride{}))
					require.NoError(t, db.Table("user_channel_routing_overrides").Create(&previousOverride{UserId: user.Id, Model: "gpt-5", ChannelId: 9801, Priority: common.GetPointer(int64(25))}).Error)
					require.NoError(t, db.Table("user_group_channel_routing_overrides").Create(&previousScopedOverride{Override: previousOverride{UserId: user.Id, Model: "gpt-5", ChannelId: 9801, Priority: common.GetPointer(int64(30))}, RoutingGroup: "vip"}).Error)
				}
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
				}
				if upgrade {
					rows, err := model.ListUserChannelRoutingOverrides(user.Id, "gpt-5")
					require.NoError(t, err)
					require.Len(t, rows, 1)
					assert.Nil(t, rows[0].Weight)
					assert.EqualValues(t, 25, *rows[0].Priority)
					scoped, err := model.ListUserGroupChannelRoutingOverrides(user.Id, "gpt-5")
					require.NoError(t, err)
					require.Len(t, scoped, 1)
					assert.Nil(t, scoped[0].Weight)
					assert.EqualValues(t, 30, *scoped[0].Priority)
					revision, err := model.UserChannelRoutingRevision(rows)
					require.NoError(t, err)
					require.NoError(t, model.ResetUserChannelRoutingForGroup(user.Id, "gpt-5", "", revision))
				}
				pool := model.ModelRoutingPool{RoutingGroup: "default", Model: "gpt-5", Mode: model.RoutingModeSmart, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9801, Priority: 100, Weight: 7, Enabled: true}}}
				require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
				read := func(group string) userChannelRoutingResponse {
					response := userRoutingRequestAtURL(t, user.Id, http.MethodGet, "/api/user/1/channel-routing-overrides?view=pool&model=gpt-5&routing_group="+group, "", GetUserChannelRouting)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					var body struct {
						Success bool                       `json:"success"`
						Data    userChannelRoutingResponse `json:"data"`
					}
					require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
					require.True(t, body.Success, response.Body.String())
					return body.Data
				}
				inherited := read("default")
				assert.Equal(t, "default_pool", inherited.Source)
				assert.Equal(t, "smart", inherited.Mode)
				require.Len(t, inherited.PoolChannels, 1)
				assert.EqualValues(t, 100, inherited.PoolChannels[0].Priority)
				assert.EqualValues(t, 7, inherited.PoolChannels[0].Weight)
				payload := fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"overrides":[{"channel_id":9801,"priority_override":0,"weight_override":0,"enabled":true}]}`, inherited.Revision)
				require.Equal(t, http.StatusOK, userRoutingRequest(t, user.Id, http.MethodPatch, payload, PatchUserChannelRouting).Code)
				configured := read("default")
				assert.Equal(t, "user", configured.Source)
				require.Len(t, configured.Channels, 1)
				require.NotNil(t, configured.Channels[0].WeightOverride)
				assert.Zero(t, *configured.Channels[0].WeightOverride)
				for _, cached := range []bool{false, true} {
					common.MemoryCacheEnabled = cached
					if cached {
						model.InitChannelCache()
					}
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
					common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
					param := &service.RetryParam{Ctx: ctx, ModelName: "gpt-5"}
					filters, err := param.SelectionFilters("default")
					require.NoError(t, err)
					candidates, err := model.ListChannelSelectionCandidates("default", "gpt-5", filters)
					require.NoError(t, err)
					require.Len(t, candidates, 1)
					assert.Zero(t, candidates[0].Weight)
					assert.Zero(t, *candidates[0].Priority)
					preview := read("default")
					assert.Zero(t, preview.PoolChannels[0].Weight)
					assert.Zero(t, preview.PoolChannels[0].Priority)
				}
				common.MemoryCacheEnabled = false
				// A complete replacement with an empty weight inherits channel defaults.
				body := fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"overrides":[{"channel_id":9801,"priority_override":5,"enabled":true}]}`, configured.Revision)
				require.Equal(t, http.StatusOK, userRoutingRequest(t, user.Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
				current := read("default")
				assert.Nil(t, current.Channels[0].WeightOverride)
				assert.EqualValues(t, 20, current.Channels[0].EffectiveWeight)
				assert.ErrorIs(t, model.ResetUserChannelRoutingForGroup(user.Id, "gpt-5", "default", configured.Revision), model.ErrUserChannelRoutingConflict)
				body = fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"overrides":[{"channel_id":9801,"priority_override":null,"weight_override":null,"inherit_weight":true,"enabled":true}]}`, current.Revision)
				require.Equal(t, http.StatusOK, userRoutingRequest(t, user.Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
				current = read("default")
				assert.Nil(t, current.Channels[0].WeightOverride)
				assert.Equal(t, "user", current.Source)
				body = fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"reset":true}`, current.Revision)
				require.Equal(t, http.StatusOK, userRoutingRequest(t, user.Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
				assert.False(t, read("default").Configured)
				assert.Equal(t, upgrade, read("vip").Configured, "reset is isolated to one public group")
				for _, weight := range []string{"-1", "1.5", "2147483638"} {
					body = fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"overrides":[{"channel_id":9801,"weight_override":%s,"enabled":true}]}`, inherited.Revision, weight)
					assert.Equal(t, http.StatusBadRequest, userRoutingRequest(t, user.Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
				}
				for range 2 {
					require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
				}
				catalog := userRoutingRequestAtURL(t, user.Id, http.MethodGet, "/api/user/1/channel-routing-overrides?view=pool", "", GetUserChannelRouting)
				assert.Contains(t, catalog.Body.String(), `"model_groups":{"gpt-5":["default","vip"]}`)
			})
		}
	}
}

func TestUserRoutingPoolMembershipDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
				t.Skip("isolated database DSN not configured")
			}
			t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
			db := setupManageUserTestDB(t)
			previousCache := common.MemoryCacheEnabled
			t.Cleanup(func() { common.MemoryCacheEnabled = previousCache })
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			user := model.User{Username: "complete-user-pool", Group: "default", AffCode: "complete-pool"}
			require.NoError(t, db.Create(&user).Error)
			for _, channel := range []model.Channel{
				{Id: 9901, Name: "Default", Type: 1, Models: "gpt-5", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(99)), Weight: common.GetPointer(uint(20))},
				{Id: 9902, Name: "User selected", Type: 1, Models: "gpt-5", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(7)), Weight: common.GetPointer(uint(8))},
				{Id: 9903, Name: "Not selected", Type: 1, Models: "gpt-5", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(999)), Weight: common.GetPointer(uint(30))},
			} {
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(nil))
			}
			pool := model.ModelRoutingPool{RoutingGroup: "default", Model: "gpt-5", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9901, Priority: 100, Weight: 6, Enabled: true}}}
			require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
			read := func() userChannelRoutingResponse {
				response := userRoutingRequestAtURL(t, user.Id, http.MethodGet, "/api/user/1/channel-routing-overrides?view=pool&model=gpt-5&routing_group=default", "", GetUserChannelRouting)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var body struct {
					Success bool                       `json:"success"`
					Data    userChannelRoutingResponse `json:"data"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
				require.True(t, body.Success)
				return body.Data
			}
			write := func(rows string) {
				body := fmt.Sprintf(`{"model":"gpt-5","routing_group":"default","revision":%q,"overrides":%s}`, read().Revision, rows)
				response := userRoutingRequest(t, user.Id, http.MethodPatch, body, PatchUserChannelRouting)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			}
			check := func(ids []int, priority int64, weight uint) {
				for _, cached := range []bool{false, true} {
					common.MemoryCacheEnabled = cached
					if cached {
						model.InitChannelCache()
					}
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
					common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
					common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
					param := &service.RetryParam{Ctx: ctx, ModelName: "gpt-5"}
					filters, err := param.SelectionFilters("default")
					require.NoError(t, err)
					candidates, err := model.ListChannelSelectionCandidates("default", "gpt-5", filters)
					require.NoError(t, err)
					actual := make([]int, 0, len(candidates))
					for _, candidate := range candidates {
						actual = append(actual, candidate.ChannelId)
						assert.Equal(t, priority, *candidate.Priority)
						assert.Equal(t, weight, candidate.Weight)
					}
					assert.ElementsMatch(t, ids, actual, "cache=%t", cached)
					for _, id := range []int{9901, 9902, 9903} {
						allowed, err := param.ModelRoutingPoolAllows("default", id)
						require.NoError(t, err)
						want := false
						for _, selected := range ids {
							want = want || selected == id
						}
						assert.Equal(t, want, allowed, "post-selection policy for channel %d", id)
						assert.Equal(t, want, service.ValidateSelectedChannelGroupPolicy(ctx, id, "gpt-5") == nil)
					}
				}
			}
			check([]int{9901}, 100, 6)
			write(`[{"channel_id":9902,"priority_override":0,"weight_override":0,"enabled":true}]`)
			check([]int{9902}, 0, 0)
			assert.Equal(t, []model.ModelRoutingPoolChannel{{ChannelID: 9902, Priority: 0, Weight: 0, Enabled: true}}, read().PoolChannels)
			write(`[{"channel_id":9902,"priority_override":null,"weight_override":null,"enabled":true}]`)
			check([]int{9902}, 7, 8)
			write(`[{"channel_id":9902,"priority_override":0,"weight_override":0,"enabled":false}]`)
			check([]int{}, 0, 0)
			require.Len(t, read().PoolChannels, 1, "a disabled member must remain editable")
			write(`[]`)
			check([]int{9901}, 100, 6)
			assert.False(t, read().Configured)
			write(`[{"channel_id":9903,"priority_override":1,"weight_override":2,"enabled":true}]`)
			check([]int{9903}, 1, 2)
			write(`[{"channel_id":9902,"priority_override":3,"weight_override":4,"enabled":true}]`)
			check([]int{9902}, 3, 4)
			stored, err := model.ListUserGroupChannelRoutingOverrides(user.Id, "gpt-5")
			require.NoError(t, err)
			require.Len(t, stored, 1, "saving a complete pool removes omitted members")
		})
	}
}

// The same fixture is also executed against the pre-change readers.
// Every eligible channel has an explicit value, including disabled exclusions,
// so a complete cutover snapshot works with both sparse and whitelist readers.
func TestUserRoutingPoolCutoverSnapshotDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
				t.Skip("isolated database DSN not configured")
			}
			t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
			db := setupManageUserTestDB(t)
			previousCache := common.MemoryCacheEnabled
			t.Cleanup(func() { common.MemoryCacheEnabled = previousCache })
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
			user := model.User{Username: "cutover-snapshot", Group: "default", AffCode: "cutover-snapshot"}
			require.NoError(t, db.Create(&user).Error)
			for _, channel := range []model.Channel{
				{Id: 9951, Name: "Default", Type: 1, Models: "gpt-5", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(50)), Weight: common.GetPointer(uint(5))},
				{Id: 9952, Name: "User preferred", Type: 1, Models: "gpt-5", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(20)), Weight: common.GetPointer(uint(8))},
				{Id: 9953, Name: "Excluded", Type: 1, Models: "gpt-5", Group: "default,vip", Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(10)), Weight: common.GetPointer(uint(13))},
			} {
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(nil))
			}
			// Retaining legacy storage during cutover must not change the snapshot.
			require.NoError(t, db.Create(&[]model.UserChannelRoutingOverride{
				{UserId: user.Id, Model: "gpt-5", ChannelId: 9952, Priority: common.GetPointer(int64(70)), Weight: common.GetPointer(uint(17))},
				{UserId: user.Id, Model: "gpt-5", ChannelId: 9953, Disabled: true},
			}).Error)
			pool := model.ModelRoutingPool{RoutingGroup: "default", Model: "gpt-5", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 9951, Priority: 100, Weight: 90, Enabled: true}}}
			require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
			for _, group := range []string{"default", "vip"} {
				firstPriority := int64(30)
				if group == "vip" {
					firstPriority = 50
				}
				snapshot := []model.UserChannelRoutingPatch{
					{ChannelId: 9951, Priority: &firstPriority, Weight: common.GetPointer(uint(5)), Enabled: common.GetPointer(true)},
					{ChannelId: 9952, Priority: common.GetPointer(int64(70)), Weight: common.GetPointer(uint(17)), Enabled: common.GetPointer(true)},
					{ChannelId: 9953, Priority: common.GetPointer(int64(10)), Weight: common.GetPointer(uint(13)), Enabled: common.GetPointer(false)},
				}
				previousRevision := ""
				for range 2 {
					var stored []model.UserChannelRoutingOverride
					require.NoError(t, db.Table("user_group_channel_routing_overrides").Where("user_id = ? AND model = ? AND routing_group = ?", user.Id, "gpt-5", group).Find(&stored).Error)
					revision, err := model.UserChannelRoutingRevision(stored)
					require.NoError(t, err)
					require.NoError(t, model.PatchUserChannelRoutingForGroup(user.Id, "gpt-5", group, revision, snapshot))
					stored = nil
					require.NoError(t, db.Table("user_group_channel_routing_overrides").Where("user_id = ? AND model = ? AND routing_group = ?", user.Id, "gpt-5", group).Find(&stored).Error)
					require.Len(t, stored, 3)
					currentRevision, err := model.UserChannelRoutingRevision(stored)
					require.NoError(t, err)
					if previousRevision != "" {
						assert.Equal(t, previousRevision, currentRevision, "reapplying the snapshot is idempotent")
					}
					previousRevision = currentRevision
					for _, cached := range []bool{false, true} {
						common.MemoryCacheEnabled = cached
						if cached {
							model.InitChannelCache()
						}
						ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
						common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
						param := &service.RetryParam{Ctx: ctx, ModelName: "gpt-5"}
						filters, err := param.SelectionFilters(group)
						require.NoError(t, err)
						candidates, err := model.ListChannelSelectionCandidates(group, "gpt-5", filters)
						require.NoError(t, err)
						require.Len(t, candidates, 2, "group=%s cache=%t", group, cached)
						actual := make(map[int][2]int64, len(candidates))
						for _, candidate := range candidates {
							actual[candidate.ChannelId] = [2]int64{*candidate.Priority, int64(candidate.Weight)}
						}
						assert.Equal(t, map[int][2]int64{9951: {firstPriority, 5}, 9952: {70, 17}}, actual)
					}
				}
			}
		})
	}
}
