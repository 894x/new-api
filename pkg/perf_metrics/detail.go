package perfmetrics

import (
	"sync"
	"sync/atomic"
	"time"
)

var detailHotBuckets sync.Map

type atomicDetailBucket struct {
	tpotGenerationTokens atomic.Int64
	tpotAverageCount     atomic.Int64
	ttftSumMs            atomic.Int64
	tpotGenerationMs     atomic.Int64
	mu                   sync.Mutex
	sealed               bool
	requestCount         atomic.Int64
	successCount         atomic.Int64
	ttftCount            atomic.Int64
	tpotCount            atomic.Int64
	inputTokens          atomic.Int64
	outputTokens         atomic.Int64
	totalTokens          atomic.Int64
	cacheReadTokens      atomic.Int64
	requestBodySamples   atomic.Int64
	requestBodyBytes     atomic.Int64
	requestBodyMaxBytes  atomic.Int64
	cacheReadSamples     atomic.Int64
	cacheWriteTokens     atomic.Int64
	cacheWriteSamples    atomic.Int64
	cacheWrite5mTokens   atomic.Int64
	cacheWrite1hTokens   atomic.Int64

	ttftBuckets [len(histogramUpperBoundsMs)]atomic.Int64
	tpotBuckets [len(histogramUpperBoundsMs)]atomic.Int64
}

func recordDetail(sample Sample, recordedAt time.Time) {
	if sample.UserId <= 0 {
		return
	}
	key := detailBucketKey{
		model:    sample.Model,
		userId:   sample.UserId,
		tokenId:  sample.TokenId,
		bucketTs: bucketStart(recordedAt.Unix()),
	}
	for {
		actual, _ := detailHotBuckets.LoadOrStore(key, &atomicDetailBucket{})
		bucket := actual.(*atomicDetailBucket)
		if bucket.add(sample) {
			return
		}
		detailHotBuckets.CompareAndDelete(key, bucket)
	}
}

func (bucket *atomicDetailBucket) add(sample Sample) bool {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if bucket.sealed {
		return false
	}

	bucket.requestCount.Add(1)
	if sample.Success {
		bucket.successCount.Add(1)
	}
	if sample.HasTtft && sample.TtftMs >= 0 {
		bucket.ttftCount.Add(1)
		bucket.ttftSumMs.Add(sample.TtftMs)
		bucket.ttftBuckets[histogramBucketIndex(sample.TtftMs)].Add(1)
	}
	if sample.HasTpot && sample.TpotMs >= 0 {
		bucket.tpotCount.Add(1)
		if sample.GenerationMs >= 0 && sample.OutputTokens > 1 {
			bucket.tpotGenerationMs.Add(sample.GenerationMs)
			bucket.tpotGenerationTokens.Add(sample.OutputTokens - 1)
			bucket.tpotAverageCount.Add(1)
		}
		bucket.tpotBuckets[histogramBucketIndex(sample.TpotMs)].Add(1)
	}
	if sample.InputTokens > 0 {
		bucket.inputTokens.Add(sample.InputTokens)
	}
	if sample.OutputTokens > 0 {
		bucket.outputTokens.Add(sample.OutputTokens)
	}
	if sample.TotalTokens > 0 {
		bucket.totalTokens.Add(sample.TotalTokens)
	}
	if sample.CacheReadTokens > 0 {
		bucket.cacheReadTokens.Add(sample.CacheReadTokens)
	}
	if sample.RequestBodyBytes != nil && *sample.RequestBodyBytes >= 0 {
		bucket.requestBodySamples.Add(1)
		bucket.requestBodyBytes.Add(*sample.RequestBodyBytes)
		bucket.requestBodyMaxBytes.Store(max(bucket.requestBodyMaxBytes.Load(), *sample.RequestBodyBytes))
	}
	if sample.CacheReadKnown || sample.CacheReadTokens > 0 {
		bucket.cacheReadSamples.Add(1)
	}
	if sample.CacheWriteKnown || sample.CacheWriteTokens > 0 {
		bucket.cacheWriteSamples.Add(1)
	}
	bucket.cacheWriteTokens.Add(max(sample.CacheWriteTokens, 0))
	bucket.cacheWrite5mTokens.Add(max(sample.CacheWrite5mTokens, 0))
	bucket.cacheWrite1hTokens.Add(max(sample.CacheWrite1hTokens, 0))
	return true
}

