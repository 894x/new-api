package service

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newBillingRefundWalletSession(
	t *testing.T,
	requestID string,
	userID int,
	startingQuota int,
	preConsumedQuota int,
) (*BillingSession, *gin.Context) {
	t.Helper()
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingRefundOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingRefundOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})

	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: fmt.Sprintf("billing-refund-%d", userID),
		Status:   common.UserStatusEnabled,
		Quota:    startingQuota,
		AffCode:  fmt.Sprintf("billing-refund-aff-%d", userID),
	}).Error)
	seedToken(t, userID+1, userID, fmt.Sprintf("billing-refund-token-%d", userID), startingQuota)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId:       requestID,
		UserId:          userID,
		TokenId:         userID + 1,
		TokenKey:        fmt.Sprintf("billing-refund-token-%d", userID),
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

func newBillingRefundSubscriptionSession(
	t *testing.T,
	requestID string,
	userID int,
	initialQuota int,
	extraQuota int,
) (*BillingSession, *gin.Context, int) {
	t.Helper()
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingRefundOperation{}).Error)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.SubscriptionPreConsumeRecord{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Where("id = ?", userID+2).Delete(&model.UserSubscription{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingRefundOperation{})
		model.DB.Where("request_id = ?", requestID).Delete(&model.SubscriptionPreConsumeRecord{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Where("id = ?", userID+2).Delete(&model.UserSubscription{})
	})

	subscriptionID := userID + 2
	totalQuota := initialQuota + extraQuota
	seedToken(t, userID+1, userID, fmt.Sprintf("billing-refund-sub-token-%d", userID), 1_000-totalQuota)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", userID+1).Update("used_quota", totalQuota).Error)
	seedSubscription(t, subscriptionID, userID, 10_000, int64(totalQuota))
	require.NoError(t, model.DB.Create(&model.SubscriptionPreConsumeRecord{
		RequestId:          requestID,
		UserId:             userID,
		UserSubscriptionId: subscriptionID,
		PreConsumed:        int64(initialQuota),
		Status:             "consumed",
	}).Error)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId:      requestID,
		UserId:         userID,
		TokenId:        userID + 1,
		TokenKey:       fmt.Sprintf("billing-refund-sub-token-%d", userID),
		SubscriptionId: subscriptionID,
	}
	session := &BillingSession{
		relayInfo: relayInfo,
		funding: &SubscriptionFunding{
			requestId:      requestID,
			userId:         userID,
			subscriptionId: subscriptionID,
			preConsumed:    int64(initialQuota),
		},
		preConsumedQuota: totalQuota,
		tokenConsumed:    totalQuota,
		extraReserved:    extraQuota,
	}
	return session, ctx, subscriptionID
}

func waitBillingRefundRun(t *testing.T, session *BillingSession) {
	t.Helper()
	require.Eventually(t, func() bool {
		session.mu.Lock()
		defer session.mu.Unlock()
		return session.refunded || session.requiresReconciliation
	}, 3*time.Second, 5*time.Millisecond, "billing refund operation did not finish")
}

func billingRefundOperationForRequest(t *testing.T, requestID string) model.BillingRefundOperation {
	t.Helper()
	var operations []model.BillingRefundOperation
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Find(&operations).Error)
	require.Len(t, operations, 1)
	return operations[0]
}

func assertBillingRefundWalletQuota(t *testing.T, userID, wantUserQuota, wantTokenQuota int) {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, wantUserQuota, user.Quota)
	var token model.Token
	require.NoError(t, model.DB.First(&token, userID+1).Error)
	assert.Equal(t, wantTokenQuota, token.RemainQuota)
}

func assertBillingRefundSubscriptionQuota(t *testing.T, subscriptionID int, wantUsed int64, tokenID, wantTokenQuota int) {
	t.Helper()
	var subscription model.UserSubscription
	require.NoError(t, model.DB.First(&subscription, subscriptionID).Error)
	assert.Equal(t, wantUsed, subscription.AmountUsed)
	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	assert.Equal(t, wantTokenQuota, token.RemainQuota)
}

