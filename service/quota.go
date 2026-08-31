package service

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
}

type QuotaInfo struct {
	InputDetails  TokenDetails
	OutputDetails TokenDetails
	ModelName     string
	UsePrice      bool
	ModelPrice    float64
	ModelRatio    float64
	GroupRatio    float64
}

type AudioQuotaResult struct {
	OriginalQuota int
	ChargedQuota  int
	OriginalClamp *common.QuotaClamp
	ChargedClamp  *common.QuotaClamp
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) AudioQuotaResult {
	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	var original decimal.Decimal
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		original = modelPrice.Mul(quotaPerUnit)
	} else {
		completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
		audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
		audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))

		modelRatio := decimal.NewFromFloat(info.ModelRatio)

		inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
		outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
		inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
		outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

		original = original.Add(inputTextTokens)
		original = original.Add(outputTextTokens.Mul(completionRatio))
		original = original.Add(inputAudioTokens.Mul(audioRatio))
		original = original.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio))
		original = original.Mul(modelRatio)

		if !modelRatio.IsZero() && original.LessThanOrEqual(decimal.Zero) {
			original = decimal.NewFromInt(1)
		}
	}

	charged := original.Mul(groupRatio)
	if !groupRatio.IsZero() && original.IsPositive() && charged.LessThanOrEqual(decimal.Zero) {
		charged = decimal.NewFromInt(1)
	}
	originalQuota, originalClamp := common.QuotaFromDecimalChecked(original)
	if original.IsPositive() && originalQuota == 0 {
		originalQuota = 1
	}
	chargedQuota, chargedClamp := common.QuotaFromDecimalChecked(charged)
	return AudioQuotaResult{
		OriginalQuota: originalQuota,
		ChargedQuota:  chargedQuota,
		OriginalClamp: originalClamp,
		ChargedClamp:  chargedClamp,
	}
}

func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   relayInfo.PriceData.UsePrice,
		ModelPrice: relayInfo.PriceData.ModelPrice,
		ModelRatio: relayInfo.PriceData.ModelRatio,
		GroupRatio: relayInfo.PriceData.GroupRatioInfo.GroupRatio,
	}

	result := calculateAudioQuota(quotaInfo)
	if relayInfo.GroupModelDiscountSnapshot != nil {
		noteQuotaClamp(relayInfo, result.OriginalClamp)
	}
	noteQuotaClamp(relayInfo, result.ChargedClamp)
	targetQuota := result.ChargedQuota
	if relayInfo.GroupModelDiscountSnapshot != nil {
		targetQuota = result.OriginalQuota
	}
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		usedVars := billingexpr.UsedVars(snap.ExprString)
		if tieredOK, tieredQuota, tieredResult := TryTieredSettle(
			relayInfo,
			buildRealtimeTieredTokenParams(usage, usedVars),
		); tieredOK {
			targetQuota = tieredQuota
			if relayInfo.GroupModelDiscountSnapshot != nil {
				if tieredResult != nil {
					var clamp *common.QuotaClamp
					targetQuota, clamp = common.QuotaRoundChecked(tieredResult.ActualQuotaBeforeGroup)
					if tieredResult.ActualQuotaBeforeGroup > 0 && targetQuota == 0 {
						targetQuota = 1
					}
					noteQuotaClamp(relayInfo, clamp)
				} else {
					targetQuota = relayInfo.PriceData.OriginalQuotaToPreConsume
				}
			}
		}
	}
	if relayInfo.Billing == nil {
		if apiErr := PreConsumeBilling(ctx, targetQuota, relayInfo); apiErr != nil {
			return apiErr
		}
	} else if err := relayInfo.Billing.ReserveForAdmission(targetQuota); err != nil {
		return err
	}
	relayInfo.FinalPreConsumedQuota = relayInfo.Billing.GetPreConsumedQuota()
	logger.LogInfo(ctx, "realtime streaming reserved cumulative quota: "+fmt.Sprintf("%d", targetQuota))
	return nil
}

