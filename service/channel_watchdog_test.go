package service

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupWatchdogDB(t *testing.T) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Log{}, &model.QuotaData{}))
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	model.DB = database
	model.LOG_DB = database
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		_ = sqlDB.Close()
	})
}

func TestRunChannelHealthWatchdog(t *testing.T) {
	setupWatchdogDB(t)
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	resetWecomBotAlertSlots(t)
	captured := captureWecomBotAlerts(t)
	enableTestWecomBot(t, "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test", "")

	withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
		setting.WatchdogEnabled = true
		setting.WatchdogMinutes = 10
		setting.WatchdogThreshold = 3
	})

	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 1, Name: "正常渠道", Type: 1, Key: "k", Status: common.ChannelStatusEnabled,
	}).Error)
	// 渠道 2 已禁用：即使高频错误也不播报
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: 2, Name: "禁用渠道", Type: 1, Key: "k", Status: common.ChannelStatusAutoDisabled,
	}).Error)
	// 渠道 1 错误 4 次（超阈值 3）；渠道 2 错误 9 次（禁用不播）；渠道 3 错误 2 次（未达阈值）
	for i := range 4 {
		require.NoError(t, model.DB.Create(&model.Log{
			UserId: 1, CreatedAt: now - 60, Type: model.LogTypeError,
			Content: "upstream 500", ChannelId: 1, ModelName: "gpt-4o",
		}).Error)
		_ = i
	}
	for range 9 {
		require.NoError(t, model.DB.Create(&model.Log{
			UserId: 1, CreatedAt: now - 60, Type: model.LogTypeError,
			Content: "disabled channel error", ChannelId: 2, ModelName: "gpt-4o",
		}).Error)
	}
	for range 2 {
		require.NoError(t, model.DB.Create(&model.Log{
			UserId: 1, CreatedAt: now - 60, Type: model.LogTypeError,
			Content: "minor error", ChannelId: 3, ModelName: "gpt-4o",
		}).Error)
	}

	summary, err := RunChannelHealthWatchdog()
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Unhealthy)
	assert.Equal(t, 1, summary.Alerted)
	assert.Equal(t, 0, summary.Suppressed)

	select {
	case alert := <-captured:
		assert.Equal(t, "渠道错误频率异常", alert.title)
		joined := strings.Join(alert.lines, "\n")
		assert.Contains(t, joined, "渠道：正常渠道（#1）")
		assert.Contains(t, joined, "错误 4 次（阈值 3）")
		assert.Contains(t, joined, "upstream 500")
	default:
		t.Fatal("expected watchdog alert")
	}

	// 30 分钟闸门内重复巡检抑制（计数递增）
	summary, err = RunChannelHealthWatchdog()
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Unhealthy)
	assert.Equal(t, 0, summary.Alerted)
	assert.Equal(t, 1, summary.Suppressed)
}
