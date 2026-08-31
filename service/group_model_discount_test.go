package service

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newGroupModelDiscountTestContext() *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

type groupDiscountBillingRecorder struct {
	preConsumed    int
	settleCalls    []int
	settleErr      error
	fundingSettled bool
	needsRefund    bool
	refundCalls    int
}

func (s *groupDiscountBillingRecorder) Settle(quota int) error {
	s.settleCalls = append(s.settleCalls, quota)
	return s.settleErr
}

func (s *groupDiscountBillingRecorder) Refund(*gin.Context) { s.refundCalls++ }

func (s *groupDiscountBillingRecorder) NeedsRefund() bool { return s.needsRefund }

func (s *groupDiscountBillingRecorder) GetPreConsumedQuota() int { return s.preConsumed }

func (*groupDiscountBillingRecorder) Reserve(int) error { return nil }

func (*groupDiscountBillingRecorder) ReserveForAdmission(int) error { return nil }

func (s *groupDiscountBillingRecorder) FundingWasSettled() bool { return s.fundingSettled }

func testGroupModelDiscountSnapshot() groupdiscount.Snapshot {
	return groupdiscount.Snapshot{
		PolicyHash:  "policy-v1",
		UsingGroup:  "vip",
		OriginModel: "gpt-tiered",
		Timezone:    "Asia/Shanghai",
		PeriodStart: 1_788_192_000,
		PeriodEnd:   1_790_870_400,
		Tiers: []groupdiscount.Tier{
			{MinMonthlyOriginalQuota: 0, Ratio: 0.9},
			{MinMonthlyOriginalQuota: 500, Ratio: 0.8},
		},
	}
}

func prepareGroupModelDiscountServiceTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
	))
	require.NoError(t, model.DB.Exec("DELETE FROM group_model_discount_adjustments").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM group_model_discount_settlements").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM user_group_model_monthly_usages").Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", 7101).Delete(&model.User{}).Error)
	require.NoError(t, model.DB.Where("id = ?", 7102).Delete(&model.Channel{}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id:       7101,
		Username: "group-discount-service-user",
		Password: "password",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:     7102,
		Name:   "group-discount-service-channel",
		Key:    "sk-group-discount-service",
		Status: common.ChannelStatusEnabled,
	}).Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM group_model_discount_adjustments")
		model.DB.Exec("DELETE FROM group_model_discount_settlements")
		model.DB.Exec("DELETE FROM user_group_model_monthly_usages")
		model.DB.Unscoped().Where("id = ?", 7101).Delete(&model.User{})
		model.DB.Where("id = ?", 7102).Delete(&model.Channel{})
	})
}

func newGroupModelDiscountRelayInfo(requestID string, billing relaycommon.BillingSettler) *relaycommon.RelayInfo {
	snapshot := testGroupModelDiscountSnapshot()
	return &relaycommon.RelayInfo{
		RequestId:                  requestID,
		UserId:                     7101,
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		Billing:                    billing,
		FinalPreConsumedQuota:      300,
		GroupModelDiscountSnapshot: &snapshot,
		ChannelMeta:                &relaycommon.ChannelMeta{ChannelId: 7102},
	}
}

func TestSettleModelChargeLeavesFundingAppliedSettlementPendingWhenAtomicUsageFails(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	forcedErr := errors.New("forced channel accounting failure")
	callbackName := "test:settle_model_charge_channel_accounting_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })
	billing := &groupDiscountBillingRecorder{preConsumed: 300}
	info := newGroupModelDiscountRelayInfo("monthly-accounting-failed", billing)

	decision, err := SettleModelCharge(newGroupModelDiscountTestContext(), info, info.RequestId, 600, 300)

	require.ErrorIs(t, err, forcedErr)
	assert.True(t, decision.RequiresReconciliation)
	assert.Equal(t, []int{530}, billing.settleCalls, "funding must not be replayed after accounting failure")
	settlement, getErr := model.GetGroupModelDiscountSettlement(info.RequestId)
	require.NoError(t, getErr)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionCommitAfterFunding, settlement.PendingAction)
	var user model.User
	require.NoError(t, model.DB.First(&user, info.UserId).Error)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, info.ChannelId).Error)
	assert.Zero(t, channel.UsedQuota)
}

func TestSettleModelChargeFallsBackWithoutMonthlyPolicy(t *testing.T) {
	billing := &groupDiscountBillingRecorder{preConsumed: 150}
	info := &relaycommon.RelayInfo{Billing: billing, FinalPreConsumedQuota: 150}

	decision, err := SettleModelCharge(newGroupModelDiscountTestContext(), info, "fallback-request", 300, 150)

	require.NoError(t, err)
	assert.False(t, decision.Applied)
	assert.Equal(t, 150, decision.ChargedQuota)
	assert.Equal(t, []int{150}, billing.settleCalls)
}

