package common

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// CacheCreationUsageFromHeaders fills a missing TTL breakdown only when a
// complete, valid upstream header pair agrees with the reported write total.
func CacheCreationUsageFromHeaders(ctx *gin.Context, usage *dto.Usage) *dto.Usage {
	if ctx == nil || usage == nil || usage.CacheCreation != nil ||
		usage.ClaudeCacheCreation5mTokens != 0 || usage.ClaudeCacheCreation1hTokens != 0 {
		return usage
	}
	headers, ok := GetContextKeyType[map[string]string](ctx, UpstreamResponseHeadersKey)
	if !ok {
		return usage
	}
	fiveMinutes, err5m := strconv.Atoi(strings.TrimSpace(headers[http.CanonicalHeaderKey("Msh-Usage-Cache-Write-Tokens-5m")]))
	oneHour, err1h := strconv.Atoi(strings.TrimSpace(headers[http.CanonicalHeaderKey("Msh-Usage-Cache-Write-Tokens-1h")]))
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
	adjusted.CacheCreation = &dto.ClaudeCacheCreationUsage{Ephemeral5mInputTokens: fiveMinutes, Ephemeral1hInputTokens: oneHour}
	return &adjusted
}

// CacheWriteSimulationApplies limits the policy to text protocols that can
// represent cache creation, keeping image, audio and task accounting unchanged.
func CacheWriteSimulationApplies(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil || GetContextKeyBool(c, constant.ContextKeyLocalCountTokens) {
		return false
	}
	path := c.Request.URL.Path
	return strings.HasSuffix(path, "/chat/completions") || strings.HasSuffix(path, "/completions") ||
		strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/responses/compact") || strings.HasSuffix(path, "/messages")
}

// ApplyCacheWriteSimulation returns a detached usage copy. OpenAI totals include
// cache, while Anthropic input excludes it. Both retain the same context length.
// The caller retains the original BillingUsage as upstream evidence.
func ApplyCacheWriteSimulation(usage *dto.Usage, policy *dto.CacheWriteSimulationSettings, anthropic bool) (*dto.Usage, int) {
	if usage == nil || policy == nil || !policy.Enabled || policy.Validate() != nil || policy.Percentage == 0 ||
		usage.BillingUsage != nil && usage.BillingUsage.Estimated {
		return usage, 0
	}
	if total, _, _ := usage.CacheCreationTokenCounts(); total > 0 {
		return usage, 0
	}
	prompt := usage.PromptTokens
	cached := usage.PromptTokensDetails.CachedTokens
	image := usage.PromptTokensDetails.ImageTokens
	audio := usage.PromptTokensDetails.AudioTokens
	if prompt <= 0 || prompt > math.MaxInt32 || cached < 0 || cached > math.MaxInt32 || image < 0 || image > math.MaxInt32 || audio < 0 || audio > math.MaxInt32 {
		return usage, 0
	}
	totalInput := int64(prompt)
	uncached := totalInput
	if anthropic {
		totalInput += int64(cached)
	} else {
		uncached -= int64(cached)
	}
	// Do not reclassify independently billed image/audio categories as text cache.
	uncached -= int64(image)
	uncached -= int64(audio)
	if totalInput <= int64(policy.MinInputTokens) || uncached <= 0 {
		return usage, 0
	}
	tokens := QuotaFromDecimal(decimal.NewFromInt(uncached).Mul(decimal.NewFromFloat(policy.Percentage)).Div(decimal.NewFromInt(100)).Floor())
	if tokens <= 0 {
		return usage, 0
	}
	adjusted := *usage
	adjusted.BillingUsage = nil
	adjusted.PromptTokensDetails = usage.PromptTokensDetails.Clone()
	adjusted.PromptTokensDetails.CacheWriteTokens = tokens
	adjusted.PromptTokensDetails.CachedCreationTokens = tokens
	if usage.InputTokensDetails != nil {
		details := usage.InputTokensDetails.Clone()
		details.CacheWriteTokens, details.CachedCreationTokens = tokens, tokens
		adjusted.InputTokensDetails = &details
	}
	adjusted.CacheCreation = &dto.ClaudeCacheCreationUsage{}
	if policy.TTL == "1h" {
		adjusted.CacheCreation.Ephemeral1hInputTokens = tokens
	} else {
		adjusted.CacheCreation.Ephemeral5mInputTokens = tokens
	}
	if anthropic {
		adjusted.PromptTokens -= tokens
		adjusted.UsageSemantic = dto.BillingUsageSemanticAnthropic
	} else if adjusted.UsageSemantic == "" {
		adjusted.UsageSemantic = dto.BillingUsageSemanticOpenAI
	}
	return &adjusted, tokens
}

