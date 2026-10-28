package controller

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestExportLogsRejectsUpstreamFieldsInDownstreamView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/log/export", strings.NewReader(`{"view":"downstream","fields":["request_id","channel_id"]}`))
	context.Request.Header.Set("Content-Type", "application/json")

	ExportLogs(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "channel_id")
}

func selfLogExportTestRequest(t *testing.T, userID int, request any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", userID)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/log/self/export", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	ExportSelfLogs(context)
	return recorder
}

func readSelfLogExportArchive(t *testing.T, recorder *httptest.ResponseRecorder) map[string][][]string {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/zip", recorder.Header().Get("Content-Type"), recorder.Body.String())
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	archive, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	require.NoError(t, err)
	require.Len(t, archive.File, 2)
	files := make(map[string][][]string, len(archive.File))
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		for _, secret := range []string{"CHANNEL_SECRET", "UPSTREAM_SECRET", "DIAGNOSTIC_SECRET", "OTHER_USER_SECRET", "PRICE_SECRET", "GROUP_SECRET", "USERNAME_SECRET"} {
			assert.NotContains(t, string(data), secret, file.Name)
		}
		records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
		require.NoError(t, err)
		files[file.Name] = records
	}
	return files
}

func TestExportSelfLogsOwnConsumptionLedgerAndRedaction(t *testing.T) {
	db, dialect := openTaskDialectDatabase(t, &model.Log{})
	logDB, logDialect := db, dialect
	if dsn := os.Getenv("TEST_TASK_LOG_CLICKHOUSE_DSN"); dsn != "" {
		var err error
		logDB, err = gorm.Open(clickhouse.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("selfexport_%d_", time.Now().UnixNano())}})
		require.NoError(t, err)
		require.NoError(t, logDB.Set("gorm:table_options", "ENGINE=MergeTree() ORDER BY (created_at, request_id)").AutoMigrate(&model.Log{}))
		sqlDB, err := logDB.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		t.Cleanup(func() { require.NoError(t, logDB.Migrator().DropTable(&model.Log{})) })
		var version string
		require.NoError(t, logDB.Raw("SELECT version()").Scan(&version).Error)
		t.Logf("log database: clickhouse %s", version)
		logDialect = common.DatabaseTypeClickHouse
	}
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousQuotaPerUnit := common.QuotaPerUnit
	model.DB, model.LOG_DB = db, logDB
	common.SetDatabaseTypes(dialect, logDialect)
	common.QuotaPerUnit = 1 // Current settings must never reprice historical entries.
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	other := map[string]any{
		"quota_per_unit": 500000, "cache_tokens": 40, "group_ratio": 0.5,
		"channel_name": "CHANNEL_SECRET", "upstream_model_name": "UPSTREAM_SECRET",
		"admin_info":         map[string]any{"api_key": "DIAGNOSTIC_SECRET"},
		"billing_expression": "PRICE_SECRET", "model_price": 999,
	}
	logs := []*model.Log{
		{UserId: 7, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "=client-model", TokenName: "=token", Quota: 1000, PromptTokens: 100, CompletionTokens: 20, Other: common.MapToJsonStr(other)},
		{UserId: 7, CreatedAt: 101, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "=client-model", TokenName: "=token", Quota: 250, PromptTokens: 10, CompletionTokens: 5, Other: `{"quota_per_unit":500000}`},
		{UserId: 7, CreatedAt: 102, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "=client-model", TokenName: "=token", Quota: 100, PromptTokens: 100, CompletionTokens: 20, Other: `{"quota_per_unit":500000,"violation_fee":true,"cache_tokens":40}`},
		{UserId: 7, CreatedAt: 103, Type: model.LogTypeRefund, RequestId: "shared-request", ModelName: "=client-model", TokenName: "=token", Quota: 200, PromptTokens: 100, CompletionTokens: 20, Other: `{"quota_per_unit":500000,"cache_tokens":40}`},
		{UserId: 7, CreatedAt: 104, Type: model.LogTypeConsume, RequestId: "unknown-snapshot", ModelName: "=client-model", TokenName: "=token", Quota: 50, PromptTokens: 1},
		{UserId: 7, CreatedAt: 105, Type: model.LogTypeError, RequestId: "failed-request", ModelName: "DIAGNOSTIC_SECRET", Content: "DIAGNOSTIC_SECRET"},
		{UserId: 7, CreatedAt: 106, Type: model.LogTypeSystem, RequestId: "system-request", ModelName: "DIAGNOSTIC_SECRET"},
		{UserId: 7, CreatedAt: 107, Type: model.LogTypeTopup, RequestId: "topup-request", ModelName: "DIAGNOSTIC_SECRET"},
		{UserId: 7, CreatedAt: 300, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "DIAGNOSTIC_SECRET", Quota: 999999},
		{UserId: 8, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "OTHER_USER_SECRET", TokenName: "OTHER_USER_SECRET", Quota: 999999},
		{UserId: 8, CreatedAt: 103, Type: model.LogTypeRefund, RequestId: "shared-request", ModelName: "OTHER_USER_SECRET", Quota: 999999},
	}
	for _, entry := range logs {
		entry.Username = "USERNAME_SECRET"
		entry.ChannelId = 987654
		entry.UpstreamRequestId = "UPSTREAM_SECRET"
		entry.Group = "GROUP_SECRET"
		entry.Content = "DIAGNOSTIC_SECRET"
	}
	require.NoError(t, logDB.Create(&logs).Error)

	gin.SetMode(gin.TestMode)
	request := map[string]any{
		"start_timestamp": 90, "end_timestamp": 200,
		"user_id": 8, "username": "OTHER_USER_SECRET", "channel": 987654,
		"channel_id": 987654, "group": "OTHER_USER_SECRET", "upstream_request_id": "UPSTREAM_SECRET",
		"type": model.LogTypeError,
	}
	files := readSelfLogExportArchive(t, selfLogExportTestRequest(t, 7, request))
	assert.Equal(t, [][]string{
		{"created_at", "request_id", "consumption_type", "token_name", "model_name", "input_tokens", "cached_input_tokens", "output_tokens", "charged_quota", "actual_amount_usd"},
		{"1970-01-01T00:01:40Z", "shared-request", "consumption", "'=token", "'=client-model", "100", "40", "20", "1000", "0.002"},
		{"1970-01-01T00:01:41Z", "shared-request", "consumption", "'=token", "'=client-model", "10", "0", "5", "250", "0.0005"},
		{"1970-01-01T00:01:42Z", "shared-request", "violation_fee", "'=token", "'=client-model", "0", "0", "0", "100", "0.0002"},
		{"1970-01-01T00:01:43Z", "shared-request", "refund", "'=token", "'=client-model", "0", "0", "0", "-200", "-0.0004"},
		{"1970-01-01T00:01:44Z", "unknown-snapshot", "consumption", "'=token", "'=client-model", "1", "0", "0", "50", ""},
	}, files["self-details.csv"])
	assert.Equal(t, [][]string{
		{"model_name", "entry_count", "consumption_count", "violation_fee_count", "refund_count", "input_tokens", "cached_input_tokens", "output_tokens", "charged_quota", "known_actual_amount_usd", "unknown_amount_count"},
		{"'=client-model", "5", "3", "1", "1", "111", "40", "25", "1200", "0.0023", "1"},
	}, files["self-summary.csv"])

	for _, tc := range []struct {
		name    string
		filters map[string]any
		count   int
	}{
		{"shared request cannot expand to other users or dates", map[string]any{"request_id": "shared-request"}, 4},
		{"model filter", map[string]any{"model_name": "=client-model"}, 5},
		{"token filter", map[string]any{"token_name": "missing-token"}, 0},
		{"request filter", map[string]any{"request_id": "unknown-snapshot"}, 1},
		{"bounded date filter", map[string]any{"start_timestamp": 102, "end_timestamp": 103}, 2},
		{"other user model", map[string]any{"model_name": "OTHER_USER_SECRET"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filters := map[string]any{"view": "self", "start_timestamp": 90, "end_timestamp": 200}
			for key, value := range tc.filters {
				filters[key] = value
			}
			files := readSelfLogExportArchive(t, selfLogExportTestRequest(t, 7, filters))
			assert.Len(t, files["self-details.csv"], tc.count+1)
		})
	}
}

