package perfmetrics

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAnalyticsLightweightFactsSurviveFlushAndRetryWithoutCapture(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "metrics.db")+"?_pragma=journal_mode(WAL)&_txlock=immediate"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	model.DB, model.LOG_DB = db, nil
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		detailHotBuckets = sync.Map{}
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	now := time.Now()
	// A rolling upgrade can append new measurements to an existing old bucket.
	require.NoError(t, db.Create(&model.PerfMetricDetail{ModelName: "facts", UserId: 7, TokenId: 11, BucketTs: bucketStart(now.Unix()), RequestCount: 2, InputTokens: 200, CacheReadTokens: 40}).Error)
	for i, size := range []int64{1024, 3072} {
		recordDetail(Sample{Model: "facts", UserId: 7, TokenId: 11, Success: true, RequestBodyBytes: &size, HasTtft: true, TtftMs: []int64{641, 645}[i], HasTpot: true, TpotMs: []int64{8, 2}[i], GenerationMs: []int64{8, 18}[i], OutputTokens: []int64{2, 10}[i], InputTokens: 100, CacheReadTokens: 40, CacheWriteTokens: 30, CacheWrite5mTokens: 10, CacheWrite1hTokens: 20}, now)
	}
	// An older/missing measurement must not become a zero-byte observation.
	recordDetail(Sample{Model: "facts", UserId: 7, TokenId: 11, Success: true}, now)
	params := AnalyticsQueryParams{Model: "facts", UserId: 7, StartTs: now.Add(-time.Hour).Unix(), EndTs: now.Add(time.Hour).Unix()}
	flushed := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:flush_during_snapshot", func(tx *gorm.DB) {
		if !flushed && tx.Statement.Table == "perf_metric_details" {
			flushed = true
			flushDetailBuckets()
		}
	}))
	result, err := QueryAnalytics(params)
	require.NoError(t, err)
	require.True(t, flushed)
	assert.Equal(t, int64(5), result.Summary.RequestCount)
	assert.Equal(t, int64(2), result.Summary.RequestBodySamples)
	assert.Equal(t, int64(4096), result.Summary.RequestBodyBytes)
	assert.Equal(t, int64(3072), result.Summary.RequestBodyMaxBytes)
	require.NotNil(t, result.Summary.RequestBodyAvgBytes)
	assert.Equal(t, float64(2048), *result.Summary.RequestBodyAvgBytes)
	assert.Equal(t, int64(120), result.Summary.CacheReadTokens)
	assert.Equal(t, int64(60), result.Summary.CacheWriteTokens)
	assert.Equal(t, int64(20), result.Summary.CacheWrite5mTokens)
	assert.Equal(t, int64(40), result.Summary.CacheWrite1hTokens)
	assert.Equal(t, 30.0, result.Summary.CacheHitRate)
	require.NotNil(t, result.Summary.Ttft.AvgMs)
	require.NotNil(t, result.Summary.Tpot.AvgMs)
	assert.Equal(t, 643.0, *result.Summary.Ttft.AvgMs)
	assert.Equal(t, 2.6, *result.Summary.Tpot.AvgMs)
	require.NoError(t, db.Create(&model.PerfMetricHistogram{ModelName: "facts", UserId: 7, TokenId: 11, BucketTs: bucketStart(now.Unix()), Metric: "ttft", UpperBoundMs: 1000, Count: 1}).Error)
	mixed, err := QueryAnalytics(params)
	require.NoError(t, err)
	assert.Nil(t, mixed.Summary.Ttft.AvgMs, "partial historical sums must not become an average for all samples")

}

