package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapacity"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

const (
	channelCapacityRedisPrefix              = "channelModelCapacity:v1"
	defaultChannelModelCapacityOutputTokens = int64(8192)
	channelModelCapacityRetryParamKey       = "__channel_model_capacity_retry_param"
)

var channelCapacityMemoryLimiter channelcapacity.Limiter = channelcapacity.NewMemoryLimiter()
var channelCapacityNow = time.Now

type ChannelModelCapacityError struct {
	Model      string
	RetryAfter time.Duration
}

type ChannelModelCapacityAdmissionError struct {
	ChannelID  int
	Model      string
	RetryAfter time.Duration
}

func (e *ChannelModelCapacityAdmissionError) Error() string {
	return fmt.Sprintf("channel %d for model %s reached its RPM or TPM limit", e.ChannelID, e.Model)
}

type ChannelModelCapacityPlan struct {
	Enabled               bool
	RequiresTokenEstimate bool
}

// BindChannelModelCapacityRequest makes the request-scoped selector state
// available at the final outbound-body admission boundary.
func BindChannelModelCapacityRequest(param *RetryParam) {
	if param == nil || param.Ctx == nil {
		return
	}
	param.Ctx.Set(channelModelCapacityRetryParamKey, param)
}

// AdmitFinalChannelModelCapacity estimates the fully transformed outbound body
// when it is replayable, then performs the single atomic RPM/TPM acquisition.
func AdmitFinalChannelModelCapacity(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) error {
	if c == nil || info == nil {
		return nil
	}
	param, channelID, rpm, tpm, enabled, err := resolveFinalChannelModelCapacityState(c, info)
	if err != nil || !enabled {
		return err
	}
	tokens := int64(0)
	if param.CapacityTokens != nil {
		tokens = *param.CapacityTokens
	}
	if replayable, ok := body.(common.ReplayableBody); ok && tpm > 0 {
		tokens, err = EstimateFinalChannelModelCapacityTokens(c, info, replayable, tokens)
		if err != nil {
			return err
		}
	}
	return acquireResolvedFinalChannelModelCapacity(param, channelID, rpm, tpm, tokens)
}

// AcquireFinalChannelModelCapacity atomically admits the fully transformed
// request immediately before its physical upstream dispatch.
func AcquireFinalChannelModelCapacity(c *gin.Context, info *relaycommon.RelayInfo, tokens int64) error {
	if c == nil || info == nil {
		return nil
	}
	param, channelID, rpm, tpm, enabled, err := resolveFinalChannelModelCapacityState(c, info)
	if err != nil || !enabled {
		return err
	}
	return acquireResolvedFinalChannelModelCapacity(param, channelID, rpm, tpm, tokens)
}

func resolveFinalChannelModelCapacityState(
	c *gin.Context,
	info *relaycommon.RelayInfo,
) (*RetryParam, int, int64, int64, bool, error) {
	value, exists := c.Get(channelModelCapacityRetryParamKey)
	if !exists {
		return nil, 0, 0, 0, false, nil
	}
	param, ok := value.(*RetryParam)
	if !ok || param == nil {
		return nil, 0, 0, 0, false, errors.New("invalid channel-model capacity request state")
	}
	channelID := info.ChannelId
	if channelID <= 0 {
		channelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	}
	channel, err := model.CacheGetChannel(channelID)
	if err != nil {
		return nil, 0, 0, 0, false, err
	}
	rpm, tpm, err := model.ResolveChannelModelRateLimits(channel, param.ModelName)
	if err != nil {
		return nil, 0, 0, 0, false, err
	}
	return param, channelID, rpm, tpm, true, nil
}

func acquireResolvedFinalChannelModelCapacity(
	param *RetryParam,
	channelID int,
	rpm int64,
	tpm int64,
	tokens int64,
) error {
	param.markCapacityEligible(channelID)
	decision, err := acquireChannelCapacity(param, channelID, rpm, tpm, tokens)
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	param.markCapacityDenied(channelID, decision.RetryAfter)
	return &ChannelModelCapacityAdmissionError{
		ChannelID:  channelID,
		Model:      param.ModelName,
		RetryAfter: decision.RetryAfter,
	}
}

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
			value := int64(*request.MaxOutputTokens)
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
			value := int64(max(lo.FromPtrOr(request.MaxTokens, uint(0)), lo.FromPtrOr(request.MaxTokensToSample, uint(0))))
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
	value := int64(max(lo.FromPtrOr(request.MaxTokens, uint(0)), lo.FromPtrOr(request.MaxCompletionTokens, uint(0))))
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
		return min(promptTokens, model.MaxChannelModelRateLimit)
	}
	outputTokens := defaultChannelModelCapacityOutputTokens
	if outputLimit != nil {
		outputTokens = max(*outputLimit, 0)
	}
	outputCount = max(outputCount, 1)
	if outputTokens > 0 && outputCount > model.MaxChannelModelRateLimit/outputTokens {
		outputTokens = model.MaxChannelModelRateLimit
	} else {
		outputTokens *= outputCount
	}
	return saturatingChannelModelCapacityAdd(promptTokens, outputTokens)
}

