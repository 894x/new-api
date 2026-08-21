package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRequestTimingKeepsFirstMilestoneAndReturnsOrderedSnapshot(t *testing.T) {
	start := time.UnixMilli(1_700_000_000_000)
	timing := NewRequestTiming(start)

	timing.Mark(RequestTimingBodyRead, start.Add(20*time.Millisecond))
	timing.Mark(RequestTimingUpstreamRequestStarted, start.Add(45*time.Millisecond))
	timing.Mark(RequestTimingUpstreamRequestWritten, start.Add(60*time.Millisecond))
	timing.Mark(RequestTimingUpstreamResponseHeaders, start.Add(160*time.Millisecond))
	timing.Mark(RequestTimingFirstResponse, start.Add(220*time.Millisecond))
	timing.Mark(RequestTimingCompleted, start.Add(420*time.Millisecond))
	// A repeated callback must not move the original milestone.
	timing.Mark(RequestTimingFirstResponse, start.Add(300*time.Millisecond))

	snapshot := timing.Snapshot()

	assert.Equal(t, start.UnixMilli(), snapshot.RequestReceivedAtMs)
	assert.Equal(t, start.Add(20*time.Millisecond).UnixMilli(), snapshot.RequestBodyReadAtMs)
	assert.Equal(t, start.Add(45*time.Millisecond).UnixMilli(), snapshot.UpstreamRequestStartedAtMs)
	assert.Equal(t, start.Add(60*time.Millisecond).UnixMilli(), snapshot.UpstreamRequestWrittenAtMs)
	assert.Equal(t, start.Add(160*time.Millisecond).UnixMilli(), snapshot.UpstreamResponseHeadersAtMs)
	assert.Equal(t, start.Add(220*time.Millisecond).UnixMilli(), snapshot.FirstResponseAtMs)
	assert.Equal(t, start.Add(420*time.Millisecond).UnixMilli(), snapshot.RequestCompletedAtMs)
}

func TestRequestTimingSnapshotLeavesUnobservedMilestonesEmpty(t *testing.T) {
	start := time.UnixMilli(1_700_000_000_000)
	timing := NewRequestTiming(start)

	snapshot := timing.Snapshot()

	assert.Equal(t, start.UnixMilli(), snapshot.RequestReceivedAtMs)
	assert.Zero(t, snapshot.RequestBodyReadAtMs)
	assert.Zero(t, snapshot.FirstResponseAtMs)
	assert.Zero(t, snapshot.RequestCompletedAtMs)
}