func TestAnalyticsDegradesErrorScanFailureAndHonorsCancellation(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	model.DB, model.LOG_DB = db, db // No logs table: only the optional breakdown fails.
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB; detailHotBuckets = sync.Map{} })
	params := AnalyticsQueryParams{Model: "facts", UserId: 7, StartTs: 100, EndTs: 200}
	result, err := QueryAnalytics(params)
	require.NoError(t, err)
	assert.True(t, result.ErrorsUnavailable)
	assert.True(t, result.Truncated)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	params.Context = ctx
	_, err = QueryAnalytics(params)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, errors.Is(err, context.DeadlineExceeded))

	ctx, cancel = context.WithCancel(context.Background())
	params.Context = ctx
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:cancel_snapshot", func(tx *gorm.DB) {
		if tx.Statement.Table == "perf_metric_details" {
			cancel()
		}
	}))
	_, err = QueryAnalytics(params)
	assert.ErrorIs(t, err, context.Canceled)
	require.NoError(t, db.Callback().Query().Remove("test:cancel_snapshot"))
	// Reuse the same pool after cancellation: a transaction must not leak.
	params.Context = context.Background()
	_, err = QueryAnalytics(params)
	require.NoError(t, err)
}

func TestRecordRelaySampleFeedsVisibleTtftAndTpotPercentiles(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	model.DB = db
	hotBuckets = sync.Map{}
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() {
		model.DB = previousDB
		hotBuckets = sync.Map{}
		detailHotBuckets = sync.Map{}
	})

	now := time.Now()
	start := now.Add(-1050 * time.Millisecond)
	RecordRelaySampleAt(&relaycommon.RelayInfo{
		UserId:            7,
		TokenId:           11,
		OriginModelName:   "gpt-test",
		UsingGroup:        "default",
		StartTime:         start,
		FirstResponseTime: start.Add(250 * time.Millisecond),
		IsStream:          true,
	}, true, RelayTokenUsage{InputTokens: 100, OutputTokens: 11, CacheReadTokens: 25}, now)
	flushDetailBuckets()

	result, err := QueryAnalytics(AnalyticsQueryParams{
		Model: "gpt-test", UserId: 7, TokenId: 11,
		StartTs: now.Add(-time.Hour).Unix(), EndTs: now.Add(time.Hour).Unix(),
	})
	require.NoError(t, err)
	require.Len(t, result.Series, 1)
	assert.Equal(t, int64(1), result.Summary.RequestCount)
	assert.Equal(t, 100.0, result.Summary.SuccessRate)
	avgTtft, avgTpot := 250.0, 80.0
	maxTtft, maxTpot := int64(250), int64(80)
	assert.Equal(t, AnalyticsPercentiles{AvgMs: &avgTtft, P50Ms: 250, P90Ms: 250, P95Ms: 250, P99Ms: 250, MaxMs: &maxTtft, SampleCount: 1}, result.Summary.Ttft)
	assert.Equal(t, AnalyticsPercentiles{AvgMs: &avgTpot, P50Ms: 80, P90Ms: 80, P95Ms: 80, P99Ms: 80, MaxMs: &maxTpot, SampleCount: 1}, result.Summary.Tpot)
}

func TestRecordRelaySampleDoesNotInventStreamingMetricsForNonStreamRequests(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	model.DB = db
	hotBuckets = sync.Map{}
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() {
		model.DB = previousDB
		hotBuckets = sync.Map{}
		detailHotBuckets = sync.Map{}
	})

	now := time.Now()
	RecordRelaySample(&relaycommon.RelayInfo{
		UserId:          7,
		TokenId:         11,
		OriginModelName: "gpt-test",
		UsingGroup:      "default",
		StartTime:       now.Add(-time.Second),
	}, true, RelayTokenUsage{InputTokens: 100, OutputTokens: 11})
	flushDetailBuckets()

	result, err := QueryAnalytics(AnalyticsQueryParams{
		Model: "gpt-test", UserId: 7, TokenId: 11,
		StartTs: now.Add(-time.Hour).Unix(), EndTs: now.Add(time.Hour).Unix(),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Summary.RequestCount)
	assert.Zero(t, result.Summary.Ttft.SampleCount)
	assert.Zero(t, result.Summary.Tpot.SampleCount)
}

