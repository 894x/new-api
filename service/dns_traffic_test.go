package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDNSConfigPersistenceAcrossDatabases(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector = sqlite.Open(":memory:")
			if dialect != "sqlite" {
				dsn := os.Getenv("DNS_TEST_" + strings.ToUpper(dialect) + "_DSN")
				if dsn == "" {
					t.Skip("isolated real database DSN is required for the full matrix")
				}
				if dialect == "mysql" {
					driver = mysql.Open(dsn)
				} else {
					driver = postgres.Open(dsn)
				}
			}
			db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			var version string
			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("Real %s version: %s", dialect, version)
			previousDB := model.DB
			model.DB = db
			t.Cleanup(func() { model.DB = previousDB; _ = sqlDB.Close() })
			// The options schema is unchanged. Seed an existing setting and verify
			// that adding/updating the encrypted row leaves it intact after reload.
			require.NoError(t, db.AutoMigrate(&model.Option{}))
			require.NoError(t, db.Create(&model.Option{Key: "DNS-test-sentinel", Value: "existing-value"}).Error)
			for range 2 {
				require.NoError(t, db.AutoMigrate(&model.Option{}))
			}
			t.Setenv("CRYPTO_SECRET", strings.Repeat("test-key-", 8))
			t.Setenv("DNSPOD_SECRET_ID", "environment-id")
			t.Setenv("DNSPOD_SECRET_KEY", "environment-key")
			t.Setenv("DNSPOD_SESSION_TOKEN", "environment-token")
			t.Setenv("DNSPOD_DOMAIN", "example.com")
			t.Setenv("DNSPOD_SUBDOMAIN", "api")
			t.Setenv("DNSPOD_RECORD_LINE", "默认")
			t.Setenv("DNSPOD_RECORD_TYPE", "A")
			_, provider, server := newDNSTestClient(t)
			state, err := GetDNSPodConfig(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "environment", state.Source)
			update := DNSPodConfigUpdate{Revision: state.Revision, Enabled: true, Domain: "example.com", Subdomain: "api", RecordLine: "默认", RecordType: "A", SecretKey: "saved-secret-key"}
			saved, err := SaveDNSPodConfig(context.Background(), update)
			require.NoError(t, err)
			assert.Equal(t, "database", saved.Source)
			assert.True(t, saved.SecretIDSet && saved.SecretKeySet && saved.SessionTokenSet)
			assert.Empty(t, server.Keys())
			assert.Empty(t, provider.writes)
			var persisted model.Option
			require.NoError(t, db.Where(&model.Option{Key: model.DNSPodConfigOptionKey}).First(&persisted).Error)
			for _, secret := range []string{"environment-id", "saved-secret-key", "environment-token"} {
				assert.NotContains(t, persisted.Value, secret)
				response, err := common.Marshal(saved)
				require.NoError(t, err)
				assert.NotContains(t, string(response), secret)
			}
			client, err := NewDNSPodTrafficClient()
			require.NoError(t, err)
			assert.Equal(t, "environment-id", client.config.SecretID)
			assert.Equal(t, "saved-secret-key", client.config.SecretKey)
			_, err = SaveDNSPodConfig(context.Background(), update)
			require.Error(t, err, "a stale revision must not replace the credentials")
			update.Revision = saved.Revision
			update.SecretKey = ""
			update.ClearSessionToken = true
			update.Enabled = false
			disabled, err := SaveDNSPodConfig(context.Background(), update)
			require.NoError(t, err)
			assert.False(t, disabled.Enabled)
			assert.False(t, disabled.SessionTokenSet)
			client, err = NewDNSPodTrafficClient()
			require.NoError(t, err)
			assert.Nil(t, client, "disabled database configuration must override environment credentials")
			require.Error(t, model.UpdateOption(model.DNSPodConfigOptionKey, "unverified"))
			require.Error(t, model.UpdateOptionsBulk(map[string]string{model.DNSPodConfigOptionKey: "unverified"}))
			update.Revision = disabled.Revision
			update.Enabled = true
			_, err = SaveDNSPodConfig(context.Background(), update)
			require.NoError(t, err)
			client, err = NewDNSPodTrafficClient()
			require.NoError(t, err)
			assert.Equal(t, "saved-secret-key", client.config.SecretKey, "blank secret inputs retain the stored value")
			server.Set("dns-management:lock", "other-operation")
			_, err = SaveDNSPodConfig(context.Background(), update)
			var failure *DNSTrafficError
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, "DNS_BUSY", failure.Code)
			t.Setenv("CRYPTO_SECRET", strings.Repeat("different-key-", 4))
			_, err = NewDNSPodTrafficClient()
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, "DNS_CONFIG_DECRYPT_FAILED", failure.Code)
			var sentinel model.Option
			require.NoError(t, db.Where(&model.Option{Key: "DNS-test-sentinel"}).First(&sentinel).Error)
			assert.Equal(t, "existing-value", sentinel.Value)
		})
	}
}

