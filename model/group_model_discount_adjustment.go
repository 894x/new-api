package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrGroupModelDiscountAdjustmentConflict            = errors.New("group model discount adjustment id conflicts with a different target")
	ErrGroupModelDiscountAdjustmentRequiresFullReverse = errors.New("zero original quota requires a full settled reversal")
	ErrGroupModelDiscountAdjustmentChargeUnderflow     = errors.New("group model discount adjustment would make the parent charge negative")
)

// GroupModelDiscountAdjustment is an idempotent two-phase change to an
// already-settled task charge. The parent settlement's initial fields remain
// immutable; Current* fields represent the amount that is still active.
type GroupModelDiscountAdjustment struct {
	Id                             int64                       `json:"id" gorm:"primaryKey"`
	AdjustmentID                   string                      `json:"adjustment_id" gorm:"type:varchar(191);not null;uniqueIndex"`
	Fingerprint                    string                      `json:"fingerprint" gorm:"type:char(64);not null"`
	SettlementID                   int64                       `json:"settlement_id" gorm:"not null;index"`
	SettlementRequestID            string                      `json:"settlement_request_id" gorm:"type:varchar(191);not null;index"`
	UsageID                        int64                       `json:"usage_id" gorm:"not null;index:idx_discount_adjustment_usage_status,priority:1"`
	UserID                         int                         `json:"user_id" gorm:"not null;index"`
	ChannelID                      int                         `json:"channel_id" gorm:"not null"`
	PolicyHash                     string                      `json:"policy_hash" gorm:"type:varchar(128);not null"`
	ProgressBasis                  groupdiscount.ProgressBasis `json:"progress_basis" gorm:"type:varchar(16);not null"`
	PreviousOriginalQuota          int64                       `json:"previous_original_quota" gorm:"not null"`
	PreviousChargedQuota           int64                       `json:"previous_charged_quota" gorm:"not null"`
	PreviousProgressQuota          string                      `json:"previous_progress_quota" gorm:"type:text;not null"`
	NewOriginalQuota               int64                       `json:"new_original_quota" gorm:"not null"`
	NewChargedQuota                int64                       `json:"new_charged_quota" gorm:"not null"`
	NewProgressQuota               string                      `json:"new_progress_quota" gorm:"type:text;not null"`
	DeltaOriginalQuota             int64                       `json:"delta_original_quota" gorm:"not null"`
	DeltaChargedQuota              int64                       `json:"delta_charged_quota" gorm:"not null"`
	DeltaProgressQuota             string                      `json:"delta_progress_quota" gorm:"type:text;not null"`
	UsageOriginalBefore            int64                       `json:"usage_original_before" gorm:"not null"`
	UsageOriginalAfter             int64                       `json:"usage_original_after" gorm:"not null"`
	UsageChargedBefore             int64                       `json:"usage_charged_before" gorm:"not null"`
	UsageChargedAfter              int64                       `json:"usage_charged_after" gorm:"not null"`
	UsageProgressBefore            string                      `json:"usage_progress_before" gorm:"type:text;not null"`
	UsageProgressAfter             string                      `json:"usage_progress_after" gorm:"type:text;not null"`
	PreviousTailSettlementID       int64                       `json:"previous_tail_settlement_id" gorm:"not null"`
	ParentAdjustmentRevisionBefore int64                       `json:"parent_adjustment_revision_before" gorm:"not null"`
	ParentAdjustmentRevisionAfter  int64                       `json:"parent_adjustment_revision_after" gorm:"not null"`
	ParentLastCursorRevisionBefore int64                       `json:"parent_last_cursor_revision_before" gorm:"not null"`
	ParentLastCursorRevisionAfter  int64                       `json:"parent_last_cursor_revision_after" gorm:"not null"`
	Segments                       string                      `json:"segments" gorm:"type:text;not null"`
	Status                         string                      `json:"status" gorm:"type:varchar(32);not null;index:idx_discount_adjustment_usage_status,priority:2"`
	PendingAction                  string                      `json:"pending_action" gorm:"type:varchar(32);not null"`
	AccountingApplied              bool                        `json:"accounting_applied" gorm:"not null"`
	AccountingUserID               int                         `json:"accounting_user_id" gorm:"not null"`
	AccountingChannelID            int                         `json:"accounting_channel_id" gorm:"not null"`
	AccountingQuotaDelta           int                         `json:"accounting_quota_delta" gorm:"not null"`
	AccountingRequestCountDelta    int                         `json:"accounting_request_count_delta" gorm:"not null"`
	Revision                       int64                       `json:"revision" gorm:"not null"`
	CreatedAt                      time.Time                   `json:"created_at"`
	UpdatedAt                      time.Time                   `json:"updated_at"`
}

