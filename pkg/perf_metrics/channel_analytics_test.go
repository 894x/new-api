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