func TestExportSelfLogsRejectsUnsafeViewsFieldsAndMissingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, field := range []string{
		"user_id", "username", "token_id", "group", "status", "channel", "channel_id", "channel_name",
		"upstream_request_id", "upstream_model_name", "error_message", "content", "other", "ip",
		"model_price", "model_ratio", "completion_ratio", "cache_ratio", "group_ratio", "quota_per_unit", "original_amount_usd",
	} {
		t.Run(field, func(t *testing.T) {
			recorder := selfLogExportTestRequest(t, 7, map[string]any{"fields": []string{"request_id", field}})
			assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
			assert.Contains(t, recorder.Body.String(), `"success":false`)
			var archive bytes.Buffer
			assert.Error(t, service.WriteLogExportArchive(&archive, service.LogExportViewSelf, []string{field}, nil, nil))
			assert.Zero(t, archive.Len())
		})
	}
	for _, view := range []string{"upstream", "downstream", "unknown"} {
		t.Run(view, func(t *testing.T) {
			recorder := selfLogExportTestRequest(t, 7, map[string]any{"view": view})
			assert.Contains(t, recorder.Body.String(), `"success":false`)
			assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
		})
	}
	for _, userID := range []int{0, -1} {
		recorder := selfLogExportTestRequest(t, userID, map[string]any{"user_id": 7})
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
	}
	for _, request := range []map[string]any{
		{"fields": []string{"request_id", "request_id"}},
		{"start_timestamp": 200, "end_timestamp": 100},
	} {
		recorder := selfLogExportTestRequest(t, 7, request)
		assert.Contains(t, recorder.Body.String(), `"success":false`)
	}
}

