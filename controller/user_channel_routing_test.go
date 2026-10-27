package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

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

func userRoutingRequest(t *testing.T, userID int, method, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	return userRoutingRequestAtURL(t, userID, method, "/api/user/1/channel-routing-overrides?model=gpt-5", body, handler)
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

func readUserRoutingForGroups(t *testing.T, userID int, groups []string) userChannelRoutingResponse {
	t.Helper()
	encoded, err := common.Marshal(groups)
	require.NoError(t, err)
	requestURL := "/api/user/1/channel-routing-overrides?model=gpt-5&channel_groups=" + url.QueryEscape(string(encoded))
	recorder := userRoutingRequestAtURL(t, userID, http.MethodGet, requestURL, "", GetUserChannelRouting)
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
			previousCache, previousPolicies := common.MemoryCacheEnabled, setting.GroupModelChannelGroupsJSON()
			common.MemoryCacheEnabled = false
			require.NoError(t, setting.UpdateGroupModelChannelGroups(`{}`))
			t.Cleanup(func() {
				common.MemoryCacheEnabled = previousCache
				require.NoError(t, setting.UpdateGroupModelChannelGroups(previousPolicies))
			})
			// These pre-existing schemas and records model an upgrade from the
			// current release; no existing schema changes are needed by this feature.
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}))
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
			poolA := readUserRoutingForGroups(t, users[0].Id, []string{"pool-a"})
			require.Len(t, poolA.Channels, 1)
			assert.Equal(t, 9101, poolA.Channels[0].ChannelID)
			poolB := readUserRoutingForGroups(t, users[0].Id, []string{"pool-b"})
			require.Len(t, poolB.Channels, 1)
			assert.Equal(t, 9102, poolB.Channels[0].ChannelID)
			assert.Empty(t, readUserRoutingForGroups(t, users[0].Id, []string{}).Channels)
			invalidGroups := userRoutingRequestAtURL(t, users[0].Id, http.MethodGet, "/api/user/1/channel-routing-overrides?model=gpt-5&channel_groups=%7B", "", GetUserChannelRouting)
			assert.Equal(t, http.StatusBadRequest, invalidGroups.Code)
			payload := fmt.Sprintf(`{"model":"gpt-5","revision":%q,"overrides":[{"channel_id":9101,"priority_override":0,"enabled":true},{"channel_id":9102,"priority_override":200,"enabled":true}]}`, initial.Revision)
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
			configuredPoolA := readUserRoutingForGroups(t, users[0].Id, []string{"pool-a"})
			require.Len(t, configuredPoolA.Channels, 1)
			assert.Equal(t, 9101, configuredPoolA.Channels[0].ChannelID)
			assert.Error(t, db.Create(&model.UserChannelRoutingOverride{UserId: users[0].Id, Model: "gpt-5", ChannelId: 9102, Priority: common.GetPointer(int64(1))}).Error)
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
				body := fmt.Sprintf(`{"model":"gpt-5","revision":%q,"overrides":[{"channel_id":9102,"priority_override":200,"enabled":%t}]}`, current.Revision, enabled)
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
			err := model.PatchUserChannelRouting(users[0].Id, "gpt-5", current.Revision, []model.UserChannelRoutingPatch{
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
				body := fmt.Sprintf(`{"model":"gpt-5","revision":%q,"overrides":[%s]}`, current.Revision, invalid)
				assert.Equal(t, http.StatusBadRequest, userRoutingRequest(t, users[0].Id, http.MethodPatch, body, PatchUserChannelRouting).Code)
			}
			// Null restores model-level inheritance; the second user's row is separate.
			empty, err := model.UserChannelRoutingRevision([]model.UserChannelRoutingOverride{})
			require.NoError(t, err)
			require.NoError(t, model.PatchUserChannelRouting(users[1].Id, "gpt-5", empty, []model.UserChannelRoutingPatch{{ChannelId: 9102, Enabled: common.GetPointer(false)}}))
			require.NoError(t, model.PatchUserChannelRouting(users[0].Id, "gpt-5", current.Revision, []model.UserChannelRoutingPatch{
				{ChannelId: 9101, Enabled: common.GetPointer(true)}, {ChannelId: 9102, Enabled: common.GetPointer(true)},
			}))
			assert.Equal(t, initial.Revision, readUserRouting(t, users[0].Id).Revision)
			otherRows, err := model.ListUserChannelRoutingOverrides(users[1].Id, "gpt-5")
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
				require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.UserChannelRoutingOverride{}))
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
						{"pool-a", "gpt-5", 9201}, {"pool-b", "gpt-5", 9202}, {"pool-a", "gpt-5", 9201},
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
					}{{"pool-a", 9201}, {"pool-b", 9202}} {
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
						if upgrade {
							expected = 25
						}
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