func TestDNSConfigInputAndEncryptionKeyValidation(t *testing.T) {
	valid := DNSPodConfigUpdate{Revision: strings.Repeat("a", 64), Enabled: true, Domain: "xunlitec.com", Subdomain: "newapi", RecordLine: "默认", RecordType: "A", SecretID: "new-id", SecretKey: "new-key"}
	require.NoError(t, ValidateDNSPodConfigUpdate(valid))
	assert.Equal(t, "3d5c9b5b3cfe37dbd1e8cc8ab7603e768d1ab99c3ac106a7de1172b8e83751e7", DNSPodConfigChangeHash(valid))
	for _, scenario := range []string{"domain", "subdomain", "line", "type", "credential", "token"} {
		t.Run(scenario, func(t *testing.T) {
			update := valid
			switch scenario {
			case "domain":
				update.Domain = "https://example.com"
			case "subdomain":
				update.Subdomain = "bad/host"
			case "line":
				update.RecordLine = ""
			case "type":
				update.RecordType = "CNAME"
			case "credential":
				update.SecretKey = "key\r\nInjected: header"
			case "token":
				update.ClearSessionToken, update.SessionToken = true, "new-token"
			}
			require.Error(t, ValidateDNSPodConfigUpdate(update))
		})
	}
	t.Setenv("CRYPTO_SECRET", "")
	t.Setenv("SESSION_SECRET", "")
	_, err := dnsConfigCipher()
	require.Error(t, err, "random process defaults must never encrypt persisted credentials")
	t.Setenv("SESSION_SECRET", strings.Repeat("s", 32))
	_, err = dnsConfigCipher()
	require.NoError(t, err)
}

func TestDNSConfigChangeRejectsPreviouslyPreparedWeightUpdates(t *testing.T) {
	setupAuthSessionTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	t.Setenv("CRYPTO_SECRET", strings.Repeat("test-key-", 8))
	for _, name := range []string{"DNSPOD_SECRET_ID", "DNSPOD_SECRET_KEY", "DNSPOD_SESSION_TOKEN", "DNSPOD_DOMAIN", "DNSPOD_SUBDOMAIN"} {
		t.Setenv(name, "")
	}
	fakeClient, provider, _ := newDNSTestClient(t)
	state, err := GetDNSPodConfig(context.Background())
	require.NoError(t, err)
	configUpdate := DNSPodConfigUpdate{Revision: state.Revision, Enabled: true, Domain: "example.com", Subdomain: "api", RecordLine: "默认", RecordType: "A", SecretID: "test-id", SecretKey: "test-key"}
	saved, err := SaveDNSPodConfig(context.Background(), configUpdate)
	require.NoError(t, err)
	oldClient, err := NewDNSPodTrafficClient()
	require.NoError(t, err)
	oldClient.httpClient = fakeClient.httpClient
	before, err := oldClient.Read(context.Background())
	require.NoError(t, err)
	configUpdate.Revision, configUpdate.SecretKey = saved.Revision, "replacement-key"
	_, err = SaveDNSPodConfig(context.Background(), configUpdate)
	require.NoError(t, err)
	weights := DNSTrafficUpdate{Revision: before.Revision, Weights: []DNSInstanceWeight{{ID: "11", Weight: 70}, {ID: "12", Weight: 30}}}
	_, err = oldClient.Apply(context.Background(), weights)
	var failure *DNSTrafficError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "DNS_CONFIG_CONFLICT", failure.Code)
	newClient, err := NewDNSPodTrafficClient()
	require.NoError(t, err)
	newClient.httpClient = fakeClient.httpClient
	_, err = newClient.Apply(context.Background(), weights)
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "DNS_CONFLICT", failure.Code)
	assert.Empty(t, provider.writes)
}

