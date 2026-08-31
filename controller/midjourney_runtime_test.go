package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMidjourneyRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMemoryCache := common.MemoryCacheEnabled
	previousRedis := common.RedisEnabled
	previousBatchUpdate := common.BatchUpdateEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	service.InitHttpClient()
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Channel{},
		&model.Midjourney{},
		&model.Log{},
	))

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled = previousMemoryCache
		common.RedisEnabled = previousRedis
		common.BatchUpdateEnabled = previousBatchUpdate
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func seedMidjourneyRuntimeUser(t *testing.T, db *gorm.DB, id int, usedQuota int) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id:        id,
		Username:  fmt.Sprintf("mj-user-%d", id),
		AffCode:   fmt.Sprintf("mj-aff-%d", id),
		Status:    common.UserStatusEnabled,
		UsedQuota: usedQuota,
	}).Error)
}

func TestRunMidjourneyTaskUpdateOnceUpdatesAndRefundsEveryLocalTaskSharingUpstreamID(t *testing.T) {
	db := setupMidjourneyRuntimeTestDB(t)
	seedMidjourneyRuntimeUser(t, db, 1, 10)
	seedMidjourneyRuntimeUser(t, db, 2, 10)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/mj/task/list-by-condition", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`[{"id":"duplicate-upstream-id","progress":"100%","status":"FAILURE","failReason":"upstream failed"}]`))
		assert.NoError(t, err)
	}))
	defer upstream.Close()

	baseURL := upstream.URL
	channel := &model.Channel{
		Id:        11,
		Name:      "mj-runtime",
		Key:       "mj-secret",
		Status:    common.ChannelStatusEnabled,
		BaseURL:   &baseURL,
		UsedQuota: 20,
	}
	require.NoError(t, db.Create(channel).Error)

	for _, userID := range []int{1, 2} {
		require.NoError(t, db.Create(&model.Midjourney{
			UserId:           userID,
			MjId:             "duplicate-upstream-id",
			Action:           "IMAGINE",
			Status:           "SUBMITTED",
			Progress:         "0%",
			ChannelId:        channel.Id,
			BillingChannelId: channel.Id,
			Quota:            10,
			OriginalQuota:    10,
			ChargeState:      model.TaskChargeStateCharged,
			SubmitTime:       time.Now().UnixMilli(),
		}).Error)
	}

	runMidjourneyTaskUpdateOnce(context.Background(), nil)

	var tasks []model.Midjourney
	require.NoError(t, db.Order("id ASC").Find(&tasks).Error)
	require.Len(t, tasks, 2)
	for _, task := range tasks {
		assert.Equal(t, "FAILURE", task.Status)
		assert.Equal(t, "100%", task.Progress)
		assert.Equal(t, 0, task.Quota)
		assert.Equal(t, 10, task.RefundedQuota)
		assert.Equal(t, model.TaskRefundStateCommitted, task.RefundState)
	}

	for _, userID := range []int{1, 2} {
		var user model.User
		require.NoError(t, db.First(&user, userID).Error)
		assert.Equal(t, 10, user.Quota)
		assert.Equal(t, 0, user.UsedQuota)
	}
	require.NoError(t, db.First(channel, channel.Id).Error)
	assert.EqualValues(t, 0, channel.UsedQuota)

	var refundLogs int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&refundLogs).Error)
	assert.EqualValues(t, 2, refundLogs)

	// A later polling pass sees no unfinished rows and must not refund again.
	runMidjourneyTaskUpdateOnce(context.Background(), nil)
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&refundLogs).Error)
	assert.EqualValues(t, 2, refundLogs)
}

