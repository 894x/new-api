package service

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAdmissionReserveWalletSession(
	t *testing.T,
	requestID string,
	userID int,
	startingQuota int,
	preConsumedQuota int,
) (*BillingSession, *gin.Context) {
	t.Helper()
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})

	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: fmt.Sprintf("admission-reserve-%d", userID),
		Status:   common.UserStatusEnabled,
		Quota:    startingQuota,
		AffCode:  fmt.Sprintf("admission-reserve-aff-%d", userID),
	}).Error)
	seedToken(t, userID+1, userID, fmt.Sprintf("admission-reserve-token-%d", userID), startingQuota)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId:       requestID,
		UserId:          userID,
		TokenId:         userID + 1,
		TokenKey:        fmt.Sprintf("admission-reserve-token-%d", userID),
		ForcePreConsume: true,
		UserSetting: dto.UserSetting{
			BillingPreference: "wallet_only",
		},
	}
	require.Nil(t, PreConsumeBilling(ctx, preConsumedQuota, relayInfo))
	session, ok := relayInfo.Billing.(*BillingSession)
	require.True(t, ok)
	return session, ctx
}

func billingAdmissionOperationsForRequest(t *testing.T, requestID string) []model.BillingAdmissionReserveOperation {
	t.Helper()
	var operations []model.BillingAdmissionReserveOperation
	require.NoError(t, model.DB.Where("request_id = ? AND mode <> ?", requestID, model.BillingAdmissionReserveModeInitial).Order("attempt ASC").Find(&operations).Error)
	return operations
}

func billingInitialAdmissionOperationForRequest(t *testing.T, requestID string) model.BillingAdmissionReserveOperation {
	t.Helper()
	var operations []model.BillingAdmissionReserveOperation
	require.NoError(t, model.DB.Where("request_id = ? AND mode = ?", requestID, model.BillingAdmissionReserveModeInitial).Find(&operations).Error)
	require.Len(t, operations, 1)
	return operations[0]
}

func assertAdmissionQuotaState(t *testing.T, userID, wantUser, wantToken int) {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, wantUser, user.Quota)
	var token model.Token
	require.NoError(t, model.DB.First(&token, userID+1).Error)
	assert.Equal(t, wantToken, token.RemainQuota)
}

func injectBillingUpdateError(t *testing.T, callbackName, table string, forcedErr error) {
	t.Helper()
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() {
		_ = model.DB.Callback().Update().Remove(callbackName)
	})
}

func injectBillingAdmissionStatusError(t *testing.T, callbackName, status string, forcedErr error) {
	t.Helper()
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "billing_admission_reserve_operations" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if ok && updates["status"] == status {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() {
		_ = model.DB.Callback().Update().Remove(callbackName)
	})
}

func refundAdmissionSessionAndWait(t *testing.T, session *BillingSession, ctx *gin.Context, userID int) {
	t.Helper()
	userRefunded := make(chan struct{}, 1)
	tokenRefunded := make(chan struct{}, 1)
	callbackName := fmt.Sprintf("test:admission_reserve_refund_barrier_%d", userID)
	require.NoError(t, model.DB.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		switch tx.Statement.Table {
		case "users":
			select {
			case userRefunded <- struct{}{}:
			default:
			}
		case "tokens":
			select {
			case tokenRefunded <- struct{}{}:
			default:
			}
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	session.Refund(ctx)
	select {
	case <-userRefunded:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "initial wallet pre-consume refund did not complete")
	}
	select {
	case <-tokenRefunded:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "initial token pre-consume refund did not complete")
	}
}

