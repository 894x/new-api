package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func groupDiscountTestSnapshot(group, model string, periodStart int64) groupdiscount.Snapshot {
	return groupdiscount.Snapshot{
		PolicyHash:   "policy-v1",
		UsingGroup:   group,
		OriginModel:  model,
		MatchedModel: model,
		Timezone:     "UTC",
		PeriodStart:  periodStart,
		PeriodEnd:    periodStart + int64((31*24*time.Hour)/time.Second),
		Tiers: []groupdiscount.Tier{
			{MinMonthlyOriginalQuota: 0, Ratio: 0.9},
			{MinMonthlyOriginalQuota: 1000, Ratio: 0.85},
		},
	}
}

func groupDiscountReserveInput(requestID, group, model string, periodStart int64, original int) GroupModelDiscountReserveInput {
	return GroupModelDiscountReserveInput{
		RequestID:     requestID,
		UserID:        101,
		UsingGroup:    group,
		OriginModel:   model,
		Snapshot:      groupDiscountTestSnapshot(group, model, periodStart),
		OriginalQuota: original,
	}
}

func newGroupDiscountFileDB(t *testing.T) *gorm.DB {
	t.Helper()
	databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "group-discount.db"))
	dsn := "file:" + databasePath + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&UserGroupModelMonthlyUsage{}, &GroupModelDiscountSettlement{}, &GroupModelDiscountAdjustment{}))
	return db
}

func TestIsRetryableGroupModelDiscountErrorRecognizesDatabaseTransientCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "mysql deadlock",
			err:  &mysqldriver.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"},
			want: true,
		},
		{
			name: "wrapped mysql lock wait timeout",
			err: fmt.Errorf("reserve settlement: %w", &mysqldriver.MySQLError{
				Number:  1205,
				Message: "Lock wait timeout exceeded",
			}),
			want: true,
		},
		{
			name: "postgres serialization failure",
			err:  &pgconn.PgError{Code: "40001", Message: "could not serialize access due to concurrent update"},
			want: true,
		},
		{
			name: "wrapped postgres deadlock",
			err: fmt.Errorf("reserve settlement: %w", &pgconn.PgError{
				Code:    "40P01",
				Message: "deadlock detected",
			}),
			want: true,
		},
		{
			name: "mysql syntax error is permanent",
			err:  &mysqldriver.MySQLError{Number: 1064, Message: "syntax error"},
		},
		{
			name: "postgres check violation is permanent",
			err:  &pgconn.PgError{Code: "23514", Message: "check constraint violation"},
		},
		{
			name: "business error mentioning deadlock is permanent",
			err:  errors.New("business deadlock policy rejection"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, isRetryableGroupModelDiscountError(test.err))
		})
	}
}

func TestReserveGroupModelDiscountSegmentsAcrossTier(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	first, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("req-first", "vip", "gpt-5", 100, 900))
	require.NoError(t, err)
	assert.Equal(t, 810, first.Calculation.ChargedQuota)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "req-first"))

	second, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("req-cross", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	assert.False(t, second.Reused)
	assert.Equal(t, 260, second.Calculation.ChargedQuota)
	assert.Equal(t, int64(900), second.Calculation.MonthlyOriginalBefore)
	require.Len(t, second.Calculation.Segments, 2)

	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1200), usage.OriginalQuota)
	assert.Equal(t, int64(1070), usage.ChargedQuota)
}

func TestReserveGroupModelDiscountKeepsScopesIndependent(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	inputs := []GroupModelDiscountReserveInput{
		groupDiscountReserveInput("req-a", "vip", "gpt-5", 100, 100),
		groupDiscountReserveInput("req-b", "vip", "claude", 100, 200),
		groupDiscountReserveInput("req-c", "svip", "gpt-5", 100, 300),
		groupDiscountReserveInput("req-d", "vip", "gpt-5", 200, 400),
	}
	for _, input := range inputs {
		reservation, err := reserveGroupModelDiscount(db, input)
		require.NoError(t, err)
		assert.Zero(t, reservation.Calculation.MonthlyOriginalBefore)
	}

	var usages []UserGroupModelMonthlyUsage
	require.NoError(t, db.Order("id").Find(&usages).Error)
	require.Len(t, usages, len(inputs))
}