func TestSettleModelChargeCommitsMonthlyDecisionFromOriginalQuota(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	billing := &groupDiscountBillingRecorder{preConsumed: 300}
	info := newGroupModelDiscountRelayInfo("monthly-success", billing)

	decision, err := SettleModelCharge(newGroupModelDiscountTestContext(), info, info.RequestId, 600, 300)

	require.NoError(t, err)
	assert.True(t, decision.Applied)
	assert.False(t, decision.Reused)
	assert.Equal(t, 530, decision.ChargedQuota)
	assert.Equal(t, []int{530}, billing.settleCalls)

	settlement, err := model.GetGroupModelDiscountSettlement(info.RequestId)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlement.Status)
	assert.EqualValues(t, 600, settlement.OriginalQuota)
	assert.EqualValues(t, 530, settlement.ChargedQuota)

	usage, err := model.GetUserGroupModelMonthlyUsage(info.UserId, info.UsingGroup, info.OriginModelName, info.GroupModelDiscountSnapshot.PeriodStart)
	require.NoError(t, err)
	assert.EqualValues(t, 600, usage.OriginalQuota)
	assert.EqualValues(t, 530, usage.ChargedQuota)
}

func TestSettleModelChargeFundingErrorStaysUnknownWithoutAutomaticCompensation(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	settleErr := errors.New("wallet write failed")
	billing := &groupDiscountBillingRecorder{preConsumed: 300, settleErr: settleErr, needsRefund: true}
	info := newGroupModelDiscountRelayInfo("monthly-funding-failed", billing)

	decision, err := SettleModelCharge(newGroupModelDiscountTestContext(), info, info.RequestId, 300, 150)

	require.ErrorIs(t, err, settleErr)
	assert.True(t, decision.RequiresReconciliation)
	assert.Zero(t, billing.refundCalls, "a Settle error cannot prove which funding/token stages completed")
	settlement, getErr := model.GetGroupModelDiscountSettlement(info.RequestId)
	require.NoError(t, getErr)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, settlement.PendingAction)
	usage, getErr := model.GetUserGroupModelMonthlyUsage(info.UserId, info.UsingGroup, info.OriginModelName, info.GroupModelDiscountSnapshot.PeriodStart)
	require.NoError(t, getErr)
	assert.EqualValues(t, 300, usage.OriginalQuota)
	assert.EqualValues(t, 270, usage.ChargedQuota)
}

func TestSettleModelChargeReserveErrorWithoutVisibleSettlementRetainsPreConsume(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	billing := &groupDiscountBillingRecorder{preConsumed: 300, needsRefund: true}
	info := newGroupModelDiscountRelayInfo("monthly-reserve-invalid", billing)

	decision, err := SettleModelCharge(
		newGroupModelDiscountTestContext(),
		info,
		info.RequestId,
		common.MaxQuota+1,
		150,
	)

	require.ErrorIs(t, err, groupdiscount.ErrInvalidOriginalQuota)
	assert.False(t, decision.RequiresReconciliation)
	assert.False(t, decision.Applied)
	assert.False(t, decision.FundingStarted)
	assert.True(t, decision.AdmissionRefundSafe)
	assert.Zero(t, billing.refundCalls, "the caller owns the one safe admission refund")
}

func TestSettleModelChargeRequestConflictDoesNotMutateVisibleSettlement(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	info := newGroupModelDiscountRelayInfo("monthly-reserve-visible", &groupDiscountBillingRecorder{preConsumed: 300})
	_, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     info.RequestId,
		UserID:        info.UserId,
		UsingGroup:    info.UsingGroup,
		OriginModel:   info.OriginModelName,
		Snapshot:      *info.GroupModelDiscountSnapshot,
		OriginalQuota: 300,
	})
	require.NoError(t, err)

	billing := &groupDiscountBillingRecorder{preConsumed: 300, needsRefund: true}
	info.Billing = billing
	decision, err := SettleModelCharge(newGroupModelDiscountTestContext(), info, info.RequestId, 301, 150)

	require.ErrorIs(t, err, model.ErrGroupModelDiscountRequestConflict)
	assert.False(t, decision.Applied)
	assert.False(t, decision.FundingStarted)
	assert.True(t, decision.AdmissionRefundSafe)
	assert.Zero(t, billing.refundCalls)
	settlement, getErr := model.GetGroupModelDiscountSettlement(info.RequestId)
	require.NoError(t, getErr)
	assert.Equal(t, model.GroupModelDiscountStatusReserved, settlement.Status)
	assert.Empty(t, settlement.PendingAction)
}

