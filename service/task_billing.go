package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo, decisions ...GroupModelDiscountDecision) {
	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	// 支持任务仅按次计费
	if common.StringsContains(constant.TaskPricePatches, info.OriginModelName) {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if otherRatios := info.PriceData.OtherRatios(); len(otherRatios) > 0 {
			var contents []string
			for key, ra := range otherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	other["is_task"] = true
	other["request_path"] = c.Request.URL.Path
	other["model_price"] = info.PriceData.ModelPrice
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	if len(decisions) > 0 {
		InjectGroupModelDiscountInfo(other, decisions[0])
	}
	attachQuotaSaturation(c, info, other)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     info.UsingGroup,
		Other:     other,
	})
	if len(decisions) == 0 || !decisions[0].Applied {
		model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
		model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
	}
}

// ---------------------------------------------------------------------------
// 异步任务计费辅助函数
// ---------------------------------------------------------------------------

// resolveTokenKey 通过 TokenId 运行时获取令牌 Key（用于 Redis 缓存操作）。
// 如果令牌已被删除或查询失败，返回空字符串。
func resolveTokenKey(ctx context.Context, tokenId int, taskID string) string {
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("获取令牌 key 失败 (tokenId=%d, task=%s): %s", tokenId, taskID, err.Error()))
		return ""
	}
	return token.Key
}

// taskIsSubscription 判断任务是否通过订阅计费。
func taskIsSubscription(task *model.Task) bool {
	return task.PrivateData.BillingSource == BillingSourceSubscription && task.PrivateData.SubscriptionId > 0
}

// taskAdjustFunding 调整任务的资金来源（钱包或订阅），delta > 0 表示扣费，delta < 0 表示退还。
func taskAdjustFunding(task *model.Task, delta int) error {
	if taskIsSubscription(task) {
		return model.PostConsumeUserSubscriptionDelta(task.PrivateData.SubscriptionId, int64(delta))
	}
	if delta > 0 {
		return model.DecreaseUserQuota(task.UserId, delta, false)
	}
	return model.IncreaseUserQuota(task.UserId, -delta, false)
}

