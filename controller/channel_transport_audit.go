package controller

import "github.com/QuantumNous/new-api/relaykit/dto"

// Explicit allowlist: never include channel keys, request data or proxy URLs.
func channelTransportChanges(before, after dto.ChannelSettings) map[string]any {
	changes := make(map[string]any)
	if before.HTTPProtocol != after.HTTPProtocol {
		changes["http_protocol"] = map[string]any{"before": before.HTTPProtocol, "after": after.HTTPProtocol}
	}
	if before.HTTP2ConnectionShards != after.HTTP2ConnectionShards {
		changes["http2_connection_shards"] = map[string]any{"before": before.HTTP2ConnectionShards, "after": after.HTTP2ConnectionShards}
	}
	if before.HTTP1LargeBodyEnabled != after.HTTP1LargeBodyEnabled {
		changes["http1_large_body_enabled"] = map[string]any{"before": before.HTTP1LargeBodyEnabled, "after": after.HTTP1LargeBodyEnabled}
	}
	if before.HTTP1LargeBodyThresholdBytes != after.HTTP1LargeBodyThresholdBytes {
		changes["http1_large_body_threshold_bytes"] = map[string]any{"before": before.HTTP1LargeBodyThresholdBytes, "after": after.HTTP1LargeBodyThresholdBytes}
	}
	return changes
}
