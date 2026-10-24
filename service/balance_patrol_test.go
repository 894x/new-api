package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBalancePatrolDB(t *testing.T) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}))
	previousDB := model.DB
	model.DB = database
	t.Cleanup(func() {
		model.DB = previousDB
		_ = sqlDB.Close()
	})
}

func withBalancePatrolSetting(t *testing.T, mutate func(*operation_setting.BalancePatrolSetting)) {
	t.Helper()
	setting := operation_setting.GetBalancePatrolSetting()
	original := *setting
	mutate(setting)
	t.Cleanup(func() { *setting = original })
}

func createBalancePatrolChannel(t *testing.T, id int, autoBan int) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Id:      id,
		Type:    1,
		Name:    fmt.Sprintf("OpenAI-%02d", id),
		Status:  common.ChannelStatusEnabled,
		AutoBan: &autoBan,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	return channel
}

func createBalancePatrolUser(t *testing.T, id int, username string, quota int, status int) {
	t.Helper()
	user := &model.User{
		Id:       id,
		Username: username,
		Quota:    quota,
		Status:   status,
		AffCode:  fmt.Sprintf("AFF%03d", id),
	}
	require.NoError(t, model.DB.Create(user).Error)
}

func TestApplyChannelBalanceResultDisablesOnZero(t *testing.T) {
	setupBalancePatrolDB(t)
	withWecomBotMemoryCooldown(t)
	withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
		setting.DisableOnZeroBalance = true
	})

	t.Run("zero balance disables auto-ban channel", func(t *testing.T) {
		channel := createBalancePatrolChannel(t, 101, 1)
		disabled, alerted := ApplyChannelBalanceResult(channel, 0, false)
		assert.True(t, disabled)
		assert.False(t, alerted)
		var reloaded model.Channel
		require.NoError(t, model.DB.First(&reloaded, 101).Error)
		assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	})

	t.Run("auto-ban disabled channel is left enabled", func(t *testing.T) {
		channel := createBalancePatrolChannel(t, 102, 0)
		disabled, alerted := ApplyChannelBalanceResult(channel, 0, false)
		assert.False(t, disabled)
		assert.False(t, alerted)
		var reloaded model.Channel
		require.NoError(t, model.DB.First(&reloaded, 102).Error)
		assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
	})

	t.Run("disable switch off leaves channel enabled", func(t *testing.T) {
		withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
			setting.DisableOnZeroBalance = false
		})
		channel := createBalancePatrolChannel(t, 103, 1)
		disabled, alerted := ApplyChannelBalanceResult(channel, 0, false)
		assert.False(t, disabled)
		assert.False(t, alerted)
		var reloaded model.Channel
		require.NoError(t, model.DB.First(&reloaded, 103).Error)
		assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
	})
}

func TestApplyChannelBalanceResultLowBalanceAlert(t *testing.T) {
	setupBalancePatrolDB(t)
	withWecomBotMemoryCooldown(t)
	captured := captureWecomBotAlerts(t)

	withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
		setting.LowBalanceThreshold = 5
		setting.DisableOnZeroBalance = true
	})
	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Enabled = true
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
	})

	channel := createBalancePatrolChannel(t, 201, 1)
	// 余额 3 落在预警级区间（严重阈值为 5*50%=2.5）
	disabled, alerted := ApplyChannelBalanceResult(channel, 3, true)
	assert.False(t, disabled)
	assert.True(t, alerted)

	alert, ok := waitForWecomBotAlert(t, captured)
	require.True(t, ok)
	assert.Equal(t, "渠道低余额预警", alert.title)
	joined := strings.Join(alert.lines, "\n")
	assert.Contains(t, joined, "渠道：OpenAI-201（#201）")
	assert.Contains(t, joined, "当前余额：$3.00")
	assert.Contains(t, joined, "预警阈值：$5.00")

	t.Run("cooldown suppresses repeated alert", func(t *testing.T) {
		_, alerted := ApplyChannelBalanceResult(channel, 3, true)
		assert.True(t, alerted)
		assertNoWecomBotAlert(t, captured)
	})
}