func TestFlushPersistsActiveDetailBucketForDashboardDiscovery(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	model.DB = db
	hotBuckets = sync.Map{}
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() {
		model.DB = previousDB
		hotBuckets = sync.Map{}
		detailHotBuckets = sync.Map{}
	})

	Record(Sample{
		Model: "gpt-test", Group: "default", UserId: 7, TokenId: 11,
		LatencyMs: 500, TtftMs: 100, HasTtft: true, TpotMs: 40, HasTpot: true,
		Success: true, OutputTokens: 11, GenerationMs: 400,
	})
	flushCompletedBuckets()

	var detailCount int64
	require.NoError(t, db.Model(&model.PerfMetricDetail{}).Count(&detailCount).Error)
	assert.Equal(t, int64(1), detailCount)
}

func TestQueryAnalyticsIncludesActiveDetailBucket(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	model.DB = db
	hotBuckets = sync.Map{}
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() {
		model.DB = previousDB
		hotBuckets = sync.Map{}
		detailHotBuckets = sync.Map{}
	})

	now := time.Now()
	Record(Sample{
		Model: "gpt-test", Group: "default", UserId: 7, TokenId: 11,
		LatencyMs: 500, TtftMs: 100, HasTtft: true, Success: true,
	})
	params := AnalyticsQueryParams{
		Model: "gpt-test", UserId: 7, TokenId: 11,
		StartTs: now.Add(-time.Hour).Unix(), EndTs: now.Add(time.Hour).Unix(),
	}

	result, err := QueryAnalytics(params)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Summary.RequestCount)
	assert.Equal(t, 100.0, result.Summary.SuccessRate)

	flushDetailBuckets()
	result, err = QueryAnalytics(params)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Summary.RequestCount)
	assert.Equal(t, int64(1), result.Summary.Ttft.SampleCount)

	Record(Sample{
		Model: "gpt-test", Group: "default", UserId: 7, TokenId: 11,
		LatencyMs: 700, TtftMs: 200, HasTtft: true, Success: false,
	})
	result, err = QueryAnalytics(params)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Summary.RequestCount)
	assert.Equal(t, 50.0, result.Summary.SuccessRate)
	avgTtft := 150.0
	maximum := int64(200)
	assert.Equal(t, AnalyticsPercentiles{AvgMs: &avgTtft, P50Ms: 100, P90Ms: 200, P95Ms: 200, P99Ms: 200, MaxMs: &maximum, SampleCount: 2}, result.Summary.Ttft)

	flushDetailBuckets()
	result, err = QueryAnalytics(params)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Summary.RequestCount)
	assert.Equal(t, 50.0, result.Summary.SuccessRate)
	assert.Equal(t, int64(2), result.Summary.Ttft.SampleCount)
}

func TestQueryAnalyticsAggregatesErrorsForSelectedUser(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	model.DB, model.LOG_DB = db, logDB
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		detailHotBuckets = sync.Map{}
	})

	payload, err := common.Marshal(map[string]any{
		"error_code":  "upstream_timeout",
		"error_type":  "upstream",
		"status_code": 504,
	})
	require.NoError(t, err)
	now := int64(1_000_120)
	require.NoError(t, logDB.Create(&[]model.Log{
		{UserId: 7, TokenId: 11, ModelName: "gpt-test", Type: model.LogTypeError, CreatedAt: now - 70, Other: string(payload)},
		{UserId: 7, TokenId: 11, ModelName: "gpt-test", Type: model.LogTypeError, CreatedAt: now - 20, Other: string(payload)},
		{UserId: 7, TokenId: 11, ModelName: "gpt-test", Type: model.LogTypeError, CreatedAt: now - 10, Other: string(payload)},
		{UserId: 8, TokenId: 11, ModelName: "gpt-test", Type: model.LogTypeError, CreatedAt: now - 10, Other: string(payload)},
		{UserId: 7, TokenId: 11, ModelName: "gpt-test", Type: model.LogTypeConsume, CreatedAt: now - 10, Other: string(payload)},
	}).Error)

	result, err := QueryAnalytics(AnalyticsQueryParams{
		Model: "gpt-test", UserId: 7, TokenId: 11,
		StartTs: now - 60, EndTs: now + 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.ScannedLogs)
	assert.Equal(t, int64(2), result.Summary.ErrorCount)
	require.Len(t, result.ErrorGroups, 1)
	assert.Equal(t, int64(2), result.ErrorGroups[0].Count)
	assert.Equal(t, "upstream_timeout", result.ErrorGroups[0].ErrorCode)
	assert.Equal(t, 504, result.ErrorGroups[0].StatusCode)
}

