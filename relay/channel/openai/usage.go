package openai

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func isKimiK3Model(model string) bool {
	model = strings.ToLower(model)
	return model == "kimi-k3" || strings.HasPrefix(model, "kimi-k3-")
}

// Normalize only the K3 compatibility case, never arbitrary hidden reasoning.
func shouldNormalizeEmptyK3ReasoningUsage(model string, hasReasoningContent bool, body []byte) bool {
	if hasReasoningContent || !isKimiK3Model(model) {
		return false
	}
	found := false
	for _, details := range []string{"completion_tokens_details", "output_tokens_details"} {
		tokens := gjson.GetBytes(body, "usage."+details+".reasoning_tokens")
		if !tokens.Exists() {
			continue
		}
		// Conflicting aliases and counts other than exactly one stay untouched.
		if tokens.Type != gjson.Number || tokens.Float() != 1 {
			return false
		}
		found = true
	}
	return found
}

// Call only after the original provider usage satisfies the K3 rule. Checking
// the formatted body instead would mistake generated zero aliases for conflicts.
func clearClientReasoningUsage(body []byte, forceFormat bool) ([]byte, error) {
	if forceFormat {
		// Fill aliases before clearing, so the existing reasoning-only detail
		// object remains present in both families. This is a client copy only.
		var response struct {
			Usage dto.Usage `json:"usage"`
		}
		if err := common.Unmarshal(body, &response); err != nil {
			return nil, err
		}
		usage, err := common.Marshal(clientUsageWithAliases(response.Usage, types.RelayFormatOpenAI))
		if err != nil {
			return nil, err
		}
		body, err = sjson.SetRawBytes(body, "usage", usage)
		if err != nil {
			return nil, err
		}
	}
	for _, details := range []string{"completion_tokens_details", "output_tokens_details"} {
		path := "usage." + details + ".reasoning_tokens"
		if !gjson.GetBytes(body, path).Exists() {
			continue
		}
		var err error
		body, err = sjson.SetBytes(body, path, 0)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

// Only usage snapshots are deferred, never text or tool deltas. The bounded
// buffer falls back to unchanged forwarding for oversized provider metadata.
type emptyK3ReasoningStreamUsage struct {
	model               string
	hasReasoningContent bool
	pending             []string
	pendingBytes        int
	passthrough         bool
}

func (s *emptyK3ReasoningStreamUsage) Observe(data string) {
	if s.model == "" {
		return
	}
	for _, choice := range gjson.Get(data, "choices").Array() {
		if choice.Get("delta.reasoning_content").String() != "" || choice.Get("delta.reasoning").String() != "" {
			s.hasReasoningContent = true
			return
		}
	}
}

func (s *emptyK3ReasoningStreamUsage) Prepare(data string) ([]string, error) {
	if s.passthrough || !gjson.Get(data, "usage").IsObject() ||
		(len(s.pending) == 0 && !shouldNormalizeEmptyK3ReasoningUsage(s.model, s.hasReasoningContent, common.StringToByteSlice(data))) {
		return []string{data}, nil
	}
	usageFrame := data
	hasChoices := len(gjson.Get(data, "choices").Array()) > 0
	if hasChoices {
		var err error
		usageFrame, err = sjson.SetRaw(data, "choices", "[]")
		if err != nil {
			return nil, err
		}
	}
	if len(usageFrame) > (64<<10)-s.pendingBytes {
		frames := append(s.pending, data)
		s.pending = nil
		s.pendingBytes = 0
		s.passthrough = true
		return frames, nil
	}
	s.pending = append(s.pending, usageFrame)
	s.pendingBytes += len(usageFrame)
	if !hasChoices {
		return nil, nil
	}
	// Deliver choices once, immediately, while waiting to classify the usage.
	data, err := sjson.Delete(data, "usage")
	if err != nil {
		return nil, err
	}
	return []string{data}, nil
}

func (s *emptyK3ReasoningStreamUsage) Flush(c *gin.Context, info *relaycommon.RelayInfo, normalEnd bool) error {
	normalize := normalEnd && len(s.pending) > 0 &&
		shouldNormalizeEmptyK3ReasoningUsage(s.model, s.hasReasoningContent, common.StringToByteSlice(s.pending[len(s.pending)-1]))
	for _, frame := range s.pending {
		if normalize && shouldNormalizeEmptyK3ReasoningUsage(s.model, false, common.StringToByteSlice(frame)) {
			body, err := clearClientReasoningUsage([]byte(frame), info.ChannelSetting.ForceFormat)
			if err != nil {
				return err
			}
			frame = string(body)
		}
		if err := sendStreamData(c, info, frame, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent); err != nil {
			return err
		}
	}
	s.pending = nil
	s.pendingBytes = 0
	return nil
}

func clientUsageWithAliases(usage dto.Usage, source types.RelayFormat) dto.Usage {
	usage.FillOpenAIUsageAliases(source)
	if usage.PromptTokensDetails.CachedCreationTokens == 0 && usage.PromptTokensDetails.CacheWriteTokens > 0 {
		usage.PromptTokensDetails.CachedCreationTokens = usage.PromptTokensDetails.CacheWriteTokens
	}
	if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedCreationTokens == 0 && usage.InputTokensDetails.CacheWriteTokens > 0 {
		// Detach the client details from the settlement usage before filling.
		details := usage.InputTokensDetails.Clone()
		details.CachedCreationTokens = details.CacheWriteTokens
		usage.InputTokensDetails = &details
	}
	return usage
}

func addChatUsageAliasesToResponsesBody(body []byte, path string, usage *dto.Usage) ([]byte, error) {
	if len(body) == 0 || usage == nil {
		return body, nil
	}

	clientUsage := clientUsageWithAliases(*usage, types.RelayFormatOpenAIResponses)
	var err error
	body, err = sjson.SetBytes(body, path+".prompt_tokens", clientUsage.PromptTokens)
	if err != nil {
		return nil, err
	}
	body, err = sjson.SetBytes(body, path+".completion_tokens", clientUsage.CompletionTokens)
	if err != nil {
		return nil, err
	}
	if usage.InputTokensDetails != nil && usage.PromptTokensDetails == (dto.InputTokenDetails{}) {
		details := gjson.GetBytes(body, path+".input_tokens_details")
		if details.Exists() {
			body, err = sjson.SetRawBytes(body, path+".prompt_tokens_details", []byte(details.Raw))
			if err != nil {
				return nil, err
			}
		}
	}
	if usage.OutputTokensDetails != nil && usage.CompletionTokenDetails == (dto.OutputTokenDetails{}) {
		details := gjson.GetBytes(body, path+".output_tokens_details")
		if details.Exists() {
			body, err = sjson.SetRawBytes(body, path+".completion_tokens_details", []byte(details.Raw))
			if err != nil {
				return nil, err
			}
		}
	}
	// Patch only the compatibility field so provider-specific details survive.
	for _, detailName := range []string{"input_tokens_details", "prompt_tokens_details"} {
		detailPath := path + "." + detailName
		details := gjson.GetBytes(body, detailPath)
		cacheWriteTokens := details.Get("cache_write_tokens").Int()
		if details.Get("cached_creation_tokens").Int() != 0 || cacheWriteTokens <= 0 {
			continue
		}
		body, err = sjson.SetBytes(body, detailPath+".cached_creation_tokens", cacheWriteTokens)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func applyUsagePostProcessing(info *relaycommon.RelayInfo, usage *dto.Usage, responseBody []byte) {
	if info == nil || usage == nil {
		return
	}

	switch info.ChannelType {
	case constant.ChannelTypeDeepSeek:
		if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptCacheHitTokens != 0 {
			usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
		}
	case constant.ChannelTypeZhipu_v4:
		// 智普的cached_tokens在标准位置: usage.prompt_tokens_details.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeMoonshot:
		// Moonshot的cached_tokens在非标准位置: choices[].usage.cached_tokens
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractMoonshotCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	case constant.ChannelTypeOpenAI:
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if cachedTokens, ok := extractLlamaCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			}
		}
	}
}

func addTopLevelCachedTokensToChatResponseBody(body []byte, usage *dto.Usage) ([]byte, error) {
	if len(body) == 0 || usage == nil {
		return body, nil
	}
	cachedTokens := usage.PromptTokensDetails.CachedTokens
	if cachedTokens <= 0 {
		return body, nil
	}

	usage.CachedTokens = cachedTokens
	return sjson.SetBytes(body, "usage.cached_tokens", cachedTokens)
}

func extractCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Usage struct {
			PromptTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CachedTokens         *int `json:"cached_tokens"`
			PromptCacheHitTokens *int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Usage.PromptTokensDetails.CachedTokens != nil {
		return *payload.Usage.PromptTokensDetails.CachedTokens, true
	}
	if payload.Usage.CachedTokens != nil {
		return *payload.Usage.CachedTokens, true
	}
	if payload.Usage.PromptCacheHitTokens != nil {
		return *payload.Usage.PromptCacheHitTokens, true
	}
	return 0, false
}

// extractMoonshotCachedTokensFromBody 从Moonshot的非标准位置提取cached_tokens
// Moonshot的流式响应格式: {"choices":[{"usage":{"cached_tokens":111}}]}
func extractMoonshotCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Choices []struct {
			Usage struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"usage"`
		} `json:"choices"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	// 遍历choices查找cached_tokens
	for _, choice := range payload.Choices {
		if choice.Usage.CachedTokens != nil && *choice.Usage.CachedTokens > 0 {
			return *choice.Usage.CachedTokens, true
		}
	}

	return 0, false
}

// extractLlamaCachedTokensFromBody 从llama.cpp的非标准位置提取cache_n
func extractLlamaCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Timings struct {
			CachedTokens *int `json:"cache_n"`
		} `json:"timings"`
	}

	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Timings.CachedTokens == nil {
		return 0, false
	}
	return *payload.Timings.CachedTokens, true
}
