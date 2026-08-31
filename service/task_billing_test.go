package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	if err := db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Token{},
		&model.Log{},
		&model.Channel{},
		&model.Midjourney{},
		&model.TopUp{},
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.SubscriptionPreConsumeRecord{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Seed helpers
// ---------------------------------------------------------------------------

func truncate(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM tasks")
		model.DB.Exec("DELETE FROM users")
		model.DB.Exec("DELETE FROM tokens")
		model.DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM midjourneys")
		model.DB.Exec("DELETE FROM top_ups")
		model.DB.Exec("DELETE FROM subscription_pre_consume_records")
		model.DB.Exec("DELETE FROM user_subscriptions")
		model.DB.Exec("DELETE FROM subscription_plans")
		model.DB.Exec("DELETE FROM system_task_locks")
		model.DB.Exec("DELETE FROM system_tasks")
		model.DB.Exec("DELETE FROM group_model_discount_adjustments")
		model.DB.Exec("DELETE FROM group_model_discount_settlements")
		model.DB.Exec("DELETE FROM user_group_model_monthly_usages")
		model.DB.Exec("DELETE FROM billing_refund_operations")
		model.DB.Exec("DELETE FROM billing_admission_reserve_operations")
	})
}

func seedUser(t *testing.T, id int, quota int) {
	t.Helper()
	user := &model.User{Id: id, Username: "test_user", Quota: quota, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedToken(t *testing.T, id int, userId int, key string, remainQuota int) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "test_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
		UsedQuota:   0,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func seedSubscription(t *testing.T, id int, userId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedChannel(t *testing.T, id int) {
	t.Helper()
	ch := &model.Channel{Id: id, Name: "test_channel", Key: "sk-test", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(ch).Error)
}

func seedChargedAccounting(t *testing.T, userID, channelID, tokenID, quota, requestCount int) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
		"used_quota":    quota,
		"request_count": requestCount,
	}).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID).
		Update("used_quota", quota).Error)
	if tokenID > 0 {
		require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).
			Update("used_quota", quota).Error)
	}
}

func commitGroupModelSettlementWithAccounting(
	t *testing.T,
	settlementID string,
	userID int,
	channelID int,
	tokenID int,
	quota int,
) {
	t.Helper()
	require.NoError(t, model.CommitGroupModelDiscountSettlementWithUsage(
		settlementID,
		model.BillingUsageDelta{
			UserID:            userID,
			ChannelID:         channelID,
			QuotaDelta:        quota,
			RequestCountDelta: 1,
		},
	))
	if tokenID > 0 {
		require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).
			Update("used_quota", quota).Error)
	}
}

func makeTask(userId, channelId, quota, tokenId int, billingSource string, subscriptionId int) *model.Task {
	return &model.Task{
		TaskID:    "task_" + time.Now().Format("150405.000"),
		UserId:    userId,
		ChannelId: channelId,
		Quota:     quota,
		Status:    model.TaskStatus(model.TaskStatusInProgress),
		Group:     "default",
		Data:      json.RawMessage(`{}`),
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Properties: model.Properties{
			OriginModelName: "test-model",
		},
		PrivateData: model.TaskPrivateData{
			BillingSource:  billingSource,
			SubscriptionId: subscriptionId,
			TokenId:        tokenId,
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.02,
				GroupRatio:      1.0,
				OriginModelName: "test-model",
			},
		},
	}
}

func TestPriceDataOtherRatiosFilterAndSnapshot(t *testing.T) {
	priceData := types.PriceData{}

	priceData.AddOtherRatio("zero", 0)
	priceData.AddOtherRatio("negative", -0.5)
	priceData.AddOtherRatio("nan", math.NaN())
	priceData.AddOtherRatio("inf", math.Inf(1))
	priceData.AddOtherRatio("one", 1)
	priceData.AddOtherRatio("positive", 2.5)

	ratios := priceData.OtherRatios()
	require.Len(t, ratios, 2)
	assert.Equal(t, 1.0, ratios["one"])
	assert.Equal(t, 2.5, ratios["positive"])
	assert.True(t, priceData.HasOtherRatio("one"))
	assert.False(t, priceData.HasOtherRatio("zero"))

	ratios["positive"] = 99
	ratios["new"] = 3
	nextSnapshot := priceData.OtherRatios()
	assert.Equal(t, 2.5, nextSnapshot["positive"])
	assert.NotContains(t, nextSnapshot, "new")
}

func TestPriceDataReplaceAndApplyOtherRatios(t *testing.T) {
	priceData := types.PriceData{}

	replaced := priceData.ReplaceOtherRatios(map[string]float64{
		"zero":     0,
		"negative": -3,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
		"one":      1,
		"duration": 2,
		"size":     1.5,
	})

	require.True(t, replaced)
	assert.Equal(t, 3.0, priceData.OtherRatioMultiplier())
	assert.Equal(t, 30.0, priceData.ApplyOtherRatiosToFloat(10))
	assert.Equal(t, 10.0, priceData.RemoveOtherRatiosFromFloat(30))
	assert.True(t, decimal.NewFromInt(30).Equal(priceData.ApplyOtherRatiosToDecimal(decimal.NewFromInt(10))))

	replaced = priceData.ReplaceOtherRatios(map[string]float64{
		"zero": 0,
		"nan":  math.NaN(),
	})

	require.False(t, replaced)
	assert.Nil(t, priceData.OtherRatios())
	assert.Equal(t, 1.0, priceData.OtherRatioMultiplier())
}

func TestTaskBillingOtherFiltersHistoricalOtherRatios(t *testing.T) {
	task := makeTask(1, 1, 100, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{
		"seconds":  2,
		"identity": 1,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	}

	other := taskBillingOther(task)

	assert.Equal(t, 2.0, other["seconds"])
	assert.Equal(t, 1.0, other["identity"])
	assert.NotContains(t, other, "zero")
	assert.NotContains(t, other, "negative")
	assert.NotContains(t, other, "nan")
	assert.NotContains(t, other, "inf")
}

func TestTaskBillingContextPriceDataFiltersMultiplier(t *testing.T) {
	priceData := taskBillingContextPriceData(&model.TaskBillingContext{
		OtherRatios: map[string]float64{
			"seconds":  2,
			"size":     3,
			"identity": 1,
			"zero":     0,
			"negative": -1,
			"nan":      math.NaN(),
			"inf":      math.Inf(1),
		},
	})

	require.NotNil(t, priceData)
	assert.Equal(t, 6.0, priceData.OtherRatioMultiplier())
	assert.Equal(t, map[string]float64{
		"seconds":  2,
		"size":     3,
		"identity": 1,
	}, priceData.OtherRatios())
}

// ---------------------------------------------------------------------------
// Read-back helpers
// ---------------------------------------------------------------------------

func getUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return user.Quota
}

func getUserUsageAccounting(t *testing.T, id int) (int, int) {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("used_quota", "request_count").Where("id = ?", id).First(&user).Error)
	return user.UsedQuota, user.RequestCount
}

func getChannelUsedQuota(t *testing.T, id int) int64 {
	t.Helper()
	var channel model.Channel
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&channel).Error)
	return channel.UsedQuota
}

func getTokenRemainQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("remain_quota").Where("id = ?", id).First(&token).Error)
	return token.RemainQuota
}

func getTokenUsedQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&token).Error)
	return token.UsedQuota
}

func getSubscriptionUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", id).First(&sub).Error)
	return sub.AmountUsed
}

func getTaskQuota(t *testing.T, id int64) int {
	t.Helper()
	var task model.Task
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&task).Error)
	return task.Quota
}

func getMidjourneyTask(t *testing.T, id int) model.Midjourney {
	t.Helper()
	var task model.Midjourney
	require.NoError(t, model.DB.First(&task, id).Error)
	return task
}

func getLastLog(t *testing.T) *model.Log {
	t.Helper()
	var log model.Log
	err := model.LOG_DB.Order("id desc").First(&log).Error
	if err != nil {
		return nil
	}
	return &log
}

func countLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	model.LOG_DB.Model(&model.Log{}).Count(&count)
	return count
}

// ===========================================================================
// Legacy Midjourney billing tests
// ===========================================================================

func TestPrepareMidjourneyTaskBillingKeepsUnbilledMarkerClear(t *testing.T) {
	task := &model.Midjourney{Quota: 900, TokenId: 7, BillingChannelId: 8}

	prepared, err := PrepareMidjourneyTaskBilling(&relaycommon.RelayInfo{}, task, 900, false)

	require.NoError(t, err)
	assert.False(t, prepared)
	assert.Zero(t, task.Quota)
	assert.Zero(t, task.TokenId)
	assert.Zero(t, task.BillingChannelId)
}

func TestPrepareMidjourneyTaskBillingFreezesMonthlyOriginalAndDurableKey(t *testing.T) {
	snapshot := testGroupModelDiscountSnapshot()
	relayInfo := &relaycommon.RelayInfo{
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		GroupModelDiscountSnapshot: &snapshot,
		PriceData: types.PriceData{
			OriginalQuota: 600,
			Quota:         300,
		},
	}
	task := &model.Midjourney{MjId: "mj-monthly-prepare", ChannelId: 8}

	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 300, true)

	require.NoError(t, err)
	require.True(t, prepared)
	assert.Equal(t, 600, task.OriginalQuota)
	assert.Regexp(t, `^mj:[0-9a-f-]{36}$`, task.DiscountSettlementID)
	assert.NotContains(t, task.DiscountSettlementID, task.MjId)
	assert.Equal(t, snapshot.UsingGroup, task.UsingGroup)
	assert.Equal(t, snapshot.OriginModel, task.OriginModelName)
	assert.NotEmpty(t, task.DiscountPolicySnapshot)
	assert.Equal(t, model.TaskChargeStatePrepared, task.ChargeState)
}

func TestPrepareMidjourneyTaskBillingPreservesHistoricalSettlementKey(t *testing.T) {
	snapshot := testGroupModelDiscountSnapshot()
	relayInfo := &relaycommon.RelayInfo{
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		GroupModelDiscountSnapshot: &snapshot,
		PriceData:                  types.PriceData{OriginalQuota: 600, Quota: 300},
	}
	task := &model.Midjourney{
		MjId:                 "provider-id-after-upgrade",
		ChannelId:            8,
		DiscountSettlementID: "mj:legacy-provider-id",
	}

	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 300, true)

	require.NoError(t, err)
	require.True(t, prepared)
	assert.Equal(t, "mj:legacy-provider-id", task.DiscountSettlementID)
}

func TestPrepareMidjourneyTaskBillingValidationFailureLeavesNoRefundableMarker(t *testing.T) {
	snapshot := testGroupModelDiscountSnapshot()
	relayInfo := &relaycommon.RelayInfo{
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		GroupModelDiscountSnapshot: &snapshot,
		PriceData:                  types.PriceData{OriginalQuota: -1, Quota: 300},
	}
	task := &model.Midjourney{ChannelId: 8}

	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 300, true)

	require.Error(t, err)
	assert.False(t, prepared)
	assert.Zero(t, task.Quota)
	assert.Zero(t, task.OriginalQuota)
	assert.Empty(t, task.DiscountSettlementID)
}

func TestRefundMidjourneyQuotaRefusesPreparedUnchargedRow(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 45, 45, 45
	seedUser(t, userID, 10_000)
	seedToken(t, tokenID, userID, "sk-midjourney-prepared", 5_000)
	seedChannel(t, channelID)
	relayInfo := &relaycommon.RelayInfo{
		UserId:   userID,
		TokenId:  tokenID,
		TokenKey: "sk-midjourney-prepared",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID,
		},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-prepared", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 3_000, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())

	assert.False(t, RefundMidjourneyQuota(ctx, task, "must not refund prepared row"))
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 5_000, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, model.TaskChargeStatePrepared, persisted.ChargeState)
	assert.Equal(t, 3_000, persisted.Quota)
}

