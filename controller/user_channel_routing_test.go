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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
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
			require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}))
			require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}))
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
			require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}))
			require.NoError(t, db.AutoMigrate(&model.UserChannelRoutingOverride{}))
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
