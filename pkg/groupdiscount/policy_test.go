package groupdiscount

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateSegmentsAcrossTierAndRoundsOnce(t *testing.T) {
	snapshot := Snapshot{
		PolicyHash:  "policy-v1",
		Timezone:    "UTC",
		PeriodStart: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC).Unix(),
		PeriodEnd:   time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC).Unix(),
		Tiers: []Tier{
			{MinMonthlyOriginalQuota: 0, Ratio: 0.9},
			{MinMonthlyOriginalQuota: 1000, Ratio: 0.85},
		},
	}

	calculation, err := Calculate(snapshot, 900, 300)
	require.NoError(t, err)
	assert.Equal(t, int64(900), calculation.MonthlyOriginalBefore)
	assert.Equal(t, int64(1200), calculation.MonthlyOriginalAfter)
	assert.Equal(t, 260, calculation.ChargedQuota)
	require.Len(t, calculation.Segments, 2)
	assert.Equal(t, Segment{TierMin: 0, OriginalQuota: 100, Ratio: 0.9}, calculation.Segments[0])
	assert.Equal(t, Segment{TierMin: 1000, OriginalQuota: 200, Ratio: 0.85}, calculation.Segments[1])
}

func TestCalculateUsesCumulativeRoundingSoRequestSplittingCannotChangeCharge(t *testing.T) {
	snapshot := Snapshot{
		PolicyHash:  "policy-v1",
		Timezone:    "UTC",
		PeriodStart: 1,
		PeriodEnd:   2,
		Tiers: []Tier{
			{MinMonthlyOriginalQuota: 0, Ratio: 0.5},
			{MinMonthlyOriginalQuota: 1, Ratio: 0.5},
		},
	}

	single, err := Calculate(snapshot, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 1, single.ChargedQuota)
	require.Len(t, single.Segments, 2)

	first, err := Calculate(snapshot, 0, 1)
	require.NoError(t, err)
	second, err := Calculate(snapshot, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, single.ChargedQuota, first.ChargedQuota+second.ChargedQuota)
}

func TestFreezePolicyUsesLocalMonthAndActivationBoundary(t *testing.T) {
	effectiveFrom := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)).Unix()
	effectiveUntil := time.Date(2026, time.September, 20, 9, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)).Unix()
	policy := Policy{
		Enabled:        true,
		EffectiveFrom:  effectiveFrom,
		EffectiveUntil: &effectiveUntil,
		Timezone:       "Asia/Shanghai",
		Tiers:          []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.9}},
	}

	before := time.Unix(effectiveFrom-1, 0)
	_, active, err := FreezePolicy(policy, "vip", "gpt-5", "gpt-5", before)
	require.NoError(t, err)
	assert.False(t, active)

	atStart := time.Unix(effectiveFrom, 0)
	snapshot, active, err := FreezePolicy(policy, "vip", "gpt-5", "gpt-5", atStart)
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, effectiveFrom, snapshot.PeriodStart, "the first period starts at activation; pre-activation usage is not backfilled")
	assert.Equal(t, time.Date(2026, time.September, 1, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)).Unix(), snapshot.PeriodEnd)

	secondMonth := time.Date(2026, time.September, 3, 1, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	snapshot, active, err = FreezePolicy(policy, "vip", "gpt-5", "gpt-5", secondMonth)
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, time.Date(2026, time.September, 1, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)).Unix(), snapshot.PeriodStart)
	assert.Equal(t, effectiveUntil, snapshot.PeriodEnd)

	_, active, err = FreezePolicy(policy, "vip", "gpt-5", "gpt-5", time.Unix(effectiveUntil, 0))
	require.NoError(t, err)
	assert.False(t, active, "effective_until is exclusive")
}