func TestSettleMidjourneyTaskBillingRequiresPersistedTask(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 49, 49, 49
	const initialUserQuota, initialTokenQuota, chargedQuota = 10000, 5000, 3000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-unpersisted", initialTokenQuota)
	seedChannel(t, channelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId:    userID,
		TokenId:   tokenID,
		TokenKey:  "sk-midjourney-unpersisted",
		UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID,
		},
	}
	task := &model.Midjourney{UserId: userID, ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)

	require.Error(t, err)
	assert.False(t, billed)
	assert.Equal(t, initialUserQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
}

func TestMidjourneyPollingWaitsUntilBillingIsReady(t *testing.T) {
	truncate(t)
	task := &model.Midjourney{
		MjId: "mj-billing-readiness-gate", Status: string(model.TaskStatusInProgress),
		Progress: "0%", ChargeState: model.TaskChargeStatePrepared,
	}
	require.NoError(t, task.Insert())

	assert.Empty(t, model.GetAllUnFinishTasks())
	assert.False(t, model.HasUnfinishedMidjourneyTasks())

	task.ChargeState = model.TaskChargeStatePendingReconcile
	require.NoError(t, task.UpdateBillingState())
	assert.Empty(t, model.GetAllUnFinishTasks())
	assert.False(t, model.HasUnfinishedMidjourneyTasks())

	task.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, task.UpdateBillingState())
	tasks := model.GetAllUnFinishTasks()
	require.Len(t, tasks, 1)
	assert.Equal(t, task.Id, tasks[0].Id)
	assert.True(t, model.HasUnfinishedMidjourneyTasks())
}

func TestSettleMidjourneyTaskBillingPendingPrewriteFailureMovesNoFunding(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 4901, 4901, 4901
	const initialUserQuota, initialTokenQuota, chargedQuota = 10_000, 5_000, 3_000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-prewrite", initialTokenQuota)
	seedChannel(t, channelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId: userID, TokenId: tokenID, TokenKey: "sk-midjourney-prewrite", UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-prewrite-failure", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_midjourney_pending_prewrite
		BEFORE UPDATE OF charge_state ON midjourneys
		WHEN NEW.charge_state = 'pending_reconcile'
		BEGIN
			SELECT RAISE(ABORT, 'forced Midjourney pending prewrite failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_pending_prewrite").Error })

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)

	require.Error(t, err)
	assert.False(t, billed)
	assert.Equal(t, initialUserQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, model.TaskChargeStatePrepared, persisted.ChargeState)
}

func TestSettleMidjourneyTaskBillingFinalStateFailureKeepsDurablePendingAndCannotReplay(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 4902, 4902, 4902
	const initialUserQuota, initialTokenQuota, chargedQuota = 10_000, 5_000, 3_000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-final-state", initialTokenQuota)
	seedChannel(t, channelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId: userID, TokenId: tokenID, TokenKey: "sk-midjourney-final-state", UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-final-state-failure", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_midjourney_final_charge_state
		BEFORE UPDATE OF charge_state ON midjourneys
		WHEN NEW.charge_state = 'charged'
		BEGIN
			SELECT RAISE(ABORT, 'forced Midjourney final state failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_final_charge_state").Error })

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)

	require.Error(t, err)
	assert.False(t, billed)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, chargedQuota, persisted.Quota)
	assert.Equal(t, chargedQuota, persisted.OriginalQuota)
	assert.Equal(t, BillingSourceWallet, persisted.BillingSource)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.ChargeState)

	billed, retryErr := SettleMidjourneyTaskBilling(relayInfo, &persisted, prepared)
	require.Error(t, retryErr)
	assert.False(t, billed)
	assert.False(t, RefundMidjourneyQuota(ctx, &persisted, "must not refund ambiguous fixed charge"))
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
}

func TestSettleMidjourneyTaskBillingRejectsStalePreparedClaimWithoutDoubleCharge(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 4903, 4903, 4903
	const initialUserQuota, initialTokenQuota, chargedQuota = 10_000, 5_000, 3_000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-stale-claim", initialTokenQuota)
	seedChannel(t, channelID)
	relayInfo := &relaycommon.RelayInfo{
		UserId: userID, TokenId: tokenID, TokenKey: "sk-midjourney-stale-claim", UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-stale-claim", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())
	stalePreparedCopy := getMidjourneyTask(t, task.Id)

	firstBilled, firstErr := SettleMidjourneyTaskBilling(relayInfo, task, prepared)
	secondBilled, secondErr := SettleMidjourneyTaskBilling(relayInfo, &stalePreparedCopy, prepared)

	require.NoError(t, firstErr)
	assert.True(t, firstBilled)
	require.Error(t, secondErr)
	assert.False(t, secondBilled)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, model.TaskChargeStateCharged, persisted.ChargeState)
}

func TestSettleMidjourneyTaskModelChargePersistsExactMonthlyNet(t *testing.T) {
	truncate(t)

	const userID, channelID = 48, 48
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	billing := &groupDiscountBillingRecorder{preConsumed: 600}
	relayInfo := &relaycommon.RelayInfo{
		UserId:                     userID,
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		Billing:                    billing,
		GroupModelDiscountSnapshot: &snapshot,
		PriceData: types.PriceData{
			OriginalQuota: 600,
			Quota:         300,
		},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-monthly-settle", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 300, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())

	billed, decision, err := SettleMidjourneyTaskModelCharge(newGroupModelDiscountTestContext(), relayInfo, task, prepared)

	require.NoError(t, err)
	require.True(t, billed)
	assert.True(t, decision.Applied)
	assert.Equal(t, 530, decision.ChargedQuota)
	assert.Equal(t, []int{530}, billing.settleCalls)
	assert.Equal(t, 530, task.Quota)
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, 600, persisted.OriginalQuota)
	assert.Equal(t, 530, persisted.Quota)
	assert.Equal(t, task.DiscountSettlementID, persisted.DiscountSettlementID)
	assert.Regexp(t, `^mj:[0-9a-f-]{36}$`, persisted.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStateCharged, persisted.ChargeState)
	settlement, err := model.GetGroupModelDiscountSettlement(task.DiscountSettlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlement.Status)
}

func TestSettleMidjourneyTaskModelChargeClearsUnfundedReserveFailure(t *testing.T) {
	truncate(t)
	const userID, channelID = 4804, 4805
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	billing := &groupDiscountBillingRecorder{preConsumed: 600, needsRefund: true}
	relayInfo := &relaycommon.RelayInfo{
		UserId:                     userID,
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		Billing:                    billing,
		GroupModelDiscountSnapshot: &snapshot,
		PriceData:                  types.PriceData{OriginalQuota: common.MaxQuota + 1, Quota: 300},
		ChannelMeta:                &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-monthly-reserve-failed", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 300, true)
	require.NoError(t, err)
	require.NoError(t, task.Insert())

	billed, decision, err := SettleMidjourneyTaskModelCharge(newGroupModelDiscountTestContext(), relayInfo, task, prepared)

	require.ErrorIs(t, err, groupdiscount.ErrInvalidOriginalQuota)
	assert.False(t, billed)
	assert.False(t, decision.Applied)
	assert.Zero(t, billing.settleCalls, "funding must not start when ledger reservation fails")
	assert.Zero(t, task.Quota)
	assert.Empty(t, task.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStateUncharged, task.ChargeState)
	persisted := getMidjourneyTask(t, task.Id)
	assert.Zero(t, persisted.Quota)
	assert.Empty(t, persisted.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStateUncharged, persisted.ChargeState)
}

func TestMidjourneyTaskNeedsRefundRecoversSettledAccountingAfterTaskStateWriteFailure(t *testing.T) {
	truncate(t)
	const userID, channelID = 4806, 4807
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	requestID := "mj:recover-settled-accounting"
	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     requestID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	require.NoError(t, model.CommitGroupModelDiscountSettlementWithUsage(requestID, model.BillingUsageDelta{
		UserID: userID, ChannelID: channelID, QuotaDelta: reservation.Calculation.ChargedQuota, RequestCountDelta: 1,
	}))
	task := &model.Midjourney{
		UserId: userID, MjId: "mj-recover-settled-accounting", ChannelId: channelID,
		BillingChannelId: channelID, OriginalQuota: 600, Quota: 300,
		DiscountSettlementID: requestID, ChargeState: model.TaskChargeStatePrepared,
	}
	require.NoError(t, task.Insert())
	assert.Empty(t, model.GetAllUnFinishTasks(), "settled evidence without an explicit recovery handoff may be a replay")
	require.NoError(t, task.MarkBillingRecoveryPending(model.TaskChargeStatePrepared))
	staleRecoveryOwner := getMidjourneyTask(t, task.Id)

	pollable := model.GetAllUnFinishTasks()
	require.Len(t, pollable, 1, "settled accounting must be recovered before the polling readiness gate")
	task = pollable[0]
	assert.False(t, staleRecoveryOwner.RecoverSettledInitialBilling(), "the recovery handoff must be claimed only once")
	assert.True(t, MidjourneyTaskNeedsRefund(task))
	assert.Equal(t, reservation.Calculation.ChargedQuota, task.Quota)
	assert.Equal(t, model.TaskChargeStateCharged, task.ChargeState)
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, reservation.Calculation.ChargedQuota, persisted.Quota)
	assert.Equal(t, model.TaskChargeStateCharged, persisted.ChargeState)
}

