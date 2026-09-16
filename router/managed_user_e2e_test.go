package router

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	projecti18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type managedUserE2EFixture struct {
	server                 *httptest.Server
	root, managerA         model.User
	managerB, unprivileged model.User
	tokens                 map[int]string
}

func setupManagedUserE2E(t *testing.T) *managedUserE2EFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	previousSecret, previousMaster := common.SessionSecret, common.IsMasterNode
	previousSetup, previousNewUserQuota := constant.Setup, common.QuotaForNewUser

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.SessionSecret = "managed-user-e2e-secret"
	common.IsMasterNode = true
	common.QuotaForNewUser = 0
	constant.Setup = true
	require.NoError(t, projecti18n.Init())
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.UserSession{},
		&model.TopUp{},
		&model.Log{},
		&model.CasbinRule{},
		&model.AuthzRole{},
	))
	require.NoError(t, authz.Init(db))

	f := &managedUserE2EFixture{tokens: map[int]string{}}
	f.root = model.User{Username: "managed-root", Password: "unused-password", DisplayName: "Root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "managed-root", AuthVersion: 1}
	f.managerA = model.User{Username: "manager-a", Password: "unused-password", DisplayName: "Manager A", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "manager-a", AuthVersion: 1}
	f.managerB = model.User{Username: "manager-b", Password: "unused-password", DisplayName: "Manager B", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "manager-b", AuthVersion: 1}
	f.unprivileged = model.User{Username: "unprivileged", Password: "unused-password", DisplayName: "Unprivileged", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "unprivileged", AuthVersion: 1}
	for _, user := range []*model.User{&f.root, &f.managerA, &f.managerB, &f.unprivileged} {
		require.NoError(t, db.Create(user).Error)
		bundle, sessionErr := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "managed-user-e2e")
		require.NoError(t, sessionErr)
		f.tokens[user.Id] = bundle.AccessToken
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	SetApiRouter(engine)
	f.server = httptest.NewServer(engine)

	t.Cleanup(func() {
		f.server.Close()
		_ = sqlDB.Close()
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemoryCache
		common.SessionSecret, common.IsMasterNode = previousSecret, previousMaster
		constant.Setup, common.QuotaForNewUser = previousSetup, previousNewUserQuota
	})
	return f
}

func (f *managedUserE2EFixture) request(t *testing.T, method, path string, userID int, payload any) (int, map[string]any) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := common.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, f.server.URL+path, body)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+f.tokens[userID])
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	result := map[string]any{}
	require.NoError(t, common.Unmarshal(data, &result), string(data))
	return response.StatusCode, result
}

func grantManagedUserPermissions(t *testing.T, f *managedUserE2EFixture, target model.User) {
	t.Helper()
	status, response := f.request(t, http.MethodPut, "/api/user/", f.root.Id, map[string]any{
		"id":           target.Id,
		"username":     target.Username,
		"display_name": target.DisplayName,
		"role":         target.Role,
		"group":        target.Group,
		"admin_permissions": map[string]map[string]bool{
			authz.ResourceUser: {
				authz.ActionCreate:      true,
				authz.ActionRead:        true,
				authz.ActionBillingRead: true,
			},
		},
	})
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, response["success"])
}

func pageItems(t *testing.T, response map[string]any) []any {
	t.Helper()
	data, ok := response["data"].(map[string]any)
	require.True(t, ok)
	items, ok := data["items"].([]any)
	require.True(t, ok)
	return items
}

func itemIDs(t *testing.T, items []any, key string) []int {
	t.Helper()
	ids := make([]int, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		require.True(t, ok)
		value, ok := row[key].(float64)
		require.True(t, ok)
		ids = append(ids, int(value))
	}
	return ids
}

