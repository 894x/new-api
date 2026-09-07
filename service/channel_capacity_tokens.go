package service

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

const defaultChannelModelCapacityOutputTokens int64 = 8192

// One above the largest limit makes oversized reservations fail closed.
const maximumCapacityReservation = model.MaxChannelModelRateLimit + 1

// EstimateFinalChannelModelCapacityTokens counts the fully transformed body
// that is about to be sent upstream. This keeps admission aligned with channel
// system prompts, format conversion, request policies, and handler defaults.
func EstimateFinalChannelModelCapacityTokens(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	body common.ReplayableBody,
	fallback int64,
) (int64, error) {
	fallback = conservativeChannelModelCapacityReservation(fallback, 0)
	if info == nil || body == nil {
		return fallback, nil
	}
	reader, err := body.NewReader()
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return 0, err
	}
	finalBodyScan := scanFinalChannelModelCapacityJSON(data, info)
	promptEstimate := finalChannelModelCapacityJSONPromptTokens(finalBodyScan, info)
	providerReservation, providerOutputLimit := estimateProviderSpecificChannelModelCapacityReservation(finalBodyScan, info)
	initialPromptTokens := int64(info.GetEstimatePromptTokens())
	if value, exists := c.Get(channelCapacityContextKey); exists {
		if param, ok := value.(*RetryParam); ok && param.Capacity != nil {
			initialPromptTokens = param.Capacity.PromptTokens
		}
	}
	fallback = conservativeChannelModelCapacityReservation(
		fallback,
		providerReservation,
	)

	switch info.GetFinalRequestRelayFormat() {
	case relaytypes.RelayFormatOpenAI:
		request := &dto.GeneralOpenAIRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		promptTokens, err := estimateFinalRequestPromptTokens(c, request, info)
		if err != nil {
			return 0, err
		}
		outputLimit := openAIChannelCapacityOutputLimit(request)
		if outputLimit == nil {
			outputLimit = providerOutputLimit
		}
		choices := int64(1)
		if request.N != nil && *request.N > 1 {
			choices = int64(*request.N)
		}
		computed := finalChannelModelCapacityReservation(info, int64(promptTokens), outputLimit, choices)
		computed = addFinalChannelModelCapacityPromptSupplement(computed, int64(promptTokens), initialPromptTokens, promptEstimate)
		return conservativeChannelModelCapacityReservation(providerReservation, computed), nil
	case relaytypes.RelayFormatOpenAIResponses:
		request := &dto.OpenAIResponsesRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		promptTokens, err := estimateFinalRequestPromptTokens(c, request, info)
		if err != nil {
			return 0, err
		}
		var outputLimit *int64
		if request.MaxOutputTokens != nil {
			value := int64(min(*request.MaxOutputTokens, uint(maximumCapacityReservation)))
			outputLimit = &value
		}
		if outputLimit == nil {
			outputLimit = providerOutputLimit
		}
		computed := finalChannelModelCapacityReservation(info, int64(promptTokens), outputLimit, 1)
		computed = addFinalChannelModelCapacityPromptSupplement(computed, int64(promptTokens), initialPromptTokens, promptEstimate)
		return conservativeChannelModelCapacityReservation(providerReservation, computed), nil
	case relaytypes.RelayFormatOpenAIResponsesCompaction:
		request := &dto.OpenAIResponsesCompactionRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		promptTokens, err := estimateFinalRequestPromptTokens(c, request, info)
		if err != nil {
			return 0, err
		}
		computed := finalChannelModelCapacityReservation(info, int64(promptTokens), providerOutputLimit, 1)
		computed = addFinalChannelModelCapacityPromptSupplement(computed, int64(promptTokens), initialPromptTokens, promptEstimate)
		return conservativeChannelModelCapacityReservation(providerReservation, computed), nil
	case relaytypes.RelayFormatClaude:
		request := &dto.ClaudeRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		promptTokens, err := estimateFinalRequestPromptTokens(c, request, info)
		if err != nil {
			return 0, err
		}
		var outputLimit *int64
		if request.MaxTokens != nil || request.MaxTokensToSample != nil {
			value := int64(min(max(lo.FromPtrOr(request.MaxTokens, uint(0)), lo.FromPtrOr(request.MaxTokensToSample, uint(0))), uint(maximumCapacityReservation)))
			outputLimit = &value
		}
		if outputLimit == nil {
			outputLimit = providerOutputLimit
		}
		computed := finalChannelModelCapacityReservation(info, int64(promptTokens), outputLimit, 1)
		computed = addFinalChannelModelCapacityPromptSupplement(computed, int64(promptTokens), initialPromptTokens, promptEstimate)
		return conservativeChannelModelCapacityReservation(providerReservation, computed), nil
	case relaytypes.RelayFormatGemini:
		if info.RelayMode == relayconstant.RelayModeEmbeddings {
			batch := &dto.GeminiBatchEmbeddingRequest{}
			if err := common.Unmarshal(data, batch); err == nil && len(batch.Requests) > 0 {
				for _, request := range batch.Requests {
					if request == nil {
						return fallback, nil
					}
				}
				return estimateFinalPromptOnlyChannelModelCapacityTokens(c, info, batch, fallback, initialPromptTokens, promptEstimate)
			}
			request := &dto.GeminiEmbeddingRequest{}
			if err := common.Unmarshal(data, request); err != nil {
				return fallback, nil
			}
			return estimateFinalPromptOnlyChannelModelCapacityTokens(c, info, request, fallback, initialPromptTokens, promptEstimate)
		}
		request := &dto.GeminiChatRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		tokens, typedPromptTokens, err := estimateFinalGeminiChannelModelCapacityTokens(c, info, request)
		tokens = addFinalChannelModelCapacityPromptSupplement(tokens, typedPromptTokens, initialPromptTokens, promptEstimate)
		return conservativeChannelModelCapacityReservation(providerReservation, tokens), err
	case relaytypes.RelayFormatEmbedding:
		request := &dto.EmbeddingRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		return estimateFinalPromptOnlyChannelModelCapacityTokens(c, info, request, fallback, initialPromptTokens, promptEstimate)
	case relaytypes.RelayFormatRerank:
		request := &dto.RerankRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		return estimateFinalPromptOnlyChannelModelCapacityTokens(c, info, request, fallback, initialPromptTokens, promptEstimate)
	case relaytypes.RelayFormatOpenAIImage:
		request := &dto.ImageRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		return estimateFinalPromptOnlyChannelModelCapacityTokens(c, info, request, fallback, initialPromptTokens, promptEstimate)
	case relaytypes.RelayFormatOpenAIAudio:
		request := &dto.AudioRequest{}
		if err := common.Unmarshal(data, request); err != nil {
			return fallback, nil
		}
		return estimateFinalPromptOnlyChannelModelCapacityTokens(c, info, request, fallback, initialPromptTokens, promptEstimate)
	default:
		return fallback, nil
	}
}

