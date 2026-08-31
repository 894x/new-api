package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newBillingAdmissionReserveFileDB(t *testing.T) *gorm.DB {
	t.Helper()
	databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "billing-admission-reserve.db"))
	dsn := "file:" + databasePath + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&BillingAdmissionReserveOperation{}))
	return db
}

func billingAdmissionReserveInput(operationID, sessionID, requestID string, attempt int64) BillingAdmissionReserveInput {
	return BillingAdmissionReserveInput{
		OperationID:        operationID,
		SessionID:          sessionID,
		RequestID:          requestID,
		Attempt:            attempt,
		UserID:             101,
		TokenID:            202,
		FundingSource:      "wallet",
		FundingReferenceID: 101,
		FromQuota:          300,
		TargetQuota:        500,
		Mode:               BillingAdmissionReserveModeStrictWallet,
	}
}

func TestBeginBillingAdmissionReserveOperationPersistsPendingBeforeExternalAction(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-persist", "session-persist", "request-persist", 0)

	operation, err := beginBillingAdmissionReserveOperation(db, input)

	require.NoError(t, err)
	assert.NotZero(t, operation.Id)
	assert.Equal(t, input.OperationID, operation.OperationID)
	assert.Equal(t, input.SessionID, operation.SessionID)
	assert.Equal(t, input.RequestID, operation.RequestID)
	assert.Equal(t, input.Attempt, operation.Attempt)
	assert.Equal(t, input.UserID, operation.UserID)
	assert.Equal(t, input.TokenID, operation.TokenID)
	assert.Equal(t, input.FundingSource, operation.FundingSource)
	assert.Equal(t, input.FundingReferenceID, operation.FundingReferenceID)
	assert.Equal(t, input.FromQuota, operation.FromQuota)
	assert.Equal(t, input.TargetQuota, operation.TargetQuota)
	assert.Equal(t, 200, operation.DeltaQuota)
	assert.Equal(t, input.Mode, operation.Mode)
	assert.Equal(t, BillingAdmissionReserveStatusPendingReconcile, operation.Status)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingReady, operation.PendingAction)
	assert.Zero(t, operation.Revision)
	assert.False(t, operation.CreatedAt.IsZero())
	assert.False(t, operation.UpdatedAt.IsZero())

	stored, err := getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, operation.Id, stored.Id)
	assert.Equal(t, BillingAdmissionReserveStatusPendingReconcile, stored.Status)
	var durableRows int64
	require.NoError(t, db.Table("billing_admission_reserve_operations").Count(&durableRows).Error)
	assert.Equal(t, int64(1), durableRows)
}

