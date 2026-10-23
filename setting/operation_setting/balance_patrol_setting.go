package operation_setting

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// BalancePatrolSetting 定时余额巡检配置：渠道侧巡检上游余额并在低余额时预警，
// 用户侧巡检剩余额度并聚合预警。LowBalanceThreshold 为 0 时关闭渠道低余额预警；
// UserMinQuota 为 0 时跟随全局 QuotaRemindThreshold。
type BalancePatrolSetting struct {
	ChannelEnabled       bool    `json:"channel_enabled"`
	ChannelMinutes       float64 `json:"channel_minutes"`
	LowBalanceThreshold  float64 `json:"low_balance_threshold"`
	DisableOnZeroBalance bool    `json:"disable_on_zero_balance"`
	UserEnabled          bool    `json:"user_enabled"`
	UserMinutes          float64 `json:"user_minutes"`
	UserMinQuota         int     `json:"user_min_quota"`
	UserListLimit        int     `json:"user_list_limit"`
}

const (
	DefaultBalancePatrolMinutes       = 30
	MinBalancePatrolMinutes           = 1
	MaxBalancePatrolMinutes           = 10080
	DefaultLowBalanceThreshold        = 1.0
	MaxBalancePatrolUserListLimit     = 50
	DefaultBalancePatrolUserListLimit = 20
)

var balancePatrolSetting = BalancePatrolSetting{
	ChannelMinutes:       DefaultBalancePatrolMinutes,
	LowBalanceThreshold:  DefaultLowBalanceThreshold,
	DisableOnZeroBalance: true,
	UserMinutes:          DefaultBalancePatrolMinutes,
	UserListLimit:        DefaultBalancePatrolUserListLimit,
}

func init() {
	config.GlobalConfig.Register("balance_patrol_setting", &balancePatrolSetting)
}

func GetBalancePatrolSetting() *BalancePatrolSetting {
	// 旧版 CHANNEL_UPDATE_FREQUENCY 环境变量（单位分钟）兼容：由系统任务接管巡检
	if os.Getenv("CHANNEL_UPDATE_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_UPDATE_FREQUENCY"))
		if err == nil && frequency > 0 {
			balancePatrolSetting.ChannelEnabled = true
			balancePatrolSetting.ChannelMinutes = float64(frequency)
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_UPDATE_ENABLED"); ok {
		if parsed, err := strconv.ParseBool(enabled); err == nil {
			balancePatrolSetting.ChannelEnabled = parsed
		}
	}
	balancePatrolSetting.ChannelMinutes = NormalizeBalancePatrolMinutes(balancePatrolSetting.ChannelMinutes)
	balancePatrolSetting.UserMinutes = NormalizeBalancePatrolMinutes(balancePatrolSetting.UserMinutes)
	if balancePatrolSetting.LowBalanceThreshold < 0 {
		balancePatrolSetting.LowBalanceThreshold = 0
	}
	if balancePatrolSetting.UserMinQuota < 0 {
		balancePatrolSetting.UserMinQuota = 0
	}
	balancePatrolSetting.UserListLimit = NormalizeBalancePatrolUserListLimit(balancePatrolSetting.UserListLimit)
	return &balancePatrolSetting
}

func NormalizeBalancePatrolMinutes(minutes float64) float64 {
	if minutes <= 0 {
		return DefaultBalancePatrolMinutes
	}
	if minutes > MaxBalancePatrolMinutes {
		return MaxBalancePatrolMinutes
	}
	return minutes
}

func NormalizeBalancePatrolUserListLimit(limit int) int {
	if limit <= 0 {
		return DefaultBalancePatrolUserListLimit
	}
	if limit > MaxBalancePatrolUserListLimit {
		return MaxBalancePatrolUserListLimit
	}
	return limit
}

func ValidateBalancePatrolMinutes(value string) error {
	minutes, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || minutes < MinBalancePatrolMinutes || minutes > MaxBalancePatrolMinutes {
		return fmt.Errorf("balance patrol minutes must be between %d and %d", MinBalancePatrolMinutes, MaxBalancePatrolMinutes)
	}
	return nil
}

func ValidateLowBalanceThreshold(value string) error {
	threshold, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || threshold < 0 || threshold > 1e6 {
		return fmt.Errorf("low balance threshold must be a non-negative number")
	}
	return nil
}

func ValidateBalancePatrolUserMinQuota(value string) error {
	quota, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || quota < 0 {
		return fmt.Errorf("user min quota must be a non-negative integer")
	}
	return nil
}

func ValidateBalancePatrolUserListLimit(value string) error {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit < 0 || limit > MaxBalancePatrolUserListLimit {
		return fmt.Errorf("user list limit must be between 0 and %d", MaxBalancePatrolUserListLimit)
	}
	return nil
}