func TestExportSelfLogsHistoricalAmountsAndSafeErrors(t *testing.T) {
	for _, prefix := range []string{"=", "+", "-", "@", "\t", "\r", "\n", "\x00"} {
		t.Run("CSV prefix "+prefix, func(t *testing.T) {
			text := prefix + "formula"
			rows := service.BuildLogExportRows(service.LogExportViewSelf, []*model.Log{{Type: model.LogTypeConsume, RequestId: text, ModelName: text, TokenName: text}})
			var archive bytes.Buffer
			require.NoError(t, service.WriteLogExportArchive(&archive, service.LogExportViewSelf, []string{"request_id", "model_name", "token_name"}, rows, service.BuildLogExportSummary(service.LogExportViewSelf, rows)))
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Type", "application/zip")
			recorder.Header().Set("Cache-Control", "private, no-store")
			_, err := recorder.Write(archive.Bytes())
			require.NoError(t, err)
			files := readSelfLogExportArchive(t, recorder)
			assert.Equal(t, []string{"'" + text, "'" + text, "'" + text}, files["self-details.csv"][1])
			assert.Equal(t, "'"+text, files["self-summary.csv"][1][0])
		})
	}
	for _, snapshot := range []string{"", `{}`, `{"quota_per_unit":0}`, `{"quota_per_unit":-1}`, `{"quota_per_unit":"invalid"}`, `{"quota_per_unit":1e309}`, `{"quota_per_unit":1e-320}`} {
		t.Run(snapshot, func(t *testing.T) {
			rows := service.BuildLogExportRows(service.LogExportViewSelf, []*model.Log{{Type: model.LogTypeConsume, Quota: 1000, Other: snapshot}})
			require.Len(t, rows, 1)
			assert.Nil(t, rows[0].ActualAmountUSD)
			summary := service.BuildLogExportSummary(service.LogExportViewSelf, rows)
			require.Len(t, summary, 1)
			assert.Equal(t, 1, summary[0].UnpricedRequestCount)
		})
	}
	db, _ := openTaskDialectDatabase(t, &model.Log{})
	previousLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previousLogDB })
	// Force a driver error with a private table name; only the safe API error may reach the user.
	model.LOG_DB = db.Table("CHANNEL_SECRET_PRIVATE_TABLE")
	recorder := selfLogExportTestRequest(t, 7, map[string]any{})
	assert.Contains(t, recorder.Body.String(), "unable to export consumption data")
	assert.NotContains(t, recorder.Body.String(), "CHANNEL_SECRET")
	assert.NotContains(t, recorder.Body.String(), "SELECT")
}

