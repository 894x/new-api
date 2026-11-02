package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTrafficObservesEachUpstreamAttempt(t *testing.T) {
	for _, tc := range []struct {
		name             string
		status           int
		stream           bool
		end              relaycommon.StreamEndReason
		terminal         bool
		cancelled, local bool
		want             string
	}{
		{name: "non-stream success", status: 200, want: "healthy"},
		{name: "400 remains raw for configurable exclusion", status: 400, want: "unavailable"},
		{name: "503 remains a failure", status: 503, want: "unavailable"},
		{name: "completed stream", status: 200, stream: true, end: relaycommon.StreamEndReasonDone, want: "healthy"},
		{name: "broken stream", status: 200, stream: true, end: relaycommon.StreamEndReasonScannerErr, want: "unavailable"},
		{name: "missing protocol terminal", status: 200, stream: true, terminal: true, end: relaycommon.StreamEndReasonEOF, want: "unavailable"},
		{name: "client cancellation", status: 200, cancelled: true},
		{name: "downstream disconnect", status: 200, stream: true, end: relaycommon.StreamEndReasonClientGone},
		{name: "local failure before upstream", status: 400, local: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			info := &relaycommon.RelayInfo{IsStream: tc.stream}
			info.BeginChannelMonitoringAttempt(11, 1, "public-model")
			if !tc.local {
				info.MarkAttemptUpstreamStarted()
			}
			info.SetAttemptHTTPStatus(tc.status)
			if tc.stream {
				info.SetFirstResponseTime()
				info.StreamStatus = relaycommon.NewStreamStatus()
				info.StreamStatus.SetEndReason(tc.end, nil)
				if tc.terminal {
					info.StreamStatus.RequireTerminal()
				}
			}
			if tc.cancelled {
				cancel()
			}
			var apiErr *types.NewAPIError
			if tc.status >= 400 {
				apiErr = types.NewOpenAIError(errors.New("private upstream body"), types.ErrorCodeBadResponse, 502)
			}
			record, ok := ChannelTrafficObservation(ctx, info, "openai", apiErr, 300000)
			assert.Equal(t, tc.want != "", ok)
			if ok {
				assert.Equal(t, tc.want, record.State)
				assert.Equal(t, tc.status, record.StatusCode, "client status mapping must not change monitoring exclusions")
				assert.Equal(t, 11, record.ChannelID)
				assert.Equal(t, "public-model", record.Model)
				assert.NotContains(t, record.ErrorCode, "private")
			}
		})
	}
	info := &relaycommon.RelayInfo{}
	for _, id := range []int{11, 22} {
		info.BeginChannelMonitoringAttempt(id, 1, "public-model")
		info.MarkAttemptUpstreamStarted()
		status := 503
		if id == 22 {
			status = 200
		}
		info.SetAttemptHTTPStatus(status)
		record, ok := ChannelTrafficObservation(context.Background(), info, "openai", nil, 300000)
		require.True(t, ok)
		assert.Equal(t, id, record.ChannelID)
		if id == 11 {
			assert.Equal(t, "unavailable", record.State)
		} else {
			assert.Equal(t, "healthy", record.State)
		}
		info.FinishDynamicRoutingAttempt(nil)
	}
}

func TestChannelTrafficCollectorRetriesAndBoundsMemory(t *testing.T) {
	now := time.Unix(1800000000, 0)
	rows := make(map[string]model.ChannelMonitorRecord)
	fail := true
	collector := channelTrafficCollector{runID: "test-run", limit: 2, records: make(map[channelTrafficKey]model.ChannelMonitorRecord)}
	collector.save = func(_ context.Context, records []model.ChannelMonitorRecord) error {
		for _, r := range records {
			rows[r.SampleKey] = r
		}
		if fail {
			return errors.New("ambiguous commit")
		}
		return nil
	}
	r := model.ChannelMonitorRecord{Source: "traffic", Count: 1, ChannelID: 11, Model: "model-a", EndpointType: "openai", State: "healthy", StatusCode: 200, CreatedAt: now.Unix(), BucketAt: now.Unix() / 900 * 900}
	collector.observe(r)
	collector.observe(r)
	require.Error(t, collector.flush(context.Background(), now))
	fail = false
	collector.observe(r)
	require.NoError(t, collector.flush(context.Background(), now))
	require.Len(t, rows, 1)
	for _, row := range rows {
		assert.Equal(t, 3, row.Count)
	}
	r.ChannelID = 22
	collector.observe(r)
	r.ChannelID = 33
	collector.observe(r)
	require.NoError(t, collector.flush(context.Background(), now.Add(15*time.Minute)))
	assert.Empty(t, collector.records, "acknowledged closed intervals can leave memory")
	var total, gaps int
	for _, row := range rows {
		if row.Source == "gap" {
			gaps++
		} else {
			total += row.Count
		}
	}
	assert.Equal(t, 4, total)
	assert.Equal(t, 1, gaps, "overflow must invalidate completeness rather than silently inflate availability")
	// A late observation of a released interval starts a new idempotent segment.
	r.ChannelID = 11
	collector.observe(r)
	require.NoError(t, collector.flush(context.Background(), now.Add(15*time.Minute)))
	total = 0
	for _, row := range rows {
		if row.Source == "traffic" {
			total += row.Count
		}
	}
	assert.Equal(t, 5, total)
}

func TestChannelTrafficRegistrationUsesChannelSwitchAndOriginalProtocol(t *testing.T) {
	channel := &model.Channel{Models: "model-a,model-b", MonitorConfig: `{"targets":[{"model":"model-a","endpoint_type":"openai"}]}`}
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}
	assert.Equal(t, "anthropic", ChannelTrafficMonitoringEndpoint(channel, info, "model-b"), "active target configuration does not filter real traffic")
	assert.Equal(t, "anthropic", ChannelTrafficMonitoringEndpoint(channel, info, "model-a-high"), "routed model modifiers retain their public identity")
	channel.MonitorConfig = `{"enabled":false}`
	assert.Empty(t, ChannelTrafficMonitoringEndpoint(channel, info, "model-b"))
	channel.MonitorConfig = ""
	info.IsChannelTest = true
	assert.Empty(t, ChannelTrafficMonitoringEndpoint(channel, info, "model-a"), "active probes must not be recorded twice")
}

func TestChannelTrafficFlushKeepsObservationsArrivingDuringWrite(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := model.ChannelMonitorRecord{Source: "traffic", ChannelID: 11, Model: "model-a", EndpointType: "openai", State: "healthy", StatusCode: 200, CreatedAt: now.Unix(), BucketAt: now.Unix() / 900 * 900}
	collector := channelTrafficCollector{runID: "interleaving", limit: 2, records: make(map[channelTrafficKey]model.ChannelMonitorRecord)}
	collector.observe(r)
	var saved int
	collector.save = func(_ context.Context, records []model.ChannelMonitorRecord) error {
		saved = records[0].Count
		collector.observe(r)
		return nil
	}
	require.NoError(t, collector.flush(context.Background(), now.Add(15*time.Minute)))
	assert.Equal(t, 1, saved)
	require.Len(t, collector.records, 1, "the newer in-memory count must not be retired with the older snapshot")
	collector.save = func(_ context.Context, records []model.ChannelMonitorRecord) error {
		saved = records[0].Count
		return nil
	}
	require.NoError(t, collector.flush(context.Background(), now.Add(15*time.Minute)))
	assert.Equal(t, 2, saved)
	assert.Empty(t, collector.records)
}
