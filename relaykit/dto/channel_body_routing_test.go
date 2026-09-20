package dto

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLargeBodyTransportValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings ChannelSettings
		valid    bool
	}{
		{"legacy", ChannelSettings{}, true},
		{"enabled", ChannelSettings{HTTP1LargeBodyEnabled: true, HTTP1LargeBodyThresholdBytes: 1024}, true},
		{"missing threshold", ChannelSettings{HTTP1LargeBodyEnabled: true}, false},
		{"negative", ChannelSettings{HTTP1LargeBodyThresholdBytes: -1}, false},
		{"too large", ChannelSettings{HTTP1LargeBodyThresholdBytes: 1<<30 + 1}, false},
		{"forced http1", ChannelSettings{HTTPProtocol: "http1", HTTP1LargeBodyEnabled: true, HTTP1LargeBodyThresholdBytes: 1024}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.settings.ValidateHTTPTransport()
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