// taskAdjustTokenQuota 调整任务的令牌额度，delta > 0 表示扣费，delta < 0 表示退还。
// 需要通过 resolveTokenKey 运行时获取 key（不从 PrivateData 中读取）。
func taskAdjustTokenQuota(ctx context.Context, task *model.Task, delta int) error {
	if task.PrivateData.TokenId <= 0 || delta == 0 {
		return nil
	}
	tokenKey := resolveTokenKey(ctx, task.PrivateData.TokenId, task.TaskID)
	if tokenKey == "" {
		return fmt.Errorf("resolve token key for task %s", task.TaskID)
	}
	var err error
	if delta > 0 {
		err = model.DecreaseTokenQuota(task.PrivateData.TokenId, tokenKey, delta)
	} else {
		err = model.IncreaseTokenQuota(task.PrivateData.TokenId, tokenKey, -delta)
	}
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("调整令牌额度失败 (delta=%d, task=%s): %s", delta, task.TaskID, err.Error()))
		return err
	}
	return nil
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if bc := task.PrivateData.BillingContext; bc != nil {
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
		if priceData := taskBillingContextPriceData(bc); priceData != nil {
			for k, v := range priceData.OtherRatios() {
				other[k] = v
			}
		}
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

func taskBillingContextPriceData(bc *model.TaskBillingContext) *types.PriceData {
	if bc == nil || len(bc.OtherRatios) == 0 {
		return nil
	}
	priceData := &types.PriceData{}
	if !priceData.ReplaceOtherRatios(bc.OtherRatios) {
		return nil
	}
	return priceData
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

func taskNeedsBillingRefund(task *model.Task) bool {
	if task == nil {
		return false
	}
	recoverTaskInitialGroupModelSettlement(task)
	if task.Quota != 0 {
		return true
	}
	billingContext := task.PrivateData.BillingContext
	if billingContext == nil || billingContext.DiscountSettlementID == "" ||
		billingContext.RefundState == model.TaskRefundStateCommitted {
		return false
	}
	return billingContext.ChargeState == "" || billingContext.ChargeState == model.TaskChargeStateCharged
}

func recoverTaskInitialGroupModelSettlement(task *model.Task) bool {
	return task != nil && task.RecoverSettledInitialBilling()
}

// RefundTaskQuota 统一的任务失败退款逻辑。
// 当异步任务失败时，退还资金与令牌额度，并回减用户和渠道用量。
// 返回资金来源是否已成功退还；失败时保留 quota，供显式重试或人工对账。
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) bool {
	if task == nil {
		return false
	}
	recoverTaskInitialGroupModelSettlement(task)
	billingContext := task.PrivateData.BillingContext
	if billingContext != nil && billingContext.ChargeState != "" && billingContext.ChargeState != model.TaskChargeStateCharged {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费状态为 %s，跳过自动退款并等待对账", task.TaskID, billingContext.ChargeState))
		return false
	}
	quota := task.Quota
	if quota < 0 {
		logger.LogError(ctx, fmt.Sprintf("任务 %s 的退款额度不能为负数: %d", task.TaskID, quota))
		return false
	}

	if billingContext == nil {
		if quota == 0 {
			return true
		}
		if err := taskAdjustFunding(task, -quota); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败 task %s: %s", task.TaskID, err.Error()))
			return false
		}
		if err := taskAdjustTokenQuota(ctx, task, -quota); err != nil {
			logger.LogError(ctx, fmt.Sprintf("任务资金已退但令牌退款失败，需人工对账 task %s: %s", task.TaskID, err.Error()))
			return false
		}
		// Legacy rows have no ledger/accounting idempotency evidence. Preserve
		// their historical best-effort accounting behavior so an unavailable
		// stats target cannot leave quota intact and replay the funding refund.
		model.UpdateUserUsedQuota(task.UserId, -quota)
		model.UpdateChannelUsedQuota(task.ChannelId, -quota)
		task.Quota = 0
		if err := task.UpdateQuota(); err != nil {
			logger.LogError(ctx, fmt.Sprintf("退款成功但清除 task quota 失败 task %s: %s", task.TaskID, err.Error()))
			return false
		}
	} else {
		if billingContext.RefundState == model.TaskRefundStateCommitted {
			return true
		}
		if billingContext.RefundedQuota > 0 {
			quota = billingContext.RefundedQuota
			if billingContext.RefundState == "" {
				// Compatibility with rows written before the staged refund state was
				// introduced: RefundedQuota was persisted only after funding, token,
				// and usage had already been returned.
				billingContext.RefundState = model.TaskRefundStateAccountingApplied
			}
		}
		discountSettlementID := billingContext.DiscountSettlementID
		if quota == 0 && billingContext.RefundState == "" {
			if billingContext.DiscountSettlementID == "" {
				return true
			}
			if err := model.BeginGroupModelDiscountSettlementReverse(discountSettlementID); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("零额度任务退款前门禁失败 task %s: %s", task.TaskID, err.Error()))
				return false
			}
			billingContext.RefundState = model.TaskRefundStateAccountingPending
			if err := task.UpdateBillingState(); err != nil {
				billingContext.RefundState = ""
				if cancelErr := model.CancelGroupModelDiscountSettlementReverse(discountSettlementID); cancelErr != nil {
					logger.LogWarn(ctx, fmt.Sprintf("取消零额度任务退款门禁失败 task %s: %s", task.TaskID, cancelErr.Error()))
				}
				logger.LogError(ctx, fmt.Sprintf("持久化零额度任务账本退款状态失败 task %s: %s", task.TaskID, err.Error()))
				return false
			}
		}

		switch billingContext.RefundState {
		case model.TaskRefundStateFundingPending, model.TaskRefundStateTokenPending:
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 退款停留在模糊阶段 %s，禁止自动重放资金并等待对账", task.TaskID, billingContext.RefundState))
			return false
		case model.TaskRefundStateAccountingPending:
			if discountSettlementID == "" || !completeTaskDiscountRefundAccounting(task, quota) {
				logger.LogWarn(ctx, fmt.Sprintf("任务 %s 退款统计阶段缺少可确认的账本证据", task.TaskID))
				return false
			}
		case "":
			if discountSettlementID != "" {
				if err := model.BeginGroupModelDiscountSettlementReverse(discountSettlementID); err != nil {
					logger.LogWarn(ctx, fmt.Sprintf("任务退款前门禁失败 task %s: %s", task.TaskID, err.Error()))
					return false
				}
			}
			billingContext.RefundState = model.TaskRefundStateFundingPending
			if err := task.UpdateBillingState(); err != nil {
				billingContext.RefundState = ""
				if discountSettlementID != "" {
					if cancelErr := model.CancelGroupModelDiscountSettlementReverse(discountSettlementID); cancelErr != nil {
						logger.LogWarn(ctx, fmt.Sprintf("取消未出资任务退款门禁失败 task %s: %s", task.TaskID, cancelErr.Error()))
					}
				}
				logger.LogError(ctx, fmt.Sprintf("持久化任务退款资金 pending 失败 task %s: %s", task.TaskID, err.Error()))
				return false
			}
			if err := taskAdjustFunding(task, -quota); err != nil {
				if discountSettlementID != "" {
					if markErr := model.MarkGroupModelDiscountSettlementReverseFundingUnknown(discountSettlementID); markErr != nil {
						logger.LogWarn(ctx, fmt.Sprintf("标记任务退款资金结果未知失败 task %s: %s", task.TaskID, markErr.Error()))
					}
				}
				logger.LogWarn(ctx, fmt.Sprintf("退还资金来源失败并保持 pending 等待对账 task %s: %s", task.TaskID, err.Error()))
				return false
			}
			billingContext.RefundedQuota = quota
			billingContext.RefundState = model.TaskRefundStateFundingApplied
			if err := task.UpdateBillingState(); err != nil {
				billingContext.RefundState = model.TaskRefundStateFundingPending
				logger.LogError(ctx, fmt.Sprintf("任务资金已退但持久化 applied 失败，等待人工对账 task %s: %s", task.TaskID, err.Error()))
				return false
			}
		}

		if billingContext.RefundState == model.TaskRefundStateFundingApplied {
			billingContext.RefundState = model.TaskRefundStateTokenPending
			if err := task.UpdateBillingState(); err != nil {
				billingContext.RefundState = model.TaskRefundStateFundingApplied
				logger.LogError(ctx, fmt.Sprintf("持久化任务令牌退款 pending 失败 task %s: %s", task.TaskID, err.Error()))
				return false
			}
			if err := taskAdjustTokenQuota(ctx, task, -quota); err != nil {
				logger.LogError(ctx, fmt.Sprintf("任务资金已退但令牌退款失败，保持 pending 等待对账 task %s: %s", task.TaskID, err.Error()))
				return false
			}
			billingContext.RefundState = model.TaskRefundStateTokenApplied
			if err := task.UpdateBillingState(); err != nil {
				billingContext.RefundState = model.TaskRefundStateTokenPending
				logger.LogError(ctx, fmt.Sprintf("任务令牌已退但持久化 applied 失败，等待人工对账 task %s: %s", task.TaskID, err.Error()))
				return false
			}
		}

		if billingContext.RefundState == model.TaskRefundStateTokenApplied {
			billingContext.RefundState = model.TaskRefundStateAccountingPending
			if err := task.UpdateBillingState(); err != nil {
				billingContext.RefundState = model.TaskRefundStateTokenApplied
				logger.LogError(ctx, fmt.Sprintf("持久化任务统计退款 pending 失败 task %s: %s", task.TaskID, err.Error()))
				return false
			}
			var accountingErr error
			if discountSettlementID != "" {
				accountingErr = model.ReverseGroupModelDiscountSettlementWithUsage(
					discountSettlementID,
					model.BillingUsageDelta{UserID: task.UserId, ChannelID: task.ChannelId, QuotaDelta: -quota},
				)
			} else {
				model.UpdateUserUsedQuota(task.UserId, -quota)
				model.UpdateChannelUsedQuota(task.ChannelId, -quota)
			}
			if accountingErr != nil {
				logger.LogError(ctx, fmt.Sprintf("任务退款统计与账本提交失败 task %s: %s", task.TaskID, accountingErr.Error()))
				return false
			}
			billingContext.RefundState = model.TaskRefundStateAccountingApplied
			if err := task.UpdateBillingState(); err != nil {
				billingContext.RefundState = model.TaskRefundStateAccountingPending
				logger.LogError(ctx, fmt.Sprintf("任务统计已退但持久化 applied 失败，等待人工对账 task %s: %s", task.TaskID, err.Error()))
				return false
			}
		}

		if billingContext.RefundState != model.TaskRefundStateAccountingApplied {
			logger.LogWarn(ctx, fmt.Sprintf("任务 %s 退款状态 %s 不可自动继续", task.TaskID, billingContext.RefundState))
			return false
		}
	}

	if billingContext != nil {
		previousQuota := task.Quota
		task.Quota = 0
		billingContext.RefundState = model.TaskRefundStateCommitted
		if err := task.UpdateBillingState(); err != nil {
			task.Quota = previousQuota
			billingContext.RefundState = model.TaskRefundStateAccountingApplied
			logger.LogError(ctx, fmt.Sprintf("退款完成但提交 task 退款状态失败 task %s: %s", task.TaskID, err.Error()))
			return false
		}
	}

	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     quota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
	})
	return true
}

