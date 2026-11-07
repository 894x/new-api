package perfmetrics

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDecodeChannelTimingUsesAdminRequestTimingAndLogType(t *testing.T) {
	payload, err := common.Marshal(map[string]any{
		"admin_info": map[string]any{
			"request_timing": common.RequestTimingSnapshot{
				RequestReceivedAtMs:         1000,
				RequestBodyReadAtMs:         1100,
				UpstreamRequestStartedAtMs:  1200,
				UpstreamRequestWrittenAtMs:  1500,
				UpstreamResponseHeadersAtMs: 2500,
				FirstResponseAtMs:           2700,
				RequestCompletedAtMs:        3200,
			},
		},
	})
	require.NoError(t, err)

	sample, ok := decodeChannelTiming(model.PerfChannelLog{
		Type:  model.LogTypeConsume,
		Other: string(payload),
	})
	require.True(t, ok)
	assert.True(t, sample.success)
	assert.Equal(t, int64(1000), sample.startMs)
	assert.Equal(t, int64(3200), sample.endMs)
	assert.Equal(t, int64(3000), sample.latency.totalMs.percentiles().P50Ms)
	assert.Equal(t, int64(1000), sample.latency.providerWaitMs.percentiles().P50Ms)
}

func TestChannelAnalyticsSpansBucketsAndKeepsEmptyIntervals(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	old := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = old })
	payload, err := common.Marshal(map[string]any{"admin_info": map[string]any{"request_timing": common.RequestTimingSnapshot{RequestReceivedAtMs: 105000, RequestCompletedAtMs: 135000}}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Log{ChannelId: 15, Type: model.LogTypeConsume, CreatedAt: 135, Other: string(payload)}).Error)
	result, err := QueryChannelAnalytics(ChannelAnalyticsQueryParams{ChannelId: 15, StartTs: 100, EndTs: 150, BucketSeconds: 10})
	require.NoError(t, err)
	require.Len(t, result.Series, 5)
	assert.Equal(t, []float64{0.5, 1, 1, 0.5, 0}, []float64{result.Series[0].ActiveConcurrency.Average, result.Series[1].ActiveConcurrency.Average, result.Series[2].ActiveConcurrency.Average, result.Series[3].ActiveConcurrency.Average, result.Series[4].ActiveConcurrency.Average})
	assert.Equal(t, int64(1), result.Series[3].RequestCount)
	assert.Zero(t, result.Series[0].RequestCount)
	assert.Equal(t, 0.6, result.Summary.ActiveConcurrency.Average)
	assert.False(t, result.Truncated)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = QueryChannelAnalytics(ChannelAnalyticsQueryParams{Context: ctx, ChannelId: 15, StartTs: 100, EndTs: 150})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestChannelAnalyticsDoesNotInventZeroLatencyOrClipLongTail(t *testing.T) {
	var h histogram
	h.add(-1)
	assert.Zero(t, h.percentiles().SampleCount)
	h.add(900000)
	assert.GreaterOrEqual(t, h.percentiles().P99Ms, int64(900000))
	groups := make(map[string]*channelTransportAccumulator)
	write := 120.0
	accumulateChannelTransport(groups, common.UpstreamTransportSnapshot{ChannelID: 15, BodyBytes: 2 << 20, Protocol: "HTTP/1.1", Reason: "large_body", WriteMs: &write, Outcome: "timeout"})
	result := channelTransportResults(groups)
	require.Len(t, result, 1)
	assert.Equal(t, int64(1), result[0].Timeouts)
	assert.Equal(t, int64(1), result[0].Errors)
	assert.Zero(t, result[0].AcquireMs.SampleCount)
	assert.Equal(t, int64(1), result[0].WriteMs.SampleCount)
}

func TestDecodeChannelTimingMarksErrorLogsAsFailed(t *testing.T) {
	sample, ok := decodeChannelTiming(model.PerfChannelLog{
		Type:      model.LogTypeError,
		CreatedAt: 5,
		UseTime:   2,
	})
	require.True(t, ok)
	assert.False(t, sample.success)
	assert.Equal(t, int64(3000), sample.startMs)
	assert.Equal(t, int64(5000), sample.endMs)
}