func TestCleanupCapsUnlimitedDetailRetentionAtThirtyDays(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	require.NoError(t, db.Create(&model.PerfMetricDetail{
		ModelName: "old", UserId: 7, TokenId: 11,
		BucketTs: time.Now().Add(-31 * 24 * time.Hour).Unix(), RequestCount: 1,
	}).Error)
	require.NoError(t, db.Create(&model.PerfMetricDetail{
		ModelName: "recent", UserId: 7, TokenId: 11,
		BucketTs: time.Now().Add(-29 * 24 * time.Hour).Unix(), RequestCount: 1,
	}).Error)

	cleanupExpiredMetrics(0)

	var modelNames []string
	require.NoError(t, db.Model(&model.PerfMetricDetail{}).Order("model_name ASC").Pluck("model_name", &modelNames).Error)
	assert.Equal(t, []string{"recent"}, modelNames)
}

func TestAnalyticsBucketSecondsKeepsSeriesBounded(t *testing.T) {
	const day = int64(24 * 60 * 60)

	assert.Equal(t, int64(5*60), analyticsBucketSeconds(0, day, 60))
	assert.Equal(t, int64(60*60), analyticsBucketSeconds(0, 7*day, 60))
	assert.Equal(t, int64(6*60*60), analyticsBucketSeconds(0, 29*day, 60))
	assert.Equal(t, int64(24*60*60), analyticsBucketSeconds(0, 29*day, 24*60*60))
}

func TestAnalyticsRatesUseBucketDurationAndInputWeightedCache(t *testing.T) {
	accumulator := newAnalyticsAccumulator()
	accumulator.requestCount = 30
	accumulator.inputTokens = 1000
	accumulator.totalTokens = 1500
	accumulator.cacheReadTokens = 250

	point := buildAnalyticsPoint(100, 5*60, accumulator)
	assert.Equal(t, 6.0, point.Rpm)
	assert.Equal(t, 300.0, point.Tpm)
	assert.Equal(t, 25.0, point.CacheHitRate)

	summary := buildAnalyticsSummary(10*60, accumulator)
	assert.Equal(t, 3.0, summary.Rpm)
	assert.Equal(t, 150.0, summary.Tpm)
	assert.Equal(t, 25.0, summary.CacheHitRate)
}

func TestSealedCompletedBucketRetriesBoundarySample(t *testing.T) {
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() { detailHotBuckets = sync.Map{} })

	recordedAt := time.Now().Add(-time.Hour)
	key := detailBucketKey{
		model: "gpt-test", userId: 7, tokenId: 11,
		bucketTs: bucketStart(recordedAt.Unix()),
	}
	oldBucket := &atomicDetailBucket{}
	detailHotBuckets.Store(key, oldBucket)
	_, ok := oldBucket.drain(true)
	require.True(t, ok)
	detailHotBuckets.CompareAndDelete(key, oldBucket)
	assert.False(t, oldBucket.add(Sample{Success: true}))

	recordDetail(Sample{
		Model: "gpt-test", UserId: 7, TokenId: 11, Success: true,
	}, recordedAt)
	actual, ok := detailHotBuckets.Load(key)
	require.True(t, ok)
	drained, ok := actual.(*atomicDetailBucket).drain(false)
	require.True(t, ok)
	assert.Equal(t, int64(1), drained.requestCount)
	assert.Equal(t, int64(1), drained.successCount)
}

