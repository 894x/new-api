package helper

import (
	"errors"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamResultClassifiesProtocolAndDownstreamFailures(t *testing.T) {
	t.Run("protocol decode", func(t *testing.T) {
		status := relaycommon.NewStreamStatus()
		result := newStreamResult(status)
		err := errors.New("invalid upstream event")
		result.ScannerError(err)

		assert.True(t, result.IsStopped())
		reason, endErr := status.End()
		assert.Equal(t, relaycommon.StreamEndReasonScannerErr, reason)
		require.ErrorIs(t, endErr, err)
	})

	t.Run("downstream write", func(t *testing.T) {
		status := relaycommon.NewStreamStatus()
		result := newStreamResult(status)
		err := errors.New("client write failed")
		result.ClientGone(err)

		assert.True(t, result.IsStopped())
		reason, endErr := status.End()
		assert.Equal(t, relaycommon.StreamEndReasonClientGone, reason)
		require.ErrorIs(t, endErr, err)
	})
}
