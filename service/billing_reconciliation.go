package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	billingReconciliationBatchSize    = 100
	billingReconciliationStaleAfter   = time.Minute
	billingReconciliationTickInterval = time.Minute
)

var (
	billingReconciliationWorkerOnce                        sync.Once
	billingBeforeActionClaimHook                           func(operationID, expectedReadyAction string)
	ErrBillingAdmissionReserveRequiresManualReconciliation = errors.New("billing admission reserve requires manual reconciliation")
)

func runBillingBeforeActionClaimHook(operationID, expectedReadyAction string) {
	if billingBeforeActionClaimHook != nil {
		billingBeforeActionClaimHook(operationID, expectedReadyAction)
	}
}

type billingReconciliationSummary struct {
	RefundScanned        int
	AdmissionScanned     int
	ManualRefundCount    int
	ManualAdmissionCount int
	Failed               int
}

// StartBillingReconciliationWorker starts one master-only recovery loop. The
// first pass runs immediately, then each subsequent pass is ticker driven.
func StartBillingReconciliationWorker() {
	billingReconciliationWorkerOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			runBillingReconciliationOnce(time.Now())
			ticker := time.NewTicker(billingReconciliationTickInterval)
			defer ticker.Stop()
			for now := range ticker.C {
				runBillingReconciliationOnce(now)
			}
		}()
	})
}

// runBillingReconciliationOnce is the deterministic one-pass seam used by
// startup, the ticker, and tests. Only rows older than the stale window are
// considered, preventing a live request from racing its recovery worker.
func runBillingReconciliationOnce(now time.Time) billingReconciliationSummary {
	summary := billingReconciliationSummary{}
	staleBefore := now.Add(-billingReconciliationStaleAfter)

	refunds, err := model.ListRecoverableBillingRefundOperations(staleBefore, billingReconciliationBatchSize)
	if err != nil {
		common.SysError("failed to scan recoverable billing refunds: " + err.Error())
	} else {
		summary.RefundScanned = len(refunds)
		for _, operation := range refunds {
			_, reconcileErr := reconcilePendingBillingRefundOperationOnce(operation.OperationID, false)
			if errors.Is(reconcileErr, ErrBillingRefundRequiresManualReconciliation) {
				summary.ManualRefundCount++
				continue
			}
			if reconcileErr != nil {
				summary.Failed++
				common.SysError(fmt.Sprintf("failed to reconcile billing refund operation %s: %s", operation.OperationID, reconcileErr.Error()))
			}
		}
	}

	admissions, err := model.ListRecoverableBillingAdmissionReserveOperations(staleBefore, billingReconciliationBatchSize)
	if err != nil {
		common.SysError("failed to scan recoverable billing admission reserves: " + err.Error())
	} else {
		summary.AdmissionScanned = len(admissions)
		for _, operation := range admissions {
			_, reconcileErr := ReconcilePendingBillingAdmissionReserveOperation(operation.OperationID)
			if errors.Is(reconcileErr, ErrBillingAdmissionReserveRequiresManualReconciliation) {
				summary.ManualAdmissionCount++
				continue
			}
			if reconcileErr != nil {
				summary.Failed++
				common.SysError(fmt.Sprintf("failed to reconcile billing admission reserve operation %s: %s", operation.OperationID, reconcileErr.Error()))
			}
		}
	}

	manualRefunds, err := model.ListManualBillingRefundOperations(billingReconciliationBatchSize)
	if err != nil {
		common.SysError("failed to scan manual billing refunds: " + err.Error())
	} else {
		summary.ManualRefundCount += len(manualRefunds)
	}
	manualAdmissions, err := model.ListManualBillingAdmissionReserveOperations(billingReconciliationBatchSize)
	if err != nil {
		common.SysError("failed to scan manual billing admission reserves: " + err.Error())
	} else {
		summary.ManualAdmissionCount += len(manualAdmissions)
	}
	if summary.ManualRefundCount > 0 || summary.ManualAdmissionCount > 0 {
		common.SysError(fmt.Sprintf(
			"billing operations require manual reconciliation: refunds=%d admission_reserves=%d",
			summary.ManualRefundCount,
			summary.ManualAdmissionCount,
		))
	}
	return summary
}

