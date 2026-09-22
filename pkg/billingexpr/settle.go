package billingexpr

import (
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
)

// ComputeTaskUsageQuota overlays measured facts without discarding estimates
// omitted by the provider. Only a successful evaluation updates the snapshot.
func ComputeTaskUsageQuota(snap *BillingSnapshot, measured map[string]any) (TieredResult, error) {
	facts := make(map[string]any, len(snap.UsageFacts)+len(measured))
	for key, value := range snap.UsageFacts {
		facts[key] = value
	}
	for key, value := range measured {
		facts[key] = value
	}
	result, err := ComputeTieredQuotaWithRequest(snap, TokenParams{}, RequestInput{Usage: facts})
	if err != nil {
		return TieredResult{}, err
	}
	snap.UsageFacts = facts
	snap.EstimatedTier = result.MatchedTier
	snap.RequestRules = result.RequestRules
	return result, nil
}

// quotaConversion converts raw expression output to quota based on the
// expression version. This is the central dispatch point for future versions
// that may use a different conversion formula.
func quotaConversion(exprOutput float64, snap *BillingSnapshot) float64 {
	if snap.TaskUsageBilling {
		return exprOutput * snap.QuotaPerUnit
	}
	switch snap.ExprVersion {
	default: // v1: coefficients are $/1M tokens prices
		return exprOutput / 1_000_000 * snap.QuotaPerUnit
	}
}

// ComputeTieredQuota runs the Expr from a frozen BillingSnapshot against
// actual token counts and returns the settlement result.
func ComputeTieredQuota(snap *BillingSnapshot, params TokenParams) (TieredResult, error) {
	return ComputeTieredQuotaWithRequest(snap, params, RequestInput{})
}

func ComputeTieredQuotaWithRequest(snap *BillingSnapshot, params TokenParams, request RequestInput) (TieredResult, error) {
	if snap.TaskUsageBilling && snap.RequestInput != nil {
		usage := request.Usage
		request = *snap.RequestInput
		request.Usage = usage
	}
	cost, trace, err := RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, params, request)
	if err != nil {
		return TieredResult{}, err
	}
	if snap.TaskUsageBilling && (cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0)) {
		return TieredResult{}, fmt.Errorf("task expression result must be finite and non-negative")
	}

	quotaBeforeGroup := quotaConversion(cost, snap)
	afterGroup, clamp := common.QuotaRoundChecked(quotaBeforeGroup * snap.GroupRatio)
	crossed := trace.MatchedTier != snap.EstimatedTier

	return TieredResult{
		ActualQuotaBeforeGroup: quotaBeforeGroup,
		ActualQuotaAfterGroup:  afterGroup,
		MatchedTier:            trace.MatchedTier,
		RequestRules:           trace.RequestRules,
		CrossedTier:            crossed,
		Clamp:                  clamp,
	}, nil
}