func estimateFinalRequestPromptTokens(c *gin.Context, request dto.Request, info *relaycommon.RelayInfo) (int, error) {
	return estimateRequestTokenForCapacityFormat(c, request.GetTokenCountMeta(), info, info.GetFinalRequestRelayFormat())
}

func estimateFinalPromptOnlyChannelModelCapacityTokens(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	request dto.Request,
	fallback int64,
	initialPromptTokens int64,
	promptEstimate channelModelCapacityJSONPromptEstimate,
) (int64, error) {
	promptTokens, err := estimateFinalRequestPromptTokens(c, request, info)
	if err != nil {
		return 0, err
	}
	computed := addFinalChannelModelCapacityPromptSupplement(int64(promptTokens), int64(promptTokens), initialPromptTokens, promptEstimate)
	return conservativeChannelModelCapacityReservation(fallback, computed), nil
}

func openAIChannelCapacityOutputLimit(request *dto.GeneralOpenAIRequest) *int64 {
	if request == nil || (request.MaxTokens == nil && request.MaxCompletionTokens == nil) {
		return nil
	}
	value := int64(min(max(lo.FromPtrOr(request.MaxTokens, uint(0)), lo.FromPtrOr(request.MaxCompletionTokens, uint(0))), uint(maximumCapacityReservation)))
	return &value
}

