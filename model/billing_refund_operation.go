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
	BillingRefundStatusPendingReconcile = "pending_reconcile"
	BillingRefundStatusApplied          = "applied"

	BillingRefundPendingActionFundingReady             = "funding_refund_ready"
	BillingRefundPendingActionFundingUnknown           = "funding_refund_unknown"
	BillingRefundPendingActionSubscriptionExtraReady   = "subscription_extra_refund_ready"
	BillingRefundPendingActionSubscriptionExtraUnknown = "subscription_extra_refund_unknown"
	BillingRefundPendingActionTokenReady               = "token_refund_ready"
	BillingRefundPendingActionTokenUnknown             = "token_refund_unknown"
	BillingRefundPendingActionCommitAfterRefund        = "commit_after_refund"

	billingRefundFundingWallet       = "wallet"
	billingRefundFundingSubscription = "subscription"

	billingRefundMaxAttempts         = 64
	maxBillingRefundOperationIDLen   = 191
	maxBillingRefundSessionIDLen     = 128
	maxBillingRefundRequestIDLen     = 191
	maxBillingRefundFundingSourceLen = 32
)

var (
	ErrBillingRefundInvalidInput        = errors.New("invalid billing refund operation input")
	ErrBillingRefundConflict            = errors.New("billing refund operation conflicts with an existing operation")
	ErrBillingRefundInvalidTransition   = errors.New("invalid billing refund operation transition")
	ErrBillingRefundCorrupt             = errors.New("billing refund operation is inconsistent")
	ErrBillingRefundContention          = errors.New("billing refund operation contention exceeded retry limit")
	ErrBillingRefundIntentNotPersisted  = errors.New("billing refund intent was definitely not persisted")
	ErrBillingRefundIntentCommitUnknown = errors.New("billing refund intent commit outcome is unknown")
	errBillingRefundRevisionConflict    = errors.New("billing refund operation revision conflict")
)

// BillingRefundOperation is the durable, fail-closed evidence for refunding a
// BillingSession's initial reservation. PendingAction names the next external
// action whose outcome may be unknown. The refunded quota columns record only
// steps whose successful outcome was durably confirmed before another
// external action was allowed to start.
type BillingRefundOperation struct {
	Id                             int64     `json:"id" gorm:"primaryKey"`
	OperationID                    string    `json:"operation_id" gorm:"type:varchar(191);not null;uniqueIndex:idx_billing_refund_operation_id"`
	SessionID                      string    `json:"session_id" gorm:"type:varchar(128);not null;uniqueIndex:idx_billing_refund_session_id"`
	RequestID                      string    `json:"request_id" gorm:"type:varchar(191);not null;index:idx_billing_refund_request_id"`
	UserID                         int       `json:"user_id" gorm:"not null;index"`
	TokenID                        int       `json:"token_id" gorm:"not null;index"`
	FundingSource                  string    `json:"funding_source" gorm:"type:varchar(32);not null"`
	FundingReferenceID             int64     `json:"funding_reference_id" gorm:"not null;index"`
	FundingQuota                   int       `json:"funding_quota" gorm:"not null"`
	SubscriptionExtraQuota         int       `json:"subscription_extra_quota" gorm:"not null"`
	TokenQuota                     int       `json:"token_quota" gorm:"not null"`
	FundingRefundedQuota           int       `json:"funding_refunded_quota" gorm:"not null"`
	SubscriptionExtraRefundedQuota int       `json:"subscription_extra_refunded_quota" gorm:"not null"`
	TokenRefundedQuota             int       `json:"token_refunded_quota" gorm:"not null"`
	Status                         string    `json:"status" gorm:"type:varchar(32);not null;index"`
	PendingAction                  string    `json:"pending_action" gorm:"type:varchar(64);not null;index"`
	Revision                       int64     `json:"revision" gorm:"not null"`
	CreatedAt                      time.Time `json:"created_at"`
	UpdatedAt                      time.Time `json:"updated_at"`
}

func (BillingRefundOperation) TableName() string {
	return "billing_refund_operations"
}

type BillingRefundOperationInput struct {
	OperationID            string
	SessionID              string
	RequestID              string
	UserID                 int
	TokenID                int
	FundingSource          string
	FundingReferenceID     int64
	FundingQuota           int
	SubscriptionExtraQuota int
	TokenQuota             int
}

