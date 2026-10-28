package model

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUpsertPerfAnalyticsBucketAddsRepeatedFlushes(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PerfMetricDetail{}, &PerfMetricHistogram{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	first := &PerfMetricDetail{
		ModelName: "gpt-test", UserId: 7, TokenId: 11, BucketTs: 100,
		RequestCount: 2, SuccessCount: 1, TtftCount: 2, TpotCount: 1,
		InputTokens: 200, OutputTokens: 80, TotalTokens: 280, CacheReadTokens: 50,
	}
	second := &PerfMetricDetail{
		ModelName: "gpt-test", UserId: 7, TokenId: 11, BucketTs: 100,
		RequestCount: 1, SuccessCount: 1, TtftCount: 1, TpotCount: 1,
		InputTokens: 100, OutputTokens: 40, TotalTokens: 140, CacheReadTokens: 20,
	}
	require.NoError(t, UpsertPerfAnalyticsBucket(first, []PerfMetricHistogram{{
		ModelName: "gpt-test", UserId: 7, TokenId: 11, BucketTs: 100,
		Metric: "ttft", UpperBoundMs: 250, Count: 2, MaxValue: 221, MaxSamples: 2,
	}}))
	require.NoError(t, UpsertPerfAnalyticsBucket(second, []PerfMetricHistogram{{
		ModelName: "gpt-test", UserId: 7, TokenId: 11, BucketTs: 100,
		Metric: "ttft", UpperBoundMs: 250, Count: 3, MaxValue: 244, MaxSamples: 3,
	}}))

	var detail PerfMetricDetail
	require.NoError(t, DB.First(&detail).Error)
	assert.Equal(t, int64(3), detail.RequestCount)
	assert.Equal(t, int64(2), detail.SuccessCount)
	assert.Equal(t, int64(3), detail.TtftCount)
	assert.Equal(t, int64(2), detail.TpotCount)
	assert.Equal(t, int64(300), detail.InputTokens)
	assert.Equal(t, int64(120), detail.OutputTokens)
	assert.Equal(t, int64(420), detail.TotalTokens)
	assert.Equal(t, int64(70), detail.CacheReadTokens)

	var histogram PerfMetricHistogram
	require.NoError(t, DB.First(&histogram).Error)
	assert.Equal(t, int64(5), histogram.Count)
	assert.Equal(t, int64(244), histogram.MaxValue)
	assert.Equal(t, int64(5), histogram.MaxSamples)

	require.NoError(t, UpsertPerfAnalyticsBucket(&PerfMetricDetail{
		ModelName: "gpt-test", UserId: 8, TokenId: 22, BucketTs: 100,
		RequestCount: 2, SuccessCount: 2, TtftCount: 2, TpotCount: 0,
	}, []PerfMetricHistogram{{
		ModelName: "gpt-test", UserId: 8, TokenId: 22, BucketTs: 100,
		Metric: "ttft", UpperBoundMs: 250, Count: 4,
	}}))

	details, err := GetPerfMetricDetails(PerfMetricDetailFilter{
		ModelName: "gpt-test", StartTs: 1, EndTs: 200, BucketSeconds: 300,
	})
	require.NoError(t, err)
	require.Len(t, details, 1)
	assert.Equal(t, int64(5), details[0].RequestCount)
	assert.Equal(t, int64(4), details[0].SuccessCount)
	assert.Equal(t, int64(300), details[0].InputTokens)
	assert.Equal(t, int64(120), details[0].OutputTokens)
	assert.Equal(t, int64(420), details[0].TotalTokens)
	assert.Equal(t, int64(70), details[0].CacheReadTokens)
	assert.Equal(t, int64(0), details[0].BucketTs)

	histograms, err := GetPerfMetricHistograms(PerfMetricDetailFilter{
		ModelName: "gpt-test", StartTs: 1, EndTs: 200, BucketSeconds: 300,
	})
	require.NoError(t, err)
	require.Len(t, histograms, 1)
	assert.Equal(t, int64(9), histograms[0].Count)
	assert.Equal(t, int64(0), histograms[0].BucketTs)
}

func TestUpsertPerfAnalyticsBucketsAddsRowsInBatches(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PerfMetricDetail{}, &PerfMetricHistogram{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	details := []PerfMetricDetail{
		{ModelName: "model-a", UserId: 7, TokenId: 11, BucketTs: 100, RequestCount: 2},
		{ModelName: "model-b", UserId: 8, TokenId: 22, BucketTs: 100, RequestCount: 3},
	}
	histograms := []PerfMetricHistogram{
		{ModelName: "model-a", UserId: 7, TokenId: 11, BucketTs: 100, Metric: "ttft", UpperBoundMs: 250, Count: 2},
		{ModelName: "model-b", UserId: 8, TokenId: 22, BucketTs: 100, Metric: "ttft", UpperBoundMs: 500, Count: 3},
	}
	require.NoError(t, UpsertPerfAnalyticsBuckets(details, histograms))
	require.NoError(t, UpsertPerfAnalyticsBuckets(details, histograms))

	var requestCount int64
	require.NoError(t, db.Model(&PerfMetricDetail{}).Select("SUM(request_count)").Scan(&requestCount).Error)
	assert.Equal(t, int64(10), requestCount)
	var histogramCount int64
	require.NoError(t, db.Model(&PerfMetricHistogram{}).Select("SUM(count)").Scan(&histogramCount).Error)
	assert.Equal(t, int64(10), histogramCount)
}

// Histogram schema from main before measured maxima were persisted.
type legacyPerfMetricHistogram struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	ModelName    string `json:"model_name" gorm:"size:128;uniqueIndex:idx_perf_hist_dimension_bucket,priority:1;index:idx_perf_hist_model_bucket,priority:1;index:idx_perf_hist_user_model_bucket,priority:2"`
	UserId       int    `json:"user_id" gorm:"uniqueIndex:idx_perf_hist_dimension_bucket,priority:2;index:idx_perf_hist_user_model_bucket,priority:1"`
	TokenId      int    `json:"token_id" gorm:"uniqueIndex:idx_perf_hist_dimension_bucket,priority:3"`
	BucketTs     int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_perf_hist_dimension_bucket,priority:4;index:idx_perf_hist_model_bucket,priority:2;index:idx_perf_hist_user_model_bucket,priority:3;index:idx_perf_hist_bucket"`
	Metric       string `json:"metric" gorm:"size:8;uniqueIndex:idx_perf_hist_dimension_bucket,priority:5"`
	UpperBoundMs int64  `json:"upper_bound_ms" gorm:"uniqueIndex:idx_perf_hist_dimension_bucket,priority:6"`
	Count        int64  `json:"-" gorm:"default:0"`
}

func (legacyPerfMetricHistogram) TableName() string { return "perf_metric_histograms" }

// DSNs must name isolated codex_perf databases; no production database is used.
func TestPerfAnalyticsDatabaseMatrix(t *testing.T) {
	for _, engine := range []struct {
		kind common.DatabaseType
		env  string
	}{
		{common.DatabaseTypeSQLite, ""}, {common.DatabaseTypeMySQL, "TEST_PERF_ANALYTICS_MYSQL_DSN"}, {common.DatabaseTypePostgreSQL, "TEST_PERF_ANALYTICS_POSTGRES_DSN"},
	} {
		t.Run(string(engine.kind), func(t *testing.T) {
			var driver gorm.Dialector = sqlite.Open(":memory:")
			if engine.env != "" {
				dsn := os.Getenv(engine.env)
				if dsn == "" {
					t.Skip(engine.env + " is not configured")
				}
				require.Contains(t, dsn, "codex_perf", "verification requires an isolated database")
				if engine.kind == common.DatabaseTypeMySQL {
					driver = mysql.Open(dsn)
				} else {
					driver = postgres.Open(dsn)
				}
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			previousDB, previousType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(engine.kind)
			t.Cleanup(func() {
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				sqlDB, _ := db.DB()
				_ = sqlDB.Close()
			})
			var version string
			versionSQL := "SELECT VERSION()"
			if engine.kind == common.DatabaseTypeSQLite {
				versionSQL = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
			t.Logf("database=%s", version)
			for _, upgrade := range []bool{false, true} {
				t.Run(fmt.Sprintf("upgrade_%t", upgrade), func(t *testing.T) {
					require.NoError(t, db.Migrator().DropTable(&PerfMetricHistogram{}, &PerfMetricDetail{}))
					if upgrade {
						require.NoError(t, db.AutoMigrate(&PerfMetricDetail{}, &legacyPerfMetricHistogram{}))
						require.NoError(t, db.Create(&PerfMetricDetail{
							ModelName: "matrix", UserId: 7, TokenId: 11, BucketTs: 100,
							RequestCount: 2, InputTokens: 200, CacheReadTokens: 40,
							RequestBodySamples: 1, RequestBodyBytes: 4096, RequestBodyMaxBytes: 4096,
							CacheWriteSamples: 1, CacheWriteTokens: 9, CacheWrite5mTokens: 4, CacheWrite1hTokens: 5,
						}).Error)
						require.NoError(t, db.Create(&legacyPerfMetricHistogram{ModelName: "matrix", UserId: 7, TokenId: 11, BucketTs: 100, Metric: "ttft", UpperBoundMs: 250, Count: 2}).Error)
					}
					// Exercise a fresh install and the previous main schema, migrating twice.
					for range 2 {
						require.NoError(t, db.AutoMigrate(&PerfMetricDetail{}, &PerfMetricHistogram{}))
					}
					for _, model := range []any{&PerfMetricDetail{}, &PerfMetricHistogram{}} {
						prefix := "idx_perf_detail_"
						if _, histogram := model.(*PerfMetricHistogram); histogram {
							prefix = "idx_perf_hist_"
						}
						for _, index := range []string{"dimension_bucket", "model_bucket", "user_model_bucket", "bucket"} {
							assert.True(t, db.Migrator().HasIndex(model, prefix+index), prefix+index)
						}
					}
					require.True(t, db.Migrator().HasColumn(&PerfMetricHistogram{}, "MaxValue"))
					require.True(t, db.Migrator().HasColumn(&PerfMetricHistogram{}, "MaxSamples"))
					if upgrade {
						var legacy PerfMetricHistogram
						require.NoError(t, db.First(&legacy).Error)
						assert.Equal(t, int64(2), legacy.Count)
						assert.Zero(t, legacy.MaxValue, "migration cannot reconstruct a historical maximum")
						assert.Zero(t, legacy.MaxSamples, "historical samples must remain unmeasured")
					}
					row := PerfMetricDetail{ModelName: "matrix", UserId: 7, TokenId: 11, BucketTs: 100, RequestCount: 1, RequestBodySamples: 1, RequestBodyBytes: 2048, RequestBodyMaxBytes: 2048, CacheWriteSamples: 1, CacheWriteTokens: 30, CacheWrite5mTokens: 10, CacheWrite1hTokens: 20}
					require.NoError(t, UpsertPerfAnalyticsBucket(&row, []PerfMetricHistogram{{ModelName: "matrix", UserId: 7, TokenId: 11, BucketTs: 100, Metric: "ttft", UpperBoundMs: 250, Count: 1, MaxSamples: 1, MaxValue: 233}}))
					row.Id = 0
					row.RequestBodyBytes = 1024
					row.RequestBodyMaxBytes = 1024
					require.NoError(t, UpsertPerfAnalyticsBucket(&row, []PerfMetricHistogram{{ModelName: "matrix", UserId: 7, TokenId: 11, BucketTs: 100, Metric: "ttft", UpperBoundMs: 250, Count: 1, MaxSamples: 1, MaxValue: 211}}))
					details, histograms, err := GetPerfAnalyticsBuckets(PerfMetricDetailFilter{Context: context.Background(), ModelName: "matrix", UserId: 7, TokenId: 11, StartTs: 1, EndTs: 200, BucketSeconds: 300})
					require.NoError(t, err)
					require.Len(t, details, 1)
					require.Len(t, histograms, 1)
					assert.Equal(t, int64(233), histograms[0].MaxValue)
					assert.Equal(t, int64(2), histograms[0].MaxSamples)
					histogramCount := int64(2)
					if upgrade {
						histogramCount += 2
					}
					assert.Equal(t, histogramCount, histograms[0].Count)
					expected := int64(2)
					if upgrade {
						expected = 4
					}
					assert.Equal(t, expected, details[0].RequestCount)
					expectedBodySamples, expectedBodyBytes, expectedBodyMax := int64(2), int64(3072), int64(2048)
					expectedWrites, expectedWrites5m, expectedWrites1h := int64(60), int64(20), int64(40)
					if upgrade {
						expectedBodySamples++
						expectedBodyBytes += 4096
						expectedBodyMax = 4096
						expectedWrites += 9
						expectedWrites5m += 4
						expectedWrites1h += 5
					}
					assert.Equal(t, expectedBodySamples, details[0].RequestBodySamples)
					assert.Equal(t, expectedBodyBytes, details[0].RequestBodyBytes)
					assert.Equal(t, expectedBodyMax, details[0].RequestBodyMaxBytes)
					assert.Equal(t, expectedWrites, details[0].CacheWriteTokens)
					assert.Equal(t, expectedWrites5m, details[0].CacheWrite5mTokens)
					assert.Equal(t, expectedWrites1h, details[0].CacheWrite1hTokens)
					duplicate := PerfMetricDetail{ModelName: "matrix", UserId: 7, TokenId: 11, BucketTs: 100}
					require.Error(t, db.Create(&duplicate).Error, "dimension uniqueness survives migration")
					duplicateHistogram := PerfMetricHistogram{ModelName: "matrix", UserId: 7, TokenId: 11, BucketTs: 100, Metric: "ttft", UpperBoundMs: 250}
					require.Error(t, db.Create(&duplicateHistogram).Error, "histogram dimension uniqueness survives migration")
					if upgrade {
						assert.Equal(t, int64(200), details[0].InputTokens)
						assert.Equal(t, int64(40), details[0].CacheReadTokens)
					}
					// Expose canceled contexts to the database rather than retaining abandoned scans.
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					_, _, err = GetPerfAnalyticsBuckets(PerfMetricDetailFilter{Context: ctx, ModelName: "matrix", StartTs: 1, EndTs: 200})
					require.Error(t, err)
				})
			}
		})
	}
}