func completeTaskDiscountRefundAccounting(task *model.Task, quota int) bool {
	if task == nil || task.PrivateData.BillingContext == nil {
		return false
	}
	billingContext := task.PrivateData.BillingContext
	delta := model.BillingUsageDelta{
		UserID: task.UserId, ChannelID: task.ChannelId, QuotaDelta: -quota,
	}
	if err := model.ReverseGroupModelDiscountSettlementWithUsage(billingContext.DiscountSettlementID, delta); err != nil {
		return false
	}
	billingContext.RefundState = model.TaskRefundStateAccountingApplied
	if err := task.UpdateBillingState(); err != nil {
		billingContext.RefundState = model.TaskRefundStateAccountingPending
		return false
	}
	return true
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
// clamps 可选：若计算 actualQuota 时发生额度饱和，将其记入日志 admin_info（仅管理员可见）。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	if task == nil {
		return
	}
	recoverTaskInitialGroupModelSettlement(task)
	if actualQuota <= 0 {
		return
	}
	if billingContext := task.PrivateData.BillingContext; billingContext != nil &&
		billingContext.ChargeState != "" && billingContext.ChargeState != model.TaskChargeStateCharged {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费状态为 %s，跳过差额结算", task.TaskID, billingContext.ChargeState))
		return
	}
	if billingContext := task.PrivateData.BillingContext; billingContext != nil && billingContext.DiscountSettlementID != "" {
		logger.LogError(ctx, fmt.Sprintf("任务 %s 的 adaptor 仅返回结算额度，缺少月度折扣所需原始额度，跳过不安全调整", task.TaskID))
		return
	}
	_, err := applyTaskQuotaDelta(ctx, task, actualQuota, reason, clamps...)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("任务差额结算失败 task %s: %s", task.TaskID, err.Error()))
	}
}

