package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedGroupModelDiscountAccountingTargets(t *testing.T, db *gorm.DB) (User, Channel) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&User{}, &Channel{}))
	user := User{Id: 101, Username: "tiered-accounting-user", Password: "password", UsedQuota: 10, RequestCount: 2}
	channel := Channel{Id: 201, Name: "tiered-accounting-channel", Key: "sk-tiered-accounting", UsedQuota: 10}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&channel).Error)
	return user, channel
}

func loadGroupModelDiscountAccountingTargets(t *testing.T, db *gorm.DB, userID, channelID int) (User, Channel) {
	t.Helper()
	var user User
	require.NoError(t, db.Select("id", "used_quota", "request_count").First(&user, userID).Error)
	var channel Channel
	require.NoError(t, db.Select("id", "used_quota").First(&channel, channelID).Error)
	return user, channel
}

func TestCommitGroupModelDiscountSettlementWithUsageIsAtomicAndReplaySafe(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	user, channel := seedGroupModelDiscountAccountingTargets(t, db)
	reservation, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("accounting-settlement", "vip", "gpt-5", 100, 100))
	require.NoError(t, err)
	delta := BillingUsageDelta{
		UserID:            user.Id,
		ChannelID:         channel.Id,
		QuotaDelta:        reservation.Calculation.ChargedQuota,
		RequestCountDelta: 1,
	}

	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, delta))
	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, delta))
	storedUser, storedChannel := loadGroupModelDiscountAccountingTargets(t, db, user.Id, channel.Id)
	assert.Equal(t, 10+reservation.Calculation.ChargedQuota, storedUser.UsedQuota)
	assert.Equal(t, 3, storedUser.RequestCount)
	assert.Equal(t, int64(10+reservation.Calculation.ChargedQuota), storedChannel.UsedQuota)
	settlement, err := getGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	require.NoError(t, err)
	assert.True(t, settlement.AccountingApplied)
	assert.Equal(t, channel.Id, settlement.AccountingChannelID)

	conflicting := delta
	conflicting.QuotaDelta++
	assert.ErrorIs(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, conflicting), ErrGroupModelDiscountAccountingConflict)
	storedUser, storedChannel = loadGroupModelDiscountAccountingTargets(t, db, user.Id, channel.Id)
	assert.Equal(t, 10+reservation.Calculation.ChargedQuota, storedUser.UsedQuota)
	assert.Equal(t, int64(10+reservation.Calculation.ChargedQuota), storedChannel.UsedQuota)
}

func TestCommitGroupModelDiscountSettlementWithUsageRollsBackAccountingOnTargetFailure(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	user, channel := seedGroupModelDiscountAccountingTargets(t, db)
	reservation, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("accounting-settlement-fail", "vip", "gpt-5", 100, 100))
	require.NoError(t, err)

	err = commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, BillingUsageDelta{
		UserID:            user.Id,
		ChannelID:         channel.Id + 999,
		QuotaDelta:        reservation.Calculation.ChargedQuota,
		RequestCountDelta: 1,
	})
	assert.ErrorIs(t, err, ErrBillingUsageTargetNotFound)
	storedUser, storedChannel := loadGroupModelDiscountAccountingTargets(t, db, user.Id, channel.Id)
	assert.Equal(t, 10, storedUser.UsedQuota)
	assert.Equal(t, 2, storedUser.RequestCount)
	assert.Equal(t, int64(10), storedChannel.UsedQuota)
	settlement, err := getGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusReserved, settlement.Status)
	assert.False(t, settlement.AccountingApplied)
}

func TestSettledWithoutAccountingEvidenceCannotApplyUsageOnReplay(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	user, channel := seedGroupModelDiscountAccountingTargets(t, db)
	reservation, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("accounting-legacy-settled", "vip", "gpt-5", 100, 100))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlement(db, reservation.Settlement.RequestID))

	err = commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: reservation.Calculation.ChargedQuota, RequestCountDelta: 1,
	})
	assert.ErrorIs(t, err, ErrGroupModelDiscountAccountingConflict)
	storedUser, storedChannel := loadGroupModelDiscountAccountingTargets(t, db, user.Id, channel.Id)
	assert.Equal(t, 10, storedUser.UsedQuota)
	assert.Equal(t, 2, storedUser.RequestCount)
	assert.Equal(t, int64(10), storedChannel.UsedQuota)
}

