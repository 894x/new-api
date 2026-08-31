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
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	GroupModelDiscountStatusReserved         = "reserved"
	GroupModelDiscountStatusSettled          = "settled"
	GroupModelDiscountStatusReversed         = "reversed"
	GroupModelDiscountStatusPendingReconcile = "pending_reconcile"

	GroupModelDiscountPendingActionUnknownManual      = "unknown_manual"
	GroupModelDiscountPendingActionCommitAfterFunding = "commit_after_funding"
	GroupModelDiscountPendingActionReverseAfterRefund = "reverse_after_refund"
	GroupModelDiscountPendingActionRollbackUnfunded   = "rollback_unfunded"

	groupModelDiscountMaxAttempts = 64
	maxDiscountRequestIDLength    = 191
)

var (
	ErrGroupModelDiscountInvalidInput       = errors.New("invalid group model discount input")
	ErrGroupModelDiscountRequestConflict    = errors.New("group model discount request id conflicts with a different settlement")
	ErrGroupModelDiscountPolicyConflict     = errors.New("group model discount policy changed inside an existing period")
	ErrGroupModelDiscountScopeBusy          = errors.New("group model discount scope has an unresolved reservation")
	ErrGroupModelDiscountPendingReconcile   = errors.New("group model discount scope is blocked by pending reconciliation")
	ErrGroupModelDiscountInvalidTransition  = errors.New("invalid group model discount settlement transition")
	ErrGroupModelDiscountAggregateCorrupt   = errors.New("group model discount aggregate is inconsistent")
	ErrGroupModelDiscountNonTailReverse     = errors.New("group model discount settlement is not the active ledger tail")
	ErrGroupModelDiscountRequiresReconcile  = errors.New("group model discount cursor cannot be safely restored and requires reconciliation")
	ErrGroupModelDiscountAccountingConflict = errors.New("group model discount accounting evidence conflicts with requested usage delta")
	ErrGroupModelDiscountContention         = errors.New("group model discount settlement contention exceeded retry limit")
	errGroupModelDiscountRevisionConflict   = errors.New("group model discount revision conflict")
)

// UserGroupModelMonthlyUsage owns the exact pricing-progress cursor for one
// user + using group + origin model + policy period. ProgressBasis determines
// whether ProgressQuota tracks original quota or exact discounted quota;
// ChargedQuota remains the integer reconciliation total actually funded.
type UserGroupModelMonthlyUsage struct {
	Id               int64                       `json:"id" gorm:"primaryKey"`
	ScopeHash        string                      `json:"scope_hash" gorm:"type:char(64);not null;uniqueIndex:idx_user_group_model_period_scope"`
	UserID           int                         `json:"user_id" gorm:"not null"`
	UsingGroup       string                      `json:"using_group" gorm:"type:varchar(128);not null"`
	OriginModel      string                      `json:"origin_model" gorm:"type:varchar(255);not null"`
	PeriodStart      int64                       `json:"period_start" gorm:"not null"`
	PeriodEnd        int64                       `json:"period_end" gorm:"not null"`
	PolicyHash       string                      `json:"policy_hash" gorm:"type:varchar(128);not null"`
	PolicySnapshot   string                      `json:"policy_snapshot" gorm:"type:text;not null"`
	ProgressBasis    groupdiscount.ProgressBasis `json:"progress_basis" gorm:"type:varchar(16);not null"`
	OriginalQuota    int64                       `json:"original_quota" gorm:"not null"`
	ChargedQuota     int64                       `json:"charged_quota" gorm:"not null"`
	ProgressQuota    string                      `json:"progress_quota" gorm:"type:text;not null"`
	TailSettlementID int64                       `json:"tail_settlement_id" gorm:"not null"`
	Revision         int64                       `json:"revision" gorm:"not null"`
	CreatedAt        time.Time                   `json:"created_at"`
	UpdatedAt        time.Time                   `json:"updated_at"`
}

func (UserGroupModelMonthlyUsage) TableName() string {
	return "user_group_model_monthly_usages"
}

// GroupModelDiscountSettlement makes request settlement idempotent and keeps
// the exact frozen policy/segments needed for audit and reconciliation.
type GroupModelDiscountSettlement struct {
	Id                                 int64                       `json:"id" gorm:"primaryKey"`
	RequestID                          string                      `json:"request_id" gorm:"type:varchar(191);not null;uniqueIndex"`
	Fingerprint                        string                      `json:"fingerprint" gorm:"type:char(64);not null"`
	UsageID                            int64                       `json:"usage_id" gorm:"not null;index:idx_discount_usage_status,priority:1"`
	UserID                             int                         `json:"user_id" gorm:"not null;index"`
	UsingGroup                         string                      `json:"using_group" gorm:"type:varchar(128);not null"`
	OriginModel                        string                      `json:"origin_model" gorm:"type:varchar(255);not null"`
	PeriodStart                        int64                       `json:"period_start" gorm:"not null;index"`
	PeriodEnd                          int64                       `json:"period_end" gorm:"not null"`
	PolicyHash                         string                      `json:"policy_hash" gorm:"type:varchar(128);not null"`
	PolicySnapshot                     string                      `json:"policy_snapshot" gorm:"type:text;not null"`
	ProgressBasis                      groupdiscount.ProgressBasis `json:"progress_basis" gorm:"type:varchar(16);not null"`
	MonthlyOriginalBefore              int64                       `json:"monthly_original_before" gorm:"not null"`
	MonthlyOriginalAfter               int64                       `json:"monthly_original_after" gorm:"not null"`
	MonthlyProgressBefore              string                      `json:"monthly_progress_before" gorm:"type:text;not null"`
	MonthlyProgressAfter               string                      `json:"monthly_progress_after" gorm:"type:text;not null"`
	OriginalQuota                      int64                       `json:"original_quota" gorm:"not null"`
	ChargedQuota                       int64                       `json:"charged_quota" gorm:"not null"`
	ProgressQuota                      string                      `json:"progress_quota" gorm:"type:text;not null"`
	CurrentOriginalQuota               int64                       `json:"current_original_quota" gorm:"not null"`
	CurrentChargedQuota                int64                       `json:"current_charged_quota" gorm:"not null"`
	CurrentProgressQuota               string                      `json:"current_progress_quota" gorm:"type:text;not null"`
	AdjustmentRevision                 int64                       `json:"adjustment_revision" gorm:"not null"`
	LastCursorRevision                 int64                       `json:"last_cursor_revision" gorm:"not null"`
	Segments                           string                      `json:"segments" gorm:"type:text;not null"`
	Status                             string                      `json:"status" gorm:"type:varchar(32);not null;index:idx_discount_usage_status,priority:2"`
	PendingAction                      string                      `json:"pending_action" gorm:"type:varchar(32);not null"`
	AccountingApplied                  bool                        `json:"accounting_applied" gorm:"not null"`
	AccountingUserID                   int                         `json:"accounting_user_id" gorm:"not null"`
	AccountingChannelID                int                         `json:"accounting_channel_id" gorm:"not null"`
	AccountingQuotaDelta               int                         `json:"accounting_quota_delta" gorm:"not null"`
	AccountingRequestCountDelta        int                         `json:"accounting_request_count_delta" gorm:"not null"`
	ReverseAccountingApplied           bool                        `json:"reverse_accounting_applied" gorm:"not null"`
	ReverseAccountingUserID            int                         `json:"reverse_accounting_user_id" gorm:"not null"`
	ReverseAccountingChannelID         int                         `json:"reverse_accounting_channel_id" gorm:"not null"`
	ReverseAccountingQuotaDelta        int                         `json:"reverse_accounting_quota_delta" gorm:"not null"`
	ReverseAccountingRequestCountDelta int                         `json:"reverse_accounting_request_count_delta" gorm:"not null"`
	Revision                           int64                       `json:"revision" gorm:"not null"`
	CreatedAt                          time.Time                   `json:"created_at"`
	UpdatedAt                          time.Time                   `json:"updated_at"`
}

