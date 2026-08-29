package perfmetrics

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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
	assert.Equal(t, AnalyticsPercentiles{P50Ms: 250, P90Ms: 250, P99Ms: 250, SampleCount: 1}, result.Summary.Ttft)
	assert.Equal(t, AnalyticsPercentiles{P50Ms: 80, P90Ms: 80, P99Ms: 80, SampleCount: 1}, result.Summary.Tpot)
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

func TestQueryAnalyticsOnlyReturnsPersistedBuckets(t *testing.T) {
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
	assert.Zero(t, result.Summary.RequestCount)

	flushDetailBuckets()
	result, err = QueryAnalytics(params)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Summary.RequestCount)
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