func TestChannelAnalyticsAggregatesErrorDimensions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	old := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = old })

	payload, err := common.Marshal(map[string]any{
		"error_code":  "upstream_timeout",
		"error_type":  "upstream",
		"status_code": 504,
		"admin_info": map[string]any{
			"request_timing": common.RequestTimingSnapshot{
				RequestReceivedAtMs: 1000, RequestCompletedAtMs: 3000,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Log{
		ChannelId: 15, Type: model.LogTypeError, CreatedAt: 3, Other: string(payload),
	}).Error)

	result, err := QueryChannelAnalytics(ChannelAnalyticsQueryParams{
		ChannelId: 15, StartTs: 1, EndTs: 5, BucketSeconds: 2,
	})
	require.NoError(t, err)
	require.Len(t, result.ErrorGroups, 1)
	assert.Equal(t, "upstream_timeout", result.ErrorGroups[0].ErrorCode)
	assert.Equal(t, "upstream", result.ErrorGroups[0].ErrorType)
	assert.Equal(t, 504, result.ErrorGroups[0].StatusCode)
	assert.Equal(t, int64(1), result.ErrorGroups[0].Count)
	assert.Equal(t, int64(1), result.Summary.ErrorCount)
}

func TestChannelAnalyticsPreservesMeasuredAveragesAndFractionalTransportTiming(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	old := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = old
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	for _, sample := range []struct {
		start, end int64
		acquire    float64
	}{{1000, 3200, .125}, {4000, 4800, .375}} {
		payload, err := common.Marshal(map[string]any{
			"admin_info": map[string]any{
				"request_timing": common.RequestTimingSnapshot{
					RequestReceivedAtMs: sample.start, RequestCompletedAtMs: sample.end,
				},
				"upstream_transport": []common.UpstreamTransportSnapshot{
					{ChannelID: 15, AcquireMs: &sample.acquire},
				},
			},
		})
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.Log{
			ChannelId: 15, Type: model.LogTypeConsume,
			CreatedAt: sample.end / 1000, Other: string(payload),
		}).Error)
	}
	result, err := QueryChannelAnalytics(ChannelAnalyticsQueryParams{Context: context.Background(), ChannelId: 15, StartTs: 1, EndTs: 6, BucketSeconds: 1})
	require.NoError(t, err)
	require.NotNil(t, result.Summary.Latency.TotalMs.AvgMs)
	assert.Equal(t, 1500.0, *result.Summary.Latency.TotalMs.AvgMs)
	require.NotNil(t, result.Summary.Latency.TotalMs.MaxMs)
	assert.Equal(t, 2200.0, *result.Summary.Latency.TotalMs.MaxMs)
	assert.Equal(t, int64(3000), result.Summary.Latency.TotalMs.P90Ms)
	require.NotNil(t, result.Summary.Latency.AcquireMs.AvgMs)
	assert.Equal(t, .25, *result.Summary.Latency.AcquireMs.AvgMs)
	require.NotNil(t, result.Summary.Latency.AcquireMs.MaxMs)
	assert.Equal(t, .375, *result.Summary.Latency.AcquireMs.MaxMs)
	assert.Nil(t, result.Summary.Latency.ProviderWaitMs.AvgMs)
	assert.Nil(t, result.Summary.Latency.ProviderWaitMs.MaxMs)
}

