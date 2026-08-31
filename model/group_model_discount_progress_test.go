package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chargedProgressReserveInput(requestID string, original int) GroupModelDiscountReserveInput {
	return GroupModelDiscountReserveInput{
		RequestID:   requestID,
		UserID:      101,
		UsingGroup:  "charged-vip",
		OriginModel: "gpt-5",
		Snapshot: groupdiscount.Snapshot{
			PolicyHash:    "charged-policy-v1",
			ProgressBasis: groupdiscount.ProgressBasisCharged,
			UsingGroup:    "charged-vip",
			OriginModel:   "gpt-5",
			MatchedModel:  "gpt-5",
			Timezone:      "UTC",
			PeriodStart:   100,
			PeriodEnd:     200,
			Tiers: []groupdiscount.Tier{
				{MinMonthlyOriginalQuota: 0, Ratio: 0.8},
				{MinMonthlyOriginalQuota: 16, Ratio: 0.5},
			},
		},
		OriginalQuota: original,
	}
}

func TestChargedProgressLedgerPersistsExactCursorAndReplaysSettlement(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	first, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("charged-first", 20))
	require.NoError(t, err)
	assert.Equal(t, 16, first.Calculation.ChargedQuota)
	assert.Equal(t, groupdiscount.ProgressBasisCharged, first.Settlement.ProgressBasis)
	assert.Equal(t, "16", first.Settlement.MonthlyProgressAfter)
	assert.Equal(t, "16", first.Settlement.ProgressQuota)
	require.NoError(t, commitGroupModelDiscountSettlement(db, first.Settlement.RequestID))

	second, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("charged-second", 8))
	require.NoError(t, err)
	assert.Equal(t, 4, second.Calculation.ChargedQuota)
	assert.Equal(t, "16", second.Calculation.MonthlyProgressBefore)
	assert.Equal(t, "20", second.Calculation.MonthlyProgressAfter)
	replayed, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("charged-second", 8))
	require.NoError(t, err)
	assert.True(t, replayed.Reused)
	assert.Equal(t, second.Calculation, replayed.Calculation)

	usage, err := getUserGroupModelMonthlyUsage(db, 101, "charged-vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(28), usage.OriginalQuota)
	assert.Equal(t, int64(20), usage.ChargedQuota)
	assert.Equal(t, "20", usage.ProgressQuota)
	assert.Equal(t, groupdiscount.ProgressBasisCharged, usage.ProgressBasis)
}

func TestChargedProgressReverseRequiresTailAndRestoresExactCursorInOrder(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	first, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("charged-reverse-first", 20))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, first.Settlement.RequestID))
	second, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("charged-reverse-second", 8))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, second.Settlement.RequestID))

	assert.ErrorIs(t, beginGroupModelDiscountSettlementReverse(db, first.Settlement.RequestID), ErrGroupModelDiscountNonTailReverse)
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, second.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, second.Settlement.RequestID))
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "charged-vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, int64(20), usage.OriginalQuota)
	assert.Equal(t, int64(16), usage.ChargedQuota)
	assert.Equal(t, "16", usage.ProgressQuota)
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, first.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, first.Settlement.RequestID))
}

func TestChargedProgressAdjustmentAndRollbackRestoreExactEvidence(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	parent, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("charged-adjust-parent", 20))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, parent.Settlement.RequestID))

	positive, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "charged-adjust-parent-up",
		SettlementRequestID: parent.Settlement.RequestID,
		NewOriginalQuota:    28,
	})
	require.NoError(t, err)
	assert.Equal(t, 4, positive.DeltaChargedQuota)
	assert.Equal(t, "4", positive.Adjustment.DeltaProgressQuota)
	assert.Equal(t, "20", positive.Adjustment.UsageProgressAfter)
	require.NoError(t, commitGroupModelDiscountAdjustment(db, positive.Adjustment.AdjustmentID))

	negative, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "charged-adjust-parent-down",
		SettlementRequestID: parent.Settlement.RequestID,
		NewOriginalQuota:    20,
	})
	require.NoError(t, err)
	assert.Equal(t, -4, negative.DeltaChargedQuota)
	assert.Equal(t, "-4", negative.Adjustment.DeltaProgressQuota)
	assert.Equal(t, "16", negative.Adjustment.UsageProgressAfter)

	require.NoError(t, rollbackGroupModelDiscountAdjustment(db, negative.Adjustment.AdjustmentID))
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "charged-vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Equal(t, "20", usage.ProgressQuota)
	settlement, err := getGroupModelDiscountSettlement(db, parent.Settlement.RequestID)
	require.NoError(t, err)
	assert.Equal(t, "20", settlement.CurrentProgressQuota)
}