func TestGroupModelDiscountMaximumLengthScopeUsesFixedWidthUniqueIdentity(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	usingGroup := strings.Repeat("g", groupdiscount.MaxUsingGroupLength)
	originModel := strings.Repeat("m", groupdiscount.MaxOriginModelLength)
	firstInput := groupDiscountReserveInput("scope-hash-first", usingGroup, originModel, 100, 100)
	first, err := reserveGroupModelDiscount(db, firstInput)
	require.NoError(t, err)
	usage, err := getUserGroupModelMonthlyUsage(db, firstInput.UserID, usingGroup, originModel, firstInput.Snapshot.PeriodStart)
	require.NoError(t, err)
	assert.Len(t, usage.ScopeHash, 64)

	secondModel := originModel[:len(originModel)-1] + "n"
	secondInput := groupDiscountReserveInput("scope-hash-second", usingGroup, secondModel, 100, 100)
	second, err := reserveGroupModelDiscount(db, secondInput)
	require.NoError(t, err)
	secondUsage, err := getUserGroupModelMonthlyUsage(db, secondInput.UserID, usingGroup, secondModel, secondInput.Snapshot.PeriodStart)
	require.NoError(t, err)
	assert.NotEqual(t, usage.ScopeHash, secondUsage.ScopeHash)
	assert.NotEqual(t, first.Settlement.UsageID, second.Settlement.UsageID)
}

func TestReserveGroupModelDiscountIsIdempotentAndRejectsRequestConflicts(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	input := groupDiscountReserveInput("req-idempotent", "vip", "gpt-5", 100, 300)
	first, err := reserveGroupModelDiscount(db, input)
	require.NoError(t, err)
	second, err := reserveGroupModelDiscount(db, input)
	require.NoError(t, err)
	assert.False(t, first.Reused)
	assert.True(t, second.Reused)
	assert.Equal(t, first.Settlement.Fingerprint, second.Settlement.Fingerprint)
	assert.Equal(t, first.Calculation, second.Calculation)

	conflicting := input
	conflicting.OriginalQuota++
	_, err = reserveGroupModelDiscount(db, conflicting)
	assert.ErrorIs(t, err, ErrGroupModelDiscountRequestConflict)

	var settlementCount int64
	require.NoError(t, db.Model(&GroupModelDiscountSettlement{}).Count(&settlementCount).Error)
	assert.Equal(t, int64(1), settlementCount)
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.OriginalQuota)
}

func TestReserveGroupModelDiscountRejectsPolicyChangeInsideExistingPeriod(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	firstInput := groupDiscountReserveInput("req-policy-v1", "vip", "gpt-5", 100, 300)
	_, err := reserveGroupModelDiscount(db, firstInput)
	require.NoError(t, err)

	changed := groupDiscountReserveInput("req-policy-v2", "vip", "gpt-5", 100, 300)
	changed.Snapshot.PolicyHash = "policy-v2"
	changed.Snapshot.Tiers[1].Ratio = 0.8
	_, err = reserveGroupModelDiscount(db, changed)
	assert.ErrorIs(t, err, ErrGroupModelDiscountPolicyConflict)

	usage, getErr := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, getErr)
	assert.Equal(t, firstInput.Snapshot.PolicyHash, usage.PolicyHash)
	assert.NotEmpty(t, usage.PolicySnapshot)
	assert.Equal(t, int64(300), usage.OriginalQuota)
	var settlementCount int64
	require.NoError(t, db.Model(&GroupModelDiscountSettlement{}).Count(&settlementCount).Error)
	assert.Equal(t, int64(1), settlementCount)
}

func TestReserveGroupModelDiscountConcurrentRequestsPreserveCumulativeCharge(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	snapshot := groupDiscountTestSnapshot("vip", "gpt-5", 100)
	snapshot.Tiers = []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.5}}
	const requestCount = 12

	start := make(chan struct{})
	errs := make(chan error, requestCount)
	var wg sync.WaitGroup
	for index := range requestCount {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			requestID := fmt.Sprintf("req-concurrent-%d", index)
			_, err := reserveGroupModelDiscount(db, GroupModelDiscountReserveInput{
				RequestID:     requestID,
				UserID:        101,
				UsingGroup:    "vip",
				OriginModel:   "gpt-5",
				Snapshot:      snapshot,
				OriginalQuota: 1,
			})
			if err == nil {
				err = commitGroupModelDiscountSettlement(db, requestID)
			}
			errs <- err
		}(index)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(requestCount), usage.OriginalQuota)
	assert.Equal(t, int64(requestCount/2), usage.ChargedQuota, "sum of concurrent request charges must equal Round(C(total))")

	var chargedTotal int64
	require.NoError(t, db.Model(&GroupModelDiscountSettlement{}).Select("COALESCE(SUM(charged_quota), 0)").Scan(&chargedTotal).Error)
	assert.Equal(t, usage.ChargedQuota, chargedTotal)
}