func TestBillingSessionReserveBeginErrorDoesNotBlockInitialRefund(t *testing.T) {
	const (
		requestID = "admission-reserve-begin-error"
		userID    = 98100
	)
	session, ctx := newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected admission operation begin failure")
	callbackName := "test:admission_reserve_begin_error"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_admission_reserve_operations" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Create().Remove(callbackName) })

	err := session.Reserve(200)

	assert.ErrorIs(t, err, forcedErr)
	assert.NotErrorIs(t, err, ErrBillingSessionRequiresReconciliation)
	assert.Empty(t, billingAdmissionOperationsForRequest(t, requestID))
	assertAdmissionQuotaState(t, userID, 400, 400)
	assert.True(t, session.NeedsRefund(), "no top-up external action started, so the initial pre-consume remains refundable")
	refundAdmissionSessionAndWait(t, session, ctx, userID)
	assertAdmissionQuotaState(t, userID, 500, 500)
}

func TestBillingSessionInitialPreConsumePersistsAppliedAdmissionEvidence(t *testing.T) {
	const (
		requestID = "admission-initial-applied"
		userID    = 98070
	)
	session, _ := newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)

	operation := billingInitialAdmissionOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingAdmissionReserveStatusApplied, operation.Status)
	assert.Empty(t, operation.PendingAction)
	assert.EqualValues(t, 0, operation.Attempt)
	assert.EqualValues(t, 0, operation.FromQuota)
	assert.EqualValues(t, 100, operation.TargetQuota)
	assert.EqualValues(t, 100, operation.FundingReservedQuota)
	assert.EqualValues(t, 100, operation.TokenReservedQuota)
	assert.Equal(t, 100, session.GetPreConsumedQuota())
	assertAdmissionQuotaState(t, userID, 400, 400)
}

func TestBillingSessionInitialBeginFailureStartsNoBalanceAction(t *testing.T) {
	const (
		requestID = "admission-initial-begin-failure"
		userID    = 98071
	)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: fmt.Sprintf("admission-initial-%d", userID), Status: common.UserStatusEnabled, Quota: 500, AffCode: fmt.Sprintf("admission-initial-aff-%d", userID)}).Error)
	key := fmt.Sprintf("admission-initial-token-%d", userID)
	seedToken(t, userID+1, userID, key, 500)
	forcedErr := errors.New("injected initial admission begin failure")
	callbackName := "test:admission_initial_begin_failure"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_admission_reserve_operations" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Create().Remove(callbackName) })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId: requestID, UserId: userID, TokenId: userID + 1, TokenKey: key, ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
	}

	apiErr := PreConsumeBilling(ctx, 100, relayInfo)

	require.NotNil(t, apiErr)
	assert.ErrorIs(t, apiErr, forcedErr)
	assertAdmissionQuotaState(t, userID, 500, 500)
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingAdmissionReserveOperation{}).Where("request_id = ?", requestID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestBillingSessionInitialFundingFailureIsDurableUnknownBeforeTokenAction(t *testing.T) {
	const (
		requestID = "admission-initial-funding-unknown"
		userID    = 98072
	)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: fmt.Sprintf("admission-initial-%d", userID), Status: common.UserStatusEnabled, Quota: 500, AffCode: fmt.Sprintf("admission-initial-aff-%d", userID)}).Error)
	key := fmt.Sprintf("admission-initial-token-%d", userID)
	seedToken(t, userID+1, userID, key, 500)
	forcedErr := errors.New("injected initial funding reserve failure")
	injectBillingUpdateError(t, "test:admission_initial_funding_unknown", "users", forcedErr)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId: requestID, UserId: userID, TokenId: userID + 1, TokenKey: key, ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
	}

	apiErr := PreConsumeBilling(ctx, 100, relayInfo)

	require.NotNil(t, apiErr)
	assert.ErrorIs(t, apiErr, ErrBillingSessionRequiresReconciliation)
	operation := billingInitialAdmissionOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingAdmissionReservePendingActionFundingUnknown, operation.PendingAction)
	assert.Zero(t, operation.FundingReservedQuota)
	assert.Zero(t, operation.TokenReservedQuota)
	assertAdmissionQuotaState(t, userID, 500, 500)
	assert.Nil(t, relayInfo.Billing)
	ageBillingReconciliationRow(t, "billing_admission_reserve_operations", operation.OperationID)
	summary := runBillingReconciliationOnce(time.Now())
	assert.Equal(t, 1, summary.ManualAdmissionCount)
	stillUnknown, err := model.GetBillingAdmissionReserveOperation(operation.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingAdmissionReservePendingActionFundingUnknown, stillUnknown.PendingAction)
	assertAdmissionQuotaState(t, userID, 500, 500)
}

