package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestSanitizeUserErrorLogsHidesProviderDetails(t *testing.T) {
	original := operation_setting.GetErrorSetting().HideErrorDetails
	operation_setting.UpdateHideErrorDetails(true)
	t.Cleanup(func() {
		operation_setting.UpdateHideErrorDetails(original)
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", common.RoleCommonUser)
	logs := []*model.Log{
		{
			Type:              model.LogTypeError,
			Content:           "status_code=500, xunfei response error",
			RequestId:         "request-log",
			UpstreamRequestId: "upstream-secret",
			Other: common.MapToJsonStr(map[string]any{
				"error_code":   "11200",
				"channel_name": "from xunfei",
				"status_code":  500,
			}),
		},
	}

	sanitizeUserErrorLogs(c, logs)

	assert.Contains(t, logs[0].Content, "request-log")
	assert.NotContains(t, logs[0].Content, "xunfei")
	assert.Empty(t, logs[0].UpstreamRequestId)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, other, "error_code")
	assert.NotContains(t, other, "channel_name")
	assert.EqualValues(t, 500, other["status_code"])
}

func TestSanitizeUserErrorLogsKeepsAdministratorDetails(t *testing.T) {
	original := operation_setting.GetErrorSetting().HideErrorDetails
	operation_setting.UpdateHideErrorDetails(true)
	t.Cleanup(func() {
		operation_setting.UpdateHideErrorDetails(original)
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", common.RoleAdminUser)
	logs := []*model.Log{{
		Type:              model.LogTypeError,
		Content:           "full upstream error",
		UpstreamRequestId: "upstream-request",
	}}

	sanitizeUserErrorLogs(c, logs)

	assert.Equal(t, "full upstream error", logs[0].Content)
	assert.Equal(t, "upstream-request", logs[0].UpstreamRequestId)
}

func TestGetAllLogsStatusCodeFilter(t *testing.T) {
	logDB, logDialect := openTaskDialectDatabase(t, &model.Log{})
	if dsn := os.Getenv("TEST_TASK_LOG_CLICKHOUSE_DSN"); dsn != "" {
		var err error
		logDB, err = gorm.Open(clickhouse.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("statusfilter_%d_", time.Now().UnixNano())}})
		require.NoError(t, err)
		sqlDB, err := logDB.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		require.NoError(t, logDB.Set("gorm:table_options", "ENGINE=MergeTree() ORDER BY (created_at, request_id)").AutoMigrate(&model.Log{}))
		t.Cleanup(func() { require.NoError(t, logDB.Migrator().DropTable(&model.Log{})) })
		logDialect = common.DatabaseTypeClickHouse
		var version string
		require.NoError(t, logDB.Raw("SELECT version()").Scan(&version).Error)
		t.Logf("log database: clickhouse %s", version)
	}
	// Keep the primary database on SQLite to verify log-database dialect selection.
	mainDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	mainSQLDB, err := mainDB.DB()
	require.NoError(t, err)
	mainSQLDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, mainSQLDB.Close()) })
	require.NoError(t, mainDB.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY, name TEXT)").Error)
	require.NoError(t, mainDB.Exec("INSERT INTO channels (id, name) VALUES (7, 'test-channel')").Error)
	statement := &gorm.Statement{DB: logDB}
	require.NoError(t, statement.Parse(&model.Log{}))
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousMemoryCache := common.MemoryCacheEnabled
	model.DB, model.LOG_DB = mainDB, logDB.Table(statement.Schema.Table+" AS logs").Session(&gorm.Session{})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, logDialect)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.MemoryCacheEnabled = previousMemoryCache
	})

	rows := []model.Log{
		{RequestId: "numeric", Other: `{"status_code":429,"client_status_code":200}`},
		{RequestId: "string", Other: `{ "status_code" : "429" }`},
		{RequestId: "other-status", Other: `{"status_code":500,"admin_info":{"status_code":429}}`},
		{RequestId: "missing", Other: `{"client_status_code":429,"admin_info":{"status_code":429}}`},
		{RequestId: "empty", Type: model.LogTypeConsume},
		{RequestId: "whitespace", Other: " "},
		{RequestId: "null", Other: `{"status_code":null}`},
		{RequestId: "longer", Other: `{"status_code":4290}`},
		{RequestId: "recorded-ok", Type: model.LogTypeConsume, Other: `{"status_code":200}`},
		{RequestId: "other-user", Username: "bob", Other: `{"status_code":429}`},
		{RequestId: "other-model", ModelName: "model-b", Other: `{"status_code":429}`},
		{RequestId: "non-error", Type: model.LogTypeConsume, Other: `{"status_code":429}`},
		{RequestId: "nested-string", Other: `{"detail":"{\"status_code\":429}"}`},
		{RequestId: "boolean", Other: `{"status_code":true}`},
	}
	allRequestIDs := make([]string, len(rows))
	for i := range rows {
		rows[i].CreatedAt = int64(100 + i)
		rows[i].ChannelId = 7
		rows[i].TokenName = "key"
		rows[i].UpstreamRequestId = "up-" + rows[i].RequestId
		if rows[i].Type == 0 {
			rows[i].Type = model.LogTypeError
		}
		if rows[i].Username == "" {
			rows[i].Username = "alice"
		}
		if rows[i].ModelName == "" {
			rows[i].ModelName = "model-a"
		}
		allRequestIDs[len(rows)-1-i] = rows[i].RequestId
	}
	require.NoError(t, logDB.Create(&rows).Error)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("role", common.RoleAdminUser) })
	router.GET("/api/log/", GetAllLogs)
	const filters = "username=alice&model_name=model-a&token_name=key&channel=7&start_timestamp=100&end_timestamp=200"
	for _, tc := range []struct {
		name  string
		query string
		total int
		want  []string
	}{
		{name: "omitted", query: "page_size=100", total: len(rows), want: allRequestIDs},
		{name: "zero", query: "status_code=0&page_size=100", total: len(rows), want: allRequestIDs},
		{name: "empty", query: "status_code=&page_size=100", total: len(rows), want: allRequestIDs},
		{name: "first page", query: "status_code=429&page_size=2", total: 5, want: []string{"non-error", "other-model"}},
		{name: "second page", query: "status_code=429&page_size=2&p=2", total: 5, want: []string{"other-user", "string"}},
		{name: "last page", query: "status_code=429&page_size=2&p=3", total: 5, want: []string{"numeric"}},
		{name: "combined filters", query: filters + "&status_code=429", total: 3, want: []string{"non-error", "string", "numeric"}},
		{name: "error type", query: filters + "&status_code=429&type=5", total: 2, want: []string{"string", "numeric"}},
		{name: "explicit wildcard", query: "status_code=429&username=alice&model_name=model-%25", total: 4, want: []string{"non-error", "other-model", "string", "numeric"}},
		{name: "request IDs", query: "status_code=429&request_id=numeric&upstream_request_id=up-numeric", total: 1, want: []string{"numeric"}},
		{name: "recorded success only", query: "status_code=200", total: 1, want: []string{"recorded-ok"}},
		{name: "other status", query: "status_code=500", total: 1, want: []string{"other-status"}},
		{name: "no matches", query: "status_code=499", total: 0, want: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/log/?"+tc.query, nil))
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
				Data    struct {
					Total int         `json:"total"`
					Items []model.Log `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, response.Message)
			assert.Equal(t, tc.total, response.Data.Total)
			requestIDs := make([]string, len(response.Data.Items))
			for i, item := range response.Data.Items {
				requestIDs[i] = item.RequestId
				assert.Equal(t, "test-channel", item.ChannelName)
			}
			assert.Equal(t, tc.want, requestIDs)
		})
	}
	for _, value := range []string{"abc", "99", "600", "-1", "4xx", "429,500", "9999999999999999999999"} {
		t.Run("invalid/"+value, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/log/?status_code="+value, nil))
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.JSONEq(t, `{"success":false,"message":"Invalid HTTP status code"}`, recorder.Body.String())
		})
	}
}