func TestReserveGroupModelDiscountConcurrentSameRequestOnlyAppliesOnce(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	input := groupDiscountReserveInput("req-same", "vip", "gpt-5", 100, 300)
	const goroutines = 12

	start := make(chan struct{})
	results := make(chan GroupModelDiscountReservation, goroutines)
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			reservation, err := reserveGroupModelDiscount(db, input)
			results <- reservation
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	for result := range results {
		assert.Equal(t, 270, result.Calculation.ChargedQuota)
	}

	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.OriginalQuota)
	var settlementCount int64
	require.NoError(t, db.Model(&GroupModelDiscountSettlement{}).Count(&settlementCount).Error)
	assert.Equal(t, int64(1), settlementCount)
}

func TestGroupModelDiscountStateMachineAndReverseCompensatesOnce(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	input := groupDiscountReserveInput("req-state", "vip", "gpt-5", 100, 300)
	_, err := reserveGroupModelDiscount(db, input)
	require.NoError(t, err)

	require.NoError(t, markGroupModelDiscountPendingReconcile(db, input.RequestID, GroupModelDiscountPendingActionCommitAfterFunding))
	settlement, err := getGroupModelDiscountSettlement(db, input.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusPendingReconcile, settlement.Status)

	require.NoError(t, commitGroupModelDiscountSettlement(db, input.RequestID))
	require.NoError(t, commitGroupModelDiscountSettlement(db, input.RequestID), "commit is idempotent")
	settlement, err = getGroupModelDiscountSettlement(db, input.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusSettled, settlement.Status)

	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, input.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, input.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, input.RequestID), "reverse only compensates once")
	settlement, err = getGroupModelDiscountSettlement(db, input.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusReversed, settlement.Status)
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)

	err = commitGroupModelDiscountSettlement(db, input.RequestID)
	assert.ErrorIs(t, err, ErrGroupModelDiscountInvalidTransition)
}

func TestGroupModelDiscountPendingActionGatesAutomaticTransitions(t *testing.T) {
	t.Run("commit after confirmed funding", func(t *testing.T) {
		db := newGroupDiscountFileDB(t)
		input := groupDiscountReserveInput("req-pending-commit", "vip", "gpt-5", 100, 300)
		_, err := reserveGroupModelDiscount(db, input)
		require.NoError(t, err)
		require.NoError(t, markGroupModelDiscountPendingReconcile(db, input.RequestID, GroupModelDiscountPendingActionCommitAfterFunding))

		settlement, err := getGroupModelDiscountSettlement(db, input.RequestID)
		require.NoError(t, err)
		assert.Equal(t, GroupModelDiscountPendingActionCommitAfterFunding, settlement.PendingAction)
		require.NoError(t, commitGroupModelDiscountSettlement(db, input.RequestID))
		settlement, err = getGroupModelDiscountSettlement(db, input.RequestID)
		require.NoError(t, err)
		assert.Empty(t, settlement.PendingAction)
	})

	t.Run("reverse after confirmed refund", func(t *testing.T) {
		db := newGroupDiscountFileDB(t)
		input := groupDiscountReserveInput("req-pending-reverse", "vip", "gpt-5", 100, 300)
		_, err := reserveGroupModelDiscount(db, input)
		require.NoError(t, err)
		require.NoError(t, commitGroupModelDiscountSettlement(db, input.RequestID))
		require.NoError(t, beginGroupModelDiscountSettlementReverse(db, input.RequestID))

		assert.ErrorIs(t, commitGroupModelDiscountSettlement(db, input.RequestID), ErrGroupModelDiscountInvalidTransition)
		require.NoError(t, reverseGroupModelDiscountSettlement(db, input.RequestID))
	})

	t.Run("rollback after proven unfunded result", func(t *testing.T) {
		db := newGroupDiscountFileDB(t)
		input := groupDiscountReserveInput("req-pending-rollback", "vip", "gpt-5", 100, 300)
		_, err := reserveGroupModelDiscount(db, input)
		require.NoError(t, err)
		require.NoError(t, markGroupModelDiscountPendingReconcile(db, input.RequestID, GroupModelDiscountPendingActionRollbackUnfunded))

		assert.ErrorIs(t, commitGroupModelDiscountSettlement(db, input.RequestID), ErrGroupModelDiscountInvalidTransition)
		assert.ErrorIs(t, reverseGroupModelDiscountSettlement(db, input.RequestID), ErrGroupModelDiscountInvalidTransition)
		require.NoError(t, rollbackGroupModelDiscountReservation(db, input.RequestID))
	})

	t.Run("unknown manual state never advances automatically", func(t *testing.T) {
		db := newGroupDiscountFileDB(t)
		input := groupDiscountReserveInput("req-pending-unknown", "vip", "gpt-5", 100, 300)
		_, err := reserveGroupModelDiscount(db, input)
		require.NoError(t, err)
		require.NoError(t, markGroupModelDiscountPendingReconcile(db, input.RequestID, GroupModelDiscountPendingActionUnknownManual))

		assert.ErrorIs(t, commitGroupModelDiscountSettlement(db, input.RequestID), ErrGroupModelDiscountInvalidTransition)
		assert.ErrorIs(t, reverseGroupModelDiscountSettlement(db, input.RequestID), ErrGroupModelDiscountInvalidTransition)
		assert.ErrorIs(t, rollbackGroupModelDiscountReservation(db, input.RequestID), ErrGroupModelDiscountInvalidTransition)
		assert.ErrorIs(t,
			markGroupModelDiscountPendingReconcile(db, input.RequestID, GroupModelDiscountPendingActionCommitAfterFunding),
			ErrGroupModelDiscountInvalidTransition,
		)
	})
}