func TestBillingSessionInitialTokenFailureLeavesConfirmedFundingAndUnknownToken(t *testing.T) {
	const (
		requestID = "admission-initial-token-unknown"
		userID    = 98074
	)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: fmt.Sprintf("admission-initial-%d", userID), Status: common.UserStatusEnabled, Quota: 500, AffCode: fmt.Sprintf("admission-initial-aff-%d", userID)}).Error)
	key := fmt.Sprintf("admission-initial-token-%d", userID)
	seedToken(t, userID+1, userID, key, 500)
	forcedErr := errors.New("injected initial token reserve failure")
	injectBillingUpdateError(t, "test:admission_initial_token_unknown", "tokens", forcedErr)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId: requestID, UserId: userID, TokenId: userID + 1, TokenKey: key, ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
	}

	apiErr := PreConsumeBilling(ctx, 100, relayInfo)

	require.NotNil(t, apiErr)
	assert.ErrorIs(t, apiErr, ErrBillingSessionRequiresReconciliation)
	operation := billingInitialAdmissionOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingAdmissionReservePendingActionTokenUnknown, operation.PendingAction)
	assert.Equal(t, 100, operation.FundingReservedQuota)
	assert.Zero(t, operation.TokenReservedQuota)
	assertAdmissionQuotaState(t, userID, 400, 500)
	assert.Nil(t, relayInfo.Billing)
}

func TestBillingSessionStaleTokenClaimCannotClaimWorkerFundingRefundStage(t *testing.T) {
	const (
		requestID = "admission-initial-stale-token-claim"
		userID    = 98075
	)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		billingBeforeActionClaimHook = nil
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: fmt.Sprintf("admission-initial-%d", userID), Status: common.UserStatusEnabled, Quota: 500, AffCode: fmt.Sprintf("admission-initial-aff-%d", userID)}).Error)
	key := fmt.Sprintf("admission-initial-token-%d", userID)
	seedToken(t, userID+1, userID, key, 500)

	tokenClaimPaused := make(chan struct{})
	resumeTokenClaim := make(chan struct{})
	fundingRefundClaimPaused := make(chan struct{})
	resumeFundingRefundClaim := make(chan struct{})
	var hookMu sync.Mutex
	tokenClaimBlocked := false
	fundingRefundClaimBlocked := false
	billingBeforeActionClaimHook = func(_ string, expectedReadyAction string) {
		hookMu.Lock()
		blockToken := expectedReadyAction == model.BillingAdmissionReservePendingActionTokenReady && !tokenClaimBlocked
		if blockToken {
			tokenClaimBlocked = true
		}
		blockFundingRefund := expectedReadyAction == model.BillingAdmissionReservePendingActionFundingRefundReady && !fundingRefundClaimBlocked
		if blockFundingRefund {
			fundingRefundClaimBlocked = true
		}
		hookMu.Unlock()
		if blockToken {
			close(tokenClaimPaused)
			<-resumeTokenClaim
		}
		if blockFundingRefund {
			close(fundingRefundClaimPaused)
			<-resumeFundingRefundClaim
		}
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId: requestID, UserId: userID, TokenId: userID + 1, TokenKey: key, ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
	}
	preConsumeDone := make(chan *types.NewAPIError, 1)
	go func() { preConsumeDone <- PreConsumeBilling(ctx, 100, relayInfo) }()
	<-tokenClaimPaused

	operation := billingInitialAdmissionOperationForRequest(t, requestID)
	reconcileDone := make(chan error, 1)
	go func() {
		_, reconcileErr := ReconcilePendingBillingAdmissionReserveOperation(operation.OperationID)
		reconcileDone <- reconcileErr
	}()
	<-fundingRefundClaimPaused

	close(resumeTokenClaim)
	apiErr := <-preConsumeDone
	require.NotNil(t, apiErr)
	assert.ErrorIs(t, apiErr, ErrBillingSessionRequiresReconciliation)
	close(resumeFundingRefundClaim)
	require.NoError(t, <-reconcileDone)

	operation, err := model.GetBillingAdmissionReserveOperation(operation.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingAdmissionReserveStatusCanceled, operation.Status)
	assert.Equal(t, 100, operation.FundingReservedQuota)
	assert.Equal(t, 100, operation.FundingRefundedQuota)
	assert.Zero(t, operation.TokenReservedQuota)
	assertAdmissionQuotaState(t, userID, 500, 500)
}