// ReconcilePendingBillingAdmissionReserveOperation advances only states that
// prove no action started, or that confirmed actions can be compensated after
// a fresh CAS claim. Unknown wallet/token outcomes are never replayed.
func ReconcilePendingBillingAdmissionReserveOperation(operationID string) (model.BillingAdmissionReserveOperation, error) {
	var operation model.BillingAdmissionReserveOperation
	for step := 0; step < 8; step++ {
		operation, err := model.GetBillingAdmissionReserveOperation(operationID)
		if err != nil {
			if errors.Is(err, model.ErrBillingAdmissionReserveCorrupt) {
				return operation, errors.Join(ErrBillingAdmissionReserveRequiresManualReconciliation, err)
			}
			return operation, err
		}
		if operation.Status == model.BillingAdmissionReserveStatusApplied ||
			operation.Status == model.BillingAdmissionReserveStatusCanceled {
			return operation, nil
		}

		switch operation.PendingAction {
		case model.BillingAdmissionReservePendingActionFundingReady:
			if err := model.CancelUnstartedBillingAdmissionReserveOperation(operationID); err != nil {
				if errors.Is(err, model.ErrBillingAdmissionReserveInvalidTransition) {
					continue
				}
				return operation, err
			}

		case model.BillingAdmissionReservePendingActionTokenReady,
			model.BillingAdmissionReservePendingActionCommitAfterReserve:
			if err := model.PrepareBillingAdmissionReserveCompensation(operationID); err != nil {
				if errors.Is(err, model.ErrBillingAdmissionReserveInvalidTransition) {
					continue
				}
				return operation, err
			}

		case model.BillingAdmissionReservePendingActionTokenRefundReady:
			token, tokenErr := model.GetTokenById(operation.TokenID)
			if tokenErr != nil {
				return operation, tokenErr
			}
			if token.UserId != operation.UserID {
				return operation, errors.Join(ErrBillingAdmissionReserveRequiresManualReconciliation, model.ErrBillingAdmissionReserveCorrupt)
			}
			runBillingBeforeActionClaimHook(operationID, model.BillingAdmissionReservePendingActionTokenRefundReady)
			claim, claimErr := model.ClaimNextBillingAdmissionReserveAction(operationID, model.BillingAdmissionReservePendingActionTokenRefundReady)
			if claimErr != nil {
				if errors.Is(claimErr, model.ErrBillingAdmissionReserveInvalidTransition) {
					continue
				}
				return operation, claimErr
			}
			if !claim.Claimed {
				continue
			}
			operation = claim.Operation
			if refundErr := model.RefundTokenQuotaForBilling(operation.TokenID, token.Key, operation.TokenReservedQuota); refundErr != nil {
				return operation, errors.Join(ErrBillingAdmissionReserveRequiresManualReconciliation, refundErr)
			}
			if confirmErr := model.ConfirmBillingAdmissionReserveTokenRefund(operationID); confirmErr != nil {
				if errors.Is(confirmErr, model.ErrBillingAdmissionReserveInvalidTransition) {
					continue
				}
				return operation, errors.Join(ErrBillingAdmissionReserveRequiresManualReconciliation, confirmErr)
			}

		case model.BillingAdmissionReservePendingActionFundingRefundReady:
			runBillingBeforeActionClaimHook(operationID, model.BillingAdmissionReservePendingActionFundingRefundReady)
			claim, claimErr := model.ClaimNextBillingAdmissionReserveAction(operationID, model.BillingAdmissionReservePendingActionFundingRefundReady)
			if claimErr != nil {
				if errors.Is(claimErr, model.ErrBillingAdmissionReserveInvalidTransition) {
					continue
				}
				return operation, claimErr
			}
			if !claim.Claimed {
				continue
			}
			operation = claim.Operation
			var refundErr error
			switch operation.FundingSource {
			case BillingSourceWallet:
				if operation.FundingReferenceID != int64(operation.UserID) {
					return operation, errors.Join(ErrBillingAdmissionReserveRequiresManualReconciliation, model.ErrBillingAdmissionReserveCorrupt)
				}
				refundErr = model.RefundUserQuotaForBilling(operation.UserID, operation.FundingReservedQuota)
			case BillingSourceSubscription:
				if operation.Mode == model.BillingAdmissionReserveModeInitial {
					refundErr = model.RefundSubscriptionPreConsume(operation.RequestID)
				} else {
					refundErr = model.PostConsumeUserSubscriptionDelta(int(operation.FundingReferenceID), -int64(operation.FundingReservedQuota))
				}
			default:
				refundErr = model.ErrBillingAdmissionReserveCorrupt
			}
			if refundErr != nil {
				return operation, errors.Join(ErrBillingAdmissionReserveRequiresManualReconciliation, refundErr)
			}
			if confirmErr := model.ConfirmBillingAdmissionReserveFundingRefund(operationID); confirmErr != nil {
				if errors.Is(confirmErr, model.ErrBillingAdmissionReserveInvalidTransition) {
					continue
				}
				return operation, errors.Join(ErrBillingAdmissionReserveRequiresManualReconciliation, confirmErr)
			}

		case model.BillingAdmissionReservePendingActionCommitAfterRefund:
			if err := model.CancelBillingAdmissionReserveOperation(operationID); err != nil {
				if errors.Is(err, model.ErrBillingAdmissionReserveInvalidTransition) {
					continue
				}
				return operation, err
			}

		case model.BillingAdmissionReservePendingActionFundingUnknown,
			model.BillingAdmissionReservePendingActionTokenUnknown,
			model.BillingAdmissionReservePendingActionTokenRefundUnknown,
			model.BillingAdmissionReservePendingActionFundingRefundUnknown:
			return operation, ErrBillingAdmissionReserveRequiresManualReconciliation

		default:
			return operation, errors.Join(ErrBillingAdmissionReserveRequiresManualReconciliation, model.ErrBillingAdmissionReserveCorrupt)
		}
	}
	operation, err := model.GetBillingAdmissionReserveOperation(operationID)
	if err != nil {
		return operation, err
	}
	if operation.Status != model.BillingAdmissionReserveStatusCanceled {
		return operation, ErrBillingAdmissionReserveRequiresManualReconciliation
	}
	return operation, nil
}
