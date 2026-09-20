package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelTransportAuditCapturesOnlySafeChanges(t *testing.T) {
	before := dto.ChannelSettings{Proxy: "http://user:secret@example.test", HTTP2ConnectionShards: 1}
	after := dto.ChannelSettings{Proxy: "http://other:password@example.test", HTTP2ConnectionShards: 6, HTTP1LargeBodyEnabled: true, HTTP1LargeBodyThresholdBytes: 1 << 20}
	changes := channelTransportChanges(before, after)
	require.Len(t, changes, 3)
	assert.Equal(t, map[string]any{"before": 1, "after": 6}, changes["http2_connection_shards"])
	encoded, err := common.Marshal(changes)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret")
	assert.NotContains(t, string(encoded), "password")
	assert.NotContains(t, string(encoded), "proxy")
	assert.Empty(t, channelTransportChanges(after, after))
}