func TestExportSelfLogsDashboardAuthenticationAndAdminBoundary(t *testing.T) {
	db, dialect := openTaskDialectDatabase(t, &model.User{}, &model.UserSession{}, &model.Log{})
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousSearchLimit := common.RedisEnabled, common.SearchRateLimitEnable
	previousSecret := common.SessionSecret
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(dialect, dialect)
	common.RedisEnabled, common.SearchRateLimitEnable = false, false
	common.SessionSecret = "self-consumption-export-test-secret"
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.SearchRateLimitEnable = previousRedis, previousSearchLimit
		common.SessionSecret = previousSecret
	})
	user := &model.User{Username: "export-user", Password: "unused-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, AffCode: "export-user-aff"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&[]*model.Log{
		{UserId: user.Id, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "shared-request", Quota: 123},
		{UserId: user.Id + 1, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "OTHER_USER_SECRET", Quota: 999},
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/log/self/export", middleware.UserAuth(), middleware.DisableCache(), middleware.SearchRateLimit(), ExportSelfLogs)
	router.POST("/api/log/export", middleware.AdminAuth(), ExportLogs)

	var activeToken string
	for _, tc := range []struct {
		name      string
		status    string
		expiresAt int64
		want      int
	}{
		{"active", model.UserSessionStatusActive, time.Now().Add(time.Hour).Unix(), http.StatusOK},
		{"expired", model.UserSessionStatusActive, time.Now().Add(-time.Hour).Unix(), http.StatusUnauthorized},
		{"revoked", model.UserSessionStatusRevoked, time.Now().Add(time.Hour).Unix(), http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &model.UserSession{SID: "export-session-" + tc.name, UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: tc.status, RefreshHash: strings.Repeat("x", 64), LoginMethod: "password", ExpiresAt: tc.expiresAt}
			require.NoError(t, db.Create(session).Error)
			token, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1})
			require.NoError(t, err)
			if tc.name == "active" {
				activeToken = token
			}
			request := httptest.NewRequest(http.MethodPost, "/api/log/self/export", strings.NewReader(`{"request_id":"shared-request","user_id":999}`))
			request.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.want, recorder.Code, recorder.Body.String())
			if tc.want == http.StatusOK {
				files := readSelfLogExportArchive(t, recorder)
				require.Len(t, files["self-details.csv"], 2)
				assert.Equal(t, "123", files["self-details.csv"][1][8])
			} else {
				assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
			}
		})
	}
	for _, token := range []string{"", "invalid-token", activeToken[:len(activeToken)-2] + "!!"} {
		request := httptest.NewRequest(http.MethodPost, "/api/log/self/export", strings.NewReader(`{}`))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
	}
	request := httptest.NewRequest(http.MethodPost, "/api/log/export", strings.NewReader(`{"view":"upstream"}`))
	request.Header.Set("Authorization", "Bearer "+activeToken)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
}

func TestExportLogsDownstreamUsesTrueTerminalRecordBeforeReportFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "controller-log-export.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY, name TEXT)").Error)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	logs := []*model.Log{
		{Id: 1, CreatedAt: 100, Type: model.LogTypeError, RequestId: "req-success", ChannelId: 10, ModelName: "gpt-x"},
		{Id: 2, CreatedAt: 101, Type: model.LogTypeConsume, RequestId: "req-success", ChannelId: 12, ModelName: "gpt-x"},
		{Id: 3, CreatedAt: 150, Type: model.LogTypeError, RequestId: "req-outside", ChannelId: 10, ModelName: "gpt-x"},
		{Id: 4, CreatedAt: 250, Type: model.LogTypeConsume, RequestId: "req-outside", ChannelId: 12, ModelName: "gpt-x"},
	}
	require.NoError(t, db.Create(&logs).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/log/export", strings.NewReader(`{"view":"downstream","fields":["request_id","status"],"start_timestamp":90,"end_timestamp":200,"channel":10}`))
	context.Request.Header.Set("Content-Type", "application/json")

	ExportLogs(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/zip", recorder.Header().Get("Content-Type"))
	archive, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	require.NoError(t, err)
	require.NotEmpty(t, archive.File)
	detail, err := archive.File[0].Open()
	require.NoError(t, err)
	defer detail.Close()
	detailBytes, err := io.ReadAll(detail)
	require.NoError(t, err)
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(detailBytes, []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"request_id", "status"}, {"req-success", "success"}}, records)
}
