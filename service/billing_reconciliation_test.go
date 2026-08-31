package service

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ageBillingReconciliationRow(t *testing.T, table, operationID string) {
	t.Helper()
	require.NoError(t, model.DB.Table(table).
		Where("operation_id = ?", operationID).
		Update("updated_at", time.Now().Add(-time.Hour)).Error)
}

func TestRunBillingReconciliationOnceRecoversRefundReadyButNeverUnknown(t *testing.T) {
	const (
		readyRequestID = "billing-reconcile-refund-ready"
		readyUserID    = 98900
		unknownRequest = "billing-reconcile-refund-unknown"
		unknownUserID  = 98910
	)
	_, _ = newBillingRefundWalletSession(t, readyRequestID, readyUserID, 500, 100)
	readyInput := model.BillingRefundOperationInput{
		OperationID:        "billing-reconcile-refund-ready-op",
		SessionID:          "billing-reconcile-refund-ready-session",
		RequestID:          readyRequestID,
		UserID:             readyUserID,
		TokenID:            readyUserID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: readyUserID,
		FundingQuota:       100,
		TokenQuota:         100,
	}
	_, err := model.BeginBillingRefundOperation(readyInput)
	require.NoError(t, err)
	ageBillingReconciliationRow(t, "billing_refund_operations", readyInput.OperationID)

	_, _ = newBillingRefundWalletSession(t, unknownRequest, unknownUserID, 500, 100)
	unknownInput := model.BillingRefundOperationInput{
		OperationID:        "billing-reconcile-refund-unknown-op",
		SessionID:          "billing-reconcile-refund-unknown-session",
		RequestID:          unknownRequest,
		UserID:             unknownUserID,
		TokenID:            unknownUserID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: unknownUserID,
		FundingQuota:       100,
		TokenQuota:         100,
	}
	_, err = model.BeginBillingRefundOperation(unknownInput)
	require.NoError(t, err)
	claim, err := model.ClaimNextBillingRefundAction(unknownInput.OperationID, model.BillingRefundPendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	ageBillingReconciliationRow(t, "billing_refund_operations", unknownInput.OperationID)

	runBillingReconciliationOnce(time.Now())

	ready, err := model.GetBillingRefundOperation(readyInput.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingRefundStatusApplied, ready.Status)
	assertBillingRefundWalletQuota(t, readyUserID, 500, 500)

	unknown, err := model.GetBillingRefundOperation(unknownInput.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingRefundPendingActionFundingUnknown, unknown.PendingAction)
	assertBillingRefundWalletQuota(t, unknownUserID, 400, 400)
}

func TestBillingRefundStaleReconcilerCannotClaimAnAdvancedRefundStage(t *testing.T) {
	const (
		requestID   = "billing-reconcile-refund-stale-claim"
		operationID = "billing-reconcile-refund-stale-claim-op"
		userID      = 98914
	)
	_, _ = newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	_, err := model.BeginBillingRefundOperation(model.BillingRefundOperationInput{
		OperationID:        operationID,
		SessionID:          "billing-reconcile-refund-stale-claim-session",
		RequestID:          requestID,
		UserID:             userID,
		TokenID:            userID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: int64(userID),
		FundingQuota:       100,
		TokenQuota:         100,
	})
	require.NoError(t, err)

	firstFundingPaused := make(chan struct{})
	resumeFirstFunding := make(chan struct{})
	firstTokenPaused := make(chan struct{})
	resumeFirstToken := make(chan struct{})
	var hookMu sync.Mutex
	fundingClaims := 0
	tokenClaims := 0
	billingBeforeActionClaimHook = func(claimOperationID, expectedReadyAction string) {
		if claimOperationID != operationID {
			return
		}
		hookMu.Lock()
		fundingClaimNumber := 0
		tokenClaimNumber := 0
		switch expectedReadyAction {
		case model.BillingRefundPendingActionFundingReady:
			fundingClaims++
			fundingClaimNumber = fundingClaims
		case model.BillingRefundPendingActionTokenReady:
			tokenClaims++
			tokenClaimNumber = tokenClaims
		}
		hookMu.Unlock()
		if fundingClaimNumber == 1 {
			close(firstFundingPaused)
			<-resumeFirstFunding
		}
		if tokenClaimNumber == 1 {
			close(firstTokenPaused)
			<-resumeFirstToken
		}
	}
	t.Cleanup(func() { billingBeforeActionClaimHook = nil })

	firstDone := make(chan error, 1)
	go func() {
		_, reconcileErr := ReconcilePendingBillingRefundOperation(operationID)
		firstDone <- reconcileErr
	}()
	<-firstFundingPaused

	secondDone := make(chan error, 1)
	go func() {
		_, reconcileErr := ReconcilePendingBillingRefundOperation(operationID)
		secondDone <- reconcileErr
	}()
	<-firstTokenPaused

	close(resumeFirstFunding)
	require.NoError(t, <-firstDone)
	close(resumeFirstToken)
	require.NoError(t, <-secondDone)

	operation, err := model.GetBillingRefundOperation(operationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingRefundStatusApplied, operation.Status)
	assert.Equal(t, 100, operation.FundingRefundedQuota)
	assert.Equal(t, 100, operation.TokenRefundedQuota)
	assertAdmissionQuotaState(t, userID, 500, 500)
}

func TestRunBillingReconciliationOnceCompensatesConfirmedAdmissionTopUpOnlyOnceAcrossWorkers(t *testing.T) {
	const (
		requestID = "billing-reconcile-admission-confirmed"
		userID    = 98920
	)
	_, _ = newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	input := model.BillingAdmissionReserveInput{
		OperationID:        "billing-reconcile-admission-confirmed-op",
		SessionID:          "billing-reconcile-admission-confirmed-session",
		RequestID:          requestID,
		Attempt:            1,
		UserID:             userID,
		TokenID:            userID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: userID,
		FromQuota:          100,
		TargetQuota:        200,
		TokenQuota:         100,
		Mode:               model.BillingAdmissionReserveModeStrictWallet,
	}
	_, err := model.BeginBillingAdmissionReserveOperation(input)
	require.NoError(t, err)
	fundingClaim, err := model.ClaimNextBillingAdmissionReserveAction(input.OperationID, model.BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, fundingClaim.Claimed)
	require.NoError(t, model.DecreaseUserQuota(userID, 100, false))
	require.NoError(t, model.ConfirmBillingAdmissionReserveFunding(input.OperationID))
	tokenClaim, err := model.ClaimNextBillingAdmissionReserveAction(input.OperationID, model.BillingAdmissionReservePendingActionTokenReady)
	require.NoError(t, err)
	require.True(t, tokenClaim.Claimed)
	require.NoError(t, model.DecreaseTokenQuota(userID+1, "admission-reserve-token-98920", 100))
	require.NoError(t, model.ConfirmBillingAdmissionReserveToken(input.OperationID))
	ageBillingReconciliationRow(t, "billing_admission_reserve_operations", input.OperationID)
	assertAdmissionQuotaState(t, userID, 300, 300)

	var wait sync.WaitGroup
	wait.Add(2)
	summaries := make(chan billingReconciliationSummary, 2)
	for range 2 {
		go func() {
			defer wait.Done()
			summaries <- runBillingReconciliationOnce(time.Now())
		}()
	}
	wait.Wait()
	close(summaries)
	for summary := range summaries {
		assert.Zero(t, summary.Failed)
	}

	operation, err := model.GetBillingAdmissionReserveOperation(input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingAdmissionReserveStatusCanceled, operation.Status)
	assert.Equal(t, 100, operation.TokenRefundedQuota)
	assert.Equal(t, 100, operation.FundingRefundedQuota)
	assertAdmissionQuotaState(t, userID, 400, 400)
}

func TestRunBillingReconciliationOnceCompensatesConfirmedInitialSubscriptionByRequest(t *testing.T) {
	const (
		requestID      = "billing-reconcile-admission-initial-subscription"
		userID         = 98924
		tokenID        = userID + 1
		subscriptionID = userID + 2
	)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.SubscriptionPreConsumeRecord{}).Error)
	require.NoError(t, model.DB.Where("id = ?", subscriptionID).Delete(&model.UserSubscription{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", tokenID).Delete(&model.Token{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Where("request_id = ?", requestID).Delete(&model.SubscriptionPreConsumeRecord{})
		model.DB.Where("id = ?", subscriptionID).Delete(&model.UserSubscription{})
		model.DB.Unscoped().Where("id = ?", tokenID).Delete(&model.Token{})
	})
	seedToken(t, tokenID, userID, "billing-reconcile-initial-subscription-token", 500)
	seedSubscription(t, subscriptionID, userID, 1_000, 100)
	require.NoError(t, model.DB.Create(&model.SubscriptionPreConsumeRecord{
		RequestId: requestID, UserId: userID, UserSubscriptionId: subscriptionID,
		PreConsumed: 100, Status: "consumed",
	}).Error)
	input := model.BillingAdmissionReserveInput{
		OperationID:        "billing-reconcile-admission-initial-subscription-op",
		SessionID:          "billing-reconcile-admission-initial-subscription-session",
		RequestID:          requestID,
		Attempt:            0,
		UserID:             userID,
		TokenID:            tokenID,
		FundingSource:      BillingSourceSubscription,
		FundingReferenceID: subscriptionID,
		FromQuota:          0,
		TargetQuota:        100,
		TokenQuota:         0,
		Mode:               model.BillingAdmissionReserveModeInitial,
	}
	_, err := model.BeginBillingAdmissionReserveOperation(input)
	require.NoError(t, err)
	claim, err := model.ClaimNextBillingAdmissionReserveAction(input.OperationID, model.BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	require.NoError(t, model.ConfirmBillingAdmissionReserveFunding(input.OperationID))
	ageBillingReconciliationRow(t, "billing_admission_reserve_operations", input.OperationID)

	summary := runBillingReconciliationOnce(time.Now())

	assert.Zero(t, summary.Failed)
	operation, err := model.GetBillingAdmissionReserveOperation(input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingAdmissionReserveStatusCanceled, operation.Status)
	var record model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&record).Error)
	assert.Equal(t, "refunded", record.Status)
	var subscription model.UserSubscription
	require.NoError(t, model.DB.First(&subscription, subscriptionID).Error)
	assert.Zero(t, subscription.AmountUsed)
}