func TestBillingSessionInitialTokenInsufficientCompensatesFundingThroughAdmissionLedger(t *testing.T) {
	const (
		requestID = "admission-initial-token-insufficient"
		userID    = 98076
	)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: fmt.Sprintf("admission-initial-%d", userID), Status: common.UserStatusEnabled, Quota: 500, AffCode: fmt.Sprintf("admission-initial-aff-%d", userID)}).Error)
	key := fmt.Sprintf("admission-initial-token-%d", userID)
	seedToken(t, userID+1, userID, key, 50)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId: requestID, UserId: userID, TokenId: userID + 1, TokenKey: key, ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
	}

	apiErr := PreConsumeBilling(ctx, 100, relayInfo)

	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodePreConsumeTokenQuotaFailed, apiErr.GetErrorCode())
	operation := billingInitialAdmissionOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingAdmissionReserveStatusCanceled, operation.Status)
	assert.Equal(t, 100, operation.FundingReservedQuota)
	assert.Equal(t, 100, operation.FundingRefundedQuota)
	assert.Zero(t, operation.TokenReservedQuota)
	assertAdmissionQuotaState(t, userID, 500, 50)
}

func TestBillingSessionInitialCompensationFailureBlocksWalletFirstFallback(t *testing.T) {
	const (
		requestID = "admission-initial-compensation-failure"
		userID    = 98077
	)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: fmt.Sprintf("admission-initial-%d", userID), Status: common.UserStatusEnabled, Quota: 500, AffCode: fmt.Sprintf("admission-initial-aff-%d", userID)}).Error)
	key := fmt.Sprintf("admission-initial-token-%d", userID)
	seedToken(t, userID+1, userID, key, 50)
	forcedErr := errors.New("injected initial funding compensation failure")
	callbackName := "test:admission_initial_compensation_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]interface{})
		if ok && strings.Contains(fmt.Sprint(updates["quota"]), "quota +") {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId: requestID, UserId: userID, TokenId: userID + 1, TokenKey: key, ForcePreConsume: true,
		UserSetting: dto.UserSetting{BillingPreference: "wallet_first"},
	}

	apiErr := PreConsumeBilling(ctx, 100, relayInfo)

	require.NotNil(t, apiErr)
	assert.ErrorIs(t, apiErr, forcedErr)
	assert.ErrorIs(t, apiErr, ErrBillingSessionRequiresReconciliation)
	assert.Equal(t, types.ErrorCodeUpdateDataError, apiErr.GetErrorCode())
	var operations []model.BillingAdmissionReserveOperation
	require.NoError(t, model.DB.Where("request_id = ? AND mode = ?", requestID, model.BillingAdmissionReserveModeInitial).Find(&operations).Error)
	require.Len(t, operations, 1, "manual compensation must block subscription fallback")
	assert.Equal(t, BillingSourceWallet, operations[0].FundingSource)
	assert.Equal(t, model.BillingAdmissionReservePendingActionFundingRefundUnknown, operations[0].PendingAction)
	assert.Equal(t, 100, operations[0].FundingReservedQuota)
	assert.Zero(t, operations[0].FundingRefundedQuota)
	assertAdmissionQuotaState(t, userID, 400, 50)
}