func (GroupModelDiscountSettlement) TableName() string {
	return "group_model_discount_settlements"
}

type GroupModelDiscountReserveInput struct {
	RequestID     string
	UserID        int
	UsingGroup    string
	OriginModel   string
	Snapshot      groupdiscount.Snapshot
	OriginalQuota int
}

type GroupModelDiscountReservation struct {
	Settlement  GroupModelDiscountSettlement
	Calculation groupdiscount.Calculation
	Reused      bool
}

type groupModelDiscountFingerprintPayload struct {
	UserID        int                    `json:"user_id"`
	UsingGroup    string                 `json:"using_group"`
	OriginModel   string                 `json:"origin_model"`
	Snapshot      groupdiscount.Snapshot `json:"snapshot"`
	OriginalQuota int                    `json:"original_quota"`
}

type groupModelDiscountScopeIdentity struct {
	UserID      int    `json:"user_id"`
	UsingGroup  string `json:"using_group"`
	OriginModel string `json:"origin_model"`
	PeriodStart int64  `json:"period_start"`
}

func ReserveGroupModelDiscount(input GroupModelDiscountReserveInput) (GroupModelDiscountReservation, error) {
	if DB == nil {
		return GroupModelDiscountReservation{}, errors.New("database is not initialized")
	}
	return reserveGroupModelDiscount(DB, input)
}

func reserveGroupModelDiscount(db *gorm.DB, input GroupModelDiscountReserveInput) (GroupModelDiscountReservation, error) {
	fingerprint, snapshotJSON, err := validateAndFingerprintGroupModelDiscountInput(input)
	if err != nil {
		return GroupModelDiscountReservation{}, err
	}
	scopeHash, err := groupModelDiscountScopeHash(input.UserID, input.UsingGroup, input.OriginModel, input.Snapshot.PeriodStart)
	if err != nil {
		return GroupModelDiscountReservation{}, err
	}

	for attempt := 0; attempt < groupModelDiscountMaxAttempts; attempt++ {
		var reservation GroupModelDiscountReservation
		err = db.Transaction(func(tx *gorm.DB) error {
			var existing GroupModelDiscountSettlement
			existingErr := lockForUpdate(tx).Where("request_id = ?", input.RequestID).First(&existing).Error
			if existingErr == nil {
				loaded, loadErr := reservationFromSettlement(existing, fingerprint, true)
				if loadErr != nil {
					return loadErr
				}
				reservation = loaded
				return nil
			}
			if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
				return existingErr
			}

			usageSeed := UserGroupModelMonthlyUsage{
				ScopeHash:      scopeHash,
				UserID:         input.UserID,
				UsingGroup:     input.UsingGroup,
				OriginModel:    input.OriginModel,
				PeriodStart:    input.Snapshot.PeriodStart,
				PeriodEnd:      input.Snapshot.PeriodEnd,
				PolicyHash:     input.Snapshot.PolicyHash,
				PolicySnapshot: snapshotJSON,
				ProgressBasis:  groupdiscount.NormalizeProgressBasis(input.Snapshot.ProgressBasis),
				ProgressQuota:  "0",
				Revision:       1,
			}
			if createErr := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "scope_hash"},
				},
				DoNothing: true,
			}).Create(&usageSeed).Error; createErr != nil {
				return createErr
			}

			var usage UserGroupModelMonthlyUsage
			if queryErr := lockForUpdate(tx).
				Where("scope_hash = ?", scopeHash).
				First(&usage).Error; queryErr != nil {
				return queryErr
			}
			if usage.ScopeHash != scopeHash || usage.UserID != input.UserID ||
				usage.UsingGroup != input.UsingGroup || usage.OriginModel != input.OriginModel ||
				usage.PeriodStart != input.Snapshot.PeriodStart {
				return ErrGroupModelDiscountAggregateCorrupt
			}
			if usage.PolicyHash != input.Snapshot.PolicyHash || usage.PolicySnapshot != snapshotJSON ||
				usage.PeriodEnd != input.Snapshot.PeriodEnd ||
				usage.ProgressBasis != groupdiscount.NormalizeProgressBasis(input.Snapshot.ProgressBasis) {
				return ErrGroupModelDiscountPolicyConflict
			}
			if unresolvedErr := checkGroupModelDiscountScopeUnresolved(tx, usage.Id); unresolvedErr != nil {
				return unresolvedErr
			}
			calculation, calculateErr := groupdiscount.CalculateWithProgress(
				input.Snapshot,
				usage.OriginalQuota,
				usage.ProgressQuota,
				input.OriginalQuota,
			)
			if calculateErr != nil {
				return calculateErr
			}
			if usage.ChargedQuota > math.MaxInt64-int64(calculation.ChargedQuota) {
				return groupdiscount.ErrMonthlyQuotaOverflow
			}
			segmentsJSON, marshalErr := common.Marshal(calculation.Segments)
			if marshalErr != nil {
				return marshalErr
			}

			settlement := GroupModelDiscountSettlement{
				RequestID:             input.RequestID,
				Fingerprint:           fingerprint,
				UsageID:               usage.Id,
				UserID:                input.UserID,
				UsingGroup:            input.UsingGroup,
				OriginModel:           input.OriginModel,
				PeriodStart:           input.Snapshot.PeriodStart,
				PeriodEnd:             input.Snapshot.PeriodEnd,
				PolicyHash:            input.Snapshot.PolicyHash,
				PolicySnapshot:        snapshotJSON,
				ProgressBasis:         groupdiscount.NormalizeProgressBasis(input.Snapshot.ProgressBasis),
				MonthlyOriginalBefore: calculation.MonthlyOriginalBefore,
				MonthlyOriginalAfter:  calculation.MonthlyOriginalAfter,
				MonthlyProgressBefore: calculation.MonthlyProgressBefore,
				MonthlyProgressAfter:  calculation.MonthlyProgressAfter,
				OriginalQuota:         int64(input.OriginalQuota),
				ChargedQuota:          int64(calculation.ChargedQuota),
				ProgressQuota:         calculation.ProgressQuota,
				CurrentOriginalQuota:  int64(input.OriginalQuota),
				CurrentChargedQuota:   int64(calculation.ChargedQuota),
				CurrentProgressQuota:  calculation.ProgressQuota,
				LastCursorRevision:    usage.Revision + 1,
				Segments:              string(segmentsJSON),
				Status:                GroupModelDiscountStatusReserved,
				Revision:              1,
			}
			if createErr := tx.Create(&settlement).Error; createErr != nil {
				return createErr
			}
			updatedUsage := tx.Model(&UserGroupModelMonthlyUsage{}).
				Where("id = ? AND revision = ?", usage.Id, usage.Revision).
				Updates(map[string]any{
					"original_quota":     calculation.MonthlyOriginalAfter,
					"charged_quota":      usage.ChargedQuota + int64(calculation.ChargedQuota),
					"progress_quota":     calculation.MonthlyProgressAfter,
					"tail_settlement_id": settlement.Id,
					"revision":           usage.Revision + 1,
				})
			if updatedUsage.Error != nil {
				return updatedUsage.Error
			}
			if updatedUsage.RowsAffected != 1 {
				return errGroupModelDiscountRevisionConflict
			}
			reservation = GroupModelDiscountReservation{
				Settlement:  settlement,
				Calculation: calculation,
			}
			return nil
		})
		if err == nil {
			return reservation, nil
		}

		// A concurrent transaction using the same request ID may have won the
		// unique key race. Read it back and enforce fingerprint equality.
		if existing, getErr := getGroupModelDiscountSettlement(db, input.RequestID); getErr == nil {
			return reservationFromSettlement(existing, fingerprint, true)
		}
		if !isRetryableGroupModelDiscountError(err) {
			return GroupModelDiscountReservation{}, err
		}
		time.Sleep(groupModelDiscountRetryDelay(attempt))
	}
	if errors.Is(err, ErrGroupModelDiscountScopeBusy) {
		return GroupModelDiscountReservation{}, fmt.Errorf("%w after %d attempts", ErrGroupModelDiscountScopeBusy, groupModelDiscountMaxAttempts)
	}
	return GroupModelDiscountReservation{}, fmt.Errorf("%w: %w", ErrGroupModelDiscountContention, err)
}

