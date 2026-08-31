package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

var ErrGroupModelDiscountSettlementPending = errors.New("group model discount settlement requires reconciliation")

const groupModelDiscountAdmissionErrorKey = "group_model_discount_admission_error"

func recordGroupModelDiscountAdmissionError(ctx *gin.Context, err error) {
	if ctx != nil && err != nil {
		ctx.Set(groupModelDiscountAdmissionErrorKey, err)
	}
}

// TakeGroupModelDiscountAdmissionError transfers an unfunded settlement error
// from a post-response billing helper to the controller that owns the one safe
// refund of this attempt's admission pre-consume.
func TakeGroupModelDiscountAdmissionError(ctx *gin.Context) error {
	if ctx == nil {
		return nil
	}
	value, ok := ctx.Get(groupModelDiscountAdmissionErrorKey)
	if !ok || value == nil {
		return nil
	}
	ctx.Set(groupModelDiscountAdmissionErrorKey, nil)
	err, _ := value.(error)
	return err
}

// GroupModelDiscountDecision is the immutable amount decision used by every
// downstream accounting consumer (funding, user/channel usage and logs).
type GroupModelDiscountDecision struct {
	Applied bool
	// FundingStarted is the external-side-effect boundary for this attempt.
	// Callers must not infer it from whether a ledger row was observed.
	FundingStarted bool
	// AdmissionRefundSafe means this attempt failed before any settlement
	// funding action, so its fresh admission pre-consume may be returned once.
	AdmissionRefundSafe bool
	// Reused is true only when an existing settled decision was replayed and
	// this attempt's fresh pre-consume was safely returned.
	Reused bool
	// RequiresReconciliation tells callers to retain their durable billing
	// marker and stop automatic funding/token transitions.
	RequiresReconciliation bool
	RequestID              string
	OriginalQuota          int
	ChargedQuota           int
	Settlement             *model.GroupModelDiscountSettlement
	Calculation            *groupdiscount.Calculation
}