func TestSettleMidjourneyTaskModelChargeFinalWriteFailureHandsOffToPollingRecovery(t *testing.T) {
	truncate(t)
	const userID, channelID = 4810, 4811
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	billing := &groupDiscountBillingRecorder{preConsumed: 600}
	relayInfo := &relaycommon.RelayInfo{
		UserId: userID, UsingGroup: snapshot.UsingGroup, OriginModelName: snapshot.OriginModel,
		BillingSource: BillingSourceWallet, Billing: billing, GroupModelDiscountSnapshot: &snapshot,
		PriceData:   types.PriceData{OriginalQuota: 600, Quota: 300},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-final-write-poll-recovery", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 300, true)
	require.NoError(t, err)
	require.NoError(t, task.Insert())
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_mj_monthly_final_write
		BEFORE UPDATE OF charge_state ON midjourneys
		WHEN NEW.charge_state = 'charged'
		BEGIN
			SELECT RAISE(ABORT, 'forced monthly Midjourney final write failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_mj_monthly_final_write").Error })

	billed, decision, settleErr := SettleMidjourneyTaskModelCharge(
		newGroupModelDiscountTestContext(), relayInfo, task, prepared,
	)

	require.Error(t, settleErr)
	assert.False(t, billed)
	assert.True(t, decision.Applied)
	assert.False(t, decision.Reused)
	assert.Equal(t, []int{decision.ChargedQuota}, billing.settleCalls)
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, model.TaskChargeStatePrepared, persisted.ChargeState)
	assert.True(t, persisted.BillingRecoveryPending)
	require.NotNil(t, persisted.BillingReady)
	assert.False(t, *persisted.BillingReady)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_mj_monthly_final_write").Error)
	pollable := model.GetAllUnFinishTasks()
	require.Len(t, pollable, 1)
	assert.Equal(t, model.TaskChargeStateCharged, pollable[0].ChargeState)
	assert.Equal(t, decision.ChargedQuota, pollable[0].Quota)
	assert.False(t, pollable[0].BillingRecoveryPending)
	require.NotNil(t, pollable[0].BillingReady)
	assert.True(t, *pollable[0].BillingReady)
	assert.Equal(t, []int{decision.ChargedQuota}, billing.settleCalls, "poll recovery must not replay funding")
}

func TestSettleMidjourneyTaskModelChargeHandoffFailureMarksLedgerManual(t *testing.T) {
	truncate(t)
	const userID, channelID = 4812, 4813
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	billing := &groupDiscountBillingRecorder{preConsumed: 600}
	relayInfo := &relaycommon.RelayInfo{
		UserId: userID, UsingGroup: snapshot.UsingGroup, OriginModelName: snapshot.OriginModel,
		BillingSource: BillingSourceWallet, Billing: billing, GroupModelDiscountSnapshot: &snapshot,
		PriceData:   types.PriceData{OriginalQuota: 600, Quota: 300},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-final-write-handoff-failure", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 300, true)
	require.NoError(t, err)
	require.NoError(t, task.Insert())
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_mj_monthly_final_and_handoff
		BEFORE UPDATE ON midjourneys
		WHEN NEW.charge_state = 'charged' OR NEW.billing_recovery_pending = 1
		BEGIN
			SELECT RAISE(ABORT, 'forced monthly Midjourney final and handoff failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_mj_monthly_final_and_handoff").Error })

	billed, decision, settleErr := SettleMidjourneyTaskModelCharge(
		newGroupModelDiscountTestContext(), relayInfo, task, prepared,
	)

	require.Error(t, settleErr)
	assert.False(t, billed)
	assert.True(t, decision.Applied)
	settlement, loadErr := model.GetGroupModelDiscountSettlement(decision.RequestID)
	require.NoError(t, loadErr)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, settlement.PendingAction)
	assert.Empty(t, model.GetAllUnFinishTasks(), "failed handoff must remain blocked for manual reconciliation")
	assert.Equal(t, []int{decision.ChargedQuota}, billing.settleCalls)
}

func TestTaskNeedsBillingRefundRecoversSettledAccountingAfterTaskStateWriteFailure(t *testing.T) {
	truncate(t)
	const userID, channelID = 4808, 4809
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	requestID := "task:recover-settled-accounting"
	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     requestID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	require.NoError(t, model.CommitGroupModelDiscountSettlementWithUsage(requestID, model.BillingUsageDelta{
		UserID: userID, ChannelID: channelID, QuotaDelta: reservation.Calculation.ChargedQuota, RequestCountDelta: 1,
	}))
	task := &model.Task{
		TaskID: "task-recover-settled-accounting", UserId: userID, ChannelId: channelID,
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			OriginalQuota: 600, DiscountSettlementID: requestID, ChargeState: model.TaskChargeStatePrepared,
		}},
	}
	require.NoError(t, task.Insert())
	assert.Empty(t, model.GetAllUnFinishSyncTasks(10), "settled evidence without an explicit recovery handoff may be a replay")
	require.NoError(t, task.MarkBillingRecoveryPending())
	staleRecoveryOwner, err := model.GetTaskBillingState(task.ID)
	require.NoError(t, err)

	pollable := model.GetAllUnFinishSyncTasks(10)
	require.Len(t, pollable, 1, "settled accounting must be recovered before the polling readiness gate")
	task = pollable[0]
	assert.False(t, staleRecoveryOwner.RecoverSettledInitialBilling(), "the recovery handoff must be claimed only once")
	assert.True(t, taskNeedsBillingRefund(task))
	assert.Equal(t, reservation.Calculation.ChargedQuota, task.Quota)
	assert.Equal(t, model.TaskChargeStateCharged, task.PrivateData.BillingContext.ChargeState)
	persisted, err := model.GetTaskBillingState(task.ID)
	require.NoError(t, err)
	assert.Equal(t, reservation.Calculation.ChargedQuota, persisted.Quota)
	assert.Equal(t, model.TaskChargeStateCharged, persisted.PrivateData.BillingContext.ChargeState)
}

func TestSettleMidjourneyTaskModelChargeSameUpstreamIDAcrossUsersSettlesIndependently(t *testing.T) {
	truncate(t)

	const firstUserID, secondUserID, channelID = 4801, 4802, 4803
	require.NoError(t, model.DB.Create(&model.User{
		Id: firstUserID, Username: "mj-cross-user-first", AffCode: "mj-cross-user-first", Quota: 10_000, Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: secondUserID, Username: "mj-cross-user-second", AffCode: "mj-cross-user-second", Quota: 10_000, Status: common.UserStatusEnabled,
	}).Error)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()

	newTask := func(t *testing.T, userID int) (*relaycommon.RelayInfo, *model.Midjourney, *groupDiscountBillingRecorder) {
		t.Helper()
		billing := &groupDiscountBillingRecorder{preConsumed: 600}
		relayInfo := &relaycommon.RelayInfo{
			UserId:                     userID,
			UsingGroup:                 snapshot.UsingGroup,
			OriginModelName:            snapshot.OriginModel,
			BillingSource:              BillingSourceWallet,
			Billing:                    billing,
			GroupModelDiscountSnapshot: &snapshot,
			PriceData:                  types.PriceData{OriginalQuota: 600, Quota: 300},
			ChannelMeta:                &relaycommon.ChannelMeta{ChannelId: channelID},
		}
		task := &model.Midjourney{UserId: userID, MjId: "provider-reused-id", ChannelId: channelID}
		prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 300, true)
		require.NoError(t, err)
		require.True(t, prepared)
		require.NoError(t, task.Insert())
		return relayInfo, task, billing
	}

	firstInfo, firstTask, firstBilling := newTask(t, firstUserID)
	secondInfo, secondTask, secondBilling := newTask(t, secondUserID)
	require.NotEmpty(t, firstTask.DiscountSettlementID)
	require.NotEmpty(t, secondTask.DiscountSettlementID)
	require.NotEqual(t, firstTask.DiscountSettlementID, secondTask.DiscountSettlementID)

	firstBilled, firstDecision, firstErr := SettleMidjourneyTaskModelCharge(
		newGroupModelDiscountTestContext(), firstInfo, firstTask, true,
	)
	secondBilled, secondDecision, secondErr := SettleMidjourneyTaskModelCharge(
		newGroupModelDiscountTestContext(), secondInfo, secondTask, true,
	)

	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.True(t, firstBilled)
	assert.True(t, secondBilled)
	assert.False(t, firstDecision.Reused)
	assert.False(t, secondDecision.Reused)
	assert.Equal(t, 530, firstDecision.ChargedQuota)
	assert.Equal(t, 530, secondDecision.ChargedQuota)
	assert.Equal(t, []int{530}, firstBilling.settleCalls)
	assert.Equal(t, []int{530}, secondBilling.settleCalls)

	firstUsage, err := model.GetUserGroupModelMonthlyUsage(firstUserID, snapshot.UsingGroup, snapshot.OriginModel, snapshot.PeriodStart)
	require.NoError(t, err)
	secondUsage, err := model.GetUserGroupModelMonthlyUsage(secondUserID, snapshot.UsingGroup, snapshot.OriginModel, snapshot.PeriodStart)
	require.NoError(t, err)
	assert.EqualValues(t, 600, firstUsage.OriginalQuota)
	assert.EqualValues(t, 600, secondUsage.OriginalQuota)
}