func (GroupModelDiscountAdjustment) TableName() string {
	return "group_model_discount_adjustments"
}

type GroupModelDiscountAdjustmentInput struct {
	AdjustmentID        string
	SettlementRequestID string
	NewOriginalQuota    int
}

type GroupModelDiscountAdjustmentReservation struct {
	Adjustment            GroupModelDiscountAdjustment
	PreviousOriginalQuota int
	PreviousChargedQuota  int
	NewOriginalQuota      int
	NewChargedQuota       int
	DeltaOriginalQuota    int
	DeltaChargedQuota     int
	Segments              []groupdiscount.Segment
	Reused                bool
}

type groupModelDiscountAdjustmentFingerprint struct {
	SettlementRequestID string `json:"settlement_request_id"`
	NewOriginalQuota    int    `json:"new_original_quota"`
}

func ReserveGroupModelDiscountAdjustment(input GroupModelDiscountAdjustmentInput) (GroupModelDiscountAdjustmentReservation, error) {
	if DB == nil {
		return GroupModelDiscountAdjustmentReservation{}, errors.New("database is not initialized")
	}
	return reserveGroupModelDiscountAdjustment(DB, input)
}

func reserveGroupModelDiscountAdjustment(db *gorm.DB, input GroupModelDiscountAdjustmentInput) (GroupModelDiscountAdjustmentReservation, error) {
	fingerprint, err := validateAndFingerprintGroupModelDiscountAdjustment(input)
	if err != nil {
		return GroupModelDiscountAdjustmentReservation{}, err
	}

	for attempt := 0; attempt < groupModelDiscountMaxAttempts; attempt++ {
		var reservation GroupModelDiscountAdjustmentReservation
		err = db.Transaction(func(tx *gorm.DB) error {
			var existing GroupModelDiscountAdjustment
			existingErr := lockForUpdate(tx).Where("adjustment_id = ?", input.AdjustmentID).First(&existing).Error
			if existingErr == nil {
				loaded, loadErr := reservationFromGroupModelDiscountAdjustment(existing, fingerprint, true)
				if loadErr != nil {
					return loadErr
				}
				reservation = loaded
				return nil
			}
			if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
				return existingErr
			}

			var parentHint GroupModelDiscountSettlement
			if queryErr := tx.Where("request_id = ?", input.SettlementRequestID).First(&parentHint).Error; queryErr != nil {
				return queryErr
			}
			var usage UserGroupModelMonthlyUsage
			if queryErr := lockForUpdate(tx).Where("id = ?", parentHint.UsageID).First(&usage).Error; queryErr != nil {
				return queryErr
			}
			var parent GroupModelDiscountSettlement
			if queryErr := lockForUpdate(tx).Where("id = ?", parentHint.Id).First(&parent).Error; queryErr != nil {
				return queryErr
			}
			if parent.RequestID != input.SettlementRequestID || parent.Status != GroupModelDiscountStatusSettled {
				return fmt.Errorf("%w: parent settlement status %s", ErrGroupModelDiscountInvalidTransition, parent.Status)
			}
			if parent.CurrentOriginalQuota <= 0 || parent.CurrentOriginalQuota > common.MaxQuota ||
				parent.CurrentChargedQuota < 0 || parent.CurrentChargedQuota > common.MaxQuota ||
				parent.LastCursorRevision <= 0 || usage.TailSettlementID <= 0 {
				return ErrGroupModelDiscountAggregateCorrupt
			}
			if usage.Id != parent.UsageID || usage.PolicyHash != parent.PolicyHash ||
				usage.PolicySnapshot != parent.PolicySnapshot || usage.ProgressBasis != parent.ProgressBasis {
				return ErrGroupModelDiscountAggregateCorrupt
			}
			if unresolvedErr := checkGroupModelDiscountScopeUnresolved(tx, usage.Id); unresolvedErr != nil {
				return unresolvedErr
			}

			var snapshot groupdiscount.Snapshot
			if decodeErr := common.UnmarshalJsonStr(parent.PolicySnapshot, &snapshot); decodeErr != nil {
				return fmt.Errorf("decode group model discount policy snapshot: %w", decodeErr)
			}
			if groupdiscount.NormalizeProgressBasis(snapshot.ProgressBasis) != parent.ProgressBasis {
				return ErrGroupModelDiscountAggregateCorrupt
			}
			previousOriginal := parent.CurrentOriginalQuota
			previousCharged := parent.CurrentChargedQuota
			previousProgress, parseErr := groupdiscount.ParseProgressQuota(parent.CurrentProgressQuota)
			if parseErr != nil {
				return ErrGroupModelDiscountAggregateCorrupt
			}
			usageProgress, parseErr := groupdiscount.ParseProgressQuota(usage.ProgressQuota)
			if parseErr != nil {
				return ErrGroupModelDiscountAggregateCorrupt
			}
			newOriginal := int64(input.NewOriginalQuota)
			deltaOriginal := newOriginal - previousOriginal
			deltaCharged := int64(0)
			deltaProgress := decimal.Zero
			segments := make([]groupdiscount.Segment, 0)

			switch {
			case deltaOriginal > 0:
				calculation, calculateErr := groupdiscount.CalculateWithProgress(
					snapshot,
					usage.OriginalQuota,
					usage.ProgressQuota,
					int(deltaOriginal),
				)
				if calculateErr != nil {
					return calculateErr
				}
				deltaCharged = int64(calculation.ChargedQuota)
				deltaProgress, parseErr = decimal.NewFromString(calculation.ProgressQuota)
				if parseErr != nil {
					return ErrGroupModelDiscountAggregateCorrupt
				}
				segments = calculation.Segments
			case deltaOriginal < 0:
				removeOriginal := -deltaOriginal
				if usage.OriginalQuota < removeOriginal {
					return ErrGroupModelDiscountAggregateCorrupt
				}
				calculation, calculateErr := groupdiscount.CalculateDecrease(
					snapshot,
					usage.OriginalQuota,
					usage.ProgressQuota,
					int(removeOriginal),
				)
				if calculateErr != nil {
					return calculateErr
				}
				removedCharged := -int64(calculation.ChargedQuota)
				if previousCharged < removedCharged {
					return ErrGroupModelDiscountAdjustmentChargeUnderflow
				}
				deltaCharged = int64(calculation.ChargedQuota)
				deltaProgress, parseErr = decimal.NewFromString(calculation.ProgressQuota)
				if parseErr != nil {
					return ErrGroupModelDiscountAggregateCorrupt
				}
				segments = calculation.Segments
			}

			if deltaOriginal > 0 && usage.OriginalQuota > math.MaxInt64-deltaOriginal {
				return groupdiscount.ErrMonthlyQuotaOverflow
			}
			if deltaCharged > 0 && usage.ChargedQuota > math.MaxInt64-deltaCharged {
				return groupdiscount.ErrMonthlyQuotaOverflow
			}
			usageOriginalAfter := usage.OriginalQuota + deltaOriginal
			usageChargedAfter := usage.ChargedQuota + deltaCharged
			usageProgressAfter := usageProgress.Add(deltaProgress)
			newCharged := previousCharged + deltaCharged
			newProgress := previousProgress.Add(deltaProgress)
			if usageOriginalAfter < 0 || usageChargedAfter < 0 || newCharged < 0 || newCharged > common.MaxQuota ||
				usageProgressAfter.IsNegative() || newProgress.IsNegative() {
				return ErrGroupModelDiscountAggregateCorrupt
			}
			segmentsJSON, marshalErr := common.Marshal(segments)
			if marshalErr != nil {
				return marshalErr
			}

			updatedUsage := tx.Model(&UserGroupModelMonthlyUsage{}).
				Where("id = ? AND revision = ?", usage.Id, usage.Revision).
				Updates(map[string]any{
					"original_quota":     usageOriginalAfter,
					"charged_quota":      usageChargedAfter,
					"progress_quota":     usageProgressAfter.String(),
					"tail_settlement_id": parent.Id,
					"revision":           usage.Revision + 1,
				})
			if updatedUsage.Error != nil {
				return updatedUsage.Error
			}
			if updatedUsage.RowsAffected != 1 {
				return errGroupModelDiscountRevisionConflict
			}

			parentRevisionAfter := parent.AdjustmentRevision + 1
			updatedParent := tx.Model(&GroupModelDiscountSettlement{}).
				Where("id = ? AND revision = ? AND adjustment_revision = ? AND status = ? AND current_original_quota = ? AND current_charged_quota = ? AND current_progress_quota = ?",
					parent.Id, parent.Revision, parent.AdjustmentRevision, GroupModelDiscountStatusSettled,
					parent.CurrentOriginalQuota, parent.CurrentChargedQuota, parent.CurrentProgressQuota).
				Updates(map[string]any{
					"current_original_quota": newOriginal,
					"current_charged_quota":  newCharged,
					"current_progress_quota": newProgress.String(),
					"adjustment_revision":    parentRevisionAfter,
					"last_cursor_revision":   usage.Revision + 1,
				})
			if updatedParent.Error != nil {
				return updatedParent.Error
			}
			if updatedParent.RowsAffected != 1 {
				return errGroupModelDiscountRevisionConflict
			}

			adjustment := GroupModelDiscountAdjustment{
				AdjustmentID:                   input.AdjustmentID,
				Fingerprint:                    fingerprint,
				SettlementID:                   parent.Id,
				SettlementRequestID:            parent.RequestID,
				UsageID:                        usage.Id,
				UserID:                         parent.UserID,
				ChannelID:                      parent.AccountingChannelID,
				PolicyHash:                     parent.PolicyHash,
				ProgressBasis:                  parent.ProgressBasis,
				PreviousOriginalQuota:          previousOriginal,
				PreviousChargedQuota:           previousCharged,
				PreviousProgressQuota:          previousProgress.String(),
				NewOriginalQuota:               newOriginal,
				NewChargedQuota:                newCharged,
				NewProgressQuota:               newProgress.String(),
				DeltaOriginalQuota:             deltaOriginal,
				DeltaChargedQuota:              deltaCharged,
				DeltaProgressQuota:             deltaProgress.String(),
				UsageOriginalBefore:            usage.OriginalQuota,
				UsageOriginalAfter:             usageOriginalAfter,
				UsageChargedBefore:             usage.ChargedQuota,
				UsageChargedAfter:              usageChargedAfter,
				UsageProgressBefore:            usageProgress.String(),
				UsageProgressAfter:             usageProgressAfter.String(),
				PreviousTailSettlementID:       usage.TailSettlementID,
				ParentAdjustmentRevisionBefore: parent.AdjustmentRevision,
				ParentAdjustmentRevisionAfter:  parentRevisionAfter,
				ParentLastCursorRevisionBefore: parent.LastCursorRevision,
				ParentLastCursorRevisionAfter:  usage.Revision + 1,
				Segments:                       string(segmentsJSON),
				Status:                         GroupModelDiscountStatusReserved,
				Revision:                       1,
			}
			if createErr := tx.Create(&adjustment).Error; createErr != nil {
				return createErr
			}
			reservation = newGroupModelDiscountAdjustmentReservation(adjustment, segments, false)
			return nil
		})
		if err == nil {
			return reservation, nil
		}
		if existing, getErr := getGroupModelDiscountAdjustment(db, input.AdjustmentID); getErr == nil {
			return reservationFromGroupModelDiscountAdjustment(existing, fingerprint, true)
		}
		if !isRetryableGroupModelDiscountError(err) {
			return GroupModelDiscountAdjustmentReservation{}, err
		}
		time.Sleep(groupModelDiscountRetryDelay(attempt))
	}
	if errors.Is(err, ErrGroupModelDiscountScopeBusy) {
		return GroupModelDiscountAdjustmentReservation{}, fmt.Errorf("%w after %d attempts", ErrGroupModelDiscountScopeBusy, groupModelDiscountMaxAttempts)
	}
	return GroupModelDiscountAdjustmentReservation{}, fmt.Errorf("%w: %w", ErrGroupModelDiscountContention, err)
}