// SettleModelCharge applies a captured group/model monthly policy to the true
// pre-group original quota. The ledger reservation is durable before funds are
// settled. A successful funding settlement is then committed; any ambiguous
// post-funding failure is fail-closed as pending_reconcile and is never replayed.
//
// With no captured policy this is a strict compatibility wrapper around the
// existing fixed GroupRatio settlement path.
func SettleModelCharge(
	ctx *gin.Context,
	relayInfo *relaycommon.RelayInfo,
	requestID string,
	originalQuota int,
	fallbackQuota int,
	usageOverrides ...model.BillingUsageDelta,
) (GroupModelDiscountDecision, error) {
	decision := GroupModelDiscountDecision{
		RequestID:     requestID,
		OriginalQuota: originalQuota,
		ChargedQuota:  fallbackQuota,
	}
	if relayInfo == nil {
		return decision, errors.New("relay info is nil")
	}
	if relayInfo.GroupModelDiscountSnapshot == nil {
		decision.FundingStarted = true
		err := SettleBilling(ctx, relayInfo, fallbackQuota)
		decision.RequiresReconciliation = err != nil
		return decision, err
	}
	if requestID == "" {
		requestID = relayInfo.RequestId
		decision.RequestID = requestID
	}
	if originalQuota <= 0 {
		decision.ChargedQuota = 0
		decision.FundingStarted = true
		err := SettleBilling(ctx, relayInfo, 0)
		decision.RequiresReconciliation = err != nil
		return decision, err
	}
	if relayInfo.Billing == nil {
		decision.AdmissionRefundSafe = true
		return decision, errors.New("monthly group model discount requires a billing session")
	}

	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     requestID,
		UserID:        relayInfo.UserId,
		UsingGroup:    relayInfo.UsingGroup,
		OriginModel:   relayInfo.OriginModelName,
		Snapshot:      *relayInfo.GroupModelDiscountSnapshot,
		OriginalQuota: originalQuota,
	})
	if err != nil {
		decision.AdmissionRefundSafe = true
		reserveErr := fmt.Errorf("reserve monthly group model discount: %w", err)
		if errors.Is(err, model.ErrGroupModelDiscountRequestConflict) {
			return decision, reserveErr
		}
		settlement, getErr := model.GetGroupModelDiscountSettlement(requestID)
		if getErr != nil {
			return decision, reserveErr
		}
		if settlement.UserID != relayInfo.UserId || settlement.UsingGroup != relayInfo.UsingGroup ||
			settlement.OriginModel != relayInfo.OriginModelName ||
			settlement.PeriodStart != relayInfo.GroupModelDiscountSnapshot.PeriodStart ||
			settlement.PolicyHash != relayInfo.GroupModelDiscountSnapshot.PolicyHash ||
			settlement.OriginalQuota != int64(originalQuota) {
			return decision, reserveErr
		}
		decision.Settlement = &settlement
		if settlement.Status != model.GroupModelDiscountStatusReserved {
			return decision, reserveErr
		}
		if rollbackErr := model.RollbackGroupModelDiscountReservation(requestID); rollbackErr == nil {
			settlement.Status = model.GroupModelDiscountStatusReversed
			return decision, reserveErr
		}

		decision.Applied = true
		decision.RequiresReconciliation = true
		if markErr := model.MarkGroupModelDiscountPendingReconcile(
			requestID,
			model.GroupModelDiscountPendingActionUnknownManual,
		); markErr != nil {
			return decision, errors.Join(
				reserveErr,
				ErrGroupModelDiscountSettlementPending,
				fmt.Errorf("mark commit-unknown monthly group model discount pending reconciliation: %w", markErr),
			)
		}
		settlement.Status = model.GroupModelDiscountStatusPendingReconcile
		settlement.PendingAction = model.GroupModelDiscountPendingActionUnknownManual
		return decision, errors.Join(reserveErr, ErrGroupModelDiscountSettlementPending)
	}
	decision.Applied = true
	decision.Reused = reservation.Reused
	decision.ChargedQuota = reservation.Calculation.ChargedQuota
	decision.Settlement = &reservation.Settlement
	decision.Calculation = &reservation.Calculation

	if reservation.Reused {
		return settleReusedGroupModelDiscount(ctx, relayInfo, decision)
	}

	decision.FundingStarted = true
	if err := SettleBilling(ctx, relayInfo, decision.ChargedQuota); err != nil {
		decision.RequiresReconciliation = true
		return decision, handleGroupModelDiscountSettleFailure(requestID, err)
	}
	usageDelta := model.BillingUsageDelta{
		UserID:            relayInfo.UserId,
		ChannelID:         relayInfo.ChannelId,
		QuotaDelta:        decision.ChargedQuota,
		RequestCountDelta: 1,
	}
	if len(usageOverrides) > 0 {
		usageDelta = usageOverrides[0]
		usageDelta.QuotaDelta = decision.ChargedQuota
	}
	if err := model.CommitGroupModelDiscountSettlementWithUsage(requestID, usageDelta); err != nil {
		decision.RequiresReconciliation = true
		markErr := model.MarkGroupModelDiscountPendingReconcile(
			requestID,
			model.GroupModelDiscountPendingActionCommitAfterFunding,
		)
		if markErr != nil {
			return decision, errors.Join(
				fmt.Errorf("commit monthly group model discount after funding settled: %w", err),
				fmt.Errorf("mark monthly group model discount pending reconciliation: %w", markErr),
			)
		}
		return decision, fmt.Errorf("commit monthly group model discount after funding settled: %w", err)
	}
	decision.Settlement.Status = model.GroupModelDiscountStatusSettled
	return decision, nil
}