// applyTaskQuotaDelta applies one exact fixed-ratio net-quota delta. The
// returned boolean reports whether funding moved before a later failure.
func applyTaskQuotaDelta(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) (bool, error) {
	if actualQuota < 0 {
		return false, fmt.Errorf("actual task quota cannot be negative: %d", actualQuota)
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota

	if quotaDelta == 0 {
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
		return false, nil
	}

	logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
		task.TaskID,
		logger.LogQuota(quotaDelta),
		logger.LogQuota(actualQuota),
		logger.LogQuota(preConsumedQuota),
		reason,
	))

	billingContext := task.PrivateData.BillingContext
	previousNetQuota := preConsumedQuota
	previousPendingNetQuota := 0
	previousChargeState := ""
	if billingContext == nil {
		billingContext = &model.TaskBillingContext{NetQuota: preConsumedQuota}
		task.PrivateData.BillingContext = billingContext
	} else {
		previousNetQuota = billingContext.NetQuota
		previousPendingNetQuota = billingContext.PendingNetQuota
		previousChargeState = billingContext.ChargeState
	}
	billingContext.PendingNetQuota = actualQuota
	billingContext.ChargeState = model.TaskChargeStatePendingReconcile
	if task.ID != 0 {
		if err := task.UpdateBillingState(); err != nil {
			billingContext.NetQuota = previousNetQuota
			billingContext.PendingNetQuota = previousPendingNetQuota
			billingContext.ChargeState = previousChargeState
			return false, fmt.Errorf("persist pending task adjustment intent: %w", err)
		}
	}

	// 调整资金来源
	if err := taskAdjustFunding(task, quotaDelta); err != nil {
		return false, fmt.Errorf("adjust task funding: %w", err)
	}

	// 资金已移动后，令牌失败必须传播给账本协调器并进入 pending；
	// 不能继续提交一个令牌、资金和任务额度不一致的最终状态。
	if err := taskAdjustTokenQuota(ctx, task, quotaDelta); err != nil {
		return true, fmt.Errorf("adjust task token quota: %w", err)
	}

	task.Quota = actualQuota
	billingContext.NetQuota = actualQuota
	billingContext.PendingNetQuota = 0
	billingContext.ChargeState = model.TaskChargeStateCharged

	// 提交阶段已经累计过一次请求；结算阶段只调整最终用量。
	model.UpdateUserUsedQuota(task.UserId, quotaDelta)
	model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)

	if task.ID != 0 {
		var persistErr error
		persistErr = task.UpdateBillingState()
		if persistErr != nil {
			task.Quota = preConsumedQuota
			billingContext.NetQuota = previousNetQuota
			billingContext.PendingNetQuota = actualQuota
			billingContext.ChargeState = model.TaskChargeStatePendingReconcile
			return true, fmt.Errorf("persist task billing state: %w", persistErr)
		}
	}

	recordTaskQuotaDeltaLog(task, preConsumedQuota, actualQuota, reason, clamps...)
	return true, nil
}

