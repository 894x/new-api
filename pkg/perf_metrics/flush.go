package perfmetrics

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
)

var flushMu sync.Mutex

// Readers validate this generation around their database snapshot. Only the
// in-memory snapshot is protected, so slow database reads never hold up a flush.
var detailSnapshotMu sync.RWMutex
var detailFlushGeneration uint64
var detailFlushDone chan struct{}

type pendingDetailFlush struct {
	key        detailBucketKey
	counters   detailCounters
	detail     model.PerfMetricDetail
	histograms []model.PerfMetricHistogram
}

func flushLoop() {
	for {
		interval := perf_metrics_setting.GetFlushIntervalMinutes()
		time.Sleep(time.Duration(interval) * time.Minute)
		setting := perf_metrics_setting.GetSetting()
		if !setting.Enabled {
			continue
		}
		flushCompletedBuckets()
		cleanupExpiredMetrics(setting.RetentionDays)
	}
}

func flushCompletedBuckets() {
	flushMu.Lock()
	defer flushMu.Unlock()

	currentBucket := bucketStart(time.Now().Unix())
	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		if k.bucketTs >= currentBucket {
			return true
		}

		bucket := value.(*atomicBucket)
		drained := bucket.drain()
		if drained.requestCount == 0 {
			deleteOldEmptyBucket(k, key)
			return true
		}

		err := model.UpsertPerfMetric(&model.PerfMetric{
			ModelName:      k.model,
			Group:          k.group,
			BucketTs:       k.bucketTs,
			RequestCount:   drained.requestCount,
			SuccessCount:   drained.successCount,
			TotalLatencyMs: drained.totalLatencyMs,
			TtftSumMs:      drained.ttftSumMs,
			TtftCount:      drained.ttftCount,
			OutputTokens:   drained.outputTokens,
			GenerationMs:   drained.generationMs,
		})
		if err != nil {
			bucket.addCounters(drained)
			common.SysError(fmt.Sprintf("failed to flush perf metric bucket model=%s group=%s bucket=%d: %s", k.model, k.group, k.bucketTs, err.Error()))
			return true
		}

		deleteOldEmptyBucket(k, key)
		return true
	})
	flushDetailBuckets()
}

func Flush() {
	pendingRecordings.Wait()
	flushCompletedBuckets()
}

func flushDetailBuckets() {
	detailSnapshotMu.Lock()
	detailFlushGeneration++
	detailFlushDone = make(chan struct{})
	detailSnapshotMu.Unlock()
	defer func() {
		detailSnapshotMu.Lock()
		detailFlushGeneration++
		close(detailFlushDone)
		detailSnapshotMu.Unlock()
	}()
	currentBucket := bucketStart(time.Now().Unix())
	const dimensionBatchSize = 100
	pending := make([]pendingDetailFlush, 0, dimensionBatchSize)
	detailHotBuckets.Range(func(key, value any) bool {
		bucketKey := key.(detailBucketKey)
		bucket := value.(*atomicDetailBucket)
		completed := bucketKey.bucketTs < currentBucket
		drained, ok := bucket.drain(completed)
		if !ok {
			return true
		}
		if completed {
			detailHotBuckets.CompareAndDelete(key, bucket)
		}
		if drained.requestCount == 0 {
			return true
		}

		histograms := make([]model.PerfMetricHistogram, 0, len(histogramUpperBoundsMs)*2)
		for i, upperBoundMs := range histogramUpperBoundsMs {
			if drained.ttftBuckets[i] > 0 {
				histograms = append(histograms, model.PerfMetricHistogram{
					ModelName: bucketKey.model, UserId: bucketKey.userId, TokenId: bucketKey.tokenId,
					BucketTs: bucketKey.bucketTs, Metric: metricTtft, UpperBoundMs: upperBoundMs,
					Count: drained.ttftBuckets[i],
				})
			}
			if drained.tpotBuckets[i] > 0 {
				histograms = append(histograms, model.PerfMetricHistogram{
					ModelName: bucketKey.model, UserId: bucketKey.userId, TokenId: bucketKey.tokenId,
					BucketTs: bucketKey.bucketTs, Metric: metricTpot, UpperBoundMs: upperBoundMs,
					Count: drained.tpotBuckets[i],
				})
			}
		}
		// Sufficient statistics share the existing dimension/bucket transaction.
		// Separate metric names keep them out of the latency histogram and preserve
		// compatibility with existing schemas and readers from older releases.
		for _, statistic := range []struct {
			metric         string
			value, samples int64
		}{
			{metricTtftSum, drained.ttftSumMs, drained.ttftCount},
			{metricTpotSum, drained.tpotGenerationMs, drained.tpotCount},
			{metricTtftAverageCount, drained.ttftCount, drained.ttftCount},
			{metricTpotAverageCount, drained.tpotAverageCount, drained.tpotAverageCount},
			{metricTpotTokens, drained.tpotGenerationTokens, drained.tpotAverageCount},
		} {
			if statistic.samples > 0 {
				histograms = append(histograms, model.PerfMetricHistogram{ModelName: bucketKey.model, UserId: bucketKey.userId, TokenId: bucketKey.tokenId, BucketTs: bucketKey.bucketTs, Metric: statistic.metric, UpperBoundMs: 0, Count: statistic.value})
			}
		}
		pending = append(pending, pendingDetailFlush{
			key:      bucketKey,
			counters: drained,
			detail: model.PerfMetricDetail{
				ModelName: bucketKey.model, UserId: bucketKey.userId, TokenId: bucketKey.tokenId,
				BucketTs: bucketKey.bucketTs, RequestCount: drained.requestCount,
				SuccessCount: drained.successCount, TtftCount: drained.ttftCount, TpotCount: drained.tpotCount,
				InputTokens: drained.inputTokens, OutputTokens: drained.outputTokens,
				TotalTokens: drained.totalTokens, CacheReadTokens: drained.cacheReadTokens,
				RequestBodySamples:  drained.requestBodySamples,
				RequestBodyBytes:    drained.requestBodyBytes,
				RequestBodyMaxBytes: drained.requestBodyMaxBytes,
				CacheReadSamples:    drained.cacheReadSamples,
				CacheWriteTokens:    drained.cacheWriteTokens,
				CacheWriteSamples:   drained.cacheWriteSamples,
				CacheWrite5mTokens:  drained.cacheWrite5mTokens,
				CacheWrite1hTokens:  drained.cacheWrite1hTokens,
			},
			histograms: histograms,
		})
		if len(pending) == dimensionBatchSize {
			persistDetailFlushBatch(pending)
			pending = pending[:0]
		}
		return true
	})
	persistDetailFlushBatch(pending)
}