func CommitGroupModelDiscountAdjustment(adjustmentID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return commitGroupModelDiscountAdjustment(DB, adjustmentID)
}

func commitGroupModelDiscountAdjustment(db *gorm.DB, adjustmentID string) error {
	return commitGroupModelDiscountAdjustmentAccounting(db, adjustmentID, nil)
}

// CommitGroupModelDiscountAdjustmentWithUsage advances an adjustment and its
// quota-only user/channel delta in one primary-database transaction.
func CommitGroupModelDiscountAdjustmentWithUsage(adjustmentID string, delta BillingUsageDelta) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return commitGroupModelDiscountAdjustmentWithUsage(DB, adjustmentID, delta)
}

func commitGroupModelDiscountAdjustmentWithUsage(db *gorm.DB, adjustmentID string, delta BillingUsageDelta) error {
	return commitGroupModelDiscountAdjustmentAccounting(db, adjustmentID, &delta)
}

func commitGroupModelDiscountAdjustmentAccounting(db *gorm.DB, adjustmentID string, accounting *BillingUsageDelta) error {
	return transitionGroupModelDiscountAdjustment(db, adjustmentID, func(tx *gorm.DB, adjustment GroupModelDiscountAdjustment) error {
		switch adjustment.Status {
		case GroupModelDiscountStatusSettled:
			if accounting != nil {
				return validateGroupModelDiscountAdjustmentAccountingEvidence(adjustment, *accounting)
			}
			return nil
		case GroupModelDiscountStatusReserved:
			// Continue below.
		case GroupModelDiscountStatusPendingReconcile:
			if adjustment.PendingAction != GroupModelDiscountPendingActionCommitAfterFunding {
				return fmt.Errorf("%w: adjustment %s -> %s", ErrGroupModelDiscountInvalidTransition, adjustment.Status, GroupModelDiscountStatusSettled)
			}
		default:
			return fmt.Errorf("%w: adjustment %s -> %s", ErrGroupModelDiscountInvalidTransition, adjustment.Status, GroupModelDiscountStatusSettled)
		}
		if accounting == nil {
			return updateGroupModelDiscountAdjustmentStatusCAS(tx, adjustment, GroupModelDiscountStatusSettled)
		}
		if err := validateGroupModelDiscountAdjustmentAccountingDelta(adjustment, *accounting); err != nil {
			return err
		}
		if err := applyGroupModelDiscountUsageDelta(tx, *accounting); err != nil {
			return err
		}
		return updateGroupModelDiscountAdjustmentStatusWithAccountingCAS(tx, adjustment, *accounting)
	})
}