func BeginBillingRefundOperation(input BillingRefundOperationInput) (BillingRefundOperation, error) {
	if DB == nil {
		return BillingRefundOperation{}, errors.New("database is not initialized")
	}
	return beginBillingRefundOperation(DB, input)
}

func beginBillingRefundOperation(db *gorm.DB, input BillingRefundOperationInput) (BillingRefundOperation, error) {
	return beginBillingRefundOperationWithCommit(db, input, func(tx *gorm.DB) error {
		return tx.Commit().Error
	})
}

func beginBillingRefundOperationWithCommit(
	db *gorm.DB,
	input BillingRefundOperationInput,
	commitTransaction func(*gorm.DB) error,
) (BillingRefundOperation, error) {
	if err := validateBillingRefundOperationInput(input); err != nil {
		return BillingRefundOperation{}, err
	}
	if commitTransaction == nil {
		return BillingRefundOperation{}, ErrBillingRefundInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < billingRefundMaxAttempts; attempt++ {
		operation := BillingRefundOperation{
			OperationID:            input.OperationID,
			SessionID:              input.SessionID,
			RequestID:              input.RequestID,
			UserID:                 input.UserID,
			TokenID:                input.TokenID,
			FundingSource:          input.FundingSource,
			FundingReferenceID:     input.FundingReferenceID,
			FundingQuota:           input.FundingQuota,
			SubscriptionExtraQuota: input.SubscriptionExtraQuota,
			TokenQuota:             input.TokenQuota,
			Status:                 BillingRefundStatusPendingReconcile,
			PendingAction:          BillingRefundPendingActionFundingReady,
		}
		tx := db.Begin()
		if tx.Error != nil {
			lastErr = errors.Join(ErrBillingRefundIntentNotPersisted, tx.Error)
		} else if createErr := tx.Create(&operation).Error; createErr != nil {
			if rollbackErr := tx.Rollback().Error; rollbackErr != nil {
				lastErr = errors.Join(ErrBillingRefundIntentCommitUnknown, createErr, rollbackErr)
			} else if isBillingRefundUniquenessError(createErr) {
				// This transaction definitely did not insert, but another caller may
				// already own the operation/session identity. Until read-back resolves
				// that row, an unaudited fallback would risk a duplicate refund.
				lastErr = errors.Join(ErrBillingRefundIntentCommitUnknown, createErr)
			} else {
				lastErr = errors.Join(ErrBillingRefundIntentNotPersisted, createErr)
			}
		} else if commitErr := commitTransaction(tx); commitErr != nil {
			lastErr = errors.Join(ErrBillingRefundIntentCommitUnknown, commitErr)
		} else {
			lastErr = nil
		}
		if lastErr == nil {
			return operation, nil
		}

		existing, getErr := getBillingRefundOperation(db, input.OperationID)
		if getErr == nil {
			return reuseBillingRefundOperation(existing, input)
		}
		if getErr != nil && !errors.Is(getErr, gorm.ErrRecordNotFound) {
			lastErr = getErr
		}

		var sameSession BillingRefundOperation
		sessionErr := db.Where("session_id = ?", input.SessionID).First(&sameSession).Error
		if sessionErr == nil {
			return reuseBillingRefundOperation(sameSession, input)
		}
		if sessionErr != nil && !errors.Is(sessionErr, gorm.ErrRecordNotFound) {
			lastErr = sessionErr
		}

		if errors.Is(lastErr, ErrBillingRefundIntentNotPersisted) && !isRetryableBillingRefundError(lastErr) {
			return BillingRefundOperation{}, lastErr
		}
		time.Sleep(billingRefundRetryDelay(attempt))
	}
	if errors.Is(lastErr, ErrBillingRefundIntentCommitUnknown) {
		return BillingRefundOperation{}, lastErr
	}
	return BillingRefundOperation{}, fmt.Errorf("%w: %w", ErrBillingRefundContention, lastErr)
}

func ConfirmBillingRefundFunding(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return confirmBillingRefundFunding(DB, operationID)
}

func confirmBillingRefundFunding(db *gorm.DB, operationID string) error {
	return progressBillingRefundOperation(db, operationID, BillingRefundPendingActionFundingUnknown)
}

func ConfirmBillingRefundSubscriptionExtra(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return confirmBillingRefundSubscriptionExtra(DB, operationID)
}

func confirmBillingRefundSubscriptionExtra(db *gorm.DB, operationID string) error {
	return progressBillingRefundOperation(db, operationID, BillingRefundPendingActionSubscriptionExtraUnknown)
}

func ConfirmBillingRefundToken(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return confirmBillingRefundToken(DB, operationID)
}

func confirmBillingRefundToken(db *gorm.DB, operationID string) error {
	return progressBillingRefundOperation(db, operationID, BillingRefundPendingActionTokenUnknown)
}

func CommitBillingRefundOperation(operationID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return commitBillingRefundOperation(DB, operationID)
}

func commitBillingRefundOperation(db *gorm.DB, operationID string) error {
	if err := validateBillingRefundOperationID(operationID); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < billingRefundMaxAttempts; attempt++ {
		operation, err := getBillingRefundOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingRefundError(err) {
				return err
			}
			time.Sleep(billingRefundRetryDelay(attempt))
			continue
		}
		if err := validateBillingRefundOperation(operation); err != nil {
			return err
		}
		if operation.Status == BillingRefundStatusApplied {
			return nil
		}
		if operation.PendingAction != BillingRefundPendingActionCommitAfterRefund {
			return fmt.Errorf("%w: %s -> %s", ErrBillingRefundInvalidTransition, operation.PendingAction, BillingRefundStatusApplied)
		}
		result := db.Model(&BillingRefundOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingRefundStatusPendingReconcile, BillingRefundPendingActionCommitAfterRefund).
			Updates(map[string]any{
				"status":         BillingRefundStatusApplied,
				"pending_action": "",
				"revision":       operation.Revision + 1,
			})
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			return nil
		} else {
			lastErr = errBillingRefundRevisionConflict
		}
		if !isRetryableBillingRefundError(lastErr) {
			return lastErr
		}
		time.Sleep(billingRefundRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrBillingRefundContention, lastErr)
}