type dnsTestTransport func(*http.Request) (*http.Response, error)

func (transport dnsTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type dnsTestRecord struct {
	RecordID                                           int64 `json:"RecordId"`
	Name, Value, Type, Line, Status, Remark, UpdatedOn string
	Weight                                             int
	TTL                                                int
}

type dnsTestProvider struct {
	records      []dnsTestRecord
	writes       []int64
	failRecord   int64
	ignoreWrites bool
}

func newDNSTestClient(t *testing.T) (*DNSPodTrafficClient, *dnsTestProvider, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	previousRedis, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() { _ = common.RDB.Close(); common.RDB = previousRedis; common.RedisEnabled = previousEnabled })
	provider := &dnsTestProvider{records: []dnsTestRecord{
		{RecordID: 11, Name: "api", Value: "192.0.2.1", Type: "A", Line: "默认", Status: "ENABLE", Weight: 50, TTL: 600, Remark: "old"},
		{RecordID: 12, Name: "api", Value: "192.0.2.2", Type: "A", Line: "默认", Status: "ENABLE", Weight: 50, TTL: 600, Remark: "new"},
		{RecordID: 13, Name: "api", Value: "192.0.2.3", Type: "A", Line: "默认", Status: "DISABLE", Weight: 50, TTL: 300, Remark: "paused"},
	}}
	client := &DNSPodTrafficClient{config: DNSPodTrafficConfig{Domain: "example.com", Subdomain: "api", RecordLine: "默认", RecordType: "A", SecretID: "test-id", SecretKey: "test-key"}}
	client.httpClient = &http.Client{Transport: dnsTestTransport(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://dnspod.tencentcloudapi.com/", request.URL.String())
		assert.Equal(t, "2021-03-23", request.Header.Get("X-TC-Version"))
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		var payload map[string]common.RawMessage
		require.NoError(t, common.Unmarshal(body, &payload))
		var domain string
		require.NoError(t, common.Unmarshal(payload["Domain"], &domain))
		assert.Equal(t, "example.com", domain)
		var result any
		switch request.Header.Get("X-TC-Action") {
		case "DescribeRecordList":
			var offset int
			require.NoError(t, common.Unmarshal(payload["Offset"], &offset))
			assert.JSONEq(t, `"api"`, string(payload["SubDomain"]))
			if offset < len(provider.records) {
				result = map[string]any{"RecordList": provider.records[offset:min(offset+100, len(provider.records))], "RequestId": "list-request"}
			} else {
				result = map[string]any{"RecordList": []dnsTestRecord{}, "RequestId": "list-request"}
			}
		case "ModifyRecordFields":
			assert.Len(t, payload, 3, "only weight should be changed")
			var id int64
			require.NoError(t, common.Unmarshal(payload["RecordId"], &id))
			var fields []struct{ Key, Value string }
			require.NoError(t, common.Unmarshal(payload["FieldList"], &fields))
			require.Len(t, fields, 1)
			assert.Equal(t, "weight", fields[0].Key)
			provider.writes = append(provider.writes, id)
			if id == provider.failRecord {
				result = map[string]any{"Error": map[string]string{"Code": "OperationDenied.NoPermission", "Message": "secret-key must never reach the UI"}, "RequestId": "failure-request"}
				break
			}
			weight, err := strconv.Atoi(fields[0].Value)
			require.NoError(t, err)
			if !provider.ignoreWrites {
				for i := range provider.records {
					if provider.records[i].RecordID == id {
						provider.records[i].Weight = weight
						provider.records[i].UpdatedOn = "updated"
					}
				}
			}
			result = map[string]any{"RecordId": id, "RequestId": "write-request"}
		default:
			require.FailNow(t, "unexpected DNS operation")
		}
		data, err := common.Marshal(map[string]any{"Response": result})
		require.NoError(t, err)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})}
	return client, provider, server
}