// MarkGroupModelDiscountAdjustmentPendingReconcile records that the funding
// delta outcome must be reconciled. PendingAction durably records the only
// automatic ledger transition that may be retried; unknown/manual requires
// explicit reconciliation.
func MarkGroupModelDiscountAdjustmentPendingReconcile(adjustmentID, pendingAction string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return markGroupModelDiscountAdjustmentPendingReconcile(DB, adjustmentID, pendingAction)
}

func markGroupModelDiscountAdjustmentPendingReconcile(db *gorm.DB, adjustmentID, pendingAction string) error {
	if pendingAction == GroupModelDiscountPendingActionReverseAfterRefund || !isGroupModelDiscountPendingAction(pendingAction) {
		return ErrGroupModelDiscountInvalidInput
	}
	return transitionGroupModelDiscountAdjustment(db, adjustmentID, func(tx *gorm.DB, adjustment GroupModelDiscountAdjustment) error {
		switch adjustment.Status {
		case GroupModelDiscountStatusPendingReconcile:
			if adjustment.PendingAction == pendingAction {
				return nil
			}
		case GroupModelDiscountStatusReserved:
			return updateGroupModelDiscountAdjustmentPendingCAS(tx, adjustment, pendingAction)
		}
		return fmt.Errorf("%w: adjustment %s/%s -> %s/%s",
			ErrGroupModelDiscountInvalidTransition,
			adjustment.Status,
			adjustment.PendingAction,
			GroupModelDiscountStatusPendingReconcile,
			pendingAction,
		)
	})
}

