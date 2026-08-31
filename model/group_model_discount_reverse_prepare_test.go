package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeginGroupModelDiscountSettlementReverseRejectsNonTailBeforeFunding(t *testing.T) {
	tests := []struct {
		name  string
		input func(string, int) GroupModelDiscountReserveInput
	}{
		{
			name: "original progress",
			input: func(requestID string, original int) GroupModelDiscountReserveInput {
				return groupDiscountReserveInput(requestID, "reverse-original", "gpt-5", 100, original)
			},
		},
		{
			name:  "charged progress",
			input: chargedProgressReserveInput,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newGroupDiscountFileDB(t)
			first, err := reserveGroupModelDiscount(db, test.input("reverse-first", 20))
			require.NoError(t, err)
			require.NoError(t, commitGroupModelDiscountSettlement(db, first.Settlement.RequestID))
			second, err := reserveGroupModelDiscount(db, test.input("reverse-second", 8))
			require.NoError(t, err)
			require.NoError(t, commitGroupModelDiscountSettlement(db, second.Settlement.RequestID))

			err = beginGroupModelDiscountSettlementReverse(db, first.Settlement.RequestID)

			assert.ErrorIs(t, err, ErrGroupModelDiscountNonTailReverse)
			settlement, getErr := getGroupModelDiscountSettlement(db, first.Settlement.RequestID)
			require.NoError(t, getErr)
			assert.Equal(t, GroupModelDiscountStatusSettled, settlement.Status)
			assert.Empty(t, settlement.PendingAction)
			usage, getErr := getUserGroupModelMonthlyUsage(
				db,
				first.Settlement.UserID,
				first.Settlement.UsingGroup,
				first.Settlement.OriginModel,
				first.Settlement.PeriodStart,
			)
			require.NoError(t, getErr)
			assert.Equal(t, int64(28), usage.OriginalQuota)
		})
	}
}

func TestPreparedGroupModelDiscountReverseBlocksScopeAndKnownFundingFailureCancels(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	reservation, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("reverse-cancel", 20))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, reservation.Settlement.RequestID))

	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID))
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID), "begin replay is idempotent")
	prepared, err := getGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusPendingReconcile, prepared.Status)
	assert.Equal(t, GroupModelDiscountPendingActionReverseAfterRefund, prepared.PendingAction)

	_, err = reserveGroupModelDiscount(db, chargedProgressReserveInput("reverse-blocked-new", 1))
	assert.ErrorIs(t, err, ErrGroupModelDiscountPendingReconcile)
	_, err = reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "reverse-blocked-adjustment",
		SettlementRequestID: reservation.Settlement.RequestID,
		NewOriginalQuota:    21,
	})
	assert.ErrorIs(t, err, ErrGroupModelDiscountInvalidTransition)

	require.NoError(t, cancelGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID))
	require.NoError(t, cancelGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID), "cancel replay is idempotent")
	cancelled, err := getGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusSettled, cancelled.Status)
	assert.Empty(t, cancelled.PendingAction)

	newReservation, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("reverse-after-cancel", 1))
	require.NoError(t, err)
	assert.Equal(t, "16.5", newReservation.Calculation.MonthlyProgressAfter)
}

func TestPreparedGroupModelDiscountReverseConfirmsOnlyAfterBegin(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	reservation, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("reverse-confirm", 20))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, reservation.Settlement.RequestID))

	err = reverseGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	assert.ErrorIs(t, err, ErrGroupModelDiscountInvalidTransition, "funding-success confirmation requires a durable prepare")

	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, reservation.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, reservation.Settlement.RequestID), "confirmed replay is idempotent")
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "charged-vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
	assert.Equal(t, "0", usage.ProgressQuota)
}

func TestUnknownGroupModelDiscountReverseFundingStaysManualAndBlocksAutomaticTransitions(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	reservation, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("reverse-unknown", 20))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, reservation.Settlement.RequestID))
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID))

	require.NoError(t, markGroupModelDiscountSettlementReverseFundingUnknown(db, reservation.Settlement.RequestID))
	require.NoError(t, markGroupModelDiscountSettlementReverseFundingUnknown(db, reservation.Settlement.RequestID), "unknown replay is idempotent")
	settlement, err := getGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, GroupModelDiscountPendingActionUnknownManual, settlement.PendingAction)
	assert.ErrorIs(t, cancelGroupModelDiscountSettlementReverse(db, settlement.RequestID), ErrGroupModelDiscountInvalidTransition)
	assert.ErrorIs(t, reverseGroupModelDiscountSettlement(db, settlement.RequestID), ErrGroupModelDiscountInvalidTransition)
	_, err = reserveGroupModelDiscount(db, chargedProgressReserveInput("reverse-unknown-blocked", 1))
	assert.ErrorIs(t, err, ErrGroupModelDiscountPendingReconcile)
}

func TestAdjustedTailSettlementCanPrepareAndConfirmReverse(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	reservation, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("reverse-adjusted-tail", 20))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, reservation.Settlement.RequestID))
	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "reverse-adjusted-tail-final",
		SettlementRequestID: reservation.Settlement.RequestID,
		NewOriginalQuota:    28,
	})
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountAdjustment(db, adjustment.Adjustment.AdjustmentID))

	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, reservation.Settlement.RequestID))
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "charged-vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
	assert.Equal(t, "0", usage.ProgressQuota)
}

func TestLatestAdjustmentOwnerCanReverseAndLeavesOtherSettlementAdjustable(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	first, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("reverse-owner-first", "vip", "gpt-5", 100, 900))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, first.Settlement.RequestID))
	second, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("reverse-owner-second", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, second.Settlement.RequestID))
	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "reverse-owner-first-adjustment",
		SettlementRequestID: first.Settlement.RequestID,
		NewOriginalQuota:    800,
	})
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountAdjustment(db, adjustment.Adjustment.AdjustmentID))

	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, first.Settlement.RequestID),
		"the parent of the latest settled cursor adjustment is the safe reverse owner")
	require.NoError(t, reverseGroupModelDiscountSettlement(db, first.Settlement.RequestID))
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.OriginalQuota)
	assert.Equal(t, int64(260), usage.ChargedQuota)

	secondAdjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "reverse-owner-second-adjustment",
		SettlementRequestID: second.Settlement.RequestID,
		NewOriginalQuota:    400,
	})
	require.NoError(t, err)
	assert.Equal(t, 90, secondAdjustment.DeltaChargedQuota)
}

func TestRolledBackAdjustmentRestoresPreviousTailOwner(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	first, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("rollback-owner-first", "vip", "gpt-5", 100, 900))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, first.Settlement.RequestID))
	second, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("rollback-owner-second", "vip", "gpt-5", 100, 300))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, second.Settlement.RequestID))
	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "rollback-owner-first-adjustment",
		SettlementRequestID: first.Settlement.RequestID,
		NewOriginalQuota:    800,
	})
	require.NoError(t, err)
	require.NoError(t, rollbackGroupModelDiscountAdjustment(db, adjustment.Adjustment.AdjustmentID))

	beginFirstErr := beginGroupModelDiscountSettlementReverse(db, first.Settlement.RequestID)
	if beginFirstErr == nil {
		require.NoError(t, cancelGroupModelDiscountSettlementReverse(db, first.Settlement.RequestID))
	}
	assert.ErrorIs(t, beginFirstErr, ErrGroupModelDiscountNonTailReverse)
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, second.Settlement.RequestID))
}
