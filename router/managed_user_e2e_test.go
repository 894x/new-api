package router

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	projecti18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type managedUserE2EFixture struct {
	server                 *httptest.Server
	root, managerA         model.User
	managerB, unprivileged model.User
	tokens                 map[int]string
}

func TestDNSTrafficRoutesRequireRootAndOperationProof(t *testing.T) {
	for _, key := range []string{"DNSPOD_SECRET_ID", "DNSPOD_SECRET_KEY", "DNSPOD_SESSION_TOKEN", "DNSPOD_DOMAIN", "DNSPOD_SUBDOMAIN", "DNSPOD_RECORD_LINE", "DNSPOD_RECORD_TYPE"} {
		t.Setenv(key, "")
	}
	fixture := setupManagedUserE2E(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	status, response := fixture.request(t, http.MethodGet, "/api/system-info/dns-traffic", fixture.root.Id, nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, false, response["data"].(map[string]any)["configured"])
	for _, userID := range []int{0, fixture.unprivileged.Id} {
		status, _ = fixture.request(t, http.MethodGet, "/api/system-info/dns-traffic", userID, nil)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
		status, _ = fixture.request(t, http.MethodPut, "/api/system-info/dns-traffic", userID, nil)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	}
	status, response = fixture.request(t, http.MethodPut, "/api/system-info/dns-traffic", fixture.root.Id, map[string]any{
		"revision": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"weights":  []map[string]any{{"id": "11", "weight": 70}, {"id": "12", "weight": 30}},
	})
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "SECURITY_PROOF_REQUIRED", response["code"])
	for _, userID := range []int{0, fixture.unprivileged.Id, fixture.managerA.Id} {
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			status, _ = fixture.request(t, method, "/api/system-info/dns-traffic/config", userID, nil)
			assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
		}
	}
	t.Setenv("DNSPOD_SECRET_ID", "hidden-dnspod-id")
	t.Setenv("DNSPOD_SECRET_KEY", "hidden-dnspod-key")
	t.Setenv("DNSPOD_DOMAIN", "example.com")
	t.Setenv("DNSPOD_SUBDOMAIN", "api")
	status, response = fixture.request(t, http.MethodGet, "/api/system-info/dns-traffic/config", fixture.root.Id, nil)
	require.Equal(t, http.StatusOK, status)
	config := response["data"].(map[string]any)
	assert.Equal(t, true, config["secret_id_set"])
	assert.Equal(t, true, config["secret_key_set"])
	encoded, err := common.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "hidden-dnspod")
	status, response = fixture.request(t, http.MethodPut, "/api/system-info/dns-traffic/config", fixture.root.Id, map[string]any{
		"revision": config["revision"], "enabled": true, "domain": "example.com", "subdomain": "api", "record_line": "默认", "record_type": "A", "secret_key": "replacement-key",
	})
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "SECURITY_PROOF_REQUIRED", response["code"])
	status, response = fixture.request(t, http.MethodPut, "/api/option/", fixture.root.Id, map[string]any{"key": model.DNSPodConfigOptionKey, "value": "unverified-secret"})
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, false, response["success"])
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{model.DNSPodConfigOptionKey: "private-value"}
	t.Cleanup(func() { common.OptionMap = previousOptions })
	status, response = fixture.request(t, http.MethodGet, "/api/option/", fixture.root.Id, nil)
	require.Equal(t, http.StatusOK, status)
	encoded, err = common.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), model.DNSPodConfigOptionKey)
	assert.NotContains(t, string(encoded), "private-value")
	// Exercise the actual controller's credential digest binding and one-use
	// proof, rather than testing the persistence service alone.
	require.NoError(t, model.DB.AutoMigrate(&model.AuthFlow{}, &model.TwoFA{}, &model.PasskeyCredential{}))
	t.Setenv("CRYPTO_SECRET", "dns-route-test-key-with-at-least-32-characters")
	previousRDB, previousPassword := common.RDB, common.PasswordLoginEnabled
	redisServer := miniredis.RunT(t)
	common.RDB = redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	common.RedisEnabled, common.PasswordLoginEnabled = true, true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RDB = previousRDB
		common.PasswordLoginEnabled = previousPassword
	})
	identity, err := service.ParseAccessToken(fixture.tokens[fixture.root.Id])
	require.NoError(t, err)
	update := service.DNSPodConfigUpdate{Revision: config["revision"].(string), Enabled: true, Domain: "example.com", Subdomain: "api", RecordLine: "默认", RecordType: "A", SecretKey: "verified-replacement-key"}
	proofContext, err := common.Marshal(service.DNSPodConfigProofContext{Revision: update.Revision, ChangeHash: service.DNSPodConfigChangeHash(update)})
	require.NoError(t, err)
	binding, err := service.BindVerificationOperation(service.VerificationOperation{Scope: service.VerificationScopeDNSConfigUpdate, Context: proofContext})
	require.NoError(t, err)
	proof, _, err := service.IssueSecurityProof(identity, service.VerificationMethodPassword, binding)
	require.NoError(t, err)
	changed := update
	changed.SecretKey = "unauthorized-replacement"
	status, response = fixture.request(t, http.MethodPut, "/api/system-info/dns-traffic/config", fixture.root.Id, changed, map[string]string{"X-Security-Proof": proof})
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "SECURITY_PROOF_CONTEXT_MISMATCH", response["code"])
	status, response = fixture.request(t, http.MethodPut, "/api/system-info/dns-traffic/config", fixture.root.Id, update, map[string]string{"X-Security-Proof": proof})
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, true, response["success"])
	encoded, err = common.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "verified-replacement-key")
	status, response = fixture.request(t, http.MethodPut, "/api/system-info/dns-traffic/config", fixture.root.Id, update, map[string]string{"X-Security-Proof": proof})
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "SECURITY_PROOF_CONSUMED", response["code"])
}

func setupManagedUserE2E(t *testing.T) *managedUserE2EFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	previousSecret, previousMaster := common.SessionSecret, common.IsMasterNode
	previousSetup, previousNewUserQuota := constant.Setup, common.QuotaForNewUser

	var dialector gorm.Dialector = sqlite.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	databaseType := common.DatabaseTypeSQLite
	switch os.Getenv("TEST_MANAGED_USER_DIALECT") {
	case "", "sqlite":
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		require.NotEmpty(t, dsn, "TEST_MYSQL_DSN is required for the external test database")
		dialector = mysql.Open(dsn)
		databaseType = common.DatabaseTypeMySQL
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		require.NotEmpty(t, dsn, "TEST_POSTGRES_DSN is required for the external test database")
		dialector = postgres.Open(dsn)
		databaseType = common.DatabaseTypePostgreSQL
	default:
		t.Fatalf("unsupported TEST_MANAGED_USER_DIALECT %q", os.Getenv("TEST_MANAGED_USER_DIALECT"))
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(databaseType, databaseType)
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

func (f *managedUserE2EFixture) request(t *testing.T, method, path string, userID int, payload any, extraHeaders ...map[string]string) (int, map[string]any) {
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
	for _, headers := range extraHeaders {
		for name, value := range headers {
			request.Header.Set(name, value)
		}
	}
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