func recordTaskQuotaDeltaLog(task *model.Task, preConsumedQuota int, actualQuota int, reason string, clamps ...*common.QuotaClamp) {
	quotaDelta := actualQuota - preConsumedQuota
	if quotaDelta == 0 {
		return
	}
	logType := model.LogTypeConsume
	logQuota := quotaDelta
	if quotaDelta < 0 {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	for _, clamp := range clamps {
		attachQuotaSaturationToOther(other, clamp)
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   logType,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     logQuota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
		NodeName:  task.PrivateData.NodeName,
	})
}

// recoverTaskGroupModelDiscountAdjustment finalizes the task-local marker only
// from a settled adjustment whose accounting evidence matches this task. It is
// safe after a crash between the atomic ledger/accounting commit and the task
// row update because it never replays funding or usage deltas.
func recoverTaskGroupModelDiscountAdjustment(task *model.Task, adjustmentID string) error {
	if task == nil || task.PrivateData.BillingContext == nil || adjustmentID == "" {
		return model.ErrGroupModelDiscountAccountingConflict
	}
	billingContext := task.PrivateData.BillingContext
	adjustment, err := model.GetGroupModelDiscountAdjustment(adjustmentID)
	if err != nil {
		return err
	}
	if adjustment.Status != model.GroupModelDiscountStatusSettled || !adjustment.AccountingApplied ||
		adjustment.AdjustmentID != adjustmentID || adjustment.SettlementRequestID != billingContext.DiscountSettlementID ||
		adjustment.UserID != task.UserId || adjustment.ChannelID != task.ChannelId ||
		adjustment.AccountingUserID != task.UserId || adjustment.AccountingChannelID != task.ChannelId ||
		adjustment.AccountingQuotaDelta != int(adjustment.DeltaChargedQuota) ||
		adjustment.AccountingRequestCountDelta != 0 || adjustment.NewOriginalQuota < 0 ||
		adjustment.NewOriginalQuota > common.MaxQuota || adjustment.NewChargedQuota < 0 ||
		adjustment.NewChargedQuota > common.MaxQuota {
		return model.ErrGroupModelDiscountAccountingConflict
	}

	previousQuota := task.Quota
	previousOriginalQuota := billingContext.OriginalQuota
	previousNetQuota := billingContext.NetQuota
	previousAdjustmentID := billingContext.DiscountAdjustmentID
	previousChargeState := billingContext.ChargeState
	task.Quota = int(adjustment.NewChargedQuota)
	billingContext.OriginalQuota = int(adjustment.NewOriginalQuota)
	billingContext.NetQuota = int(adjustment.NewChargedQuota)
	billingContext.DiscountAdjustmentID = adjustmentID
	billingContext.ChargeState = model.TaskChargeStateCharged
	if err := task.UpdateBillingState(); err != nil {
		task.Quota = previousQuota
		billingContext.OriginalQuota = previousOriginalQuota
		billingContext.NetQuota = previousNetQuota
		billingContext.DiscountAdjustmentID = previousAdjustmentID
		billingContext.ChargeState = previousChargeState
		return err
	}
	return nil
}

