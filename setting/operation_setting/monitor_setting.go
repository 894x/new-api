package operation_setting

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled               bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes               float64 `json:"auto_test_channel_minutes"`
	ChannelTestMode                      string  `json:"channel_test_mode"`
	ChannelTestConcurrency               int     `json:"channel_test_concurrency"`
	ChannelMonitoringEnabled             bool    `json:"channel_monitoring_enabled"`
	ChannelMonitoringMinutes             int     `json:"channel_monitoring_minutes"`
	ChannelMonitoringTimeoutSeconds      int     `json:"channel_monitoring_timeout_seconds"`
	ChannelMonitoringPrompt              string  `json:"channel_monitoring_prompt"`
	ChannelMonitoringMaxTokens           int     `json:"channel_monitoring_max_tokens"`
	ChannelMonitoringStream              bool    `json:"channel_monitoring_stream"`
	ChannelMonitoringSlowMs              int     `json:"channel_monitoring_slow_ms"` // Legacy saved-policy option; colors use fixed 5 s / 10 s boundaries.
	ChannelMonitoringExcludedStatusCodes string  `json:"channel_monitoring_excluded_status_codes"`
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModeAutoBanOnly     = "auto_ban_only"
	ChannelTestModePassiveRecovery = "passive_recovery"

	ChannelTestConcurrencyOptionKey = "monitor_setting.channel_test_concurrency"
	DefaultChannelTestConcurrency   = 1
	MaxChannelTestConcurrency       = 32
)

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:               false,
	AutoTestChannelMinutes:               10,
	ChannelTestMode:                      ChannelTestModeScheduledAll,
	ChannelTestConcurrency:               DefaultChannelTestConcurrency,
	ChannelMonitoringEnabled:             true,
	ChannelMonitoringMinutes:             5,
	ChannelMonitoringTimeoutSeconds:      30,
	ChannelMonitoringPrompt:              "Reply with OK.",
	ChannelMonitoringMaxTokens:           128,
	ChannelMonitoringStream:              true,
	ChannelMonitoringSlowMs:              3000,
	ChannelMonitoringExcludedStatusCodes: "400",
}

func ValidateChannelMonitoringOptions(options map[string]string) error {
	for key, limits := range map[string][2]int{
		"channel_monitoring_minutes":         {1, 1440},
		"channel_monitoring_timeout_seconds": {1, 300},
		"channel_monitoring_max_tokens":      {1, 512},
		"channel_monitoring_slow_ms":         {1, 300000},
	} {
		value, err := strconv.Atoi(options["monitor_setting."+key])
		if err != nil || value < limits[0] || value > limits[1] {
			return fmt.Errorf("%s must be between %d and %d", key, limits[0], limits[1])
		}
	}
	for _, key := range []string{"channel_monitoring_enabled", "channel_monitoring_stream"} {
		if _, err := strconv.ParseBool(options["monitor_setting."+key]); err != nil {
			return fmt.Errorf("invalid monitoring boolean: %s", key)
		}
	}
	prompt := options["monitor_setting.channel_monitoring_prompt"]
	if len(strings.TrimSpace(prompt)) == 0 || len(prompt) > 4096 {
		return fmt.Errorf("monitoring prompt must contain 1 to 4096 bytes")
	}
	excluded := options["monitor_setting.channel_monitoring_excluded_status_codes"]
	if len(excluded) > 1024 {
		return fmt.Errorf("monitoring status code rules must not exceed 1024 bytes")
	}
	if _, err := ParseHTTPStatusCodeRanges(excluded); err != nil {
		return err
	}
	return nil
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
			monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			monitorSetting.AutoTestChannelEnabled = parsed
		}
	}
	switch monitorSetting.ChannelTestMode {
	case ChannelTestModeAutoBanOnly, ChannelTestModePassiveRecovery:
	default:
		monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
	}
	monitorSetting.ChannelTestConcurrency = NormalizeChannelTestConcurrency(monitorSetting.ChannelTestConcurrency)
	return &monitorSetting
}

func NormalizeChannelTestConcurrency(concurrency int) int {
	if concurrency < 1 {
		return DefaultChannelTestConcurrency
	}
	if concurrency > MaxChannelTestConcurrency {
		return MaxChannelTestConcurrency
	}
	return concurrency
}

func ValidateChannelTestConcurrency(value string) error {
	concurrency, err := strconv.Atoi(value)
	if err != nil || concurrency < 1 || concurrency > MaxChannelTestConcurrency {
		return fmt.Errorf("channel test concurrency must be between 1 and %d", MaxChannelTestConcurrency)
	}
	return nil
}
