package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	groupModelDiscountCrossDBUserID    = 2_000_000_031
	groupModelDiscountCrossDBChannelID = 2_000_000_034
)

type groupModelDiscountCrossDBUserTarget struct {
	Id           int `gorm:"primaryKey"`
	UsedQuota    int
	RequestCount int
	DeletedAt    gorm.DeletedAt
}

func (groupModelDiscountCrossDBUserTarget) TableName() string { return "users" }

type groupModelDiscountCrossDBChannelTarget struct {
	Id        int `gorm:"primaryKey"`
	UsedQuota int64
}

func (groupModelDiscountCrossDBChannelTarget) TableName() string { return "channels" }

func TestGroupModelDiscountLedgerCrossDatabaseContract(t *testing.T) {
	tests := []struct {
		name         string
		databaseType common.DatabaseType
		envName      string
		open         func(string) gorm.Dialector
	}{
		{
			name:         "sqlite",
			databaseType: common.DatabaseTypeSQLite,
			open: func(dsn string) gorm.Dialector {
				return sqlite.Open(dsn)
			},
		},
		{
			name:         "mysql",
			databaseType: common.DatabaseTypeMySQL,
			envName:      "TEST_MYSQL_DSN",
			open: func(dsn string) gorm.Dialector {
				return mysql.Open(dsn)
			},
		},
		{
			name:         "postgres",
			databaseType: common.DatabaseTypePostgreSQL,
			envName:      "TEST_POSTGRES_DSN",
			open: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := ""
			if test.envName == "" {
				databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "group-discount-cross-db.db"))
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

			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			common.SetDatabaseTypes(test.databaseType, test.databaseType)
			t.Cleanup(func() {
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
			})
			initCol()

			require.NoError(t, db.AutoMigrate(
				&groupModelDiscountCrossDBUserTarget{},
				&groupModelDiscountCrossDBChannelTarget{},
				&UserGroupModelMonthlyUsage{},
				&GroupModelDiscountSettlement{},
				&GroupModelDiscountAdjustment{},
				&BillingRefundOperation{},
			))
			cleanupGroupModelDiscountCrossDBRows(t, db)
			t.Cleanup(func() { cleanupGroupModelDiscountCrossDBRows(t, db) })

			var version string
			versionQuery := "SELECT VERSION()"
			if test.databaseType == common.DatabaseTypeSQLite {
				versionQuery = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)

			runGroupModelDiscountCrossDBContract(t, db)
			runGroupModelDiscountAccountingCrossDBContract(t, db)
			runBillingRefundOperationCrossDBContract(t, db)
		})
	}
}

func cleanupGroupModelDiscountCrossDBRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Where("operation_id LIKE ?", "xdb-refund-contract-%").Delete(&BillingRefundOperation{}).Error)
	require.NoError(t, db.Where("settlement_request_id LIKE ?", "xdb-gmd-contract-%").Delete(&GroupModelDiscountAdjustment{}).Error)
	require.NoError(t, db.Where("request_id LIKE ?", "xdb-gmd-contract-%").Delete(&GroupModelDiscountSettlement{}).Error)
	require.NoError(t, db.Where("user_id = ?", groupModelDiscountCrossDBUserID).Delete(&UserGroupModelMonthlyUsage{}).Error)
	require.NoError(t, db.Where("id = ?", groupModelDiscountCrossDBChannelID).Delete(&groupModelDiscountCrossDBChannelTarget{}).Error)
	require.NoError(t, db.Unscoped().Where("id = ?", groupModelDiscountCrossDBUserID).Delete(&groupModelDiscountCrossDBUserTarget{}).Error)
}

func runGroupModelDiscountAccountingCrossDBContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	user := groupModelDiscountCrossDBUserTarget{Id: groupModelDiscountCrossDBUserID}
	channel := groupModelDiscountCrossDBChannelTarget{Id: groupModelDiscountCrossDBChannelID}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&channel).Error)
	snapshot := groupDiscountTestSnapshot("xdb-accounting-vip", "xdb-accounting-model", 1_893_456_100)
	reservation, err := reserveGroupModelDiscount(db, GroupModelDiscountReserveInput{
		RequestID:     "xdb-gmd-contract-accounting",
		UserID:        user.Id,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 100,
	})
	require.NoError(t, err)
	commitDelta := BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: reservation.Calculation.ChargedQuota, RequestCountDelta: 1,
	}
	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, commitDelta))
	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, commitDelta))

	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "xdb-gmd-contract-accounting-adjustment",
		SettlementRequestID: reservation.Settlement.RequestID,
		NewOriginalQuota:    200,
	})
	require.NoError(t, err)
	adjustmentDelta := BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: adjustment.DeltaChargedQuota,
	}
	require.NoError(t, commitGroupModelDiscountAdjustmentWithUsage(db, adjustment.Adjustment.AdjustmentID, adjustmentDelta))
	require.NoError(t, commitGroupModelDiscountAdjustmentWithUsage(db, adjustment.Adjustment.AdjustmentID, adjustmentDelta))

	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, reservation.Settlement.RequestID))
	reverseDelta := BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, QuotaDelta: -adjustment.NewChargedQuota,
	}
	require.NoError(t, reverseGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, reverseDelta))
	require.NoError(t, reverseGroupModelDiscountSettlementWithUsage(db, reservation.Settlement.RequestID, reverseDelta))

	require.NoError(t, db.First(&user, user.Id).Error)
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.Zero(t, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Zero(t, channel.UsedQuota)
	settlement, err := getGroupModelDiscountSettlement(db, reservation.Settlement.RequestID)
	require.NoError(t, err)
	assert.True(t, settlement.AccountingApplied)
	assert.True(t, settlement.ReverseAccountingApplied)

	zeroSnapshot := groupDiscountTestSnapshot("xdb-accounting-zero-vip", "xdb-accounting-zero-model", 1_893_456_200)
	zeroSnapshot.PolicyHash = "xdb-accounting-zero-policy"
	zeroSnapshot.Tiers = []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0}}
	zeroReservation, err := reserveGroupModelDiscount(db, GroupModelDiscountReserveInput{
		RequestID:     "xdb-gmd-contract-accounting-zero",
		UserID:        user.Id,
		UsingGroup:    zeroSnapshot.UsingGroup,
		OriginModel:   zeroSnapshot.OriginModel,
		Snapshot:      zeroSnapshot,
		OriginalQuota: 100,
	})
	require.NoError(t, err)
	assert.Zero(t, zeroReservation.Calculation.ChargedQuota)
	zeroCommitDelta := BillingUsageDelta{
		UserID: user.Id, ChannelID: channel.Id, RequestCountDelta: 1,
	}
	require.NoError(t, commitGroupModelDiscountSettlementWithUsage(db, zeroReservation.Settlement.RequestID, zeroCommitDelta))
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, zeroReservation.Settlement.RequestID))
	zeroReverseDelta := BillingUsageDelta{UserID: user.Id, ChannelID: channel.Id}
	require.NoError(t, reverseGroupModelDiscountSettlementWithUsage(db, zeroReservation.Settlement.RequestID, zeroReverseDelta))
	require.NoError(t, db.First(&user, user.Id).Error)
	require.NoError(t, db.First(&channel, channel.Id).Error)
	assert.Zero(t, user.UsedQuota)
	assert.Equal(t, 2, user.RequestCount)
	assert.Zero(t, channel.UsedQuota)
}

func runBillingRefundOperationCrossDBContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	input := BillingRefundOperationInput{
		OperationID:            "xdb-refund-contract-operation",
		SessionID:              "xdb-refund-contract-session",
		RequestID:              "xdb-refund-contract-request",
		UserID:                 groupModelDiscountCrossDBUserID,
		TokenID:                2_000_000_032,
		FundingSource:          billingRefundFundingSubscription,
		FundingReferenceID:     2_000_000_033,
		FundingQuota:           320,
		SubscriptionExtraQuota: 80,
		TokenQuota:             500,
	}

	start := make(chan struct{})
	results := make(chan BillingRefundOperation, 2)
	errs := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for range 2 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			operation, err := beginBillingRefundOperation(db, input)
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
	var operationID int64
	for operation := range results {
		require.NotZero(t, operation.Id)
		if operationID == 0 {
			operationID = operation.Id
		}
		assert.Equal(t, operationID, operation.Id)
	}
	var operationCount int64
	require.NoError(t, db.Model(&BillingRefundOperation{}).
		Where("operation_id = ?", input.OperationID).
		Count(&operationCount).Error)
	assert.Equal(t, int64(1), operationCount)

	replayed, err := beginBillingRefundOperation(db, input)
	require.NoError(t, err)
	assert.Equal(t, operationID, replayed.Id)
	assert.Equal(t, BillingRefundStatusPendingReconcile, replayed.Status)
	assert.Equal(t, BillingRefundPendingActionFundingReady, replayed.PendingAction)
	assert.Zero(t, replayed.Revision)

	claim, err := claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionFundingReady)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	assert.Equal(t, BillingRefundPendingActionFundingUnknown, claim.Operation.PendingAction)
	require.NoError(t, confirmBillingRefundFunding(db, input.OperationID))
	require.NoError(t, confirmBillingRefundFunding(db, input.OperationID), "funding evidence replay is idempotent")
	operation, err := getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, 320, operation.FundingRefundedQuota)
	assert.Zero(t, operation.SubscriptionExtraRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assert.Equal(t, BillingRefundPendingActionSubscriptionExtraReady, operation.PendingAction)
	assert.Equal(t, int64(2), operation.Revision)

	claim, err = claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionSubscriptionExtraReady)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	assert.Equal(t, BillingRefundPendingActionSubscriptionExtraUnknown, claim.Operation.PendingAction)
	require.NoError(t, confirmBillingRefundSubscriptionExtra(db, input.OperationID))
	require.NoError(t, confirmBillingRefundSubscriptionExtra(db, input.OperationID), "extra evidence replay is idempotent")
	operation, err = getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, 320, operation.FundingRefundedQuota)
	assert.Equal(t, 80, operation.SubscriptionExtraRefundedQuota)
	assert.Zero(t, operation.TokenRefundedQuota)
	assert.Equal(t, BillingRefundPendingActionTokenReady, operation.PendingAction)
	assert.Equal(t, int64(4), operation.Revision)

	claim, err = claimNextBillingRefundAction(db, input.OperationID, BillingRefundPendingActionTokenReady)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	assert.Equal(t, BillingRefundPendingActionTokenUnknown, claim.Operation.PendingAction)
	require.NoError(t, confirmBillingRefundToken(db, input.OperationID))
	require.NoError(t, confirmBillingRefundToken(db, input.OperationID), "token evidence replay is idempotent")
	operation, err = getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, 500, operation.TokenRefundedQuota)
	assert.Equal(t, BillingRefundPendingActionCommitAfterRefund, operation.PendingAction)
	assert.Equal(t, int64(6), operation.Revision)

	require.NoError(t, commitBillingRefundOperation(db, input.OperationID))
	require.NoError(t, commitBillingRefundOperation(db, input.OperationID), "terminal replay is idempotent")
	operation, err = getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, BillingRefundStatusApplied, operation.Status)
	assert.Empty(t, operation.PendingAction)
	assert.Equal(t, 320, operation.FundingRefundedQuota)
	assert.Equal(t, 80, operation.SubscriptionExtraRefundedQuota)
	assert.Equal(t, 500, operation.TokenRefundedQuota)
	assert.Equal(t, int64(7), operation.Revision)

	replayed, err = beginBillingRefundOperation(db, input)
	require.NoError(t, err)
	assert.Equal(t, operationID, replayed.Id)
	assert.Equal(t, BillingRefundStatusApplied, replayed.Status)
	require.NoError(t, confirmBillingRefundFunding(db, input.OperationID))
	require.NoError(t, confirmBillingRefundSubscriptionExtra(db, input.OperationID))
	require.NoError(t, confirmBillingRefundToken(db, input.OperationID))
	require.NoError(t, commitBillingRefundOperation(db, input.OperationID))
	operation, err = getBillingRefundOperation(db, input.OperationID)
	require.NoError(t, err)
	assert.Equal(t, int64(7), operation.Revision, "replays must not duplicate durable evidence")
}

func runGroupModelDiscountCrossDBContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	const (
		usingGroup  = "xdb-vip"
		originModel = "xdb-gpt-5"
		periodStart = int64(1_893_456_000)
	)
	snapshot := groupdiscount.Snapshot{
		PolicyHash:   "xdb-policy-v1",
		UsingGroup:   usingGroup,
		OriginModel:  originModel,
		MatchedModel: originModel,
		Timezone:     "UTC",
		PeriodStart:  periodStart,
		PeriodEnd:    periodStart + int64((31*24*time.Hour)/time.Second),
		Tiers: []groupdiscount.Tier{
			{MinMonthlyOriginalQuota: 0, Ratio: 0.9},
			{MinMonthlyOriginalQuota: 1000, Ratio: 0.85},
		},
	}
	reserve := func(requestID string, originalQuota int) (GroupModelDiscountReservation, error) {
		return reserveGroupModelDiscount(db, GroupModelDiscountReserveInput{
			RequestID:     requestID,
			UserID:        groupModelDiscountCrossDBUserID,
			UsingGroup:    usingGroup,
			OriginModel:   originModel,
			Snapshot:      snapshot,
			OriginalQuota: originalQuota,
		})
	}

	first, err := reserve("xdb-gmd-contract-first", 900)
	require.NoError(t, err)
	assert.Equal(t, 810, first.Calculation.ChargedQuota)
	require.NoError(t, commitGroupModelDiscountSettlement(db, first.Settlement.RequestID))

	second, err := reserve("xdb-gmd-contract-cross", 300)
	require.NoError(t, err)
	assert.Equal(t, 260, second.Calculation.ChargedQuota)
	assert.Equal(t, int64(900), second.Calculation.MonthlyOriginalBefore)
	require.Len(t, second.Calculation.Segments, 2)
	replayed, err := reserve("xdb-gmd-contract-cross", 300)
	require.NoError(t, err)
	assert.True(t, replayed.Reused)
	assert.Equal(t, second.Calculation, replayed.Calculation)
	require.NoError(t, commitGroupModelDiscountSettlement(db, second.Settlement.RequestID))

	adjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "xdb-gmd-contract-first-final",
		SettlementRequestID: first.Settlement.RequestID,
		NewOriginalQuota:    800,
	})
	require.NoError(t, err)
	assert.Equal(t, -100, adjustment.DeltaOriginalQuota)
	assert.Equal(t, -85, adjustment.DeltaChargedQuota)
	replayedAdjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "xdb-gmd-contract-first-final",
		SettlementRequestID: first.Settlement.RequestID,
		NewOriginalQuota:    800,
	})
	require.NoError(t, err)
	assert.True(t, replayedAdjustment.Reused)
	require.NoError(t, commitGroupModelDiscountAdjustment(db, adjustment.Adjustment.AdjustmentID))

	usage, err := getUserGroupModelMonthlyUsage(db, groupModelDiscountCrossDBUserID, usingGroup, originModel, periodStart)
	require.NoError(t, err)
	assert.Equal(t, int64(1100), usage.OriginalQuota)
	assert.Equal(t, int64(985), usage.ChargedQuota)

	assert.ErrorIs(t, beginGroupModelDiscountSettlementReverse(db, second.Settlement.RequestID), ErrGroupModelDiscountNonTailReverse,
		"the settlement owning the latest cursor mutation must reverse first")
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, first.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, first.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, first.Settlement.RequestID), "reverse is idempotent")
	usage, err = getUserGroupModelMonthlyUsage(db, groupModelDiscountCrossDBUserID, usingGroup, originModel, periodStart)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.OriginalQuota)
	assert.Equal(t, int64(260), usage.ChargedQuota)

	secondAdjustment, err := reserveGroupModelDiscountAdjustment(db, GroupModelDiscountAdjustmentInput{
		AdjustmentID:        "xdb-gmd-contract-second-final",
		SettlementRequestID: second.Settlement.RequestID,
		NewOriginalQuota:    400,
	})
	require.NoError(t, err)
	assert.Equal(t, 90, secondAdjustment.DeltaChargedQuota)
	require.NoError(t, commitGroupModelDiscountAdjustment(db, secondAdjustment.Adjustment.AdjustmentID))

	runGroupModelDiscountCrossDBConcurrencyContract(t, db, periodStart+1)
	runGroupModelDiscountChargedProgressCrossDBContract(t, db, periodStart+2)
}

