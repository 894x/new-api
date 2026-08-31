package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	BillingAdmissionReserveStatusPendingReconcile = "pending_reconcile"
	BillingAdmissionReserveStatusApplied          = "applied"
	BillingAdmissionReserveStatusCanceled         = "canceled"

	BillingAdmissionReserveModeStandard        = "standard"
	BillingAdmissionReserveModeStrictWallet    = "strict_wallet"
	BillingAdmissionReserveModeInitial         = "initial"
	billingAdmissionReserveFundingWallet       = "wallet"
	billingAdmissionReserveFundingSubscription = "subscription"

	BillingAdmissionReservePendingActionFundingReady         = "funding_reserve_ready"
	BillingAdmissionReservePendingActionFundingUnknown       = "funding_reserve_unknown"
	BillingAdmissionReservePendingActionTokenReady           = "token_reserve_ready"
	BillingAdmissionReservePendingActionTokenUnknown         = "token_reserve_unknown"
	BillingAdmissionReservePendingActionCommitAfterReserve   = "commit_after_reserve"
	BillingAdmissionReservePendingActionTokenRefundReady     = "token_refund_ready"
	BillingAdmissionReservePendingActionTokenRefundUnknown   = "token_refund_unknown"
	BillingAdmissionReservePendingActionFundingRefundReady   = "funding_refund_ready"
	BillingAdmissionReservePendingActionFundingRefundUnknown = "funding_refund_unknown"
	BillingAdmissionReservePendingActionCommitAfterRefund    = "commit_after_refund"

	billingAdmissionReserveMaxAttempts     = 64
	maxBillingAdmissionOperationIDLength   = 191
	maxBillingAdmissionSessionIDLength     = 128
	maxBillingAdmissionRequestIDLength     = 191
	maxBillingAdmissionFundingSourceLength = 32
)

var (
	ErrBillingAdmissionReserveInvalidInput      = errors.New("invalid billing admission reserve input")
	ErrBillingAdmissionReserveConflict          = errors.New("billing admission reserve operation conflicts with an existing operation")
	ErrBillingAdmissionReserveInvalidTransition = errors.New("invalid billing admission reserve operation transition")
	ErrBillingAdmissionReserveCorrupt           = errors.New("billing admission reserve operation is inconsistent")
	ErrBillingAdmissionReserveContention        = errors.New("billing admission reserve operation contention exceeded retry limit")
	errBillingAdmissionReserveRevisionConflict  = errors.New("billing admission reserve operation revision conflict")
)

