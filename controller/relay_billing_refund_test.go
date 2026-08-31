package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type refundRecordingBilling struct {
	refundCalls int
	settleCalls []int
}

func (b *refundRecordingBilling) Settle(quota int) error {
	b.settleCalls = append(b.settleCalls, quota)
	return nil
}
func (*refundRecordingBilling) NeedsRefund() bool        { return true }
func (*refundRecordingBilling) GetPreConsumedQuota() int { return 100 }
func (*refundRecordingBilling) Reserve(int) error        { return nil }

func (*refundRecordingBilling) ReserveForAdmission(int) error { return nil }

func (b *refundRecordingBilling) Refund(*gin.Context) {
	b.refundCalls++
}

func TestRefundRelayBillingOnCommittedBusinessFailure(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	billing := &refundRecordingBilling{}
	info := &relaycommon.RelayInfo{Billing: billing}
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "NotEnoughCvError",
		Code:    "11210",
	}, http.StatusTooManyRequests, types.ErrOptionWithResponseCommitted())
	require.NotNil(t, err)

	refundRelayBillingOnFailure(c, info, err)

	assert.Equal(t, 1, billing.refundCalls)
}

func TestRefundTaskBillingOnFailureStopsAtSettlementBoundary(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	taskErr := &taskdto.TaskError{Code: "billing_failed", Message: "billing failed"}

	beforeSettlement := &refundRecordingBilling{}
	refundTaskBillingOnFailure(c, &relaycommon.RelayInfo{Billing: beforeSettlement}, taskErr, false)
	assert.Equal(t, 1, beforeSettlement.refundCalls, "admission and upstream failures still return the unused pre-consume")

	afterSettlementStarted := &refundRecordingBilling{}
	refundTaskBillingOnFailure(c, &relaycommon.RelayInfo{Billing: afterSettlementStarted}, taskErr, true)
	assert.Zero(t, afterSettlementStarted.refundCalls, "a settlement error has an unknown funding outcome and cannot be compensated automatically")
}

func TestApplyTaskSettlementDecisionKeepsReusedSettleErrorPending(t *testing.T) {
	settleErr := errors.New("fresh replay funding outcome is unknown")
	task := &model.Task{
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				DiscountSettlementID: "task:replayed-settlement",
				ChargeState:          model.TaskChargeStatePrepared,
			},
		},
	}
	decision := service.GroupModelDiscountDecision{
		Applied:      true,
		Reused:       true,
		RequestID:    "task:replayed-settlement",
		ChargedQuota: 270,
	}

	applyTaskSettlementDecision(task, decision, settleErr)

	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Equal(t, 270, task.Quota)
	assert.Equal(t, 270, task.PrivateData.BillingContext.NetQuota)
	assert.Equal(t, "task:replayed-settlement", task.PrivateData.BillingContext.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, task.PrivateData.BillingContext.ChargeState)
}

func TestApplyTaskSettlementDecisionClearsUnfundedMonthlyReservationFailure(t *testing.T) {
	task := &model.Task{
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				DiscountSettlementID: "task:reserve-failed",
				ChargeState:          model.TaskChargeStatePrepared,
			},
		},
	}
	decision := service.GroupModelDiscountDecision{ChargedQuota: 150, AdmissionRefundSafe: true}

	applyTaskSettlementDecision(task, decision, errors.New("funding API returned an error before confirmation"))

	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Zero(t, task.Quota)
	assert.Zero(t, task.PrivateData.BillingContext.NetQuota)
	assert.Empty(t, task.PrivateData.BillingContext.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStateUncharged, task.PrivateData.BillingContext.ChargeState)
}

func TestApplyTaskSettlementDecisionClearsOnlySuccessfulReplay(t *testing.T) {
	task := &model.Task{
		Quota: 270,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				NetQuota:             270,
				DiscountSettlementID: "task:successful-replay",
				ChargeState:          model.TaskChargeStatePrepared,
			},
		},
	}
	decision := service.GroupModelDiscountDecision{
		Applied:      true,
		Reused:       true,
		RequestID:    "task:successful-replay",
		ChargedQuota: 270,
	}

	applyTaskSettlementDecision(task, decision, nil)

	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Zero(t, task.Quota)
	assert.Zero(t, task.PrivateData.BillingContext.NetQuota)
	assert.Empty(t, task.PrivateData.BillingContext.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStateReused, task.PrivateData.BillingContext.ChargeState)
}

