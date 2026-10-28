package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

// wecomBotLowBalanceTTL 低余额预警按渠道 24 小时最多提醒一次（对齐 LiteLLM 预算
// 告警的 budget_alert_ttl 模式），避免每次巡检重复打扰。严重不足级别 6 小时一次。
const (
	wecomBotLowBalanceTTL          = 24 * time.Hour
	wecomBotCriticalBalanceTTL     = 6 * time.Hour
	wecomBotCriticalThresholdRatio = 0.5 // 未显式配置严重阈值时取预警阈值的 50%
)

// resetWecomBotAlertGates 余额回升后清掉该渠道的告警闸门，下次再跌破立即提醒。
func resetWecomBotBalanceGates(channelId int) {
	if common.RedisEnabled {
		for _, key := range []string{
			fmt.Sprintf("%sbalance:%d", wecomBotGatePrefix, channelId),
			fmt.Sprintf("%sbalance-critical:%d", wecomBotGatePrefix, channelId),
		} {
			_ = common.RDB.Del(context.Background(), key).Err()
		}
	}
	// 内存闸门以原始 slotKey 为键（wecom_bot_alert: 前缀，无 gate: 段）
	for _, key := range []string{
		fmt.Sprintf("wecom_bot_alert:balance:%d", channelId),
		fmt.Sprintf("wecom_bot_alert:balance-critical:%d", channelId),
	} {
		wecomBotAlertGates.Delete(key)
	}
}

// ApplyChannelBalanceResult 处理一次渠道余额查询结果：余额用尽且开启自动禁用时走既有
// DisableChannel 路径（含企微禁用告警）；否则分级推送低余额预警（预警/严重两级），
// 余额回升到阈值之上时重置闸门。返回是否禁用、是否推送了预警。
func ApplyChannelBalanceResult(channel *model.Channel, balance float64, alert bool) (disabled bool, alerted bool) {
	setting := operation_setting.GetBalancePatrolSetting()
	if balance <= 0 && setting.DisableOnZeroBalance && channel.GetAutoBan() {
		channelError := *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan())
		DisableChannel(channelError, "余额不足")
		return true, false
	}
	if !alert || setting.LowBalanceThreshold <= 0 {
		return false, false
	}
	if balance >= setting.LowBalanceThreshold {
		// 回升解除抑制：下次跌破立即再提醒
		resetWecomBotBalanceGates(channel.Id)
		return false, false
	}
	criticalThreshold := setting.CriticalBalanceThreshold
	if criticalThreshold <= 0 {
		criticalThreshold = setting.LowBalanceThreshold * wecomBotCriticalThresholdRatio
	}
	if balance < criticalThreshold {
		NotifyWecomBotChannelLowBalance(channel.Name, channel.Id, balance, setting.LowBalanceThreshold, true)
		return false, true
	}
	NotifyWecomBotChannelLowBalance(channel.Name, channel.Id, balance, setting.LowBalanceThreshold, false)
	return false, true
}