func buildRealtimeTieredTokenParams(usage *dto.RealtimeUsage, usedVars map[string]bool) billingexpr.TokenParams {
	if usage == nil {
		return billingexpr.TokenParams{}
	}
	promptTokens := float64(usage.InputTokens)
	completionTokens := float64(usage.OutputTokens)
	cacheReadTokens := float64(usage.InputTokenDetails.CachedTokens)
	audioInputTokens := float64(usage.InputTokenDetails.AudioTokens)
	audioOutputTokens := float64(usage.OutputTokenDetails.AudioTokens)
	if usedVars["cr"] {
		promptTokens -= cacheReadTokens
	}
	if usedVars["ai"] {
		promptTokens -= audioInputTokens
	}
	if usedVars["ao"] {
		completionTokens -= audioOutputTokens
	}
	if promptTokens < 0 {
		promptTokens = 0
	}
	if completionTokens < 0 {
		completionTokens = 0
	}
	return billingexpr.TokenParams{
		P:   promptTokens,
		C:   completionTokens,
		Len: float64(usage.InputTokens),
		CR:  cacheReadTokens,
		AI:  audioInputTokens,
		AO:  audioOutputTokens,
	}
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, _ string,
	usage *dto.RealtimeUsage, extraContent string) {

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, buildRealtimeTieredTokenParams(usage, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	// Billing always follows the client-visible origin model. The upstream
	// mapped name may have different ratio settings and must not split either
	// the price decision or the monthly model ledger.
	billingModelName := relayInfo.OriginModelName
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(billingModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(billingModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  billingModelName,
		UsePrice:   usePrice,
		ModelPrice: modelPrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quotaResult := calculateAudioQuota(quotaInfo)
	quota := quotaResult.ChargedQuota
	originalQuota := quotaResult.OriginalQuota
	if relayInfo.GroupModelDiscountSnapshot != nil {
		noteQuotaClamp(relayInfo, quotaResult.OriginalClamp)
	}
	noteQuotaClamp(relayInfo, quotaResult.ChargedClamp)
	if tieredOk {
		quota = tieredQuota
		var clamp *common.QuotaClamp
		if tieredRes != nil {
			originalQuota, clamp = common.QuotaRoundChecked(tieredRes.ActualQuotaBeforeGroup)
			if tieredRes.ActualQuotaBeforeGroup > 0 && originalQuota == 0 {
				originalQuota = 1
			}
		} else if snap := relayInfo.TieredBillingSnapshot; snap != nil {
			originalQuota, clamp = common.QuotaRoundChecked(snap.EstimatedQuotaBeforeGroup)
			if snap.EstimatedQuotaBeforeGroup > 0 && originalQuota == 0 {
				originalQuota = 1
			}
		}
		if relayInfo.GroupModelDiscountSnapshot != nil {
			noteQuotaClamp(relayInfo, clamp)
		}
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		originalQuota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, billingModelName, relayInfo.FinalPreConsumedQuota))
	}

	groupDiscountDecision, err := SettleModelCharge(ctx, relayInfo, relayInfo.RequestId, originalQuota, quota)
	if err != nil {
		if groupDiscountDecision.AdmissionRefundSafe {
			recordGroupModelDiscountAdmissionError(ctx, err)
		}
		logger.LogError(ctx, "error settling billing: "+err.Error())
		return
	}
	quota = groupDiscountDecision.ChargedQuota
	if groupDiscountDecision.Reused {
		if err := SettleModelRequestTPM(ctx, usage.InputTokens, usage.OutputTokens); err != nil {
			logger.LogError(ctx, "error settling model request TPM: "+err.Error())
		}
		return
	}
	if totalTokens != 0 && !groupDiscountDecision.Applied {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	logModel := billingModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	InjectGroupModelDiscountInfo(other, groupDiscountDecision)
	attachQuotaSaturation(ctx, relayInfo, other)
	if err := SettleModelRequestTPM(ctx, usage.InputTokens, usage.OutputTokens); err != nil {
		logger.LogError(ctx, "error settling model request TPM: "+err.Error())
	}
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
}

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) int {
	if priceData.CacheCreationRatio == 1 {
		return 0
	}
	quotaPrice := priceData.ModelRatio / common.QuotaPerUnit
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	return int(math.Round((cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)))
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) {
	completedAt := time.Now()

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, false, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(relayInfo.OriginModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(relayInfo.OriginModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  relayInfo.OriginModelName,
		UsePrice:   usePrice,
		ModelPrice: modelPrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quotaResult := calculateAudioQuota(quotaInfo)
	quota := quotaResult.ChargedQuota
	originalQuota := quotaResult.OriginalQuota
	if relayInfo.GroupModelDiscountSnapshot != nil {
		noteQuotaClamp(relayInfo, quotaResult.OriginalClamp)
	}
	noteQuotaClamp(relayInfo, quotaResult.ChargedClamp)
	if tieredOk {
		quota = tieredQuota
		var clamp *common.QuotaClamp
		if tieredRes != nil {
			originalQuota, clamp = common.QuotaRoundChecked(tieredRes.ActualQuotaBeforeGroup)
			if tieredRes.ActualQuotaBeforeGroup > 0 && originalQuota == 0 {
				originalQuota = 1
			}
		} else if snap := relayInfo.TieredBillingSnapshot; snap != nil {
			originalQuota, clamp = common.QuotaRoundChecked(snap.EstimatedQuotaBeforeGroup)
			if snap.EstimatedQuotaBeforeGroup > 0 && originalQuota == 0 {
				originalQuota = 1
			}
		}
		if relayInfo.GroupModelDiscountSnapshot != nil {
			noteQuotaClamp(relayInfo, clamp)
		}
	}

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		originalQuota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, relayInfo.OriginModelName, relayInfo.FinalPreConsumedQuota))
	}

	groupDiscountDecision, err := SettleModelCharge(ctx, relayInfo, relayInfo.RequestId, originalQuota, quota)
	if err != nil {
		if groupDiscountDecision.AdmissionRefundSafe {
			recordGroupModelDiscountAdmissionError(ctx, err)
		}
		logger.LogError(ctx, "error settling billing: "+err.Error())
		return
	}
	quota = groupDiscountDecision.ChargedQuota
	if groupDiscountDecision.Reused {
		if err := SettleModelRequestTPM(ctx, usage.PromptTokens, usage.CompletionTokens); err != nil {
			logger.LogError(ctx, "error settling model request TPM: "+err.Error())
		}
		perfmetrics.RecordRelaySampleAsync(relayInfo, true, performanceTokenUsage(usage), completedAt)
		return
	}
	if totalTokens != 0 && !groupDiscountDecision.Applied {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	logModel := relayInfo.OriginModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	InjectGroupModelDiscountInfo(other, groupDiscountDecision)
	attachQuotaSaturation(ctx, relayInfo, other)
	if err := SettleModelRequestTPM(ctx, usage.PromptTokens, usage.CompletionTokens); err != nil {
		logger.LogError(ctx, "error settling model request TPM: "+err.Error())
	}
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
	perfmetrics.RecordRelaySampleAsync(relayInfo, true, performanceTokenUsage(usage), completedAt)
}

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	// 原子预扣：检查与扣减在同一操作中完成，并发请求不可能同时通过检查后超扣。
	reserved, err := model.TryReserveTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota, relayInfo.TokenUnlimited)
	if err != nil {
		return err
	}
	if !reserved {
		remainQuota := 0
		if token, tokenErr := model.GetTokenByKey(relayInfo.TokenKey, false); tokenErr == nil && token != nil {
			remainQuota = token.RemainQuota
		}
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(remainQuota), logger.FormatQuota(quota))
	}
	return nil
}

