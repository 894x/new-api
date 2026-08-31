package model

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBillingAdmissionReserveCrossDatabaseContract(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		open    func(string) gorm.Dialector
	}{
		{
			name: "sqlite",
			open: func(dsn string) gorm.Dialector {
				return sqlite.Open(dsn)
			},
		},
		{
			name:    "mysql",
			envName: "TEST_MYSQL_DSN",
			open: func(dsn string) gorm.Dialector {
				return mysql.Open(dsn)
			},
		},
		{
			name:    "postgres",
			envName: "TEST_POSTGRES_DSN",
			open: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := ""
			if test.envName == "" {
				databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "billing-admission-reserve-cross-db.db"))
				dsn = "file:" + databasePath + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
			} else {
				dsn = strings.TrimSpace(os.Getenv(test.envName))
				if dsn == "" {
					t.Skip(test.envName + " is not configured")
				}
			}

			db, err := gorm.Open(test.open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(16)
			require.NoError(t, sqlDB.Ping())
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

			require.NoError(t, db.AutoMigrate(&BillingAdmissionReserveOperation{}))
			require.True(t, db.Migrator().HasIndex(&BillingAdmissionReserveOperation{}, "idx_billing_admission_reserve_operation_id"))
			require.True(t, db.Migrator().HasIndex(&BillingAdmissionReserveOperation{}, "idx_billing_admission_reserve_session_attempt"))
			cleanup := func() {
				require.NoError(t, db.Where("operation_id LIKE ?", "xdb-admission-contract-%").Delete(&BillingAdmissionReserveOperation{}).Error)
			}
			cleanup()
			t.Cleanup(cleanup)

			runBillingAdmissionReserveCrossDatabaseContract(t, db)
		})
	}
}

func runBillingAdmissionReserveCrossDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	input := BillingAdmissionReserveInput{
		OperationID:        "xdb-admission-contract-forward",
		SessionID:          "xdb-admission-contract-session-forward",
		RequestID:          "xdb-admission-contract-request-forward",
		Attempt:            0,
		UserID:             2_000_000_041,
		TokenID:            2_000_000_042,
		FundingSource:      billingAdmissionReserveFundingWallet,
		FundingReferenceID: 2_000_000_041,
		FromQuota:          300,
		TargetQuota:        500,
		TokenQuota:         200,
		Mode:               BillingAdmissionReserveModeStrictWallet,
	}

	start := make(chan struct{})
	results := make(chan BillingAdmissionReserveOperation, 2)
	errs := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			operation, err := beginBillingAdmissionReserveOperation(db, input)
			results <- operation
			errs <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var operationRowID int64
	for operation := range results {
		require.NotZero(t, operation.Id)
		if operationRowID == 0 {
			operationRowID = operation.Id
		}
		assert.Equal(t, operationRowID, operation.Id)
	}
	var count int64
	require.NoError(t, db.Model(&BillingAdmissionReserveOperation{}).
		Where("operation_id = ?", input.OperationID).
		Count(&count).Error)
	assert.Equal(t, int64(1), count)

	claimBillingAdmissionActionAcrossTwoWorkers(t, db, input.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, confirmBillingAdmissionReserveFunding(db, input.OperationID))
	require.NoError(t, confirmBillingAdmissionReserveFunding(db, input.OperationID), "confirmed funding evidence is idempotent")
	claimBillingAdmissionActionAcrossTwoWorkers(t, db, input.OperationID, BillingAdmissionReservePendingActionTokenReady)
	require.NoError(t, confirmBillingAdmissionReserveToken(db, input.OperationID))
	require.NoError(t, confirmBillingAdmissionReserveToken(db, input.OperationID), "confirmed token evidence is idempotent")
	require.NoError(t, commitBillingAdmissionReserveOperation(db, input.OperationID))
	require.NoError(t, commitBillingAdmissionReserveOperation(db, input.OperationID), "terminal replay is idempotent")

	operation, err := getBillingAdmissionReserveOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingAdmissionReserveStatusApplied, operation.Status)
	assert.Empty(t, operation.PendingAction)
	assert.Equal(t, input.TargetQuota-input.FromQuota, operation.FundingReservedQuota)
	assert.Equal(t, input.TokenQuota, operation.TokenReservedQuota)
	assert.Equal(t, int64(5), operation.Revision)

	reverseInput := input
	reverseInput.OperationID = "xdb-admission-contract-reverse"
	reverseInput.SessionID = "xdb-admission-contract-session-reverse"
	reverseInput.RequestID = "xdb-admission-contract-request-reverse"
	_, err = beginBillingAdmissionReserveOperation(db, reverseInput)
	require.NoError(t, err)
	claimBillingAdmissionActionAcrossTwoWorkers(t, db, reverseInput.OperationID, BillingAdmissionReservePendingActionFundingReady)
	require.NoError(t, confirmBillingAdmissionReserveFunding(db, reverseInput.OperationID))
	claimBillingAdmissionActionAcrossTwoWorkers(t, db, reverseInput.OperationID, BillingAdmissionReservePendingActionTokenReady)
	require.NoError(t, confirmBillingAdmissionReserveToken(db, reverseInput.OperationID))
	require.NoError(t, prepareBillingAdmissionReserveCompensation(db, reverseInput.OperationID))
	claimBillingAdmissionActionAcrossTwoWorkers(t, db, reverseInput.OperationID, BillingAdmissionReservePendingActionTokenRefundReady)
	require.NoError(t, confirmBillingAdmissionReserveTokenRefund(db, reverseInput.OperationID))
	claimBillingAdmissionActionAcrossTwoWorkers(t, db, reverseInput.OperationID, BillingAdmissionReservePendingActionFundingRefundReady)
	require.NoError(t, confirmBillingAdmissionReserveFundingRefund(db, reverseInput.OperationID))
	require.NoError(t, cancelBillingAdmissionReserveOperation(db, reverseInput.OperationID))

	reversed, err := getBillingAdmissionReserveOperation(db, reverseInput.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingAdmissionReserveStatusCanceled, reversed.Status)
	assert.Empty(t, reversed.PendingAction)
	assert.Equal(t, reversed.FundingReservedQuota, reversed.FundingRefundedQuota)
	assert.Equal(t, reversed.TokenReservedQuota, reversed.TokenRefundedQuota)
}

func claimBillingAdmissionActionAcrossTwoWorkers(
	t *testing.T,
	db *gorm.DB,
	operationID string,
	expectedReadyAction string,
) {
	t.Helper()
	wantAction := billingAdmissionReserveUnknownAction(expectedReadyAction)
	require.NotEmpty(t, wantAction)
	start := make(chan struct{})
	claims := make(chan BillingAdmissionReserveActionClaim, 2)
	errs := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			claim, err := claimNextBillingAdmissionReserveAction(db, operationID, expectedReadyAction)
			claims <- claim
			errs <- err
		}()
	}
	close(start)
	waitGroup.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	claimed := 0
	for claim := range claims {
		if claim.Claimed {
			claimed++
			assert.Equal(t, wantAction, claim.Operation.PendingAction)
		}
	}
	assert.Equal(t, 1, claimed, "only one worker may claim a ready external action")
}
