package groupdiscount

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func progressBasisTestPolicy(ratios ...float64) Policy {
	tiers := make([]Tier, 0, len(ratios))
	for index, ratio := range ratios {
		threshold := int64(index * 16)
		tiers = append(tiers, Tier{MinMonthlyOriginalQuota: threshold, Ratio: ratio})
	}
	return Policy{
		Enabled:       true,
		EffectiveFrom: 0,
		Timezone:      "UTC",
		Tiers:         tiers,
	}
}

func progressBasisTestSnapshot(basis ProgressBasis, tiers []Tier) Snapshot {
	return Snapshot{
		PolicyHash:    "progress-policy",
		ProgressBasis: basis,
		UsingGroup:    "vip",
		OriginModel:   "gpt-5",
		MatchedModel:  "gpt-5",
		Timezone:      "UTC",
		PeriodStart:   1,
		PeriodEnd:     2,
		Tiers:         tiers,
	}
}

func TestParsePoliciesJSONDefaultsLegacyGroupToOriginalAndMarshalsCanonicalWrapper(t *testing.T) {
	policies, err := ParsePoliciesJSON(`{
		"vip": {
			"gpt-5": {
				"enabled": true,
				"effective_from": 0,
				"effective_until": null,
				"timezone": "UTC",
				"tiers": [{"min_monthly_original_quota": 0, "ratio": 0.8}]
			}
		}
	}`)
	require.NoError(t, err)
	require.Contains(t, policies, "vip")
	assert.Equal(t, ProgressBasisOriginal, policies["vip"].ProgressBasis)
	require.Contains(t, policies["vip"].Models, "gpt-5")

	encoded, err := common.Marshal(policies)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"vip": {
			"progress_basis": "original",
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
	}`, string(encoded))
}

func TestParsePoliciesJSONKeepsLegacyModelsNamedLikeWrapperFields(t *testing.T) {
	for _, model := range []string{"models", "progress_basis"} {
		t.Run(model, func(t *testing.T) {
			raw := `{"vip":{"` + model + `":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`
			policies, err := ParsePoliciesJSON(raw)
			require.NoError(t, err)
			assert.Equal(t, ProgressBasisOriginal, policies["vip"].ProgressBasis)
			require.Contains(t, policies["vip"].Models, model)
		})
	}
}

func TestParsePoliciesJSONRejectsMalformedCanonicalWrapperInsteadOfTreatingItAsLegacy(t *testing.T) {
	tests := []string{
		`{"vip":{"progress_basis":"charged"}}`,
		`{"vip":{"models":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}}`,
		`{"vip":{"progress_basis":"charged","models":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}},"unexpected":true}}`,
	}
	for _, raw := range tests {
		assert.Error(t, ValidatePoliciesJSON(raw))
	}
}

func TestParsePoliciesJSONRejectsDuplicateObjectKeysAtEveryConfigurationLevel(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "duplicate group including escaped equivalent",
			raw:  `{"vip":{},"v\u0069p":{}}`,
		},
		{
			name: "duplicate wrapper field",
			raw:  `{"vip":{"progress_basis":"original","progress_basis":"charged","models":{}}}`,
		},
		{
			name: "duplicate model",
			raw:  `{"vip":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]},"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.7}]}}}`,
		},
		{
			name: "duplicate model including escaped equivalent",
			raw:  `{"vip":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]},"\u0067pt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.7}]}}}`,
		},
		{
			name: "duplicate policy money field",
			raw:  `{"vip":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}],"tiers":[{"min_monthly_original_quota":0,"ratio":0.7}]}}}`,
		},
		{
			name: "duplicate tier ratio",
			raw:  `{"vip":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8,"ratio":0.7}]}}}`,
		},
		{
			name: "duplicate tier ratio including escaped equivalent",
			raw:  `{"vip":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8,"\u0072atio":0.7}]}}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Error(t, ValidatePoliciesJSON(test.raw))
		})
	}
}

func TestParsePoliciesJSONRejectsPrototypeKeyAtGroupAndModelBoundaries(t *testing.T) {
	policy := `{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}`
	tests := []struct {
		name string
		raw  string
	}{
		{name: "legacy group", raw: `{"__proto__":{"gpt":` + policy + `}}`},
		{name: "escaped legacy group", raw: `{"\u005f\u005fproto\u005f\u005f":{"gpt":` + policy + `}}`},
		{name: "legacy model", raw: `{"vip":{"__proto__":` + policy + `}}`},
		{name: "canonical group", raw: `{"__proto__":{"progress_basis":"original","models":{"gpt":` + policy + `}}}`},
		{name: "canonical model", raw: `{"vip":{"progress_basis":"original","models":{"__proto__":` + policy + `}}}`},
		{name: "escaped canonical model", raw: `{"vip":{"progress_basis":"original","models":{"\u005f\u005fproto\u005f\u005f":` + policy + `}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Error(t, ValidatePoliciesJSON(test.raw))
		})
	}
}