func GetBillingRefundOperation(operationID string) (BillingRefundOperation, error) {
	if DB == nil {
		return BillingRefundOperation{}, errors.New("database is not initialized")
	}
	operation, err := getBillingRefundOperation(DB, operationID)
	if err != nil {
		return BillingRefundOperation{}, err
	}
	if err := validateBillingRefundOperation(operation); err != nil {
		return BillingRefundOperation{}, err
	}
	return operation, nil
}

type BillingRefundActionClaim struct {
	Operation BillingRefundOperation
	Claimed   bool
}

// ClaimNextBillingRefundAction durably records that the expected external
// refund action is about to start. A stale caller cannot claim a later ready
// action because expectedReadyAction is part of the compare-and-swap contract.
func ClaimNextBillingRefundAction(operationID, expectedReadyAction string) (BillingRefundActionClaim, error) {
	if DB == nil {
		return BillingRefundActionClaim{}, errors.New("database is not initialized")
	}
	return claimNextBillingRefundAction(DB, operationID, expectedReadyAction)
}

func claimNextBillingRefundAction(db *gorm.DB, operationID, expectedReadyAction string) (BillingRefundActionClaim, error) {
	if err := validateBillingRefundOperationID(operationID); err != nil {
		return BillingRefundActionClaim{}, err
	}
	unknownAction := billingRefundUnknownAction(expectedReadyAction)
	if unknownAction == "" {
		return BillingRefundActionClaim{}, ErrBillingRefundInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < billingRefundMaxAttempts; attempt++ {
		operation, err := getBillingRefundOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingRefundError(err) {
				return BillingRefundActionClaim{}, err
			}
			time.Sleep(billingRefundRetryDelay(attempt))
			continue
		}
		if err := validateBillingRefundOperation(operation); err != nil {
			return BillingRefundActionClaim{}, err
		}

		if operation.Status != BillingRefundStatusPendingReconcile || operation.PendingAction != expectedReadyAction {
			return BillingRefundActionClaim{Operation: operation}, nil
		}
		result := db.Model(&BillingRefundOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingRefundStatusPendingReconcile, expectedReadyAction).
			Updates(map[string]any{
				"pending_action": unknownAction,
				"revision":       operation.Revision + 1,
			})
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			operation.PendingAction = unknownAction
			operation.Revision++
			return BillingRefundActionClaim{Operation: operation, Claimed: true}, nil
		} else {
			lastErr = errBillingRefundRevisionConflict
		}
		if !isRetryableBillingRefundError(lastErr) {
			return BillingRefundActionClaim{}, lastErr
		}
		time.Sleep(billingRefundRetryDelay(attempt))
	}
	return BillingRefundActionClaim{}, fmt.Errorf("%w: %w", ErrBillingRefundContention, lastErr)
}