func TestChannelAnalyticsBodySizesFollowChannelAndCompletionWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	original := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = original
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	for _, sample := range []struct {
		channel, logType int
		end              int64
		attempts         []map[string]any
	}{
		{15, model.LogTypeConsume, 121, []map[string]any{{"channel_id": 14, "body_bytes": 4096}, {"channel_id": 15, "body_bytes": 0}}},
		{15, model.LogTypeConsume, 122, []map[string]any{{"channel_id": 15, "body_bytes": 16}}},
		{15, model.LogTypeError, 135, []map[string]any{{"channel_id": 15, "body_bytes": 80}}},
		{16, model.LogTypeConsume, 123, []map[string]any{{"channel_id": 16, "body_bytes": 20 << 20}}},
		{17, model.LogTypeConsume, 123, []map[string]any{{"channel_id": 17, "body_bytes": 17}}},
		{17, model.LogTypeConsume, 124, []map[string]any{{"channel_id": 17, "body_bytes": 33}}},
		{15, model.LogTypeConsume, 140, []map[string]any{{"channel_id": 15, "body_bytes": -1}}},
		{15, model.LogTypeConsume, 141, []map[string]any{{"channel_id": 15}}},
		{15, model.LogTypeConsume, 142, []map[string]any{{"channel_id": 15, "body_bytes": nil}}},
		{15, model.LogTypeConsume, 143, nil},
		{15, model.LogTypeConsume, 144, []map[string]any{{"channel_id": 16, "body_bytes": 1024}}},
		{15, model.LogTypeConsume, 99, []map[string]any{{"channel_id": 15, "body_bytes": 2048}}},
		{15, model.LogTypeConsume, 151, []map[string]any{{"channel_id": 15, "body_bytes": 2048}}},
	} {
		payload, err := common.Marshal(map[string]any{"admin_info": map[string]any{
			"request_timing":     common.RequestTimingSnapshot{RequestReceivedAtMs: (sample.end - 2) * 1000, RequestCompletedAtMs: sample.end * 1000},
			"upstream_transport": sample.attempts,
		}})
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.Log{ChannelId: sample.channel, Type: sample.logType, CreatedAt: sample.end, Other: string(payload)}).Error)
	}

	for _, tc := range []struct {
		name            string
		channel         int
		start, end      int64
		count           int64
		avg, p50, upper float64
	}{
		{"selected channel counts only its final attempts including errors and zero", 15, 100, 150, 3, 32, 16, 80},
		{"shorter window excludes later attempts", 15, 100, 130, 2, 8, 0, 16},
		{"large bodies retain byte units and measured maximum", 16, 100, 150, 1, 20 << 20, 20 << 20, 20 << 20},
		{"byte histogram estimates percentiles and keeps the exact mean and maximum", 17, 100, 150, 2, 25, 18, 33},
		{"unknown and missing lengths do not become zero", 15, 140, 150, 0, 0, 0, 0},
		{"empty channel has no measured values", 18, 100, 150, 0, 0, 0, 0},
		{"all channels combine samples without replaying earlier retry snapshots", 0, 100, 150, 6, float64((20<<20)+146) / 6, 18, 20 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := QueryChannelAnalytics(ChannelAnalyticsQueryParams{ChannelId: tc.channel, StartTs: tc.start, EndTs: tc.end, BucketSeconds: 10})
			require.NoError(t, err)
			stats := result.Summary.RequestBodyBytes
			assert.Equal(t, tc.count, stats.SampleCount)
			if tc.count == 0 {
				assert.Nil(t, stats.Avg)
				assert.Nil(t, stats.Max)
				return
			}
			require.NotNil(t, stats.Avg)
			require.NotNil(t, stats.Max)
			assert.Equal(t, tc.avg, *stats.Avg)
			assert.Equal(t, tc.p50, stats.P50)
			assert.Equal(t, tc.upper, stats.P90)
			assert.Equal(t, tc.upper, stats.P95)
			assert.Equal(t, tc.upper, stats.P99)
			assert.Equal(t, tc.upper, *stats.Max)
			if tc.channel == 15 && tc.start == 100 {
				require.Len(t, result.Series, int((tc.end-tc.start)/10))
				assert.Zero(t, result.Series[0].RequestBodyBytes.SampleCount)
				assert.Nil(t, result.Series[0].RequestBodyBytes.Avg)
				bucket := result.Series[2].RequestBodyBytes
				assert.Equal(t, int64(2), bucket.SampleCount)
				require.NotNil(t, bucket.Avg)
				assert.Equal(t, 8.0, *bucket.Avg)
				assert.Zero(t, bucket.P50)
			}
		})
	}
}