func TestRefundedSettledGroupModelDiscountCanRemainPendingUntilLedgerReverse(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	input := groupDiscountReserveInput("req-refunded-pending", "vip", "gpt-5", 100, 300)
	_, err := reserveGroupModelDiscount(db, input)
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, input.RequestID))

	// The caller has already completed the exact funding refund, but the first
	// ledger reverse attempt was unavailable. Persisting pending_reconcile makes
	// retries fail closed without issuing a second funding refund.
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, input.RequestID))
	settlement, err := getGroupModelDiscountSettlement(db, input.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusPendingReconcile, settlement.Status)
	require.NoError(t, reverseGroupModelDiscountSettlement(db, input.RequestID))
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
}

func TestReverseSettledGroupModelDiscountRequiresTailAndThenCompensatesInOrder(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	first := groupDiscountReserveInput("req-non-tail-first", "vip", "gpt-5", 100, 900)
	second := groupDiscountReserveInput("req-non-tail-second", "vip", "gpt-5", 100, 300)
	_, err := reserveGroupModelDiscount(db, first)
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, first.RequestID))
	_, err = reserveGroupModelDiscount(db, second)
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, second.RequestID))

	err = beginGroupModelDiscountSettlementReverse(db, first.RequestID)
	assert.ErrorIs(t, err, ErrGroupModelDiscountNonTailReverse)
	usage, getErr := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, getErr)
	assert.Equal(t, int64(1200), usage.OriginalQuota)
	assert.Equal(t, int64(1070), usage.ChargedQuota)
	settlement, getErr := getGroupModelDiscountSettlement(db, first.RequestID)
	require.NoError(t, getErr)
	assert.Equal(t, GroupModelDiscountStatusSettled, settlement.Status)

	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, second.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, second.RequestID))
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, first.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, first.RequestID))
	usage, getErr = getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, getErr)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
}

func TestReverseSettledGroupModelDiscountRejectsOtherUnresolvedSettlementInScope(t *testing.T) {
	for _, test := range []struct {
		name        string
		makePending bool
		wantErr     error
	}{
		{name: "reserved", wantErr: ErrGroupModelDiscountScopeBusy},
		{name: "pending reconcile", makePending: true, wantErr: ErrGroupModelDiscountPendingReconcile},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := newGroupDiscountFileDB(t)
			first := groupDiscountReserveInput("req-reverse-with-unresolved-a", "vip", "gpt-5", 100, 300)
			second := groupDiscountReserveInput("req-reverse-with-unresolved-b", "vip", "gpt-5", 100, 300)
			_, err := reserveGroupModelDiscount(db, first)
			require.NoError(t, err)
			require.NoError(t, commitGroupModelDiscountSettlement(db, first.RequestID))
			_, err = reserveGroupModelDiscount(db, second)
			require.NoError(t, err)
			if test.makePending {
				require.NoError(t, markGroupModelDiscountPendingReconcile(
					db,
					second.RequestID,
					GroupModelDiscountPendingActionUnknownManual,
				))
			}

			err = beginGroupModelDiscountSettlementReverse(db, first.RequestID)

			assert.ErrorIs(t, err, test.wantErr)
			settlement, getErr := getGroupModelDiscountSettlement(db, first.RequestID)
			require.NoError(t, getErr)
			assert.Equal(t, GroupModelDiscountStatusSettled, settlement.Status)
			usage, getErr := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
			require.NoError(t, getErr)
			assert.EqualValues(t, 600, usage.OriginalQuota)
			assert.EqualValues(t, 540, usage.ChargedQuota)
		})
	}
}

