package relay

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMidjourneyNotifyTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousMemoryCache := common.MemoryCacheEnabled
	previousRedis := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Midjourney{}))

	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
		common.RedisEnabled = previousRedis
		common.SetMainDatabaseType(previousMainDatabaseType)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func seedMidjourneyNotifyUser(t *testing.T, db *gorm.DB, id int) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id:       id,
		Username: fmt.Sprintf("mj-notify-user-%d", id),
		AffCode:  fmt.Sprintf("mj-notify-aff-%d", id),
		Status:   common.UserStatusEnabled,
	}).Error)
}

func TestRelayMidjourneyNotifyScopesDuplicateUpstreamIDToAuthenticatedUser(t *testing.T) {
	db := setupMidjourneyNotifyTestDB(t)
	seedMidjourneyNotifyUser(t, db, 1)
	seedMidjourneyNotifyUser(t, db, 2)

	tasks := []*model.Midjourney{
		{UserId: 1, MjId: "duplicate-upstream-id", ChannelId: 11, Status: "SUBMITTED", Progress: "0%"},
		{UserId: 2, MjId: "duplicate-upstream-id", ChannelId: 22, Status: "SUBMITTED", Progress: "0%"},
		{UserId: 2, MjId: "duplicate-upstream-id", ChannelId: 33, Status: "SUBMITTED", Progress: "0%"},
	}
	for _, task := range tasks {
		require.NoError(t, db.Create(task).Error)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/notify", bytes.NewBufferString(
		`{"id":"duplicate-upstream-id","progress":"50%","status":"IN_PROGRESS","promptEn":"owned update"}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 2)

	response := RelayMidjourneyNotify(c)
	require.Nil(t, response)

	for index, task := range tasks {
		require.NoError(t, db.First(task, task.Id).Error)
		if index == 0 {
			assert.Equal(t, "SUBMITTED", task.Status)
			assert.Equal(t, "0%", task.Progress)
			assert.Empty(t, task.PromptEn)
			continue
		}
		assert.Equal(t, "IN_PROGRESS", task.Status)
		assert.Equal(t, "50%", task.Progress)
		assert.Equal(t, "owned update", task.PromptEn)
	}
}

func TestRelayMidjourneyNotifyRejectsEmptyUpstreamID(t *testing.T) {
	db := setupMidjourneyNotifyTestDB(t)
	seedMidjourneyNotifyUser(t, db, 1)
	task := &model.Midjourney{UserId: 1, MjId: "", Status: "SUBMITTED", Progress: "0%"}
	require.NoError(t, db.Create(task).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/mj/notify", bytes.NewBufferString(
		`{"id":"","progress":"100%","status":"SUCCESS"}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)

	response := RelayMidjourneyNotify(c)
	require.NotNil(t, response)
	assert.Equal(t, 4, response.Code)
	assert.Equal(t, "midjourney_task_id_required", response.Description)

	require.NoError(t, db.First(task, task.Id).Error)
	assert.Equal(t, "SUBMITTED", task.Status)
	assert.Equal(t, "0%", task.Progress)
}