func TestParsePoliciesJSONRejectsInvalidBasisAndZeroRatioInChargedMode(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "unknown basis",
			raw:  `{"vip":{"progress_basis":"net","models":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}}`,
		},
		{
			name: "charged progress cannot advance through a zero ratio",
			raw:  `{"vip":{"progress_basis":"charged","models":{"gpt":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0}]}}}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Error(t, ValidatePoliciesJSON(test.raw))
		})
	}
}

func TestFreezePolicyIncludesProgressBasisInSnapshotAndPolicyHash(t *testing.T) {
	policy := progressBasisTestPolicy(0.8)
	requestAt := time.Unix(1, 0)
	original, active, err := FreezePolicyWithBasis(policy, ProgressBasisOriginal, "vip", "gpt", "gpt", requestAt)
	require.NoError(t, err)
	require.True(t, active)
	charged, active, err := FreezePolicyWithBasis(policy, ProgressBasisCharged, "vip", "gpt", "gpt", requestAt)
	require.NoError(t, err)
	require.True(t, active)

	assert.Equal(t, ProgressBasisOriginal, original.ProgressBasis)
	assert.Equal(t, ProgressBasisCharged, charged.ProgressBasis)
	assert.NotEqual(t, original.PolicyHash, charged.PolicyHash)
}

func TestCalculateChargedProgressAdvancesByExactDiscountedAmount(t *testing.T) {
	snapshot := progressBasisTestSnapshot(ProgressBasisCharged, []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.8}})

	calculation, err := CalculateWithProgress(snapshot, 0, "0", 20)
	require.NoError(t, err)

	assert.Equal(t, 16, calculation.ChargedQuota)
	assert.Equal(t, "0", calculation.MonthlyProgressBefore)
	assert.Equal(t, "16", calculation.MonthlyProgressAfter)
	assert.Equal(t, "16", calculation.ProgressQuota)
	require.Len(t, calculation.Segments, 1)
	assert.Equal(t, "20", calculation.Segments[0].OriginalQuotaExact)
	assert.Equal(t, "16", calculation.Segments[0].ProgressQuota)
}

func TestCalculateChargedProgressCrossesExactBoundaryAndIsSplitInvariant(t *testing.T) {
	snapshot := progressBasisTestSnapshot(ProgressBasisCharged, []Tier{
		{MinMonthlyOriginalQuota: 0, Ratio: 0.8},
		{MinMonthlyOriginalQuota: 16, Ratio: 0.5},
	})

	combined, err := CalculateWithProgress(snapshot, 0, "10", 10)
	require.NoError(t, err)
	assert.Equal(t, "17.25", combined.MonthlyProgressAfter)
	assert.Equal(t, "7.25", combined.ProgressQuota)
	assert.Equal(t, 7, combined.ChargedQuota)
	require.Len(t, combined.Segments, 2)
	assert.Equal(t, "7.5", combined.Segments[0].OriginalQuotaExact)
	assert.Equal(t, "6", combined.Segments[0].ProgressQuota)
	assert.Equal(t, "2.5", combined.Segments[1].OriginalQuotaExact)
	assert.Equal(t, "1.25", combined.Segments[1].ProgressQuota)

	first, err := CalculateWithProgress(snapshot, 0, "10", 3)
	require.NoError(t, err)
	second, err := CalculateWithProgress(snapshot, 3, first.MonthlyProgressAfter, 7)
	require.NoError(t, err)
	assert.Equal(t, combined.MonthlyProgressAfter, second.MonthlyProgressAfter)
	assert.Equal(t, combined.ChargedQuota, first.ChargedQuota+second.ChargedQuota)
}

func TestCalculateChargedProgressDecreaseWalksBackwardFromExactCursor(t *testing.T) {
	snapshot := progressBasisTestSnapshot(ProgressBasisCharged, []Tier{
		{MinMonthlyOriginalQuota: 0, Ratio: 0.8},
		{MinMonthlyOriginalQuota: 16, Ratio: 0.5},
	})

	decrease, err := CalculateDecrease(snapshot, 28, "20", 8)
	require.NoError(t, err)
	assert.Equal(t, int64(20), decrease.MonthlyOriginalAfter)
	assert.Equal(t, "16", decrease.MonthlyProgressAfter)
	assert.Equal(t, "-4", decrease.ProgressQuota)
	assert.Equal(t, -4, decrease.ChargedQuota)
	require.Len(t, decrease.Segments, 1)
	assert.Equal(t, "8", decrease.Segments[0].OriginalQuotaExact)
	assert.Equal(t, "-4", decrease.Segments[0].ProgressQuota)
}

func TestCalculateChargedProgressDecreaseRestoresCanonicalCursorAcrossRecurringBoundary(t *testing.T) {
	snapshot := progressBasisTestSnapshot(ProgressBasisCharged, []Tier{
		{MinMonthlyOriginalQuota: 0, Ratio: 0.3},
		{MinMonthlyOriginalQuota: 1, Ratio: 0.2},
	})

	combined, err := CalculateWithProgress(snapshot, 0, "0", 11)
	require.NoError(t, err)
	assert.Equal(t, "2.53333333333333334", combined.MonthlyProgressAfter)
	canonicalRemaining, err := CalculateWithProgress(snapshot, 0, "0", 1)
	require.NoError(t, err)
	assert.Equal(t, "0.3", canonicalRemaining.MonthlyProgressAfter)

	decrease, err := CalculateDecrease(snapshot, 11, combined.MonthlyProgressAfter, 10)
	require.NoError(t, err)
	assert.Equal(t, canonicalRemaining.MonthlyProgressAfter, decrease.MonthlyProgressAfter)
	assert.Equal(t, "-2.23333333333333334", decrease.ProgressQuota)

	restored, err := CalculateWithProgress(snapshot, decrease.MonthlyOriginalAfter, decrease.MonthlyProgressAfter, 10)
	require.NoError(t, err)
	assert.Equal(t, combined.MonthlyProgressAfter, restored.MonthlyProgressAfter)
}