func TestRunBillingReconciliationOnceCancelsUnstartedAdmissionAndLeavesUnknownManual(t *testing.T) {
	const (
		requestID = "billing-reconcile-admission-safe-states"
		userID    = 98930
	)
	_, _ = newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	readyInput := model.BillingAdmissionReserveInput{
		OperationID:        "billing-reconcile-admission-ready-op",
		SessionID:          "billing-reconcile-admission-ready-session",
		RequestID:          requestID,
		Attempt:            1,
		UserID:             userID,
		TokenID:            userID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: userID,
		FromQuota:          100,
		TargetQuota:        200,
		TokenQuota:         100,
		Mode:               model.BillingAdmissionReserveModeStrictWallet,
	}
	_, err := model.BeginBillingAdmissionReserveOperation(readyInput)
	require.NoError(t, err)
	ageBillingReconciliationRow(t, "billing_admission_reserve_operations", readyInput.OperationID)

	unknownInput := readyInput
	unknownInput.OperationID = "billing-reconcile-admission-unknown-op"
	unknownInput.Attempt = 2
	_, err = model.BeginBillingAdmissionReserveOperation(unknownInput)
	require.NoError(t, err)
	claim, err := model.ClaimNextBillingAdmissionReserveAction(unknownInput.OperationID, model.BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	ageBillingReconciliationRow(t, "billing_admission_reserve_operations", unknownInput.OperationID)

	runBillingReconciliationOnce(time.Now())

	ready, err := model.GetBillingAdmissionReserveOperation(readyInput.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingAdmissionReserveStatusCanceled, ready.Status)
	unknown, err := model.GetBillingAdmissionReserveOperation(unknownInput.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingAdmissionReservePendingActionFundingUnknown, unknown.PendingAction)
	assertAdmissionQuotaState(t, userID, 400, 400)
}

func TestReconcilePendingBillingAdmissionReserveOperationMarksCorruptEvidenceManual(t *testing.T) {
	const (
		requestID = "billing-reconcile-admission-corrupt"
		userID    = 98940
	)
	_, _ = newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	input := model.BillingAdmissionReserveInput{
		OperationID:        "billing-reconcile-admission-corrupt-op",
		SessionID:          "billing-reconcile-admission-corrupt-session",
		RequestID:          requestID,
		Attempt:            1,
		UserID:             userID,
		TokenID:            userID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: userID,
		FromQuota:          100,
		TargetQuota:        200,
		TokenQuota:         100,
		Mode:               model.BillingAdmissionReserveModeStrictWallet,
	}
	operation, err := model.BeginBillingAdmissionReserveOperation(input)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.BillingAdmissionReserveOperation{}).
		Where("id = ?", operation.Id).
		Update("delta_quota", 99).Error)

	_, err = ReconcilePendingBillingAdmissionReserveOperation(input.OperationID)

	assert.ErrorIs(t, err, ErrBillingAdmissionReserveRequiresManualReconciliation)
	assert.ErrorIs(t, err, model.ErrBillingAdmissionReserveCorrupt)
	assertAdmissionQuotaState(t, userID, 400, 400)
	ageBillingReconciliationRow(t, "billing_admission_reserve_operations", input.OperationID)
	summary := runBillingReconciliationOnce(time.Now())
	assert.Equal(t, 1, summary.ManualAdmissionCount)
	assert.Zero(t, summary.Failed)
}

func TestBillingReconciliationRefundPersistsBeforeConfirmationWhenBatchUpdatesEnabled(t *testing.T) {
	const (
		requestID = "billing-reconcile-refund-batch-durable"
		userID    = 98950
	)
	_, _ = newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	input := model.BillingRefundOperationInput{
		OperationID:        "billing-reconcile-refund-batch-durable-op",
		SessionID:          "billing-reconcile-refund-batch-durable-session",
		RequestID:          requestID,
		UserID:             userID,
		TokenID:            userID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: userID,
		FundingQuota:       100,
		TokenQuota:         100,
	}
	_, err := model.BeginBillingRefundOperation(input)
	require.NoError(t, err)
	ageBillingReconciliationRow(t, "billing_refund_operations", input.OperationID)
	wasBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = wasBatchEnabled })

	summary := runBillingReconciliationOnce(time.Now())

	assert.Zero(t, summary.Failed)
	operation, err := model.GetBillingRefundOperation(input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingRefundStatusApplied, operation.Status)
	assertBillingRefundWalletQuota(t, userID, 500, 500)
}