func CommitGroupModelDiscountSettlement(requestID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return commitGroupModelDiscountSettlement(DB, requestID)
}

func commitGroupModelDiscountSettlement(db *gorm.DB, requestID string) error {
	return commitGroupModelDiscountSettlementAccounting(db, requestID, nil)
}

// CommitGroupModelDiscountSettlementWithUsage advances the reservation and
// its user/channel accounting in one primary-database transaction. A settled
// replay succeeds only when its durable evidence exactly matches delta.
func CommitGroupModelDiscountSettlementWithUsage(requestID string, delta BillingUsageDelta) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return commitGroupModelDiscountSettlementWithUsage(DB, requestID, delta)
}

func commitGroupModelDiscountSettlementWithUsage(db *gorm.DB, requestID string, delta BillingUsageDelta) error {
	return commitGroupModelDiscountSettlementAccounting(db, requestID, &delta)
}

func commitGroupModelDiscountSettlementAccounting(db *gorm.DB, requestID string, accounting *BillingUsageDelta) error {
	return transitionGroupModelDiscountSettlement(db, requestID, func(tx *gorm.DB, settlement GroupModelDiscountSettlement) error {
		switch settlement.Status {
		case GroupModelDiscountStatusSettled:
			if accounting != nil {
				return validateGroupModelDiscountSettlementAccountingEvidence(settlement, *accounting)
			}
			return nil
		case GroupModelDiscountStatusReserved:
			// Continue below.
		case GroupModelDiscountStatusPendingReconcile:
			if settlement.PendingAction != GroupModelDiscountPendingActionCommitAfterFunding {
				return fmt.Errorf("%w: %s -> %s", ErrGroupModelDiscountInvalidTransition, settlement.Status, GroupModelDiscountStatusSettled)
			}
		default:
			return fmt.Errorf("%w: %s -> %s", ErrGroupModelDiscountInvalidTransition, settlement.Status, GroupModelDiscountStatusSettled)
		}
		if accounting == nil {
			return updateGroupModelDiscountStatusCAS(tx, settlement, GroupModelDiscountStatusSettled)
		}
		if err := validateGroupModelDiscountSettlementAccountingDelta(settlement, *accounting); err != nil {
			return err
		}
		if err := applyGroupModelDiscountUsageDelta(tx, *accounting); err != nil {
			return err
		}
		return updateGroupModelDiscountStatusWithAccountingCAS(tx, settlement, GroupModelDiscountStatusSettled, *accounting)
	})
}

// MarkGroupModelDiscountPendingReconcile fail-closes a reservation whose
// funding result is unknown. Settled reversals use Begin/Cancel/MarkUnknown so
// their safety gate always precedes the external refund. PendingAction durably
// records which single ledger transition, if any, may be retried;
// unknown/manual never authorizes an automatic transition.
func MarkGroupModelDiscountPendingReconcile(requestID, pendingAction string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return markGroupModelDiscountPendingReconcile(DB, requestID, pendingAction)
}

func markGroupModelDiscountPendingReconcile(db *gorm.DB, requestID, pendingAction string) error {
	if !isGroupModelDiscountPendingAction(pendingAction) {
		return ErrGroupModelDiscountInvalidInput
	}
	return transitionGroupModelDiscountSettlementScope(db, requestID, func(tx *gorm.DB, _ UserGroupModelMonthlyUsage, settlement GroupModelDiscountSettlement) error {
		switch settlement.Status {
		case GroupModelDiscountStatusPendingReconcile:
			if settlement.PendingAction == pendingAction {
				return nil
			}
		case GroupModelDiscountStatusReserved:
			if pendingAction != GroupModelDiscountPendingActionReverseAfterRefund {
				return updateGroupModelDiscountPendingCAS(tx, settlement, pendingAction)
			}
		case GroupModelDiscountStatusSettled:
			if pendingAction == GroupModelDiscountPendingActionUnknownManual {
				return updateGroupModelDiscountPendingCAS(tx, settlement, pendingAction)
			}
		}
		return fmt.Errorf("%w: %s/%s -> %s/%s",
			ErrGroupModelDiscountInvalidTransition,
			settlement.Status,
			settlement.PendingAction,
			GroupModelDiscountStatusPendingReconcile,
			pendingAction,
		)
	})
}

