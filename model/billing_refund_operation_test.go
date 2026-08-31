package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newBillingRefundOperationFileDB(t *testing.T) *gorm.DB {
	t.Helper()
	databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "billing-refund-operation.db"))
	dsn := "file:" + databasePath + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&BillingRefundOperation{}))
	return db
}

func billingRefundOperationInput(operationID, sessionID, requestID string) BillingRefundOperationInput {
	return BillingRefundOperationInput{
		OperationID:            operationID,
		SessionID:              sessionID,
		RequestID:              requestID,
		UserID:                 101,
		TokenID:                202,
		FundingSource:          "subscription",
		FundingReferenceID:     303,
		FundingQuota:           300,
		SubscriptionExtraQuota: 200,
		TokenQuota:             500,
	}
}

func TestBeginBillingRefundOperationPersistsRecoverableFundingIntentBeforeExternalAction(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-persist", "refund-session-persist", "refund-request-persist")

	operation, err := beginBillingRefundOperation(db, input)

	require.NoError(t, err)
	assert.NotZero(t, operation.Id)
	assert.Equal(t, input.OperationID, operation.OperationID)
	assert.Equal(t, input.SessionID, operation.SessionID)
	assert.Equal(t, input.RequestID, operation.RequestID)
	assert.Equal(t, input.UserID, operation.UserID)
	assert.Equal(t, input.TokenID, operation.TokenID)
	assert.Equal(t, input.FundingSource, operation.FundingSource)
	assert.Equal(t, input.FundingReferenceID, operation.FundingReferenceID)
	assert.Equal(t, input.FundingQuota, operation.FundingQuota)
	assert.Equal(t, input.SubscriptionExtraQuota, operation.SubscriptionExtraQuota)
	assert.Equal(t, input.TokenQuota, operation.TokenQuota)
	assert.Zero(t, operation.FundingRefundedQuota)
	assert.Zero(t, operation.SubscriptionExtraRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assert.Equal(t, BillingRefundStatusPendingReconcile, operation.Status)
	assert.Equal(t, BillingRefundPendingActionFundingReady, operation.PendingAction)
	assert.Zero(t, operation.Revision)
	assert.False(t, operation.CreatedAt.IsZero())
	assert.False(t, operation.UpdatedAt.IsZero())

	stored, err := getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, operation.Id, stored.Id)
	assert.Equal(t, BillingRefundPendingActionFundingReady, stored.PendingAction)
}

func TestBillingRefundOperationPersistsExactEvidenceBeforeEveryNextExternalAction(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-stages", "refund-session-stages", "refund-request-stages")
	_, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)

	claim, err := claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionFundingReady)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	assert.Equal(t, BillingRefundPendingActionFundingUnknown, claim.Operation.PendingAction)
	require.NoError(t, confirmBillingRefundFunding(db, input.OperationID))
	operation, err := getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, input.FundingQuota, operation.FundingRefundedQuota)
	assert.Zero(t, operation.SubscriptionExtraRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assert.Equal(t, BillingRefundPendingActionSubscriptionExtraReady, operation.PendingAction)
	assert.Equal(t, int64(2), operation.Revision)

	claim, err = claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionSubscriptionExtraReady)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	assert.Equal(t, BillingRefundPendingActionSubscriptionExtraUnknown, claim.Operation.PendingAction)
	require.NoError(t, confirmBillingRefundSubscriptionExtra(db, input.OperationID))
	operation, err = getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, input.SubscriptionExtraQuota, operation.SubscriptionExtraRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assert.Equal(t, BillingRefundPendingActionTokenReady, operation.PendingAction)
	assert.Equal(t, int64(4), operation.Revision)

	claim, err = claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionTokenReady)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	assert.Equal(t, BillingRefundPendingActionTokenUnknown, claim.Operation.PendingAction)
	require.NoError(t, confirmBillingRefundToken(db, input.OperationID))
	operation, err = getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, input.TokenQuota, operation.TokenRefundedQuota)
	assert.Equal(t, BillingRefundPendingActionCommitAfterRefund, operation.PendingAction)
	assert.Equal(t, int64(6), operation.Revision)

	require.NoError(t, commitBillingRefundOperation(db, input.OperationID))
	require.NoError(t, commitBillingRefundOperation(db, input.OperationID), "terminal persistence is idempotent")
	operation, err = getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingRefundStatusApplied, operation.Status)
	assert.Empty(t, operation.PendingAction)
	assert.Equal(t, int64(7), operation.Revision)
}