func injectBillingRefundCommitError(t *testing.T, callbackName string, forcedErr error) {
	t.Helper()
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "billing_refund_operations" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if ok && updates["status"] == model.BillingRefundStatusApplied {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })
}

func TestBillingSessionRefundBeginKnownNotWrittenFailsClosedWithoutUnauditedFallback(t *testing.T) {
	const (
		requestID = "billing-refund-begin-error"
		userID    = 98300
	)
	session, ctx := newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected refund operation begin error")
	callbackName := "test:billing_refund_begin_error"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_refund_operations" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Create().Remove(callbackName) })

	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	assertBillingRefundWalletQuota(t, userID, 400, 400)
	assert.False(t, session.NeedsRefund())
	assert.False(t, session.refunded)
	assert.True(t, session.requiresReconciliation)
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingRefundOperation{}).Where("request_id = ?", requestID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestBillingSessionRefundNeverCreditsAnUnflushedBatchPreConsume(t *testing.T) {
	const (
		requestID = "billing-refund-batch-preconsume-durable"
		userID    = 98307
	)
	server := miniredis.RunT(t)
	oldRedisEnabled := common.RedisEnabled
	oldRedisClient := common.RDB
	oldBatchEnabled := common.BatchUpdateEnabled
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.BatchUpdateEnabled = true
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRedisClient
		common.BatchUpdateEnabled = oldBatchEnabled
	})

	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.BillingRefundOperation{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{}).Error)
	require.NoError(t, model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{}).Error)
	t.Cleanup(func() {
		model.DB.Where("request_id = ?", requestID).Delete(&model.BillingRefundOperation{})
		model.DB.Unscoped().Where("id = ?", userID+1).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", userID).Delete(&model.User{})
	})
	require.NoError(t, model.DB.Create(&model.User{
		Id:          userID,
		Username:    fmt.Sprintf("billing-refund-%d", userID),
		Status:      common.UserStatusEnabled,
		Quota:       500,
		AuthVersion: 1,
		AffCode:     fmt.Sprintf("billing-refund-aff-%d", userID),
	}).Error)
	tokenKey := fmt.Sprintf("billing-refund-token-%d", userID)
	seedToken(t, userID+1, userID, tokenKey, 500)
	require.NoError(t, common.RDB.HSet(context.Background(), fmt.Sprintf("user:%d", userID), map[string]any{
		"Id":          userID,
		"Quota":       500,
		"CacheSchema": 2,
	}).Err())
	require.NoError(t, common.RDB.HSet(context.Background(), "token:"+common.GenerateHMAC(tokenKey), map[string]any{
		"Id":          userID + 1,
		"RemainQuota": 500,
		"UsedQuota":   0,
	}).Err())

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		RequestId:       requestID,
		UserId:          userID,
		TokenId:         userID + 1,
		TokenKey:        tokenKey,
		ForcePreConsume: true,
		UserSetting: dto.UserSetting{
			BillingPreference: "wallet_only",
		},
	}
	require.Nil(t, PreConsumeBilling(ctx, 100, relayInfo))
	session, ok := relayInfo.Billing.(*BillingSession)
	require.True(t, ok)
	assertBillingRefundWalletQuota(t, userID, 400, 400)

	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	operation := billingRefundOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingRefundStatusApplied, operation.Status)
	assertBillingRefundWalletQuota(t, userID, 500, 500)

}