func TestBillingSessionWalletFirstRaceFallsBackWithoutTokenRollbackOrDoubleDebit(t *testing.T) {
	const (
		requestID      = "admission-initial-wallet-first-race"
		userID         = 98078
		tokenID        = userID + 1
		subscriptionID = userID + 2
		planID         = userID + 3
	)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.SubscriptionPreConsumeRecord{}).Error)
	require.NoError(t, model.DB.Where("id = ?", subscriptionID).Delete(&model.UserSubscription{}).Error)
	require.NoError(t, model.DB.Where("id = ?", planID).Delete(&model.SubscriptionPlan{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", tokenID).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
		model.DB.Where("request_id = ?", requestID).Delete(&model.SubscriptionPreConsumeRecord{})
		model.DB.Where("id = ?", subscriptionID).Delete(&model.UserSubscription{})
		model.DB.Where("id = ?", planID).Delete(&model.SubscriptionPlan{})
		model.DB.Unscoped().Where("id = ?", tokenID).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{
		Id: userID, Username: fmt.Sprintf("admission-initial-%d", userID), Status: common.UserStatusEnabled,
		Quota: 500, AffCode: fmt.Sprintf("admission-initial-aff-%d", userID),
	}).Error)
	key := fmt.Sprintf("admission-initial-token-%d", userID)
	seedToken(t, tokenID, userID, key, 500)
	allowOverflow := true
	require.NoError(t, model.DB.Create(&model.SubscriptionPlan{
		Id: planID, Title: "wallet-first-race", Enabled: true, TotalAmount: 1_000,
		DurationUnit: "month", DurationValue: 1, AllowWalletOverflow: &allowOverflow,
	}).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id: subscriptionID, UserId: userID, PlanId: planID, AmountTotal: 1_000,
		Status: "active", StartTime: time.Now().Unix(), EndTime: time.Now().Add(24 * time.Hour).Unix(),
		AllowWalletOverflow: true,
	}).Error)

	userCallback := "test:admission_initial_wallet_first_race"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(userCallback, func(tx *gorm.DB) {
		if tx.Statement.Table != "users" {
			return
		}
		_, err := tx.Statement.ConnPool.ExecContext(context.Background(), "UPDATE users SET quota = 0 WHERE id = ?", userID)
		if err != nil {
			tx.AddError(err)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(userCallback) })

	var tokenUpdates int
	tokenCallback := "test:admission_initial_wallet_first_token_updates"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(tokenCallback, func(tx *gorm.DB) {
		if tx.Statement.Table != "tokens" {
			return
		}
		tokenUpdates++
		if tokenUpdates == 2 {
			tx.AddError(errors.New("legacy token rollback would fail"))
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(tokenCallback) })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId: requestID, UserId: userID, TokenId: tokenID, TokenKey: key, ForcePreConsume: true,
		OriginModelName: "gpt-test", UserSetting: dto.UserSetting{BillingPreference: "wallet_first"},
	}

	apiErr := PreConsumeBilling(ctx, 100, relayInfo)

	require.Nil(t, apiErr)
	assert.Equal(t, 1, tokenUpdates, "the rejected wallet attempt must not touch token quota")
	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, 400, token.RemainQuota)
	var subscription model.UserSubscription
	require.NoError(t, model.DB.First(&subscription, subscriptionID).Error)
	assert.EqualValues(t, 100, subscription.AmountUsed)
	var operations []model.BillingAdmissionReserveOperation
	require.NoError(t, model.DB.Where("request_id = ? AND mode = ?", requestID, model.BillingAdmissionReserveModeInitial).Order("id ASC").Find(&operations).Error)
	require.Len(t, operations, 2)
	assert.Equal(t, model.BillingAdmissionReserveStatusCanceled, operations[0].Status)
	assert.Equal(t, BillingSourceWallet, operations[0].FundingSource)
	assert.Equal(t, model.BillingAdmissionReserveStatusApplied, operations[1].Status)
	assert.Equal(t, BillingSourceSubscription, operations[1].FundingSource)
	assert.EqualValues(t, subscriptionID, operations[1].FundingReferenceID)
}