func TestBillingRefundOperationSkipsAbsentStepsWithoutInventingEvidence(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-wallet", "refund-session-wallet", "refund-request-wallet")
	input.FundingSource = "wallet"
	input.FundingReferenceID = int64(input.UserID)
	input.SubscriptionExtraQuota = 0
	input.TokenQuota = 0
	_, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)

	claim, err := claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionFundingReady)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	require.NoError(t, confirmBillingRefundFunding(db, input.OperationID))
	operation, err := getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, input.FundingQuota, operation.FundingRefundedQuota)
	assert.Zero(t, operation.SubscriptionExtraRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assert.Equal(t, BillingRefundPendingActionCommitAfterRefund, operation.PendingAction)
	require.NoError(t, commitBillingRefundOperation(db, input.OperationID))
}

func TestBillingRefundClaimDoesNotClaimAReadyActionOtherThanTheExpectedStage(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-stale-claim", "refund-session-stale-claim", "refund-request-stale-claim")
	input.FundingSource = "wallet"
	input.FundingReferenceID = int64(input.UserID)
	input.SubscriptionExtraQuota = 0
	_, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)

	fundingClaim, err := claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, fundingClaim.Claimed)
	require.NoError(t, confirmBillingRefundFunding(db, input.OperationID))

	staleFundingClaim, err := claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionFundingReady)
	require.NoError(t, err)
	assert.False(t, staleFundingClaim.Claimed)
	assert.Equal(t, BillingRefundPendingActionTokenReady, staleFundingClaim.Operation.PendingAction)

	operation, err := getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingRefundPendingActionTokenReady, operation.PendingAction)
}

func TestBillingRefundOperationRejectsStaleOrOutOfOrderProgress(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-order", "refund-session-order", "refund-request-order")
	_, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)

	assert.ErrorIs(t, confirmBillingRefundSubscriptionExtra(db, input.OperationID), ErrBillingRefundInvalidTransition)
	assert.ErrorIs(t, confirmBillingRefundToken(db, input.OperationID), ErrBillingRefundInvalidTransition)
	assert.ErrorIs(t, commitBillingRefundOperation(db, input.OperationID), ErrBillingRefundInvalidTransition)
	claim, err := claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionFundingReady)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	require.NoError(t, confirmBillingRefundFunding(db, input.OperationID))
	require.NoError(t, confirmBillingRefundFunding(db, input.OperationID), "persisted progress may be acknowledged idempotently")
	assert.ErrorIs(t, commitBillingRefundOperation(db, input.OperationID), ErrBillingRefundInvalidTransition)
}

func TestBeginBillingRefundOperationScopesIdempotencyToConcreteSession(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-idempotent", "refund-session-idempotent", "shared-refund-request")

	first, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)
	second, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)
	assert.Equal(t, first.Id, second.Id)

	conflict := input
	conflict.TokenQuota++
	_, err = beginBillingRefundOperation(db, conflict)
	assert.ErrorIs(t, err, ErrBillingRefundConflict)

	otherSession := input
	otherSession.OperationID = "refund-op-other-session"
	otherSession.SessionID = "refund-session-other"
	third, err := beginBillingRefundOperation(db, otherSession)
	require.NoError(t, err, "request_id is only an audit index")
	assert.NotEqual(t, first.Id, third.Id)

	sameSessionDifferentOperation := input
	sameSessionDifferentOperation.OperationID = "refund-op-same-session-conflict"
	_, err = beginBillingRefundOperation(db, sameSessionDifferentOperation)
	assert.ErrorIs(t, err, ErrBillingRefundConflict)
}