func TestValidatePolicyMapRejectsUnsafeOrAmbiguousConfigurations(t *testing.T) {
	valid := PolicyMap{
		"vip": {
			Models: map[string]Policy{"gpt-5": {
				Enabled:  true,
				Timezone: "UTC",
				Tiers: []Tier{
					{MinMonthlyOriginalQuota: 0, Ratio: 1},
					{MinMonthlyOriginalQuota: 1000, Ratio: 0.9},
				},
			}},
		},
	}
	require.NoError(t, ValidatePolicyMap(valid))

	tests := []struct {
		name   string
		mutate func(PolicyMap)
	}{
		{name: "first threshold is not zero", mutate: func(p PolicyMap) { p["vip"].Models["gpt-5"].Tiers[0].MinMonthlyOriginalQuota = 1 }},
		{name: "thresholds do not increase", mutate: func(p PolicyMap) { p["vip"].Models["gpt-5"].Tiers[1].MinMonthlyOriginalQuota = 0 }},
		{name: "ratio above one", mutate: func(p PolicyMap) { p["vip"].Models["gpt-5"].Tiers[0].Ratio = 1.01 }},
		{name: "ratio increases", mutate: func(p PolicyMap) { p["vip"].Models["gpt-5"].Tiers[0].Ratio = 0.8 }},
		{name: "invalid timezone", mutate: func(p PolicyMap) {
			policy := p["vip"].Models["gpt-5"]
			policy.Timezone = "Mars/Olympus"
			p["vip"].Models["gpt-5"] = policy
		}},
		{name: "machine local timezone", mutate: func(p PolicyMap) {
			policy := p["vip"].Models["gpt-5"]
			policy.Timezone = "Local"
			p["vip"].Models["gpt-5"] = policy
		}},
		{name: "until not after from", mutate: func(p PolicyMap) {
			until := int64(9)
			policy := p["vip"].Models["gpt-5"]
			policy.EffectiveFrom = 10
			policy.EffectiveUntil = &until
			p["vip"].Models["gpt-5"] = policy
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := clonePolicyMapForTest(valid)
			test.mutate(candidate)
			assert.Error(t, ValidatePolicyMap(candidate))
		})
	}
}

func TestPolicyStoreExactModelOverridesWildcardIncludingDisabled(t *testing.T) {
	store, err := NewPolicyStore(PolicyMap{
		"vip": {
			Models: map[string]Policy{
				"*":     {Enabled: true, Timezone: "UTC", Tiers: []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.9}}},
				"gpt-5": {Enabled: false, Timezone: "UTC", Tiers: []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.9}}},
			},
		},
	})
	require.NoError(t, err)

	policy, matchedModel, ok := store.Resolve("vip", "gpt-5")
	require.True(t, ok)
	assert.False(t, policy.Enabled)
	assert.Equal(t, "gpt-5", matchedModel)

	policy, matchedModel, ok = store.Resolve("vip", "claude")
	require.True(t, ok)
	assert.True(t, policy.Enabled)
	assert.Equal(t, "*", matchedModel)
}

func TestPolicyStoreFreezeResolverKeepsCopyOnWriteSnapshot(t *testing.T) {
	oldPolicies := PolicyMap{
		"vip": {
			Models: map[string]Policy{"gpt-5": {Enabled: true, Timezone: "UTC", Tiers: []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.9}}}},
		},
	}
	store, err := NewPolicyStore(oldPolicies)
	require.NoError(t, err)
	requestAt := time.Unix(10, 0)
	oldResolver := store.FreezeResolver(nil, "gpt-5", requestAt)

	require.NoError(t, store.Replace(PolicyMap{
		"vip": {
			Models: map[string]Policy{"gpt-5": {Enabled: true, Timezone: "UTC", Tiers: []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.5}}}},
		},
	}))
	newResolver := store.FreezeResolver(nil, "gpt-5", requestAt)

	oldSnapshot, active, err := oldResolver.Resolve("vip")
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, 0.9, oldSnapshot.Tiers[0].Ratio)

	newSnapshot, active, err := newResolver.Resolve("vip")
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, 0.5, newSnapshot.Tiers[0].Ratio)
}