// NotifyWecomBotChannelLowBalance 推送渠道低余额分级预警；预警级 24 小时一次、
// 严重级 6 小时一次，持续低余额时在下一次放行的消息里携带窗口内累计次数。
func NotifyWecomBotChannelLowBalance(channelName string, channelId int, balance float64, threshold float64, critical bool) {
	wecomSetting := operation_setting.GetWecomBotSetting()
	if !wecomSetting.Enabled || !wecomSetting.HasEnabledBot() {
		return
	}
	slotKey := fmt.Sprintf("wecom_bot_alert:balance:%d", channelId)
	ttl := wecomBotLowBalanceTTL
	title := "渠道低余额预警"
	if critical {
		slotKey = fmt.Sprintf("wecom_bot_alert:balance-critical:%d", channelId)
		ttl = wecomBotCriticalBalanceTTL
		title = "渠道余额严重不足"
	}
	admitted, suppressed := admitWecomBotAlert(slotKey, ttl)
	if !admitted {
		return
	}
	lines := []string{
		fmt.Sprintf("渠道：%s（#%d）", channelName, channelId),
		fmt.Sprintf("当前余额：$%.2f", balance),
		fmt.Sprintf("预警阈值：$%.2f", threshold),
	}
	if critical {
		lines = append(lines, fmt.Sprintf("级别：严重（低于预警阈值的 50%%）"))
	}
	if forecast := channelBalanceExhaustionDays(channelId, balance); forecast > 0 {
		lines = append(lines, fmt.Sprintf("按近 7 天消费速度预计约 %d 天耗尽", forecast))
	}
	if suppressed > 0 {
		window := "24 小时"
		if critical {
			window = "6 小时"
		}
		lines = append(lines, fmt.Sprintf("持续低余额：%s内已抑制 %d 次提醒", window, suppressed))
	}
	lines = append(lines, fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")))
	enqueueWecomBotAlert(wecomBotAlert{
		title:    title,
		color:    "warning",
		lines:    lines,
		category: operation_setting.WecomBotCategoryBalancePatrol,
		scope:    fmt.Sprintf("channel:%d", channelId),
	})
}

// channelBalanceExhaustionDays 依据数据看板近 7 天的渠道消费预估余额耗尽天数；
// 无消费数据时返回 0（不展示）。
func channelBalanceExhaustionDays(channelId int, balanceUSD float64) int {
	spend, err := model.GetChannelQuotaSince(channelId, time.Now().Add(-7*24*time.Hour).Unix())
	if err != nil || spend <= 0 {
		return 0
	}
	dailyUSD := float64(spend) / common.QuotaPerUnit / 7
	if dailyUSD <= 0 {
		return 0
	}
	days := int(balanceUSD / dailyUSD)
	if days < 1 {
		return 1
	}
	return days
}

type UserQuotaPatrolSummary struct {
	Total  int `json:"total"`
	Listed int `json:"listed"`
}

// RunUserQuotaPatrolCheck 巡检 enabled 用户的剩余额度，低于阈值时向企微机器人推送
// 一条聚合预警（管理员视角，不重复通知用户本人）。摘要持久化到任务 Result。
func RunUserQuotaPatrolCheck(ctx context.Context) (*UserQuotaPatrolSummary, error) {
	patrolSetting := operation_setting.GetBalancePatrolSetting()
	threshold := patrolSetting.UserMinQuota
	if threshold <= 0 {
		threshold = common.QuotaRemindThreshold
	}
	summary := &UserQuotaPatrolSummary{}

	baseQuery := model.DB.Model(&model.User{}).
		Where("status = ? AND quota < ?", common.UserStatusEnabled, threshold)
	if monitorNames := operation_setting.ParseUserMonitorList(patrolSetting.UserMonitorList); len(monitorNames) > 0 {
		baseQuery = baseQuery.Where("username IN ?", monitorNames)
	}
	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, err
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	summary.Total = int(total)
	if summary.Total == 0 {
		return summary, nil
	}

	listLimit := operation_setting.NormalizeBalancePatrolUserListLimit(patrolSetting.UserListLimit)
	type lowQuotaUser struct {
		Username string
		Quota    int
	}
	var users []lowQuotaUser
	if err := baseQuery.Session(&gorm.Session{}).
		Select("username", "quota").
		Order("quota asc").
		Limit(listLimit).
		Find(&users).Error; err != nil {
		return nil, err
	}
	summary.Listed = len(users)

	wecomSetting := operation_setting.GetWecomBotSetting()
	if wecomSetting.Enabled && wecomSetting.HasEnabledBot() {
		var lowest strings.Builder
		for i, user := range users {
			if i > 0 {
				lowest.WriteString("，")
			}
			fmt.Fprintf(&lowest, "%s（剩余 %s）", user.Username, logger.FormatQuota(user.Quota))
		}
		lines := []string{
			fmt.Sprintf("阈值：低于 %s", logger.FormatQuota(threshold)),
			fmt.Sprintf("低余额用户：%d 名", summary.Total),
			fmt.Sprintf("余额最低：%s", lowest.String()),
			fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")),
		}
		enqueueWecomBotAlert(wecomBotAlert{
			title:    "用户低余额巡检",
			color:    "warning",
			lines:    lines,
			category: operation_setting.WecomBotCategoryBalancePatrol,
		})
	}
	return summary, nil
}
