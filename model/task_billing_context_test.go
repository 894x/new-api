package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskBillingContextRoundTripsDurableDiscountSettlement(t *testing.T) {
	const raw = `{
		"model_price": 0.02,
		"group_ratio": 0.75,
		"model_ratio": 2,
		"other_ratios": {"seconds": 2},
		"origin_model_name": "video-model",
		"original_quota": 600,
		"net_quota": 450,
		"discount_settlement_id": "task:task_public_id",
		"refunded_quota": 0,
		"charge_state": "charged",
		"refund_state": "funding_applied",
		"group_model_discount_snapshot": {
			"policy_hash": "policy-v1",
			"using_group": "vip",
			"origin_model": "video-model",
			"matched_model": "video-model",
			"timezone": "Asia/Shanghai",
			"period_start": 100,
			"period_end": 200,
			"tiers": [{"min_monthly_original_quota": 0, "ratio": 0.75}]
		}
	}`

	var billingContext TaskBillingContext
	require.NoError(t, common.Unmarshal([]byte(raw), &billingContext))

	encoded, err := common.Marshal(billingContext)
	require.NoError(t, err)
	var actual map[string]any
	require.NoError(t, common.Unmarshal(encoded, &actual))

	assert.Equal(t, float64(600), actual["original_quota"])
	assert.Equal(t, float64(450), actual["net_quota"])
	assert.Equal(t, "task:task_public_id", actual["discount_settlement_id"])
	assert.Equal(t, TaskChargeStateCharged, actual["charge_state"])
	assert.Equal(t, TaskRefundStateFundingApplied, actual["refund_state"])
	snapshot, ok := actual["group_model_discount_snapshot"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "policy-v1", snapshot["policy_hash"])
}