func TestSettleMidjourneyTaskModelChargeReplayDoesNotDuplicateUsageMarker(t *testing.T) {
	truncate(t)

	const userID, channelID = 47, 47
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	firstBilling := &groupDiscountBillingRecorder{preConsumed: 600}
	firstInfo := &relaycommon.RelayInfo{
		UserId:                     userID,
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		Billing:                    firstBilling,
		GroupModelDiscountSnapshot: &snapshot,
		PriceData:                  types.PriceData{OriginalQuota: 600, Quota: 300},
		ChannelMeta:                &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	firstTask := &model.Midjourney{UserId: userID, MjId: "mj-monthly-replay", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(firstInfo, firstTask, 300, true)
	require.NoError(t, err)
	require.NoError(t, firstTask.Insert())
	billed, firstDecision, err := SettleMidjourneyTaskModelCharge(newGroupModelDiscountTestContext(), firstInfo, firstTask, prepared)
	require.NoError(t, err)
	require.True(t, billed)
	require.Equal(t, 530, firstDecision.ChargedQuota)

	replayBilling := &groupDiscountBillingRecorder{preConsumed: 600}
	replayInfo := &relaycommon.RelayInfo{
		UserId:                     userID,
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		Billing:                    replayBilling,
		GroupModelDiscountSnapshot: &snapshot,
		PriceData:                  types.PriceData{OriginalQuota: 600, Quota: 300},
		ChannelMeta:                &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	replayTask := &model.Midjourney{
		UserId: userID, MjId: "mj-monthly-replay", ChannelId: channelID,
		DiscountSettlementID: firstDecision.RequestID,
	}
	prepared, err = PrepareMidjourneyTaskBilling(replayInfo, replayTask, 300, true)
	require.NoError(t, err)
	require.NoError(t, replayTask.Insert())

	billed, replayDecision, err := SettleMidjourneyTaskModelCharge(newGroupModelDiscountTestContext(), replayInfo, replayTask, prepared)

	require.NoError(t, err)
	assert.False(t, billed)
	assert.True(t, replayDecision.Reused)
	assert.Equal(t, []int{0}, replayBilling.settleCalls)
	assert.Zero(t, replayTask.Quota)
	assert.Empty(t, replayTask.DiscountSettlementID)
	persisted := getMidjourneyTask(t, replayTask.Id)
	assert.Zero(t, persisted.Quota)
	assert.Empty(t, persisted.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStateReused, persisted.ChargeState)
	usage, err := model.GetUserGroupModelMonthlyUsage(userID, snapshot.UsingGroup, snapshot.OriginModel, snapshot.PeriodStart)
	require.NoError(t, err)
	assert.EqualValues(t, 600, usage.OriginalQuota)
	assert.EqualValues(t, 530, usage.ChargedQuota)
}

func TestSettleMidjourneyTaskModelChargeReplaySettleErrorStaysPending(t *testing.T) {
	truncate(t)

	const userID, channelID = 46, 46
	seedUser(t, userID, 10_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	firstInfo := &relaycommon.RelayInfo{
		UserId:                     userID,
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		Billing:                    &groupDiscountBillingRecorder{preConsumed: 600},
		GroupModelDiscountSnapshot: &snapshot,
		PriceData:                  types.PriceData{OriginalQuota: 600, Quota: 300},
		ChannelMeta:                &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	firstTask := &model.Midjourney{UserId: userID, MjId: "mj-monthly-replay-error", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(firstInfo, firstTask, 300, true)
	require.NoError(t, err)
	require.NoError(t, firstTask.Insert())
	_, firstDecision, err := SettleMidjourneyTaskModelCharge(newGroupModelDiscountTestContext(), firstInfo, firstTask, prepared)
	require.NoError(t, err)
	require.Equal(t, 530, firstDecision.ChargedQuota)

	settleErr := errors.New("fresh replay funding outcome is unknown")
	replayBilling := &groupDiscountBillingRecorder{preConsumed: 600, settleErr: settleErr}
	replayInfo := &relaycommon.RelayInfo{
		UserId:                     userID,
		UsingGroup:                 snapshot.UsingGroup,
		OriginModelName:            snapshot.OriginModel,
		BillingSource:              BillingSourceWallet,
		Billing:                    replayBilling,
		GroupModelDiscountSnapshot: &snapshot,
		PriceData:                  types.PriceData{OriginalQuota: 600, Quota: 300},
		ChannelMeta:                &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	replayTask := &model.Midjourney{
		UserId: userID, MjId: "mj-monthly-replay-error", ChannelId: channelID,
		DiscountSettlementID: firstDecision.RequestID,
	}
	prepared, err = PrepareMidjourneyTaskBilling(replayInfo, replayTask, 300, true)
	require.NoError(t, err)
	require.NoError(t, replayTask.Insert())

	billed, replayDecision, err := SettleMidjourneyTaskModelCharge(newGroupModelDiscountTestContext(), replayInfo, replayTask, prepared)

	require.ErrorIs(t, err, settleErr)
	assert.False(t, billed)
	assert.Equal(t, []int{0}, replayBilling.settleCalls)
	assert.Equal(t, replayDecision.ChargedQuota, replayTask.Quota)
	assert.Equal(t, firstDecision.RequestID, replayTask.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, replayTask.ChargeState)
	persisted := getMidjourneyTask(t, replayTask.Id)
	assert.Equal(t, replayDecision.ChargedQuota, persisted.Quota)
	assert.Equal(t, firstDecision.RequestID, persisted.DiscountSettlementID)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.ChargeState)

	billed, _, retryErr := SettleMidjourneyTaskModelCharge(newGroupModelDiscountTestContext(), replayInfo, &persisted, prepared)
	require.Error(t, retryErr)
	assert.False(t, billed)
	assert.Equal(t, []int{0}, replayBilling.settleCalls, "pending replay must not move funding a second time")
}

func TestMidjourneyRefundRestoresEveryAccountingElementOnBillingChannel(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, billingChannelID, executionChannelID = 50, 50, 50, 51
	const initialUserQuota, initialTokenQuota, chargedQuota = 10000, 5000, 3000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney", initialTokenQuota)
	seedChannel(t, billingChannelID)
	seedChannel(t, executionChannelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId:     userID,
		TokenId:    tokenID,
		TokenKey:   "sk-midjourney",
		UserQuota:  initialUserQuota,
		UsingGroup: "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: billingChannelID,
		},
	}
	task := &model.Midjourney{
		UserId:    userID,
		Action:    "IMAGINE",
		MjId:      "mj-accounting-refund",
		ChannelId: executionChannelID,
		Progress:  "0%",
	}

	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	assert.Equal(t, chargedQuota, task.Quota)
	assert.Zero(t, task.TokenId)
	assert.Equal(t, billingChannelID, task.BillingChannelId)
	require.NoError(t, task.Insert())

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)
	require.NoError(t, err)
	require.True(t, billed)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota-chargedQuota, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, chargedQuota, persisted.Quota)
	assert.Equal(t, tokenID, persisted.TokenId)
	assert.Equal(t, billingChannelID, persisted.BillingChannelId)

	seedChargedAccounting(t, userID, billingChannelID, tokenID, chargedQuota, 1)

	assert.True(t, RefundMidjourneyQuota(ctx, task, "构图失败"))
	assert.Equal(t, initialUserQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, billingChannelID))
	assert.Zero(t, getChannelUsedQuota(t, executionChannelID))

	persisted = getMidjourneyTask(t, task.Id)
	assert.Zero(t, persisted.Quota)
	assert.Equal(t, tokenID, persisted.TokenId)
	assert.Equal(t, billingChannelID, persisted.BillingChannelId)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, chargedQuota, log.Quota)
	assert.Equal(t, tokenID, log.TokenId)
	assert.Equal(t, billingChannelID, log.ChannelId)

	assert.True(t, RefundMidjourneyQuota(ctx, task, "duplicate poll"))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestSettleMidjourneyTaskBillingFundingErrorStaysPendingAndCannotReplay(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID = 52, 52, 52
	const initialUserQuota, initialTokenQuota, chargedQuota = 10000, 5000, 3000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-funding-failure", initialTokenQuota)
	seedChannel(t, channelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId:    userID,
		TokenId:   tokenID,
		TokenKey:  "sk-midjourney-funding-failure",
		UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID,
		},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-funding-failure", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())

	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_midjourney_user_update
		BEFORE UPDATE ON users
		WHEN OLD.id = 52
		BEGIN
			SELECT RAISE(ABORT, 'forced user quota failure');
		END;
	`).Error)
	t.Cleanup(func() {
		model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_user_update")
	})

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)

	require.Error(t, err)
	assert.False(t, billed)
	assert.Equal(t, initialUserQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, chargedQuota, persisted.Quota)
	assert.Equal(t, tokenID, persisted.TokenId, "pending intent keeps the token funding source auditable")
	assert.Equal(t, channelID, persisted.BillingChannelId)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.ChargeState)
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Zero(t, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	assert.Zero(t, countLogs(t))

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_midjourney_user_update").Error)
	billed, retryErr := SettleMidjourneyTaskBilling(relayInfo, &persisted, prepared)
	require.Error(t, retryErr)
	assert.False(t, billed)
	assert.Equal(t, initialUserQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
}

func TestSettleMidjourneyTaskBillingTokenFailureStaysPendingAndCannotOverRefund(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 53, 53, 53
	const initialUserQuota, initialTokenQuota, chargedQuota = 10000, 5000, 3000
	seedUser(t, userID, initialUserQuota)
	seedToken(t, tokenID, userID, "sk-midjourney-token-failure", initialTokenQuota)
	seedChannel(t, channelID)

	relayInfo := &relaycommon.RelayInfo{
		UserId:    userID,
		TokenId:   tokenID,
		TokenKey:  "sk-midjourney-token-failure",
		UserQuota: initialUserQuota,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: channelID,
		},
	}
	task := &model.Midjourney{UserId: userID, MjId: "mj-token-failure", ChannelId: channelID}
	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, chargedQuota, true)
	require.NoError(t, err)
	require.True(t, prepared)
	require.NoError(t, task.Insert())

	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_midjourney_token_update
		BEFORE UPDATE ON tokens
		WHEN OLD.id = 53
		BEGIN
			SELECT RAISE(ABORT, 'forced token quota failure');
		END;
	`).Error)
	t.Cleanup(func() {
		model.DB.Exec("DROP TRIGGER IF EXISTS fail_midjourney_token_update")
	})

	billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)

	require.Error(t, err)
	require.False(t, billed)
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, chargedQuota, persisted.Quota)
	assert.Equal(t, tokenID, persisted.TokenId, "pending intent keeps the token funding source auditable")
	assert.Equal(t, channelID, persisted.BillingChannelId)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.ChargeState)

	seedChargedAccounting(t, userID, channelID, 0, chargedQuota, 1)
	assert.False(t, RefundMidjourneyQuota(ctx, task, "token settlement failed"))
	assert.Equal(t, initialUserQuota-chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, initialTokenQuota, getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, chargedQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(chargedQuota), getChannelUsedQuota(t, channelID))
	assert.Zero(t, countLogs(t))
}

func TestPrepareMidjourneyTaskBillingRejectsSubscriptionBeforeCharge(t *testing.T) {
	task := &model.Midjourney{Quota: 900, TokenId: 7, BillingChannelId: 8}
	relayInfo := &relaycommon.RelayInfo{BillingSource: BillingSourceSubscription, SubscriptionId: 1}

	prepared, err := PrepareMidjourneyTaskBilling(relayInfo, task, 900, true)

	require.Error(t, err)
	assert.False(t, prepared)
	assert.Zero(t, task.Quota)
	assert.Zero(t, task.TokenId)
	assert.Zero(t, task.BillingChannelId)
}

func TestRefundMidjourneyQuotaUsesLegacyChannelFallbackWithoutTokenAdjustment(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 54, 54, 54
	const walletAfterCharge, tokenQuota, chargedQuota = 7000, 5000, 3000
	seedUser(t, userID, walletAfterCharge)
	seedToken(t, tokenID, userID, "sk-midjourney-legacy", tokenQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, chargedQuota, 1)
	task := &model.Midjourney{
		UserId:    userID,
		MjId:      "mj-legacy-fallback",
		Action:    "IMAGINE",
		ChannelId: channelID,
		Quota:     chargedQuota,
		TokenId:   0,
		Progress:  "0%",
	}
	require.NoError(t, task.Insert())

	assert.True(t, RefundMidjourneyQuota(ctx, task, "legacy failure"))

	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenQuota, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, channelID, log.ChannelId)
	assert.Zero(t, log.TokenId)
}

func TestRefundMidjourneyQuotaReversesMonthlySettlementAfterExactNetRefund(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 55, 55
	const walletAfterCharge = 10_000
	seedUser(t, userID, walletAfterCharge)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "mj:mj-monthly-refund"
	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	require.Equal(t, 530, reservation.Calculation.ChargedQuota)
	chargedQuota := reservation.Calculation.ChargedQuota
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, chargedQuota)

	snapshotJSON, err := common.Marshal(&snapshot)
	require.NoError(t, err)
	task := &model.Midjourney{
		UserId:                 userID,
		MjId:                   "mj-monthly-refund",
		Action:                 "IMAGINE",
		ChannelId:              channelID,
		BillingChannelId:       channelID,
		Quota:                  chargedQuota,
		OriginalQuota:          600,
		BillingSource:          BillingSourceWallet,
		UsingGroup:             snapshot.UsingGroup,
		OriginModelName:        snapshot.OriginModel,
		DiscountSettlementID:   settlementID,
		DiscountPolicySnapshot: string(snapshotJSON),
	}
	require.NoError(t, task.Insert())

	require.True(t, RefundMidjourneyQuota(ctx, task, "upstream failed"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	usage, err := model.GetUserGroupModelMonthlyUsage(userID, snapshot.UsingGroup, snapshot.OriginModel, snapshot.PeriodStart)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
	settlement, err := model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusReversed, settlement.Status)

	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, chargedQuota, persisted.RefundedQuota)
	assert.Zero(t, persisted.Quota)
	require.True(t, RefundMidjourneyQuota(ctx, &persisted, "duplicate poll"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestRefundMidjourneyQuotaRecoversAfterAtomicReverseBeforeTaskState(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 57, 57
	const walletAfterCharge = 10_000
	seedUser(t, userID, walletAfterCharge)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "mj:mj-monthly-reverse-state-recovery"
	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID: settlementID, UserID: userID, UsingGroup: snapshot.UsingGroup,
		OriginModel: snapshot.OriginModel, Snapshot: snapshot, OriginalQuota: 600,
	})
	require.NoError(t, err)
	chargedQuota := reservation.Calculation.ChargedQuota
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, chargedQuota)
	task := &model.Midjourney{
		UserId: userID, MjId: "mj-monthly-reverse-state-recovery", Action: "IMAGINE",
		ChannelId: channelID, BillingChannelId: channelID, Quota: chargedQuota,
		OriginalQuota: 600, BillingSource: BillingSourceWallet,
		UsingGroup: snapshot.UsingGroup, OriginModelName: snapshot.OriginModel,
		DiscountSettlementID: settlementID, ChargeState: model.TaskChargeStateCharged,
	}
	require.NoError(t, task.Insert())
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_mj_reverse_final_state
		BEFORE UPDATE OF refund_state ON midjourneys
		WHEN NEW.refund_state = 'accounting_applied'
		BEGIN
			SELECT RAISE(ABORT, 'forced Midjourney reverse final state failure');
		END;
	`).Error)

	require.False(t, RefundMidjourneyQuota(ctx, task, "upstream failed"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	settlement, err := model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusReversed, settlement.Status)
	assert.True(t, settlement.ReverseAccountingApplied)
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, model.TaskRefundStateAccountingPending, persisted.RefundState)
	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_mj_reverse_final_state").Error)

	require.True(t, RefundMidjourneyQuota(ctx, &persisted, "retry local state"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	assert.Zero(t, getMidjourneyTask(t, task.Id).Quota)
}

func TestRefundMidjourneyQuotaTokenFailureLeavesAmbiguousStageAndNeverRefundsFundsTwice(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, quota = 56, 56, 56, 3_000
	seedUser(t, userID, 7_000)
	seedToken(t, tokenID, userID, "sk-mj-refund-pending", 2_000)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, quota, 1)
	task := &model.Midjourney{
		UserId:           userID,
		MjId:             "mj-refund-token-pending",
		Action:           "IMAGINE",
		ChannelId:        channelID,
		BillingChannelId: channelID,
		Quota:            quota,
		TokenId:          tokenID,
		BillingSource:    BillingSourceWallet,
		ChargeState:      model.TaskChargeStateCharged,
	}
	require.NoError(t, task.Insert())
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_mj_refund_token_update
		BEFORE UPDATE ON tokens
		WHEN OLD.id = 56
		BEGIN
			SELECT RAISE(ABORT, 'forced Midjourney refund token failure');
		END;
	`).Error)

	assert.False(t, RefundMidjourneyQuota(ctx, task, "token refund failure"))
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 2_000, getTokenRemainQuota(t, tokenID))
	persisted := getMidjourneyTask(t, task.Id)
	assert.Equal(t, model.TaskRefundStateTokenPending, persisted.RefundState)
	assert.Equal(t, quota, persisted.RefundedQuota)
	assert.Equal(t, quota, persisted.Quota)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_mj_refund_token_update").Error)
	assert.False(t, RefundMidjourneyQuota(ctx, &persisted, "ambiguous replay"))
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 2_000, getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, quota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(quota), getChannelUsedQuota(t, channelID))
}