func TestBeginBillingAdmissionReserveOperationIsIdempotentAndScopesAttemptsBySession(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-idempotent", "session-idempotent", "shared-request", 1)

	first, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	second, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	assert.Equal(t, first.Id, second.Id)

	operationConflict := input
	operationConflict.TargetQuota = 501
	_, err = beginBillingAdmissionReserveOperation(db, operationConflict)
	assert.ErrorIs(t, err, ErrBillingAdmissionReserveConflict)

	attemptConflict := input
	attemptConflict.OperationID = "op-same-attempt-conflict"
	_, err = beginBillingAdmissionReserveOperation(db, attemptConflict)
	assert.ErrorIs(t, err, ErrBillingAdmissionReserveConflict)

	otherSession := input
	otherSession.OperationID = "op-other-session"
	otherSession.SessionID = "session-other"
	otherSession.Attempt = 0
	third, err := beginBillingAdmissionReserveOperation(db, otherSession)
	require.NoError(t, err, "request_id is an audit index, not an idempotency key")
	assert.NotEqual(t, first.Id, third.Id)

	var count int64
	require.NoError(t, db.Model(&BillingAdmissionReserveOperation{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestBeginBillingAdmissionReserveOperationValidatesQuotaAndIdentity(t *testing.T) {
	valid := billingAdmissionReserveInput("op-valid", "session-valid", "request-valid", 0)
	tests := []struct {
		name   string
		mutate func(*BillingAdmissionReserveInput)
	}{
		{name: "operation id", mutate: func(input *BillingAdmissionReserveInput) { input.OperationID = "" }},
		{name: "session id", mutate: func(input *BillingAdmissionReserveInput) { input.SessionID = "" }},
		{name: "request id", mutate: func(input *BillingAdmissionReserveInput) { input.RequestID = "" }},
		{name: "negative attempt", mutate: func(input *BillingAdmissionReserveInput) { input.Attempt = -1 }},
		{name: "invalid user", mutate: func(input *BillingAdmissionReserveInput) { input.UserID = 0 }},
		{name: "negative token", mutate: func(input *BillingAdmissionReserveInput) { input.TokenID = -1 }},
		{name: "funding source", mutate: func(input *BillingAdmissionReserveInput) { input.FundingSource = "" }},
		{name: "unsupported funding source", mutate: func(input *BillingAdmissionReserveInput) { input.FundingSource = "credits" }},
		{name: "wallet funding reference", mutate: func(input *BillingAdmissionReserveInput) { input.FundingReferenceID++ }},
		{name: "negative funding reference", mutate: func(input *BillingAdmissionReserveInput) { input.FundingReferenceID = -1 }},
		{name: "negative from quota", mutate: func(input *BillingAdmissionReserveInput) { input.FromQuota = -1 }},
		{name: "target does not increase", mutate: func(input *BillingAdmissionReserveInput) { input.TargetQuota = input.FromQuota }},
		{name: "target exceeds safe quota", mutate: func(input *BillingAdmissionReserveInput) { input.TargetQuota = common.MaxQuota + 1 }},
		{name: "negative token quota", mutate: func(input *BillingAdmissionReserveInput) { input.TokenQuota = -1 }},
		{name: "partial token quota", mutate: func(input *BillingAdmissionReserveInput) { input.TokenQuota = 100 }},
		{name: "excess token quota", mutate: func(input *BillingAdmissionReserveInput) { input.TokenQuota = 201 }},
		{name: "unsupported mode", mutate: func(input *BillingAdmissionReserveInput) { input.Mode = "best_effort" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newBillingAdmissionReserveFileDB(t)
			input := valid
			test.mutate(&input)

			_, err := beginBillingAdmissionReserveOperation(db, input)

			assert.ErrorIs(t, err, ErrBillingAdmissionReserveInvalidInput)
			var count int64
			require.NoError(t, db.Model(&BillingAdmissionReserveOperation{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestBillingAdmissionReserveOperationStateMachineIsCASAndIdempotent(t *testing.T) {
	t.Run("applied", func(t *testing.T) {
		db := newBillingAdmissionReserveFileDB(t)
		input := billingAdmissionReserveInput("op-applied", "session-applied", "request-applied", 0)
		_, err := beginBillingAdmissionReserveOperation(db, input)
		require.NoError(t, err)
		claim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
		require.NoError(t, err)
		require.True(t, claim.Claimed)
		require.NoError(t, confirmBillingAdmissionReserveFunding(db, input.OperationID))

		require.NoError(t, commitBillingAdmissionReserveOperation(db, input.OperationID))
		require.NoError(t, commitBillingAdmissionReserveOperation(db, input.OperationID), "applied is idempotent")
		operation, err := getBillingAdmissionReserveOperation(db, input.OperationID)
		require.NoError(t, err)
		assert.Equal(t, BillingAdmissionReserveStatusApplied, operation.Status)
		assert.Equal(t, int64(3), operation.Revision)
		assert.ErrorIs(t, cancelBillingAdmissionReserveOperation(db, input.OperationID), ErrBillingAdmissionReserveInvalidTransition)
	})

	t.Run("strict wallet not applied", func(t *testing.T) {
		db := newBillingAdmissionReserveFileDB(t)
		input := billingAdmissionReserveInput("op-canceled", "session-canceled", "request-canceled", 0)
		_, err := beginBillingAdmissionReserveOperation(db, input)
		require.NoError(t, err)
		claim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
		require.NoError(t, err)
		require.True(t, claim.Claimed)

		require.NoError(t, cancelBillingAdmissionReserveOperation(db, input.OperationID))
		require.NoError(t, cancelBillingAdmissionReserveOperation(db, input.OperationID), "canceled is idempotent")
		operation, err := getBillingAdmissionReserveOperation(db, input.OperationID)
		require.NoError(t, err)
		assert.Equal(t, BillingAdmissionReserveStatusCanceled, operation.Status)
		assert.Equal(t, int64(2), operation.Revision)
		assert.ErrorIs(t, commitBillingAdmissionReserveOperation(db, input.OperationID), ErrBillingAdmissionReserveInvalidTransition)
	})

	t.Run("standard funding cannot prove not applied", func(t *testing.T) {
		db := newBillingAdmissionReserveFileDB(t)
		input := billingAdmissionReserveInput("op-standard", "session-standard", "request-standard", 0)
		input.Mode = BillingAdmissionReserveModeStandard
		_, err := beginBillingAdmissionReserveOperation(db, input)
		require.NoError(t, err)
		claim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
		require.NoError(t, err)
		require.True(t, claim.Claimed)

		err = cancelBillingAdmissionReserveOperation(db, input.OperationID)

		assert.ErrorIs(t, err, ErrBillingAdmissionReserveInvalidTransition)
		operation, getErr := getBillingAdmissionReserveOperation(db, input.OperationID)
		require.NoError(t, getErr)
		assert.Equal(t, BillingAdmissionReserveStatusPendingReconcile, operation.Status)
		assert.Equal(t, int64(1), operation.Revision)
	})

	t.Run("strict mode cannot cancel a subscription operation", func(t *testing.T) {
		db := newBillingAdmissionReserveFileDB(t)
		input := billingAdmissionReserveInput("op-strict-subscription", "session-strict-subscription", "request-strict-subscription", 0)
		input.FundingSource = "subscription"
		input.FundingReferenceID = 303
		_, err := beginBillingAdmissionReserveOperation(db, input)
		require.NoError(t, err)
		claim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
		require.NoError(t, err)
		require.True(t, claim.Claimed)

		err = cancelBillingAdmissionReserveOperation(db, input.OperationID)

		assert.ErrorIs(t, err, ErrBillingAdmissionReserveInvalidTransition)
		operation, getErr := getBillingAdmissionReserveOperation(db, input.OperationID)
		require.NoError(t, getErr)
		assert.Equal(t, BillingAdmissionReserveStatusPendingReconcile, operation.Status)
	})
}

func TestBillingAdmissionReserveInitialPreConsumeRecordsDefiniteTokenRejection(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-initial-token-rejected", "session-initial-token-rejected", "request-initial-token-rejected", 0)
	input.FromQuota = 0
	input.TargetQuota = 200
	input.TokenQuota = 200
	input.Mode = BillingAdmissionReserveModeInitial

	_, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	fundingClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, fundingClaim.Claimed)
	require.NoError(t, confirmBillingAdmissionReserveFunding(db, input.OperationID))
	tokenClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionTokenReady)
	require.NoError(t, err)
	require.True(t, tokenClaim.Claimed)

	require.NoError(t, rejectBillingAdmissionReserveToken(db, input.OperationID))
	operation, err := getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingRefundReady, operation.PendingAction)
	assert.Equal(t, input.TargetQuota, operation.FundingReservedQuota)
	assert.Zero(t, operation.TokenReservedQuota)
	assert.Equal(t, int64(4), operation.Revision)
}

func TestBillingAdmissionReserveClaimDoesNotClaimAReadyActionOtherThanTheExpectedStage(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-stale-token-claim", "session-stale-token-claim", "request-stale-token-claim", 0)
	input.TokenQuota = 200
	_, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)

	fundingClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, fundingClaim.Claimed)
	require.NoError(t, confirmBillingAdmissionReserveFunding(db, input.OperationID))
	require.NoError(t, prepareBillingAdmissionReserveCompensation(db, input.OperationID))

	staleTokenClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionTokenReady)
	require.NoError(t, err)
	assert.False(t, staleTokenClaim.Claimed)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingRefundReady, staleTokenClaim.Operation.PendingAction)

	operation, err := getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingRefundReady, operation.PendingAction)
}

func TestBillingAdmissionReserveInitialSubscriptionPersistsResolvedFundingReference(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-initial-subscription", "session-initial-subscription", "request-initial-subscription", 0)
	input.FromQuota = 0
	input.TargetQuota = 200
	input.TokenQuota = 0
	input.Mode = BillingAdmissionReserveModeInitial
	input.FundingSource = billingAdmissionReserveFundingSubscription
	input.FundingReferenceID = 0

	_, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	claim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	require.NoError(t, confirmBillingAdmissionReserveFundingReference(db, input.OperationID, 303))

	operation, err := getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.EqualValues(t, 303, operation.FundingReferenceID)
	assert.Equal(t, BillingAdmissionReservePendingActionCommitAfterReserve, operation.PendingAction)
	assert.Equal(t, input.TargetQuota, operation.FundingReservedQuota)
	replayed, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err, "the original begin input remains idempotent after resolving the subscription reference")
	assert.Equal(t, operation.Id, replayed.Id)
	assert.EqualValues(t, 303, replayed.FundingReferenceID)
}

func TestBillingAdmissionReserveInitialModeRejectsTopUpShape(t *testing.T) {
	valid := billingAdmissionReserveInput("op-initial-shape", "session-initial-shape", "request-initial-shape", 0)
	valid.Mode = BillingAdmissionReserveModeInitial
	valid.FromQuota = 0
	valid.TargetQuota = 200

	for _, test := range []struct {
		name   string
		mutate func(*BillingAdmissionReserveInput)
	}{
		{name: "nonzero attempt", mutate: func(input *BillingAdmissionReserveInput) { input.Attempt = 1 }},
		{name: "nonzero cursor", mutate: func(input *BillingAdmissionReserveInput) {
			input.FromQuota = 100
			input.TargetQuota = 300
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := newBillingAdmissionReserveFileDB(t)
			input := valid
			test.mutate(&input)

			_, err := beginBillingAdmissionReserveOperation(db, input)

			assert.ErrorIs(t, err, ErrBillingAdmissionReserveInvalidInput)
		})
	}
}

func TestBeginBillingAdmissionReserveOperationConcurrentReplayCreatesOneRow(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-concurrent-begin", "session-concurrent-begin", "request-concurrent-begin", 0)
	const goroutines = 12

	start := make(chan struct{})
	results := make(chan BillingAdmissionReserveOperation, goroutines)
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			operation, err := beginBillingAdmissionReserveOperation(db, input)
			results <- operation
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
	var id int64
	for operation := range results {
		if id == 0 {
			id = operation.Id
		}
		assert.Equal(t, id, operation.Id)
		assert.Equal(t, BillingAdmissionReserveStatusPendingReconcile, operation.Status)
	}
	var count int64
	require.NoError(t, db.Model(&BillingAdmissionReserveOperation{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestBillingAdmissionReserveOperationConcurrentTerminalTransitionsHaveOneWinner(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-concurrent-transition", "session-concurrent-transition", "request-concurrent-transition", 0)
	_, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	claim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, claim.Claimed)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs <- confirmBillingAdmissionReserveFunding(db, input.OperationID)
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- cancelBillingAdmissionReserveOperation(db, input.OperationID)
	}()
	close(start)
	wg.Wait()
	close(errs)

	var successCount, rejectedCount int
	for transitionErr := range errs {
		switch {
		case transitionErr == nil:
			successCount++
		case errors.Is(transitionErr, ErrBillingAdmissionReserveInvalidTransition):
			rejectedCount++
		default:
			require.NoError(t, transitionErr)
		}
	}
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, rejectedCount)
	operation, err := getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Contains(t, []string{BillingAdmissionReserveStatusPendingReconcile, BillingAdmissionReserveStatusCanceled}, operation.Status)
	assert.Equal(t, int64(2), operation.Revision)

	if operation.Status == BillingAdmissionReserveStatusPendingReconcile {
		assert.Equal(t, BillingAdmissionReservePendingActionCommitAfterReserve, operation.PendingAction)
		require.NoError(t, confirmBillingAdmissionReserveFunding(db, input.OperationID))
		assert.ErrorIs(t, cancelBillingAdmissionReserveOperation(db, input.OperationID), ErrBillingAdmissionReserveInvalidTransition)
	} else {
		require.NoError(t, cancelBillingAdmissionReserveOperation(db, input.OperationID))
		assert.ErrorIs(t, confirmBillingAdmissionReserveFunding(db, input.OperationID), ErrBillingAdmissionReserveInvalidTransition)
	}
}

func TestBillingAdmissionReserveOperationRejectsCorruptDeltaOnReplay(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-corrupt-delta", "session-corrupt-delta", "request-corrupt-delta", 0)
	operation, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	require.NoError(t, db.Model(&BillingAdmissionReserveOperation{}).
		Where("id = ?", operation.Id).
		Update("delta_quota", 199).Error)

	_, err = beginBillingAdmissionReserveOperation(db, input)

	assert.ErrorIs(t, err, ErrBillingAdmissionReserveCorrupt)
	assert.ErrorIs(t, commitBillingAdmissionReserveOperation(db, input.OperationID), ErrBillingAdmissionReserveCorrupt)
}

func TestBillingAdmissionReserveOperationConcurrentDistinctAttemptsCoexist(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	const attempts = 8

	start := make(chan struct{})
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for attempt := range attempts {
		wg.Add(1)
		go func(attempt int) {
			defer wg.Done()
			<-start
			input := billingAdmissionReserveInput(
				fmt.Sprintf("op-attempt-%d", attempt),
				"session-attempts",
				"request-attempts",
				int64(attempt),
			)
			_, err := beginBillingAdmissionReserveOperation(db, input)
			errs <- err
		}(attempt)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var count int64
	require.NoError(t, db.Model(&BillingAdmissionReserveOperation{}).Count(&count).Error)
	assert.Equal(t, int64(attempts), count)
}

func TestBillingAdmissionReserveOperationPersistsAndClaimsEveryForwardStage(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-forward-stages", "session-forward-stages", "request-forward-stages", 0)
	input.TokenQuota = 200

	operation, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingReady, operation.PendingAction)
	assert.Zero(t, operation.FundingReservedQuota)
	assert.Zero(t, operation.TokenReservedQuota)

	fundingClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	assert.True(t, fundingClaim.Claimed)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingUnknown, fundingClaim.Operation.PendingAction)
	require.NoError(t, confirmBillingAdmissionReserveFunding(db, input.OperationID))

	operation, err = getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, 200, operation.FundingReservedQuota)
	assert.Equal(t, BillingAdmissionReservePendingActionTokenReady, operation.PendingAction)

	tokenClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionTokenReady)
	require.NoError(t, err)
	assert.True(t, tokenClaim.Claimed)
	assert.Equal(t, BillingAdmissionReservePendingActionTokenUnknown, tokenClaim.Operation.PendingAction)
	require.NoError(t, confirmBillingAdmissionReserveToken(db, input.OperationID))

	operation, err = getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, 200, operation.TokenReservedQuota)
	assert.Equal(t, BillingAdmissionReservePendingActionCommitAfterReserve, operation.PendingAction)
	require.NoError(t, commitBillingAdmissionReserveOperation(db, input.OperationID))

	operation, err = getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingAdmissionReserveStatusApplied, operation.Status)
	assert.Empty(t, operation.PendingAction)
}