func adjustTaskMonthlyModelCharge(
	ctx context.Context,
	task *model.Task,
	newOriginalQuota int,
	fallbackNetQuota int,
	reason string,
	clamps ...*common.QuotaClamp,
) {
	billingContext := task.PrivateData.BillingContext
	if billingContext == nil || billingContext.DiscountSettlementID == "" {
		RecalculateTaskQuota(ctx, task, fallbackNetQuota, reason, clamps...)
		return
	}

	adjustmentID := billingContext.DiscountSettlementID + ":complete"
	reservation, err := model.ReserveGroupModelDiscountAdjustment(model.GroupModelDiscountAdjustmentInput{
		AdjustmentID:        adjustmentID,
		SettlementRequestID: billingContext.DiscountSettlementID,
		NewOriginalQuota:    newOriginalQuota,
	})
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("预留任务月度折扣调整失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	if reservation.Reused {
		switch reservation.Adjustment.Status {
		case model.GroupModelDiscountStatusSettled:
			if err := recoverTaskGroupModelDiscountAdjustment(task, adjustmentID); err != nil {
				logger.LogError(ctx, fmt.Sprintf("回写已结算任务月度折扣调整失败 task %s: %s", task.TaskID, err.Error()))
			}
			return
		case model.GroupModelDiscountStatusPendingReconcile:
			if reservation.Adjustment.PendingAction != model.GroupModelDiscountPendingActionCommitAfterFunding {
				logger.LogError(ctx, fmt.Sprintf(
					"任务月度折扣调整 pending action %s 不允许自动提交 task %s adjustment %s",
					reservation.Adjustment.PendingAction,
					task.TaskID,
					adjustmentID,
				))
				return
			}
			// Pending alone is ambiguous: it can also mean a known-unfunded
			// reservation could not be rolled back. Only a task row that already
			// durably stores this exact adjustment and its parent totals proves
			// the funding delta ran; otherwise reconciliation stays fail-closed.
			persisted, getErr := model.GetTaskBillingState(task.ID)
			if getErr != nil || persisted.PrivateData.BillingContext == nil ||
				persisted.PrivateData.BillingContext.DiscountAdjustmentID != adjustmentID ||
				persisted.PrivateData.BillingContext.OriginalQuota != reservation.NewOriginalQuota ||
				persisted.PrivateData.BillingContext.NetQuota != reservation.NewChargedQuota ||
				persisted.Quota != reservation.NewChargedQuota {
				if getErr != nil {
					logger.LogError(ctx, fmt.Sprintf("读取任务月度折扣调整出资证据失败 task %s: %s", task.TaskID, getErr.Error()))
				} else {
					logger.LogError(ctx, fmt.Sprintf("任务月度折扣调整缺少已出资持久证据 task %s adjustment %s", task.TaskID, adjustmentID))
				}
				return
			}
			task.Quota = persisted.Quota
			task.PrivateData.BillingContext = persisted.PrivateData.BillingContext
			if err := model.CommitGroupModelDiscountAdjustmentWithUsage(adjustmentID, model.BillingUsageDelta{
				UserID: task.UserId, ChannelID: task.ChannelId, QuotaDelta: reservation.DeltaChargedQuota,
			}); err != nil {
				logger.LogError(ctx, fmt.Sprintf("重试提交任务月度折扣调整失败 task %s: %s", task.TaskID, err.Error()))
				return
			}
			if err := recoverTaskGroupModelDiscountAdjustment(task, adjustmentID); err != nil {
				logger.LogError(ctx, fmt.Sprintf("恢复已提交任务月度折扣调整状态失败 task %s: %s", task.TaskID, err.Error()))
			}
			return
		case model.GroupModelDiscountStatusReserved:
			// A reused bare reservation has an ambiguous crash boundary. Never
			// guess whether its funding delta ran or seize recovery from its owner.
			return
		default:
			logger.LogError(ctx, fmt.Sprintf("任务月度折扣调整状态不可重放 task %s: %s", task.TaskID, reservation.Adjustment.Status))
			return
		}
	}

	previousOriginalQuota := billingContext.OriginalQuota
	previousNetQuota := billingContext.NetQuota
	previousAdjustmentID := billingContext.DiscountAdjustmentID
	previousChargeState := billingContext.ChargeState
	previousQuota := task.Quota
	billingContext.OriginalQuota = reservation.NewOriginalQuota
	billingContext.NetQuota = reservation.NewChargedQuota
	billingContext.DiscountAdjustmentID = adjustmentID
	billingContext.ChargeState = model.TaskChargeStatePendingReconcile
	task.Quota = reservation.NewChargedQuota

	if reservation.DeltaChargedQuota != 0 {
		if err := taskAdjustFunding(task, reservation.DeltaChargedQuota); err != nil {
			billingContext.OriginalQuota = previousOriginalQuota
			billingContext.NetQuota = previousNetQuota
			billingContext.ChargeState = model.TaskChargeStatePendingReconcile
			task.Quota = previousQuota
			if persistErr := task.UpdateBillingState(); persistErr != nil {
				logger.LogError(ctx, fmt.Sprintf("持久化任务月度折扣资金未知状态失败 task %s: %s", task.TaskID, persistErr.Error()))
			}
			if markErr := model.MarkGroupModelDiscountAdjustmentPendingReconcile(adjustmentID, model.GroupModelDiscountPendingActionUnknownManual); markErr != nil {
				logger.LogError(ctx, fmt.Sprintf("标记任务月度折扣资金未知失败 task %s: %s", task.TaskID, markErr.Error()))
			}
			logger.LogError(ctx, fmt.Sprintf("应用任务月度折扣资金差额失败 task %s: %s", task.TaskID, err.Error()))
			return
		}
		if err := taskAdjustTokenQuota(ctx, task, reservation.DeltaChargedQuota); err != nil {
			billingContext.OriginalQuota = previousOriginalQuota
			billingContext.NetQuota = previousNetQuota
			billingContext.ChargeState = model.TaskChargeStatePendingReconcile
			task.Quota = previousQuota
			if persistErr := task.UpdateBillingState(); persistErr != nil {
				logger.LogError(ctx, fmt.Sprintf("持久化任务月度折扣令牌未知状态失败 task %s: %s", task.TaskID, persistErr.Error()))
			}
			if markErr := model.MarkGroupModelDiscountAdjustmentPendingReconcile(adjustmentID, model.GroupModelDiscountPendingActionUnknownManual); markErr != nil {
				logger.LogError(ctx, fmt.Sprintf("标记任务月度折扣令牌未知失败 task %s: %s", task.TaskID, markErr.Error()))
			}
			logger.LogError(ctx, fmt.Sprintf("应用任务月度折扣令牌差额失败 task %s: %s", task.TaskID, err.Error()))
			return
		}
	}

	if err := task.UpdateBillingState(); err != nil {
		billingContext.OriginalQuota = previousOriginalQuota
		billingContext.NetQuota = previousNetQuota
		billingContext.DiscountAdjustmentID = previousAdjustmentID
		billingContext.ChargeState = previousChargeState
		task.Quota = previousQuota
		if reservation.DeltaChargedQuota == 0 {
			if rollbackErr := model.RollbackGroupModelDiscountAdjustment(adjustmentID); rollbackErr != nil {
				_ = model.MarkGroupModelDiscountAdjustmentPendingReconcile(adjustmentID, model.GroupModelDiscountPendingActionRollbackUnfunded)
			}
		} else {
			_ = model.MarkGroupModelDiscountAdjustmentPendingReconcile(adjustmentID, model.GroupModelDiscountPendingActionUnknownManual)
		}
		logger.LogError(ctx, fmt.Sprintf("持久化任务月度折扣出资证据失败 task %s: %s", task.TaskID, err.Error()))
		return
	}

	if err := model.CommitGroupModelDiscountAdjustmentWithUsage(adjustmentID, model.BillingUsageDelta{
		UserID: task.UserId, ChannelID: task.ChannelId, QuotaDelta: reservation.DeltaChargedQuota,
	}); err != nil {
		if markErr := model.MarkGroupModelDiscountAdjustmentPendingReconcile(
			adjustmentID,
			model.GroupModelDiscountPendingActionCommitAfterFunding,
		); markErr != nil {
			logger.LogError(ctx, fmt.Sprintf("标记任务月度折扣调整待对账失败 task %s: %s", task.TaskID, markErr.Error()))
		}
		logger.LogError(ctx, fmt.Sprintf("提交任务月度折扣调整失败 task %s: %s", task.TaskID, err.Error()))
		return
	}
	if err := recoverTaskGroupModelDiscountAdjustment(task, adjustmentID); err != nil {
		logger.LogError(ctx, fmt.Sprintf("提交任务月度折扣调整后回写任务状态失败 task %s: %s", task.TaskID, err.Error()))
		return
	}
	recordTaskQuotaDeltaLog(task, previousQuota, reservation.NewChargedQuota, reason, clamps...)
}