func TestApplyChannelBalanceResultSkipsAlert(t *testing.T) {
	setupBalancePatrolDB(t)
	withWecomBotMemoryCooldown(t)
	captured := captureWecomBotAlerts(t)

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Enabled = true
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
	})

	t.Run("threshold zero disables alert", func(t *testing.T) {
		withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
			setting.LowBalanceThreshold = 0
		})
		channel := createBalancePatrolChannel(t, 301, 1)
		disabled, alerted := ApplyChannelBalanceResult(channel, 0.5, true)
		assert.False(t, disabled)
		assert.False(t, alerted)
	})

	t.Run("manual refresh keeps legacy behavior", func(t *testing.T) {
		withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
			setting.LowBalanceThreshold = 5
		})
		channel := createBalancePatrolChannel(t, 302, 1)
		disabled, alerted := ApplyChannelBalanceResult(channel, 2, false)
		assert.False(t, disabled)
		assert.False(t, alerted)
	})

	assertNoWecomBotAlert(t, captured)
}

func TestRunUserQuotaPatrolCheck(t *testing.T) {
	setupBalancePatrolDB(t)
	captured := captureWecomBotAlerts(t)

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Enabled = true
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
	})

	previousThreshold := common.QuotaRemindThreshold
	common.QuotaRemindThreshold = 1000
	t.Cleanup(func() { common.QuotaRemindThreshold = previousThreshold })

	createBalancePatrolUser(t, 1, "alice", 100, common.UserStatusEnabled)
	createBalancePatrolUser(t, 2, "bob", 500, common.UserStatusEnabled)
	createBalancePatrolUser(t, 3, "carol", 2000, common.UserStatusEnabled)
	createBalancePatrolUser(t, 4, "dave", 50, common.UserStatusDisabled)

	t.Run("falls back to global threshold and aggregates lowest users", func(t *testing.T) {
		withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
			setting.UserMinQuota = 0
			setting.UserListLimit = 0
		})
		summary, err := RunUserQuotaPatrolCheck(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, summary.Total)
		assert.Equal(t, 2, summary.Listed)

		alert, ok := waitForWecomBotAlert(t, captured)
		require.True(t, ok)
		assert.Equal(t, "用户低余额巡检", alert.title)
		joined := strings.Join(alert.lines, "\n")
		assert.Contains(t, joined, "低余额用户：2 名")
		assert.Contains(t, joined, "alice（剩余")
		assert.Contains(t, joined, "bob（剩余")
		assert.NotContains(t, joined, "carol")
		assert.NotContains(t, joined, "dave")
	})

	t.Run("list limit truncates the aggregated message", func(t *testing.T) {
		withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
			setting.UserMinQuota = 1000
			setting.UserListLimit = 1
		})
		summary, err := RunUserQuotaPatrolCheck(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, summary.Total)
		assert.Equal(t, 1, summary.Listed)

		alert, ok := waitForWecomBotAlert(t, captured)
		require.True(t, ok)
		joined := strings.Join(alert.lines, "\n")
		assert.Contains(t, joined, "alice（剩余")
		assert.NotContains(t, joined, "bob（剩余")
	})

	t.Run("no low-quota users sends no alert", func(t *testing.T) {
		withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
			setting.UserMinQuota = 50
		})
		summary, err := RunUserQuotaPatrolCheck(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, summary.Total)
		assert.Equal(t, 0, summary.Listed)
		assertNoWecomBotAlert(t, captured)
	})

	t.Run("wecom bot disabled still returns summary", func(t *testing.T) {
		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Enabled = false
		})
		withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
			setting.UserMinQuota = 0
			setting.UserListLimit = 0
		})
		summary, err := RunUserQuotaPatrolCheck(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, summary.Total)
		assertNoWecomBotAlert(t, captured)
	})
}

