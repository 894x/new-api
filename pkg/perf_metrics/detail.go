package perfmetrics

import (
	"sync"
	"sync/atomic"
	"time"
)

var detailHotBuckets sync.Map

type atomicDetailBucket struct {
	mu              sync.Mutex
	sealed          bool
	requestCount    atomic.Int64
	successCount    atomic.Int64
	ttftCount       atomic.Int64
	tpotCount       atomic.Int64
	inputTokens     atomic.Int64
	outputTokens    atomic.Int64
	totalTokens     atomic.Int64
	cacheReadTokens atomic.Int64
	ttftBuckets     [len(histogramUpperBoundsMs)]atomic.Int64
	tpotBuckets     [len(histogramUpperBoundsMs)]atomic.Int64
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
		bucket.ttftBuckets[histogramBucketIndex(sample.TtftMs)].Add(1)
	}
	if sample.HasTpot && sample.TpotMs >= 0 {
		bucket.tpotCount.Add(1)
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
		requestCount:    bucket.requestCount.Swap(0),
		successCount:    bucket.successCount.Swap(0),
		ttftCount:       bucket.ttftCount.Swap(0),
		tpotCount:       bucket.tpotCount.Swap(0),
		inputTokens:     bucket.inputTokens.Swap(0),
		outputTokens:    bucket.outputTokens.Swap(0),
		totalTokens:     bucket.totalTokens.Swap(0),
		cacheReadTokens: bucket.cacheReadTokens.Swap(0),
	}
	for i := range histogramUpperBoundsMs {
		counters.ttftBuckets[i] = bucket.ttftBuckets[i].Swap(0)
		counters.tpotBuckets[i] = bucket.tpotBuckets[i].Swap(0)
	}
	return counters, true
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
	bucket.tpotCount.Add(counters.tpotCount)
	bucket.inputTokens.Add(counters.inputTokens)
	bucket.outputTokens.Add(counters.outputTokens)
	bucket.totalTokens.Add(counters.totalTokens)
	bucket.cacheReadTokens.Add(counters.cacheReadTokens)
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
