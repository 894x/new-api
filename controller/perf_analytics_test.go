package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type perfAnalyticsMetricResponse struct {
	P50Ms       int64 `json:"p50_ms"`
	P90Ms       int64 `json:"p90_ms"`
	P99Ms       int64 `json:"p99_ms"`
	SampleCount int64 `json:"sample_count"`
}

type perfAnalyticsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		EffectiveStartTs int64 `json:"effective_start_timestamp"`
		EffectiveEndTs   int64 `json:"effective_end_timestamp"`
		Summary          struct {
			RequestCount int64                       `json:"request_count"`
			SuccessRate  float64                     `json:"success_rate"`
			Rpm          float64                     `json:"rpm"`
			Tpm          float64                     `json:"tpm"`
			CacheHitRate float64                     `json:"cache_hit_rate"`
			Ttft         perfAnalyticsMetricResponse `json:"ttft"`
			Tpot         perfAnalyticsMetricResponse `json:"tpot"`
		} `json:"summary"`
		Series []struct {
			Ts           int64   `json:"ts"`
			RequestCount int64   `json:"request_count"`
			Rpm          float64 `json:"rpm"`
			Tpm          float64 `json:"tpm"`
			CacheHitRate float64 `json:"cache_hit_rate"`
		} `json:"series"`
	} `json:"data"`
}

type perfAnalyticsOptionsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Models []string `json:"models"`
		Users  []struct {
			Id       int    `json:"id"`
			Username string `json:"username"`
		} `json:"users"`
		Tokens []struct {
			Id     int    `json:"id"`
			UserId int    `json:"user_id"`
			Name   string `json:"name"`
		} `json:"tokens"`
	} `json:"data"`
}