func TestBillingAdmissionReserveOperationPreparesDurableReverseStagesAfterConfirmedTopUp(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-reverse-stages", "session-reverse-stages", "request-reverse-stages", 0)
	input.TokenQuota = 200
	_, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	_, err = claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.NoError(t, confirmBillingAdmissionReserveFunding(db, input.OperationID))
	_, err = claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionTokenReady)
	require.NoError(t, err)
	require.NoError(t, confirmBillingAdmissionReserveToken(db, input.OperationID))

	require.NoError(t, prepareBillingAdmissionReserveCompensation(db, input.OperationID))
	operation, err := getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingAdmissionReservePendingActionTokenRefundReady, operation.PendingAction)

	tokenClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionTokenRefundReady)
	require.NoError(t, err)
	assert.True(t, tokenClaim.Claimed)
	assert.Equal(t, BillingAdmissionReservePendingActionTokenRefundUnknown, tokenClaim.Operation.PendingAction)
	require.NoError(t, confirmBillingAdmissionReserveTokenRefund(db, input.OperationID))

	operation, err = getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, 200, operation.TokenRefundedQuota)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingRefundReady, operation.PendingAction)

	fundingClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingRefundReady)
	require.NoError(t, err)
	assert.True(t, fundingClaim.Claimed)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingRefundUnknown, fundingClaim.Operation.PendingAction)
	require.NoError(t, confirmBillingAdmissionReserveFundingRefund(db, input.OperationID))

	operation, err = getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, 200, operation.FundingRefundedQuota)
	assert.Equal(t, BillingAdmissionReservePendingActionCommitAfterRefund, operation.PendingAction)
	require.NoError(t, cancelBillingAdmissionReserveOperation(db, input.OperationID))

	operation, err = getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingAdmissionReserveStatusCanceled, operation.Status)
	assert.Empty(t, operation.PendingAction)
}

