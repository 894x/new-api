package service

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

const (
	cacheWrite5mHeader = "Msh-Usage-Cache-Write-Tokens-5m"
	cacheWrite1hHeader = "Msh-Usage-Cache-Write-Tokens-1h"
)

const (
	usageBillingPathLocal              = "local"
	usageBillingPathUpstream           = "upstream"
	usageBillingPathOpenAI             = "billing-usage-openai"
	usageBillingPathOpenAIEstimated    = "billing-usage-openai-estimated"
	usageBillingPathAnthropic          = "billing-usage-anthropic"
	usageBillingPathAnthropicEstimated = "billing-usage-anthropic-estimated"
	usageBillingPathGemini             = "billing-usage-gemini"
	usageBillingPathGeminiEstimated    = "billing-usage-gemini-estimated"
)

func effectiveBillingUsage(usage *dto.Usage) *dto.Usage {
	if billingUsage, ok := usageFromBillingUsage(usage); ok {
		return billingUsage
	}
	return usage
}

// Some OpenAI-compatible providers report TTL buckets only in response headers.
// Use them only when the body has no breakdown and the complete header pair
// agrees with its aggregate. Never infer a billed TTL from the request's TTL.
func cacheCreationUsageFromHeaders(ctx *gin.Context, usage *dto.Usage) *dto.Usage {
	if ctx == nil || usage == nil || usage.CacheCreation != nil ||
		usage.ClaudeCacheCreation5mTokens != 0 || usage.ClaudeCacheCreation1hTokens != 0 {
		return usage
	}
	headers, ok := common.GetContextKeyType[map[string]string](ctx, common.UpstreamResponseHeadersKey)
	if !ok {
		return usage
	}
	fiveMinutes, err5m := strconv.Atoi(strings.TrimSpace(headers[http.CanonicalHeaderKey(cacheWrite5mHeader)]))
	oneHour, err1h := strconv.Atoi(strings.TrimSpace(headers[http.CanonicalHeaderKey(cacheWrite1hHeader)]))
	maxInt := int(^uint(0) >> 1)
	if err5m != nil || err1h != nil || fiveMinutes < 0 || oneHour < 0 || fiveMinutes > maxInt-oneHour {
		return usage
	}
	total, _, _ := usage.CacheCreationTokenCounts()
	split := fiveMinutes + oneHour
	if split == 0 || total > 0 && total != split {
		return usage
	}
	adjusted := *usage
	adjusted.CacheCreation = &dto.ClaudeCacheCreationUsage{
		Ephemeral5mInputTokens: fiveMinutes,
		Ephemeral1hInputTokens: oneHour,
	}
	return &adjusted
}

func captureRelayPerformanceUsage(info *relaycommon.RelayInfo, usage *dto.Usage) {
	tokens := performanceTokenUsage(usage)
	info.PerformanceInputTokens = tokens.InputTokens
	info.PerformanceOutputTokens = tokens.OutputTokens
	info.PerformanceCacheReadTokens = tokens.CacheReadTokens
}

func performanceTokenUsage(usage *dto.Usage) perfmetrics.RelayTokenUsage {
	usage = effectiveBillingUsage(usage)
	if usage == nil {
		return perfmetrics.RelayTokenUsage{}
	}

	inputTokens := usage.InputTokens
	if inputTokens <= 0 {
		inputTokens = usage.PromptTokens
	}
	outputTokens := usage.OutputTokens
	if outputTokens <= 0 {
		outputTokens = usage.CompletionTokens
	}
	cacheReadTokens := usage.PromptTokensDetails.CachedTokens
	if cacheReadTokens <= 0 && usage.InputTokensDetails != nil {
		cacheReadTokens = usage.InputTokensDetails.CachedTokens
	}
	if cacheReadTokens <= 0 {
		cacheReadTokens = usage.PromptCacheHitTokens
	}

	return perfmetrics.RelayTokenUsage{
		InputTokens:     int64(inputTokens),
		OutputTokens:    int64(outputTokens),
		CacheReadTokens: int64(cacheReadTokens),
	}
}

func usageBillingPathForLog(isLocalCountTokens bool, usage *dto.Usage) string {
	effectiveUsage, ok := usageFromBillingUsage(usage)
	if !ok {
		if isLocalCountTokens {
			return usageBillingPathLocal
		}
		return usageBillingPathUpstream
	}

	switch effectiveUsage.UsageSemantic {
	case dto.BillingUsageSemanticOpenAI:
		if usage.BillingUsage.Estimated {
			return usageBillingPathOpenAIEstimated
		}
		return usageBillingPathOpenAI
	case dto.BillingUsageSemanticAnthropic:
		if usage.BillingUsage.Estimated {
			return usageBillingPathAnthropicEstimated
		}
		return usageBillingPathAnthropic
	case dto.BillingUsageSemanticGemini:
		if usage.BillingUsage.Estimated {
			return usageBillingPathGeminiEstimated
		}
		return usageBillingPathGemini
	}

	return usageBillingPathUpstream
}

func appendUsageBillingPathForLog(other *model.LogOther, isLocalCountTokens bool, usage *dto.Usage) {
	if other == nil {
		return
	}
	other.SetAdmin("usage_billing_path", usageBillingPathForLog(isLocalCountTokens, usage))
}

func usageFromBillingUsage(usage *dto.Usage) (*dto.Usage, bool) {
	if usage == nil || usage.BillingUsage == nil {
		return nil, false
	}
	return usage.BillingUsage.CanonicalUsage()
}