func runGroupModelDiscountChargedProgressCrossDBContract(t *testing.T, db *gorm.DB, periodStart int64) {
	t.Helper()
	snapshot := groupdiscount.Snapshot{
		PolicyHash:    "xdb-charged-policy-v1",
		ProgressBasis: groupdiscount.ProgressBasisCharged,
		UsingGroup:    "xdb-vip",
		OriginModel:   "xdb-charged-model",
		MatchedModel:  "xdb-charged-model",
		Timezone:      "UTC",
		PeriodStart:   periodStart,
		PeriodEnd:     periodStart + int64((31*24*time.Hour)/time.Second),
		Tiers: []groupdiscount.Tier{
			{MinMonthlyOriginalQuota: 0, Ratio: 0.8},
			{MinMonthlyOriginalQuota: 401, Ratio: 0.7},
		},
	}
	reserve := func(requestID string) GroupModelDiscountReservation {
		reservation, err := reserveGroupModelDiscount(db, GroupModelDiscountReserveInput{
			RequestID:     requestID,
			UserID:        groupModelDiscountCrossDBUserID,
			UsingGroup:    snapshot.UsingGroup,
			OriginModel:   snapshot.OriginModel,
			Snapshot:      snapshot,
			OriginalQuota: 300,
		})
		require.NoError(t, err)
		return reservation
	}

	first := reserve("xdb-gmd-contract-charged-first")
	assert.Equal(t, 240, first.Calculation.ChargedQuota)
	assert.Equal(t, "240", first.Calculation.MonthlyProgressAfter)
	require.NoError(t, commitGroupModelDiscountSettlement(db, first.Settlement.RequestID))
	second := reserve("xdb-gmd-contract-charged-second")
	assert.Equal(t, 230, second.Calculation.ChargedQuota)
	assert.Equal(t, "470.125", second.Calculation.MonthlyProgressAfter)
	require.Len(t, second.Calculation.Segments, 2)
	assert.Equal(t, "201.25", second.Calculation.Segments[0].OriginalQuotaExact)
	assert.Equal(t, "98.75", second.Calculation.Segments[1].OriginalQuotaExact)
	require.NoError(t, commitGroupModelDiscountSettlement(db, second.Settlement.RequestID))

	usage, err := getUserGroupModelMonthlyUsage(
		db,
		groupModelDiscountCrossDBUserID,
		snapshot.UsingGroup,
		snapshot.OriginModel,
		periodStart,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(600), usage.OriginalQuota)
	assert.Equal(t, int64(470), usage.ChargedQuota)
	assert.Equal(t, "470.125", usage.ProgressQuota)

	assert.ErrorIs(t, beginGroupModelDiscountSettlementReverse(db, first.Settlement.RequestID), ErrGroupModelDiscountNonTailReverse)
	require.NoError(t, beginGroupModelDiscountSettlementReverse(db, second.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, second.Settlement.RequestID))
	require.NoError(t, reverseGroupModelDiscountSettlement(db, second.Settlement.RequestID), "reverse is idempotent")
	usage, err = getUserGroupModelMonthlyUsage(
		db,
		groupModelDiscountCrossDBUserID,
		snapshot.UsingGroup,
		snapshot.OriginModel,
		periodStart,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(300), usage.OriginalQuota)
	assert.Equal(t, int64(240), usage.ChargedQuota)
	assert.Equal(t, "240", usage.ProgressQuota)
}

func runGroupModelDiscountCrossDBConcurrencyContract(t *testing.T, db *gorm.DB, periodStart int64) {
	t.Helper()
	const requestCount = 12
	snapshot := groupdiscount.Snapshot{
		PolicyHash:   "xdb-concurrent-policy-v1",
		UsingGroup:   "xdb-vip",
		OriginModel:  "xdb-concurrent-model",
		MatchedModel: "xdb-concurrent-model",
		Timezone:     "UTC",
		PeriodStart:  periodStart,
		PeriodEnd:    periodStart + int64((31*24*time.Hour)/time.Second),
		Tiers:        []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.5}},
	}

	start := make(chan struct{})
	errs := make(chan error, requestCount)
	var waitGroup sync.WaitGroup
	for index := range requestCount {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			requestID := fmt.Sprintf("xdb-gmd-contract-concurrent-%d", index)
			_, err := reserveGroupModelDiscount(db, GroupModelDiscountReserveInput{
				RequestID:     requestID,
				UserID:        groupModelDiscountCrossDBUserID,
				UsingGroup:    snapshot.UsingGroup,
				OriginModel:   snapshot.OriginModel,
				Snapshot:      snapshot,
				OriginalQuota: 1,
			})
			if err == nil {
				err = commitGroupModelDiscountSettlement(db, requestID)
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

	usage, err := getUserGroupModelMonthlyUsage(
		db,
		groupModelDiscountCrossDBUserID,
		snapshot.UsingGroup,
		snapshot.OriginModel,
		periodStart,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(requestCount), usage.OriginalQuota)
	assert.Equal(t, int64(requestCount/2), usage.ChargedQuota)

	var settlementCount int64
	require.NoError(t, db.Model(&GroupModelDiscountSettlement{}).
		Where("request_id LIKE ?", "xdb-gmd-contract-concurrent-%").
		Count(&settlementCount).Error)
	assert.Equal(t, int64(requestCount), settlementCount)
}