func TestFixedGroupSettlementPersistsPositiveAndNegativeDeltaWhenRedisIsUnavailable(t *testing.T) {
	tests := []struct {
		name        string
		requestID   string
		userID      int
		actualQuota int
		wantQuota   int
	}{
		{name: "additional debit", requestID: "fixed-settle-redis-down-debit", userID: 98308, actualQuota: 200, wantQuota: 300},
		{name: "partial refund", requestID: "fixed-settle-redis-down-refund", userID: 98309, actualQuota: 80, wantQuota: 420},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			oldRedisEnabled := common.RedisEnabled
			oldRedisClient := common.RDB
			common.RedisEnabled = true
			common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() {
				_ = common.RDB.Close()
				common.RedisEnabled = oldRedisEnabled
				common.RDB = oldRedisClient
			})

			session, ctx := newBillingRefundWalletSession(t, test.requestID, test.userID, 500, 100)
			session.relayInfo.UserQuota = 1_000_000
			server.Close()

			decision, err := SettleModelCharge(ctx, session.relayInfo, test.requestID, test.actualQuota, test.actualQuota)

			require.NoError(t, err)
			assert.False(t, decision.RequiresReconciliation)
			assert.True(t, session.settled)
			assertAdmissionQuotaState(t, test.userID, test.wantQuota, test.wantQuota)
		})
	}
}

func TestBillingSessionRefundInvalidIdentityFailsClosedWithoutUnauditedFallback(t *testing.T) {
	const (
		requestID = "billing-refund-invalid-identity"
		userID    = 98303
	)
	session, ctx := newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	session.relayInfo.UserId = 0

	session.Refund(ctx)

	assertBillingRefundWalletQuota(t, userID, 400, 400)
	assert.True(t, session.requiresReconciliation)
	assert.False(t, session.refunded)
	assert.False(t, session.NeedsRefund())
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingRefundOperation{}).Where("request_id = ?", requestID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestBillingSessionRefundCorruptDurableIntentFailsClosedWithoutFallback(t *testing.T) {
	const (
		requestID   = "billing-refund-corrupt-intent"
		operationID = "billing-refund-corrupt-intent-op"
		sessionID   = "billing-refund-corrupt-intent-session"
		userID      = 98304
	)
	session, ctx := newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	session.refundOperationID = operationID
	session.refundSessionID = sessionID
	operation, err := model.BeginBillingRefundOperation(model.BillingRefundOperationInput{
		OperationID:        operationID,
		SessionID:          sessionID,
		RequestID:          requestID,
		UserID:             userID,
		TokenID:            userID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: int64(userID),
		FundingQuota:       100,
		TokenQuota:         100,
	})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.BillingRefundOperation{}).
		Where("id = ?", operation.Id).
		Update("funding_refunded_quota", 1).Error)
	_, reconcileErr := ReconcilePendingBillingRefundOperation(operationID)
	assert.ErrorIs(t, reconcileErr, ErrBillingRefundRequiresManualReconciliation)
	assert.ErrorIs(t, reconcileErr, model.ErrBillingRefundCorrupt)

	session.Refund(ctx)

	assertBillingRefundWalletQuota(t, userID, 400, 400)
	assert.True(t, session.requiresReconciliation)
	assert.False(t, session.refunded)
	assert.False(t, session.NeedsRefund())
}

func TestReconcilePendingBillingRefundOperationCompletesRecoverableWalletIntent(t *testing.T) {
	const (
		requestID   = "billing-refund-recover-ready"
		operationID = "billing-refund-recover-ready-op"
		userID      = 98305
	)
	_, _ = newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	_, err := model.BeginBillingRefundOperation(model.BillingRefundOperationInput{
		OperationID:        operationID,
		SessionID:          "billing-refund-recover-ready-session",
		RequestID:          requestID,
		UserID:             userID,
		TokenID:            userID + 1,
		FundingSource:      BillingSourceWallet,
		FundingReferenceID: int64(userID),
		FundingQuota:       100,
		TokenQuota:         100,
	})
	require.NoError(t, err)

	operation, err := ReconcilePendingBillingRefundOperation(operationID)

	require.NoError(t, err)
	assert.Equal(t, model.BillingRefundStatusApplied, operation.Status)
	assertBillingRefundWalletQuota(t, userID, 500, 500)
}