// BeginGroupModelDiscountSettlementReverse atomically gates the monthly scope
// before any funding refund is attempted. Only the active ledger tail can be
// prepared, because removing a historical settlement would otherwise make
// cumulative tier rounding impossible to reconstruct without repricing later
// requests.
func BeginGroupModelDiscountSettlementReverse(requestID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return beginGroupModelDiscountSettlementReverse(DB, requestID)
}

func beginGroupModelDiscountSettlementReverse(db *gorm.DB, requestID string) error {
	return transitionGroupModelDiscountSettlementScope(db, requestID, func(tx *gorm.DB, usage UserGroupModelMonthlyUsage, settlement GroupModelDiscountSettlement) error {
		if settlement.Status == GroupModelDiscountStatusPendingReconcile &&
			settlement.PendingAction == GroupModelDiscountPendingActionReverseAfterRefund {
			return nil
		}
		if settlement.Status != GroupModelDiscountStatusSettled {
			return fmt.Errorf("%w: %s/%s -> %s/%s",
				ErrGroupModelDiscountInvalidTransition,
				settlement.Status,
				settlement.PendingAction,
				GroupModelDiscountStatusPendingReconcile,
				GroupModelDiscountPendingActionReverseAfterRefund,
			)
		}
		if unresolvedErr := checkGroupModelDiscountOtherSettlementUnresolved(tx, usage.Id, settlement.Id); unresolvedErr != nil {
			return unresolvedErr
		}
		if unresolvedErr := checkGroupModelDiscountAdjustmentUnresolved(tx, usage.Id); unresolvedErr != nil {
			return unresolvedErr
		}
		if _, err := groupModelDiscountProgressAfterCompensation(usage, settlement); err != nil {
			return err
		}
		tail, err := isGroupModelDiscountSettlementTail(usage, settlement)
		if err != nil {
			return err
		}
		if !tail {
			return ErrGroupModelDiscountNonTailReverse
		}
		return updateGroupModelDiscountPendingCAS(tx, settlement, GroupModelDiscountPendingActionReverseAfterRefund)
	})
}

// CancelGroupModelDiscountSettlementReverse reopens a prepared settlement
// only when the caller knows the funding refund failed and no money moved.
func CancelGroupModelDiscountSettlementReverse(requestID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return cancelGroupModelDiscountSettlementReverse(DB, requestID)
}

func cancelGroupModelDiscountSettlementReverse(db *gorm.DB, requestID string) error {
	return transitionGroupModelDiscountSettlementScope(db, requestID, func(tx *gorm.DB, _ UserGroupModelMonthlyUsage, settlement GroupModelDiscountSettlement) error {
		if settlement.Status == GroupModelDiscountStatusSettled {
			return nil
		}
		if settlement.Status == GroupModelDiscountStatusPendingReconcile &&
			settlement.PendingAction == GroupModelDiscountPendingActionReverseAfterRefund {
			return updateGroupModelDiscountStatusCAS(tx, settlement, GroupModelDiscountStatusSettled)
		}
		return fmt.Errorf("%w: %s/%s -> %s",
			ErrGroupModelDiscountInvalidTransition,
			settlement.Status,
			settlement.PendingAction,
			GroupModelDiscountStatusSettled,
		)
	})
}

// MarkGroupModelDiscountSettlementReverseFundingUnknown makes an ambiguous
// refund outcome manual-only. Neither automatic cancel nor confirmation may
// cross this state, and the pending settlement continues to gate the scope.
func MarkGroupModelDiscountSettlementReverseFundingUnknown(requestID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return markGroupModelDiscountSettlementReverseFundingUnknown(DB, requestID)
}

func markGroupModelDiscountSettlementReverseFundingUnknown(db *gorm.DB, requestID string) error {
	return transitionGroupModelDiscountSettlementScope(db, requestID, func(tx *gorm.DB, _ UserGroupModelMonthlyUsage, settlement GroupModelDiscountSettlement) error {
		if settlement.Status == GroupModelDiscountStatusPendingReconcile &&
			settlement.PendingAction == GroupModelDiscountPendingActionUnknownManual {
			return nil
		}
		if settlement.Status == GroupModelDiscountStatusPendingReconcile &&
			settlement.PendingAction == GroupModelDiscountPendingActionReverseAfterRefund {
			return updateGroupModelDiscountPendingCAS(tx, settlement, GroupModelDiscountPendingActionUnknownManual)
		}
		return fmt.Errorf("%w: %s/%s -> %s/%s",
			ErrGroupModelDiscountInvalidTransition,
			settlement.Status,
			settlement.PendingAction,
			GroupModelDiscountStatusPendingReconcile,
			GroupModelDiscountPendingActionUnknownManual,
		)
	})
}

// ReverseGroupModelDiscountSettlement updates only the monthly ledger. The
// caller must first prepare the safe tail with Begin, then complete and persist
// the exact funding refund. The prepared transition may be retried here without
// issuing the refund a second time.
func ReverseGroupModelDiscountSettlement(requestID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return reverseGroupModelDiscountSettlement(DB, requestID)
}

func reverseGroupModelDiscountSettlement(db *gorm.DB, requestID string) error {
	return compensateGroupModelDiscountSettlement(db, requestID, false, nil)
}

// ReverseGroupModelDiscountSettlementWithUsage confirms a prepared refund and
// applies its negative usage delta atomically with the ledger compensation.
func ReverseGroupModelDiscountSettlementWithUsage(requestID string, delta BillingUsageDelta) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return reverseGroupModelDiscountSettlementWithUsage(DB, requestID, delta)
}

func reverseGroupModelDiscountSettlementWithUsage(db *gorm.DB, requestID string, delta BillingUsageDelta) error {
	return compensateGroupModelDiscountSettlement(db, requestID, false, &delta)
}

// RollbackGroupModelDiscountReservation compensates a funding attempt that is
// known not to have completed. It is deliberately separate from Reverse: an
// unresolved reservation may only be removed while it is the latest cursor,
// preserving the cumulative rounding invariant for every later request.
func RollbackGroupModelDiscountReservation(requestID string) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return rollbackGroupModelDiscountReservation(DB, requestID)
}