func TestReserveGroupModelDiscountBlocksOnUnresolvedSettlement(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	first := groupDiscountReserveInput("req-unresolved-first", "vip", "gpt-5", 100, 300)
	second := groupDiscountReserveInput("req-unresolved-second", "vip", "gpt-5", 100, 300)
	_, err := reserveGroupModelDiscount(db, first)
	require.NoError(t, err)

	_, err = reserveGroupModelDiscount(db, second)
	assert.ErrorIs(t, err, ErrGroupModelDiscountScopeBusy)
	usage, getErr := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, getErr)
	assert.Equal(t, int64(300), usage.OriginalQuota)

	require.NoError(t, commitGroupModelDiscountSettlement(db, first.RequestID))
	_, err = reserveGroupModelDiscount(db, second)
	require.NoError(t, err)
}

func TestReserveGroupModelDiscountFailsClosedOnPendingReconcile(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	first := groupDiscountReserveInput("req-pending-first", "vip", "gpt-5", 100, 300)
	second := groupDiscountReserveInput("req-pending-second", "vip", "gpt-5", 100, 300)
	_, err := reserveGroupModelDiscount(db, first)
	require.NoError(t, err)
	require.NoError(t, markGroupModelDiscountPendingReconcile(db, first.RequestID, GroupModelDiscountPendingActionUnknownManual))

	_, err = reserveGroupModelDiscount(db, second)
	assert.ErrorIs(t, err, ErrGroupModelDiscountPendingReconcile)
}

func TestRollbackGroupModelDiscountReservationRequiresTailAndCompensatesOnce(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	input := groupDiscountReserveInput("req-rollback", "vip", "gpt-5", 100, 300)
	_, err := reserveGroupModelDiscount(db, input)
	require.NoError(t, err)
	require.NoError(t, rollbackGroupModelDiscountReservation(db, input.RequestID))
	require.NoError(t, rollbackGroupModelDiscountReservation(db, input.RequestID))

	usage, getErr := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, getErr)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
	settlement, getErr := getGroupModelDiscountSettlement(db, input.RequestID)
	require.NoError(t, getErr)
	assert.Equal(t, GroupModelDiscountStatusReversed, settlement.Status)
}

func TestReverseGroupModelDiscountRejectsCorruptAggregateInsteadOfGoingNegative(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	input := groupDiscountReserveInput("req-corrupt", "vip", "gpt-5", 100, 300)
	reservation, err := reserveGroupModelDiscount(db, input)
	require.NoError(t, err)
	require.NoError(t, db.Model(&UserGroupModelMonthlyUsage{}).
		Where("id = ?", reservation.Settlement.UsageID).
		Updates(map[string]any{"original_quota": 1, "charged_quota": 1}).Error)

	err = rollbackGroupModelDiscountReservation(db, input.RequestID)
	assert.ErrorIs(t, err, ErrGroupModelDiscountAggregateCorrupt)
	settlement, getErr := getGroupModelDiscountSettlement(db, input.RequestID)
	require.NoError(t, getErr)
	assert.Equal(t, GroupModelDiscountStatusReserved, settlement.Status)
}

func TestReserveGroupModelDiscountValidatesOriginalQuotaAndSnapshotScope(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	input := groupDiscountReserveInput("req-invalid", "vip", "gpt-5", 100, 1)
	input.OriginalQuota = 0
	_, err := reserveGroupModelDiscount(db, input)
	assert.ErrorIs(t, err, groupdiscount.ErrInvalidOriginalQuota)

	input = groupDiscountReserveInput("req-invalid-scope", "vip", "gpt-5", 100, 1)
	input.Snapshot.OriginModel = "different"
	_, err = reserveGroupModelDiscount(db, input)
	assert.True(t, errors.Is(err, ErrGroupModelDiscountInvalidInput))
}

func TestValidateOptionValueProtectsModelTieredRatioConfiguration(t *testing.T) {
	valid := `{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	require.NoError(t, validateOptionValue("group_ratio_setting.model_tiered_ratios", valid))

	missingRatio := `{"vip":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0}]}}}`
	assert.Error(t, validateOptionValue("group_ratio_setting.model_tiered_ratios", missingRatio))
}