// BillingAdmissionReserveOperation is the durable intent written before an
// admission retry raises its external funding and token reservations. A
// pending row is deliberately fail-closed until the caller proves that every
// external action applied or the caller records definitive evidence that a
// strict funding/token action made no change.
type BillingAdmissionReserveOperation struct {
	Id                   int64     `json:"id" gorm:"primaryKey"`
	OperationID          string    `json:"operation_id" gorm:"type:varchar(191);not null;uniqueIndex:idx_billing_admission_reserve_operation_id"`
	SessionID            string    `json:"session_id" gorm:"type:varchar(128);not null;uniqueIndex:idx_billing_admission_reserve_session_attempt,priority:1"`
	RequestID            string    `json:"request_id" gorm:"type:varchar(191);not null;index:idx_billing_admission_reserve_request_id"`
	Attempt              int64     `json:"attempt" gorm:"not null;uniqueIndex:idx_billing_admission_reserve_session_attempt,priority:2"`
	UserID               int       `json:"user_id" gorm:"not null;index"`
	TokenID              int       `json:"token_id" gorm:"not null;index"`
	FundingSource        string    `json:"funding_source" gorm:"type:varchar(32);not null"`
	FundingReferenceID   int64     `json:"funding_reference_id" gorm:"not null;index"`
	FromQuota            int       `json:"from_quota" gorm:"not null"`
	TargetQuota          int       `json:"target_quota" gorm:"not null"`
	DeltaQuota           int       `json:"delta_quota" gorm:"not null"`
	TokenQuota           int       `json:"token_quota" gorm:"not null"`
	FundingReservedQuota int       `json:"funding_reserved_quota" gorm:"not null"`
	TokenReservedQuota   int       `json:"token_reserved_quota" gorm:"not null"`
	FundingRefundedQuota int       `json:"funding_refunded_quota" gorm:"not null"`
	TokenRefundedQuota   int       `json:"token_refunded_quota" gorm:"not null"`
	Mode                 string    `json:"mode" gorm:"type:varchar(32);not null"`
	Status               string    `json:"status" gorm:"type:varchar(32);not null;index"`
	PendingAction        string    `json:"pending_action" gorm:"type:varchar(64);not null;index"`
	Revision             int64     `json:"revision" gorm:"not null"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (BillingAdmissionReserveOperation) TableName() string {
	return "billing_admission_reserve_operations"
}

type BillingAdmissionReserveInput struct {
	OperationID        string
	SessionID          string
	RequestID          string
	Attempt            int64
	UserID             int
	TokenID            int
	FundingSource      string
	FundingReferenceID int64
	FromQuota          int
	TargetQuota        int
	TokenQuota         int
	Mode               string
}

func BeginBillingAdmissionReserveOperation(input BillingAdmissionReserveInput) (BillingAdmissionReserveOperation, error) {
	if DB == nil {
		return BillingAdmissionReserveOperation{}, errors.New("database is not initialized")
	}
	return beginBillingAdmissionReserveOperation(DB, input)
}

func beginBillingAdmissionReserveOperation(db *gorm.DB, input BillingAdmissionReserveInput) (BillingAdmissionReserveOperation, error) {
	if err := validateBillingAdmissionReserveInput(input); err != nil {
		return BillingAdmissionReserveOperation{}, err
	}
	delta := input.TargetQuota - input.FromQuota
	var lastErr error
	for attempt := 0; attempt < billingAdmissionReserveMaxAttempts; attempt++ {
		operation := BillingAdmissionReserveOperation{
			OperationID:        input.OperationID,
			SessionID:          input.SessionID,
			RequestID:          input.RequestID,
			Attempt:            input.Attempt,
			UserID:             input.UserID,
			TokenID:            input.TokenID,
			FundingSource:      input.FundingSource,
			FundingReferenceID: input.FundingReferenceID,
			FromQuota:          input.FromQuota,
			TargetQuota:        input.TargetQuota,
			DeltaQuota:         delta,
			TokenQuota:         input.TokenQuota,
			Mode:               input.Mode,
			Status:             BillingAdmissionReserveStatusPendingReconcile,
			PendingAction:      BillingAdmissionReservePendingActionFundingReady,
		}
		lastErr = db.Create(&operation).Error
		if lastErr == nil {
			return operation, nil
		}

		existing, getErr := getBillingAdmissionReserveOperation(db, input.OperationID)
		if getErr == nil {
			return reuseBillingAdmissionReserveOperation(existing, input)
		}
		if getErr != nil && !errors.Is(getErr, gorm.ErrRecordNotFound) {
			lastErr = getErr
		}

		var sameAttempt BillingAdmissionReserveOperation
		attemptErr := db.Where("session_id = ? AND attempt = ?", input.SessionID, input.Attempt).First(&sameAttempt).Error
		if attemptErr == nil {
			return reuseBillingAdmissionReserveOperation(sameAttempt, input)
		}
		if attemptErr != nil && !errors.Is(attemptErr, gorm.ErrRecordNotFound) {
			lastErr = attemptErr
		}

		if !isRetryableBillingAdmissionReserveError(lastErr) {
			return BillingAdmissionReserveOperation{}, lastErr
		}
		time.Sleep(billingAdmissionReserveRetryDelay(attempt))
	}
	return BillingAdmissionReserveOperation{}, fmt.Errorf("%w: %w", ErrBillingAdmissionReserveContention, lastErr)
}

func CommitBillingAdmissionReserveOperation(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return commitBillingAdmissionReserveOperation(DB, operationID)
}

func commitBillingAdmissionReserveOperation(db *gorm.DB, operationID string) error {
	return finishBillingAdmissionReserveOperation(
		db,
		operationID,
		BillingAdmissionReservePendingActionCommitAfterReserve,
		BillingAdmissionReserveStatusApplied,
	)
}

func CancelBillingAdmissionReserveOperation(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return cancelBillingAdmissionReserveOperation(DB, operationID)
}

func cancelBillingAdmissionReserveOperation(db *gorm.DB, operationID string) error {
	if err := validateBillingAdmissionReserveOperationID(operationID); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < billingAdmissionReserveMaxAttempts; attempt++ {
		operation, err := getBillingAdmissionReserveOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingAdmissionReserveError(err) {
				return err
			}
			time.Sleep(billingAdmissionReserveRetryDelay(attempt))
			continue
		}
		if err := validateBillingAdmissionReserveOperation(operation); err != nil {
			return err
		}
		if operation.Status == BillingAdmissionReserveStatusCanceled {
			return nil
		}
		knownNotApplied := operation.PendingAction == BillingAdmissionReservePendingActionFundingUnknown &&
			(operation.Mode == BillingAdmissionReserveModeInitial ||
				(operation.Mode == BillingAdmissionReserveModeStrictWallet && operation.FundingSource == billingAdmissionReserveFundingWallet)) &&
			operation.FundingReservedQuota == 0 && operation.TokenReservedQuota == 0
		compensated := operation.PendingAction == BillingAdmissionReservePendingActionCommitAfterRefund
		if operation.Status != BillingAdmissionReserveStatusPendingReconcile || (!knownNotApplied && !compensated) {
			return fmt.Errorf("%w: %s -> %s", ErrBillingAdmissionReserveInvalidTransition, operation.PendingAction, BillingAdmissionReserveStatusCanceled)
		}
		result := db.Model(&BillingAdmissionReserveOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingAdmissionReserveStatusPendingReconcile, operation.PendingAction).
			Updates(map[string]any{
				"status":         BillingAdmissionReserveStatusCanceled,
				"pending_action": "",
				"revision":       operation.Revision + 1,
			})
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			return nil
		} else {
			lastErr = errBillingAdmissionReserveRevisionConflict
		}
		if !isRetryableBillingAdmissionReserveError(lastErr) {
			return lastErr
		}
		time.Sleep(billingAdmissionReserveRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrBillingAdmissionReserveContention, lastErr)
}

func CancelUnstartedBillingAdmissionReserveOperation(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return cancelUnstartedBillingAdmissionReserveOperation(DB, operationID)
}

func cancelUnstartedBillingAdmissionReserveOperation(db *gorm.DB, operationID string) error {
	return finishBillingAdmissionReserveOperation(
		db,
		operationID,
		BillingAdmissionReservePendingActionFundingReady,
		BillingAdmissionReserveStatusCanceled,
	)
}

func GetBillingAdmissionReserveOperation(operationID string) (BillingAdmissionReserveOperation, error) {
	if DB == nil {
		return BillingAdmissionReserveOperation{}, errors.New("database is not initialized")
	}
	operation, err := getBillingAdmissionReserveOperation(DB, operationID)
	if err != nil {
		return BillingAdmissionReserveOperation{}, err
	}
	if err := validateBillingAdmissionReserveOperation(operation); err != nil {
		return BillingAdmissionReserveOperation{}, err
	}
	return operation, nil
}

func getBillingAdmissionReserveOperation(db *gorm.DB, operationID string) (BillingAdmissionReserveOperation, error) {
	if err := validateBillingAdmissionReserveOperationID(operationID); err != nil {
		return BillingAdmissionReserveOperation{}, err
	}
	var operation BillingAdmissionReserveOperation
	err := db.Where("operation_id = ?", operationID).First(&operation).Error
	return operation, err
}

type BillingAdmissionReserveActionClaim struct {
	Operation BillingAdmissionReserveOperation
	Claimed   bool
}

func ClaimNextBillingAdmissionReserveAction(operationID, expectedReadyAction string) (BillingAdmissionReserveActionClaim, error) {
	if DB == nil {
		return BillingAdmissionReserveActionClaim{}, errors.New("database is not initialized")
	}
	return claimNextBillingAdmissionReserveAction(DB, operationID, expectedReadyAction)
}

func claimNextBillingAdmissionReserveAction(db *gorm.DB, operationID, expectedReadyAction string) (BillingAdmissionReserveActionClaim, error) {
	if err := validateBillingAdmissionReserveOperationID(operationID); err != nil {
		return BillingAdmissionReserveActionClaim{}, err
	}
	unknownAction := billingAdmissionReserveUnknownAction(expectedReadyAction)
	if unknownAction == "" {
		return BillingAdmissionReserveActionClaim{}, ErrBillingAdmissionReserveInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < billingAdmissionReserveMaxAttempts; attempt++ {
		operation, err := getBillingAdmissionReserveOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingAdmissionReserveError(err) {
				return BillingAdmissionReserveActionClaim{}, err
			}
			time.Sleep(billingAdmissionReserveRetryDelay(attempt))
			continue
		}
		if err := validateBillingAdmissionReserveOperation(operation); err != nil {
			return BillingAdmissionReserveActionClaim{}, err
		}
		if operation.Status != BillingAdmissionReserveStatusPendingReconcile || operation.PendingAction != expectedReadyAction {
			return BillingAdmissionReserveActionClaim{Operation: operation}, nil
		}
		result := db.Model(&BillingAdmissionReserveOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingAdmissionReserveStatusPendingReconcile, expectedReadyAction).
			Updates(map[string]any{
				"pending_action": unknownAction,
				"revision":       operation.Revision + 1,
			})
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			operation.PendingAction = unknownAction
			operation.Revision++
			return BillingAdmissionReserveActionClaim{Operation: operation, Claimed: true}, nil
		} else {
			lastErr = errBillingAdmissionReserveRevisionConflict
		}
		if !isRetryableBillingAdmissionReserveError(lastErr) {
			return BillingAdmissionReserveActionClaim{}, lastErr
		}
		time.Sleep(billingAdmissionReserveRetryDelay(attempt))
	}
	return BillingAdmissionReserveActionClaim{}, fmt.Errorf("%w: %w", ErrBillingAdmissionReserveContention, lastErr)
}

func billingAdmissionReserveUnknownAction(readyAction string) string {
	switch readyAction {
	case BillingAdmissionReservePendingActionFundingReady:
		return BillingAdmissionReservePendingActionFundingUnknown
	case BillingAdmissionReservePendingActionTokenReady:
		return BillingAdmissionReservePendingActionTokenUnknown
	case BillingAdmissionReservePendingActionTokenRefundReady:
		return BillingAdmissionReservePendingActionTokenRefundUnknown
	case BillingAdmissionReservePendingActionFundingRefundReady:
		return BillingAdmissionReservePendingActionFundingRefundUnknown
	default:
		return ""
	}
}

func ConfirmBillingAdmissionReserveFunding(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return confirmBillingAdmissionReserveFunding(DB, operationID)
}

func confirmBillingAdmissionReserveFunding(db *gorm.DB, operationID string) error {
	return progressBillingAdmissionReserveOperation(db, operationID, BillingAdmissionReservePendingActionFundingUnknown)
}

// ConfirmBillingAdmissionReserveFundingReference records the concrete
// subscription selected by an initial pre-consume in the same CAS transition
// that confirms its funding side effect.
func ConfirmBillingAdmissionReserveFundingReference(operationID string, fundingReferenceID int64) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return confirmBillingAdmissionReserveFundingReference(DB, operationID, fundingReferenceID)
}

func confirmBillingAdmissionReserveFundingReference(db *gorm.DB, operationID string, fundingReferenceID int64) error {
	if fundingReferenceID <= 0 {
		return ErrBillingAdmissionReserveInvalidInput
	}
	return progressBillingAdmissionReserveOperationWithFundingReference(
		db,
		operationID,
		BillingAdmissionReservePendingActionFundingUnknown,
		fundingReferenceID,
	)
}

func ConfirmBillingAdmissionReserveToken(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return confirmBillingAdmissionReserveToken(DB, operationID)
}

func confirmBillingAdmissionReserveToken(db *gorm.DB, operationID string) error {
	return progressBillingAdmissionReserveOperation(db, operationID, BillingAdmissionReservePendingActionTokenUnknown)
}

// RejectBillingAdmissionReserveToken records proof that a claimed strict token
// reservation definitely did not apply. Only this explicit evidence may move a
// token_unknown operation into safe funding compensation.
func RejectBillingAdmissionReserveToken(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return rejectBillingAdmissionReserveToken(DB, operationID)
}

func rejectBillingAdmissionReserveToken(db *gorm.DB, operationID string) error {
	if err := validateBillingAdmissionReserveOperationID(operationID); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < billingAdmissionReserveMaxAttempts; attempt++ {
		operation, err := getBillingAdmissionReserveOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingAdmissionReserveError(err) {
				return err
			}
			time.Sleep(billingAdmissionReserveRetryDelay(attempt))
			continue
		}
		if err := validateBillingAdmissionReserveOperation(operation); err != nil {
			return err
		}
		if operation.Status == BillingAdmissionReserveStatusPendingReconcile &&
			operation.PendingAction == BillingAdmissionReservePendingActionFundingRefundReady &&
			operation.TokenReservedQuota == 0 {
			return nil
		}
		if operation.Status != BillingAdmissionReserveStatusPendingReconcile ||
			operation.PendingAction != BillingAdmissionReservePendingActionTokenUnknown ||
			operation.TokenReservedQuota != 0 {
			return fmt.Errorf("%w: cannot reject token while %s", ErrBillingAdmissionReserveInvalidTransition, operation.PendingAction)
		}
		result := db.Model(&BillingAdmissionReserveOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingAdmissionReserveStatusPendingReconcile, BillingAdmissionReservePendingActionTokenUnknown).
			Updates(map[string]any{
				"pending_action": BillingAdmissionReservePendingActionFundingRefundReady,
				"revision":       operation.Revision + 1,
			})
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			return nil
		} else {
			lastErr = errBillingAdmissionReserveRevisionConflict
		}
		if !isRetryableBillingAdmissionReserveError(lastErr) {
			return lastErr
		}
		time.Sleep(billingAdmissionReserveRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrBillingAdmissionReserveContention, lastErr)
}

func ConfirmBillingAdmissionReserveTokenRefund(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return confirmBillingAdmissionReserveTokenRefund(DB, operationID)
}

func confirmBillingAdmissionReserveTokenRefund(db *gorm.DB, operationID string) error {
	return progressBillingAdmissionReserveOperation(db, operationID, BillingAdmissionReservePendingActionTokenRefundUnknown)
}

func ConfirmBillingAdmissionReserveFundingRefund(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return confirmBillingAdmissionReserveFundingRefund(DB, operationID)
}

func confirmBillingAdmissionReserveFundingRefund(db *gorm.DB, operationID string) error {
	return progressBillingAdmissionReserveOperation(db, operationID, BillingAdmissionReservePendingActionFundingRefundUnknown)
}

func PrepareBillingAdmissionReserveCompensation(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return prepareBillingAdmissionReserveCompensation(DB, operationID)
}

func prepareBillingAdmissionReserveCompensation(db *gorm.DB, operationID string) error {
	if err := validateBillingAdmissionReserveOperationID(operationID); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < billingAdmissionReserveMaxAttempts; attempt++ {
		operation, err := getBillingAdmissionReserveOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingAdmissionReserveError(err) {
				return err
			}
			time.Sleep(billingAdmissionReserveRetryDelay(attempt))
			continue
		}
		if err := validateBillingAdmissionReserveOperation(operation); err != nil {
			return err
		}
		if operation.Status != BillingAdmissionReserveStatusPendingReconcile {
			return fmt.Errorf("%w: %s", ErrBillingAdmissionReserveInvalidTransition, operation.Status)
		}
		if operation.PendingAction == BillingAdmissionReservePendingActionTokenRefundReady ||
			operation.PendingAction == BillingAdmissionReservePendingActionFundingRefundReady ||
			operation.PendingAction == BillingAdmissionReservePendingActionCommitAfterRefund {
			return nil
		}
		var nextAction string
		switch operation.PendingAction {
		case BillingAdmissionReservePendingActionTokenReady:
			nextAction = BillingAdmissionReservePendingActionFundingRefundReady
		case BillingAdmissionReservePendingActionCommitAfterReserve:
			if operation.TokenReservedQuota > 0 {
				nextAction = BillingAdmissionReservePendingActionTokenRefundReady
			} else {
				nextAction = BillingAdmissionReservePendingActionFundingRefundReady
			}
		default:
			return fmt.Errorf("%w: cannot compensate while %s", ErrBillingAdmissionReserveInvalidTransition, operation.PendingAction)
		}
		result := db.Model(&BillingAdmissionReserveOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingAdmissionReserveStatusPendingReconcile, operation.PendingAction).
			Updates(map[string]any{
				"pending_action": nextAction,
				"revision":       operation.Revision + 1,
			})
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			return nil
		} else {
			lastErr = errBillingAdmissionReserveRevisionConflict
		}
		if !isRetryableBillingAdmissionReserveError(lastErr) {
			return lastErr
		}
		time.Sleep(billingAdmissionReserveRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrBillingAdmissionReserveContention, lastErr)
}

func progressBillingAdmissionReserveOperation(db *gorm.DB, operationID, completedAction string) error {
	return progressBillingAdmissionReserveOperationWithFundingReference(db, operationID, completedAction, 0)
}

func progressBillingAdmissionReserveOperationWithFundingReference(db *gorm.DB, operationID, completedAction string, fundingReferenceID int64) error {
	if err := validateBillingAdmissionReserveOperationID(operationID); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < billingAdmissionReserveMaxAttempts; attempt++ {
		operation, err := getBillingAdmissionReserveOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingAdmissionReserveError(err) {
				return err
			}
			time.Sleep(billingAdmissionReserveRetryDelay(attempt))
			continue
		}
		if err := validateBillingAdmissionReserveOperation(operation); err != nil {
			return err
		}
		if billingAdmissionReserveActionAlreadyConfirmed(operation, completedAction) {
			if fundingReferenceID > 0 && operation.FundingReferenceID != fundingReferenceID {
				return ErrBillingAdmissionReserveConflict
			}
			return nil
		}
		if operation.Status != BillingAdmissionReserveStatusPendingReconcile || operation.PendingAction != completedAction {
			return fmt.Errorf("%w: cannot confirm %s while %s", ErrBillingAdmissionReserveInvalidTransition, completedAction, operation.PendingAction)
		}
		updates, err := billingAdmissionReserveProgressUpdates(operation, completedAction)
		if err != nil {
			return err
		}
		if fundingReferenceID > 0 {
			if completedAction != BillingAdmissionReservePendingActionFundingUnknown ||
				operation.FundingSource != billingAdmissionReserveFundingSubscription {
				return ErrBillingAdmissionReserveInvalidInput
			}
			updates["funding_reference_id"] = fundingReferenceID
		}
		updates["revision"] = operation.Revision + 1
		result := db.Model(&BillingAdmissionReserveOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingAdmissionReserveStatusPendingReconcile, completedAction).
			Updates(updates)
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			return nil
		} else {
			lastErr = errBillingAdmissionReserveRevisionConflict
		}
		if !isRetryableBillingAdmissionReserveError(lastErr) {
			return lastErr
		}
		time.Sleep(billingAdmissionReserveRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrBillingAdmissionReserveContention, lastErr)
}

func billingAdmissionReserveProgressUpdates(operation BillingAdmissionReserveOperation, completedAction string) (map[string]any, error) {
	switch completedAction {
	case BillingAdmissionReservePendingActionFundingUnknown:
		nextAction := BillingAdmissionReservePendingActionCommitAfterReserve
		if operation.TokenQuota > 0 {
			nextAction = BillingAdmissionReservePendingActionTokenReady
		}
		return map[string]any{
			"funding_reserved_quota": operation.DeltaQuota,
			"pending_action":         nextAction,
		}, nil
	case BillingAdmissionReservePendingActionTokenUnknown:
		return map[string]any{
			"token_reserved_quota": operation.TokenQuota,
			"pending_action":       BillingAdmissionReservePendingActionCommitAfterReserve,
		}, nil
	case BillingAdmissionReservePendingActionTokenRefundUnknown:
		return map[string]any{
			"token_refunded_quota": operation.TokenReservedQuota,
			"pending_action":       BillingAdmissionReservePendingActionFundingRefundReady,
		}, nil
	case BillingAdmissionReservePendingActionFundingRefundUnknown:
		return map[string]any{
			"funding_refunded_quota": operation.FundingReservedQuota,
			"pending_action":         BillingAdmissionReservePendingActionCommitAfterRefund,
		}, nil
	default:
		return nil, ErrBillingAdmissionReserveInvalidInput
	}
}

func billingAdmissionReserveActionAlreadyConfirmed(operation BillingAdmissionReserveOperation, completedAction string) bool {
	switch completedAction {
	case BillingAdmissionReservePendingActionFundingUnknown:
		return operation.FundingReservedQuota == operation.DeltaQuota &&
			operation.PendingAction != BillingAdmissionReservePendingActionFundingReady &&
			operation.PendingAction != BillingAdmissionReservePendingActionFundingUnknown
	case BillingAdmissionReservePendingActionTokenUnknown:
		return operation.TokenQuota > 0 && operation.TokenReservedQuota == operation.TokenQuota &&
			operation.PendingAction != BillingAdmissionReservePendingActionTokenReady &&
			operation.PendingAction != BillingAdmissionReservePendingActionTokenUnknown
	case BillingAdmissionReservePendingActionTokenRefundUnknown:
		return operation.TokenReservedQuota > 0 && operation.TokenRefundedQuota == operation.TokenReservedQuota &&
			operation.PendingAction != BillingAdmissionReservePendingActionTokenRefundReady &&
			operation.PendingAction != BillingAdmissionReservePendingActionTokenRefundUnknown
	case BillingAdmissionReservePendingActionFundingRefundUnknown:
		return operation.FundingReservedQuota > 0 && operation.FundingRefundedQuota == operation.FundingReservedQuota &&
			operation.PendingAction != BillingAdmissionReservePendingActionFundingRefundReady &&
			operation.PendingAction != BillingAdmissionReservePendingActionFundingRefundUnknown
	default:
		return false
	}
}

func finishBillingAdmissionReserveOperation(db *gorm.DB, operationID, expectedAction, targetStatus string) error {
	if err := validateBillingAdmissionReserveOperationID(operationID); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < billingAdmissionReserveMaxAttempts; attempt++ {
		operation, err := getBillingAdmissionReserveOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingAdmissionReserveError(err) {
				return err
			}
			time.Sleep(billingAdmissionReserveRetryDelay(attempt))
			continue
		}
		if err := validateBillingAdmissionReserveOperation(operation); err != nil {
			return err
		}
		if operation.Status == targetStatus {
			return nil
		}
		if operation.Status != BillingAdmissionReserveStatusPendingReconcile || operation.PendingAction != expectedAction {
			return fmt.Errorf("%w: %s -> %s", ErrBillingAdmissionReserveInvalidTransition, operation.PendingAction, targetStatus)
		}
		result := db.Model(&BillingAdmissionReserveOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingAdmissionReserveStatusPendingReconcile, expectedAction).
			Updates(map[string]any{
				"status":         targetStatus,
				"pending_action": "",
				"revision":       operation.Revision + 1,
			})
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			return nil
		} else {
			lastErr = errBillingAdmissionReserveRevisionConflict
		}
		if !isRetryableBillingAdmissionReserveError(lastErr) {
			return lastErr
		}
		time.Sleep(billingAdmissionReserveRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrBillingAdmissionReserveContention, lastErr)
}

func ListRecoverableBillingAdmissionReserveOperations(staleBefore time.Time, limit int) ([]BillingAdmissionReserveOperation, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	return listRecoverableBillingAdmissionReserveOperations(DB, staleBefore, limit)
}

func listRecoverableBillingAdmissionReserveOperations(db *gorm.DB, staleBefore time.Time, limit int) ([]BillingAdmissionReserveOperation, error) {
	if staleBefore.IsZero() || limit <= 0 || limit > 1000 {
		return nil, ErrBillingAdmissionReserveInvalidInput
	}
	actions := []string{
		BillingAdmissionReservePendingActionFundingReady,
		BillingAdmissionReservePendingActionTokenReady,
		BillingAdmissionReservePendingActionCommitAfterReserve,
		BillingAdmissionReservePendingActionTokenRefundReady,
		BillingAdmissionReservePendingActionFundingRefundReady,
		BillingAdmissionReservePendingActionCommitAfterRefund,
	}
	var operations []BillingAdmissionReserveOperation
	err := db.Where("status = ? AND pending_action IN ? AND updated_at <= ?", BillingAdmissionReserveStatusPendingReconcile, actions, staleBefore).
		Order("id ASC").Limit(limit).Find(&operations).Error
	return operations, err
}

func ListManualBillingAdmissionReserveOperations(limit int) ([]BillingAdmissionReserveOperation, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	return listManualBillingAdmissionReserveOperations(DB, limit)
}

func listManualBillingAdmissionReserveOperations(db *gorm.DB, limit int) ([]BillingAdmissionReserveOperation, error) {
	if limit <= 0 || limit > 1000 {
		return nil, ErrBillingAdmissionReserveInvalidInput
	}
	actions := []string{
		BillingAdmissionReservePendingActionFundingUnknown,
		BillingAdmissionReservePendingActionTokenUnknown,
		BillingAdmissionReservePendingActionTokenRefundUnknown,
		BillingAdmissionReservePendingActionFundingRefundUnknown,
	}
	var operations []BillingAdmissionReserveOperation
	err := db.Where("status = ? AND pending_action IN ?", BillingAdmissionReserveStatusPendingReconcile, actions).
		Order("id ASC").Limit(limit).Find(&operations).Error
	return operations, err
}

func validateBillingAdmissionReserveInput(input BillingAdmissionReserveInput) error {
	if strings.TrimSpace(input.OperationID) == "" || len(input.OperationID) > maxBillingAdmissionOperationIDLength ||
		strings.TrimSpace(input.SessionID) == "" || len(input.SessionID) > maxBillingAdmissionSessionIDLength ||
		strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > maxBillingAdmissionRequestIDLength ||
		input.Attempt < 0 || input.UserID <= 0 || input.TokenID < 0 ||
		strings.TrimSpace(input.FundingSource) == "" || len(input.FundingSource) > maxBillingAdmissionFundingSourceLength ||
		input.FundingReferenceID < 0 || input.FromQuota < 0 || input.FromQuota > common.MaxQuota ||
		input.TargetQuota <= input.FromQuota || input.TargetQuota > common.MaxQuota ||
		input.TokenQuota < 0 || input.TokenQuota > input.TargetQuota-input.FromQuota ||
		(input.TokenQuota != 0 && input.TokenQuota != input.TargetQuota-input.FromQuota) {
		return ErrBillingAdmissionReserveInvalidInput
	}
	if input.FundingSource != billingAdmissionReserveFundingWallet && input.FundingSource != billingAdmissionReserveFundingSubscription {
		return ErrBillingAdmissionReserveInvalidInput
	}
	if input.FundingSource == billingAdmissionReserveFundingWallet && input.FundingReferenceID != int64(input.UserID) {
		return ErrBillingAdmissionReserveInvalidInput
	}
	switch input.Mode {
	case BillingAdmissionReserveModeStandard, BillingAdmissionReserveModeStrictWallet:
		return nil
	case BillingAdmissionReserveModeInitial:
		if input.Attempt != 0 || input.FromQuota != 0 {
			return ErrBillingAdmissionReserveInvalidInput
		}
		return nil
	default:
		return ErrBillingAdmissionReserveInvalidInput
	}
}

func reuseBillingAdmissionReserveOperation(
	operation BillingAdmissionReserveOperation,
	input BillingAdmissionReserveInput,
) (BillingAdmissionReserveOperation, error) {
	if err := validateBillingAdmissionReserveOperation(operation); err != nil {
		return BillingAdmissionReserveOperation{}, err
	}
	fundingReferenceMatches := operation.FundingReferenceID == input.FundingReferenceID ||
		(input.Mode == BillingAdmissionReserveModeInitial &&
			input.FundingSource == billingAdmissionReserveFundingSubscription &&
			input.FundingReferenceID == 0 && operation.FundingReferenceID > 0)
	if operation.OperationID != input.OperationID || operation.SessionID != input.SessionID ||
		operation.RequestID != input.RequestID || operation.Attempt != input.Attempt ||
		operation.UserID != input.UserID || operation.TokenID != input.TokenID ||
		operation.FundingSource != input.FundingSource || !fundingReferenceMatches ||
		operation.FromQuota != input.FromQuota || operation.TargetQuota != input.TargetQuota ||
		operation.TokenQuota != input.TokenQuota || operation.Mode != input.Mode {
		return BillingAdmissionReserveOperation{}, ErrBillingAdmissionReserveConflict
	}
	return operation, nil
}

func validateBillingAdmissionReserveOperation(operation BillingAdmissionReserveOperation) error {
	input := BillingAdmissionReserveInput{
		OperationID:        operation.OperationID,
		SessionID:          operation.SessionID,
		RequestID:          operation.RequestID,
		Attempt:            operation.Attempt,
		UserID:             operation.UserID,
		TokenID:            operation.TokenID,
		FundingSource:      operation.FundingSource,
		FundingReferenceID: operation.FundingReferenceID,
		FromQuota:          operation.FromQuota,
		TargetQuota:        operation.TargetQuota,
		TokenQuota:         operation.TokenQuota,
		Mode:               operation.Mode,
	}
	if validateBillingAdmissionReserveInput(input) != nil || operation.Revision < 0 ||
		operation.DeltaQuota != operation.TargetQuota-operation.FromQuota ||
		operation.FundingReservedQuota < 0 || operation.FundingReservedQuota > operation.DeltaQuota ||
		operation.TokenReservedQuota < 0 || operation.TokenReservedQuota > operation.TokenQuota ||
		operation.FundingRefundedQuota < 0 || operation.FundingRefundedQuota > operation.FundingReservedQuota ||
		operation.TokenRefundedQuota < 0 || operation.TokenRefundedQuota > operation.TokenReservedQuota {
		return ErrBillingAdmissionReserveCorrupt
	}
	allFundingReserved := operation.FundingReservedQuota == operation.DeltaQuota
	allTokenReserved := operation.TokenReservedQuota == operation.TokenQuota
	allFundingRefunded := operation.FundingRefundedQuota == operation.FundingReservedQuota
	allTokenRefunded := operation.TokenRefundedQuota == operation.TokenReservedQuota
	switch operation.Status {
	case BillingAdmissionReserveStatusApplied:
		if operation.PendingAction != "" || !allFundingReserved || !allTokenReserved ||
			operation.FundingRefundedQuota != 0 || operation.TokenRefundedQuota != 0 {
			return ErrBillingAdmissionReserveCorrupt
		}
		return nil
	case BillingAdmissionReserveStatusCanceled:
		if operation.PendingAction != "" {
			return ErrBillingAdmissionReserveCorrupt
		}
		neverStarted := operation.FundingReservedQuota == 0 && operation.TokenReservedQuota == 0 &&
			operation.FundingRefundedQuota == 0 && operation.TokenRefundedQuota == 0
		fullyCompensated := allFundingReserved && allTokenRefunded && allFundingRefunded
		if !neverStarted && !fullyCompensated {
			return ErrBillingAdmissionReserveCorrupt
		}
		return nil
	case BillingAdmissionReserveStatusPendingReconcile:
		switch operation.PendingAction {
		case BillingAdmissionReservePendingActionFundingReady,
			BillingAdmissionReservePendingActionFundingUnknown:
			if operation.FundingReservedQuota != 0 || operation.TokenReservedQuota != 0 ||
				operation.FundingRefundedQuota != 0 || operation.TokenRefundedQuota != 0 {
				return ErrBillingAdmissionReserveCorrupt
			}
		case BillingAdmissionReservePendingActionTokenReady,
			BillingAdmissionReservePendingActionTokenUnknown:
			if operation.TokenQuota <= 0 || !allFundingReserved || operation.TokenReservedQuota != 0 ||
				operation.FundingRefundedQuota != 0 || operation.TokenRefundedQuota != 0 {
				return ErrBillingAdmissionReserveCorrupt
			}
		case BillingAdmissionReservePendingActionCommitAfterReserve:
			if !allFundingReserved || !allTokenReserved || operation.FundingRefundedQuota != 0 || operation.TokenRefundedQuota != 0 {
				return ErrBillingAdmissionReserveCorrupt
			}
		case BillingAdmissionReservePendingActionTokenRefundReady,
			BillingAdmissionReservePendingActionTokenRefundUnknown:
			if operation.TokenReservedQuota <= 0 || !allFundingReserved || !allTokenReserved ||
				operation.FundingRefundedQuota != 0 || operation.TokenRefundedQuota != 0 {
				return ErrBillingAdmissionReserveCorrupt
			}
		case BillingAdmissionReservePendingActionFundingRefundReady,
			BillingAdmissionReservePendingActionFundingRefundUnknown:
			if !allFundingReserved || !allTokenRefunded || operation.FundingRefundedQuota != 0 {
				return ErrBillingAdmissionReserveCorrupt
			}
		case BillingAdmissionReservePendingActionCommitAfterRefund:
			if !allFundingReserved || !allTokenRefunded || !allFundingRefunded {
				return ErrBillingAdmissionReserveCorrupt
			}
		default:
			return ErrBillingAdmissionReserveCorrupt
		}
		return nil
	default:
		return ErrBillingAdmissionReserveCorrupt
	}
}

func validateBillingAdmissionReserveOperationID(operationID string) error {
	if strings.TrimSpace(operationID) == "" || len(operationID) > maxBillingAdmissionOperationIDLength {
		return ErrBillingAdmissionReserveInvalidInput
	}
	return nil
}

func isRetryableBillingAdmissionReserveError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errBillingAdmissionReserveRevisionConflict) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "error 1205") ||
		strings.Contains(message, "error 1213") ||
		strings.Contains(message, "lock wait timeout exceeded") ||
		strings.Contains(message, "deadlock found") ||
		strings.Contains(message, "sqlstate 40001") ||
		strings.Contains(message, "sqlstate 40p01") ||
		strings.Contains(message, "could not serialize access") ||
		strings.Contains(message, "deadlock detected") ||
		strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate key")
}

func billingAdmissionReserveRetryDelay(attempt int) time.Duration {
	delay := time.Duration(attempt+1) * time.Millisecond
	if delay > 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return delay
}