func billingRefundUnknownAction(readyAction string) string {
	switch readyAction {
	case BillingRefundPendingActionFundingReady:
		return BillingRefundPendingActionFundingUnknown
	case BillingRefundPendingActionSubscriptionExtraReady:
		return BillingRefundPendingActionSubscriptionExtraUnknown
	case BillingRefundPendingActionTokenReady:
		return BillingRefundPendingActionTokenUnknown
	default:
		return ""
	}
}

func getBillingRefundOperation(db *gorm.DB, operationID string) (BillingRefundOperation, error) {
	if err := validateBillingRefundOperationID(operationID); err != nil {
		return BillingRefundOperation{}, err
	}
	var operation BillingRefundOperation
	err := db.Where("operation_id = ?", operationID).First(&operation).Error
	return operation, err
}

func ListRecoverableBillingRefundOperations(staleBefore time.Time, limit int) ([]BillingRefundOperation, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	return listRecoverableBillingRefundOperations(DB, staleBefore, limit)
}

func listRecoverableBillingRefundOperations(db *gorm.DB, staleBefore time.Time, limit int) ([]BillingRefundOperation, error) {
	if staleBefore.IsZero() || limit <= 0 || limit > 1000 {
		return nil, ErrBillingRefundInvalidInput
	}
	actions := []string{
		BillingRefundPendingActionFundingReady,
		BillingRefundPendingActionSubscriptionExtraReady,
		BillingRefundPendingActionTokenReady,
		BillingRefundPendingActionCommitAfterRefund,
	}
	var operations []BillingRefundOperation
	err := db.Where("status = ? AND pending_action IN ? AND updated_at <= ?", BillingRefundStatusPendingReconcile, actions, staleBefore).
		Order("id ASC").Limit(limit).Find(&operations).Error
	return operations, err
}

func ListManualBillingRefundOperations(limit int) ([]BillingRefundOperation, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	return listManualBillingRefundOperations(DB, limit)
}

func listManualBillingRefundOperations(db *gorm.DB, limit int) ([]BillingRefundOperation, error) {
	if limit <= 0 || limit > 1000 {
		return nil, ErrBillingRefundInvalidInput
	}
	actions := []string{
		BillingRefundPendingActionFundingUnknown,
		BillingRefundPendingActionSubscriptionExtraUnknown,
		BillingRefundPendingActionTokenUnknown,
	}
	var operations []BillingRefundOperation
	err := db.Where("status = ? AND pending_action IN ?", BillingRefundStatusPendingReconcile, actions).
		Order("id ASC").Limit(limit).Find(&operations).Error
	return operations, err
}