type postConsumeQuotaResult struct {
	FundingApplied bool
	TokenApplied   bool
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) error {
	_, err := postConsumeQuotaWithResult(relayInfo, quota, preConsumedQuota, sendEmail)
	return err
}

func postConsumeQuotaWithResult(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (result postConsumeQuotaResult, err error) {

	// 1) Consume from wallet quota OR subscription item
	if relayInfo != nil && relayInfo.BillingSource == BillingSourceSubscription {
		if relayInfo.SubscriptionId == 0 {
			return result, errors.New("subscription id is missing")
		}
		delta := int64(quota)
		if delta != 0 {
			if err := model.PostConsumeUserSubscriptionDelta(relayInfo.SubscriptionId, delta); err != nil {
				return result, err
			}
			relayInfo.SubscriptionPostDelta += delta
		}
	} else {
		// Wallet
		if quota > 0 {
			err = model.DecreaseUserQuota(relayInfo.UserId, quota, false)
		} else {
			err = model.IncreaseUserQuota(relayInfo.UserId, -quota, false)
		}
		if err != nil {
			return result, err
		}
	}
	result.FundingApplied = true

	if !relayInfo.IsPlayground {
		if quota > 0 {
			err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
		} else {
			err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
		}
		if err != nil {
			return result, err
		}
		result.TokenApplied = true
	}

	if sendEmail {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return result, nil
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		//noMoreQuota := userCache.Quota-(quota+preConsumedQuota) <= 0
		quotaTooLow := false
		consumeQuota := quota + preConsumedQuota
		if relayInfo.UserQuota-consumeQuota < threshold {
			quotaTooLow = true
		}
		if quotaTooLow {
			prompt := "您的额度即将用尽"
			topUpLink := PaymentReturnURL("/wallet")

			// 根据通知方式生成不同的内容格式
			var content string
			var values []interface{}

			notifyType := userSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}

			if notifyType == dto.NotifyTypeBark {
				// Bark推送使用简短文本，不支持HTML
				content = "{{value}}，剩余额度：{{value}}，请及时充值"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else if notifyType == dto.NotifyTypeGotify {
				content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else {
				// 默认内容格式，适用于Email和Webhook（支持HTML）
				content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota), topUpLink, topUpLink}
			}

			err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
			}
		}
	})
}

func checkAndSendSubscriptionQuotaNotify(relayInfo *relaycommon.RelayInfo) {
	gopool.Go(func() {
		if relayInfo == nil {
			return
		}
		if relayInfo.SubscriptionId == 0 || relayInfo.SubscriptionAmountTotal <= 0 {
			return
		}

		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		usedAfter := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		remaining := relayInfo.SubscriptionAmountTotal - usedAfter
		if remaining >= int64(threshold) {
			return
		}

		prompt := "您的订阅额度即将用尽"
		topUpLink := PaymentReturnURL("/wallet")

		var content string
		var values []interface{}
		notifyType := userSetting.NotifyType
		if notifyType == "" {
			notifyType = dto.NotifyTypeEmail
		}

		if notifyType == dto.NotifyTypeBark {
			content = "{{value}}，剩余额度：{{value}}，请及时充值"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining))}
		} else if notifyType == dto.NotifyTypeGotify {
			content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining))}
		} else {
			content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
			values = []interface{}{prompt, logger.FormatQuota(int(remaining)), topUpLink, topUpLink}
		}

		if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values)); err != nil {
			common.SysError(fmt.Sprintf("failed to send subscription quota notify to user %d: %s", relayInfo.UserId, err.Error()))
		}
	})
}
