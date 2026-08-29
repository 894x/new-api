package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		Metric: "ttft", UpperBoundMs: 250, Count: 2,
	}}))
	require.NoError(t, UpsertPerfAnalyticsBucket(second, []PerfMetricHistogram{{
		ModelName: "gpt-test", UserId: 7, TokenId: 11, BucketTs: 100,
		Metric: "ttft", UpperBoundMs: 250, Count: 3,
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