func TestDNSReadPaginatesAndFiltersScope(t *testing.T) {
	client, provider, _ := newDNSTestClient(t)
	for i := range 98 {
		provider.records = append(provider.records, dnsTestRecord{RecordID: int64(100 + i), Name: "unmanaged", Value: "192.0.2.9", Type: "A", Line: "默认", Status: "ENABLE", Weight: 50, TTL: 600})
	}
	provider.records = append(provider.records, dnsTestRecord{RecordID: 301, Name: "api", Type: "AAAA", Line: "默认"}, dnsTestRecord{RecordID: 302, Name: "api", Type: "A", Line: "电信"})
	state, err := client.Read(context.Background())
	require.NoError(t, err)
	assert.True(t, state.CanApply)
	require.Len(t, state.Records, 3)
	assert.Equal(t, 600, state.Records[0].TTL)
	assert.False(t, state.Records[2].Enabled)
	assert.Len(t, state.Revision, 64)
	assert.Empty(t, provider.writes)
}

func TestDNSApplyChangesOnlyWeightsAndVerifiesProviderState(t *testing.T) {
	client, provider, server := newDNSTestClient(t)
	before, err := client.Read(context.Background())
	require.NoError(t, err)
	result, err := client.Apply(context.Background(), DNSTrafficUpdate{Revision: before.Revision, Weights: []DNSInstanceWeight{{ID: "11", Weight: 70}, {ID: "12", Weight: 30}}})
	require.NoError(t, err)
	assert.Equal(t, []int64{11, 12}, provider.writes)
	assert.Equal(t, 70, result.Records[0].Weight)
	assert.Equal(t, 30, result.Records[1].Weight)
	assert.Equal(t, before.Records[2], result.Records[2])
	assert.Equal(t, 600, result.Records[0].TTL)
	assert.Equal(t, "192.0.2.1", result.Records[0].IP)
	assert.NotEqual(t, before.Revision, result.Revision)
	assert.Empty(t, server.Keys(), "successful writes release the lease")
}

func TestDNSApplyRejectsInvalidOrStaleSnapshotsWithoutWrites(t *testing.T) {
	for _, scenario := range []string{"zero", "fractional-id", "duplicate", "foreign", "missing", "stale", "redis-disabled", "locked"} {
		t.Run(scenario, func(t *testing.T) {
			client, provider, server := newDNSTestClient(t)
			state, err := client.Read(context.Background())
			require.NoError(t, err)
			update := DNSTrafficUpdate{Revision: state.Revision, Weights: []DNSInstanceWeight{{ID: "11", Weight: 70}, {ID: "12", Weight: 30}}}
			switch scenario {
			case "zero":
				update.Weights[0].Weight = 0
			case "fractional-id":
				update.Weights[0].ID = "11.5"
			case "duplicate":
				update.Weights[1].ID = "11"
			case "foreign":
				update.Weights[1].ID = "999"
			case "missing":
				update.Weights = update.Weights[:1]
			case "stale":
				provider.records[0].TTL = 60
			case "redis-disabled":
				common.RedisEnabled = false
			case "locked":
				server.Set("dns-management:lock", "another-worker")
			}
			_, err = client.Apply(context.Background(), update)
			require.Error(t, err)
			assert.Empty(t, provider.writes)
		})
	}
}

func TestDNSPartialFailureAndUnverifiedWritesRetainLease(t *testing.T) {
	for _, scenario := range []string{"provider-error", "readback-mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			client, provider, server := newDNSTestClient(t)
			if scenario == "provider-error" {
				provider.failRecord = 12
			} else {
				provider.ignoreWrites = true
			}
			before, err := client.Read(context.Background())
			require.NoError(t, err)
			update := DNSTrafficUpdate{Revision: before.Revision, Weights: []DNSInstanceWeight{{ID: "11", Weight: 70}, {ID: "12", Weight: 30}}}
			_, err = client.Apply(context.Background(), update)
			var failure *DNSTrafficError
			require.ErrorAs(t, err, &failure)
			assert.True(t, failure.MayHaveChanged)
			assert.NotContains(t, failure.Error(), "secret-key")
			assert.Len(t, server.Keys(), 1)
			if scenario == "provider-error" {
				assert.Equal(t, "OperationDenied.NoPermission", failure.ProviderCode)
				assert.Equal(t, "failure-request", failure.RequestID)
				assert.Equal(t, 70, provider.records[0].Weight)
				assert.Equal(t, 50, provider.records[1].Weight)
			} else {
				assert.Equal(t, "DNS_VERIFY_FAILED", failure.Code)
			}
			writes := len(provider.writes)
			_, err = client.Apply(context.Background(), update)
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, "DNS_BUSY", failure.Code)
			assert.Len(t, provider.writes, writes)
		})
	}
}