func taskTokenQuotaFromFrozenRatios(
	totalTokens int,
	modelRatio float64,
	groupRatio float64,
	otherMultiplier float64,
) (int, int, *common.QuotaClamp, *common.QuotaClamp) {
	rawOriginalQuota := float64(totalTokens) * modelRatio * otherMultiplier
	originalQuota, originalClamp := common.QuotaFromFloatChecked(rawOriginalQuota)
	if rawOriginalQuota > 0 && originalQuota == 0 {
		originalQuota = 1
	}
	fallbackNetQuota, netClamp := common.QuotaFromFloatChecked(rawOriginalQuota * groupRatio)
	return originalQuota, fallbackNetQuota, originalClamp, netClamp
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率和分组倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还。支持钱包和订阅计费来源。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	if totalTokens <= 0 {
		return
	}

	// Async completion must use the immutable submit-time pricing snapshot.
	// Reading current model/group settings here would retroactively reprice an
	// in-flight task after an administrator changes configuration.
	billingContext := task.PrivateData.BillingContext
	if billingContext == nil || billingContext.ModelRatio <= 0 || billingContext.GroupRatio < 0 {
		return
	}
	resumableAdjustment := billingContext.ChargeState == model.TaskChargeStatePendingReconcile &&
		billingContext.DiscountSettlementID != "" &&
		billingContext.DiscountAdjustmentID == billingContext.DiscountSettlementID+":complete"
	if billingContext.ChargeState != "" && billingContext.ChargeState != model.TaskChargeStateCharged && !resumableAdjustment {
		logger.LogWarn(ctx, fmt.Sprintf("任务 %s 计费状态为 %s，跳过 token 重算", task.TaskID, billingContext.ChargeState))
		return
	}
	modelRatio := billingContext.ModelRatio
	groupRatio := billingContext.GroupRatio

	// 计算 OtherRatios 乘积（视频折扣、时长等）
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(billingContext); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}

	// First calculate the immutable pre-group original. Monthly tier selection
	// consumes that value directly; the fixed group ratio is only the fallback
	// for tasks admitted before monthly policies existed.
	originalQuota, actualQuota, originalClamp, netClamp := taskTokenQuotaFromFrozenRatios(
		totalTokens,
		modelRatio,
		groupRatio,
		otherMultiplier,
	)

	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, modelRatio, groupRatio, otherMultiplier)
	if billingContext.DiscountSettlementID != "" {
		adjustTaskMonthlyModelCharge(ctx, task, originalQuota, actualQuota, reason, originalClamp, netClamp)
		return
	}
	RecalculateTaskQuota(ctx, task, actualQuota, reason, netClamp)
}