func estimateFinalGeminiChannelModelCapacityTokens(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (int64, int64, error) {
	if request == nil {
		return 0, 0, nil
	}
	total := int64(0)
	promptTotal := int64(0)
	if len(request.Contents) > 0 || request.SystemInstructions != nil || len(request.Requests) == 0 {
		meta := request.GetTokenCountMeta()
		if request.SystemInstructions != nil {
			for _, part := range request.SystemInstructions.Parts {
				if part.Text != "" {
					if meta.CombineText != "" {
						meta.CombineText += "\n"
					}
					meta.CombineText += part.Text
				}
			}
		}
		capacityTexts := make([]string, 0, 6)
		if meta.CombineText != "" {
			capacityTexts = append(capacityTexts, meta.CombineText)
		}
		if len(request.Tools) > 0 && string(request.Tools) != "null" {
			capacityTexts = append(capacityTexts, string(request.Tools))
		}
		if request.ToolConfig != nil {
			toolConfig, err := common.Marshal(request.ToolConfig)
			if err != nil {
				return 0, 0, err
			}
			capacityTexts = append(capacityTexts, string(toolConfig))
		}
		if request.GenerationConfig.ResponseSchema != nil {
			responseSchema, err := common.Marshal(request.GenerationConfig.ResponseSchema)
			if err != nil {
				return 0, 0, err
			}
			capacityTexts = append(capacityTexts, string(responseSchema))
		}
		if len(request.GenerationConfig.ResponseJsonSchema) > 0 && string(request.GenerationConfig.ResponseJsonSchema) != "null" {
			capacityTexts = append(capacityTexts, string(request.GenerationConfig.ResponseJsonSchema))
		}
		if request.CachedContent != "" {
			capacityTexts = append(capacityTexts, request.CachedContent)
		}
		meta.CombineText = strings.Join(capacityTexts, "\n")
		promptTokens, err := estimateRequestTokenForCapacityFormat(c, meta, info, relaytypes.RelayFormatGemini)
		if err != nil {
			return 0, 0, err
		}
		var outputLimit *int64
		if request.GenerationConfig.MaxOutputTokens != nil {
			value := int64(*request.GenerationConfig.MaxOutputTokens)
			outputLimit = &value
		}
		candidates := int64(1)
		if request.GenerationConfig.CandidateCount != nil && *request.GenerationConfig.CandidateCount > 1 {
			candidates = int64(*request.GenerationConfig.CandidateCount)
		}
		promptTotal = saturatingChannelModelCapacityAdd(promptTotal, int64(promptTokens))
		total = finalChannelModelCapacityReservation(info, int64(promptTokens), outputLimit, candidates)
	}
	for i := range request.Requests {
		requestTokens, requestPromptTokens, err := estimateFinalGeminiChannelModelCapacityTokens(c, info, &request.Requests[i])
		if err != nil {
			return 0, 0, err
		}
		total = saturatingChannelModelCapacityAdd(total, requestTokens)
		promptTotal = saturatingChannelModelCapacityAdd(promptTotal, requestPromptTokens)
	}
	return total, promptTotal, nil
}

func finalChannelModelCapacityReservation(info *relaycommon.RelayInfo, promptTokens int64, outputLimit *int64, outputCount int64) int64 {
	promptTokens = max(promptTokens, 0)
	if !isChannelModelCapacityGenerationRequest(info) {
		return min(promptTokens, maximumCapacityReservation)
	}
	outputTokens := defaultChannelModelCapacityOutputTokens
	if outputLimit != nil {
		outputTokens = max(*outputLimit, 0)
	}
	outputCount = max(outputCount, 1)
	if outputTokens > 0 && outputCount > maximumCapacityReservation/outputTokens {
		outputTokens = maximumCapacityReservation
	} else {
		outputTokens *= outputCount
	}
	return saturatingChannelModelCapacityAdd(promptTokens, outputTokens)
}

func saturatingChannelModelCapacityAdd(left int64, right int64) int64 {
	left = max(left, 0)
	right = max(right, 0)
	if left >= maximumCapacityReservation || right > maximumCapacityReservation-left {
		return maximumCapacityReservation
	}
	return left + right
}

func conservativeChannelModelCapacityReservation(fallback int64, computed int64) int64 {
	fallback = min(max(fallback, 0), maximumCapacityReservation)
	computed = min(max(computed, 0), maximumCapacityReservation)
	return max(fallback, computed)
}

type channelModelCapacityJSONPromptEstimate struct {
	full   int64
	opaque int64
}

func addFinalChannelModelCapacityPromptSupplement(
	total int64,
	typedPrompt int64,
	initialPrompt int64,
	estimate channelModelCapacityJSONPromptEstimate,
) int64 {
	typedPrompt = min(max(typedPrompt, 0), maximumCapacityReservation)
	initialPrompt = min(max(initialPrompt, 0), maximumCapacityReservation)
	estimate.full = min(max(estimate.full, 0), maximumCapacityReservation)
	estimate.opaque = min(max(estimate.opaque, 0), maximumCapacityReservation)
	supplement := estimate.opaque
	if typedPrompt == 0 {
		supplement = max(supplement, estimate.full)
	}
	knownPrompt := saturatingChannelModelCapacityAdd(typedPrompt, supplement)
	total = saturatingChannelModelCapacityAdd(total, supplement)
	total = max(total, estimate.full)
	if initialPrompt > knownPrompt {
		total = saturatingChannelModelCapacityAdd(total, initialPrompt-knownPrompt)
	}
	return total
}

type providerSpecificCapacityJSONScan struct {
	hasOutputLimit bool
	outputTokens   int64
	promptTexts    []string
	promptTokenIDs int64
	opaqueTexts    []string
	opaqueTokenIDs int64
}

func estimateProviderSpecificChannelModelCapacityReservation(
	scan *providerSpecificCapacityJSONScan,
	info *relaycommon.RelayInfo,
) (int64, *int64) {
	if info == nil || !isChannelModelCapacityGenerationRequest(info) || scan == nil || !scan.hasOutputLimit {
		return 0, nil
	}
	promptTokens := finalChannelModelCapacityJSONPromptTokens(scan, info).full
	outputLimit := scan.outputTokens
	return saturatingChannelModelCapacityAdd(promptTokens, outputLimit), &outputLimit
}

func estimateFinalChannelModelCapacityJSONPromptTokens(data []byte, info *relaycommon.RelayInfo) channelModelCapacityJSONPromptEstimate {
	if info == nil {
		return channelModelCapacityJSONPromptEstimate{}
	}
	return finalChannelModelCapacityJSONPromptTokens(scanFinalChannelModelCapacityJSON(data, info), info)
}

func scanFinalChannelModelCapacityJSON(data []byte, info *relaycommon.RelayInfo) *providerSpecificCapacityJSONScan {
	scan := &providerSpecificCapacityJSONScan{}
	var value any
	if err := common.Unmarshal(data, &value); err != nil {
		return scan
	}
	scanProviderSpecificCapacityJSON(value, nil, "", info, scan)
	return scan
}

func finalChannelModelCapacityJSONPromptTokens(
	scan *providerSpecificCapacityJSONScan,
	info *relaycommon.RelayInfo,
) channelModelCapacityJSONPromptEstimate {
	if scan == nil || info == nil {
		return channelModelCapacityJSONPromptEstimate{}
	}
	textTokens := int64(CountTextToken(strings.Join(scan.promptTexts, "\n"), info.OriginModelName))
	opaqueTokens := int64(CountTextToken(strings.Join(scan.opaqueTexts, "\n"), info.OriginModelName))
	return channelModelCapacityJSONPromptEstimate{
		full:   saturatingChannelModelCapacityAdd(textTokens, scan.promptTokenIDs),
		opaque: saturatingChannelModelCapacityAdd(opaqueTokens, scan.opaqueTokenIDs),
	}
}

func scanProviderSpecificCapacityJSON(
	value any,
	path []string,
	rawKey string,
	info *relaycommon.RelayInfo,
	scan *providerSpecificCapacityJSONScan,
) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			normalizedKey := normalizeChannelModelCapacityJSONKey(key)
			nextPath := make([]string, len(path)+1)
			copy(nextPath, path)
			nextPath[len(path)] = normalizedKey
			child := typed[key]
			if isProviderSpecificOutputTokenPath(nextPath, key) {
				if outputTokens, ok := channelModelCapacityJSONLimit(child); ok {
					scan.hasOutputLimit = true
					scan.outputTokens = saturatingChannelModelCapacityAdd(scan.outputTokens, outputTokens)
				}
			}
			if isChannelModelCapacitySchemaKey(normalizedKey) {
				if schema, err := common.Marshal(child); err == nil {
					scan.promptTexts = append(scan.promptTexts, string(schema))
					if isOpaqueChannelModelCapacitySchemaKey(normalizedKey) {
						scan.opaqueTexts = append(scan.opaqueTexts, string(schema))
					}
				}
				continue
			}
			scanProviderSpecificCapacityJSON(child, nextPath, key, info, scan)
		}
	case []any:
		for _, child := range typed {
			scanProviderSpecificCapacityJSON(child, path, rawKey, info, scan)
		}
	case string:
		if len(path) > 0 && isChannelModelCapacityPromptTextKey(path[len(path)-1]) && !strings.HasPrefix(typed, "data:") {
			scan.promptTexts = append(scan.promptTexts, typed)
			if isOpaqueChannelModelCapacityPromptTextKey(path[len(path)-1], info) {
				scan.opaqueTexts = append(scan.opaqueTexts, typed)
			}
		}
	case float64:
		if len(path) > 0 && isChannelModelCapacityNumericTokenInputKey(path[len(path)-1]) {
			scan.promptTokenIDs = saturatingChannelModelCapacityAdd(scan.promptTokenIDs, 1)
			scan.opaqueTokenIDs = saturatingChannelModelCapacityAdd(scan.opaqueTokenIDs, 1)
		}
	}
}

func normalizeChannelModelCapacityJSONKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "_", "")
	return strings.ReplaceAll(key, "-", "")
}

func isProviderSpecificOutputTokenPath(path []string, rawKey string) bool {
	if len(path) == 0 || !isChannelModelCapacityOutputTokenKey(path[len(path)-1]) {
		return false
	}
	if len(path) == 1 {
		switch rawKey {
		case "max_tokens", "max_completion_tokens", "max_output_tokens", "max_tokens_to_sample":
			return false
		default:
			return true
		}
	}
	parent := path[len(path)-2]
	switch parent {
	case "inferenceconfig", "generationconfig", "options", "config", "parameters":
		return true
	case "chat":
		return len(path) >= 3 && path[len(path)-3] == "parameter"
	default:
		return false
	}
}

func isChannelModelCapacityOutputTokenKey(key string) bool {
	switch key {
	case "maxtokens", "maxcompletiontokens", "maxoutputtokens", "maxtokenstosample",
		"maxnewtokens", "maxoutputtokencount", "numpredict":
		return true
	default:
		return false
	}
}

func channelModelCapacityJSONLimit(value any) (int64, bool) {
	number, err := strconv.ParseFloat(fmt.Sprint(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0, false
	}
	if number >= float64(maximumCapacityReservation) {
		return maximumCapacityReservation, true
	}
	return int64(math.Ceil(number)), true
}

func isChannelModelCapacitySchemaKey(key string) bool {
	switch key {
	case "tools", "toolconfig", "functions", "functiondeclarations", "responseschema", "responsejsonschema",
		"responseformat", "jsonschema", "toolcalls", "functioncall", "functionresponse", "functioncalloutput",
		"args", "arguments", "response", "output", "executablecode", "codeexecutionresult":
		return true
	default:
		return false
	}
}

func isOpaqueChannelModelCapacitySchemaKey(key string) bool {
	switch key {
	case "functions", "responseformat", "toolcalls", "functioncall", "functionresponse", "functioncalloutput",
		"args", "arguments", "response", "output", "executablecode", "codeexecutionresult":
		return true
	default:
		return false
	}
}

func isChannelModelCapacityPromptTextKey(key string) bool {
	switch key {
	case "text", "content", "prompt", "input", "inputs", "instruction", "instructions",
		"system", "description", "query", "document", "documents", "title", "message", "name",
		"reasoning", "reasoningcontent", "toolcallid", "prefix", "suffix", "refusal":
		return true
	default:
		return false
	}
}

func isOpaqueChannelModelCapacityPromptTextKey(key string, info *relaycommon.RelayInfo) bool {
	switch key {
	case "reasoning", "reasoningcontent", "toolcallid", "prefix", "suffix", "refusal", "title":
		return true
	case "instruction":
		return info != nil && info.GetFinalRequestRelayFormat() == relaytypes.RelayFormatOpenAI
	case "instructions":
		return info != nil && info.GetFinalRequestRelayFormat() == relaytypes.RelayFormatOpenAIAudio
	default:
		return false
	}
}

func isChannelModelCapacityNumericTokenInputKey(key string) bool {
	switch key {
	case "prompt", "input", "inputs":
		return true
	default:
		return false
	}
}

func isChannelModelCapacityGenerationRequest(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	switch info.GetFinalRequestRelayFormat() {
	case relaytypes.RelayFormatClaude, relaytypes.RelayFormatOpenAIResponses,
		relaytypes.RelayFormatOpenAIResponsesCompaction:
		return true
	case relaytypes.RelayFormatGemini:
		return !strings.Contains(strings.ToLower(info.RequestURLPath), "embed")
	case relaytypes.RelayFormatOpenAI:
		switch info.RelayMode {
		case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeCompletions, relayconstant.RelayModeEdits:
			return true
		}
	}
	return false
}