func TestBillingAdmissionReserveOperationUnknownStageIsManualAndNeverPreparedOrReclaimed(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	input := billingAdmissionReserveInput("op-unknown-manual", "session-unknown-manual", "request-unknown-manual", 0)
	input.TokenQuota = 200
	_, err := beginBillingAdmissionReserveOperation(db, input)
	require.NoError(t, err)
	claim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, claim.Claimed)

	secondClaim, err := claimNextBillingAdmissionReserveAction(db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, err)
	assert.False(t, secondClaim.Claimed)
	assert.Equal(t, BillingAdmissionReservePendingActionFundingUnknown, secondClaim.Operation.PendingAction)
	assert.ErrorIs(t, prepareBillingAdmissionReserveCompensation(db, input.OperationID), ErrBillingAdmissionReserveInvalidTransition)

	recoverable, err := listRecoverableBillingAdmissionReserveOperations(db, time.Now().Add(time.Hour), 10)
	require.NoError(t, err)
	assert.Empty(t, recoverable)
	manual, err := listManualBillingAdmissionReserveOperations(db, 10)
	require.NoError(t, err)
	require.Len(t, manual, 1)
	assert.Equal(t, input.OperationID, manual[0].OperationID)
}

func TestBillingAdmissionReserveOperationRecoverableScanIsStaleAndBounded(t *testing.T) {
	db := newBillingAdmissionReserveFileDB(t)
	old := time.Now().Add(-time.Hour)
	for index := range 3 {
		input := billingAdmissionReserveInput(
			fmt.Sprintf("op-recoverable-%d", index),
			fmt.Sprintf("session-recoverable-%d", index),
			fmt.Sprintf("request-recoverable-%d", index),
			0,
		)
		input.TokenQuota = 200
		operation, err := beginBillingAdmissionReserveOperation(db, input)
		require.NoError(t, err)
		require.NoError(t, db.Model(&BillingAdmissionReserveOperation{}).Where("id = ?", operation.Id).Update("updated_at", old).Error)
	}

	recoverable, err := listRecoverableBillingAdmissionReserveOperations(db, time.Now().Add(-time.Minute), 2)
	require.NoError(t, err)
	require.Len(t, recoverable, 2)
	assert.Less(t, recoverable[0].Id, recoverable[1].Id)

	freshInput := billingAdmissionReserveInput("op-fresh", "session-fresh", "request-fresh", 0)
	freshInput.TokenQuota = 200
	_, err = beginBillingAdmissionReserveOperation(db, freshInput)
	require.NoError(t, err)
	recoverable, err = listRecoverableBillingAdmissionReserveOperations(db, time.Now().Add(-time.Minute), 10)
	require.NoError(t, err)
	assert.Len(t, recoverable, 3)
}

func TestBillingAdmissionReserveOperationRetriesCrossDatabaseContention(t *testing.T) {
	tests := []error{
		errors.New("Error 1205: Lock wait timeout exceeded; try restarting transaction"),
		errors.New("Error 1213: Deadlock found when trying to get lock"),
		errors.New("ERROR: could not serialize access due to concurrent update (SQLSTATE 40001)"),
		errors.New("ERROR: deadlock detected (SQLSTATE 40P01)"),
	}
	for _, err := range tests {
		assert.True(t, isRetryableBillingAdmissionReserveError(err), err.Error())
	}
}
