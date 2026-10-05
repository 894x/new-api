package router

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestListModelsSupportsOpenAIAndGeminiAuthentication(t *testing.T) {
	setupRelayRouterTestDB(t)

	user := model.User{
		Username: "models-user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "modelstestkey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	engine := gin.New()
	SetRelayRouter(engine)

	tests := []struct {
		name           string
		path           string
		headerName     string
		expectedObject string
		expectedField  string
	}{
		{
			name:           "OpenAI bearer token",
			path:           "/v1/models",
			headerName:     "Authorization",
			expectedObject: "list",
			expectedField:  "data",
		},
		{
			name:          "Gemini API key header",
			path:          "/v1/models",
			headerName:    "x-goog-api-key",
			expectedField: "models",
		},
		{
			name:          "Gemini API key query",
			path:          "/v1/models?key=modelstestkey",
			expectedField: "models",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.headerName != "" {
				value := "modelstestkey"
				if test.headerName == "Authorization" {
					value = "Bearer " + value
				}
				request.Header.Set(test.headerName, value)
			}

			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Contains(t, payload, test.expectedField)
			assert.NotContains(t, payload, "error")
			if test.expectedObject != "" {
				assert.Equal(t, test.expectedObject, payload["object"])
			}
		})
	}
}

func TestRelayRouterDisablesClaudeTokenCountingEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)

	for _, route := range engine.Routes() {
		assert.False(t, route.Method == http.MethodPost && route.Path == "/v1/messages/count_tokens")
	}
}

func setupRelayRouterTestDB(t *testing.T) func(*testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalIsMasterNode := common.IsMasterNode
	originalRedisEnabled := common.RedisEnabled
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	testDSN := "local"
	if dialect := os.Getenv("TEST_RELAY_DIALECT"); dialect != "" && dialect != "sqlite" {
		name := fmt.Sprintf("newapi_discount_relay_%d", time.Now().UnixNano())
		var driver gorm.Dialector
		switch dialect {
		case "mysql":
			dsn := os.Getenv("TEST_MYSQL_DSN")
			require.NotEmpty(t, dsn)
			parsed, err := sqlmysql.ParseDSN(dsn)
			require.NoError(t, err)
			require.Equal(t, "tcp", parsed.Net)
			host, _, err := net.SplitHostPort(parsed.Addr)
			require.NoError(t, err)
			require.True(t, net.ParseIP(host).IsLoopback(), "external relay tests require a disposable loopback instance")
			driver = mysql.Open(dsn)
			parsed.DBName = name
			testDSN = parsed.FormatDSN()
		case "postgres":
			dsn := os.Getenv("TEST_POSTGRES_DSN")
			require.NotEmpty(t, dsn)
			parsed, err := url.Parse(dsn)
			require.NoError(t, err)
			require.True(t, net.ParseIP(parsed.Hostname()).IsLoopback(), "external relay tests require a disposable loopback instance")
			driver = postgres.Open(dsn)
			parsed.Path = "/" + name
			testDSN = parsed.String()
		default:
			t.Fatalf("unsupported relay test dialect: %s", dialect)
		}
		admin, err := gorm.Open(driver, &gorm.Config{})
		require.NoError(t, err)
		create := "CREATE DATABASE " + name
		if dialect == "mysql" {
			create += " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
		}
		require.NoError(t, admin.Exec(create).Error)
		// Only the newly created, task-owned database is removed. Register this
		// before connection cleanup so refunds finish and connections close first.
		t.Cleanup(func() {
			assert.NoError(t, admin.Exec("DROP DATABASE "+name).Error)
			connection, err := admin.DB()
			require.NoError(t, err)
			assert.NoError(t, connection.Close())
		})
	}
	require.NoError(t, os.Setenv("SQL_DSN", testDSN))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.Ability{},
		&model.UserChannelRoutingOverride{},
		&model.BillingAdmissionReserveOperation{},
		&model.BillingRefundOperation{},
	))
	testDB := model.DB
	versionQuery := "SELECT version()"
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, testDB.Raw(versionQuery).Scan(&version).Error)
	t.Logf("relay database: %s %s", common.MainDatabaseType(), version)
	var completedRefundReads sync.Map
	const refundCallback = "test:relay_refund_final_read"
	// Status persistence precedes the reconciler's final read. Observe that
	// read so cleanup cannot close the database while it is still needed.
	require.NoError(t, testDB.Callback().Query().After("gorm:query").Register(refundCallback, func(tx *gorm.DB) {
		operation, ok := tx.Statement.Dest.(*model.BillingRefundOperation)
		if tx.Error == nil && ok && operation.Status == model.BillingRefundStatusApplied {
			completedRefundReads.Store(operation.OperationID, true)
		}
	}))
	waitForRefunds := func(t *testing.T) {
		t.Helper()
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			var refunds []model.BillingRefundOperation
			if !assert.NoError(collect, testDB.Find(&refunds).Error) {
				return
			}
			for _, refund := range refunds {
				assert.Equal(collect, model.BillingRefundStatusApplied, refund.Status, "refund %s", refund.OperationID)
				_, observed := completedRefundReads.Load(refund.OperationID)
				assert.True(collect, observed, "refund %s has not finished its database work", refund.OperationID)
			}
		}, 3*time.Second, 5*time.Millisecond)
	}

	t.Cleanup(func() {
		waitForRefunds(t)
		require.NoError(t, testDB.Callback().Query().Remove(refundCallback))
		if sqlDB, err := testDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		common.IsMasterNode = originalIsMasterNode
		common.RedisEnabled = originalRedisEnabled
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
	return waitForRefunds
}
