package common

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTransportSnapshotsRetainBoundedLatestAttempts(t *testing.T) {
	timing := NewRequestTiming(time.Now())
	for i := 0; i < 10; i++ {
		timing.AddUpstreamTransport(&UpstreamTransportAttempt{})
	}
	snapshots := timing.UpstreamTransports()
	require.Len(t, snapshots, 8)
	assert.Equal(t, 3, snapshots[0].Attempt)
	assert.Equal(t, 10, snapshots[7].Attempt)
	var absent *RequestTiming
	assert.Nil(t, absent.UpstreamTransports())
}