func progressBillingRefundOperation(db *gorm.DB, operationID, completedAction string) error {
	if err := validateBillingRefundOperationID(operationID); err != nil {
		return err
	}
	if completedAction != BillingRefundPendingActionFundingUnknown &&
		completedAction != BillingRefundPendingActionSubscriptionExtraUnknown &&
		completedAction != BillingRefundPendingActionTokenUnknown {
		return ErrBillingRefundInvalidInput
	}

	var lastErr error
	for attempt := 0; attempt < billingRefundMaxAttempts; attempt++ {
		operation, err := getBillingRefundOperation(db, operationID)
		if err != nil {
			lastErr = err
			if !isRetryableBillingRefundError(err) {
				return err
			}
			time.Sleep(billingRefundRetryDelay(attempt))
			continue
		}
		if err := validateBillingRefundOperation(operation); err != nil {
			return err
		}
		if billingRefundActionAlreadyConfirmed(operation, completedAction) {
			return nil
		}
		if operation.Status != BillingRefundStatusPendingReconcile || operation.PendingAction != completedAction {
			return fmt.Errorf("%w: cannot confirm %s while %s", ErrBillingRefundInvalidTransition, completedAction, operation.PendingAction)
		}

		updates, err := billingRefundProgressUpdates(operation, completedAction)
		if err != nil {
			return err
		}
		updates["revision"] = operation.Revision + 1
		result := db.Model(&BillingRefundOperation{}).
			Where("id = ? AND revision = ? AND status = ? AND pending_action = ?", operation.Id, operation.Revision, BillingRefundStatusPendingReconcile, completedAction).
			Updates(updates)
		if result.Error != nil {
			lastErr = result.Error
		} else if result.RowsAffected == 1 {
			return nil
		} else {
			lastErr = errBillingRefundRevisionConflict
		}
		if !isRetryableBillingRefundError(lastErr) {
			return lastErr
		}
		time.Sleep(billingRefundRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrBillingRefundContention, lastErr)
}

func billingRefundProgressUpdates(operation BillingRefundOperation, completedAction string) (map[string]any, error) {
	updates := make(map[string]any, 2)
	switch completedAction {
	case BillingRefundPendingActionFundingUnknown:
		updates["funding_refunded_quota"] = operation.FundingQuota
		updates["pending_action"] = nextBillingRefundAction(operation, completedAction)
	case BillingRefundPendingActionSubscriptionExtraUnknown:
		if operation.SubscriptionExtraQuota <= 0 {
			return nil, ErrBillingRefundInvalidTransition
		}
		updates["subscription_extra_refunded_quota"] = operation.SubscriptionExtraQuota
		updates["pending_action"] = nextBillingRefundAction(operation, completedAction)
	case BillingRefundPendingActionTokenUnknown:
		if operation.TokenQuota <= 0 {
			return nil, ErrBillingRefundInvalidTransition
		}
		updates["token_refunded_quota"] = operation.TokenQuota
		updates["pending_action"] = BillingRefundPendingActionCommitAfterRefund
	default:
		return nil, ErrBillingRefundInvalidInput
	}
	return updates, nil
}

func nextBillingRefundAction(operation BillingRefundOperation, completedAction string) string {
	switch completedAction {
	case BillingRefundPendingActionFundingUnknown:
		if operation.SubscriptionExtraQuota > 0 {
			return BillingRefundPendingActionSubscriptionExtraReady
		}
		if operation.TokenQuota > 0 {
			return BillingRefundPendingActionTokenReady
		}
	case BillingRefundPendingActionSubscriptionExtraUnknown:
		if operation.TokenQuota > 0 {
			return BillingRefundPendingActionTokenReady
		}
	}
	return BillingRefundPendingActionCommitAfterRefund
}

func billingRefundActionAlreadyConfirmed(operation BillingRefundOperation, completedAction string) bool {
	switch completedAction {
	case BillingRefundPendingActionFundingUnknown:
		return operation.PendingAction != BillingRefundPendingActionFundingReady &&
			operation.PendingAction != BillingRefundPendingActionFundingUnknown &&
			operation.FundingRefundedQuota == operation.FundingQuota
	case BillingRefundPendingActionSubscriptionExtraUnknown:
		return operation.SubscriptionExtraQuota > 0 &&
			operation.PendingAction != BillingRefundPendingActionFundingReady &&
			operation.PendingAction != BillingRefundPendingActionFundingUnknown &&
			operation.PendingAction != BillingRefundPendingActionSubscriptionExtraReady &&
			operation.PendingAction != BillingRefundPendingActionSubscriptionExtraUnknown &&
			operation.SubscriptionExtraRefundedQuota == operation.SubscriptionExtraQuota
	case BillingRefundPendingActionTokenUnknown:
		return operation.TokenQuota > 0 &&
			(operation.PendingAction == BillingRefundPendingActionCommitAfterRefund || operation.Status == BillingRefundStatusApplied) &&
			operation.TokenRefundedQuota == operation.TokenQuota
	default:
		return false
	}
}

func reuseBillingRefundOperation(operation BillingRefundOperation, input BillingRefundOperationInput) (BillingRefundOperation, error) {
	if err := validateBillingRefundOperation(operation); err != nil {
		return BillingRefundOperation{}, err
	}
	if operation.OperationID != input.OperationID || operation.SessionID != input.SessionID ||
		operation.RequestID != input.RequestID || operation.UserID != input.UserID || operation.TokenID != input.TokenID ||
		operation.FundingSource != input.FundingSource || operation.FundingReferenceID != input.FundingReferenceID ||
		operation.FundingQuota != input.FundingQuota || operation.SubscriptionExtraQuota != input.SubscriptionExtraQuota ||
		operation.TokenQuota != input.TokenQuota {
		return BillingRefundOperation{}, ErrBillingRefundConflict
	}
	return operation, nil
}

func validateBillingRefundOperationInput(input BillingRefundOperationInput) error {
	if strings.TrimSpace(input.OperationID) == "" || len(input.OperationID) > maxBillingRefundOperationIDLen ||
		strings.TrimSpace(input.SessionID) == "" || len(input.SessionID) > maxBillingRefundSessionIDLen ||
		strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > maxBillingRefundRequestIDLen ||
		input.UserID <= 0 || input.TokenID < 0 || input.FundingReferenceID <= 0 ||
		strings.TrimSpace(input.FundingSource) == "" || len(input.FundingSource) > maxBillingRefundFundingSourceLen ||
		!validBillingRefundQuota(input.FundingQuota) || !validBillingRefundQuota(input.SubscriptionExtraQuota) ||
		!validBillingRefundQuota(input.TokenQuota) ||
		(input.FundingQuota == 0 && input.SubscriptionExtraQuota == 0 && input.TokenQuota == 0) {
		return ErrBillingRefundInvalidInput
	}
	switch input.FundingSource {
	case billingRefundFundingWallet:
		if input.FundingReferenceID != int64(input.UserID) || input.SubscriptionExtraQuota != 0 {
			return ErrBillingRefundInvalidInput
		}
	case billingRefundFundingSubscription:
		// A subscription operation may additionally refund an admission top-up.
	default:
		return ErrBillingRefundInvalidInput
	}
	return nil
}

func validateBillingRefundOperation(operation BillingRefundOperation) error {
	input := BillingRefundOperationInput{
		OperationID:            operation.OperationID,
		SessionID:              operation.SessionID,
		RequestID:              operation.RequestID,
		UserID:                 operation.UserID,
		TokenID:                operation.TokenID,
		FundingSource:          operation.FundingSource,
		FundingReferenceID:     operation.FundingReferenceID,
		FundingQuota:           operation.FundingQuota,
		SubscriptionExtraQuota: operation.SubscriptionExtraQuota,
		TokenQuota:             operation.TokenQuota,
	}
	if validateBillingRefundOperationInput(input) != nil || operation.Revision < 0 ||
		!validBillingRefundQuota(operation.FundingRefundedQuota) ||
		!validBillingRefundQuota(operation.SubscriptionExtraRefundedQuota) ||
		!validBillingRefundQuota(operation.TokenRefundedQuota) {
		return ErrBillingRefundCorrupt
	}

	allFunding := operation.FundingRefundedQuota == operation.FundingQuota
	allExtra := operation.SubscriptionExtraRefundedQuota == operation.SubscriptionExtraQuota
	allToken := operation.TokenRefundedQuota == operation.TokenQuota
	switch operation.Status {
	case BillingRefundStatusApplied:
		if operation.PendingAction != "" || !allFunding || !allExtra || !allToken {
			return ErrBillingRefundCorrupt
		}
		return nil
	case BillingRefundStatusPendingReconcile:
		switch operation.PendingAction {
		case BillingRefundPendingActionFundingReady, BillingRefundPendingActionFundingUnknown:
			if operation.FundingRefundedQuota != 0 || operation.SubscriptionExtraRefundedQuota != 0 || operation.TokenRefundedQuota != 0 {
				return ErrBillingRefundCorrupt
			}
		case BillingRefundPendingActionSubscriptionExtraReady, BillingRefundPendingActionSubscriptionExtraUnknown:
			if operation.SubscriptionExtraQuota <= 0 || !allFunding || operation.SubscriptionExtraRefundedQuota != 0 || operation.TokenRefundedQuota != 0 {
				return ErrBillingRefundCorrupt
			}
		case BillingRefundPendingActionTokenReady, BillingRefundPendingActionTokenUnknown:
			if operation.TokenQuota <= 0 || !allFunding || !allExtra || operation.TokenRefundedQuota != 0 {
				return ErrBillingRefundCorrupt
			}
		case BillingRefundPendingActionCommitAfterRefund:
			if !allFunding || !allExtra || !allToken {
				return ErrBillingRefundCorrupt
			}
		default:
			return ErrBillingRefundCorrupt
		}
		return nil
	default:
		return ErrBillingRefundCorrupt
	}
}

func validateBillingRefundOperationID(operationID string) error {
	if strings.TrimSpace(operationID) == "" || len(operationID) > maxBillingRefundOperationIDLen {
		return ErrBillingRefundInvalidInput
	}
	return nil
}

func validBillingRefundQuota(quota int) bool {
	return quota >= 0 && quota <= common.MaxQuota
}

func isRetryableBillingRefundError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errBillingRefundRevisionConflict) {
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

func isBillingRefundUniquenessError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}

func billingRefundRetryDelay(attempt int) time.Duration {
	delay := time.Duration(attempt+1) * time.Millisecond
	if delay > 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return delay
}