// ===========================================================================
// RefundTaskQuota tests
// ===========================================================================

func TestRefundTaskQuota_Wallet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1, 1, 1
	const initQuota, preConsumed = 10000, 3000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-test-key", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)

	assert.True(t, RefundTaskQuota(ctx, task, "task failed: upstream error"))

	// User quota should increase by preConsumed
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Token remain_quota should increase, used_quota should decrease
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))

	// A refund log should be created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed, log.Quota)
	assert.Equal(t, "test-model", log.ModelName)
	assert.Zero(t, task.Quota)
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestRefundTaskQuota_Subscription(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 2, 2, 2, 1
	const preConsumed = 2000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-key", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)
	require.NoError(t, model.DB.Create(task).Error)

	assert.True(t, RefundTaskQuota(ctx, task, "subscription task failed"))

	// Subscription used should decrease by preConsumed
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, subID))

	// Token should also be refunded
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Zero(t, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestRefundTaskQuota_ZeroQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 3
	seedUser(t, userID, 5000)

	task := makeTask(userID, 0, 0, 0, BillingSourceWallet, 0)

	assert.True(t, RefundTaskQuota(ctx, task, "zero quota task"))

	// No change to user quota
	assert.Equal(t, 5000, getUserQuota(t, userID))

	// No log created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRefundTaskQuota_NoToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 4, 4
	const initQuota, preConsumed = 10000, 1500

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0) // TokenId=0
	require.NoError(t, model.DB.Create(task).Error)

	assert.True(t, RefundTaskQuota(ctx, task, "no token task failed"))

	// User quota refunded
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))

	// Log created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestRefundTaskQuota_FundingFailureKeepsAccountingAndPendingMarker(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID, preConsumed = 5, 5, 1200
	seedUser(t, userID, 5000)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, preConsumed, 1)
	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceSubscription, 9999)
	task.Status = model.TaskStatusFailure
	require.NoError(t, model.DB.Create(task).Error)

	assert.False(t, RefundTaskQuota(ctx, task, "subscription missing"))
	assert.Equal(t, 5000, getUserQuota(t, userID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, preConsumed, getTaskQuota(t, task.ID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, preConsumed, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(preConsumed), getChannelUsedQuota(t, channelID))
	assert.Equal(t, int64(0), countLogs(t))
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, model.TaskRefundStateFundingPending, persisted.PrivateData.BillingContext.RefundState)
}