func settleReusedGroupModelDiscount(
	ctx *gin.Context,
	relayInfo *relaycommon.RelayInfo,
	decision GroupModelDiscountDecision,
) (GroupModelDiscountDecision, error) {
	// A replay has its own admission-time pre-consume. Return it before using
	// the historical decision; otherwise an idempotent request would still be
	// charged twice outside the settlement ledger.
	decision.FundingStarted = true
	if err := SettleBilling(ctx, relayInfo, 0); err != nil {
		// Settle may have partially moved funding or token quota before failing.
		// The fresh replay admission is not represented by the historical ledger,
		// so do not guess at compensation here. Fail-close the historical request
		// too, giving every later replay a durable manual-reconciliation marker.
		decision.Reused = false
		decision.RequiresReconciliation = true
		settleErr := fmt.Errorf("return replay pre-consume: %w", err)
		if markErr := model.MarkGroupModelDiscountPendingReconcile(
			decision.RequestID,
			model.GroupModelDiscountPendingActionUnknownManual,
		); markErr != nil {
			return decision, errors.Join(
				settleErr,
				ErrGroupModelDiscountSettlementPending,
				fmt.Errorf("mark replay funding outcome pending reconciliation: %w", markErr),
			)
		}
		decision.Settlement.Status = model.GroupModelDiscountStatusPendingReconcile
		decision.Settlement.PendingAction = model.GroupModelDiscountPendingActionUnknownManual
		return decision, errors.Join(settleErr, ErrGroupModelDiscountSettlementPending)
	}

	switch decision.Settlement.Status {
	case model.GroupModelDiscountStatusSettled:
		return decision, nil
	case model.GroupModelDiscountStatusReserved:
		// A duplicate caller cannot know whether the original owner will commit or
		// roll back. Leave the reservation unchanged so that owner retains control.
		decision.Reused = false
		decision.RequiresReconciliation = true
		return decision, ErrGroupModelDiscountSettlementPending
	case model.GroupModelDiscountStatusPendingReconcile:
		decision.Reused = false
		decision.RequiresReconciliation = true
		return decision, ErrGroupModelDiscountSettlementPending
	default:
		decision.Reused = false
		decision.RequiresReconciliation = true
		return decision, fmt.Errorf("%w: request %s is %s", model.ErrGroupModelDiscountInvalidTransition, decision.RequestID, decision.Settlement.Status)
	}
}

func handleGroupModelDiscountSettleFailure(requestID string, settleErr error) error {
	// A BillingSession Settle error cannot prove which funding, token, or
	// accounting stages completed. Never compensate automatically; persist an
	// unknown/manual action and fail closed for explicit reconciliation.
	if markErr := model.MarkGroupModelDiscountPendingReconcile(
		requestID,
		model.GroupModelDiscountPendingActionUnknownManual,
	); markErr != nil {
		return errors.Join(settleErr, fmt.Errorf("mark monthly group model discount pending reconciliation: %w", markErr))
	}
	return errors.Join(settleErr, ErrGroupModelDiscountSettlementPending)
}

// InjectGroupModelDiscountInfo adds the exact original/net decision and tier
// segments to a consume log. It never derives original price from GroupRatio.
func InjectGroupModelDiscountInfo(other map[string]interface{}, decision GroupModelDiscountDecision) {
	if other == nil || !decision.Applied || decision.Calculation == nil || decision.Settlement == nil {
		return
	}
	other["group_model_discount"] = map[string]interface{}{
		"request_id":              decision.RequestID,
		"original_quota":          decision.OriginalQuota,
		"charged_quota":           decision.ChargedQuota,
		"progress_basis":          decision.Settlement.ProgressBasis,
		"monthly_original_before": decision.Calculation.MonthlyOriginalBefore,
		"monthly_original_after":  decision.Calculation.MonthlyOriginalAfter,
		"monthly_progress_before": decision.Calculation.MonthlyProgressBefore,
		"monthly_progress_after":  decision.Calculation.MonthlyProgressAfter,
		"progress_quota":          decision.Calculation.ProgressQuota,
		"period_start":            decision.Settlement.PeriodStart,
		"period_end":              decision.Settlement.PeriodEnd,
		"policy_hash":             decision.Settlement.PolicyHash,
		"segments":                decision.Calculation.Segments,
	}
}