func persistDetailFlushBatch(pending []pendingDetailFlush) {
	if len(pending) == 0 {
		return
	}
	details := make([]model.PerfMetricDetail, 0, len(pending))
	histograms := make([]model.PerfMetricHistogram, 0, len(pending)*len(histogramUpperBoundsMs)*2)
	for _, item := range pending {
		details = append(details, item.detail)
		histograms = append(histograms, item.histograms...)
	}
	if err := model.UpsertPerfAnalyticsBuckets(details, histograms); err != nil {
		for _, item := range pending {
			restoreDetailCounters(item.key, item.counters)
		}
		common.SysError(fmt.Sprintf("failed to flush %d detailed perf metric buckets: %s", len(pending), err.Error()))
	}
}

func deleteOldEmptyBucket(k bucketKey, rawKey any) {
	if k.bucketTs < bucketStart(time.Now().Add(-24*time.Hour).Unix()) {
		hotBuckets.Delete(rawKey)
	}
}

func cleanupExpiredMetrics(retentionDays int) {
	if retentionDays > 0 {
		cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
		if err := model.DeletePerfMetricsBefore(cutoff); err != nil {
			common.SysError("failed to cleanup expired perf metrics: " + err.Error())
		}
	}
	detailRetentionDays := retentionDays
	if detailRetentionDays <= 0 || detailRetentionDays > 30 {
		detailRetentionDays = 30
	}
	cutoff := time.Now().Add(-time.Duration(detailRetentionDays) * 24 * time.Hour).Unix()
	if err := model.DeletePerfAnalyticsBefore(cutoff); err != nil {
		common.SysError("failed to cleanup expired detailed perf metrics: " + err.Error())
	}
}

func redisCounters(values map[string]string) counters {
	return counters{
		requestCount:   parseRedisInt(values["req"]),
		successCount:   parseRedisInt(values["ok"]),
		totalLatencyMs: parseRedisInt(values["lat"]),
		ttftSumMs:      parseRedisInt(values["ttft"]),
		ttftCount:      parseRedisInt(values["ttft_n"]),
		outputTokens:   parseRedisInt(values["out"]),
		generationMs:   parseRedisInt(values["gen_ms"]),
	}
}

func parseRedisInt(value string) int64 {
	if value == "" {
		return 0
	}
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}
