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
)

// ApplyChannelBalanceResult 处理一次渠道余额查询结果：余额用尽且开启自动禁用时走既有
// DisableChannel 路径（含企微禁用告警）；否则低于预警阈值时推送企微低余额预警。
// 返回是否禁用、是否推送了预警。
func ApplyChannelBalanceResult(channel *model.Channel, balance float64, alert bool, cooldown time.Duration) (disabled bool, alerted bool) {
	setting := operation_setting.GetBalancePatrolSetting()
	if balance <= 0 && setting.DisableOnZeroBalance && channel.GetAutoBan() {
		channelError := *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", channel.GetAutoBan())
		DisableChannel(channelError, "余额不足")
		return true, false
	}
	if !alert {
		return false, false
	}
	if setting.LowBalanceThreshold > 0 && balance < setting.LowBalanceThreshold {
		NotifyWecomBotChannelLowBalance(channel.Name, channel.Id, balance, setting.LowBalanceThreshold, cooldown)
		return false, true
	}
	return false, false
}

// NotifyWecomBotChannelLowBalance 推送渠道低余额预警；同一渠道在冷却窗口内只提醒一次。
func NotifyWecomBotChannelLowBalance(channelName string, channelId int, balance float64, threshold float64, cooldown time.Duration) {
	wecomSetting := operation_setting.GetWecomBotSetting()
	if !wecomSetting.Enabled || !wecomSetting.HasEnabledBot() {
		return
	}
	slotKey := fmt.Sprintf("wecom_bot_alert:balance:%d", channelId)
	if !acquireWecomBotAlertSlot(slotKey, cooldown) {
		return
	}
	lines := []string{
		fmt.Sprintf("渠道：%s（#%d）", channelName, channelId),
		fmt.Sprintf("当前余额：$%.2f", balance),
		fmt.Sprintf("预警阈值：$%.2f", threshold),
		fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")),
	}
	enqueueWecomBotAlert(wecomBotAlert{title: "渠道低余额预警", color: "warning", lines: lines})
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

	var total int64
	if err := model.DB.Model(&model.User{}).
		Where("status = ? AND quota < ?", common.UserStatusEnabled, threshold).
		Count(&total).Error; err != nil {
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
	if err := model.DB.Model(&model.User{}).
		Select("username", "quota").
		Where("status = ? AND quota < ?", common.UserStatusEnabled, threshold).
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
		enqueueWecomBotAlert(wecomBotAlert{title: "用户低余额巡检", color: "warning", lines: lines})
	}
	return summary, nil
}
