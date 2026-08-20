package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type refundRecordingBilling struct {
	refundCalls int
}

func (*refundRecordingBilling) Settle(int) error         { return nil }
func (*refundRecordingBilling) NeedsRefund() bool        { return true }
func (*refundRecordingBilling) GetPreConsumedQuota() int { return 100 }
func (*refundRecordingBilling) Reserve(int) error        { return nil }

func (b *refundRecordingBilling) Refund(*gin.Context) {
	b.refundCalls++
}

func TestRefundRelayBillingOnCommittedBusinessFailure(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	billing := &refundRecordingBilling{}
	info := &relaycommon.RelayInfo{Billing: billing}
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "NotEnoughCvError",
		Code:    "11210",
	}, http.StatusTooManyRequests, types.ErrOptionWithResponseCommitted())
	require.NotNil(t, err)

	refundRelayBillingOnFailure(c, info, err)

	assert.Equal(t, 1, billing.refundCalls)
}

func TestShouldRetryRefusesWrittenResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	_, writeErr := c.Writer.Write([]byte(": PING\n\n"))
	require.NoError(t, writeErr)
	err := types.NewOpenAIError(errors.New("retryable upstream failure"), types.ErrorCodeBadResponse, http.StatusServiceUnavailable)

	assert.False(t, shouldRetry(c, err, 1))
}

func TestShouldRetryRefusesForcedChannelEvenForChannelError(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("specific_channel_id", 42)
	err := types.NewError(
		errors.New("forced channel failed"),
		types.ErrorCodeChannelInvalidKey,
	)
	require.True(t, types.IsChannelError(err))

	assert.False(t, shouldRetry(c, err, 1))
}