func setupPerfAnalyticsControllerTestDB(t *testing.T) {
	t.Helper()
	previousBucketTime := perf_metrics_setting.GetSetting().BucketTime
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.bucket_time": "5min"}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.bucket_time": previousBucketTime}))
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}))
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))

	require.NoError(t, db.Create(&[]model.Token{
		{Id: 11, UserId: 1, Key: "self-key", Name: "Self key"},
		{Id: 22, UserId: 2, Key: "other-key", Name: "Other key"},
	}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "alice", Password: "password", Status: 1, Role: 1, AffCode: "alice-code"},
		{Id: 2, Username: "bob", Password: "password", Status: 1, Role: 1, AffCode: "bob-code"},
	}).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO perf_metric_details
			(user_id, token_id, model_name, bucket_ts, request_count, success_count, ttft_count, tpot_count, input_tokens, output_tokens, total_tokens, cache_read_tokens)
		VALUES
			(1, 11, 'gpt-test', 1000, 10, 9, 9, 8, 10000, 5000, 15000, 2500),
			(2, 22, 'gpt-test', 1000, 4, 4, 4, 4, 4000, 2000, 6000, 1000),
			(2, 22, 'claude-test', 1000, 2, 2, 2, 2, 2000, 800, 2800, 300)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO perf_metric_histograms
			(user_id, token_id, model_name, bucket_ts, metric, upper_bound_ms, count)
		VALUES
			(1, 11, 'gpt-test', 1000, 'ttft', 100, 5),
			(1, 11, 'gpt-test', 1000, 'ttft', 500, 3),
			(1, 11, 'gpt-test', 1000, 'ttft', 1000, 1),
			(1, 11, 'gpt-test', 1000, 'tpot', 20, 4),
			(1, 11, 'gpt-test', 1000, 'tpot', 40, 3),
			(1, 11, 'gpt-test', 1000, 'tpot', 80, 1),
			(2, 22, 'gpt-test', 1000, 'ttft', 2000, 4),
			(2, 22, 'gpt-test', 1000, 'tpot', 160, 4)
	`).Error)
}

func performPerfAnalyticsOptionsRequest(
	t *testing.T,
	handler gin.HandlerFunc,
	userID int,
	rawQuery string,
) (*httptest.ResponseRecorder, perfAnalyticsOptionsResponse) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	ctx.Set("id", userID)
	handler(ctx)

	var response perfAnalyticsOptionsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return recorder, response
}

func performPerfAnalyticsRequest(
	t *testing.T,
	handler gin.HandlerFunc,
	userID int,
	rawQuery string,
) (*httptest.ResponseRecorder, perfAnalyticsResponse) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	ctx.Set("id", userID)
	handler(ctx)

	var response perfAnalyticsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return recorder, response
}

func TestGetPerfAnalyticsSelfIgnoresRequestedUserAndScopesToAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, response := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsSelf,
		1,
		"model=gpt-test&user_id=2&start_timestamp=900&end_timestamp=2000",
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, response.Success)
	assert.Equal(t, int64(900), response.Data.EffectiveStartTs)
	assert.Equal(t, int64(2099), response.Data.EffectiveEndTs)
	assert.Equal(t, int64(10), response.Data.Summary.RequestCount)
	assert.InDelta(t, 90, response.Data.Summary.SuccessRate, 0.001)
	assert.Equal(t, 0.5, response.Data.Summary.Rpm)
	assert.Equal(t, 750.0, response.Data.Summary.Tpm)
	assert.Equal(t, 25.0, response.Data.Summary.CacheHitRate)
	assert.Equal(t, perfAnalyticsMetricResponse{P50Ms: 100, P90Ms: 1000, P99Ms: 1000, SampleCount: 9}, response.Data.Summary.Ttft)
	assert.Equal(t, perfAnalyticsMetricResponse{P50Ms: 20, P90Ms: 80, P99Ms: 80, SampleCount: 8}, response.Data.Summary.Tpot)
	require.Len(t, response.Data.Series, 1)
	assert.Equal(t, int64(10), response.Data.Series[0].RequestCount)
	assert.Equal(t, 2.0, response.Data.Series[0].Rpm)
	assert.Equal(t, 3000.0, response.Data.Series[0].Tpm)
	assert.Equal(t, 25.0, response.Data.Series[0].CacheHitRate)
}

func TestGetPerfAnalyticsSelfRejectsTokenOwnedByAnotherUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, _ := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsSelf,
		1,
		"model=gpt-test&token_id=22&start_timestamp=900&end_timestamp=2000",
	)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestGetPerfAnalyticsAdminCanAggregateEveryUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, response := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsAdmin,
		99,
		"model=gpt-test&start_timestamp=900&end_timestamp=2000",
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, int64(14), response.Data.Summary.RequestCount)
	assert.Equal(t, int64(13), response.Data.Summary.Ttft.SampleCount)
	assert.Equal(t, int64(12), response.Data.Summary.Tpot.SampleCount)
	assert.Equal(t, 0.7, response.Data.Summary.Rpm)
	assert.Equal(t, 1050.0, response.Data.Summary.Tpm)
	assert.Equal(t, 25.0, response.Data.Summary.CacheHitRate)
}

func TestGetPerfAnalyticsAdminFiltersUserAndValidatesTokenOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, response := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsAdmin,
		99,
		"model=gpt-test&user_id=2&token_id=22&start_timestamp=900&end_timestamp=2000",
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, int64(4), response.Data.Summary.RequestCount)
	assert.Equal(t, int64(2000), response.Data.Summary.Ttft.P50Ms)
	assert.Equal(t, int64(160), response.Data.Summary.Tpot.P50Ms)

	invalidRecorder, _ := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsAdmin,
		99,
		"model=gpt-test&user_id=1&token_id=22&start_timestamp=900&end_timestamp=2000",
	)
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
}

func TestGetPerfAnalyticsSelfOptionsOnlyExposeAuthenticatedUsersModelsAndKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, response := performPerfAnalyticsOptionsRequest(
		t,
		GetPerfAnalyticsSelfOptions,
		1,
		"user_id=2",
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, []string{"gpt-test"}, response.Data.Models)
	assert.Empty(t, response.Data.Users)
	require.Len(t, response.Data.Tokens, 1)
	assert.Equal(t, 11, response.Data.Tokens[0].Id)
	assert.Equal(t, 1, response.Data.Tokens[0].UserId)
}

func TestGetPerfAnalyticsAdminOptionsExposeAllUsersAndCascadeBySelectedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	allRecorder, allResponse := performPerfAnalyticsOptionsRequest(
		t,
		GetPerfAnalyticsAdminOptions,
		99,
		"",
	)
	require.Equal(t, http.StatusOK, allRecorder.Code)
	assert.Equal(t, []string{"claude-test", "gpt-test"}, allResponse.Data.Models)
	require.Len(t, allResponse.Data.Users, 2)
	assert.Equal(t, "alice", allResponse.Data.Users[0].Username)
	assert.Equal(t, "bob", allResponse.Data.Users[1].Username)
	assert.Empty(t, allResponse.Data.Tokens)

	userRecorder, userResponse := performPerfAnalyticsOptionsRequest(
		t,
		GetPerfAnalyticsAdminOptions,
		99,
		"user_id=2",
	)
	require.Equal(t, http.StatusOK, userRecorder.Code)
	assert.Equal(t, []string{"claude-test", "gpt-test"}, userResponse.Data.Models)
	require.Len(t, userResponse.Data.Tokens, 1)
	assert.Equal(t, 22, userResponse.Data.Tokens[0].Id)
	assert.Equal(t, 2, userResponse.Data.Tokens[0].UserId)
}

// Each configured DSN names a disposable database created for this test run.
// The matrix exercises both primary options and the separately configured log DB.
func TestPerfAnalyticsChannelFilterDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var primary, logs gorm.Dialector
			switch dialect {
			case "sqlite":
				primary = sqlite.Open(fmt.Sprintf("file:%s-main?mode=memory&cache=shared", t.Name()))
				logs = sqlite.Open(fmt.Sprintf("file:%s-logs?mode=memory&cache=shared", t.Name()))
			case "mysql":
				dsn, logDSN := os.Getenv("TEST_PERF_CHANNEL_MYSQL_DSN"), os.Getenv("TEST_PERF_CHANNEL_MYSQL_LOG_DSN")
				if dsn == "" || logDSN == "" {
					t.Skip("dedicated MySQL test databases are not configured")
				}
				primary, logs = mysql.Open(dsn), mysql.Open(logDSN)
			case "postgres":
				dsn, logDSN := os.Getenv("TEST_PERF_CHANNEL_POSTGRES_DSN"), os.Getenv("TEST_PERF_CHANNEL_POSTGRES_LOG_DSN")
				if dsn == "" || logDSN == "" {
					t.Skip("dedicated PostgreSQL test databases are not configured")
				}
				primary, logs = postgres.Open(dsn), postgres.Open(logDSN)
			}
			db, err := gorm.Open(primary, &gorm.Config{})
			require.NoError(t, err)
			logDB, err := gorm.Open(logs, &gorm.Config{})
			require.NoError(t, err)
			var version string
			versionQuery := "SELECT VERSION()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			require.NoError(t, logDB.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database=%s", version)
			oldDB, oldLogDB := model.DB, model.LOG_DB
			oldType := common.MainDatabaseType()
			switch dialect {
			case "sqlite":
				common.SetMainDatabaseType(common.DatabaseTypeSQLite)
			case "mysql":
				common.SetMainDatabaseType(common.DatabaseTypeMySQL)
			case "postgres":
				common.SetMainDatabaseType(common.DatabaseTypePostgreSQL)
			}
			model.DB, model.LOG_DB = db, logDB
			t.Cleanup(func() {
				model.DB, model.LOG_DB = oldDB, oldLogDB
				common.SetMainDatabaseType(oldType)
				sqlDB, _ := db.DB()
				_ = sqlDB.Close()
				sqlLogs, _ := logDB.DB()
				_ = sqlLogs.Close()
			})
			// Refuse destructive test resets outside explicitly named disposable databases.
			if dialect != "sqlite" {
				var database string
				query := "SELECT DATABASE()"
				if dialect == "postgres" {
					query = "SELECT current_database()"
				}
				require.NoError(t, db.Raw(query).Scan(&database).Error)
				require.Contains(t, database, "codex_perf_channel_")
				require.NoError(t, logDB.Raw(query).Scan(&database).Error)
				require.Contains(t, database, "codex_perf_channel_")
			}
			require.NoError(t, db.Migrator().DropTable(&model.PerfMetricHistogram{}, &model.PerfMetricDetail{}, &model.Channel{}, &model.Token{}, &model.User{}))
			require.NoError(t, logDB.Migrator().DropTable(&model.Log{}))
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
			require.NoError(t, logDB.AutoMigrate(&model.Log{}))
			require.NoError(t, db.Create(&[]model.Channel{{Id: 7, Name: "Channel seven", Key: "must-not-leak"}, {Id: 8, Name: "Channel eight", Key: "private"}}).Error)
			require.NoError(t, db.Create(&[]model.User{{Id: 1, Username: "alice", AffCode: "channel-alice"}, {Id: 2, Username: "bob", AffCode: "channel-bob"}}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 11, UserId: 1, Name: "Self key", Key: "channel-test-key"}).Error)
			require.NoError(t, db.Create(&model.PerfMetricDetail{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, RequestCount: 1, SuccessCount: 1}).Error)
			precise := `{"frt":641.25,"cache_tokens":25,"cache_creation_tokens":4,"cache_creation_tokens_5m":4,"admin_info":{"request_timing":{"first_response_at_ms":1000000,"request_completed_at_ms":1000041},"upstream_transport":[{"body_bytes":99999}]}}`
			require.NoError(t, logDB.Create(&[]model.Log{
				{Id: 1, UserId: 1, TokenId: 11, ChannelId: 7, ModelName: "gpt-test", CreatedAt: 1000, Type: model.LogTypeConsume, PromptTokens: 100, CompletionTokens: 5, IsStream: true, Other: precise},
				{Id: 2, UserId: 2, TokenId: 22, ChannelId: 7, ModelName: "gpt-test", CreatedAt: 1010, Type: model.LogTypeConsume, PromptTokens: 200, CompletionTokens: 2, IsStream: false, Other: `{"cache_tokens":0,"cache_creation_tokens":0}`},
				{Id: 3, UserId: 1, TokenId: 11, ChannelId: 7, ModelName: "gpt-test", CreatedAt: 1020, Type: model.LogTypeError, Other: `{"error_code":"busy","status_code":429}`},
				{Id: 4, UserId: 1, TokenId: 11, ChannelId: 8, ModelName: "gpt-test", CreatedAt: 1000, Type: model.LogTypeConsume, PromptTokens: 9999, Other: precise},
				{Id: 5, UserId: 1, TokenId: 11, ChannelId: 7, ModelName: "other-model", CreatedAt: 1000, Type: model.LogTypeConsume, Other: precise},
				{Id: 6, UserId: 1, TokenId: 11, ChannelId: 7, ModelName: "gpt-test", CreatedAt: 899, Type: model.LogTypeConsume, Other: precise},
				{Id: 7, UserId: 1, TokenId: 11, ChannelId: 7, ModelName: "gpt-test", CreatedAt: 1200, Type: model.LogTypeConsume, Other: precise},
			}).Error)
			// The admin's channel picker exposes identifiers and names, never keys.
			adminRecorder, adminOptions := performPerfAnalyticsOptionsRequest(t, GetPerfAnalyticsAdminOptions, 99, "")
			require.Equal(t, http.StatusOK, adminRecorder.Code)
			var options map[string]any
			require.NoError(t, common.Unmarshal(adminRecorder.Body.Bytes(), &options))
			data := options["data"].(map[string]any)
			require.Len(t, data["channels"], 2)
			assert.NotContains(t, adminRecorder.Body.String(), "must-not-leak")
			assert.Equal(t, []string{"gpt-test"}, adminOptions.Data.Models)
			selfRecorder, _ := performPerfAnalyticsOptionsRequest(t, GetPerfAnalyticsSelfOptions, 1, "")
			assert.NotContains(t, selfRecorder.Body.String(), `"channels"`)
			// Forging the channel parameter through the ordinary user's endpoint is denied.
			denied, _ := performPerfAnalyticsRequest(t, GetPerfAnalyticsSelf, 1, "model=gpt-test&channel_id=7&start_timestamp=900&end_timestamp=1200")
			assert.Equal(t, http.StatusForbidden, denied.Code)
			allRecorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(allRecorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/?model=gpt-test&channel_id=7&start_timestamp=900&end_timestamp=1200", nil)
			GetPerfAnalyticsAdmin(c)
			require.Equal(t, http.StatusOK, allRecorder.Code, allRecorder.Body.String())
			var all struct {
				Success bool                             `json:"success"`
				Data    perfmetrics.ChannelMetricsResult `json:"data"`
			}
			require.NoError(t, common.Unmarshal(allRecorder.Body.Bytes(), &all))
			require.True(t, all.Success)
			assert.Equal(t, "usage_logs", all.Data.Source)
			assert.Equal(t, int64(3), all.Data.Summary.RequestCount)
			assert.Equal(t, int64(1), all.Data.Summary.ErrorCount)
			assert.Equal(t, 641.25, all.Data.Summary.Ttft.P50Ms)
			assert.Equal(t, 10.25, all.Data.Summary.Tpot.P50Ms)
			require.NotNil(t, all.Data.Summary.Ttft.MaxMs)
			assert.Equal(t, 641.25, *all.Data.Summary.Ttft.MaxMs)
			assert.Equal(t, 1, all.Data.Summary.Ttft.SampleCount)
			assert.Equal(t, 1, all.Data.Summary.Tpot.SampleCount)
			assert.Nil(t, all.Data.Summary.RequestBodyAvgBytes)
			assert.Equal(t, int64(0), all.Data.Summary.RequestBodySamples)
			assert.Equal(t, int64(2), all.Data.Summary.CacheReadSamples)
			assert.Equal(t, int64(2), all.Data.Summary.CacheWriteSamples)
			assert.Equal(t, int64(4), all.Data.Summary.CacheWriteTokens)
			assert.Equal(t, 2, all.Data.Summary.MetricPercentiles["cache_hit_rate"].SampleCount)
			assert.Equal(t, 0.0, all.Data.Summary.MetricPercentiles["cache_hit_rate"].P50)
			require.NotNil(t, all.Data.Summary.MetricPercentiles["cache_hit_rate"].Max)
			assert.Equal(t, 25.0, *all.Data.Summary.MetricPercentiles["cache_hit_rate"].Max)
			assert.Equal(t, 8.33, all.Data.Summary.CacheHitRate)
			assert.Equal(t, 0.0, all.Data.Summary.MetricPercentiles["cache_write_tokens"].P50)

			assert.Equal(t, 61.4, all.Data.Summary.Tpm)
			require.Len(t, all.Data.ErrorGroups, 1)
			assert.Equal(t, 429, all.Data.ErrorGroups[0].StatusCode)
			// The new sums/counts use existing histogram rows, with no schema change.
			// Exercise additive persistence and the read snapshot in every real dialect.
			stored := model.PerfMetricDetail{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, RequestCount: 2, SuccessCount: 2}
			histograms := []model.PerfMetricHistogram{
				{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, Metric: "ttft", UpperBoundMs: 750, Count: 2},
				{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, Metric: "ttft_sum", Count: 1283},
				{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, Metric: "ttft_n", Count: 2},
				{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, Metric: "tpot", UpperBoundMs: 10, Count: 2},
				{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, Metric: "tpot_sum", Count: 26},
				{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, Metric: "tpot_n", Count: 2},
				{UserId: 1, TokenId: 11, ModelName: "gpt-test", BucketTs: 900, Metric: "tpot_tok", Count: 10},
			}
			for range 2 {
				require.NoError(t, model.UpsertPerfAnalyticsBucket(&stored, histograms))
			}
			persisted, err := perfmetrics.QueryAnalytics(perfmetrics.AnalyticsQueryParams{Context: context.Background(), Model: "gpt-test", UserId: 1, TokenId: 11, StartTs: 900, EndTs: 1200})
			require.NoError(t, err)
			require.NotNil(t, persisted.Summary.Ttft.AvgMs)
			require.NotNil(t, persisted.Summary.Tpot.AvgMs)
			assert.Equal(t, 641.5, *persisted.Summary.Ttft.AvgMs)
			assert.Equal(t, 2.6, *persisted.Summary.Tpot.AvgMs)
			assert.Equal(t, int64(4), persisted.Summary.Ttft.SampleCount)
			assert.Equal(t, int64(4), persisted.Summary.Tpot.SampleCount)
			// User and key restrictions compose with the channel restriction.
			filtered, err := perfmetrics.QueryChannelMetrics(7, perfmetrics.AnalyticsQueryParams{Context: context.Background(), Model: "gpt-test", UserId: 1, TokenId: 11, StartTs: 900, EndTs: 1200})
			require.NoError(t, err)
			assert.Equal(t, int64(2), filtered.Summary.RequestCount)
			assert.Equal(t, 21.0, filtered.Summary.Tpm)
			partial, err := perfmetrics.QueryChannelMetrics(7, perfmetrics.AnalyticsQueryParams{Context: context.Background(), Model: "gpt-test", StartTs: 995, EndTs: 1005})
			require.NoError(t, err)
			require.Len(t, partial.Series, 1)
			assert.Equal(t, int64(995), partial.Series[0].Ts, "a partial first bucket remains visible inside the selected range")
			assert.Equal(t, 6.0, partial.Series[0].Rpm)
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			_, err = perfmetrics.QueryChannelMetrics(7, perfmetrics.AnalyticsQueryParams{Context: canceled, Model: "gpt-test", StartTs: 900, EndTs: 1200})
			assert.ErrorIs(t, err, context.Canceled)
		})
	}
}