func TestAnalyticsRequestDistributionsSurviveFlushRetry(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetricDetail{}, &model.PerfMetricHistogram{}))
	model.DB, model.LOG_DB = db, nil
	detailHotBuckets = sync.Map{}
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		detailHotBuckets = sync.Map{}
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	for _, fixture := range []struct{ value, count int64 }{{0, 50}, {100, 40}, {200, 5}, {400, 4}, {573, 1}} {
		for range fixture.count {
			size := fixture.value
			recordDetail(Sample{Model: "distribution", UserId: 7, TokenId: 11, Success: true,
				RequestBodyBytes: &size, CacheWriteKnown: true, CacheWriteTokens: size,
				CacheReadKnown: true, InputTokens: 1000, CacheReadTokens: size,
				HasTtft: true, TtftMs: size + 1, HasTpot: true, TpotMs: size + 1,
			}, time.Unix(600, 0))
		}
	}
	// Unknown observations are excluded from distributions, while measured zeros remain.
	recordDetail(Sample{Model: "distribution", UserId: 7, TokenId: 11}, time.Unix(600, 0))
	params := AnalyticsQueryParams{Model: "distribution", UserId: 7, StartTs: 300, EndTs: 900}
	before, err := QueryAnalytics(params)
	require.NoError(t, err)
	maximum := 573.0
	expected := AnalyticsValuePercentiles{P50: 0, P90: 104, P95: 208, P99: 416, Max: &maximum, SampleCount: 100}
	assert.Equal(t, expected, before.Summary.MetricPercentiles["cache_write_tokens"])
	assert.Equal(t, expected, before.Summary.MetricPercentiles["request_body_avg_bytes"])
	hit := before.Summary.MetricPercentiles["cache_hit_rate"]
	assert.Equal(t, int64(100), hit.SampleCount)
	assert.InDelta(t, 57.3, *hit.Max, 0.001)
	assert.Equal(t, int64(574), *before.Summary.Ttft.MaxMs)
	assert.Equal(t, int64(250), before.Summary.Ttft.P95Ms)
	// TPOT averages require measured generation duration and output token weights.
	ttftDistribution := before.Summary.Ttft
	ttftDistribution.AvgMs = nil
	assert.Equal(t, ttftDistribution, before.Summary.Tpot)
	require.Len(t, before.Series, 1)
	assert.Equal(t, expected, before.Series[0].MetricPercentiles["cache_write_tokens"])

	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_histogram", func(tx *gorm.DB) {
		if tx.Statement.Table == "perf_metric_histograms" {
			tx.AddError(errors.New("test histogram write failure"))
		}
	}))
	flushDetailBuckets()
	afterFailure, err := QueryAnalytics(params)
	require.NoError(t, err)
	assert.Equal(t, before, afterFailure, "failed transaction restores every distribution and exact maximum")
	require.NoError(t, db.Callback().Create().Remove("test:fail_histogram"))
	flushDetailBuckets()
	flushDetailBuckets()
	afterFlush, err := QueryAnalytics(params)
	require.NoError(t, err)
	assert.Equal(t, before, afterFlush, "persistence and repeated flushes preserve the API result")
}

func TestAnalyticsLegacyMaximumIsUnknownAndNewLongTailsKeepExactMaximum(t *testing.T) {
	legacy := histogramPercentiles(map[int64]histogramObservation{250: {count: 1}})
	assert.Equal(t, int64(250), legacy.P95Ms)
	assert.Nil(t, legacy.MaxMs)
	mixed := histogramPercentiles(map[int64]histogramObservation{250: {count: 2, maxSamples: 1, maxValue: 201}})
	assert.Nil(t, mixed.MaxMs, "a new observation cannot reconstruct a legacy maximum")
	bucket := &atomicDetailBucket{}
	require.True(t, bucket.add(Sample{HasTtft: true, TtftMs: 800001}))
	accumulator := newAnalyticsAccumulator()
	addDetailCounters(accumulator, bucket.snapshot())
	point := buildAnalyticsPoint(600, 300, accumulator)
	require.NotNil(t, point.Ttft.MaxMs)
	assert.Equal(t, int64(800001), *point.Ttft.MaxMs)
	assert.Equal(t, int64(800001), point.Ttft.P99Ms)
}
