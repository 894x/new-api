package model

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReserveGroupModelDiscountAdjustmentIncreaseUsesCurrentCursor(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	initial, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-increase", "vip", "gpt-5", 100, 900))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-increase"))

	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-increase-final",
		SettlementRequestID: "task-increase",
		NewOriginalQuota:    1200,
	})
	require.NoError(t, err)
	assert.False(t, adjustment.Reused)
	assert.Equal(t, 900, adjustment.PreviousOriginalQuota)
	assert.Equal(t, 810, adjustment.PreviousChargedQuota)
	assert.Equal(t, 1200, adjustment.NewOriginalQuota)
	assert.Equal(t, 1070, adjustment.NewChargedQuota)
	assert.Equal(t, 300, adjustment.DeltaOriginalQuota)
	assert.Equal(t, 260, adjustment.DeltaChargedQuota)
	assert.Equal(t, initial.Settlement.OriginalQuota, int64(900), "initial settlement audit fields stay immutable")

	settlement, err := getGroupModelDiscountSettlement(db, "task-increase")
	require.NoError(t, err)
	assert.Equal(t, int64(900), settlement.OriginalQuota)
	assert.Equal(t, int64(810), settlement.ChargedQuota)
	assert.Equal(t, int64(1200), settlement.CurrentOriginalQuota)
	assert.Equal(t, int64(1070), settlement.CurrentChargedQuota)
	assert.Equal(t, int64(1), settlement.AdjustmentRevision)
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1200), usage.OriginalQuota)
	assert.Equal(t, int64(1070), usage.ChargedQuota)
}

func TestReserveGroupModelDiscountAdjustmentDecreaseUsesCurrentMarginalWithoutRepricingLaterRequests(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-decrease", "vip", "gpt-5", 100, 900))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-decrease"))
	_, err = reserveGroupModelDiscount(db, groupDiscountReserveInput("later-request", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "later-request"))

	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-decrease-final",
		SettlementRequestID: "task-decrease",
		NewOriginalQuota:    800,
	})
	require.NoError(t, err)
	assert.Equal(t, -100, adjustment.DeltaOriginalQuota)
	assert.Equal(t, -85, adjustment.DeltaChargedQuota)
	assert.Equal(t, 800, adjustment.NewOriginalQuota)
	assert.Equal(t, 725, adjustment.NewChargedQuota)

	later, err := getGroupModelDiscountSettlement(db, "later-request")
	require.NoError(t, err)
	assert.Equal(t, int64(300), later.CurrentOriginalQuota)
	assert.Equal(t, int64(260), later.CurrentChargedQuota)
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1100), usage.OriginalQuota)
	assert.Equal(t, int64(985), usage.ChargedQuota)
}

func TestReserveGroupModelDiscountAdjustmentIsIdempotentAndConflictsOnDifferentTarget(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-idempotent", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-idempotent"))
	input := GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-idempotent-final",
		SettlementRequestID: "task-idempotent",
		NewOriginalQuota:    500,
	}
	first, err := reserveGroupModelDiscountAdjustment(db, input)
	require.NoError(t, err)
	second, err := reserveGroupModelDiscountAdjustment(db, input)
	require.NoError(t, err)
	assert.False(t, first.Reused)
	assert.True(t, second.Reused)
	assert.Equal(t, first.DeltaChargedQuota, second.DeltaChargedQuota)

	input.NewOriginalQuota = 501
	_, err = reserveGroupModelDiscountAdjustment(db, input)
	assert.ErrorIs(t, err, ErrGroupModelDiscountAdjustmentConflict)
}

func TestReserveGroupModelDiscountAdjustmentRejectsZeroForExplicitFullReverse(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-zero", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-zero"))

	_, err = reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-zero-final",
		SettlementRequestID: "task-zero",
		NewOriginalQuota:    0,
	})
	assert.ErrorIs(t, err, ErrGroupModelDiscountAdjustmentRequiresFullReverse)
}

func TestUnresolvedAdjustmentBlocksScopeUntilCommit(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-block", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-block"))
	_, err = reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-block-final",
		SettlementRequestID: "task-block",
		NewOriginalQuota:    500,
	})
	require.NoError(t, err)

	_, err = reserveGroupModelDiscount(db, groupDiscountReserveInput("blocked-request", "vip", "gpt-5", 100, 100))
	assert.ErrorIs(t, err, ErrGroupModelDiscountScopeBusy)
	require.NoError(t, commitGroupModelDiscountAdjustment(db, "task-block-final"))
	_, err = reserveGroupModelDiscount(db, groupDiscountReserveInput("blocked-request", "vip", "gpt-5", 100, 100))
	require.NoError(t, err)
}

func TestPendingAdjustmentFailsClosedForScope(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-pending-adjustment", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-pending-adjustment"))
	_, err = reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-pending-adjustment-final",
		SettlementRequestID: "task-pending-adjustment",
		NewOriginalQuota:    500,
	})
	require.NoError(t, err)
	require.NoError(t, markGroupModelDiscountAdjustmentPendingReconcile(
		db,
		"task-pending-adjustment-final",
		GroupModelDiscountPendingActionUnknownManual,
	))

	_, err = reserveGroupModelDiscount(db, groupDiscountReserveInput("blocked-by-pending-adjustment", "vip", "gpt-5", 100, 100))
	assert.ErrorIs(t, err, ErrGroupModelDiscountPendingReconcile)
}