func rollbackGroupModelDiscountReservation(db *gorm.DB, requestID string) error {
	return compensateGroupModelDiscountSettlement(db, requestID, true, nil)
}

// compensateGroupModelDiscountSettlement updates only the ledger. For a
// prepared reversal the caller must already have completed and verified the
// exact funding refund. Pending is accepted so a failed ledger write can be
// retried without refunding funds twice. Both rollback and refund confirmation
// are tail-only, preserving cumulative tier boundaries and rounding.
func compensateGroupModelDiscountSettlement(
	db *gorm.DB,
	requestID string,
	rollbackReservation bool,
	accounting *BillingUsageDelta,
) error {
	if strings.TrimSpace(requestID) == "" || len(requestID) > maxDiscountRequestIDLength {
		return ErrGroupModelDiscountInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < groupModelDiscountMaxAttempts; attempt++ {
		var hint GroupModelDiscountSettlement
		if lastErr = db.Where("request_id = ?", requestID).First(&hint).Error; lastErr != nil {
			return lastErr
		}
		lastErr = db.Transaction(func(tx *gorm.DB) error {
			// Every operation that mutates a monthly cursor locks usage first.
			// This avoids a settlement<->usage lock inversion with adjustments.
			var usage UserGroupModelMonthlyUsage
			if err := lockForUpdate(tx).Where("id = ?", hint.UsageID).First(&usage).Error; err != nil {
				return err
			}
			var settlement GroupModelDiscountSettlement
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&settlement).Error; err != nil {
				return err
			}
			if settlement.Status == GroupModelDiscountStatusReversed {
				if accounting != nil {
					return validateGroupModelDiscountReverseAccountingEvidence(settlement, *accounting)
				}
				return nil
			}
			validStatus := false
			if rollbackReservation {
				validStatus = settlement.Status == GroupModelDiscountStatusReserved ||
					(settlement.Status == GroupModelDiscountStatusPendingReconcile &&
						settlement.PendingAction == GroupModelDiscountPendingActionRollbackUnfunded)
			} else {
				validStatus = settlement.Status == GroupModelDiscountStatusPendingReconcile &&
					settlement.PendingAction == GroupModelDiscountPendingActionReverseAfterRefund
			}
			if !validStatus {
				return fmt.Errorf("%w: %s -> %s", ErrGroupModelDiscountInvalidTransition, settlement.Status, GroupModelDiscountStatusReversed)
			}
			if !rollbackReservation {
				var otherSettlementStatuses []string
				if err := tx.Model(&GroupModelDiscountSettlement{}).Select("status").
					Where("usage_id = ? AND id <> ? AND status IN ?", usage.Id, settlement.Id, []string{
						GroupModelDiscountStatusReserved,
						GroupModelDiscountStatusPendingReconcile,
					}).Find(&otherSettlementStatuses).Error; err != nil {
					return err
				}
				for _, status := range otherSettlementStatuses {
					if status == GroupModelDiscountStatusPendingReconcile {
						return ErrGroupModelDiscountPendingReconcile
					}
				}
				if unresolvedErr := checkGroupModelDiscountAdjustmentUnresolved(tx, usage.Id); unresolvedErr != nil {
					return unresolvedErr
				}
				if len(otherSettlementStatuses) > 0 {
					return ErrGroupModelDiscountScopeBusy
				}
			}
			usageProgressAfter, progressErr := groupModelDiscountProgressAfterCompensation(usage, settlement)
			if progressErr != nil {
				return progressErr
			}
			tail, tailErr := isGroupModelDiscountSettlementTail(usage, settlement)
			if tailErr != nil {
				return tailErr
			}
			if !tail {
				return ErrGroupModelDiscountRequiresReconcile
			}
			activeOriginal := settlement.CurrentOriginalQuota
			activeCharged := settlement.CurrentChargedQuota
			nextTailSettlementID, tailErr := previousGroupModelDiscountTailSettlementID(tx, usage.Id, settlement.Id)
			if tailErr != nil {
				return tailErr
			}
			if accounting != nil {
				if rollbackReservation {
					return ErrGroupModelDiscountAccountingConflict
				}
				if accountingErr := validateGroupModelDiscountReverseAccountingDelta(tx, settlement, *accounting); accountingErr != nil {
					return accountingErr
				}
				if accountingErr := applyGroupModelDiscountUsageDelta(tx, *accounting); accountingErr != nil {
					return accountingErr
				}
			}
			updatedUsage := tx.Model(&UserGroupModelMonthlyUsage{}).
				Where("id = ? AND revision = ?", usage.Id, usage.Revision).
				Updates(map[string]any{
					"original_quota":     usage.OriginalQuota - activeOriginal,
					"charged_quota":      usage.ChargedQuota - activeCharged,
					"progress_quota":     usageProgressAfter.String(),
					"tail_settlement_id": nextTailSettlementID,
					"revision":           usage.Revision + 1,
				})
			if updatedUsage.Error != nil {
				return updatedUsage.Error
			}
			if updatedUsage.RowsAffected != 1 {
				return errGroupModelDiscountRevisionConflict
			}
			if accounting != nil {
				return updateGroupModelDiscountStatusWithReverseAccountingCAS(tx, settlement, *accounting)
			}
			return updateGroupModelDiscountStatusCAS(tx, settlement, GroupModelDiscountStatusReversed)
		})
		if lastErr == nil {
			return nil
		}
		if !isRetryableGroupModelDiscountError(lastErr) {
			return lastErr
		}
		time.Sleep(groupModelDiscountRetryDelay(attempt))
	}
	if errors.Is(lastErr, ErrGroupModelDiscountScopeBusy) {
		return fmt.Errorf("%w after %d attempts", ErrGroupModelDiscountScopeBusy, groupModelDiscountMaxAttempts)
	}
	return fmt.Errorf("%w: %w", ErrGroupModelDiscountContention, lastErr)
}

func GetGroupModelDiscountSettlement(requestID string) (GroupModelDiscountSettlement, error) {
	if DB == nil {
		return GroupModelDiscountSettlement{}, errors.New("database is not initialized")
	}
	return getGroupModelDiscountSettlement(DB, requestID)
}

func getGroupModelDiscountSettlement(db *gorm.DB, requestID string) (GroupModelDiscountSettlement, error) {
	var settlement GroupModelDiscountSettlement
	err := db.Where("request_id = ?", requestID).First(&settlement).Error
	return settlement, err
}