func RollbackGroupModelDiscountAdjustment(adjustmentID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return rollbackGroupModelDiscountAdjustment(DB, adjustmentID)
}

func rollbackGroupModelDiscountAdjustment(db *gorm.DB, adjustmentID string) error {
	if strings.TrimSpace(adjustmentID) == "" || len(adjustmentID) > maxDiscountRequestIDLength {
		return ErrGroupModelDiscountInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < groupModelDiscountMaxAttempts; attempt++ {
		var hint GroupModelDiscountAdjustment
		if lastErr = db.Where("adjustment_id = ?", adjustmentID).First(&hint).Error; lastErr != nil {
			return lastErr
		}
		lastErr = db.Transaction(func(tx *gorm.DB) error {
			var usage UserGroupModelMonthlyUsage
			if err := lockForUpdate(tx).Where("id = ?", hint.UsageID).First(&usage).Error; err != nil {
				return err
			}
			var adjustment GroupModelDiscountAdjustment
			if err := lockForUpdate(tx).Where("adjustment_id = ?", adjustmentID).First(&adjustment).Error; err != nil {
				return err
			}
			if adjustment.Status == GroupModelDiscountStatusReversed {
				return nil
			}
			validStatus := adjustment.Status == GroupModelDiscountStatusReserved ||
				(adjustment.Status == GroupModelDiscountStatusPendingReconcile &&
					adjustment.PendingAction == GroupModelDiscountPendingActionRollbackUnfunded)
			if !validStatus {
				return fmt.Errorf("%w: adjustment %s -> %s", ErrGroupModelDiscountInvalidTransition, adjustment.Status, GroupModelDiscountStatusReversed)
			}
			if adjustment.UsageID != usage.Id || usage.OriginalQuota != adjustment.UsageOriginalAfter ||
				usage.ChargedQuota != adjustment.UsageChargedAfter || usage.ProgressQuota != adjustment.UsageProgressAfter {
				return ErrGroupModelDiscountRequiresReconcile
			}
			if usage.TailSettlementID != adjustment.SettlementID {
				return ErrGroupModelDiscountRequiresReconcile
			}
			var parent GroupModelDiscountSettlement
			if err := lockForUpdate(tx).Where("id = ?", adjustment.SettlementID).First(&parent).Error; err != nil {
				return err
			}
			if parent.Status != GroupModelDiscountStatusSettled ||
				parent.ProgressBasis != adjustment.ProgressBasis ||
				parent.CurrentOriginalQuota != adjustment.NewOriginalQuota ||
				parent.CurrentChargedQuota != adjustment.NewChargedQuota ||
				parent.CurrentProgressQuota != adjustment.NewProgressQuota ||
				parent.AdjustmentRevision != adjustment.ParentAdjustmentRevisionAfter ||
				parent.LastCursorRevision != adjustment.ParentLastCursorRevisionAfter {
				return ErrGroupModelDiscountRequiresReconcile
			}

			updatedUsage := tx.Model(&UserGroupModelMonthlyUsage{}).
				Where("id = ? AND revision = ?", usage.Id, usage.Revision).
				Updates(map[string]any{
					"original_quota":     adjustment.UsageOriginalBefore,
					"charged_quota":      adjustment.UsageChargedBefore,
					"progress_quota":     adjustment.UsageProgressBefore,
					"tail_settlement_id": adjustment.PreviousTailSettlementID,
					"revision":           usage.Revision + 1,
				})
			if updatedUsage.Error != nil {
				return updatedUsage.Error
			}
			if updatedUsage.RowsAffected != 1 {
				return errGroupModelDiscountRevisionConflict
			}
			updatedParent := tx.Model(&GroupModelDiscountSettlement{}).
				Where("id = ? AND revision = ? AND adjustment_revision = ? AND status = ?",
					parent.Id, parent.Revision, parent.AdjustmentRevision, GroupModelDiscountStatusSettled).
				Updates(map[string]any{
					"current_original_quota": adjustment.PreviousOriginalQuota,
					"current_charged_quota":  adjustment.PreviousChargedQuota,
					"current_progress_quota": adjustment.PreviousProgressQuota,
					"adjustment_revision":    parent.AdjustmentRevision + 1,
					"last_cursor_revision":   adjustment.ParentLastCursorRevisionBefore,
				})
			if updatedParent.Error != nil {
				return updatedParent.Error
			}
			if updatedParent.RowsAffected != 1 {
				return errGroupModelDiscountRevisionConflict
			}
			return updateGroupModelDiscountAdjustmentStatusCAS(tx, adjustment, GroupModelDiscountStatusReversed)
		})
		if lastErr == nil {
			return nil
		}
		if !isRetryableGroupModelDiscountError(lastErr) {
			return lastErr
		}
		time.Sleep(groupModelDiscountRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrGroupModelDiscountContention, lastErr)
}

func GetGroupModelDiscountAdjustment(adjustmentID string) (GroupModelDiscountAdjustment, error) {
	if DB == nil {
		return GroupModelDiscountAdjustment{}, errors.New("database is not initialized")
	}
	return getGroupModelDiscountAdjustment(DB, adjustmentID)
}

func getGroupModelDiscountAdjustment(db *gorm.DB, adjustmentID string) (GroupModelDiscountAdjustment, error) {
	var adjustment GroupModelDiscountAdjustment
	err := db.Where("adjustment_id = ?", adjustmentID).First(&adjustment).Error
	return adjustment, err
}

func checkGroupModelDiscountScopeUnresolved(tx *gorm.DB, usageID int64) error {
	var settlementStatuses []string
	if err := tx.Model(&GroupModelDiscountSettlement{}).Select("status").
		Where("usage_id = ? AND status IN ?", usageID, []string{
			GroupModelDiscountStatusReserved,
			GroupModelDiscountStatusPendingReconcile,
		}).Find(&settlementStatuses).Error; err != nil {
		return err
	}
	adjustmentStatuses, err := groupModelDiscountAdjustmentUnresolvedStatuses(tx, usageID)
	if err != nil {
		return err
	}
	for _, status := range append(settlementStatuses, adjustmentStatuses...) {
		if status == GroupModelDiscountStatusPendingReconcile {
			return ErrGroupModelDiscountPendingReconcile
		}
	}
	if len(settlementStatuses)+len(adjustmentStatuses) > 0 {
		return ErrGroupModelDiscountScopeBusy
	}
	return nil
}

func checkGroupModelDiscountAdjustmentUnresolved(tx *gorm.DB, usageID int64) error {
	statuses, err := groupModelDiscountAdjustmentUnresolvedStatuses(tx, usageID)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		if status == GroupModelDiscountStatusPendingReconcile {
			return ErrGroupModelDiscountPendingReconcile
		}
	}
	if len(statuses) > 0 {
		return ErrGroupModelDiscountScopeBusy
	}
	return nil
}

func groupModelDiscountAdjustmentUnresolvedStatuses(tx *gorm.DB, usageID int64) ([]string, error) {
	var statuses []string
	err := tx.Model(&GroupModelDiscountAdjustment{}).Select("status").
		Where("usage_id = ? AND status IN ?", usageID, []string{
			GroupModelDiscountStatusReserved,
			GroupModelDiscountStatusPendingReconcile,
		}).Find(&statuses).Error
	return statuses, err
}

func validateAndFingerprintGroupModelDiscountAdjustment(input GroupModelDiscountAdjustmentInput) (string, error) {
	if strings.TrimSpace(input.AdjustmentID) == "" || len(input.AdjustmentID) > maxDiscountRequestIDLength ||
		strings.TrimSpace(input.SettlementRequestID) == "" || len(input.SettlementRequestID) > maxDiscountRequestIDLength ||
		input.NewOriginalQuota < 0 || input.NewOriginalQuota > common.MaxQuota {
		return "", ErrGroupModelDiscountInvalidInput
	}
	if input.NewOriginalQuota == 0 {
		return "", ErrGroupModelDiscountAdjustmentRequiresFullReverse
	}
	payloadJSON, err := common.Marshal(groupModelDiscountAdjustmentFingerprint{
		SettlementRequestID: input.SettlementRequestID,
		NewOriginalQuota:    input.NewOriginalQuota,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payloadJSON)
	return hex.EncodeToString(digest[:]), nil
}

func reservationFromGroupModelDiscountAdjustment(adjustment GroupModelDiscountAdjustment, fingerprint string, reused bool) (GroupModelDiscountAdjustmentReservation, error) {
	if adjustment.Fingerprint != fingerprint {
		return GroupModelDiscountAdjustmentReservation{}, ErrGroupModelDiscountAdjustmentConflict
	}
	if adjustment.UserID <= 0 || adjustment.ChannelID < 0 ||
		adjustment.PreviousTailSettlementID <= 0 ||
		adjustment.ParentLastCursorRevisionBefore <= 0 ||
		adjustment.ParentLastCursorRevisionAfter <= adjustment.ParentLastCursorRevisionBefore {
		return GroupModelDiscountAdjustmentReservation{}, ErrGroupModelDiscountAggregateCorrupt
	}
	values := []int64{
		adjustment.PreviousOriginalQuota,
		adjustment.PreviousChargedQuota,
		adjustment.NewOriginalQuota,
		adjustment.NewChargedQuota,
	}
	for _, value := range values {
		if value < 0 || value > common.MaxQuota {
			return GroupModelDiscountAdjustmentReservation{}, ErrGroupModelDiscountAggregateCorrupt
		}
	}
	if adjustment.DeltaOriginalQuota < -common.MaxQuota || adjustment.DeltaOriginalQuota > common.MaxQuota ||
		adjustment.DeltaChargedQuota < -common.MaxQuota || adjustment.DeltaChargedQuota > common.MaxQuota {
		return GroupModelDiscountAdjustmentReservation{}, ErrGroupModelDiscountAggregateCorrupt
	}
	for _, progress := range []string{
		adjustment.PreviousProgressQuota,
		adjustment.NewProgressQuota,
		adjustment.UsageProgressBefore,
		adjustment.UsageProgressAfter,
	} {
		if _, err := groupdiscount.ParseProgressQuota(progress); err != nil {
			return GroupModelDiscountAdjustmentReservation{}, ErrGroupModelDiscountAggregateCorrupt
		}
	}
	if _, err := groupdiscount.ParseProgressDelta(adjustment.DeltaProgressQuota); err != nil {
		return GroupModelDiscountAdjustmentReservation{}, ErrGroupModelDiscountAggregateCorrupt
	}
	var segments []groupdiscount.Segment
	if err := common.UnmarshalJsonStr(adjustment.Segments, &segments); err != nil {
		return GroupModelDiscountAdjustmentReservation{}, fmt.Errorf("decode group model discount adjustment segments: %w", err)
	}
	return newGroupModelDiscountAdjustmentReservation(adjustment, segments, reused), nil
}

func newGroupModelDiscountAdjustmentReservation(adjustment GroupModelDiscountAdjustment, segments []groupdiscount.Segment, reused bool) GroupModelDiscountAdjustmentReservation {
	return GroupModelDiscountAdjustmentReservation{
		Adjustment:            adjustment,
		PreviousOriginalQuota: int(adjustment.PreviousOriginalQuota),
		PreviousChargedQuota:  int(adjustment.PreviousChargedQuota),
		NewOriginalQuota:      int(adjustment.NewOriginalQuota),
		NewChargedQuota:       int(adjustment.NewChargedQuota),
		DeltaOriginalQuota:    int(adjustment.DeltaOriginalQuota),
		DeltaChargedQuota:     int(adjustment.DeltaChargedQuota),
		Segments:              append([]groupdiscount.Segment(nil), segments...),
		Reused:                reused,
	}
}

func transitionGroupModelDiscountAdjustment(
	db *gorm.DB,
	adjustmentID string,
	transition func(*gorm.DB, GroupModelDiscountAdjustment) error,
) error {
	if strings.TrimSpace(adjustmentID) == "" || len(adjustmentID) > maxDiscountRequestIDLength {
		return ErrGroupModelDiscountInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < groupModelDiscountMaxAttempts; attempt++ {
		lastErr = db.Transaction(func(tx *gorm.DB) error {
			var adjustment GroupModelDiscountAdjustment
			if err := lockForUpdate(tx).Where("adjustment_id = ?", adjustmentID).First(&adjustment).Error; err != nil {
				return err
			}
			return transition(tx, adjustment)
		})
		if lastErr == nil {
			return nil
		}
		if !isRetryableGroupModelDiscountError(lastErr) {
			return lastErr
		}
		time.Sleep(groupModelDiscountRetryDelay(attempt))
	}
	return fmt.Errorf("%w: %w", ErrGroupModelDiscountContention, lastErr)
}

func updateGroupModelDiscountAdjustmentStatusCAS(tx *gorm.DB, adjustment GroupModelDiscountAdjustment, nextStatus string) error {
	result := tx.Model(&GroupModelDiscountAdjustment{}).
		Where("id = ? AND revision = ? AND status = ?", adjustment.Id, adjustment.Revision, adjustment.Status).
		Updates(map[string]any{
			"status":         nextStatus,
			"pending_action": "",
			"revision":       adjustment.Revision + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errGroupModelDiscountRevisionConflict
	}
	return nil
}

func validateGroupModelDiscountAdjustmentAccountingDelta(
	adjustment GroupModelDiscountAdjustment,
	delta BillingUsageDelta,
) error {
	if adjustment.DeltaChargedQuota < -common.MaxQuota || adjustment.DeltaChargedQuota > common.MaxQuota ||
		adjustment.UserID <= 0 || adjustment.ChannelID <= 0 ||
		delta.UserID != adjustment.UserID || delta.ChannelID != adjustment.ChannelID ||
		delta.QuotaDelta != int(adjustment.DeltaChargedQuota) || delta.RequestCountDelta != 0 {
		return ErrGroupModelDiscountAccountingConflict
	}
	return nil
}

func validateGroupModelDiscountAdjustmentAccountingEvidence(
	adjustment GroupModelDiscountAdjustment,
	delta BillingUsageDelta,
) error {
	if !adjustment.AccountingApplied ||
		adjustment.AccountingUserID != delta.UserID ||
		adjustment.AccountingChannelID != delta.ChannelID ||
		adjustment.AccountingQuotaDelta != delta.QuotaDelta ||
		adjustment.AccountingRequestCountDelta != delta.RequestCountDelta {
		return ErrGroupModelDiscountAccountingConflict
	}
	return validateGroupModelDiscountAdjustmentAccountingDelta(adjustment, delta)
}

func updateGroupModelDiscountAdjustmentStatusWithAccountingCAS(
	tx *gorm.DB,
	adjustment GroupModelDiscountAdjustment,
	delta BillingUsageDelta,
) error {
	result := tx.Model(&GroupModelDiscountAdjustment{}).
		Where("id = ? AND revision = ? AND status = ?", adjustment.Id, adjustment.Revision, adjustment.Status).
		Updates(map[string]any{
			"status":                         GroupModelDiscountStatusSettled,
			"pending_action":                 "",
			"accounting_applied":             true,
			"accounting_user_id":             delta.UserID,
			"accounting_channel_id":          delta.ChannelID,
			"accounting_quota_delta":         delta.QuotaDelta,
			"accounting_request_count_delta": delta.RequestCountDelta,
			"revision":                       adjustment.Revision + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errGroupModelDiscountRevisionConflict
	}
	return nil
}

func updateGroupModelDiscountAdjustmentPendingCAS(tx *gorm.DB, adjustment GroupModelDiscountAdjustment, pendingAction string) error {
	result := tx.Model(&GroupModelDiscountAdjustment{}).
		Where("id = ? AND revision = ? AND status = ? AND pending_action = ?",
			adjustment.Id, adjustment.Revision, adjustment.Status, adjustment.PendingAction).
		Updates(map[string]any{
			"status":         GroupModelDiscountStatusPendingReconcile,
			"pending_action": pendingAction,
			"revision":       adjustment.Revision + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errGroupModelDiscountRevisionConflict
	}
	return nil
}