func TestFrozenResolverWithOriginModelKeepsAdmissionSnapshot(t *testing.T) {
	requestAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	store, err := NewPolicyStore(PolicyMap{
		"vip": {
			Models: map[string]Policy{"resolved-model": {Enabled: true, Timezone: "UTC", Tiers: []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.8}}}},
		},
		"contract": {
			Models: map[string]Policy{"resolved-model": {Enabled: true, Timezone: "UTC", Tiers: []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.7}}}},
		},
	})
	require.NoError(t, err)
	resolver := store.FreezeResolver(map[string]struct{}{"contract": {}}, "", requestAt)

	require.NoError(t, store.Replace(PolicyMap{
		"vip": {
			Models: map[string]Policy{"resolved-model": {Enabled: true, Timezone: "UTC", Tiers: []Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.2}}}},
		},
	}))

	rebound := resolver.WithOriginModel("resolved-model")
	snapshot, active, err := rebound.Resolve("vip")
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, "resolved-model", snapshot.OriginModel)
	assert.Equal(t, "resolved-model", snapshot.MatchedModel)
	require.Len(t, snapshot.Tiers, 1)
	assert.Equal(t, 0.8, snapshot.Tiers[0].Ratio, "rebinding must retain the policy frozen at admission")
	assert.Equal(t, time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC).Unix(), snapshot.PeriodStart)

	_, active, err = rebound.Resolve("contract")
	require.NoError(t, err)
	assert.False(t, active, "rebinding must retain explicit group-contract precedence")
}

func TestParsePoliciesJSONRejectsMissingAndUnknownMoneyFields(t *testing.T) {
	tests := []string{
		`{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0}]}}}`,
		`{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ration":0.9}]}}}`,
		`{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`,
		`{" ":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`,
		`{"vip":{" ":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`,
	}
	for _, raw := range tests {
		assert.Error(t, ValidatePoliciesJSON(raw))
	}
}

func TestParsePoliciesJSONRejectsNullForRequiredScalarFields(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "legacy tier threshold",
			raw:  `{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":null,"ratio":0.8}]}}}`,
		},
		{
			name: "legacy tier ratio",
			raw:  `{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":null}]}}}`,
		},
		{
			name: "canonical tier threshold",
			raw:  `{"vip":{"progress_basis":"original","models":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":null,"ratio":0.8}]}}}}`,
		},
		{
			name: "canonical tier ratio",
			raw:  `{"vip":{"progress_basis":"original","models":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":null}]}}}}`,
		},
		{
			name: "policy enabled",
			raw:  `{"vip":{"gpt-5":{"enabled":null,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`,
		},
		{
			name: "policy effective from",
			raw:  `{"vip":{"gpt-5":{"enabled":true,"effective_from":null,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`,
		},
		{
			name: "policy timezone",
			raw:  `{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":null,"tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`,
		},
		{
			name: "canonical progress basis",
			raw:  `{"vip":{"progress_basis":null,"models":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Error(t, ValidatePoliciesJSON(test.raw))
		})
	}
}

func TestParsePoliciesJSONKeepsNullEffectiveUntilAsNoExpiry(t *testing.T) {
	policies, err := ParsePoliciesJSON(`{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`)
	require.NoError(t, err)
	assert.Nil(t, policies["vip"].Models["gpt-5"].EffectiveUntil)
}

func clonePolicyMapForTest(source PolicyMap) PolicyMap {
	cloned := make(PolicyMap, len(source))
	for group, groupPolicy := range source {
		models := make(map[string]Policy, len(groupPolicy.Models))
		for model, policy := range groupPolicy.Models {
			policy.Tiers = append([]Tier(nil), policy.Tiers...)
			models[model] = policy
		}
		cloned[group] = GroupPolicy{ProgressBasis: groupPolicy.ProgressBasis, Models: models}
	}
	return cloned
}