func TestSettleModelChargeMarksUnknownAfterFundingWasApplied(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	settleErr := errors.New("token adjustment failed")
	billing := &groupDiscountBillingRecorder{
		preConsumed:    300,
		settleErr:      settleErr,
		fundingSettled: true,
	}
	info := newGroupModelDiscountRelayInfo("monthly-needs-reconcile", billing)

	_, err := SettleModelCharge(newGroupModelDiscountTestContext(), info, info.RequestId, 300, 150)

	require.ErrorIs(t, err, settleErr)
	settlement, getErr := model.GetGroupModelDiscountSettlement(info.RequestId)
	require.NoError(t, getErr)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, settlement.PendingAction)
}

func TestSettleModelChargeReusedReservedSettlementLeavesOriginalOwnerInControl(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	info := newGroupModelDiscountRelayInfo("monthly-reserved-owner", &groupDiscountBillingRecorder{preConsumed: 300})
	_, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     info.RequestId,
		UserID:        info.UserId,
		UsingGroup:    info.UsingGroup,
		OriginModel:   info.OriginModelName,
		Snapshot:      *info.GroupModelDiscountSnapshot,
		OriginalQuota: 300,
	})
	require.NoError(t, err)

	replayBilling := &groupDiscountBillingRecorder{preConsumed: 300}
	replayInfo := newGroupModelDiscountRelayInfo(info.RequestId, replayBilling)
	decision, err := SettleModelCharge(newGroupModelDiscountTestContext(), replayInfo, replayInfo.RequestId, 300, 150)

	assert.ErrorIs(t, err, ErrGroupModelDiscountSettlementPending)
	assert.False(t, decision.Reused, "an unresolved historical reservation is not a safely reusable result")
	assert.True(t, decision.RequiresReconciliation)
	assert.Equal(t, []int{0}, replayBilling.settleCalls)
	settlement, getErr := model.GetGroupModelDiscountSettlement(info.RequestId)
	require.NoError(t, getErr)
	assert.Equal(t, model.GroupModelDiscountStatusReserved, settlement.Status)
	assert.Empty(t, settlement.PendingAction, "a duplicate caller cannot change the original owner's recovery path")
}

func TestSettleModelChargeReusedSettlementRefundsNewReservationWithoutDoubleCharge(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	firstBilling := &groupDiscountBillingRecorder{preConsumed: 300}
	firstInfo := newGroupModelDiscountRelayInfo("monthly-replay", firstBilling)
	first, err := SettleModelCharge(newGroupModelDiscountTestContext(), firstInfo, firstInfo.RequestId, 300, 150)
	require.NoError(t, err)
	require.Equal(t, 270, first.ChargedQuota)

	replayBilling := &groupDiscountBillingRecorder{preConsumed: 300}
	replayInfo := newGroupModelDiscountRelayInfo("monthly-replay", replayBilling)
	replayed, err := SettleModelCharge(newGroupModelDiscountTestContext(), replayInfo, replayInfo.RequestId, 300, 150)

	require.NoError(t, err)
	assert.True(t, replayed.Reused)
	assert.Equal(t, 270, replayed.ChargedQuota)
	assert.Equal(t, []int{0}, replayBilling.settleCalls, "the replay's fresh pre-consume must be returned")

	usage, err := model.GetUserGroupModelMonthlyUsage(replayInfo.UserId, replayInfo.UsingGroup, replayInfo.OriginModelName, replayInfo.GroupModelDiscountSnapshot.PeriodStart)
	require.NoError(t, err)
	assert.EqualValues(t, 300, usage.OriginalQuota)
	assert.EqualValues(t, 270, usage.ChargedQuota)
}

func TestSettleModelChargeReusedSettlementErrorDoesNotGuessFreshFundingOutcome(t *testing.T) {
	prepareGroupModelDiscountServiceTest(t)
	firstBilling := &groupDiscountBillingRecorder{preConsumed: 300}
	firstInfo := newGroupModelDiscountRelayInfo("monthly-replay-settle-error", firstBilling)
	_, err := SettleModelCharge(newGroupModelDiscountTestContext(), firstInfo, firstInfo.RequestId, 300, 150)
	require.NoError(t, err)

	settleErr := errors.New("fresh replay funding outcome unknown")
	replayBilling := &groupDiscountBillingRecorder{
		preConsumed: 300,
		settleErr:   settleErr,
		needsRefund: true,
	}
	replayInfo := newGroupModelDiscountRelayInfo(firstInfo.RequestId, replayBilling)
	decision, err := SettleModelCharge(newGroupModelDiscountTestContext(), replayInfo, replayInfo.RequestId, 300, 150)

	assert.ErrorIs(t, err, settleErr)
	assert.ErrorIs(t, err, ErrGroupModelDiscountSettlementPending)
	assert.False(t, decision.Reused, "a replay is reusable only after its fresh pre-consume was safely returned")
	assert.True(t, decision.RequiresReconciliation)
	assert.Zero(t, replayBilling.refundCalls)
	settlement, getErr := model.GetGroupModelDiscountSettlement(firstInfo.RequestId)
	require.NoError(t, getErr)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, settlement.PendingAction)
}