func TestDNSConfigurationNeverReturnsCredentials(t *testing.T) {
	setupAuthSessionTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	for _, key := range []string{"DNSPOD_SECRET_ID", "DNSPOD_SECRET_KEY", "DNSPOD_SESSION_TOKEN", "DNSPOD_DOMAIN", "DNSPOD_SUBDOMAIN", "DNSPOD_RECORD_LINE", "DNSPOD_RECORD_TYPE"} {
		t.Setenv(key, "")
	}
	client, err := NewDNSPodTrafficClient()
	require.NoError(t, err)
	assert.Nil(t, client)
	t.Setenv("DNSPOD_SECRET_KEY", "sensitive-key")
	_, err = NewDNSPodTrafficClient()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sensitive-key")
}

func TestDNSTC3SignatureMatchesIndependentReferenceVector(t *testing.T) {
	// Tencent's official TC3 canonical-request example, signed with dummy
	// credentials. Expected signature computed independently with Python hmac.
	// https://cloud.tencent.com/document/product/876/34813
	body := []byte(`{"Limit": 1, "Filters": [{"Values": ["\u672a\u547d\u540d"], "Name": "instance-name"}]}`)
	assert.Equal(t, "TC3-HMAC-SHA256 Credential=test-id/2019-02-25/cvm/tc3_request, SignedHeaders=content-type;host;x-tc-action, Signature=3ee338569fd07d7531146fd2d5512b238be60b4376ec46bc39bbc2456a6b2ad3",
		tencentAuthorization("test-id", "test-key", "cvm.tencentcloudapi.com", "cvm", "DescribeInstances", body, 1551113065))
}

func TestDNSProofRejectsReplayExpiryAndChangedAllocation(t *testing.T) {
	for _, scope := range []string{VerificationScopeDNSTrafficUpdate, VerificationScopeDNSConfigUpdate} {
		t.Run(scope, func(t *testing.T) {
			useTestSessionSecret(t)
			user := setupAuthSessionTestDB(t)
			require.NoError(t, model.DB.AutoMigrate(&model.TwoFA{}, &model.PasskeyCredential{}))
			require.NoError(t, model.DB.Model(user).Update("role", common.RoleRootUser).Error)
			previousPassword := common.PasswordLoginEnabled
			common.PasswordLoginEnabled = true
			t.Cleanup(func() { common.PasswordLoginEnabled = previousPassword })
			bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "dns-test")
			require.NoError(t, err)
			identity, err := ParseAccessToken(bundle.AccessToken)
			require.NoError(t, err)
			operation := VerificationOperation{Scope: VerificationScopeDNSTrafficUpdate, Context: []byte(`{"revision":"` + strings.Repeat("a", 64) + `","weights":[{"id":"11","weight":70},{"id":"12","weight":30}]}`)}
			if scope == VerificationScopeDNSConfigUpdate {
				operation = VerificationOperation{Scope: scope, Context: []byte(`{"revision":"` + strings.Repeat("a", 64) + `","change_hash":"` + strings.Repeat("b", 64) + `"}`)}
			}
			binding, err := BindVerificationOperation(operation)
			require.NoError(t, err)
			proof, _, err := IssueSecurityProof(identity, VerificationMethodPassword, binding)
			require.NoError(t, err)
			changed := operation
			changed.Context = []byte(strings.Replace(string(operation.Context), `"weight":70`, `"weight":71`, 1))
			if scope == VerificationScopeDNSConfigUpdate {
				changed.Context = []byte(strings.Replace(string(operation.Context), strings.Repeat("b", 64), strings.Repeat("c", 64), 1))
			}
			_, err = ConsumeOperationProof(proof, identity, changed)
			assert.ErrorIs(t, err, ErrProofContext)
			_, err = ConsumeOperationProof(proof, identity, VerificationOperation{Scope: VerificationScopePasskeyDelete})
			assert.ErrorIs(t, err, ErrProofScope)
			_, err = ConsumeOperationProof(proof, identity, operation)
			require.NoError(t, err)
			_, err = ConsumeOperationProof(proof, identity, operation)
			assert.ErrorIs(t, err, ErrProofConsumed)
			claims, err := parseAuthClaims(proof, securityProofTokenUse, authSigningKey(securityProofTokenUse))
			require.NoError(t, err)
			claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
			expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(authSigningKey(securityProofTokenUse))
			require.NoError(t, err)
			_, err = ConsumeOperationProof(expired, identity, operation)
			assert.ErrorIs(t, err, ErrAuthTokenExpired)
			common.PasswordLoginEnabled = false
			_, err = RequireVerificationMethod(identity, scope, VerificationMethodPassword)
			assert.ErrorIs(t, err, ErrVerificationUnavailable)
		})
	}
}