func TestCommitGroupModelDiscountAdjustmentWithUsageIsAtomicAndReplaySafe(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	user, channel := seedGroupModelDiscountAccountingTargets(t, db)
	reservation, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("accounting-adjustment-parent", "vip", "gpt-5", 100, 100))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: reservation.Calculation.ChargedQuota, RequestCountDelta: 1,
	}))
	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "accounting-adjustment",
		SettlementRequestID: reservation.Settlement.RequestID,
		NewOriginalQuota:    200,
	})
	require.NoError(t, err)
	delta := BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: adjustment.DeltaChargedQuota,
	}

	require.NoError(t, commitGroupModelDiscountAdjustmentWithUsage(db, adjustment.Adjustment.AdjustmentID, delta))
	require.NoError(t, commitGroupModelDiscountAdjustmentWithUsage(db, adjustment.Adjustment.AdjustmentID, delta))
	storedUser, storedChannel := loadGroupModelDiscountAccountingTargets(t, db, user.Id, channel.Id)
	expectedUsed := 10 + reservation.Calculation.ChargedQuota + adjustment.DeltaChargedQuota
	assert.Equal(t, expectedUsed, storedUser.UsedQuota)
	assert.Equal(t, 3, storedUser.RequestCount)
	assert.Equal(t, int64(expectedUsed), storedChannel.UsedQuota)
	storedAdjustment, err := getGroupModelDiscountAdjustment(db, adjustment.Adjustment.AdjustmentID)
	require.NoError(t, err)
	assert.True(t, storedAdjustment.AccountingApplied)
}

func TestCommitGroupModelDiscountAdjustmentWithUsageRollsBackOnAccountingFailure(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	user, channel := seedGroupModelDiscountAccountingTargets(t, db)
	reservation, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("accounting-adjustment-fail-parent", "vip", "gpt-5", 100, 100))
	require.NoError(t, err)
	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: reservation.Calculation.ChargedQuota, RequestCountDelta: 1,
	}))
	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "accounting-adjustment-fail",
		SettlementRequestID: reservation.Settlement.RequestID,
		NewOriginalQuota:    200,
	})
	require.NoError(t, err)
	forcedErr := errors.New("forced adjustment accounting failure")
	callbackName := "test:group_discount_adjustment_accounting_failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

	err = commitGroupModelDiscountAdjustmentWithUsage(db, adjustment.Adjustment.AdjustmentID, BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: adjustment.DeltaChargedQuota,
	})
	assert.ErrorIs(t, err, forcedErr)
	storedUser, storedChannel := loadGroupModelDiscountAccountingTargets(t, db, user.Id, channel.Id)
	assert.Equal(t, 10+reservation.Calculation.ChargedQuota, storedUser.UsedQuota)
	assert.Equal(t, int64(10+reservation.Calculation.ChargedQuota), storedChannel.UsedQuota)
	storedAdjustment, err := getGroupModelDiscountAdjustment(db, adjustment.Adjustment.AdjustmentID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusReserved, storedAdjustment.Status)
	assert.False(t, storedAdjustment.AccountingApplied)
}

func TestReverseGroupModelDiscountSettlementWithUsageIsAtomicAndReplaySafe(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	user, channel := seedGroupModelDiscountAccountingTargets(t, db)
	reservation, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("accounting-reverse", "vip", "gpt-5", 100, 100))
	require.NoError(t, err)
	commitDelta := BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: reservation.Calculation.ChargedQuota, RequestCountDelta: 1,
	}
	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, commitDelta))
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID))
	reverseDelta := BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: -reservation.Calculation.ChargedQuota,
	}

	require.NoError(t, reverseGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, reverseDelta))
	require.NoError(t, reverseGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, reverseDelta))
	storedUser, storedChannel := loadGroupModelDiscountAccountingTargets(t, db, user.Id, channel.Id)
	assert.Equal(t, 10, storedUser.UsedQuota)
	assert.Equal(t, 3, storedUser.RequestCount, "a refund does not erase the historical request")
	assert.Equal(t, int64(10), storedChannel.UsedQuota)
	settlement, err := getGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusReversed, settlement.Status)
	assert.True(t, settlement.ReverseAccountingApplied)

	conflicting := reverseDelta
	conflicting.ChannelID++
	assert.ErrorIs(t, reverseGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, conflicting), ErrGroupModelDiscountAccountingConflict)
}

func TestReverseGroupModelDiscountSettlementWithUsageFailureLeavesPreparedGate(t *testing.T) {
	db := newGroupDiscountFileDB(t)
	user, channel := seedGroupModelDiscountAccountingTargets(t, db)
	reservation, err := reserveGroupModelDiscount(db, groupDiscountReserveInput("accounting-reverse-fail", "vip", "gpt-5", 100, 100))
	require.NoError(t, err)
	commitDelta := BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: reservation.Calculation.ChargedQuota, RequestCountDelta: 1,
	}
	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, commitDelta))
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID))

	forcedErr := errors.New("forced reverse accounting failure")
	callbackName := "test:group_discount_reverse_accounting_failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

	err = reverseGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: -reservation.Calculation.ChargedQuota,
	})
	assert.ErrorIs(t, err, forcedErr)
	settlement, err := getGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	require.NoError(t, err)
	assert.Equal(t, GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, GroupModelDiscountPendingActionReverseAfterRefund, settlement.PendingAction)
	storedUser, storedChannel := loadGroupModelDiscountAccountingTargets(t, db, user.Id, channel.Id)
	assert.Equal(t, 10+reservation.Calculation.ChargedQuota, storedUser.UsedQuota)
	assert.Equal(t, int64(10+reservation.Calculation.ChargedQuota), storedChannel.UsedQuota)
}
