package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func CovertMjpActionToModelName(mjAction string) string {
	modelName := "mj_" + strings.ToLower(mjAction)
	if mjAction == constant.MjActionSwapFace {
		modelName = "swap_face"
	}
	return modelName
}

// PrepareMidjourneyTaskBilling sets the durable refund marker before the task is inserted.
func PrepareMidjourneyTaskBilling(relayInfo *relaycommon.RelayInfo, task *model.Midjourney, quota int, shouldBill bool) (bool, error) {
	if task == nil {
		return false, errors.New("Midjourney task is nil")
	}
	existingDiscountSettlementID := task.DiscountSettlementID
	task.Quota = 0
	task.TokenId = 0
	task.BillingChannelId = 0
	task.OriginalQuota = 0
	task.RefundedQuota = 0
	task.ChargeState = ""
	task.RefundState = ""
	task.BillingSource = ""
	task.SubscriptionId = 0
	task.UsingGroup = ""
	task.OriginModelName = ""
	task.DiscountSettlementID = ""
	task.DiscountPolicySnapshot = ""
	if !shouldBill {
		task.ChargeState = model.TaskChargeStateUncharged
		return false, nil
	}
	if relayInfo == nil {
		return false, errors.New("relay info is nil")
	}
	if quota < 0 {
		return false, errors.New("quota cannot be negative")
	}
	if relayInfo.BillingSource == BillingSourceSubscription && relayInfo.GroupModelDiscountSnapshot == nil {
		return false, errors.New("legacy Midjourney billing does not support subscriptions")
	}
	discountPolicySnapshot := ""
	if relayInfo.GroupModelDiscountSnapshot != nil {
		if relayInfo.PriceData.OriginalQuota < 0 {
			return false, errors.New("original quota cannot be negative")
		}
		snapshotJSON, err := common.Marshal(relayInfo.GroupModelDiscountSnapshot)
		if err != nil {
			return false, fmt.Errorf("marshal Midjourney monthly discount snapshot: %w", err)
		}
		discountPolicySnapshot = string(snapshotJSON)
	}

	task.Quota = quota
	task.OriginalQuota = quota
	if relayInfo.PriceData.OriginalQuota > 0 {
		task.OriginalQuota = relayInfo.PriceData.OriginalQuota
	}
	task.BillingSource = relayInfo.BillingSource
	if task.BillingSource == "" {
		task.BillingSource = BillingSourceWallet
	}
	task.SubscriptionId = relayInfo.SubscriptionId
	task.UsingGroup = relayInfo.UsingGroup
	task.OriginModelName = relayInfo.OriginModelName
	task.BillingChannelId = task.ChannelId
	if relayInfo.ChannelMeta != nil && relayInfo.ChannelId > 0 {
		task.BillingChannelId = relayInfo.ChannelId
	}
	if relayInfo.GroupModelDiscountSnapshot != nil {
		if !relayInfo.IsPlayground {
			task.TokenId = relayInfo.TokenId
		}
		if task.OriginalQuota > 0 {
			// The provider's task id is neither globally unique nor under our
			// control. Give each gateway task its own durable settlement key;
			// preserve an existing key so persisted retries and historical
			// `mj:<upstream-id>` rows keep their original accounting identity.
			if existingDiscountSettlementID != "" {
				task.DiscountSettlementID = existingDiscountSettlementID
			} else {
				task.DiscountSettlementID = "mj:" + uuid.NewString()
			}
		}
		task.DiscountPolicySnapshot = discountPolicySnapshot
	}
	task.ChargeState = model.TaskChargeStatePrepared
	return true, nil
}