func TestBeginBillingRefundOperationValidatesIdentityAndAmounts(t *testing.T) {
	valid := billingRefundOperationInput("refund-op-valid", "refund-session-valid", "refund-request-valid")
	tests := []struct {
		name   string
		mutate func(*BillingRefundOperationInput)
	}{
		{name: "operation id", mutate: func(input *BillingRefundOperationInput) { input.OperationID = "" }},
		{name: "session id", mutate: func(input *BillingRefundOperationInput) { input.SessionID = "" }},
		{name: "request id", mutate: func(input *BillingRefundOperationInput) { input.RequestID = "" }},
		{name: "user id", mutate: func(input *BillingRefundOperationInput) { input.UserID = 0 }},
		{name: "token id", mutate: func(input *BillingRefundOperationInput) { input.TokenID = -1 }},
		{name: "funding source", mutate: func(input *BillingRefundOperationInput) { input.FundingSource = "other" }},
		{name: "funding reference", mutate: func(input *BillingRefundOperationInput) { input.FundingReferenceID = 0 }},
		{name: "negative funding quota", mutate: func(input *BillingRefundOperationInput) { input.FundingQuota = -1 }},
		{name: "negative extra quota", mutate: func(input *BillingRefundOperationInput) { input.SubscriptionExtraQuota = -1 }},
		{name: "negative token quota", mutate: func(input *BillingRefundOperationInput) { input.TokenQuota = -1 }},
		{name: "nothing to refund", mutate: func(input *BillingRefundOperationInput) {
			input.FundingQuota = 0
			input.SubscriptionExtraQuota = 0
			input.TokenQuota = 0
		}},
		{name: "wallet cannot have subscription extra", mutate: func(input *BillingRefundOperationInput) {
			input.FundingSource = "wallet"
			input.FundingReferenceID = int64(input.UserID)
			input.SubscriptionExtraQuota = 1
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newBillingRefundOperationFileDB(t)
			input := valid
			test.mutate(&input)

			_, err := beginBillingRefundOperation(db, input)

			assert.ErrorIs(t, err, ErrBillingRefundInvalidInput)
			assert.NotErrorIs(t, err, ErrBillingRefundIntentNotPersisted)
			var count int64
			require.NoError(t, db.Model(&BillingRefundOperation{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestBeginBillingRefundOperationConcurrentReplayCreatesOneRow(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-concurrent", "refund-session-concurrent", "refund-request-concurrent")
	const goroutines = 12

	start := make(chan struct{})
	results := make(chan BillingRefundOperation, goroutines)
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			operation, err := beginBillingRefundOperation(db, input)
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
	}
	var count int64
	require.NoError(t, db.Model(&BillingRefundOperation{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestBillingRefundOperationConcurrentClaimHasOneCASWinner(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-progress", "refund-session-progress", "refund-request-progress")
	_, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)

	start := make(chan struct{})
	claims := make(chan BillingRefundActionClaim, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claim, claimErr := claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionFundingReady)
			claims <- claim
			errs <- claimErr
		}()
	}
	close(start)
	wg.Wait()
	close(claims)
	close(errs)
	for progressErr := range errs {
		require.NoError(t, progressErr)
	}
	claimed := 0
	for claim := range claims {
		if claim.Claimed {
			claimed++
		}
	}
	assert.Equal(t, 1, claimed)
	operation, err := getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), operation.Revision)
	assert.Equal(t, BillingRefundPendingActionFundingUnknown, operation.PendingAction)
}

func TestBillingRefundOperationRejectsCorruptEvidence(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	input := billingRefundOperationInput("refund-op-corrupt", "refund-session-corrupt", "refund-request-corrupt")
	operation, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)
	require.NoError(t, db.Model(&BillingRefundOperation{}).Where("id = ?", operation.Id).Update("funding_refunded_quota", 1).Error)

	_, err = beginBillingRefundOperation(db, input)
	assert.ErrorIs(t, err, ErrBillingRefundCorrupt)
	assert.ErrorIs(t, confirmBillingRefundFunding(db, input.OperationID), ErrBillingRefundCorrupt)
}

func TestBillingRefundOperationBeginReturnsUnderlyingNonRetryableError(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	forcedErr := errors.New("forced create failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:refund_begin_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_refund_operations" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:refund_begin_failure") })

	_, err := beginBillingRefundOperation(db, billingRefundOperationInput("refund-op-fail", "refund-session-fail", "refund-request-fail"))

	assert.ErrorIs(t, err, forcedErr)
	assert.ErrorIs(t, err, ErrBillingRefundIntentNotPersisted)
	assert.NotErrorIs(t, err, ErrBillingRefundIntentCommitUnknown)
	var count int64
	require.NoError(t, db.Model(&BillingRefundOperation{}).Count(&count).Error)
	assert.Zero(t, count, fmt.Sprintf("no durable intent should exist after a definite create failure: %v", err))
}

func TestBeginBillingRefundOperationRetriesCommitUnknownWithSameIdentity(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	forcedErr := errors.New("injected commit outcome unknown")
	commits := 0
	commitTransaction := func(tx *gorm.DB) error {
		commits++
		if commits == 1 {
			require.NoError(t, tx.Rollback().Error)
			return forcedErr
		}
		return tx.Commit().Error
	}
	input := billingRefundOperationInput("refund-op-commit-retry", "refund-session-commit-retry", "refund-request-commit-retry")

	operation, err := beginBillingRefundOperationWithCommit(db, input, commitTransaction)

	require.NoError(t, err)
	assert.Equal(t, 2, commits)
	assert.Equal(t, BillingRefundPendingActionFundingReady, operation.PendingAction)
	var count int64
	require.NoError(t, db.Model(&BillingRefundOperation{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestBeginBillingRefundOperationResolvesCommittedButUnacknowledgedIntent(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	forcedErr := errors.New("injected lost commit acknowledgement")
	commitTransaction := func(tx *gorm.DB) error {
		require.NoError(t, tx.Commit().Error)
		return forcedErr
	}
	input := billingRefundOperationInput("refund-op-commit-readback", "refund-session-commit-readback", "refund-request-commit-readback")

	operation, err := beginBillingRefundOperationWithCommit(db, input, commitTransaction)

	require.NoError(t, err)
	assert.NotZero(t, operation.Id)
	assert.Equal(t, BillingRefundPendingActionFundingReady, operation.PendingAction)
	var count int64
	require.NoError(t, db.Model(&BillingRefundOperation{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestBeginBillingRefundOperationReportsPersistentCommitUnknownWithoutClaimingNoWrite(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	forcedErr := errors.New("injected persistent commit outcome unknown")
	commitTransaction := func(tx *gorm.DB) error {
		require.NoError(t, tx.Rollback().Error)
		return forcedErr
	}

	_, err := beginBillingRefundOperationWithCommit(
		db,
		billingRefundOperationInput("refund-op-commit-unknown", "refund-session-commit-unknown", "refund-request-commit-unknown"),
		commitTransaction,
	)

	assert.ErrorIs(t, err, forcedErr)
	assert.ErrorIs(t, err, ErrBillingRefundIntentCommitUnknown)
	assert.NotErrorIs(t, err, ErrBillingRefundIntentNotPersisted)
	var count int64
	require.NoError(t, db.Model(&BillingRefundOperation{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestBillingRefundOperationRecoveryScanIncludesOnlyStaleSafeActionsAndListsUnknownAsManual(t *testing.T) {
	db := newBillingRefundOperationFileDB(t)
	old := time.Now().Add(-time.Hour)

	readyInput := billingRefundOperationInput("refund-op-scan-ready", "refund-session-scan-ready", "refund-request-scan-ready")
	readyInput.SubscriptionExtraQuota = 0
	ready, err := beginBillingRefundOperation(db, readyInput)
	require.NoError(t, err)
	require.NoError(t, db.Model(&BillingRefundOperation{}).Where("id = ?", ready.Id).Update("updated_at", old).Error)

	unknownInput := billingRefundOperationInput("refund-op-scan-unknown", "refund-session-scan-unknown", "refund-request-scan-unknown")
	unknownInput.SubscriptionExtraQuota = 0
	unknown, err := beginBillingRefundOperation(db, unknownInput)
	require.NoError(t, err)
	claim, err := claimNextBillingRefundAction(db, unknown.OperationID, BillingRefundPendingActionFundingReady)
	require.NoError(t, err)
	require.True(t, claim.Claimed)
	require.NoError(t, db.Model(&BillingRefundOperation{}).Where("id = ?", unknown.Id).Update("updated_at", old).Error)

	freshInput := billingRefundOperationInput("refund-op-scan-fresh", "refund-session-scan-fresh", "refund-request-scan-fresh")
	freshInput.SubscriptionExtraQuota = 0
	_, err = beginBillingRefundOperation(db, freshInput)
	require.NoError(t, err)

	recoverable, err := listRecoverableBillingRefundOperations(db, time.Now().Add(-time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, recoverable, 1)
	assert.Equal(t, ready.OperationID, recoverable[0].OperationID)

	manual, err := listManualBillingRefundOperations(db, 10)
	require.NoError(t, err)
	require.Len(t, manual, 1)
	assert.Equal(t, unknown.OperationID, manual[0].OperationID)
}

func TestBillingRefundOperationRetriesCrossDatabaseContention(t *testing.T) {
	tests := []error{
		errors.New("Error 1205: Lock wait timeout exceeded; try restarting transaction"),
		errors.New("Error 1213: Deadlock found when trying to get lock"),
		errors.New("ERROR: could not serialize access due to concurrent update (SQLSTATE 40001)"),
		errors.New("ERROR: deadlock detected (SQLSTATE 40P01)"),
	}
	for _, err := range tests {
		assert.True(t, isRetryableBillingRefundError(err), err.Error())
	}
}