func TestBillingSessionRefundWalletErrorStaysDurablyPendingAndNeverBlindlyRetries(t *testing.T) {
	const (
		requestID = "billing-refund-wallet-error"
		userID    = 98310
	)
	session, ctx := newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected wallet refund failure")
	injectBillingUpdateError(t, "test:billing_refund_wallet_error", "users", forcedErr)

	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	operation := billingRefundOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingRefundStatusPendingReconcile, operation.Status)
	assert.Equal(t, model.BillingRefundPendingActionFundingUnknown, operation.PendingAction)
	assert.Zero(t, operation.FundingRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assertBillingRefundWalletQuota(t, userID, 400, 400)
	assert.True(t, session.requiresReconciliation)
	assert.False(t, session.refunded)

	_, err := ReconcilePendingBillingRefundOperation(operation.OperationID)
	assert.ErrorIs(t, err, ErrBillingRefundRequiresManualReconciliation)
	assertBillingRefundWalletQuota(t, userID, 400, 400)
	session.Refund(ctx)
	assertBillingRefundWalletQuota(t, userID, 400, 400)
}

func TestBillingSessionRefundTokenErrorRecordsFundingEvidenceAndStops(t *testing.T) {
	const (
		requestID = "billing-refund-token-error"
		userID    = 98320
	)
	session, ctx := newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected token refund failure")
	injectBillingUpdateError(t, "test:billing_refund_token_error", "tokens", forcedErr)

	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	operation := billingRefundOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingRefundStatusPendingReconcile, operation.Status)
	assert.Equal(t, model.BillingRefundPendingActionTokenUnknown, operation.PendingAction)
	assert.Equal(t, 100, operation.FundingRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assertBillingRefundWalletQuota(t, userID, 500, 400)
	assert.True(t, session.requiresReconciliation)
	assert.False(t, session.refunded)

	_, err := ReconcilePendingBillingRefundOperation(operation.OperationID)
	assert.ErrorIs(t, err, ErrBillingRefundRequiresManualReconciliation)
	assertBillingRefundWalletQuota(t, userID, 500, 400)
	session.Refund(ctx)
	assertBillingRefundWalletQuota(t, userID, 500, 400)
}

func TestBillingSessionRefundCommitErrorPreservesAllExternalEvidence(t *testing.T) {
	const (
		requestID = "billing-refund-commit-error"
		userID    = 98330
	)
	session, ctx := newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected refund operation commit failure")
	injectBillingRefundCommitError(t, "test:billing_refund_commit_error", forcedErr)

	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	operation := billingRefundOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingRefundStatusPendingReconcile, operation.Status)
	assert.Equal(t, model.BillingRefundPendingActionCommitAfterRefund, operation.PendingAction)
	assert.Equal(t, 100, operation.FundingRefundedQuota)
	assert.Equal(t, 100, operation.TokenRefundedQuota)
	assertBillingRefundWalletQuota(t, userID, 500, 500)
	assert.True(t, session.requiresReconciliation)
	assert.False(t, session.refunded)

	require.NoError(t, model.DB.Callback().Update().Remove("test:billing_refund_commit_error"))
	reconciled, err := ReconcilePendingBillingRefundOperation(operation.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingRefundStatusApplied, reconciled.Status)
	assertBillingRefundWalletQuota(t, userID, 500, 500)
	session.Refund(ctx)
	assertBillingRefundWalletQuota(t, userID, 500, 500)
}

func TestBillingSessionRefundRetriesSafeCommitWithoutRepeatingBalances(t *testing.T) {
	const (
		requestID = "billing-refund-commit-transient"
		userID    = 98335
	)
	session, ctx := newBillingRefundWalletSession(t, requestID, userID, 500, 100)
	forcedErr := errors.New("injected transient refund operation commit failure")
	var failures atomic.Int32
	callbackName := "test:billing_refund_commit_transient"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "billing_refund_operations" {
			return
		}
		updates, ok := tx.Statement.Dest.(map[string]any)
		if ok && updates["status"] == model.BillingRefundStatusApplied && failures.Add(1) == 1 {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	operation := billingRefundOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingRefundStatusApplied, operation.Status)
	assert.Equal(t, int32(2), failures.Load())
	assertBillingRefundWalletQuota(t, userID, 500, 500)
	assert.True(t, session.refunded)
	assert.False(t, session.requiresReconciliation)
}

