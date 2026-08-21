package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendUpstreamResponseAdminInfoCopiesCapturedIdentifiers(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.UpstreamResponseIdKey, "gen_upstream")
	c.Set(common.UpstreamResponseHeadersKey, map[string]string{
		"X-Request-Id": "request-secret",
		"X-Trace-Id":   "trace-secret",
	})
	adminInfo := map[string]interface{}{"use_channel": []int{9}}

	AppendUpstreamResponseAdminInfo(c, adminInfo)

	assert.Equal(t, "gen_upstream", adminInfo["upstream_response_id"])
	headers, ok := adminInfo["upstream_request_ids"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "request-secret", headers["X-Request-Id"])
	assert.Equal(t, "trace-secret", headers["X-Trace-Id"])
	assert.Equal(t, []int{9}, adminInfo["use_channel"])

	headers["X-Trace-Id"] = "changed"
	original, ok := common.GetContextKeyType[map[string]string](c, common.UpstreamResponseHeadersKey)
	require.True(t, ok)
	assert.Equal(t, "trace-secret", original["X-Trace-Id"])
}

func TestGenerateTextOtherInfoAddsAdminOnlyRequestTiming(t *testing.T) {
	start := time.Now().Add(-time.Second)
	timing := common.NewRequestTiming(start)
	timing.Mark(common.RequestTimingBodyRead, start.Add(10*time.Millisecond))
	timing.Mark(common.RequestTimingUpstreamRequestStarted, start.Add(30*time.Millisecond))
	timing.Mark(common.RequestTimingUpstreamRequestWritten, start.Add(40*time.Millisecond))
	timing.Mark(common.RequestTimingUpstreamResponseHeaders, start.Add(120*time.Millisecond))
	timing.Mark(common.RequestTimingFirstResponse, start.Add(180*time.Millisecond))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	relayInfo := &relaycommon.RelayInfo{
		StartTime:         start,
		FirstResponseTime: start.Add(180 * time.Millisecond),
		RequestTiming:     timing,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}

	other := GenerateTextOtherInfo(c, relayInfo, 1, 1, 1, 0, 1, 0, 1)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	snapshot, ok := adminInfo["request_timing"].(common.RequestTimingSnapshot)
	require.True(t, ok)
	assert.Equal(t, start.UnixMilli(), snapshot.RequestReceivedAtMs)
	assert.Equal(t, start.Add(180*time.Millisecond).UnixMilli(), snapshot.FirstResponseAtMs)
	assert.GreaterOrEqual(t, snapshot.RequestCompletedAtMs, snapshot.FirstResponseAtMs)
}