func TestSettlePersistedFixedTaskSubmissionPendingPrewriteFailureStartsNoSettlement(t *testing.T) {
	useControllerTaskBillingDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	billing := &refundRecordingBilling{}
	relayInfo := &relaycommon.RelayInfo{Billing: billing, UserQuota: 1_000_000}
	task := newPreparedFixedTask(t, "fixed-submit-prewrite-failure")
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_fixed_task_submit_pending_prewrite
		BEFORE UPDATE OF private_data ON tasks
		WHEN json_extract(NEW.private_data, '$.billing_context.charge_state') = 'pending_reconcile'
		BEGIN
			SELECT RAISE(ABORT, 'forced fixed task submit pending prewrite failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_fixed_task_submit_pending_prewrite").Error })

	decision, err := settlePersistedTaskSubmission(c, relayInfo, task, "task:fixed-submit-prewrite-failure", 200, 120)

	require.Error(t, err)
	assert.False(t, decision.FundingStarted)
	assert.Empty(t, billing.settleCalls)
	persisted, loadErr := model.GetTaskBillingState(task.ID)
	require.NoError(t, loadErr)
	assert.Zero(t, persisted.Quota)
	require.NotNil(t, persisted.BillingReady)
	assert.False(t, *persisted.BillingReady)
	assert.Zero(t, persisted.PrivateData.BillingContext.NetQuota)
	assert.Equal(t, model.TaskChargeStatePrepared, persisted.PrivateData.BillingContext.ChargeState)
}

func TestSettlePersistedFixedTaskSubmissionFinalStateFailureKeepsPendingIntentWithoutReplay(t *testing.T) {
	useControllerTaskBillingDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	billing := &refundRecordingBilling{}
	relayInfo := &relaycommon.RelayInfo{Billing: billing, UserQuota: 1_000_000}
	task := newPreparedFixedTask(t, "fixed-submit-final-state-failure")
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_fixed_task_submit_final_state
		BEFORE UPDATE OF private_data ON tasks
		WHEN json_extract(NEW.private_data, '$.billing_context.charge_state') = 'charged'
		BEGIN
			SELECT RAISE(ABORT, 'forced fixed task submit final state failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_fixed_task_submit_final_state").Error })

	decision, err := settlePersistedTaskSubmission(c, relayInfo, task, "task:fixed-submit-final-state-failure", 200, 120)

	require.Error(t, err)
	assert.True(t, decision.FundingStarted)
	assert.Equal(t, []int{120}, billing.settleCalls)
	persisted, loadErr := model.GetTaskBillingState(task.ID)
	require.NoError(t, loadErr)
	assert.Zero(t, persisted.Quota, "no task charge is confirmed while the final marker is ambiguous")
	require.NotNil(t, persisted.BillingReady)
	assert.False(t, *persisted.BillingReady)
	assert.Equal(t, 200, persisted.PrivateData.BillingContext.OriginalQuota)
	assert.Equal(t, 120, persisted.PrivateData.BillingContext.NetQuota)
	assert.Equal(t, 120, persisted.PrivateData.BillingContext.PendingNetQuota)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.PrivateData.BillingContext.ChargeState)

	_, retryErr := settlePersistedTaskSubmission(c, relayInfo, persisted, "task:fixed-submit-final-state-failure", 200, 120)
	require.Error(t, retryErr)
	assert.Equal(t, []int{120}, billing.settleCalls)
	assert.False(t, service.RefundTaskQuota(c, persisted, "must not refund ambiguous fixed submit"))
	service.RecalculateTaskQuota(c, persisted, 150, "must not recalculate ambiguous fixed submit")
	assert.Equal(t, []int{120}, billing.settleCalls)
}

func TestSettlePersistedFixedTaskSubmissionSuccessConfirmsIntendedQuota(t *testing.T) {
	useControllerTaskBillingDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	billing := &refundRecordingBilling{}
	relayInfo := &relaycommon.RelayInfo{Billing: billing, UserQuota: 1_000_000}
	task := newPreparedFixedTask(t, "fixed-submit-success")

	decision, err := settlePersistedTaskSubmission(c, relayInfo, task, "task:fixed-submit-success", 200, 120)

	require.NoError(t, err)
	assert.True(t, decision.FundingStarted)
	assert.Equal(t, []int{120}, billing.settleCalls)
	persisted, loadErr := model.GetTaskBillingState(task.ID)
	require.NoError(t, loadErr)
	assert.Equal(t, 120, persisted.Quota)
	require.NotNil(t, persisted.BillingReady)
	assert.True(t, *persisted.BillingReady)
	assert.Equal(t, 200, persisted.PrivateData.BillingContext.OriginalQuota)
	assert.Equal(t, 120, persisted.PrivateData.BillingContext.NetQuota)
	assert.Zero(t, persisted.PrivateData.BillingContext.PendingNetQuota)
	assert.Equal(t, model.TaskChargeStateCharged, persisted.PrivateData.BillingContext.ChargeState)
}

func TestSettlePersistedFixedTaskSubmissionRejectsNegativeQuotaBeforeSettlement(t *testing.T) {
	useControllerTaskBillingDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	billing := &refundRecordingBilling{}
	relayInfo := &relaycommon.RelayInfo{Billing: billing, UserQuota: 1_000_000}
	task := newPreparedFixedTask(t, "fixed-submit-negative-quota")

	decision, err := settlePersistedTaskSubmission(c, relayInfo, task, "task:fixed-submit-negative-quota", 200, -1)

	require.Error(t, err)
	assert.False(t, decision.FundingStarted)
	assert.Empty(t, billing.settleCalls)
	persisted, loadErr := model.GetTaskBillingState(task.ID)
	require.NoError(t, loadErr)
	assert.Zero(t, persisted.Quota)
	require.NotNil(t, persisted.BillingReady)
	assert.False(t, *persisted.BillingReady)
	assert.Equal(t, model.TaskChargeStatePrepared, persisted.PrivateData.BillingContext.ChargeState)
}

func TestSettlePersistedDynamicTaskFinalWriteFailureHandsOffToPollingRecovery(t *testing.T) {
	useControllerTaskBillingDB(t)
	const userID, channelID = 6101, 6102
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "task-recovery-user", AffCode: "task-recovery-user"}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: channelID, Name: "task-recovery-channel", Key: "sk-task-recovery"}).Error)
	snapshot := controllerGroupModelDiscountSnapshot()
	billing := &refundRecordingBilling{}
	relayInfo := &relaycommon.RelayInfo{
		UserId: userID, UsingGroup: snapshot.UsingGroup, OriginModelName: snapshot.OriginModel,
		Billing: billing, GroupModelDiscountSnapshot: &snapshot,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	settlementID := "task:dynamic-final-write-recovery"
	task := newPreparedDynamicTask(t, settlementID, userID, channelID, snapshot)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_dynamic_task_final_write
		BEFORE UPDATE OF private_data ON tasks
		WHEN json_extract(NEW.private_data, '$.billing_context.charge_state') = 'charged'
		BEGIN
			SELECT RAISE(ABORT, 'forced dynamic task final write failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_dynamic_task_final_write").Error })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	decision, settleErr := settlePersistedTaskSubmission(c, relayInfo, task, settlementID, 600, 300)

	require.Error(t, settleErr)
	assert.True(t, decision.Applied)
	assert.False(t, decision.Reused)
	assert.Equal(t, []int{decision.ChargedQuota}, billing.settleCalls)
	persisted, loadErr := model.GetTaskBillingState(task.ID)
	require.NoError(t, loadErr)
	assert.Equal(t, model.TaskChargeStatePrepared, persisted.PrivateData.BillingContext.ChargeState)
	assert.True(t, persisted.BillingRecoveryPending)
	require.NotNil(t, persisted.BillingReady)
	assert.False(t, *persisted.BillingReady)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_dynamic_task_final_write").Error)
	pollable := model.GetAllUnFinishSyncTasks(10)
	require.Len(t, pollable, 1)
	assert.Equal(t, model.TaskChargeStateCharged, pollable[0].PrivateData.BillingContext.ChargeState)
	assert.Equal(t, decision.ChargedQuota, pollable[0].Quota)
	assert.False(t, pollable[0].BillingRecoveryPending)
	require.NotNil(t, pollable[0].BillingReady)
	assert.True(t, *pollable[0].BillingReady)
	assert.Equal(t, []int{decision.ChargedQuota}, billing.settleCalls, "poll recovery must not replay funding")
}

func TestSettlePersistedDynamicTaskHandoffFailureMarksLedgerManual(t *testing.T) {
	useControllerTaskBillingDB(t)
	const userID, channelID = 6103, 6104
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "task-handoff-user", AffCode: "task-handoff-user"}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: channelID, Name: "task-handoff-channel", Key: "sk-task-handoff"}).Error)
	snapshot := controllerGroupModelDiscountSnapshot()
	billing := &refundRecordingBilling{}
	relayInfo := &relaycommon.RelayInfo{
		UserId: userID, UsingGroup: snapshot.UsingGroup, OriginModelName: snapshot.OriginModel,
		Billing: billing, GroupModelDiscountSnapshot: &snapshot,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	settlementID := "task:dynamic-handoff-failure"
	task := newPreparedDynamicTask(t, settlementID, userID, channelID, snapshot)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_dynamic_task_final_and_handoff
		BEFORE UPDATE ON tasks
		WHEN json_extract(NEW.private_data, '$.billing_context.charge_state') = 'charged'
			OR NEW.billing_recovery_pending = 1
		BEGIN
			SELECT RAISE(ABORT, 'forced dynamic task final and handoff failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_dynamic_task_final_and_handoff").Error })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	decision, settleErr := settlePersistedTaskSubmission(c, relayInfo, task, settlementID, 600, 300)

	require.Error(t, settleErr)
	assert.True(t, decision.Applied)
	settlement, loadErr := model.GetGroupModelDiscountSettlement(decision.RequestID)
	require.NoError(t, loadErr)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, settlement.PendingAction)
	assert.Empty(t, model.GetAllUnFinishSyncTasks(10), "failed handoff must remain blocked for manual reconciliation")
	assert.Equal(t, []int{decision.ChargedQuota}, billing.settleCalls)
}

func useControllerTaskBillingDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Channel{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
	))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
}

func controllerGroupModelDiscountSnapshot() groupdiscount.Snapshot {
	return groupdiscount.Snapshot{
		PolicyHash: "controller-task-policy-v1", UsingGroup: "vip", OriginModel: "task-tiered",
		Timezone: "Asia/Shanghai", PeriodStart: 1_788_192_000, PeriodEnd: 1_790_870_400,
		Tiers: []groupdiscount.Tier{
			{MinMonthlyOriginalQuota: 0, Ratio: 0.9},
			{MinMonthlyOriginalQuota: 500, Ratio: 0.8},
		},
	}
}

func newPreparedDynamicTask(
	t *testing.T,
	settlementID string,
	userID int,
	channelID int,
	snapshot groupdiscount.Snapshot,
) *model.Task {
	t.Helper()
	task := &model.Task{
		TaskID: settlementID, UserId: userID, ChannelId: channelID,
		Status: model.TaskStatusNotStart, Progress: "0%",
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			OriginalQuota: 600, DiscountSettlementID: settlementID,
			ChargeState: model.TaskChargeStatePrepared, GroupModelDiscountSnapshot: &snapshot,
		}},
	}
	require.NoError(t, task.Insert())
	return task
}

func newPreparedFixedTask(t *testing.T, taskID string) *model.Task {
	t.Helper()
	task := &model.Task{
		TaskID: taskID,
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			ChargeState: model.TaskChargeStatePrepared,
		}},
	}
	require.NoError(t, task.Insert())
	return task
}

func TestShouldRetryRefusesWrittenResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	_, writeErr := c.Writer.Write([]byte(": PING\n\n"))
	require.NoError(t, writeErr)
	err := types.NewOpenAIError(errors.New("retryable upstream failure"), types.ErrorCodeBadResponse, http.StatusServiceUnavailable)

	assert.False(t, shouldRetry(c, err, 1))
}