func TestBillingSessionReserveFundingErrorPersistsPendingAndDisablesAutomaticActions(t *testing.T) {
	const (
		requestID = "admission-reserve-funding-error"
		userID    = 98110
	)
	session, ctx := newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected wallet reserve failure")
	injectBillingUpdateError(t, "test:admission_reserve_funding_error", "users", forcedErr)

	err := session.Reserve(200)

	assert.ErrorIs(t, err, forcedErr)
	assert.ErrorIs(t, err, ErrBillingSessionRequiresReconciliation)
	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 1)
	assert.Equal(t, model.BillingAdmissionReserveStatusPendingReconcile, operations[0].Status)
	assert.Equal(t, model.BillingAdmissionReservePendingActionFundingUnknown, operations[0].PendingAction)
	assert.EqualValues(t, 100, operations[0].DeltaQuota)
	assertAdmissionQuotaState(t, userID, 400, 400)
	assert.False(t, session.NeedsRefund())
	session.Refund(ctx)
	assert.False(t, session.refunded)
	assert.ErrorIs(t, session.Reserve(250), ErrBillingSessionRequiresReconciliation)
	assert.ErrorIs(t, session.Settle(100), ErrBillingSessionRequiresReconciliation)
}

func TestBillingSessionReserveTokenErrorPersistsPendingWithoutFundingRollback(t *testing.T) {
	const (
		requestID = "admission-reserve-token-error"
		userID    = 98120
	)
	session, ctx := newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected token reserve failure")
	injectBillingUpdateError(t, "test:admission_reserve_token_error", "tokens", forcedErr)

	err := session.Reserve(200)

	assert.ErrorIs(t, err, forcedErr)
	assert.ErrorIs(t, err, ErrBillingSessionRequiresReconciliation)
	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 1)
	assert.Equal(t, model.BillingAdmissionReserveStatusPendingReconcile, operations[0].Status)
	assert.Equal(t, model.BillingAdmissionReservePendingActionTokenUnknown, operations[0].PendingAction)
	assert.Equal(t, 100, operations[0].FundingReservedQuota)
	assert.Zero(t, operations[0].TokenReservedQuota)
	assertAdmissionQuotaState(t, userID, 300, 400)
	assert.Equal(t, 100, session.GetPreConsumedQuota(), "the uncertain top-up cannot become the next automatic cursor")
	assert.False(t, session.NeedsRefund())
	session.Refund(ctx)
	assert.False(t, session.refunded, "Refund cannot guess whether the pending top-up must be compensated")
}

func TestBillingSessionReserveTokenInsufficientDurablyCompensatesConfirmedFunding(t *testing.T) {
	const (
		requestID = "admission-reserve-token-insufficient"
		userID    = 98125
	)
	session, _ := newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", userID+1).Update("remain_quota", 50).Error)

	err := session.Reserve(200)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrBillingSessionRequiresReconciliation)
	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 1)
	assert.Equal(t, model.BillingAdmissionReserveStatusCanceled, operations[0].Status)
	assert.Equal(t, 100, operations[0].FundingReservedQuota)
	assert.Equal(t, 100, operations[0].FundingRefundedQuota)
	assert.Zero(t, operations[0].TokenReservedQuota)
	assertAdmissionQuotaState(t, userID, 400, 50)
	assert.Equal(t, 100, session.GetPreConsumedQuota())
	assert.True(t, session.NeedsRefund())
}