func TestGetBalancePatrolSettingEnvAndNormalization(t *testing.T) {
	t.Setenv("CHANNEL_UPDATE_FREQUENCY", "15")
	setting := operation_setting.GetBalancePatrolSetting()
	assert.True(t, setting.ChannelEnabled)
	assert.Equal(t, float64(15), setting.ChannelMinutes)
	assert.Equal(t, float64(operation_setting.DefaultBalancePatrolMinutes), setting.UserMinutes)
	assert.Equal(t, operation_setting.DefaultLowBalanceThreshold, setting.LowBalanceThreshold)
	assert.True(t, setting.DisableOnZeroBalance)

	t.Setenv("CHANNEL_UPDATE_ENABLED", "false")
	setting = operation_setting.GetBalancePatrolSetting()
	assert.False(t, setting.ChannelEnabled)

	t.Run("normalization", func(t *testing.T) {
		assert.Equal(t, float64(operation_setting.DefaultBalancePatrolMinutes), operation_setting.NormalizeBalancePatrolMinutes(0))
		assert.Equal(t, float64(operation_setting.MaxBalancePatrolMinutes), operation_setting.NormalizeBalancePatrolMinutes(999999))
		assert.Equal(t, float64(45), operation_setting.NormalizeBalancePatrolMinutes(45))
		assert.Equal(t, operation_setting.DefaultBalancePatrolUserListLimit, operation_setting.NormalizeBalancePatrolUserListLimit(0))
		assert.Equal(t, operation_setting.MaxBalancePatrolUserListLimit, operation_setting.NormalizeBalancePatrolUserListLimit(99))
		assert.Equal(t, 30, operation_setting.NormalizeBalancePatrolUserListLimit(30))
	})

	t.Run("validators", func(t *testing.T) {
		require.NoError(t, operation_setting.ValidateBalancePatrolMinutes("30"))
		require.Error(t, operation_setting.ValidateBalancePatrolMinutes("0"))
		require.Error(t, operation_setting.ValidateBalancePatrolMinutes("10081"))
		require.Error(t, operation_setting.ValidateBalancePatrolMinutes("abc"))
		require.NoError(t, operation_setting.ValidateLowBalanceThreshold("1.5"))
		require.Error(t, operation_setting.ValidateLowBalanceThreshold("-1"))
		require.NoError(t, operation_setting.ValidateBalancePatrolUserMinQuota("1000"))
		require.Error(t, operation_setting.ValidateBalancePatrolUserMinQuota("-1"))
		require.NoError(t, operation_setting.ValidateBalancePatrolUserListLimit("0"))
		require.Error(t, operation_setting.ValidateBalancePatrolUserListLimit("51"))
	})
}

func TestApplyChannelBalanceResultTieredAndReset(t *testing.T) {
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	resetWecomBotAlertSlots(t)
	captured := captureWecomBotAlerts(t)
	enableTestWecomBot(t, "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test", "")

	channel := &model.Channel{Id: 9, Name: "分级渠道", Status: common.ChannelStatusEnabled}
	withBalancePatrolSetting(t, func(setting *operation_setting.BalancePatrolSetting) {
		setting.LowBalanceThreshold = 10
		setting.CriticalBalanceThreshold = 0 // 跟随预警阈值 50% => 5
	})

	// 预警级：余额 7 落在 [5,10)
	_, alerted := ApplyChannelBalanceResult(channel, 7, true)
	require.True(t, alerted)
	select {
	case alert := <-captured:
		assert.Equal(t, "渠道低余额预警", alert.title)
	default:
		t.Fatal("expected warning alert")
	}
	// 同级抑制
	_, alerted = ApplyChannelBalanceResult(channel, 7, true)
	require.True(t, alerted)
	select {
	case <-captured:
		t.Fatal("warning alert should be suppressed within TTL")
	default:
	}

	// 严重级：余额 3 < 5，独立闸门立即放行
	_, alerted = ApplyChannelBalanceResult(channel, 3, true)
	require.True(t, alerted)
	select {
	case alert := <-captured:
		assert.Equal(t, "渠道余额严重不足", alert.title)
	default:
		t.Fatal("expected critical alert")
	}

	// 回升到阈值之上：闸门被重置，再次跌破立即提醒
	ApplyChannelBalanceResult(channel, 20, true)
	select {
	case <-captured:
		t.Fatal("balance above threshold should not alert")
	default:
	}
	_, alerted = ApplyChannelBalanceResult(channel, 7, true)
	require.True(t, alerted)
	select {
	case alert := <-captured:
		assert.Equal(t, "渠道低余额预警", alert.title)
	default:
		t.Fatal("expected immediate alert after recharge reset")
	}
}