func TestRefundTaskQuotaTokenFailureLeavesAmbiguousStageAndNeverRefundsFundsTwice(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, quota = 8, 8, 8, 3_000
	seedUser(t, userID, 7_000)
	seedToken(t, tokenID, userID, "sk-task-refund-pending", 2_000)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, quota, 1)
	task := makeTask(userID, channelID, quota, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task-refund-token-pending"
	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_refund_token_update
		BEFORE UPDATE ON tokens
		WHEN OLD.id = 8
		BEGIN
			SELECT RAISE(ABORT, 'forced task refund token failure');
		END;
	`).Error)

	assert.False(t, RefundTaskQuota(ctx, task, "token refund failure"))
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 2_000, getTokenRemainQuota(t, tokenID))
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, model.TaskRefundStateTokenPending, persisted.PrivateData.BillingContext.RefundState)
	assert.Equal(t, quota, persisted.PrivateData.BillingContext.RefundedQuota)
	assert.Equal(t, quota, persisted.Quota)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_task_refund_token_update").Error)
	assert.False(t, RefundTaskQuota(ctx, &persisted, "ambiguous replay"))
	assert.Equal(t, 10_000, getUserQuota(t, userID))
	assert.Equal(t, 2_000, getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, quota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(quota), getChannelUsedQuota(t, channelID))
}

func TestRefundTaskQuotaReversesMonthlySettlementAfterExactNetRefund(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 6, 6
	const walletAfterCharge = 10_000
	seedUser(t, userID, walletAfterCharge)
	seedChannel(t, channelID)

	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-refund"
	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	require.Equal(t, 530, reservation.Calculation.ChargedQuota)

	chargedQuota := reservation.Calculation.ChargedQuota
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, chargedQuota)
	task := makeTask(userID, channelID, chargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-refund"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext.OriginalQuota = 600
	task.PrivateData.BillingContext.NetQuota = chargedQuota
	task.PrivateData.BillingContext.DiscountSettlementID = settlementID
	task.PrivateData.BillingContext.GroupModelDiscountSnapshot = &snapshot
	require.NoError(t, model.DB.Create(task).Error)

	require.True(t, RefundTaskQuota(ctx, task, "upstream failed"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	usage, err := model.GetUserGroupModelMonthlyUsage(userID, snapshot.UsingGroup, snapshot.OriginModel, snapshot.PeriodStart)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
	settlement, err := model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusReversed, settlement.Status)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, chargedQuota, persisted.PrivateData.BillingContext.RefundedQuota)
	assert.Zero(t, persisted.Quota)

	require.True(t, RefundTaskQuota(ctx, &persisted, "duplicate poll"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(1), countLogs(t))
}

func TestRefundTaskQuotaRecoversAfterAtomicReverseBeforeTaskState(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 8, 8
	const walletAfterCharge = 10_000
	seedUser(t, userID, walletAfterCharge)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-reverse-state-recovery"
	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID: settlementID, UserID: userID, UsingGroup: snapshot.UsingGroup,
		OriginModel: snapshot.OriginModel, Snapshot: snapshot, OriginalQuota: 600,
	})
	require.NoError(t, err)
	chargedQuota := reservation.Calculation.ChargedQuota
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, chargedQuota)
	task := makeTask(userID, channelID, chargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-reverse-state-recovery"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: snapshot.OriginModel, OriginalQuota: 600, NetQuota: chargedQuota,
		DiscountSettlementID: settlementID, ChargeState: model.TaskChargeStateCharged,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_reverse_final_state
		BEFORE UPDATE OF private_data ON tasks
		WHEN json_extract(NEW.private_data, '$.billing_context.refund_state') = 'accounting_applied'
		BEGIN
			SELECT RAISE(ABORT, 'forced task reverse final state failure');
		END;
	`).Error)

	require.False(t, RefundTaskQuota(ctx, task, "upstream failed"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	settlement, err := model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusReversed, settlement.Status)
	assert.True(t, settlement.ReverseAccountingApplied)
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, model.TaskRefundStateAccountingPending, persisted.PrivateData.BillingContext.RefundState)
	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_task_reverse_final_state").Error)

	require.True(t, RefundTaskQuota(ctx, &persisted, "retry local state"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestRefundTaskQuotaRetriesPendingLedgerWithoutSecondFundingRefund(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 7, 7
	const walletAfterCharge = 10_000
	seedUser(t, userID, walletAfterCharge)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-refund-pending"
	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	chargedQuota := reservation.Calculation.ChargedQuota
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, chargedQuota)
	task := makeTask(userID, channelID, chargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-refund-pending"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext.OriginalQuota = 600
	task.PrivateData.BillingContext.NetQuota = chargedQuota
	task.PrivateData.BillingContext.DiscountSettlementID = settlementID
	task.PrivateData.BillingContext.GroupModelDiscountSnapshot = &snapshot
	require.NoError(t, model.DB.Create(task).Error)

	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_discount_reverse
		BEFORE UPDATE OF status ON group_model_discount_settlements
		WHEN NEW.status = 'reversed'
		BEGIN
			SELECT RAISE(ABORT, 'forced discount reverse failure');
		END;
	`).Error)
	require.False(t, RefundTaskQuota(ctx, task, "upstream failed"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	settlement, err := model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, settlement.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionReverseAfterRefund, settlement.PendingAction)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, chargedQuota, persisted.PrivateData.BillingContext.RefundedQuota)
	assert.Equal(t, chargedQuota, persisted.Quota)
	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_task_discount_reverse").Error)

	require.True(t, RefundTaskQuota(ctx, &persisted, "retry pending ledger"))
	assert.Equal(t, walletAfterCharge+chargedQuota, getUserQuota(t, userID))
	settlement, err = model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusReversed, settlement.Status)
	assert.Zero(t, getTaskQuota(t, task.ID))
}

func TestZeroNetMonthlyFailuresStillReverseOriginalQuotaCursor(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	snapshot := testGroupModelDiscountSnapshot()
	snapshot.PolicyHash = "policy-zero-refund"
	snapshot.Tiers = []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0}}

	const taskUserID, taskChannelID = 9, 9
	seedUser(t, taskUserID, 1_000)
	seedChannel(t, taskChannelID)
	taskSettlementID := "task:zero-net-failure"
	taskReservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID: taskSettlementID, UserID: taskUserID, UsingGroup: snapshot.UsingGroup,
		OriginModel: snapshot.OriginModel, Snapshot: snapshot, OriginalQuota: 1,
	})
	require.NoError(t, err)
	require.Zero(t, taskReservation.Calculation.ChargedQuota)
	commitGroupModelSettlementWithAccounting(t, taskSettlementID, taskUserID, taskChannelID, 0, 0)
	task := makeTask(taskUserID, taskChannelID, 0, 0, BillingSourceWallet, 0)
	task.TaskID = "zero-net-failure"
	task.PrivateData.BillingContext.OriginalQuota = 1
	task.PrivateData.BillingContext.DiscountSettlementID = taskSettlementID
	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, model.DB.Create(task).Error)
	require.True(t, taskNeedsBillingRefund(task))
	require.True(t, RefundTaskQuota(ctx, task, "zero net failed"))
	taskSettlement, err := model.GetGroupModelDiscountSettlement(taskSettlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusReversed, taskSettlement.Status)

	const mjUserID, mjChannelID = taskUserID, taskChannelID
	mjSettlementID := "mj:zero-net-failure"
	mjReservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID: mjSettlementID, UserID: mjUserID, UsingGroup: snapshot.UsingGroup,
		OriginModel: snapshot.OriginModel, Snapshot: snapshot, OriginalQuota: 1,
	})
	require.NoError(t, err)
	require.Zero(t, mjReservation.Calculation.ChargedQuota)
	commitGroupModelSettlementWithAccounting(t, mjSettlementID, mjUserID, mjChannelID, 0, 0)
	mjTask := &model.Midjourney{
		UserId: mjUserID, MjId: "zero-net-failure", ChannelId: mjChannelID,
		OriginalQuota: 1, DiscountSettlementID: mjSettlementID,
		ChargeState: model.TaskChargeStateCharged,
	}
	require.NoError(t, mjTask.Insert())
	require.True(t, MidjourneyTaskNeedsRefund(mjTask))
	require.True(t, RefundMidjourneyQuota(ctx, mjTask, "zero net failed"))
	mjSettlement, err := model.GetGroupModelDiscountSettlement(mjSettlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusReversed, mjSettlement.Status)
}

// ===========================================================================
// RecalculateTaskQuota tests
// ===========================================================================

func TestRecalculate_PositiveDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 10, 10, 10
	const initQuota, preConsumed = 10000, 2000
	const actualQuota = 3000 // under-charged by 1000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-pos", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should decrease by the delta (1000 additional charge)
	assert.Equal(t, initQuota-(actualQuota-preConsumed), getUserQuota(t, userID))

	// Token should also be charged the delta
	assert.Equal(t, tokenRemain-(actualQuota-preConsumed), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, actualQuota, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Consume (additional charge)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
	assert.Equal(t, actualQuota-preConsumed, log.Quota)
}

func TestRecalculate_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 11, 11, 11
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged by 2000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-neg", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should increase by abs(delta) = 2000 (refund overpayment)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))

	// Token should be refunded the difference
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, actualQuota, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))

	// task.Quota updated
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Refund
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed-actualQuota, log.Quota)
}

func TestRecalculate_ZeroDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 12
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, preConsumed, "exact match")

	// No change to user quota
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No log created (delta is zero)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_ActualQuotaZero(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 13
	const initQuota = 10000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, 5000, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, 0, "zero actual")

	// No change (early return)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestTaskPollingAndTimeoutWaitUntilBillingIsReady(t *testing.T) {
	truncate(t)
	task := makeTask(1201, 1201, 0, 0, BillingSourceWallet, 0)
	task.TaskID = "task-billing-readiness-gate"
	task.SubmitTime = 1
	task.Progress = "0%"
	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStatePrepared
	require.NoError(t, model.DB.Create(task).Error)

	assert.Empty(t, model.GetAllUnFinishSyncTasks(10))
	assert.Empty(t, model.GetTimedOutUnfinishedTasks(2, 10))
	assert.False(t, model.HasUnfinishedSyncTasks())

	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStatePendingReconcile
	require.NoError(t, task.UpdateBillingState())
	assert.Empty(t, model.GetAllUnFinishSyncTasks(10))
	assert.Empty(t, model.GetTimedOutUnfinishedTasks(2, 10))
	assert.False(t, model.HasUnfinishedSyncTasks())

	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, task.UpdateBillingState())
	pollable := model.GetAllUnFinishSyncTasks(10)
	require.Len(t, pollable, 1)
	assert.Equal(t, task.ID, pollable[0].ID)
	timedOut := model.GetTimedOutUnfinishedTasks(2, 10)
	require.Len(t, timedOut, 1)
	assert.Equal(t, task.ID, timedOut[0].ID)
	assert.True(t, model.HasUnfinishedSyncTasks())
}

func TestRecalculateTaskQuotaPendingPrewriteFailureMovesNoFundingTokenOrStats(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1301, 1301, 1301
	const walletQuota, tokenQuota, confirmedQuota, targetQuota = 10_000, 5_000, 2_000, 3_000
	seedUser(t, userID, walletQuota)
	seedToken(t, tokenID, userID, "sk-task-prewrite", tokenQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, confirmedQuota, 1)
	task := makeTask(userID, channelID, confirmedQuota, tokenID, BillingSourceWallet, 0)
	task.TaskID = "fixed-task-prewrite-failure"
	task.PrivateData.BillingContext.NetQuota = confirmedQuota
	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_pending_prewrite
		BEFORE UPDATE OF private_data ON tasks
		WHEN json_extract(NEW.private_data, '$.billing_context.charge_state') = 'pending_reconcile'
		BEGIN
			SELECT RAISE(ABORT, 'forced task pending prewrite failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_task_pending_prewrite").Error })

	RecalculateTaskQuota(ctx, task, targetQuota, "fixed task prewrite failure")

	assert.Equal(t, walletQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenQuota, getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, confirmedQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(confirmedQuota), getChannelUsedQuota(t, channelID))
	persisted, err := model.GetTaskBillingState(task.ID)
	require.NoError(t, err)
	assert.Equal(t, confirmedQuota, persisted.Quota)
	assert.Equal(t, model.TaskChargeStateCharged, persisted.PrivateData.BillingContext.ChargeState)
}

func TestRecalculateTaskQuotaFinalStateFailureKeepsConfirmedQuotaAndTargetPendingWithoutReplay(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1302, 1302, 1302
	const walletQuota, tokenQuota, confirmedQuota, targetQuota = 10_000, 5_000, 2_000, 3_000
	seedUser(t, userID, walletQuota)
	seedToken(t, tokenID, userID, "sk-task-final-state", tokenQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, confirmedQuota, 1)
	task := makeTask(userID, channelID, confirmedQuota, tokenID, BillingSourceWallet, 0)
	task.TaskID = "fixed-task-final-state-failure"
	task.PrivateData.BillingContext.NetQuota = confirmedQuota
	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_final_charge_state
		BEFORE UPDATE OF private_data ON tasks
		WHEN json_extract(NEW.private_data, '$.billing_context.charge_state') = 'charged'
		 AND json_extract(NEW.private_data, '$.billing_context.pending_net_quota') IS NULL
		BEGIN
			SELECT RAISE(ABORT, 'forced task final state failure');
		END;
	`).Error)
	t.Cleanup(func() { _ = model.DB.Exec("DROP TRIGGER IF EXISTS fail_task_final_charge_state").Error })

	RecalculateTaskQuota(ctx, task, targetQuota, "fixed task final state failure")

	assert.Equal(t, walletQuota-(targetQuota-confirmedQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenQuota-(targetQuota-confirmedQuota), getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, targetQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(targetQuota), getChannelUsedQuota(t, channelID))
	persisted, err := model.GetTaskBillingState(task.ID)
	require.NoError(t, err)
	assert.Equal(t, confirmedQuota, persisted.Quota, "task quota remains the last confirmed value")
	assert.Equal(t, confirmedQuota, persisted.PrivateData.BillingContext.NetQuota)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.PrivateData.BillingContext.ChargeState)
	persistedPrivateData, err := common.Marshal(persisted.PrivateData)
	require.NoError(t, err)
	assert.Contains(t, string(persistedPrivateData), `"pending_net_quota":3000`)

	RecalculateTaskQuota(ctx, persisted, targetQuota, "must not replay fixed task adjustment")
	assert.False(t, RefundTaskQuota(ctx, persisted, "must not refund ambiguous fixed task adjustment"))
	assert.Equal(t, walletQuota-(targetQuota-confirmedQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenQuota-(targetQuota-confirmedQuota), getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount = getUserUsageAccounting(t, userID)
	assert.Equal(t, targetQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(targetQuota), getChannelUsedQuota(t, channelID))
}

func TestRecalculate_Subscription_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 14, 14, 14, 2
	const preConsumed = 5000
	const actualQuota = 2000 // over-charged by 3000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-recalc", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription over-charge")

	// Subscription used should decrease by delta (refund 3000)
	assert.Equal(t, subUsed-int64(preConsumed-actualQuota), getSubscriptionUsed(t, subID))

	// Token refunded
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, actualQuota, getTokenUsedQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))

	assert.Equal(t, actualQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRecalculateWalletFundingErrorStaysPendingWithoutAdjustmentOrRefundReplay(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 27, 27
	const walletQuota, preConsumed, actualQuota = 10_000, 2_000, 3_000
	seedUser(t, userID, walletQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, preConsumed, 1)
	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.TaskID = "fallback-wallet-funding-unknown"
	task.PrivateData.BillingContext.NetQuota = preConsumed
	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_fallback_wallet_adjustment
		BEFORE UPDATE ON users
		WHEN OLD.id = 27
		BEGIN
			SELECT RAISE(ABORT, 'forced fallback wallet adjustment failure');
		END;
	`).Error)

	RecalculateTaskQuota(ctx, task, actualQuota, "wallet funding failure")

	assert.Equal(t, walletQuota, getUserQuota(t, userID))
	assert.Equal(t, preConsumed, task.Quota)
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, preConsumed, persisted.Quota)
	assert.Equal(t, preConsumed, persisted.PrivateData.BillingContext.NetQuota)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.PrivateData.BillingContext.ChargeState)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_fallback_wallet_adjustment").Error)
	RecalculateTaskQuota(ctx, &persisted, actualQuota, "must not replay wallet adjustment")
	assert.False(t, RefundTaskQuota(ctx, &persisted, "must not replay wallet refund"))
	assert.Equal(t, walletQuota, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, preConsumed, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(preConsumed), getChannelUsedQuota(t, channelID))
}

func TestRecalculateSubscriptionFundingErrorStaysPendingWithoutAdjustmentReplay(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID, subscriptionID = 28, 28, 28
	const preConsumed, actualQuota = 2_000, 3_000
	const subscriptionTotal, subscriptionUsed int64 = 100_000, 50_000
	seedUser(t, userID, 0)
	seedChannel(t, channelID)
	seedSubscription(t, subscriptionID, userID, subscriptionTotal, subscriptionUsed)
	seedChargedAccounting(t, userID, channelID, 0, preConsumed, 1)
	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceSubscription, subscriptionID)
	task.TaskID = "fallback-subscription-funding-unknown"
	task.PrivateData.BillingContext.NetQuota = preConsumed
	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_fallback_subscription_adjustment
		BEFORE UPDATE ON user_subscriptions
		WHEN OLD.id = 28
		BEGIN
			SELECT RAISE(ABORT, 'forced fallback subscription adjustment failure');
		END;
	`).Error)

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription funding failure")

	assert.Equal(t, subscriptionUsed, getSubscriptionUsed(t, subscriptionID))
	assert.Equal(t, preConsumed, task.Quota)
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, preConsumed, persisted.Quota)
	assert.Equal(t, preConsumed, persisted.PrivateData.BillingContext.NetQuota)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.PrivateData.BillingContext.ChargeState)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_fallback_subscription_adjustment").Error)
	RecalculateTaskQuota(ctx, &persisted, actualQuota, "must not replay subscription adjustment")
	assert.Equal(t, subscriptionUsed, getSubscriptionUsed(t, subscriptionID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, preConsumed, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(preConsumed), getChannelUsedQuota(t, channelID))
}

func TestRecalculateTaskQuotaByTokensUsesFrozenTaskRatios(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 15, 15
	const initialQuota, preConsumed = 1_000_000, 100
	seedUser(t, userID, initialQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.ModelRatio = 2
	task.PrivateData.BillingContext.GroupRatio = 0.5
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{"seconds": 2}
	require.NoError(t, model.DB.Create(task).Error)

	previousModelRatios := ratio_setting.ModelRatio2JSONString()
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-model":7}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":9}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
	})

	RecalculateTaskQuotaByTokens(ctx, task, 100)

	// Frozen submit-time inputs: 100 tokens * 2 model ratio * 0.5 group
	// ratio * 2 seconds = 200. Current settings must not affect completion.
	assert.Equal(t, 200, task.Quota)
	assert.Equal(t, initialQuota-100, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, 200, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(200), getChannelUsedQuota(t, channelID))
}

