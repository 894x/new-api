package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 渠道健康看门狗（对标 LiteLLM outage_alerts / llm_exceptions）：按窗口聚合各
// 启用渠道的错误日志，超过阈值时推送聚合告警（带抑制计数），让「用户先发现
// 挂了」变成「管理员先收到通知」。与逐条错误告警互补：后者按「渠道+状态码」
// 冷却，这里提供跨状态码的窗口级视图。

const channelWatchdogAlertTTL = 30 * time.Minute

// ChannelWatchdogSummary 一次看门狗巡检的结果摘要。
type ChannelWatchdogSummary struct {
	Window     int `json:"window_minutes"`
	Threshold  int `json:"threshold"`
	Unhealthy  int `json:"unhealthy"`
	Alerted    int `json:"alerted"`
	Suppressed int `json:"suppressed"`
}

// RunChannelHealthWatchdog 统计最近窗口内各启用渠道的错误日志数，超过阈值的
// 渠道推送聚合告警；每渠道 30 分钟内只提醒一次（抑制计数合并）。
func RunChannelHealthWatchdog() (*ChannelWatchdogSummary, error) {
	setting := operation_setting.GetBalancePatrolSetting()
	wecomSetting := operation_setting.GetWecomBotSetting()
	window := time.Duration(setting.WatchdogMinutes) * time.Minute
	summary := &ChannelWatchdogSummary{
		Window:    setting.WatchdogMinutes,
		Threshold: setting.WatchdogThreshold,
	}
	if !setting.WatchdogEnabled || !wecomSetting.Enabled || !wecomSetting.HasEnabledBot() {
		return summary, nil
	}

	counts, err := model.GetChannelErrorCounts(time.Now().Add(-window).Unix())
	if err != nil {
		return nil, err
	}
	if len(counts) == 0 {
		return summary, nil
	}

	// 只取启用渠道；GetAllChannels 的 num=0 会生成 LIMIT 0，这里直接查询
	var enabledChannels []*model.Channel
	if err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Find(&enabledChannels).Error; err != nil {
		return nil, err
	}
	channelNames := make(map[int]string, len(enabledChannels))
	for _, channel := range enabledChannels {
		channelNames[channel.Id] = channel.Name
	}

	for _, entry := range counts {
		if entry.Count < int64(setting.WatchdogThreshold) {
			continue
		}
		name, enabled := channelNames[entry.ChannelId]
		if !enabled {
			continue
		}
		summary.Unhealthy++
		slotKey := fmt.Sprintf("wecom_bot_alert:watchdog:%d", entry.ChannelId)
		admitted, suppressed := admitWecomBotAlert(slotKey, channelWatchdogAlertTTL)
		if !admitted {
			summary.Suppressed++
			continue
		}
		notifyWecomBotChannelUnhealthy(entry.ChannelId, name, entry.Count, window, suppressed, setting.WatchdogThreshold)
		summary.Alerted++
	}
	return summary, nil
}

// notifyWecomBotChannelUnhealthy 推送渠道不健康聚合告警，附最近错误摘要。
func notifyWecomBotChannelUnhealthy(channelId int, channelName string, count int64, window time.Duration, suppressed int, threshold int) {
	logs, _, err := model.GetAllLogs(model.LogTypeError, time.Now().Add(-window).Unix(), time.Now().Unix(), "", "", "", 0, 3, channelId, "", "", "")
	lines := []string{
		fmt.Sprintf("渠道：%s（#%d）", channelName, channelId),
		fmt.Sprintf("窗口：%s 内错误 %d 次（阈值 %d）", window, count, threshold),
	}
	if err == nil {
		for _, entry := range logs {
			lines = append(lines, fmt.Sprintf("最近错误：%s", truncateWecomBotMessage(entry.Content)))
		}
	}
	if suppressed > 0 {
		lines = append(lines, fmt.Sprintf("上轮抑制窗口内累计：%d 次", suppressed))
	}
	lines = append(lines, fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")))
	enqueueWecomBotAlert(wecomBotAlert{title: "渠道错误频率异常", color: "warning", lines: lines})
}
