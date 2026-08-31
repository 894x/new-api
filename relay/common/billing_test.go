package common

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type admissionBillingSettler struct {
	targets []int
}

func (*admissionBillingSettler) Settle(int) error { return nil }

func (*admissionBillingSettler) Refund(*gin.Context) {}

func (*admissionBillingSettler) NeedsRefund() bool { return false }

func (*admissionBillingSettler) GetPreConsumedQuota() int { return 0 }

func (*admissionBillingSettler) Reserve(int) error { return nil }

func (s *admissionBillingSettler) ReserveForAdmission(target int) error {
	s.targets = append(s.targets, target)
	return nil
}

func TestBillingSettlerRequiresStrictAdmissionReserve(t *testing.T) {
	var billing BillingSettler = &admissionBillingSettler{}

	require.NoError(t, billing.ReserveForAdmission(321))
	assert.Equal(t, []int{321}, billing.(*admissionBillingSettler).targets)
}