func TestTaskTokenQuotaFromFrozenRatiosKeepsPositiveOriginalBillable(t *testing.T) {
	originalQuota, fallbackNetQuota, _, _ := taskTokenQuotaFromFrozenRatios(1, 0.5, 1, 1)

	assert.Equal(t, 1, originalQuota)
	assert.Zero(t, fallbackNetQuota)
}

func TestRecalculateTaskQuotaByTokensAdjustsMonthlyLedgerFromFrozenOriginal(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 16, 16
	const walletAfterSubmit = 10_000
	seedUser(t, userID, walletAfterSubmit)
	seedChannel(t, channelID)

	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-token-adjust"
	initial, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	require.Equal(t, 530, initial.Calculation.ChargedQuota)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, initial.Calculation.ChargedQuota)

	task := makeTask(userID, channelID, initial.Calculation.ChargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-token-adjust"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio:                 0.5,
		ModelRatio:                 2,
		OriginModelName:            snapshot.OriginModel,
		OriginalQuota:              600,
		NetQuota:                   initial.Calculation.ChargedQuota,
		DiscountSettlementID:       settlementID,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)

	// Frozen original = 400 tokens * 2 model ratio = 800. The monthly
	// policy's second tier charges the +200 original delta at 0.8, so the
	// exact current net becomes 530 + 160 = 690. The fixed group fallback
	// (400) must never be used as the monthly charge.
	RecalculateTaskQuotaByTokens(ctx, task, 400)

	assert.Equal(t, 690, task.Quota)
	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, 690, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(690), getChannelUsedQuota(t, channelID))
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Equal(t, 800, task.PrivateData.BillingContext.OriginalQuota)
	assert.Equal(t, 690, task.PrivateData.BillingContext.NetQuota)

	settlement, err := model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.EqualValues(t, 800, settlement.CurrentOriginalQuota)
	assert.EqualValues(t, 690, settlement.CurrentChargedQuota)
	usage, err := model.GetUserGroupModelMonthlyUsage(userID, snapshot.UsingGroup, snapshot.OriginModel, snapshot.PeriodStart)
	require.NoError(t, err)
	assert.EqualValues(t, 800, usage.OriginalQuota)
	assert.EqualValues(t, 690, usage.ChargedQuota)

	var adjustment model.GroupModelDiscountAdjustment
	require.NoError(t, model.DB.Where("settlement_request_id = ?", settlementID).First(&adjustment).Error)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, adjustment.Status)
}

func TestRecalculateTaskQuotaByTokensDoesNotCommitPendingAdjustmentWithoutFundingEvidence(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 17, 17
	const walletAfterSubmit = 10_000
	seedUser(t, userID, walletAfterSubmit)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-pending-adjust"
	initial, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	require.Equal(t, 530, initial.Calculation.ChargedQuota)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, initial.Calculation.ChargedQuota)

	task := makeTask(userID, channelID, initial.Calculation.ChargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-pending-adjust"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio:                 0.5,
		ModelRatio:                 2,
		OriginModelName:            snapshot.OriginModel,
		OriginalQuota:              600,
		NetQuota:                   initial.Calculation.ChargedQuota,
		DiscountSettlementID:       settlementID,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)

	adjustmentID := settlementID + ":complete"
	_, err = model.ReserveGroupModelDiscountAdjustment(model.GroupModelDiscountAdjustmentInput{
		AdjustmentID:        adjustmentID,
		SettlementRequestID: settlementID,
		NewOriginalQuota:    800,
	})
	require.NoError(t, err)
	require.NoError(t, model.MarkGroupModelDiscountAdjustmentPendingReconcile(
		adjustmentID,
		model.GroupModelDiscountPendingActionUnknownManual,
	))

	RecalculateTaskQuotaByTokens(ctx, task, 400)

	assert.Equal(t, initial.Calculation.ChargedQuota, task.Quota)
	assert.Equal(t, walletAfterSubmit, getUserQuota(t, userID))
	assert.Empty(t, task.PrivateData.BillingContext.DiscountAdjustmentID)
	adjustment, err := model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, adjustment.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, adjustment.PendingAction)
}

func TestRecalculateTaskQuotaByTokensRetriesPendingCommitWithoutSecondFundingDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 18, 18
	const walletAfterSubmit = 10_000
	seedUser(t, userID, walletAfterSubmit)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-pending-commit"
	initial, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, initial.Calculation.ChargedQuota)
	task := makeTask(userID, channelID, initial.Calculation.ChargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-pending-commit"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio:                 0.5,
		ModelRatio:                 2,
		OriginModelName:            snapshot.OriginModel,
		OriginalQuota:              600,
		NetQuota:                   initial.Calculation.ChargedQuota,
		DiscountSettlementID:       settlementID,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)

	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_adjustment_commit
		BEFORE UPDATE OF status ON group_model_discount_adjustments
		WHEN NEW.status = 'settled'
		BEGIN
			SELECT RAISE(ABORT, 'forced adjustment commit failure');
		END;
	`).Error)
	RecalculateTaskQuotaByTokens(ctx, task, 400)

	assert.Equal(t, 690, task.Quota)
	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	adjustmentID := settlementID + ":complete"
	adjustment, err := model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, adjustment.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionCommitAfterFunding, adjustment.PendingAction)
	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_task_adjustment_commit").Error)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	RecalculateTaskQuotaByTokens(ctx, &persisted, 400)

	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	adjustment, err = model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, adjustment.Status)
	assert.Equal(t, 690, persisted.Quota)
}

func TestRecalculateTaskQuotaByTokensRetriesAtomicAccountingFailureWithoutSecondFundingDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 29, 29
	const walletAfterSubmit = 10_000
	seedUser(t, userID, walletAfterSubmit)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-accounting-retry"
	initial, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID: settlementID, UserID: userID, UsingGroup: snapshot.UsingGroup,
		OriginModel: snapshot.OriginModel, Snapshot: snapshot, OriginalQuota: 600,
	})
	require.NoError(t, err)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, initial.Calculation.ChargedQuota)

	task := makeTask(userID, channelID, initial.Calculation.ChargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-accounting-retry"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio: 0.5, ModelRatio: 2, OriginModelName: snapshot.OriginModel,
		OriginalQuota: 600, NetQuota: initial.Calculation.ChargedQuota,
		DiscountSettlementID: settlementID, ChargeState: model.TaskChargeStateCharged,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_adjustment_channel_accounting
		BEFORE UPDATE OF used_quota ON channels
		WHEN OLD.id = 29
		BEGIN
			SELECT RAISE(ABORT, 'forced adjustment accounting failure');
		END;
	`).Error)

	RecalculateTaskQuotaByTokens(ctx, task, 400)

	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, initial.Calculation.ChargedQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(initial.Calculation.ChargedQuota), getChannelUsedQuota(t, channelID))
	adjustmentID := settlementID + ":complete"
	adjustment, err := model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, adjustment.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionCommitAfterFunding, adjustment.PendingAction)
	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_task_adjustment_channel_accounting").Error)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	RecalculateTaskQuotaByTokens(ctx, &persisted, 400)

	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	usedQuota, requestCount = getUserUsageAccounting(t, userID)
	assert.Equal(t, 690, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(690), getChannelUsedQuota(t, channelID))
	adjustment, err = model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, adjustment.Status)
	assert.True(t, adjustment.AccountingApplied)
	assert.Equal(t, 160, adjustment.AccountingQuotaDelta)
	assert.Equal(t, model.TaskChargeStateCharged, persisted.PrivateData.BillingContext.ChargeState)
}

func TestRecalculateTaskQuotaByTokensRecoversAfterAtomicCommitBeforeFinalTaskState(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 30, 30
	const walletAfterSubmit = 10_000
	seedUser(t, userID, walletAfterSubmit)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-final-state-recovery"
	initial, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID: settlementID, UserID: userID, UsingGroup: snapshot.UsingGroup,
		OriginModel: snapshot.OriginModel, Snapshot: snapshot, OriginalQuota: 600,
	})
	require.NoError(t, err)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, initial.Calculation.ChargedQuota)

	task := makeTask(userID, channelID, initial.Calculation.ChargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-final-state-recovery"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio: 0.5, ModelRatio: 2, OriginModelName: snapshot.OriginModel,
		OriginalQuota: 600, NetQuota: initial.Calculation.ChargedQuota,
		DiscountSettlementID: settlementID, ChargeState: model.TaskChargeStateCharged,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_adjustment_final_state
		BEFORE UPDATE OF private_data ON tasks
		WHEN json_extract(NEW.private_data, '$.billing_context.discount_adjustment_id') = 'task:task-monthly-final-state-recovery:complete'
		 AND json_extract(NEW.private_data, '$.billing_context.charge_state') = 'charged'
		BEGIN
			SELECT RAISE(ABORT, 'forced final task billing state failure');
		END;
	`).Error)

	RecalculateTaskQuotaByTokens(ctx, task, 400)

	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, 690, usedQuota)
	assert.Equal(t, 1, requestCount)
	adjustmentID := settlementID + ":complete"
	adjustment, err := model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, adjustment.Status)
	assert.True(t, adjustment.AccountingApplied)
	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_task_adjustment_final_state").Error)

	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.PrivateData.BillingContext.ChargeState)
	RecalculateTaskQuotaByTokens(ctx, &persisted, 400)

	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	usedQuota, requestCount = getUserUsageAccounting(t, userID)
	assert.Equal(t, 690, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(690), getChannelUsedQuota(t, channelID))
	assert.Equal(t, 690, persisted.Quota)
	assert.Equal(t, model.TaskChargeStateCharged, persisted.PrivateData.BillingContext.ChargeState)
}

func TestRecalculateTaskQuotaByTokensAppliesExactZeroNetAdjustment(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 19, 19
	const walletAfterSubmit = 1_000
	seedUser(t, userID, walletAfterSubmit)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	snapshot.PolicyHash = "policy-zero-net"
	snapshot.Tiers = []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0.1}}
	settlementID := "task:task-monthly-zero-net"
	initial, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 6,
	})
	require.NoError(t, err)
	require.Equal(t, 1, initial.Calculation.ChargedQuota)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, 1)

	task := makeTask(userID, channelID, 1, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-zero-net"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio:                 1,
		ModelRatio:                 1,
		OriginModelName:            snapshot.OriginModel,
		OriginalQuota:              6,
		NetQuota:                   1,
		DiscountSettlementID:       settlementID,
		ChargeState:                model.TaskChargeStateCharged,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)

	RecalculateTaskQuotaByTokens(ctx, task, 1)

	assert.Zero(t, task.Quota)
	assert.Equal(t, walletAfterSubmit+1, getUserQuota(t, userID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))
	settlement, err := model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, settlement.CurrentOriginalQuota)
	assert.Zero(t, settlement.CurrentChargedQuota)
	adjustment, err := model.GetGroupModelDiscountAdjustment(settlementID + ":complete")
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, adjustment.Status)
}

func TestRecalculateTaskQuotaByTokensTokenFailureLeavesPendingWithoutSecondFundingDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 20, 20, 20
	const walletAfterSubmit = 10_000
	seedUser(t, userID, walletAfterSubmit)
	seedToken(t, tokenID, userID, "sk-task-adjust-pending", 5_000)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-token-pending"
	initial, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, tokenID, initial.Calculation.ChargedQuota)
	task := makeTask(userID, channelID, initial.Calculation.ChargedQuota, tokenID, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-token-pending"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio:                 0.5,
		ModelRatio:                 2,
		OriginModelName:            snapshot.OriginModel,
		OriginalQuota:              600,
		NetQuota:                   initial.Calculation.ChargedQuota,
		DiscountSettlementID:       settlementID,
		ChargeState:                model.TaskChargeStateCharged,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_adjust_token_update
		BEFORE UPDATE ON tokens
		WHEN OLD.id = 20
		BEGIN
			SELECT RAISE(ABORT, 'forced task adjustment token failure');
		END;
	`).Error)

	RecalculateTaskQuotaByTokens(ctx, task, 400)
	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	assert.Equal(t, 5_000, getTokenRemainQuota(t, tokenID))
	adjustmentID := settlementID + ":complete"
	adjustment, err := model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, adjustment.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, adjustment.PendingAction)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_task_adjust_token_update").Error)
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	assert.Equal(t, initial.Calculation.ChargedQuota, persisted.Quota)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.PrivateData.BillingContext.ChargeState)
	RecalculateTaskQuotaByTokens(ctx, &persisted, 400)
	assert.Equal(t, walletAfterSubmit-160, getUserQuota(t, userID))
	adjustment, err = model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, adjustment.Status)
}