// SettleMidjourneyTaskBilling persists a fail-closed intent before charging a legacy task.
func SettleMidjourneyTaskBilling(relayInfo *relaycommon.RelayInfo, task *model.Midjourney, prepared bool) (bool, error) {
	if !prepared {
		return false, nil
	}
	if relayInfo == nil {
		return false, errors.New("relay info is nil")
	}
	if task == nil || task.Id == 0 {
		return false, errors.New("Midjourney task must be persisted before billing")
	}
	if task.ChargeState != model.TaskChargeStatePrepared {
		return false, fmt.Errorf("Midjourney billing state %s cannot be claimed", task.ChargeState)
	}

	previousChargeState := task.ChargeState
	previousTokenID := task.TokenId
	task.ChargeState = model.TaskChargeStatePendingReconcile
	if !relayInfo.IsPlayground {
		task.TokenId = relayInfo.TokenId
	}
	claimed, updateErr := task.ClaimBillingState(previousChargeState)
	if updateErr != nil {
		task.ChargeState = previousChargeState
		task.TokenId = previousTokenID
		return false, fmt.Errorf("persist pending Midjourney billing intent: %w", updateErr)
	}
	if !claimed {
		task.ChargeState = previousChargeState
		task.TokenId = previousTokenID
		return false, errors.New("Midjourney billing intent was claimed by another owner")
	}

	_, billingErr := postConsumeQuotaWithResult(relayInfo, task.Quota, 0, true)
	if billingErr != nil {
		return false, billingErr
	}
	task.ChargeState = model.TaskChargeStateCharged
	claimed, updateErr = task.ClaimBillingState(model.TaskChargeStatePendingReconcile)
	if updateErr != nil {
		task.ChargeState = model.TaskChargeStatePendingReconcile
		return false, fmt.Errorf("update Midjourney billing state: %w", updateErr)
	}
	if !claimed {
		task.ChargeState = model.TaskChargeStatePendingReconcile
		return false, errors.New("Midjourney final billing state was claimed by another owner")
	}
	return true, nil
}

// SettleMidjourneyTaskModelCharge extends the legacy post-consume path with
// the captured user-group + origin-model monthly policy. Active policies must
// have pre-consumed true original quota before the upstream request; this
// coordinator persists the exact tiered net amount for every later consumer.
func SettleMidjourneyTaskModelCharge(
	c *gin.Context,
	relayInfo *relaycommon.RelayInfo,
	task *model.Midjourney,
	prepared bool,
) (bool, GroupModelDiscountDecision, error) {
	decision := GroupModelDiscountDecision{}
	if relayInfo == nil || relayInfo.GroupModelDiscountSnapshot == nil {
		billed, err := SettleMidjourneyTaskBilling(relayInfo, task, prepared)
		if task != nil {
			decision.OriginalQuota = task.OriginalQuota
			decision.ChargedQuota = task.Quota
		}
		return billed, decision, err
	}
	if !prepared {
		return false, decision, nil
	}
	if task == nil || task.Id == 0 {
		return false, decision, errors.New("Midjourney task must be persisted before billing")
	}
	if task.ChargeState != model.TaskChargeStatePrepared {
		return false, decision, fmt.Errorf("Midjourney monthly billing state %s cannot be claimed", task.ChargeState)
	}
	initialChargeState := task.ChargeState

	decision, settleErr := SettleModelCharge(
		c,
		relayInfo,
		task.DiscountSettlementID,
		task.OriginalQuota,
		task.Quota,
		model.BillingUsageDelta{
			UserID:            task.UserId,
			ChannelID:         task.GetBillingChannelId(),
			RequestCountDelta: 1,
		},
	)
	if settleErr != nil || decision.RequiresReconciliation {
		if settleErr == nil {
			settleErr = ErrGroupModelDiscountSettlementPending
		}
		if !decision.Applied {
			task.Quota = 0
			task.TokenId = 0
			task.DiscountSettlementID = ""
			task.ChargeState = model.TaskChargeStateUncharged
			claimed, updateErr := task.ClaimBillingState(initialChargeState)
			if updateErr != nil {
				return false, decision, errors.Join(settleErr, fmt.Errorf("persist unfunded Midjourney billing failure: %w", updateErr))
			}
			if !claimed {
				return false, decision, errors.Join(settleErr, errors.New("unfunded Midjourney billing state was claimed by another owner"))
			}
			return false, decision, settleErr
		}
		task.Quota = decision.ChargedQuota
		task.ChargeState = model.TaskChargeStatePendingReconcile
		if decision.Applied {
			task.DiscountSettlementID = decision.RequestID
		}
		claimed, updateErr := task.ClaimBillingState(initialChargeState)
		if updateErr != nil {
			if decision.Applied {
				updateErr = errors.Join(updateErr, model.MarkGroupModelDiscountPendingReconcile(
					decision.RequestID,
					model.GroupModelDiscountPendingActionUnknownManual,
				))
			}
			return false, decision, errors.Join(settleErr, fmt.Errorf("persist pending Midjourney monthly billing state: %w", updateErr))
		}
		if !claimed {
			return false, decision, errors.Join(settleErr, errors.New("pending Midjourney billing state was claimed by another owner"))
		}
		return false, decision, settleErr
	}
	if decision.Reused {
		// The replay's fresh admission reserve was returned by
		// SettleModelCharge(0). This row did not fund the historical charge and
		// therefore must not carry a refundable quota/settlement marker or emit
		// another consume log/stat update.
		task.Quota = 0
		task.TokenId = 0
		task.DiscountSettlementID = ""
		task.ChargeState = model.TaskChargeStateReused
		claimed, updateErr := task.ClaimBillingState(initialChargeState)
		if updateErr != nil {
			return false, decision, errors.Join(settleErr, fmt.Errorf("clear replayed Midjourney billing state: %w", updateErr))
		}
		if !claimed {
			return false, decision, errors.Join(settleErr, errors.New("replayed Midjourney billing state was claimed by another owner"))
		}
		return false, decision, settleErr
	}

	task.Quota = decision.ChargedQuota
	task.TokenId = 0
	if !relayInfo.IsPlayground && decision.ChargedQuota > 0 {
		task.TokenId = relayInfo.TokenId
	}
	task.ChargeState = model.TaskChargeStateCharged
	claimed, updateErr := task.ClaimBillingState(initialChargeState)
	if updateErr != nil {
		handoffErr := task.MarkBillingRecoveryPending(initialChargeState)
		if handoffErr != nil {
			manualErr := model.MarkGroupModelDiscountPendingReconcile(
				decision.RequestID,
				model.GroupModelDiscountPendingActionUnknownManual,
			)
			return false, decision, errors.Join(
				fmt.Errorf("persist Midjourney monthly billing state: %w", updateErr),
				fmt.Errorf("persist Midjourney billing recovery handoff: %w", handoffErr),
				manualErr,
			)
		}
		return false, decision, fmt.Errorf("persist Midjourney monthly billing state: %w", updateErr)
	}
	if !claimed {
		return false, decision, errors.New("Midjourney monthly billing state was claimed by another owner")
	}
	return true, decision, nil
}