func TestBillingSessionReserveOperationCommitErrorKeepsDurablePending(t *testing.T) {
	const (
		requestID = "admission-reserve-commit-error"
		userID    = 98130
	)
	session, ctx := newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected admission operation commit failure")
	injectBillingAdmissionStatusError(t, "test:admission_reserve_commit_error", model.BillingAdmissionReserveStatusApplied, forcedErr)

	err := session.Reserve(200)

	assert.ErrorIs(t, err, forcedErr)
	assert.ErrorIs(t, err, ErrBillingSessionRequiresReconciliation)
	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 1)
	assert.Equal(t, model.BillingAdmissionReserveStatusPendingReconcile, operations[0].Status)
	assert.Equal(t, model.BillingAdmissionReservePendingActionCommitAfterReserve, operations[0].PendingAction)
	assert.Equal(t, 100, operations[0].FundingReservedQuota)
	assert.Equal(t, 100, operations[0].TokenReservedQuota)
	assertAdmissionQuotaState(t, userID, 300, 300)
	assert.Equal(t, 100, session.GetPreConsumedQuota())
	assert.False(t, session.NeedsRefund())
	session.Refund(ctx)
	assert.False(t, session.refunded)
}

func TestBillingSessionStrictInsufficientCancelsOperationAndRefundsInitialPreconsume(t *testing.T) {
	const (
		requestID = "admission-reserve-known-insufficient"
		userID    = 98140
	)
	session, ctx := newAdmissionReserveWalletSession(t, requestID, userID, 150, 100)

	err := session.ReserveForAdmission(200)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrBillingSessionRequiresReconciliation)
	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 1)
	assert.Equal(t, model.BillingAdmissionReserveStatusCanceled, operations[0].Status)
	assert.Equal(t, model.BillingAdmissionReserveModeStrictWallet, operations[0].Mode)
	assertAdmissionQuotaState(t, userID, 50, 50)
	assert.True(t, session.NeedsRefund())

	refundAdmissionSessionAndWait(t, session, ctx, userID)
	assertAdmissionQuotaState(t, userID, 150, 150)
}

func TestBillingSessionStrictInsufficientCancelErrorKeepsPendingEvidenceWithoutBlockingInitialRefund(t *testing.T) {
	const (
		requestID = "admission-reserve-insufficient-cancel-error"
		userID    = 98145
	)
	session, ctx := newAdmissionReserveWalletSession(t, requestID, userID, 150, 100)
	forcedErr := errors.New("injected admission operation cancel failure")
	injectBillingAdmissionStatusError(t, "test:admission_reserve_cancel_error", model.BillingAdmissionReserveStatusCanceled, forcedErr)

	err := session.ReserveForAdmission(200)

	require.Error(t, err)
	assert.ErrorIs(t, err, forcedErr)
	assert.NotErrorIs(t, err, ErrBillingSessionRequiresReconciliation)
	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 1)
	assert.Equal(t, model.BillingAdmissionReserveStatusPendingReconcile, operations[0].Status)
	assert.Equal(t, model.BillingAdmissionReservePendingActionFundingUnknown, operations[0].PendingAction)
	assertAdmissionQuotaState(t, userID, 50, 50)
	assert.True(t, session.NeedsRefund(), "known insufficient means the top-up did not move, even if audit cancel failed")
	refundAdmissionSessionAndWait(t, session, ctx, userID)
	assertAdmissionQuotaState(t, userID, 150, 150)
}

func TestBillingSessionAdmissionReserveNeverMarksSubscriptionAsStrictWallet(t *testing.T) {
	const requestID = "admission-reserve-subscription-mode"
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingAdmissionReserveOperation{})
	})
	relayInfo := &relaycommon.RelayInfo{
		RequestId:    requestID,
		UserId:       98148,
		IsPlayground: true,
	}
	session := &BillingSession{
		relayInfo:        relayInfo,
		funding:          &SubscriptionFunding{userId: relayInfo.UserId, preConsumed: 100},
		preConsumedQuota: 100,
		tokenConsumed:    100,
	}

	err := session.ReserveForAdmission(200)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBillingSessionRequiresReconciliation)
	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 1)
	assert.Equal(t, model.BillingAdmissionReserveModeStandard, operations[0].Mode)
	assert.Equal(t, BillingSourceSubscription, operations[0].FundingSource)
	assert.Equal(t, model.BillingAdmissionReserveStatusPendingReconcile, operations[0].Status)
	assert.Equal(t, model.BillingAdmissionReservePendingActionFundingUnknown, operations[0].PendingAction)
}

