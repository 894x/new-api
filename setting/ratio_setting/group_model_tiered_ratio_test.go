package ratio_setting

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateModelTieredRatiosRejectsInvalidConfigurationWithoutReplacingLivePolicy(t *testing.T) {
	original := ModelTieredRatios2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateModelTieredRatiosByJSONString(original)) })

	valid := `{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	require.NoError(t, UpdateModelTieredRatiosByJSONString(valid))

	invalid := `{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":1,"ratio":0.9}]}}}`
	require.Error(t, UpdateModelTieredRatiosByJSONString(invalid))

	snapshot, active, err := ResolveModelTieredDiscount("default", "vip", "gpt-5", time.Unix(10, 0))
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, "gpt-5", snapshot.MatchedModel)
	assert.Equal(t, 0.9, snapshot.Tiers[0].Ratio)
}

func TestCheckModelTieredRatiosRejectsGroupMissingFromPricingGroups(t *testing.T) {
	originalGroups := GroupRatio2JSONString()
	originalTiered := ModelTieredRatios2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(originalGroups))
		require.NoError(t, UpdateModelTieredRatiosByJSONString(originalTiered))
	})

	require.NoError(t, UpdateModelTieredRatiosByJSONString(`{}`))
	require.NoError(t, UpdateGroupRatioByJSONString(`{"premium":1}`))

	err := CheckModelTieredRatios(`{"orphan":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`)
	assert.EqualError(t, err, `model tiered ratio group "orphan" is not configured in GroupRatio`)
}

func TestCheckGroupRatioRejectsRemovingGroupReferencedByTieredPolicy(t *testing.T) {
	originalGroups := GroupRatio2JSONString()
	originalTiered := ModelTieredRatios2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(originalGroups))
		require.NoError(t, UpdateModelTieredRatiosByJSONString(originalTiered))
	})

	require.NoError(t, UpdateModelTieredRatiosByJSONString(`{}`))
	require.NoError(t, UpdateGroupRatioByJSONString(`{"premium":1}`))
	require.NoError(t, UpdateModelTieredRatiosByJSONString(`{"premium":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`))

	err := CheckGroupRatio(`{"default":1}`)
	assert.EqualError(t, err, `model tiered ratio group "premium" is not configured in GroupRatio`)
}

func TestResolveModelTieredDiscountUsesWildcardButDefersToGroupGroupContract(t *testing.T) {
	originalTiered := ModelTieredRatios2JSONString()
	originalContracts := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelTieredRatiosByJSONString(originalTiered))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalContracts))
	})

	require.NoError(t, UpdateModelTieredRatiosByJSONString(`{"vip":{"*":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"contract-user":{"vip":0.7}}`))

	_, active, err := ResolveModelTieredDiscount("contract-user", "vip", "claude", time.Unix(10, 0))
	require.NoError(t, err)
	assert.False(t, active, "a GroupGroupRatio contract has precedence over dynamic discounts")

	snapshot, active, err := ResolveModelTieredDiscount("ordinary-user", "vip", "claude", time.Unix(10, 0))
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, "*", snapshot.MatchedModel)
	assert.Equal(t, "claude", snapshot.OriginModel)
}

func TestGroupRatioConfigExportsModelTieredRatiosOption(t *testing.T) {
	setting := GetGroupRatioSetting()
	require.NotNil(t, setting.ModelTieredRatios)
	assert.JSONEq(t, `{}`, ModelTieredRatios2JSONString())
}

func TestCapturedModelTieredDiscountResolverIsImmutableAcrossConfigChanges(t *testing.T) {
	originalTiered := ModelTieredRatios2JSONString()
	originalContracts := GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelTieredRatiosByJSONString(originalTiered))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalContracts))
	})

	require.NoError(t, UpdateModelTieredRatiosByJSONString(`{
		"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}},
		"svip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}
	}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"contract-user":{"vip":0.7}}`))

	requestAt := time.Unix(10, 0)
	ordinaryResolver := CaptureModelTieredDiscountResolver("ordinary-user", "gpt-5", requestAt)
	contractResolver := CaptureModelTieredDiscountResolver("contract-user", "gpt-5", requestAt)

	// Mutating both live policy and contract settings after admission must not
	// affect either same-group settlement or an auto-group retry.
	require.NoError(t, UpdateModelTieredRatiosByJSONString(`{
		"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.5}]}},
		"svip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.4}]}}
	}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{}`))

	vip, active, err := ordinaryResolver.Resolve("vip")
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, 0.9, vip.Tiers[0].Ratio)
	svip, active, err := ordinaryResolver.Resolve("svip")
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, 0.8, svip.Tiers[0].Ratio)

	_, active, err = contractResolver.Resolve("vip")
	require.NoError(t, err)
	assert.False(t, active, "the admission-time GroupGroup contract remains authoritative even if it is later removed")

	newResolver := CaptureModelTieredDiscountResolver("contract-user", "gpt-5", requestAt)
	newVIP, active, err := newResolver.Resolve("vip")
	require.NoError(t, err)
	require.True(t, active, "a resolver admitted after replacement must see the removed contract")
	assert.Equal(t, 0.5, newVIP.Tiers[0].Ratio, "a resolver admitted after replacement must see the new policy map")
}

func TestModelTieredRatiosCanonicalGroupWrapperFreezesChargedProgressBasis(t *testing.T) {
	original := ModelTieredRatios2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateModelTieredRatiosByJSONString(original)) })

	require.NoError(t, UpdateModelTieredRatiosByJSONString(`{
		"vip": {
			"progress_basis": "charged",
			"models": {
				"gpt-5": {
					"enabled": true,
					"effective_from": 0,
					"effective_until": null,
					"timezone": "UTC",
					"tiers": [{"min_monthly_original_quota": 0, "ratio": 0.8}]
				}
			}
		}
	}`))

	assert.JSONEq(t, `{
		"vip": {
			"progress_basis": "charged",
			"models": {
				"gpt-5": {
					"enabled": true,
					"effective_from": 0,
					"effective_until": null,
					"timezone": "UTC",
					"tiers": [{"min_monthly_original_quota": 0, "ratio": 0.8}]
				}
			}
		}
	}`, ModelTieredRatios2JSONString())

	resolver := CaptureModelTieredDiscountResolver("ordinary-user", "gpt-5", time.Unix(10, 0))
	snapshot, active, err := resolver.Resolve("vip")
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, groupdiscount.ProgressBasisCharged, snapshot.ProgressBasis)
}