func TestManagedUserPermissionsEndToEnd(t *testing.T) {
	f := setupManagedUserE2E(t)
	grantManagedUserPermissions(t, f, f.managerA)
	grantManagedUserPermissions(t, f, f.managerB)
	status, response := f.request(t, http.MethodGet, "/api/user/self", f.managerA.Id, nil)
	require.Equal(t, http.StatusOK, status)
	selfData, ok := response["data"].(map[string]any)
	require.True(t, ok)
	permissions, ok := selfData["permissions"].(map[string]any)
	require.True(t, ok)
	capabilities, ok := permissions["admin_permissions"].(map[string]any)
	require.True(t, ok)
	userCapabilities, ok := capabilities[authz.ResourceUser].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, userCapabilities[authz.ActionRead])
	assert.Equal(t, true, userCapabilities[authz.ActionCreate])
	assert.Equal(t, true, userCapabilities[authz.ActionBillingRead])

	status, response = f.request(t, http.MethodGet, "/api/user/?p=1&page_size=100", f.unprivileged.Id, nil)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, false, response["success"])

	for managerID, username := range map[int]string{f.managerA.Id: "child-a", f.managerB.Id: "child-b"} {
		status, response = f.request(t, http.MethodPost, "/api/user/", managerID, map[string]any{
			"username":     username,
			"display_name": username,
			"password":     "password-2026",
			"role":         common.RoleCommonUser,
		})
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, true, response["success"])
	}

	childA := model.User{}
	require.NoError(t, model.DB.Where("username = ?", "child-a").First(&childA).Error)
	childB := model.User{}
	require.NoError(t, model.DB.Where("username = ?", "child-b").First(&childB).Error)
	require.NotNil(t, childA.ManagedByUserId)
	require.NotNil(t, childB.ManagedByUserId)
	assert.Equal(t, f.managerA.Id, *childA.ManagedByUserId)
	assert.Equal(t, f.managerB.Id, *childB.ManagedByUserId)

	status, response = f.request(t, http.MethodPost, "/api/user/", f.managerA.Id, map[string]any{
		"username": "forbidden-admin", "display_name": "Forbidden", "password": "password-2026", "role": common.RoleAdminUser,
	})
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, false, response["success"])

	legacy := model.User{Username: "legacy-user", DisplayName: "Legacy", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "legacy", AuthVersion: 1}
	require.NoError(t, model.DB.Create(&legacy).Error)
	require.Nil(t, legacy.ManagedByUserId)

	status, response = f.request(t, http.MethodGet, "/api/user/?p=1&page_size=100", f.managerA.Id, nil)
	require.Equal(t, http.StatusOK, status)
	assert.ElementsMatch(t, []int{childA.Id}, itemIDs(t, pageItems(t, response), "id"))

	status, response = f.request(t, http.MethodGet, "/api/user/search?keyword=child-b&p=1&page_size=100", f.managerA.Id, nil)
	require.Equal(t, http.StatusOK, status)
	assert.Empty(t, pageItems(t, response))

	status, response = f.request(t, http.MethodGet, "/api/user/"+strconv.Itoa(childB.Id), f.managerA.Id, nil)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, false, response["success"])

	status, response = f.request(t, http.MethodGet, "/api/user/"+strconv.Itoa(childA.Id), f.managerA.Id, nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, true, response["success"])

	status, response = f.request(t, http.MethodGet, "/api/user/?p=1&page_size=100", f.root.Id, nil)
	require.Equal(t, http.StatusOK, status)
	rootIDs := itemIDs(t, pageItems(t, response), "id")
	assert.Contains(t, rootIDs, legacy.Id)
	assert.Contains(t, rootIDs, childA.Id)
	assert.Contains(t, rootIDs, childB.Id)

	require.NoError(t, model.DB.Create(&model.TopUp{UserId: childA.Id, TradeNo: "topup-a", Status: common.TopUpStatusSuccess}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: childB.Id, TradeNo: "topup-b", Status: common.TopUpStatusSuccess}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{UserId: legacy.Id, TradeNo: "topup-legacy", Status: common.TopUpStatusSuccess}).Error)

	status, response = f.request(t, http.MethodGet, "/api/user/topup?p=1&page_size=100", f.managerA.Id, nil)
	require.Equal(t, http.StatusOK, status)
	assert.ElementsMatch(t, []int{childA.Id}, itemIDs(t, pageItems(t, response), "user_id"))

	status, response = f.request(t, http.MethodGet, "/api/user/topup?p=1&page_size=100", f.root.Id, nil)
	require.Equal(t, http.StatusOK, status)
	assert.ElementsMatch(t, []int{childA.Id, childB.Id, legacy.Id}, itemIDs(t, pageItems(t, response), "user_id"))
}