func TestRecalculateTaskQuotaByTokensFundingErrorLeavesUnknownAdjustmentWithoutReplay(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 25, 25
	const walletAfterSubmit = 10_000
	seedUser(t, userID, walletAfterSubmit)
	seedChannel(t, channelID)
	snapshot := testGroupModelDiscountSnapshot()
	settlementID := "task:task-monthly-funding-unknown"
	initial, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 600,
	})
	require.NoError(t, err)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, initial.Calculation.ChargedQuota)
	task := makeTask(userID, channelID, initial.Calculation.ChargedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = "task-monthly-funding-unknown"
	task.Group = snapshot.UsingGroup
	task.Properties.OriginModelName = snapshot.OriginModel
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio:                 0.5,
		ModelRatio:                 2,
		OriginModelName:            snapshot.OriginModel,
		OriginalQuota:              600,
		NetQuota:                   initial.Calculation.ChargedQuota,
		DiscountSettlementID:       settlementID,
		ChargeState:                model.TaskChargeStateCharged,
		GroupModelDiscountSnapshot: &snapshot,
	}
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DB.Exec(`
		CREATE TRIGGER fail_task_adjust_funding_update
		BEFORE UPDATE ON users
		WHEN OLD.id = 25
		BEGIN
			SELECT RAISE(ABORT, 'forced task adjustment funding failure');
		END;
	`).Error)

	RecalculateTaskQuotaByTokens(ctx, task, 400)

	assert.Equal(t, walletAfterSubmit, getUserQuota(t, userID))
	adjustmentID := settlementID + ":complete"
	adjustment, err := model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, adjustment.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, adjustment.PendingAction)
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	assert.Equal(t, initial.Calculation.ChargedQuota, persisted.Quota)
	assert.Equal(t, model.TaskChargeStatePendingReconcile, persisted.PrivateData.BillingContext.ChargeState)

	require.NoError(t, model.DB.Exec("DROP TRIGGER fail_task_adjust_funding_update").Error)
	RecalculateTaskQuotaByTokens(ctx, &persisted, 400)
	assert.Equal(t, walletAfterSubmit, getUserQuota(t, userID))
	adjustment, err = model.GetGroupModelDiscountAdjustment(adjustmentID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusPendingReconcile, adjustment.Status)
	assert.Equal(t, model.GroupModelDiscountPendingActionUnknownManual, adjustment.PendingAction)
}

// ===========================================================================
// CAS + Billing integration tests
// Simulates the flow in updateVideoSingleTask (service/task_polling.go)
// ===========================================================================

type videoFailurePollingAdaptor struct{}

func (*videoFailurePollingAdaptor) Init(*relaycommon.RelayInfo) {}

func (*videoFailurePollingAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"provider":"failure"}`)),
	}, nil
}

func (*videoFailurePollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return relaycommon.FailTaskInfo("provider failed"), nil
}

func (*videoFailurePollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func TestUpdateVideoSingleTaskFailureReversesZeroNetMonthlyOriginal(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	snapshot := testGroupModelDiscountSnapshot()
	snapshot.PolicyHash = "policy-video-zero-refund"
	snapshot.Tiers = []groupdiscount.Tier{{MinMonthlyOriginalQuota: 0, Ratio: 0}}

	const userID, channelID = 26, 26
	seedUser(t, userID, 1_000)
	seedChannel(t, channelID)
	settlementID := "task:video-zero-net-failure"
	reservation, err := model.ReserveGroupModelDiscount(model.GroupModelDiscountReserveInput{
		RequestID:     settlementID,
		UserID:        userID,
		UsingGroup:    snapshot.UsingGroup,
		OriginModel:   snapshot.OriginModel,
		Snapshot:      snapshot,
		OriginalQuota: 1,
	})
	require.NoError(t, err)
	require.Zero(t, reservation.Calculation.ChargedQuota)
	commitGroupModelSettlementWithAccounting(t, settlementID, userID, channelID, 0, 0)

	task := makeTask(userID, channelID, 0, 0, BillingSourceWallet, 0)
	task.TaskID = "video-zero-net-failure"
	task.PrivateData.UpstreamTaskID = "upstream-video-zero-net-failure"
	task.PrivateData.BillingContext.OriginalQuota = 1
	task.PrivateData.BillingContext.NetQuota = 0
	task.PrivateData.BillingContext.DiscountSettlementID = settlementID
	task.PrivateData.BillingContext.ChargeState = model.TaskChargeStateCharged
	require.NoError(t, model.DB.Create(task).Error)

	err = updateVideoSingleTask(
		ctx,
		&videoFailurePollingAdaptor{},
		&model.Channel{Id: channelID, Key: "sk-video-zero-net-failure"},
		task.PrivateData.UpstreamTaskID,
		map[string]*model.Task{task.PrivateData.UpstreamTaskID: task},
	)

	require.NoError(t, err)
	settlement, err := model.GetGroupModelDiscountSettlement(settlementID)
	require.NoError(t, err)
	assert.Equal(t, model.GroupModelDiscountStatusReversed, settlement.Status)
	usage, err := model.GetUserGroupModelMonthlyUsage(userID, snapshot.UsingGroup, snapshot.OriginModel, snapshot.PeriodStart)
	require.NoError(t, err)
	assert.Zero(t, usage.OriginalQuota)
	assert.Zero(t, usage.ChargedQuota)
}

// simulatePollBilling reproduces the CAS + billing logic from updateVideoSingleTask.
// It takes a persisted task (already in DB), applies the new status, and performs
// the conditional update + billing exactly as the polling loop does.
func simulatePollBilling(ctx context.Context, task *model.Task, newStatus model.TaskStatus, actualQuota int) {
	snap := task.Snapshot()

	shouldRefund := false
	shouldSettle := false

	task.Status = newStatus
	switch string(newStatus) {
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		task.FinishTime = 9999
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Progress = "100%"
		task.FinishTime = 9999
		task.FailReason = "upstream error"
		if taskNeedsBillingRefund(task) {
			shouldRefund = true
		}
	default:
		task.Progress = "50%"
	}

	isDone := task.Status == model.TaskStatus(model.TaskStatusSuccess) || task.Status == model.TaskStatus(model.TaskStatusFailure)
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			shouldRefund = false
			shouldSettle = false
		}
	} else if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	if shouldSettle && actualQuota > 0 {
		RecalculateTaskQuota(ctx, task, actualQuota, "test settle")
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
}

func TestCASGuardedRefund_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 20, 20, 20
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-win", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS wins: task in DB should now be FAILURE
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	assert.Zero(t, reloaded.Quota)

	// Refund should have happened
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Zero(t, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Zero(t, getChannelUsedQuota(t, channelID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestCASGuardedRefund_Lose(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 21, 21, 21
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-lose", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	// Create task with IN_PROGRESS in DB
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate another process already transitioning to FAILURE
	model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusFailure)

	// Our process still has the old in-memory state (IN_PROGRESS) and tries to transition
	// task.Status is still IN_PROGRESS in the snapshot
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS lost: user quota should NOT change (no double refund)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, preConsumed, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(preConsumed), getChannelUsedQuota(t, channelID))

	// No billing log should be created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestCASGuardedSettle_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 22, 22, 22
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged, should get partial refund
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-settle-win", tokenRemain)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, tokenID, preConsumed, 1)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusSuccess), actualQuota)

	// CAS wins: task should be SUCCESS
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)

	// Settlement should refund the over-charge (5000 - 3000 = 2000 back to user)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
	usedQuota, requestCount := getUserUsageAccounting(t, userID)
	assert.Equal(t, actualQuota, usedQuota)
	assert.Equal(t, 1, requestCount)
	assert.Equal(t, int64(actualQuota), getChannelUsedQuota(t, channelID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)
}

func TestNonTerminalUpdate_NoBilling(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 23, 23
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	task.Progress = "20%"
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate a non-terminal poll update (still IN_PROGRESS, progress changed)
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusInProgress), 0)

	// User quota should NOT change
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No billing log
	assert.Equal(t, int64(0), countLogs(t))

	// Task progress should be updated in DB
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "50%", reloaded.Progress)
}

// ===========================================================================
// Mock adaptor for settleTaskBillingOnComplete tests
// ===========================================================================

type mockAdaptor struct {
	adjustReturn int
}

func (m *mockAdaptor) Init(_ *relaycommon.RelayInfo) {}
func (m *mockAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, nil
}
func (m *mockAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) { return nil, nil }
func (m *mockAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return m.adjustReturn
}

// ===========================================================================
// PerCallBilling tests — settleTaskBillingOnComplete
// ===========================================================================

func TestSettle_PerCallBilling_SkipsAdaptorAdjust(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 30, 30, 30
	const initQuota, preConsumed = 10000, 5000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-adaptor", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 2000}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no adjustment despite adaptor returning 2000
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_PerCallBilling_SkipsTotalTokens(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 31, 31, 31
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 7000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-tokens", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 0}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TotalTokens: 9999}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no recalculation by tokens
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_NonPerCallBilling_AppliesAdaptorAdjustment(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 32, 32, 32
	const initQuota, preConsumed = 10000, 5000
	const adaptorQuota = 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-nonpercall-adj", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	// PerCallBilling defaults to false

	adaptor := &mockAdaptor{adjustReturn: adaptorQuota}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Non-per-call: adaptor adjustment applies (refund 2000)
	assert.Equal(t, initQuota+(preConsumed-adaptorQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-adaptorQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, adaptorQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestSettle_MonthlyDiscountSkipsAdaptorQuotaWithoutOriginalContract(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 33, 33
	const walletQuota, submittedNet = 10_000, 5_000
	seedUser(t, userID, walletQuota)
	seedChannel(t, channelID)
	seedChargedAccounting(t, userID, channelID, 0, submittedNet, 1)
	task := makeTask(userID, channelID, submittedNet, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.DiscountSettlementID = "task:missing-original-contract"
	adaptor := &mockAdaptor{adjustReturn: 3_000}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	assert.Equal(t, submittedNet, task.Quota)
	assert.Equal(t, walletQuota, getUserQuota(t, userID))
	usedQuota, _ := getUserUsageAccounting(t, userID)
	assert.Equal(t, submittedNet, usedQuota)
}