func TestRunMidjourneyTaskUpdateOnceScopesDuplicateUpstreamIDByChannel(t *testing.T) {
	db := setupMidjourneyRuntimeTestDB(t)
	seedMidjourneyRuntimeUser(t, db, 1, 0)
	seedMidjourneyRuntimeUser(t, db, 2, 0)

	now := time.Now().UnixMilli()
	newUpstream := func(progress string, status string, prompt string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := fmt.Fprintf(
				w,
				`[{"id":"duplicate-upstream-id","progress":%q,"status":%q,"promptEn":%q,"submitTime":%d}]`,
				progress,
				status,
				prompt,
				now,
			)
			assert.NoError(t, err)
		}))
	}
	firstUpstream := newUpstream("25%", "IN_PROGRESS", "first channel")
	defer firstUpstream.Close()
	secondUpstream := newUpstream("100%", "SUCCESS", "second channel")
	defer secondUpstream.Close()

	channels := []*model.Channel{
		{Id: 11, Name: "first", Key: "first-secret", Status: common.ChannelStatusEnabled, BaseURL: &firstUpstream.URL},
		{Id: 22, Name: "second", Key: "second-secret", Status: common.ChannelStatusEnabled, BaseURL: &secondUpstream.URL},
	}
	for _, channel := range channels {
		require.NoError(t, db.Create(channel).Error)
	}
	tasks := []*model.Midjourney{
		{UserId: 1, MjId: "duplicate-upstream-id", ChannelId: 11, Status: "SUBMITTED", Progress: "0%", SubmitTime: now},
		{UserId: 2, MjId: "duplicate-upstream-id", ChannelId: 22, Status: "SUBMITTED", Progress: "0%", SubmitTime: now},
	}
	for _, task := range tasks {
		require.NoError(t, db.Create(task).Error)
	}

	runMidjourneyTaskUpdateOnce(context.Background(), nil)

	require.NoError(t, db.First(tasks[0], tasks[0].Id).Error)
	require.NoError(t, db.First(tasks[1], tasks[1].Id).Error)
	assert.Equal(t, "25%", tasks[0].Progress)
	assert.Equal(t, "IN_PROGRESS", tasks[0].Status)
	assert.Equal(t, "first channel", tasks[0].PromptEn)
	assert.Equal(t, "100%", tasks[1].Progress)
	assert.Equal(t, "SUCCESS", tasks[1].Status)
	assert.Equal(t, "second channel", tasks[1].PromptEn)
}

func TestRunMidjourneyTaskUpdateOnceChannelFailureUsesLocalPrimaryIDs(t *testing.T) {
	db := setupMidjourneyRuntimeTestDB(t)
	seedMidjourneyRuntimeUser(t, db, 1, 10)
	seedMidjourneyRuntimeUser(t, db, 2, 10)

	missingChannelTask := &model.Midjourney{
		UserId:           1,
		MjId:             "duplicate-upstream-id",
		Action:           "IMAGINE",
		Status:           "SUBMITTED",
		Progress:         "0%",
		ChannelId:        404,
		BillingChannelId: 404,
		Quota:            10,
		OriginalQuota:    10,
		ChargeState:      model.TaskChargeStateCharged,
	}
	completedOtherChannelTask := &model.Midjourney{
		UserId:           2,
		MjId:             "duplicate-upstream-id",
		Action:           "IMAGINE",
		Status:           "SUCCESS",
		Progress:         "100%",
		ChannelId:        22,
		BillingChannelId: 22,
		Quota:            10,
		OriginalQuota:    10,
		ChargeState:      model.TaskChargeStateCharged,
	}
	require.NoError(t, db.Create(missingChannelTask).Error)
	require.NoError(t, db.Create(completedOtherChannelTask).Error)

	runMidjourneyTaskUpdateOnce(context.Background(), nil)

	require.NoError(t, db.First(missingChannelTask, missingChannelTask.Id).Error)
	assert.Equal(t, "FAILURE", missingChannelTask.Status)
	assert.Equal(t, "100%", missingChannelTask.Progress)
	assert.Equal(t, 0, missingChannelTask.Quota)
	assert.Equal(t, model.TaskRefundStateCommitted, missingChannelTask.RefundState)

	require.NoError(t, db.First(completedOtherChannelTask, completedOtherChannelTask.Id).Error)
	assert.Equal(t, "SUCCESS", completedOtherChannelTask.Status)
	assert.Equal(t, "100%", completedOtherChannelTask.Progress)
	assert.Equal(t, 10, completedOtherChannelTask.Quota)
	assert.Empty(t, completedOtherChannelTask.RefundState)

	var firstUser, secondUser model.User
	require.NoError(t, db.First(&firstUser, 1).Error)
	require.NoError(t, db.First(&secondUser, 2).Error)
	assert.Equal(t, 10, firstUser.Quota)
	assert.Equal(t, 0, firstUser.UsedQuota)
	assert.Equal(t, 0, secondUser.Quota)
	assert.Equal(t, 10, secondUser.UsedQuota)
}