func saturatingChannelModelCapacityAdd(left int64, right int64) int64 {
	left = max(left, 0)
	right = max(right, 0)
	if left >= model.MaxChannelModelRateLimit || right > model.MaxChannelModelRateLimit-left {
		return model.MaxChannelModelRateLimit
	}
	return left + right
}

func conservativeChannelModelCapacityReservation(fallback int64, computed int64) int64 {
	fallback = min(max(fallback, 0), model.MaxChannelModelRateLimit)
	computed = min(max(computed, 0), model.MaxChannelModelRateLimit)
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
	typedPrompt = min(max(typedPrompt, 0), model.MaxChannelModelRateLimit)
	initialPrompt = min(max(initialPrompt, 0), model.MaxChannelModelRateLimit)
	estimate.full = min(max(estimate.full, 0), model.MaxChannelModelRateLimit)
	estimate.opaque = min(max(estimate.opaque, 0), model.MaxChannelModelRateLimit)
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
	if number >= float64(model.MaxChannelModelRateLimit) {
		return model.MaxChannelModelRateLimit, true
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

func (e *ChannelModelCapacityError) Error() string {
	return fmt.Sprintf("all channels for model %s reached their RPM or TPM limit", e.Model)
}

func currentChannelCapacityLimiter() channelcapacity.Limiter {
	if common.RedisEnabled {
		return channelcapacity.NewRedisLimiter(common.RDB, channelCapacityRedisPrefix)
	}
	return channelCapacityMemoryLimiter
}

// ResolveChannelModelCapacityPlan reports whether the request's eligible
// channels have proactive capacity configured. Token counting is forced only
// when at least one effective TPM cap can participate in selection.
func ResolveChannelModelCapacityPlan(param *RetryParam) (ChannelModelCapacityPlan, error) {
	if param == nil || param.Ctx == nil {
		return ChannelModelCapacityPlan{}, errors.New("capacity planning requires a request context")
	}
	if _, forced := param.Ctx.Get("specific_channel_id"); forced {
		channelID := param.Ctx.GetInt("channel_id")
		channel, err := model.CacheGetChannel(channelID)
		if err != nil {
			return ChannelModelCapacityPlan{}, err
		}
		rpm, tpm, err := model.ResolveChannelModelRateLimits(channel, param.ModelName)
		if err != nil {
			return ChannelModelCapacityPlan{}, err
		}
		plan := ChannelModelCapacityPlan{
			Enabled:               rpm > 0 || tpm > 0,
			RequiresTokenEstimate: tpm > 0,
		}
		if plan.Enabled {
			param.markCapacityEligible(channelID)
		}
		return plan, nil
	}

	groups := []string{param.TokenGroup}
	if param.TokenGroup == "auto" {
		userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
		groups = GetRequestAutoGroups(param.Ctx, userGroup)
	}
	plan := ChannelModelCapacityPlan{}
	for _, group := range groups {
		candidates, err := model.ListSatisfiedChannelCandidatesWithFilters(
			group,
			param.ModelName,
			param.RequestPath,
			param.VideoResolution,
			param.AllowedChannelIds,
		)
		if err != nil {
			if param.TokenGroup == "auto" && errors.Is(err, model.ErrVideoResolutionUnsupported) {
				continue
			}
			return ChannelModelCapacityPlan{}, err
		}
		for _, candidate := range candidates {
			if candidate.RPM > 0 || candidate.TPM > 0 {
				plan.Enabled = true
			}
			if candidate.TPM > 0 {
				plan.RequiresTokenEstimate = true
			}
		}
	}
	return plan, nil
}

func (p *RetryParam) markCapacityDenied(channelID int, retryAfter time.Duration) {
	if p.CapacityBlockedChannelIds == nil {
		p.CapacityBlockedChannelIds = make(map[int]struct{})
	}
	p.CapacityBlockedChannelIds[channelID] = struct{}{}
	if retryAfter > 0 && (p.capacityRetryAfter == 0 || retryAfter < p.capacityRetryAfter) {
		p.capacityRetryAfter = retryAfter
	}
}

func (p *RetryParam) markCapacityEligible(channelID int) {
	if p.CapacityEligibleChannelIds == nil {
		p.CapacityEligibleChannelIds = make(map[int]struct{})
	}
	p.CapacityEligibleChannelIds[channelID] = struct{}{}
}

func (p *RetryParam) capacityError() error {
	if len(p.CapacityEligibleChannelIds) == 0 || len(p.CapacityBlockedChannelIds) == 0 {
		return nil
	}
	for channelID := range p.CapacityEligibleChannelIds {
		if _, blocked := p.CapacityBlockedChannelIds[channelID]; !blocked {
			return nil
		}
	}
	return &ChannelModelCapacityError{Model: p.ModelName, RetryAfter: p.capacityRetryAfter}
}

func (p *RetryParam) CapacityError() error {
	return p.capacityError()
}

func selectCapacitySatisfiedChannel(param *RetryParam, group string, retry int) (*model.Channel, bool, error) {
	eligible, err := model.ListSatisfiedChannelCandidatesWithFilters(
		group,
		param.ModelName,
		param.RequestPath,
		param.VideoResolution,
		param.AllowedChannelIds,
	)
	if err != nil || len(eligible) == 0 {
		return nil, false, err
	}
	for _, candidate := range eligible {
		param.markCapacityEligible(candidate.ChannelId)
	}

	allAttempted := true
	for _, candidate := range eligible {
		if !param.HasAttempted(candidate.ChannelId) {
			allAttempted = false
			break
		}
	}

	candidate, found, _, err := chooseCapacityCandidate(param, group, retry, eligible)
	if err != nil {
		return nil, allAttempted, err
	}
	if !found {
		return nil, allAttempted, nil
	}
	channel, err := model.CacheGetChannel(candidate.ChannelId)
	return channel, false, err
}

func acquireChannelCapacity(param *RetryParam, channelID int, rpm int64, tpm int64, tokens int64) (channelcapacity.Decision, error) {
	ctx := context.Background()
	if param.Ctx != nil && param.Ctx.Request != nil {
		ctx = param.Ctx.Request.Context()
	}
	return currentChannelCapacityLimiter().Acquire(
		ctx,
		channelcapacity.Key{ChannelID: channelID, Model: param.ModelName},
		channelcapacity.Limits{RPM: rpm, TPM: tpm},
		tokens,
		channelCapacityNow(),
	)
}

func chooseCapacityCandidate(
	param *RetryParam,
	group string,
	retry int,
	eligible []model.SatisfiedChannelCandidate,
) (model.SatisfiedChannelCandidate, bool, bool, error) {
	if param.DynamicRoutingEligible {
		controller, enabled, err := currentDynamicRoutingController()
		if err != nil {
			return model.SatisfiedChannelCandidate{}, false, true, err
		}
		if enabled {
			candidates := make([]dynamicrouting.Candidate, 0, len(eligible))
			for _, candidate := range eligible {
				candidates = append(candidates, dynamicrouting.Candidate{
					ChannelID: candidate.ChannelId,
					Priority:  candidate.Priority,
					Weight:    candidate.Weight,
				})
			}
			avoid := make(map[int]struct{}, len(param.AttemptedChannelIds)+len(param.CapacityBlockedChannelIds))
			for channelID := range param.AttemptedChannelIds {
				avoid[channelID] = struct{}{}
			}
			for channelID := range param.CapacityBlockedChannelIds {
				avoid[channelID] = struct{}{}
			}
			decision := controller.SelectAvoiding(dynamicrouting.RouteKey{
				Group: group,
				Model: param.ModelName,
			}, candidates, avoid, channelCapacityNow())
			if !decision.HasSelection {
				return model.SatisfiedChannelCandidate{}, false, true, nil
			}
			for _, candidate := range eligible {
				if candidate.ChannelId == decision.SelectedChannelID {
					return candidate, true, true, nil
				}
			}
			return model.SatisfiedChannelCandidate{}, false, true, fmt.Errorf("dynamic routing selected unknown channel %d", decision.SelectedChannelID)
		}
	}

	candidate, ok := selectStaticCapacityCandidate(eligible, retry, param.CapacityBlockedChannelIds)
	return candidate, ok, false, nil
}

func selectStaticCapacityCandidate(
	candidates []model.SatisfiedChannelCandidate,
	retry int,
	blocked map[int]struct{},
) (model.SatisfiedChannelCandidate, bool) {
	priorities := make([]int64, 0)
	seenPriorities := make(map[int64]struct{})
	for _, candidate := range candidates {
		if _, seen := seenPriorities[candidate.Priority]; !seen {
			seenPriorities[candidate.Priority] = struct{}{}
			priorities = append(priorities, candidate.Priority)
		}
	}
	if len(priorities) == 0 {
		return model.SatisfiedChannelCandidate{}, false
	}
	sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
	if retry < 0 {
		retry = 0
	}
	if retry >= len(priorities) {
		retry = len(priorities) - 1
	}
	for _, targetPriority := range priorities[retry:] {
		target := make([]model.SatisfiedChannelCandidate, 0, len(candidates))
		weightSum := 0
		for _, candidate := range candidates {
			if candidate.Priority != targetPriority {
				continue
			}
			if _, denied := blocked[candidate.ChannelId]; denied {
				continue
			}
			effectiveWeight := int(candidate.Weight) + 10
			if effectiveWeight <= 0 || weightSum > math.MaxInt-effectiveWeight {
				return candidate, true
			}
			weightSum += effectiveWeight
			target = append(target, candidate)
		}
		if len(target) == 0 {
			continue
		}
		draw := common.GetRandomInt(weightSum)
		for _, candidate := range target {
			draw -= int(candidate.Weight) + 10
			if draw < 0 {
				return candidate, true
			}
		}
		return target[len(target)-1], true
	}
	return model.SatisfiedChannelCandidate{}, false
}