func (bucket *atomicDetailBucket) drain(seal bool) (detailCounters, bool) {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if bucket.sealed {
		return detailCounters{}, false
	}
	if seal {
		bucket.sealed = true
	}

	counters := detailCounters{
		tpotGenerationTokens: bucket.tpotGenerationTokens.Swap(0),
		tpotAverageCount:     bucket.tpotAverageCount.Swap(0),
		ttftSumMs:            bucket.ttftSumMs.Swap(0),
		tpotGenerationMs:     bucket.tpotGenerationMs.Swap(0),
		requestCount:         bucket.requestCount.Swap(0),
		successCount:         bucket.successCount.Swap(0),
		ttftCount:            bucket.ttftCount.Swap(0),
		tpotCount:            bucket.tpotCount.Swap(0),
		inputTokens:          bucket.inputTokens.Swap(0),
		outputTokens:         bucket.outputTokens.Swap(0),
		totalTokens:          bucket.totalTokens.Swap(0),
		cacheReadTokens:      bucket.cacheReadTokens.Swap(0),
		requestBodySamples:   bucket.requestBodySamples.Swap(0),
		requestBodyBytes:     bucket.requestBodyBytes.Swap(0),
		requestBodyMaxBytes:  bucket.requestBodyMaxBytes.Swap(0),
		cacheReadSamples:     bucket.cacheReadSamples.Swap(0),
		cacheWriteTokens:     bucket.cacheWriteTokens.Swap(0),
		cacheWriteSamples:    bucket.cacheWriteSamples.Swap(0),
		cacheWrite5mTokens:   bucket.cacheWrite5mTokens.Swap(0),
		cacheWrite1hTokens:   bucket.cacheWrite1hTokens.Swap(0),
	}
	for i := range histogramUpperBoundsMs {
		counters.ttftBuckets[i] = bucket.ttftBuckets[i].Swap(0)
		counters.tpotBuckets[i] = bucket.tpotBuckets[i].Swap(0)
	}
	return counters, true
}

func (bucket *atomicDetailBucket) snapshot() detailCounters {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if bucket.sealed {
		return detailCounters{}
	}

	counters := detailCounters{
		tpotGenerationTokens: bucket.tpotGenerationTokens.Load(),
		tpotAverageCount:     bucket.tpotAverageCount.Load(),
		ttftSumMs:            bucket.ttftSumMs.Load(),
		tpotGenerationMs:     bucket.tpotGenerationMs.Load(),
		requestCount:         bucket.requestCount.Load(),
		successCount:         bucket.successCount.Load(),
		ttftCount:            bucket.ttftCount.Load(),
		tpotCount:            bucket.tpotCount.Load(),
		inputTokens:          bucket.inputTokens.Load(),
		outputTokens:         bucket.outputTokens.Load(),
		totalTokens:          bucket.totalTokens.Load(),
		cacheReadTokens:      bucket.cacheReadTokens.Load(),
		requestBodySamples:   bucket.requestBodySamples.Load(),
		requestBodyBytes:     bucket.requestBodyBytes.Load(),
		requestBodyMaxBytes:  bucket.requestBodyMaxBytes.Load(),
		cacheReadSamples:     bucket.cacheReadSamples.Load(),
		cacheWriteTokens:     bucket.cacheWriteTokens.Load(),
		cacheWriteSamples:    bucket.cacheWriteSamples.Load(),
		cacheWrite5mTokens:   bucket.cacheWrite5mTokens.Load(),
		cacheWrite1hTokens:   bucket.cacheWrite1hTokens.Load(),
	}
	for i := range histogramUpperBoundsMs {
		counters.ttftBuckets[i] = bucket.ttftBuckets[i].Load()
		counters.tpotBuckets[i] = bucket.tpotBuckets[i].Load()
	}
	return counters
}

func (bucket *atomicDetailBucket) addCounters(counters detailCounters) bool {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if bucket.sealed {
		return false
	}

	bucket.requestCount.Add(counters.requestCount)
	bucket.successCount.Add(counters.successCount)
	bucket.ttftCount.Add(counters.ttftCount)
	bucket.ttftSumMs.Add(counters.ttftSumMs)
	bucket.tpotGenerationMs.Add(counters.tpotGenerationMs)
	bucket.tpotGenerationTokens.Add(counters.tpotGenerationTokens)
	bucket.tpotAverageCount.Add(counters.tpotAverageCount)
	bucket.tpotCount.Add(counters.tpotCount)
	bucket.inputTokens.Add(counters.inputTokens)
	bucket.outputTokens.Add(counters.outputTokens)
	bucket.totalTokens.Add(counters.totalTokens)
	bucket.cacheReadTokens.Add(counters.cacheReadTokens)
	bucket.requestBodySamples.Add(counters.requestBodySamples)
	bucket.requestBodyBytes.Add(counters.requestBodyBytes)
	bucket.requestBodyMaxBytes.Store(max(bucket.requestBodyMaxBytes.Load(), counters.requestBodyMaxBytes))
	bucket.cacheReadSamples.Add(counters.cacheReadSamples)
	bucket.cacheWriteTokens.Add(counters.cacheWriteTokens)
	bucket.cacheWriteSamples.Add(counters.cacheWriteSamples)
	bucket.cacheWrite5mTokens.Add(counters.cacheWrite5mTokens)
	bucket.cacheWrite1hTokens.Add(counters.cacheWrite1hTokens)
	for i := range histogramUpperBoundsMs {
		bucket.ttftBuckets[i].Add(counters.ttftBuckets[i])
		bucket.tpotBuckets[i].Add(counters.tpotBuckets[i])
	}
	return true
}

func restoreDetailCounters(key detailBucketKey, counters detailCounters) {
	for {
		actual, _ := detailHotBuckets.LoadOrStore(key, &atomicDetailBucket{})
		bucket := actual.(*atomicDetailBucket)
		if bucket.addCounters(counters) {
			return
		}
		detailHotBuckets.CompareAndDelete(key, bucket)
	}
}

func histogramBucketIndex(valueMs int64) int {
	index := len(histogramUpperBoundsMs) - 1
	for i, upperBoundMs := range histogramUpperBoundsMs {
		if valueMs <= upperBoundMs {
			return i
		}
	}
	return index
}