// RewriteResponseCacheWrite uses the same policy as settlement on public JSON
// and individual SSE payloads. Only usage fields are patched; provider extensions
// and content remain byte-preserved. It never emits internal simulation metadata.
func RewriteResponseCacheWrite(c *gin.Context, body []byte) ([]byte, error) {
	if !CacheWriteSimulationApplies(c) {
		return body, nil
	}
	settings, ok := GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	policy := settings.CacheWriteSimulation
	if !ok || policy == nil || !policy.Enabled || policy.Validate() != nil {
		return body, nil
	}
	for _, path := range []string{"usage", "message.usage", "response.usage"} {
		raw := gjson.GetBytes(body, path)
		if !raw.IsObject() {
			continue
		}
		var usage dto.Usage
		if err := UnmarshalJsonStr(raw.Raw, &usage); err != nil {
			return nil, err
		}
		anthropic := raw.Get("cache_read_input_tokens").Exists() || raw.Get("cache_creation_input_tokens").Exists() ||
			strings.HasSuffix(c.Request.URL.Path, "/messages") && !raw.Get("prompt_tokens").Exists() && path != "response.usage"
		if anthropic {
			usage.PromptTokens = int(raw.Get("input_tokens").Int())
			usage.PromptTokensDetails.CachedTokens = int(raw.Get("cache_read_input_tokens").Int())
			usage.PromptTokensDetails.CachedCreationTokens = int(raw.Get("cache_creation_input_tokens").Int())
		} else {
			usage.FillOpenAIUsageAliases(types.RelayFormatOpenAIResponses)
			if usage.PromptTokensDetails.CachedTokens == 0 {
				usage.PromptTokensDetails.CachedTokens = max(usage.CachedTokens, usage.PromptCacheHitTokens)
			}
		}
		if total, _, _ := usage.CacheCreationTokenCounts(); total > 0 {
			continue
		}
		sourceUsage, sourceAnthropic := &usage, anthropic
		if usage.BillingUsage != nil {
			if canonical, ok := usage.BillingUsage.CanonicalUsage(); ok {
				sourceUsage = canonical
				sourceAnthropic = canonical.UsageSemantic == dto.BillingUsageSemanticAnthropic
			}
		}
		adjusted, tokens := ApplyCacheWriteSimulation(CacheCreationUsageFromHeaders(c, sourceUsage), policy, sourceAnthropic)
		if tokens == 0 {
			continue
		}
		cacheCreation, err := Marshal(adjusted.CacheCreation)
		if err != nil {
			return nil, err
		}
		body, err = sjson.SetRawBytes(body, path+".cache_creation", cacheCreation)
		if err != nil {
			return nil, err
		}
		changes := make(map[string]int, 2)
		if anthropic {
			changes[path+".input_tokens"] = max(usage.PromptTokens-tokens, 0)
			changes[path+".cache_creation_input_tokens"] = tokens
		} else {
			detailName := "prompt_tokens_details"
			if raw.Get("input_tokens").Exists() && !raw.Get("prompt_tokens").Exists() {
				detailName = "input_tokens_details"
			}
			changes[path+"."+detailName+".cache_write_tokens"] = tokens
		}
		for field, value := range changes {
			var err error
			body, err = sjson.SetBytes(body, field, value)
			if err != nil {
				return nil, err
			}
		}
	}
	return body, nil
}
