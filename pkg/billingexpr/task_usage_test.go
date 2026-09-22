package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskUsageExpressionUsesFactsAndTaskQuotaConversion(t *testing.T) {
	expression := `tier("1080p", u("seconds") * (u("resolution") == "1080p" ? 0.4 : 0.2))`
	cost, trace, err := RunExprWithRequest(expression, TokenParams{}, RequestInput{Usage: map[string]any{"seconds": 10.0, "resolution": "1080p"}})
	require.NoError(t, err)
	assert.Equal(t, 4.0, cost)
	assert.Equal(t, "1080p", trace.MatchedTier)
	result, err := ComputeTieredQuotaWithRequest(&BillingSnapshot{ExprString: expression, ExprHash: ExprHashString(expression), GroupRatio: 2, QuotaPerUnit: 500000, ExprVersion: 1, TaskUsageBilling: true}, TokenParams{}, RequestInput{Usage: map[string]any{"seconds": 10.0, "resolution": "1080p"}})
	require.NoError(t, err)
	assert.Equal(t, 4_000_000, result.ActualQuotaAfterGroup)
}

func TestTaskUsageCompletionRejectsInvalidCostWithoutChangingSnapshot(t *testing.T) {
	for _, expression := range []string{`u("seconds") - 10`, `1 / (u("seconds") - 8)`, `0 / (u("seconds") - 8)`} {
		t.Run(expression, func(t *testing.T) {
			snapshot := &BillingSnapshot{ExprString: expression, ExprHash: ExprHashString(expression), TaskUsageBilling: true, QuotaPerUnit: 1000, GroupRatio: 1, UsageFacts: map[string]any{"seconds": 5.0, "clips": 2.0}, EstimatedTier: "reserved"}
			_, err := ComputeTaskUsageQuota(snapshot, map[string]any{"seconds": 8.0})
			require.Error(t, err)
			assert.Equal(t, map[string]any{"seconds": 5.0, "clips": 2.0}, snapshot.UsageFacts)
			assert.Equal(t, "reserved", snapshot.EstimatedTier)
		})
	}
}

func TestTaskUsageCompletionOverlaysMeasuredFactsAndRetainsRequestPricing(t *testing.T) {
	expression := `(u("seconds") > 5 ? tier("measured", u("seconds") + u("clips")) : tier("reserved", u("seconds") + u("clips"))) * (param("quality") == "high" ? 2 : 1)`
	for _, test := range []struct {
		name     string
		measured map[string]any
		seconds  float64
		quota    int
		tier     string
		crossed  bool
	}{
		{name: "partial measurement retains omitted clips", measured: map[string]any{"seconds": 8.0}, seconds: 8, quota: 5000, tier: "measured", crossed: true},
		{name: "explicit zero replaces estimate", measured: map[string]any{"seconds": 0.0}, seconds: 0, quota: 1000, tier: "reserved"},
		{name: "omitted measurement retains estimates", seconds: 5, quota: 3500, tier: "reserved"},
	} {
		t.Run(test.name, func(t *testing.T) {
			estimates := map[string]any{"seconds": 5.0, "clips": 2.0}
			snapshot := &BillingSnapshot{
				ExprString: expression, ExprHash: ExprHashString(expression),
				TaskUsageBilling: true, QuotaPerUnit: 1000, GroupRatio: 0.25,
				UsageFacts: estimates, EstimatedTier: "reserved", EstimatedQuotaAfterGroup: 3500,
				RequestInput: &RequestInput{Params: map[string]any{"quality": "high"}},
			}
			result, err := ComputeTaskUsageQuota(snapshot, test.measured)
			require.NoError(t, err)
			assert.Equal(t, test.quota, result.ActualQuotaAfterGroup)
			assert.Equal(t, float64(test.quota)*4, result.ActualQuotaBeforeGroup)
			assert.Equal(t, test.tier, result.MatchedTier)
			assert.Equal(t, test.crossed, result.CrossedTier)
			require.Len(t, result.RequestRules, 1)
			assert.True(t, result.RequestRules[0].Matched)
			assert.Equal(t, 2.0, result.RequestRules[0].Multiplier)
			assert.Equal(t, result.RequestRules, snapshot.RequestRules)
			assert.Equal(t, test.tier, snapshot.EstimatedTier)
			assert.Equal(t, map[string]any{"seconds": test.seconds, "clips": 2.0}, snapshot.UsageFacts)
			assert.Equal(t, map[string]any{"seconds": 5.0, "clips": 2.0}, estimates)
			assert.Equal(t, 3500, snapshot.EstimatedQuotaAfterGroup)
			if test.measured != nil {
				assert.Equal(t, map[string]any{"seconds": test.seconds}, test.measured)
			}
		})
	}
}