func MidjourneyTaskNeedsRefund(task *model.Midjourney) bool {
	if task == nil {
		return false
	}
	recoverMidjourneyInitialGroupModelSettlement(task)
	if task.Quota != 0 {
		return true
	}
	if task.DiscountSettlementID == "" || task.RefundState == model.TaskRefundStateCommitted {
		return false
	}
	return task.ChargeState == "" || task.ChargeState == model.TaskChargeStateCharged
}

func recoverMidjourneyInitialGroupModelSettlement(task *model.Midjourney) bool {
	return task != nil && task.RecoverSettledInitialBilling()
}

// RefundMidjourneyQuota reverses every accounting element recorded for a billed legacy task.
func RefundMidjourneyQuota(ctx context.Context, task *model.Midjourney, reason string) bool {
	if task == nil {
		return false
	}
	recoverMidjourneyInitialGroupModelSettlement(task)
	if task.ChargeState != "" && task.ChargeState != model.TaskChargeStateCharged {
		logger.LogWarn(ctx, fmt.Sprintf("Midjourney 任务 %s 计费状态为 %s，跳过自动退款并等待对账", task.MjId, task.ChargeState))
		return false
	}
	quota := task.Quota
	if quota < 0 {
		logger.LogError(ctx, fmt.Sprintf("Midjourney 任务 %s 的退款额度不能为负数: %d", task.MjId, quota))
		return false
	}
	if task.RefundState == model.TaskRefundStateCommitted {
		return true
	}
	if task.RefundedQuota > 0 {
		quota = task.RefundedQuota
		if task.RefundState == "" {
			// Compatibility with rows written before staged refunds: this marker
			// was stored only after funding, token, and accounting were returned.
			task.RefundState = model.TaskRefundStateAccountingApplied
		}
	}
	if quota == 0 && task.RefundState == "" {
		if task.DiscountSettlementID == "" {
			return true
		}
		if err := model.BeginGroupModelDiscountSettlementReverse(task.DiscountSettlementID); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("零额度 Midjourney 退款前门禁失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
		task.RefundState = model.TaskRefundStateAccountingPending
		if err := task.UpdateBillingState(); err != nil {
			task.RefundState = ""
			if cancelErr := model.CancelGroupModelDiscountSettlementReverse(task.DiscountSettlementID); cancelErr != nil {
				logger.LogWarn(ctx, fmt.Sprintf("取消零额度 Midjourney 退款门禁失败 task %s: %s", task.MjId, cancelErr.Error()))
			}
			logger.LogError(ctx, fmt.Sprintf("持久化零额度 Midjourney 账本退款状态失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
	}

	switch task.RefundState {
	case model.TaskRefundStateFundingPending, model.TaskRefundStateTokenPending:
		logger.LogWarn(ctx, fmt.Sprintf("Midjourney 任务 %s 退款停留在模糊阶段 %s，禁止自动重放并等待对账", task.MjId, task.RefundState))
		return false
	case model.TaskRefundStateAccountingPending:
		if task.DiscountSettlementID == "" || !completeMidjourneyDiscountRefundAccounting(task, quota) {
			logger.LogWarn(ctx, fmt.Sprintf("Midjourney 任务 %s 退款统计阶段缺少可确认的账本证据", task.MjId))
			return false
		}
	case "":
		if task.DiscountSettlementID != "" {
			if err := model.BeginGroupModelDiscountSettlementReverse(task.DiscountSettlementID); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("Midjourney 退款前门禁失败 task %s: %s", task.MjId, err.Error()))
				return false
			}
		}
		task.RefundState = model.TaskRefundStateFundingPending
		if err := task.UpdateBillingState(); err != nil {
			task.RefundState = ""
			if task.DiscountSettlementID != "" {
				if cancelErr := model.CancelGroupModelDiscountSettlementReverse(task.DiscountSettlementID); cancelErr != nil {
					logger.LogWarn(ctx, fmt.Sprintf("取消未出资 Midjourney 退款门禁失败 task %s: %s", task.MjId, cancelErr.Error()))
				}
			}
			logger.LogError(ctx, fmt.Sprintf("持久化 Midjourney 退款资金 pending 失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
		var fundingErr error
		if task.BillingSource == BillingSourceSubscription && task.SubscriptionId > 0 {
			fundingErr = model.PostConsumeUserSubscriptionDelta(task.SubscriptionId, -int64(quota))
		} else {
			fundingErr = model.IncreaseUserQuota(task.UserId, quota, false)
		}
		if fundingErr != nil {
			if task.DiscountSettlementID != "" {
				if markErr := model.MarkGroupModelDiscountSettlementReverseFundingUnknown(task.DiscountSettlementID); markErr != nil {
					logger.LogWarn(ctx, fmt.Sprintf("标记 Midjourney 退款资金结果未知失败 task %s: %s", task.MjId, markErr.Error()))
				}
			}
			logger.LogWarn(ctx, fmt.Sprintf("退还 Midjourney 资金来源失败并保持 pending 等待对账 task %s: %s", task.MjId, fundingErr.Error()))
			return false
		}
		task.RefundedQuota = quota
		task.RefundState = model.TaskRefundStateFundingApplied
		if err := task.UpdateBillingState(); err != nil {
			task.RefundState = model.TaskRefundStateFundingPending
			logger.LogError(ctx, fmt.Sprintf("Midjourney 资金已退但持久化 applied 失败，等待人工对账 task %s: %s", task.MjId, err.Error()))
			return false
		}
	}

	if task.RefundState == model.TaskRefundStateFundingApplied {
		task.RefundState = model.TaskRefundStateTokenPending
		if err := task.UpdateBillingState(); err != nil {
			task.RefundState = model.TaskRefundStateFundingApplied
			logger.LogError(ctx, fmt.Sprintf("持久化 Midjourney 令牌退款 pending 失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
		if task.TokenId > 0 {
			tokenKey := resolveTokenKey(ctx, task.TokenId, task.MjId)
			if tokenKey == "" {
				logger.LogError(ctx, fmt.Sprintf("Midjourney 资金已退但无法解析令牌，保持 pending 等待对账 task %s", task.MjId))
				return false
			}
			if err := model.IncreaseTokenQuota(task.TokenId, tokenKey, quota); err != nil {
				logger.LogError(ctx, fmt.Sprintf("Midjourney 资金已退但令牌退款失败，保持 pending 等待对账 task %s: %s", task.MjId, err.Error()))
				return false
			}
		}
		task.RefundState = model.TaskRefundStateTokenApplied
		if err := task.UpdateBillingState(); err != nil {
			task.RefundState = model.TaskRefundStateTokenPending
			logger.LogError(ctx, fmt.Sprintf("Midjourney 令牌已退但持久化 applied 失败，等待人工对账 task %s: %s", task.MjId, err.Error()))
			return false
		}
	}

	if task.RefundState == model.TaskRefundStateTokenApplied {
		task.RefundState = model.TaskRefundStateAccountingPending
		if err := task.UpdateBillingState(); err != nil {
			task.RefundState = model.TaskRefundStateTokenApplied
			logger.LogError(ctx, fmt.Sprintf("持久化 Midjourney 统计退款 pending 失败 task %s: %s", task.MjId, err.Error()))
			return false
		}
		billingChannelId := task.GetBillingChannelId()
		var accountingErr error
		if task.DiscountSettlementID != "" {
			accountingErr = model.ReverseGroupModelDiscountSettlementWithUsage(
				task.DiscountSettlementID,
				model.BillingUsageDelta{UserID: task.UserId, ChannelID: billingChannelId, QuotaDelta: -quota},
			)
		} else {
			// Legacy rows have no ledger/accounting idempotency evidence. Keep
			// their historical best-effort stats update after the staged funding
			// markers so a stats-target failure cannot replay the fund refund.
			model.UpdateUserUsedQuota(task.UserId, -quota)
			model.UpdateChannelUsedQuota(billingChannelId, -quota)
		}
		if accountingErr != nil {
			logger.LogError(ctx, fmt.Sprintf("Midjourney 退款统计与账本提交失败 task %s: %s", task.MjId, accountingErr.Error()))
			return false
		}
		task.RefundState = model.TaskRefundStateAccountingApplied
		if err := task.UpdateBillingState(); err != nil {
			task.RefundState = model.TaskRefundStateAccountingPending
			logger.LogError(ctx, fmt.Sprintf("Midjourney 统计已退但持久化 applied 失败，等待人工对账 task %s: %s", task.MjId, err.Error()))
			return false
		}
	}
	if task.RefundState != model.TaskRefundStateAccountingApplied {
		logger.LogWarn(ctx, fmt.Sprintf("Midjourney 任务 %s 退款状态 %s 不可自动继续", task.MjId, task.RefundState))
		return false
	}

	billingChannelId := task.GetBillingChannelId()
	previousQuota := task.Quota
	task.Quota = 0
	task.RefundState = model.TaskRefundStateCommitted
	if err := task.UpdateBillingState(); err != nil {
		task.Quota = previousQuota
		task.RefundState = model.TaskRefundStateAccountingApplied
		logger.LogError(ctx, fmt.Sprintf("Midjourney 退款完成但提交状态失败 task %s: %s", task.MjId, err.Error()))
		return false
	}

	modelName := task.OriginModelName
	if modelName == "" {
		modelName = CovertMjpActionToModelName(task.Action)
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: billingChannelId,
		ModelName: modelName,
		Quota:     quota,
		TokenId:   task.TokenId,
		Group:     task.UsingGroup,
		Other: map[string]interface{}{
			"task_id": task.MjId,
			"reason":  reason,
		},
	})
	return true
}

func completeMidjourneyDiscountRefundAccounting(task *model.Midjourney, quota int) bool {
	if task == nil || task.DiscountSettlementID == "" {
		return false
	}
	delta := model.BillingUsageDelta{
		UserID: task.UserId, ChannelID: task.GetBillingChannelId(), QuotaDelta: -quota,
	}
	if err := model.ReverseGroupModelDiscountSettlementWithUsage(task.DiscountSettlementID, delta); err != nil {
		return false
	}
	task.RefundState = model.TaskRefundStateAccountingApplied
	if err := task.UpdateBillingState(); err != nil {
		task.RefundState = model.TaskRefundStateAccountingPending
		return false
	}
	return true
}

func GetMjRequestModel(relayMode int, midjRequest *dto.MidjourneyRequest) (string, *dto.MidjourneyResponse, bool) {
	action := ""
	if relayMode == relayconstant.RelayModeMidjourneyAction {
		// plus request
		err := CoverPlusActionToNormalAction(midjRequest)
		if err != nil {
			return "", err, false
		}
		action = midjRequest.Action
	} else {
		switch relayMode {
		case relayconstant.RelayModeMidjourneyImagine:
			action = constant.MjActionImagine
		case relayconstant.RelayModeMidjourneyVideo:
			action = constant.MjActionVideo
		case relayconstant.RelayModeMidjourneyEdits:
			action = constant.MjActionEdits
		case relayconstant.RelayModeMidjourneyDescribe:
			action = constant.MjActionDescribe
		case relayconstant.RelayModeMidjourneyBlend:
			action = constant.MjActionBlend
		case relayconstant.RelayModeMidjourneyShorten:
			action = constant.MjActionShorten
		case relayconstant.RelayModeMidjourneyChange:
			action = midjRequest.Action
		case relayconstant.RelayModeMidjourneyModal:
			action = constant.MjActionModal
		case relayconstant.RelayModeSwapFace:
			action = constant.MjActionSwapFace
		case relayconstant.RelayModeMidjourneyUpload:
			action = constant.MjActionUpload
		case relayconstant.RelayModeMidjourneySimpleChange:
			params := ConvertSimpleChangeParams(midjRequest.Content)
			if params == nil {
				return "", MidjourneyErrorWrapper(constant.MjRequestError, "invalid_request"), false
			}
			action = params.Action
		case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition, relayconstant.RelayModeMidjourneyNotify:
			return "", nil, true
		default:
			return "", MidjourneyErrorWrapper(constant.MjRequestError, "unknown_relay_action"), false
		}
	}
	modelName := CovertMjpActionToModelName(action)
	return modelName, nil, true
}

func CoverPlusActionToNormalAction(midjRequest *dto.MidjourneyRequest) *dto.MidjourneyResponse {
	// "customId": "MJ::JOB::upsample::2::3dbbd469-36af-4a0f-8f02-df6c579e7011"
	customId := midjRequest.CustomId
	if customId == "" {
		return MidjourneyErrorWrapper(constant.MjRequestError, "custom_id_is_required")
	}
	splits := strings.Split(customId, "::")
	var action string
	if splits[1] == "JOB" {
		action = splits[2]
	} else {
		action = splits[1]
	}

	if action == "" {
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action")
	}
	if strings.Contains(action, "upsample") {
		index, err := strconv.Atoi(splits[3])
		if err != nil {
			return MidjourneyErrorWrapper(constant.MjRequestError, "index_parse_failed")
		}
		midjRequest.Index = index
		midjRequest.Action = constant.MjActionUpscale
	} else if strings.Contains(action, "variation") {
		midjRequest.Index = 1
		if action == "variation" {
			index, err := strconv.Atoi(splits[3])
			if err != nil {
				return MidjourneyErrorWrapper(constant.MjRequestError, "index_parse_failed")
			}
			midjRequest.Index = index
			midjRequest.Action = constant.MjActionVariation
		} else if action == "low_variation" {
			midjRequest.Action = constant.MjActionLowVariation
		} else if action == "high_variation" {
			midjRequest.Action = constant.MjActionHighVariation
		}
	} else if strings.Contains(action, "pan") {
		midjRequest.Action = constant.MjActionPan
		midjRequest.Index = 1
	} else if strings.Contains(action, "reroll") {
		midjRequest.Action = constant.MjActionReRoll
		midjRequest.Index = 1
	} else if action == "Outpaint" {
		midjRequest.Action = constant.MjActionZoom
		midjRequest.Index = 1
	} else if action == "CustomZoom" {
		midjRequest.Action = constant.MjActionCustomZoom
		midjRequest.Index = 1
	} else if action == "Inpaint" {
		midjRequest.Action = constant.MjActionInPaint
		midjRequest.Index = 1
	} else {
		return MidjourneyErrorWrapper(constant.MjRequestError, "unknown_action:"+customId)
	}
	return nil
}

func ConvertSimpleChangeParams(content string) *dto.MidjourneyRequest {
	split := strings.Split(content, " ")
	if len(split) != 2 {
		return nil
	}

	action := strings.ToLower(split[1])
	changeParams := &dto.MidjourneyRequest{}
	changeParams.TaskId = split[0]

	if action[0] == 'u' {
		changeParams.Action = "UPSCALE"
	} else if action[0] == 'v' {
		changeParams.Action = "VARIATION"
	} else if action == "r" {
		changeParams.Action = "REROLL"
		return changeParams
	} else {
		return nil
	}

	index, err := strconv.Atoi(action[1:2])
	if err != nil || index < 1 || index > 4 {
		return nil
	}
	changeParams.Index = index
	return changeParams
}

func DoMidjourneyHttpRequest(c *gin.Context, timeout time.Duration, fullRequestURL string) (*dto.MidjourneyResponseWithStatusCode, []byte, error) {
	var nullBytes []byte
	//var requestBody io.Reader
	//requestBody = c.Request.Body
	// read request body to json, delete accountFilter and notifyHook
	var mapResult map[string]interface{}
	// if get request, no need to read request body
	if c.Request.Method != "GET" {
		err := json.NewDecoder(c.Request.Body).Decode(&mapResult)
		if err != nil {
			return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "read_request_body_failed", http.StatusInternalServerError), nullBytes, err
		}
		if !setting.MjAccountFilterEnabled {
			delete(mapResult, "accountFilter")
		}
		if !setting.MjNotifyEnabled {
			delete(mapResult, "notifyHook")
		}
		//req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
		// make new request with mapResult
	}
	if setting.MjModeClearEnabled {
		if prompt, ok := mapResult["prompt"].(string); ok {
			prompt = strings.Replace(prompt, "--fast", "", -1)
			prompt = strings.Replace(prompt, "--relax", "", -1)
			prompt = strings.Replace(prompt, "--turbo", "", -1)

			mapResult["prompt"] = prompt
		}
	}
	reqBody, err := json.Marshal(mapResult)
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "marshal_request_body_failed", http.StatusInternalServerError), nullBytes, err
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "create_request_failed", http.StatusInternalServerError), nullBytes, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	// 使用带有超时的 context 创建新的请求
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	auth := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	if auth != "" {
		auth = strings.TrimPrefix(auth, "Bearer ")
		req.Header.Set("mj-api-secret", auth)
	}
	defer cancel()
	resp, err := GetHttpClient().Do(req)
	if err != nil {
		common.SysLog("do request failed: " + err.Error())
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "do_request_failed", http.StatusInternalServerError), nullBytes, err
	}
	statusCode := resp.StatusCode
	//if statusCode != 200  {
	//	return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "bad_response_status_code", statusCode), nullBytes, nil
	//}
	err = req.Body.Close()
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "close_request_body_failed", statusCode), nullBytes, err
	}
	err = c.Request.Body.Close()
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "close_request_body_failed", statusCode), nullBytes, err
	}
	var midjResponse dto.MidjourneyResponse
	var midjourneyUploadsResponse dto.MidjourneyUploadResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "read_response_body_failed", statusCode), nullBytes, err
	}
	CloseResponseBodyGracefully(resp)
	logger.LogDebug(c, "midjourney response body: %s", responseBody)
	if len(responseBody) == 0 {
		return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "empty_response_body", statusCode), responseBody, nil
	} else {
		err = json.Unmarshal(responseBody, &midjResponse)
		if err != nil {
			err2 := json.Unmarshal(responseBody, &midjourneyUploadsResponse)
			if err2 != nil {
				return MidjourneyErrorWithStatusCodeWrapper(constant.MjErrorUnknown, "unmarshal_response_body_failed", statusCode), responseBody, err
			}
		}
	}
	//for k, v := range resp.Header {
	//	c.Writer.Header().Set(k, v[0])
	//}
	return &dto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   midjResponse,
	}, responseBody, nil
}