func TestBillingSessionAppliedTopUpsAdvanceFromLastTarget(t *testing.T) {
	const (
		requestID = "admission-reserve-applied-sequence"
		userID    = 98150
	)
	session, _ := newAdmissionReserveWalletSession(t, requestID, userID, 1_000, 100)

	require.NoError(t, session.Reserve(200))
	require.NoError(t, session.Reserve(300))

	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 2)
	assert.EqualValues(t, 1, operations[0].Attempt)
	assert.EqualValues(t, 100, operations[0].FromQuota)
	assert.EqualValues(t, 200, operations[0].TargetQuota)
	assert.Equal(t, model.BillingAdmissionReserveStatusApplied, operations[0].Status)
	assert.Empty(t, operations[0].PendingAction)
	assert.EqualValues(t, 100, operations[0].FundingReservedQuota)
	assert.EqualValues(t, 100, operations[0].TokenReservedQuota)
	assert.EqualValues(t, 2, operations[1].Attempt)
	assert.EqualValues(t, 200, operations[1].FromQuota)
	assert.EqualValues(t, 300, operations[1].TargetQuota)
	assert.Equal(t, model.BillingAdmissionReserveStatusApplied, operations[1].Status)
	assert.Empty(t, operations[1].PendingAction)
	assert.Equal(t, operations[0].SessionID, operations[1].SessionID)
	assertAdmissionQuotaState(t, userID, 700, 700)
}

func TestBillingSessionRequestReplayUsesDistinctAdmissionSession(t *testing.T) {
	const (
		requestID = "admission-reserve-request-replay"
		userID    = 98160
	)
	first, ctx := newAdmissionReserveWalletSession(t, requestID, userID, 1_000, 100)
	require.NoError(t, first.Reserve(200))

	replayInfo := &relaycommon.RelayInfo{
		RequestId:       requestID,
		UserId:          userID,
		TokenId:         userID + 1,
		TokenKey:        fmt.Sprintf("admission-reserve-token-%d", userID),
		ForcePreConsume: true,
		UserSetting: dto.UserSetting{
			BillingPreference: "wallet_only",
		},
	}
	require.Nil(t, PreConsumeBilling(ctx, 100, replayInfo))
	replay, ok := replayInfo.Billing.(*BillingSession)
	require.True(t, ok)
	require.NoError(t, replay.Reserve(200))

	operations := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operations, 2)
	assert.NotEqual(t, operations[0].OperationID, operations[1].OperationID)
	assert.NotEqual(t, operations[0].SessionID, operations[1].SessionID)
	assert.EqualValues(t, 1, operations[0].Attempt)
	assert.EqualValues(t, 1, operations[1].Attempt)
	assertAdmissionQuotaState(t, userID, 600, 600)
}

func TestBillingSessionAdmissionTopUpPersistsBeforeLedgerConfirmationWhenBatchUpdatesEnabled(t *testing.T) {
	const (
		requestID = "admission-reserve-batch-durable"
		userID    = 98170
	)
	session, _ := newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	wasBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = wasBatchEnabled })

	require.NoError(t, session.Reserve(200))

	assertAdmissionQuotaState(t, userID, 300, 300)
	operation := billingAdmissionOperationsForRequest(t, requestID)
	require.Len(t, operation, 1)
	assert.Equal(t, model.BillingAdmissionReserveStatusApplied, operation[0].Status)
}

func TestBillingSessionSettlementPersistsBeforeReturningWhenBatchUpdatesEnabled(t *testing.T) {
	const (
		requestID = "billing-settle-batch-durable"
		userID    = 98180
	)
	session, _ := newAdmissionReserveWalletSession(t, requestID, userID, 500, 100)
	wasBatchEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = wasBatchEnabled })

	require.NoError(t, session.Settle(200))

	assertAdmissionQuotaState(t, userID, 300, 300)
}