func TestChargedProgressRecurringBoundaryAdjustmentAndRollbackUseCanonicalCursor(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	input := chargedProgressReserveInput("charged-recurring-adjustment", 11)
	input.Snapshot.PolicyHash = "charged-recurring-policy-v1"
	input.Snapshot.Tiers = []groupdiscount.Tier{
		{MinMonthlyOriginalQuota: 0, Ratio: 0.3},
		{MinMonthlyOriginalQuota: 1, Ratio: 0.2},
	}
	parent, err := reserveGroupModelDiscount(db, input)
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, parent.Settlement.RequestID))
	assert.Equal(t, "2.53333333333333334", parent.Settlement.MonthlyProgressAfter)

	negative, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "charged-recurring-adjustment-down",
		SettlementRequestID: parent.Settlement.RequestID,
		NewOriginalQuota:    1,
	})
	require.NoError(t, err)
	assert.Equal(t, "0.3", negative.Adjustment.UsageProgressAfter)
	assert.Equal(t, "-2.23333333333333334", negative.Adjustment.DeltaProgressQuota)
	usage, err := getUserGroupModelMonthlyUsage(db, input.UserID, input.UsingGroup, input.OriginModel, input.Snapshot.PeriodStart)
	require.NoError(t, err)
	assert.Equal(t, "0.3", usage.ProgressQuota)

	require.NoError(t, rollbackGroupModelDiscountAdjustment(db, negative.Adjustment.AdjustmentID))
	usage, err = getUserGroupModelMonthlyUsage(db, input.UserID, input.UsingGroup, input.OriginModel, input.Snapshot.PeriodStart)
	require.NoError(t, err)
	assert.Equal(t, "2.53333333333333334", usage.ProgressQuota)
	settlement, err := getGroupModelDiscountSettlement(db, parent.Settlement.RequestID)
	require.NoError(t, err)
	assert.Equal(t, "2.53333333333333334", settlement.CurrentProgressQuota)
}

func TestChargedProgressReservationRollbackRestoresExactCursor(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	reservation, err := reserveGroupModelDiscount(db, chargedProgressReserveInput("charged-reservation-rollback", 20))
	require.NoError(t, err)
	assert.Equal(t, "16", reservation.Calculation.MonthlyProgressAfter)

	require.NoError(t, rollbackGroupModelDiscountReservation(db, reservation.Settlement.RequestID))
	usage, err := getUserGroupModelMonthlyUsage(db, 101, "charged-vip", "gpt-5", 100)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
	assert.Equal(t, "0", usage.ProgressQuota)
}

func TestConcurrentChargedProgressSettlementsConserveExactAndIntegerTotals(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	const requestCount = 12
	input := chargedProgressReserveInput("", 1)
	input.OriginModel = "charged-concurrent-model"
	input.Snapshot.OriginModel = input.OriginModel
	input.Snapshot.MatchedModel = input.OriginModel
	input.Snapshot.PolicyHash = "charged-concurrent-policy"
	input.Snapshot.Tiers = []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.5}}

	start := make(chan struct{})
	errs := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	for index := range requestCount {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			request := input
			request.RequestID = fmt.Sprintf("charged-concurrent-%d", index)
			_, err := reserveGroupModelDiscount(db, request)
			if err == nil {
				err = commitGroupModelDiscountSettlement(db, request.RequestID)
			}
			errs <- err
		}(index)
	}
	close(start)
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	usage, err := getUserGroupModelMonthlyUsage(db, input.UserID, input.UsingGroup, input.OriginModel, input.Snapshot.PeriodStart)
	require.NoError(t, err)
	assert.Equal(t, int64(requestCount), usage.OriginalQuota)
	assert.Equal(t, int64(requestCount/2), usage.ChargedQuota)
	assert.Equal(t, "6", usage.ProgressQuota)
}