func GetUserGroupModelMonthlyUsage(userID int, usingGroup, originModel string, periodStart int64) (UserGroupModelMonthlyUsage, error) {
	if DB == nil {
		return UserGroupModelMonthlyUsage{}, errors.New("database is not initialized")
	}
	return getUserGroupModelMonthlyUsage(DB, userID, usingGroup, originModel, periodStart)
}

func getUserGroupModelMonthlyUsage(db *gorm.DB, userID int, usingGroup, originModel string, periodStart int64) (UserGroupModelMonthlyUsage, error) {
	scopeHash, err := groupModelDiscountScopeHash(userID, usingGroup, originModel, periodStart)
	if err != nil {
		return UserGroupModelMonthlyUsage{}, err
	}
	var usage UserGroupModelMonthlyUsage
	if err := db.Where("scope_hash = ?", scopeHash).First(&usage).Error; err != nil {
		return UserGroupModelMonthlyUsage{}, err
	}
	if usage.ScopeHash != scopeHash || usage.UserID != userID || usage.UsingGroup != usingGroup ||
		usage.OriginModel != originModel || usage.PeriodStart != periodStart {
		return UserGroupModelMonthlyUsage{}, ErrGroupModelDiscountAggregateCorrupt
	}
	return usage, nil
}

func groupModelDiscountScopeHash(userID int, usingGroup, originModel string, periodStart int64) (string, error) {
	payload, err := common.Marshal(groupModelDiscountScopeIdentity{
		UserID:      userID,
		UsingGroup:  usingGroup,
		OriginModel: originModel,
		PeriodStart: periodStart,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func validateAndFingerprintGroupModelDiscountInput(input GroupModelDiscountReserveInput) (string, string, error) {
	if strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > maxDiscountRequestIDLength ||
		input.UserID <= 0 || input.UsingGroup == "" || len(input.UsingGroup) > groupdiscount.MaxUsingGroupLength ||
		input.OriginModel == "" || len(input.OriginModel) > groupdiscount.MaxOriginModelLength ||
		input.Snapshot.UsingGroup != input.UsingGroup || input.Snapshot.OriginModel != input.OriginModel ||
		input.Snapshot.PolicyHash == "" || len(input.Snapshot.PolicyHash) > 128 ||
		input.Snapshot.PeriodStart < 0 || input.Snapshot.PeriodStart >= input.Snapshot.PeriodEnd {
		return "", "", ErrGroupModelDiscountInvalidInput
	}
	if _, err := groupdiscount.Calculate(input.Snapshot, 0, input.OriginalQuota); err != nil {
		return "", "", err
	}
	payload := groupModelDiscountFingerprintPayload{
		UserID:        input.UserID,
		UsingGroup:    input.UsingGroup,
		OriginModel:   input.OriginModel,
		Snapshot:      input.Snapshot,
		OriginalQuota: input.OriginalQuota,
	}
	payloadJSON, err := common.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(payloadJSON)
	snapshotJSON, err := common.Marshal(input.Snapshot)
	if err != nil {
		return "", "", err
	}
	return hex.EncodeToString(digest[:]), string(snapshotJSON), nil
}

func reservationFromSettlement(settlement GroupModelDiscountSettlement, fingerprint string, reused bool) (GroupModelDiscountReservation, error) {
	if settlement.Fingerprint != fingerprint {
		return GroupModelDiscountReservation{}, ErrGroupModelDiscountRequestConflict
	}
	if settlement.OriginalQuota <= 0 || settlement.OriginalQuota > common.MaxQuota ||
		settlement.ChargedQuota < 0 || settlement.ChargedQuota > common.MaxQuota ||
		settlement.LastCursorRevision <= 0 {
		return GroupModelDiscountReservation{}, ErrGroupModelDiscountAggregateCorrupt
	}
	if _, err := groupdiscount.ParseProgressQuota(settlement.MonthlyProgressBefore); err != nil {
		return GroupModelDiscountReservation{}, ErrGroupModelDiscountAggregateCorrupt
	}
	if _, err := groupdiscount.ParseProgressQuota(settlement.MonthlyProgressAfter); err != nil {
		return GroupModelDiscountReservation{}, ErrGroupModelDiscountAggregateCorrupt
	}
	if _, err := groupdiscount.ParseProgressDelta(settlement.ProgressQuota); err != nil {
		return GroupModelDiscountReservation{}, ErrGroupModelDiscountAggregateCorrupt
	}
	var snapshot groupdiscount.Snapshot
	if err := common.UnmarshalJsonStr(settlement.PolicySnapshot, &snapshot); err != nil ||
		groupdiscount.NormalizeProgressBasis(snapshot.ProgressBasis) != settlement.ProgressBasis {
		return GroupModelDiscountReservation{}, ErrGroupModelDiscountAggregateCorrupt
	}
	var segments []groupdiscount.Segment
	if err := common.UnmarshalJsonStr(settlement.Segments, &segments); err != nil {
		return GroupModelDiscountReservation{}, fmt.Errorf("decode group model discount segments: %w", err)
	}
	return GroupModelDiscountReservation{
		Settlement: settlement,
		Calculation: groupdiscount.Calculation{
			MonthlyOriginalBefore: settlement.MonthlyOriginalBefore,
			MonthlyOriginalAfter:  settlement.MonthlyOriginalAfter,
			MonthlyProgressBefore: settlement.MonthlyProgressBefore,
			MonthlyProgressAfter:  settlement.MonthlyProgressAfter,
			OriginalQuota:         int(settlement.OriginalQuota),
			ChargedQuota:          int(settlement.ChargedQuota),
			ProgressQuota:         settlement.ProgressQuota,
			Segments:              segments,
		},
		Reused: reused,
	}, nil
}

func transitionGroupModelDiscountSettlement(
	db *gorm.DB,
	requestID string,
	transition func(*gorm.DB, GroupModelDiscountSettlement) error,
) error {
	if strings.TrimSpace(requestID) == "" || len(requestID) > maxDiscountRequestIDLength {
		return ErrGroupModelDiscountInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < groupModelDiscountMaxAttempts; attempt++ {
		lastErr = db.Transaction(func(tx *gorm.DB) error {
			var settlement GroupModelDiscountSettlement
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&settlement).Error; err != nil {
				return err
			}
			return transition(tx, settlement)
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

func transitionGroupModelDiscountSettlementScope(
	db *gorm.DB,
	requestID string,
	transition func(*gorm.DB, UserGroupModelMonthlyUsage, GroupModelDiscountSettlement) error,
) error {
	if strings.TrimSpace(requestID) == "" || len(requestID) > maxDiscountRequestIDLength {
		return ErrGroupModelDiscountInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < groupModelDiscountMaxAttempts; attempt++ {
		var hint GroupModelDiscountSettlement
		if lastErr = db.Where("request_id = ?", requestID).First(&hint).Error; lastErr != nil {
			return lastErr
		}
		lastErr = db.Transaction(func(tx *gorm.DB) error {
			var usage UserGroupModelMonthlyUsage
			if err := lockForUpdate(tx).Where("id = ?", hint.UsageID).First(&usage).Error; err != nil {
				return err
			}
			var settlement GroupModelDiscountSettlement
			if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&settlement).Error; err != nil {
				return err
			}
			if settlement.UsageID != usage.Id {
				return ErrGroupModelDiscountAggregateCorrupt
			}
			return transition(tx, usage, settlement)
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

func checkGroupModelDiscountOtherSettlementUnresolved(tx *gorm.DB, usageID, excludedSettlementID int64) error {
	var statuses []string
	if err := tx.Model(&GroupModelDiscountSettlement{}).Select("status").
		Where("usage_id = ? AND id <> ? AND status IN ?", usageID, excludedSettlementID, []string{
			GroupModelDiscountStatusReserved,
			GroupModelDiscountStatusPendingReconcile,
		}).Find(&statuses).Error; err != nil {
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

func groupModelDiscountProgressAfterCompensation(
	usage UserGroupModelMonthlyUsage,
	settlement GroupModelDiscountSettlement,
) (decimal.Decimal, error) {
	expectedScopeHash, err := groupModelDiscountScopeHash(
		settlement.UserID,
		settlement.UsingGroup,
		settlement.OriginModel,
		settlement.PeriodStart,
	)
	if err != nil || usage.ScopeHash != expectedScopeHash {
		return decimal.Zero, ErrGroupModelDiscountAggregateCorrupt
	}
	activeOriginal := settlement.CurrentOriginalQuota
	activeCharged := settlement.CurrentChargedQuota
	usageProgress, err := groupdiscount.ParseProgressQuota(usage.ProgressQuota)
	if err != nil {
		return decimal.Zero, ErrGroupModelDiscountAggregateCorrupt
	}
	activeProgress, err := groupdiscount.ParseProgressQuota(settlement.CurrentProgressQuota)
	if err != nil {
		return decimal.Zero, ErrGroupModelDiscountAggregateCorrupt
	}
	usageProgressAfter := usageProgress.Sub(activeProgress)
	if activeOriginal <= 0 || activeOriginal > common.MaxQuota || activeCharged < 0 || activeCharged > common.MaxQuota ||
		usageProgressAfter.IsNegative() ||
		usage.Id != settlement.UsageID || usage.UserID != settlement.UserID || usage.UsingGroup != settlement.UsingGroup ||
		usage.OriginModel != settlement.OriginModel || usage.PeriodStart != settlement.PeriodStart ||
		usage.PolicyHash != settlement.PolicyHash || usage.PolicySnapshot != settlement.PolicySnapshot ||
		usage.ProgressBasis != settlement.ProgressBasis ||
		usage.OriginalQuota < activeOriginal || usage.ChargedQuota < activeCharged {
		return decimal.Zero, ErrGroupModelDiscountAggregateCorrupt
	}
	return usageProgressAfter, nil
}

func isGroupModelDiscountSettlementTail(
	usage UserGroupModelMonthlyUsage,
	settlement GroupModelDiscountSettlement,
) (bool, error) {
	if usage.TailSettlementID < 0 || settlement.LastCursorRevision <= 0 || settlement.LastCursorRevision > usage.Revision {
		return false, ErrGroupModelDiscountAggregateCorrupt
	}
	return usage.TailSettlementID == settlement.Id, nil
}

func previousGroupModelDiscountTailSettlementID(tx *gorm.DB, usageID, excludedSettlementID int64) (int64, error) {
	var settlement GroupModelDiscountSettlement
	err := tx.Select("id", "last_cursor_revision").
		Where("usage_id = ? AND id <> ? AND status = ?", usageID, excludedSettlementID, GroupModelDiscountStatusSettled).
		Order("last_cursor_revision DESC").
		Order("id DESC").
		First(&settlement).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if settlement.LastCursorRevision <= 0 {
		return 0, ErrGroupModelDiscountAggregateCorrupt
	}
	return settlement.Id, nil
}

func validateGroupModelDiscountSettlementAccountingDelta(
	settlement GroupModelDiscountSettlement,
	delta BillingUsageDelta,
) error {
	if settlement.ChargedQuota < 0 || settlement.ChargedQuota > common.MaxQuota ||
		delta.UserID != settlement.UserID || delta.ChannelID <= 0 ||
		delta.QuotaDelta != int(settlement.ChargedQuota) || delta.RequestCountDelta != 1 {
		return ErrGroupModelDiscountAccountingConflict
	}
	return nil
}

func applyGroupModelDiscountUsageDelta(tx *gorm.DB, delta BillingUsageDelta) error {
	if delta.QuotaDelta != 0 {
		return applyBillingUsageDelta(tx, delta)
	}
	if delta.UserID <= 0 || delta.ChannelID <= 0 ||
		(delta.RequestCountDelta != 0 && delta.RequestCountDelta != 1) {
		return ErrGroupModelDiscountAccountingConflict
	}
	if delta.RequestCountDelta == 1 {
		userUpdate := tx.Model(&User{}).
			Where("id = ?", delta.UserID).
			Update("request_count", gorm.Expr("request_count + ?", delta.RequestCountDelta))
		if userUpdate.Error != nil {
			return fmt.Errorf("update zero-quota user billing usage: %w", userUpdate.Error)
		}
		if userUpdate.RowsAffected != 1 {
			return fmt.Errorf("%w: user %d", ErrBillingUsageTargetNotFound, delta.UserID)
		}
	} else {
		var user User
		if err := lockForUpdate(tx).Select("id").Where("id = ?", delta.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: user %d", ErrBillingUsageTargetNotFound, delta.UserID)
			}
			return fmt.Errorf("load zero-delta user billing target: %w", err)
		}
	}
	var channel Channel
	if err := lockForUpdate(tx).Select("id").Where("id = ?", delta.ChannelID).First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: channel %d", ErrBillingUsageTargetNotFound, delta.ChannelID)
		}
		return fmt.Errorf("load zero-delta channel billing target: %w", err)
	}
	return nil
}

func validateGroupModelDiscountSettlementAccountingEvidence(
	settlement GroupModelDiscountSettlement,
	delta BillingUsageDelta,
) error {
	if !settlement.AccountingApplied ||
		settlement.AccountingUserID != delta.UserID ||
		settlement.AccountingChannelID != delta.ChannelID ||
		settlement.AccountingQuotaDelta != delta.QuotaDelta ||
		settlement.AccountingRequestCountDelta != delta.RequestCountDelta {
		return ErrGroupModelDiscountAccountingConflict
	}
	return validateGroupModelDiscountSettlementAccountingDelta(settlement, delta)
}

func validateGroupModelDiscountReverseAccountingDelta(
	tx *gorm.DB,
	settlement GroupModelDiscountSettlement,
	delta BillingUsageDelta,
) error {
	if !settlement.AccountingApplied ||
		settlement.AccountingUserID != settlement.UserID ||
		settlement.AccountingChannelID <= 0 ||
		settlement.AccountingQuotaDelta != int(settlement.ChargedQuota) ||
		settlement.AccountingRequestCountDelta != 1 ||
		delta.UserID != settlement.AccountingUserID ||
		delta.ChannelID != settlement.AccountingChannelID ||
		delta.QuotaDelta != -int(settlement.CurrentChargedQuota) ||
		delta.RequestCountDelta != 0 {
		return ErrGroupModelDiscountAccountingConflict
	}

	var adjustments []GroupModelDiscountAdjustment
	if err := tx.Where("settlement_id = ? AND status = ?", settlement.Id, GroupModelDiscountStatusSettled).
		Order("id").
		Find(&adjustments).Error; err != nil {
		return err
	}
	accountedCharge := int64(settlement.AccountingQuotaDelta)
	for _, adjustment := range adjustments {
		if !adjustment.AccountingApplied ||
			adjustment.AccountingUserID != settlement.AccountingUserID ||
			adjustment.AccountingChannelID != settlement.AccountingChannelID ||
			adjustment.AccountingQuotaDelta != int(adjustment.DeltaChargedQuota) ||
			adjustment.AccountingRequestCountDelta != 0 {
			return ErrGroupModelDiscountAccountingConflict
		}
		accountedCharge += int64(adjustment.AccountingQuotaDelta)
		if accountedCharge < 0 || accountedCharge > common.MaxQuota {
			return ErrGroupModelDiscountAccountingConflict
		}
	}
	if accountedCharge != settlement.CurrentChargedQuota {
		return ErrGroupModelDiscountAccountingConflict
	}
	return nil
}

func validateGroupModelDiscountReverseAccountingEvidence(
	settlement GroupModelDiscountSettlement,
	delta BillingUsageDelta,
) error {
	if !settlement.ReverseAccountingApplied ||
		settlement.ReverseAccountingUserID != delta.UserID ||
		settlement.ReverseAccountingChannelID != delta.ChannelID ||
		settlement.ReverseAccountingQuotaDelta != delta.QuotaDelta ||
		settlement.ReverseAccountingRequestCountDelta != delta.RequestCountDelta {
		return ErrGroupModelDiscountAccountingConflict
	}
	return nil
}

func updateGroupModelDiscountStatusCAS(tx *gorm.DB, settlement GroupModelDiscountSettlement, nextStatus string) error {
	result := tx.Model(&GroupModelDiscountSettlement{}).
		Where("id = ? AND revision = ? AND status = ?", settlement.Id, settlement.Revision, settlement.Status).
		Updates(map[string]any{
			"status":         nextStatus,
			"pending_action": "",
			"revision":       settlement.Revision + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errGroupModelDiscountRevisionConflict
	}
	return nil
}

func updateGroupModelDiscountStatusWithAccountingCAS(
	tx *gorm.DB,
	settlement GroupModelDiscountSettlement,
	nextStatus string,
	delta BillingUsageDelta,
) error {
	result := tx.Model(&GroupModelDiscountSettlement{}).
		Where("id = ? AND revision = ? AND status = ?", settlement.Id, settlement.Revision, settlement.Status).
		Updates(map[string]any{
			"status":                         nextStatus,
			"pending_action":                 "",
			"accounting_applied":             true,
			"accounting_user_id":             delta.UserID,
			"accounting_channel_id":          delta.ChannelID,
			"accounting_quota_delta":         delta.QuotaDelta,
			"accounting_request_count_delta": delta.RequestCountDelta,
			"revision":                       settlement.Revision + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errGroupModelDiscountRevisionConflict
	}
	return nil
}

func updateGroupModelDiscountStatusWithReverseAccountingCAS(
	tx *gorm.DB,
	settlement GroupModelDiscountSettlement,
	delta BillingUsageDelta,
) error {
	result := tx.Model(&GroupModelDiscountSettlement{}).
		Where("id = ? AND revision = ? AND status = ? AND pending_action = ?",
			settlement.Id,
			settlement.Revision,
			GroupModelDiscountStatusPendingReconcile,
			GroupModelDiscountPendingActionReverseAfterRefund,
		).
		Updates(map[string]any{
			"status":                                 GroupModelDiscountStatusReversed,
			"pending_action":                         "",
			"reverse_accounting_applied":             true,
			"reverse_accounting_user_id":             delta.UserID,
			"reverse_accounting_channel_id":          delta.ChannelID,
			"reverse_accounting_quota_delta":         delta.QuotaDelta,
			"reverse_accounting_request_count_delta": delta.RequestCountDelta,
			"revision":                               settlement.Revision + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errGroupModelDiscountRevisionConflict
	}
	return nil
}

func updateGroupModelDiscountPendingCAS(tx *gorm.DB, settlement GroupModelDiscountSettlement, pendingAction string) error {
	result := tx.Model(&GroupModelDiscountSettlement{}).
		Where("id = ? AND revision = ? AND status = ? AND pending_action = ?",
			settlement.Id, settlement.Revision, settlement.Status, settlement.PendingAction).
		Updates(map[string]any{
			"status":         GroupModelDiscountStatusPendingReconcile,
			"pending_action": pendingAction,
			"revision":       settlement.Revision + 1,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errGroupModelDiscountRevisionConflict
	}
	return nil
}

func isGroupModelDiscountPendingAction(action string) bool {
	switch action {
	case GroupModelDiscountPendingActionUnknownManual,
		GroupModelDiscountPendingActionCommitAfterFunding,
		GroupModelDiscountPendingActionReverseAfterRefund,
		GroupModelDiscountPendingActionRollbackUnfunded:
		return true
	default:
		return false
	}
}

func isRetryableGroupModelDiscountError(err error) bool {
	if errors.Is(err, errGroupModelDiscountRevisionConflict) || errors.Is(err, ErrGroupModelDiscountScopeBusy) {
		return true
	}
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1205 || mysqlErr.Number == 1213
	}
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) {
		return postgresErr.Code == "40001" || postgresErr.Code == "40P01"
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate key")
}

func groupModelDiscountRetryDelay(attempt int) time.Duration {
	delay := time.Duration(attempt+1) * time.Millisecond
	if delay > 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return delay
}