func TestBillingSessionRefundSubscriptionErrorStopsBeforeExtraAndToken(t *testing.T) {
	const (
		requestID = "billing-refund-subscription-error"
		userID    = 98340
	)
	session, ctx, subscriptionID := newBillingRefundSubscriptionSession(t, requestID, userID, 100, 200)
	forcedErr := errors.New("injected subscription initial refund failure")
	var attempts atomic.Int32
	callbackName := "test:billing_refund_subscription_error"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_subscriptions" {
			attempts.Add(1)
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	operation := billingRefundOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingRefundStatusPendingReconcile, operation.Status)
	assert.Equal(t, model.BillingRefundPendingActionFundingUnknown, operation.PendingAction)
	assert.Zero(t, operation.FundingRefundedQuota)
	assert.Zero(t, operation.SubscriptionExtraRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assertBillingRefundSubscriptionQuota(t, subscriptionID, 300, userID+1, 700)
	assert.Equal(t, int32(1), attempts.Load(), "automatic recovery must not replay an unknown external refund")
	assert.True(t, session.requiresReconciliation)

	require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
	reconciled, err := ReconcilePendingBillingRefundOperation(operation.OperationID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingRefundStatusApplied, reconciled.Status)
	assertBillingRefundSubscriptionQuota(t, subscriptionID, 0, userID+1, 1_000)
}

func TestBillingSessionRefundSubscriptionExtraErrorKeepsInitialRefundEvidence(t *testing.T) {
	const (
		requestID = "billing-refund-subscription-extra-error"
		userID    = 98350
	)
	session, ctx, subscriptionID := newBillingRefundSubscriptionSession(t, requestID, userID, 100, 200)
	forcedErr := errors.New("injected subscription extra refund failure")
	var subscriptionUpdates atomic.Int32
	callbackName := "test:billing_refund_subscription_extra_error"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_subscriptions" && subscriptionUpdates.Add(1) == 2 {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })

	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	operation := billingRefundOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingRefundStatusPendingReconcile, operation.Status)
	assert.Equal(t, model.BillingRefundPendingActionSubscriptionExtraUnknown, operation.PendingAction)
	assert.Equal(t, 100, operation.FundingRefundedQuota)
	assert.Zero(t, operation.SubscriptionExtraRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assertBillingRefundSubscriptionQuota(t, subscriptionID, 200, userID+1, 700)
	assert.True(t, session.requiresReconciliation)

	_, err := ReconcilePendingBillingRefundOperation(operation.OperationID)
	assert.ErrorIs(t, err, ErrBillingRefundRequiresManualReconciliation)
	assertBillingRefundSubscriptionQuota(t, subscriptionID, 200, userID+1, 700)
	session.Refund(ctx)
	assertBillingRefundSubscriptionQuota(t, subscriptionID, 200, userID+1, 700)
}

func TestBillingSessionRefundRepeatedCallsMoveEachBalanceOnce(t *testing.T) {
	const (
		requestID = "billing-refund-repeated"
		userID    = 98360
	)
	session, ctx := newBillingRefundWalletSession(t, requestID, userID, 500, 100)

	session.Refund(ctx)
	session.Refund(ctx)
	waitBillingRefundRun(t, session)

	operation := billingRefundOperationForRequest(t, requestID)
	assert.Equal(t, model.BillingRefundStatusApplied, operation.Status)
	assert.Empty(t, operation.PendingAction)
	assert.Equal(t, 100, operation.FundingRefundedQuota)
	assert.Equal(t, 100, operation.TokenRefundedQuota)
	assertBillingRefundWalletQuota(t, userID, 500, 500)
	assert.True(t, session.refunded)
	assert.False(t, session.requiresReconciliation)

	session.Refund(ctx)
	assertBillingRefundWalletQuota(t, userID, 500, 500)
}