func TestReverseGroupModelDiscountRejectsUnresolvedAdjustmentInScope(t *testing.T) {
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
			requestID := "task-reverse-unresolved-adjustment"
			adjustmentID := requestID + "-final"
			_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput(requestID, "vip", "gpt-5", 100, 300))
			require.NoError(t, err)
			require.NoError(t, commitGroupModelDiscountSettlement(db, requestID))
			_, err = reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
				AdjustmentID:        adjustmentID,
				SettlementRequestID: requestID,
				NewOriginalQuota:    500,
			})
			require.NoError(t, err)
			if test.makePending {
				require.NoError(t, markGroupModelDiscountAdjustmentPendingReconcile(
					db,
					adjustmentID,
					GroupModelDiscountPendingActionUnknownManual,
				))
			}

			err = beginGroupModelDiscountSettlementReverse(db, requestID)

			assert.ErrorIs(t, err, test.wantErr)
			settlement, getErr := getGroupModelDiscountSettlement(db, requestID)
			require.NoError(t, getErr)
			assert.Equal(t, GroupModelDiscountStatusSettled, settlement.Status)
		})
	}
}

func TestGroupModelDiscountAdjustmentPendingActionGatesAutomaticTransitions(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		wantCommit bool
		wantRoll   bool
	}{
		{name: "commit after funding", action: GroupModelDiscountPendingActionCommitAfterFunding, wantCommit: true},
		{name: "rollback unfunded", action: GroupModelDiscountPendingActionRollbackUnfunded, wantRoll: true},
		{name: "unknown manual", action: GroupModelDiscountPendingActionUnknownManual},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newGroupDiscountFileDB(t)
			requestID := "task-adjustment-pending-" + test.name
			adjustmentID := requestID + "-final"
			_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput(requestID, "vip", "gpt-5", 100, 300))
			require.NoError(t, err)
			require.NoError(t, commitGroupModelDiscountSettlement(db, requestID))
			_, err = reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
				AdjustmentID:        adjustmentID,
				SettlementRequestID: requestID,
				NewOriginalQuota:    500,
			})
			require.NoError(t, err)
			require.NoError(t, markGroupModelDiscountAdjustmentPendingReconcile(db, adjustmentID, test.action))

			adjustment, err := getGroupModelDiscountAdjustment(db, adjustmentID)
			require.NoError(t, err)
			assert.Equal(t, test.action, adjustment.PendingAction)
			commitErr := commitGroupModelDiscountAdjustment(db, adjustmentID)
			rollbackErr := rollbackGroupModelDiscountAdjustment(db, adjustmentID)
			if test.wantCommit {
				require.NoError(t, commitErr)
				assert.ErrorIs(t, rollbackErr, ErrGroupModelDiscountInvalidTransition)
				return
			}
			assert.ErrorIs(t, commitErr, ErrGroupModelDiscountInvalidTransition)
			if test.wantRoll {
				require.NoError(t, rollbackErr)
			} else {
				assert.ErrorIs(t, rollbackErr, ErrGroupModelDiscountInvalidTransition)
			}
		})
	}
}

func TestRollbackGroupModelDiscountAdjustmentRestoresParentAndAggregateOnce(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-rollback-adjustment", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-rollback-adjustment"))
	_, err = reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-rollback-adjustment-final",
		SettlementRequestID: "task-rollback-adjustment",
		NewOriginalQuota:    500,
	})
	require.NoError(t, err)

	require.NoError(t, rollbackGroupModelDiscountAdjustment(db, "task-rollback-adjustment-final"))
	require.NoError(t, rollbackGroupModelDiscountAdjustment(db, "task-rollback-adjustment-final"))
	settlement, err := getGroupModelDiscountSettlement(db, "task-rollback-adjustment")
	require.NoError(t, err)
	assert.Equal(t, int64(300), settlement.CurrentOriginalQuota)
	assert.Equal(t, int64(270), settlement.CurrentChargedQuota)
	assert.Equal(t, int64(2), settlement.AdjustmentRevision, "adjustment revision is monotonic even when values roll back")
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.OriginalQuota)
	assert.Equal(t, int64(270), usage.ChargedQuota)
	adjustment, err := getGroupModelDiscountAdjustment(db, "task-rollback-adjustment-final")
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusReversed, adjustment.Status)
}

func TestReverseGroupModelDiscountUsesCurrentAdjustedTotals(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-reverse-adjusted", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-reverse-adjusted"))
	_, err = reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-reverse-adjusted-final",
		SettlementRequestID: "task-reverse-adjusted",
		NewOriginalQuota:    500,
	})
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountAdjustment(db, "task-reverse-adjusted-final"))

	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, "task-reverse-adjusted"))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, "task-reverse-adjusted"))
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
}

func TestConcurrentSameGroupModelDiscountAdjustmentAppliesOnce(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	_, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("task-concurrent-adjustment", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, "task-concurrent-adjustment"))
	input := GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "task-concurrent-adjustment-final",
		SettlementRequestID: "task-concurrent-adjustment",
		NewOriginalQuota:    500,
	}

	const goroutines = 12
	start := make(chan struct{})
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, adjustmentErr := reserveGroupModelDiscountAdjustment(db, input)
			errs <- adjustmentErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	settlement, err := getGroupModelDiscountSettlement(db, input.SettlementRequestID)
	require.NoError(t, err)
	assert.Equal(t, int64(500), settlement.CurrentOriginalQuota)
	assert.Equal(t, int64(450), settlement.CurrentChargedQuota)
	var adjustmentCount int64
	require.NoError(t, db.Model(&GroupModelDiscountAdjustment{}).Count(&adjustmentCount).Error)
	assert.Equal(t, int64(1), adjustmentCount)
}