func TestDNSVerificationBindsCompleteAllocationAndEnforcesRootAndMFA(t *testing.T) {
	useTestSessionSecret(t)
	contextJSON := `{"revision":"` + strings.Repeat("a", 64) + `","weights":[{"id":"11","weight":70},{"id":"12","weight":30}]}`
	operation := VerificationOperation{Scope: VerificationScopeDNSTrafficUpdate, Context: []byte(contextJSON)}
	binding, err := BindVerificationOperation(operation)
	require.NoError(t, err)
	reordered := VerificationOperation{Scope: VerificationScopeDNSTrafficUpdate, Context: []byte(`{"revision":"` + strings.Repeat("a", 64) + `","weights":[{"id":"12","weight":30},{"id":"11","weight":70}]}`)}
	reorderedBinding, err := BindVerificationOperation(reordered)
	require.NoError(t, err)
	assert.Equal(t, binding, reorderedBinding)
	changed := operation
	changed.Context = []byte(strings.Replace(contextJSON, `"weight":70`, `"weight":71`, 1))
	changedBinding, err := BindVerificationOperation(changed)
	require.NoError(t, err)
	assert.NotEqual(t, binding.ContextHash, changedBinding.ContextHash)
	_, err = securityVerificationPolicy(VerificationScopeDNSTrafficUpdate, model.UserVerificationState{Role: common.RoleAdminUser, HasPassword: true})
	assert.ErrorIs(t, err, ErrVerificationForbidden)
	methods, err := securityVerificationPolicy(VerificationScopeDNSTrafficUpdate, model.UserVerificationState{Role: common.RoleRootUser, HasPassword: true})
	require.NoError(t, err)
	assert.Equal(t, VerificationMethodPassword, methods[0].Method)
	methods, err = securityVerificationPolicy(VerificationScopeDNSTrafficUpdate, model.UserVerificationState{Role: common.RoleRootUser, HasPassword: true, HasTwoFA: true})
	require.NoError(t, err)
	require.Len(t, methods, 1)
	assert.Equal(t, VerificationMethodTwoFA, methods[0].Method)
	setupAuthSessionTestDB(t)
	identity := AuthIdentity{UserID: 42, SessionID: "dns-session", UserAuthVersion: 1, SessionVersion: 1}
	proof, _, err := IssueSecurityProof(identity, VerificationMethodPassword, binding)
	require.NoError(t, err)
	_, err = verifySecurityProof(proof, identity, changedBinding)
	assert.ErrorIs(t, err, ErrProofContext)
	otherSession := identity
	otherSession.SessionID = "other"
	_, err = verifySecurityProof(proof, otherSession, binding)
	assert.ErrorIs(t, err, ErrAuthTokenInvalid)
	_, err = verifySecurityProof(proof, identity, binding)
	require.NoError(t, err)
}
