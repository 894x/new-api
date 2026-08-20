package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayparam"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

type ThinkingContentInfo struct {
	IsFirstThinkingContent  bool
	SendLastThinkingContent bool
	HasSentThinkingContent  bool
}

const (
	LastMessageTypeNone     = convmeta.LastMessageTypeNone
	LastMessageTypeText     = convmeta.LastMessageTypeText
	LastMessageTypeTools    = convmeta.LastMessageTypeTools
	LastMessageTypeThinking = convmeta.LastMessageTypeThinking
)

// ClaudeConvertInfo now lives with the converters (convmeta); the alias keeps
// host code and adaptors compiling unchanged.
type ClaudeConvertInfo = convmeta.ClaudeConvertInfo

type RerankerInfo struct {
	Documents       []any
	ReturnDocuments bool
}

type BuildInToolInfo struct {
	ToolName          string
	CallCount         int
	SearchContextSize string
}

type ResponsesUsageInfo struct {
	BuiltInTools map[string]*BuildInToolInfo
}

type ChannelMeta struct {
	ChannelType          int
	ChannelId            int
	ChannelIsMultiKey    bool
	ChannelMultiKeyIndex int
	ChannelBaseUrl       string
	ApiType              int
	ApiVersion           string
	ApiKey               string
	Organization         string
	ChannelCreateTime    int64
	ParamOverride        map[string]interface{}
	HeadersOverride      map[string]interface{}
	ChannelSetting       dto.ChannelSettings
	ChannelOtherSettings dto.ChannelOtherSettings
	UpstreamModelName    string
	IsModelMapped        bool
	SupportStreamOptions bool // 是否支持流式选项
}

type TokenCountMeta struct {
	//promptTokens int
	estimatePromptTokens int
}

// DynamicRoutingAttemptSample is an immutable observation of one upstream
// attempt. It deliberately carries the public model name: channel model
// mappings must not split observations for the same routed model.
type DynamicRoutingAttemptSample struct {
	ChannelID         int
	Model             string
	ObservedAt        time.Time
	UpstreamStartedAt time.Time
	FirstResponseAt   time.Time
	FirstContentAt    time.Time
	LastContentAt     time.Time
	TTFT              time.Duration
	TPOT              time.Duration
	HasTTFT           bool
	HasTPOT           bool
	TTFTInvalidated   bool
	TPOTInvalidated   bool
	Success           bool
	HardFailure       bool
	CompletionTokens  int
}

type dynamicRoutingAttempt struct {
	enabled             bool
	channelID           int
	channelType         int
	model               string
	reachedUpstream     bool
	preUpstreamHard     bool
	upstreamStarted     time.Time
	firstResponseAt     time.Time
	firstContentAt      time.Time
	lastContentAt       time.Time
	visibleTextParts    []string
	tTFTInvalid         bool
	tPOTInvalid         bool
	httpStatus          int
	completionTokens    int
	completionTokensSet bool
}

type RelayInfo struct {
	TokenId           int
	TokenKey          string
	TokenGroup        string
	UserId            int
	UsingGroup        string // 使用的分组，当auto跨分组重试时，会变动
	UserGroup         string // 用户所在分组
	TokenUnlimited    bool
	StartTime         time.Time
	FirstResponseTime time.Time
	isFirstResponse   bool
	attemptMu         sync.Mutex
	currentAttempt    dynamicRoutingAttempt
	attemptNow        func() time.Time
	//SendLastReasoningResponse bool
	IsStream               bool
	IsGeminiBatchEmbedding bool
	IsPlayground           bool
	UsePrice               bool
	RelayMode              int
	OriginModelName        string
	RequestURLPath         string
	RequestHeaders         map[string]string
	ShouldIncludeUsage     bool
	DisablePing            bool // 是否禁止向下游发送自定义 Ping
	ClientWs               *websocket.Conn
	TargetWs               *websocket.Conn
	InputAudioFormat       string
	OutputAudioFormat      string
	RealtimeTools          []dto.RealTimeTool
	IsFirstRequest         bool
	AudioUsage             bool
	ReasoningEffort        string
	UserSetting            dto.UserSetting
	UserEmail              string
	UserQuota              int
	RelayFormat            types.RelayFormat
	SendResponseCount      int
	ReceivedResponseCount  int
	FinalPreConsumedQuota  int // 最终预消耗的配额
	// ForcePreConsume 为 true 时禁用 BillingSession 的信任额度旁路，
	// 强制预扣全额。用于异步任务（视频/音乐生成等），因为请求返回后任务仍在运行，
	// 必须在提交前锁定全额。
	ForcePreConsume bool
	// Billing 是计费会话，封装了预扣费/结算/退款的统一生命周期。
	// 初始免费组可为 nil；若 auto 重试切换到付费组，会在发送前创建。
	Billing BillingSettler
	// BillingSource indicates whether this request is billed from wallet quota or subscription.
	// "" or "wallet" => wallet; "subscription" => subscription
	BillingSource string
	// SubscriptionId is the user_subscriptions.id used when BillingSource == "subscription"
	SubscriptionId int
	// SubscriptionPreConsumed is the amount pre-consumed on subscription item (quota units or 1)
	SubscriptionPreConsumed int64
	// SubscriptionPostDelta is the post-consume delta applied to amount_used (quota units; can be negative).
	SubscriptionPostDelta int64
	// SubscriptionPlanId / SubscriptionPlanTitle are used for logging/UI display.
	SubscriptionPlanId    int
	SubscriptionPlanTitle string
	// RequestId is used for idempotent pre-consume/refund
	RequestId string
	// SubscriptionAmountTotal / SubscriptionAmountUsedAfterPreConsume are used to compute remaining in logs.
	SubscriptionAmountTotal               int64
	SubscriptionAmountUsedAfterPreConsume int64
	IsClaudeBetaQuery                     bool // /v1/messages?beta=true
	IsChannelTest                         bool // channel test request
	RetryIndex                            int
	LastError                             *types.NewAPIError
	RuntimeHeadersOverride                map[string]interface{}
	UseRuntimeHeadersOverride             bool
	ParamOverrideAudit                    []string
	ParameterCapabilityAudit              []relayparam.CapabilityChange

	PriceData hosttypes.PriceData

	// QuotaClamp is set (non-nil) when a quota conversion saturated at the
	// int32 bound (or NaN fallback) while computing this request's charge.
	// It is surfaced onto the consume/task log's admin_info for auditing.
	QuotaClamp *common.QuotaClamp

	// TieredBillingSnapshot captures tiered billing rules at pre-consume time.
	// Auto-group retries refresh its group-dependent fields before each attempt
	// and again before settlement. Non-nil only when billing mode is "tiered_expr".
	TieredBillingSnapshot *billingexpr.BillingSnapshot
	BillingRequestInput   *billingexpr.RequestInput

	Request dto.Request

	// RequestConversionChain records request format conversions in order, e.g.
	// ["openai", "openai_responses"] or ["openai", "claude"].
	RequestConversionChain []types.RelayFormat
	// 最终请求到上游的格式。可由 adaptor 显式设置；
	// 若为空，调用 GetFinalRequestRelayFormat 会回退到 RequestConversionChain 的最后一项或 RelayFormat。
	FinalRequestRelayFormat types.RelayFormat

	StreamStatus *StreamStatus

	// convOptions caches the converter settings snapshot (see ConvOptions).
	convOptions *convmeta.Options

	ThinkingContentInfo
	TokenCountMeta
	*ClaudeConvertInfo
	*RerankerInfo
	*ResponsesUsageInfo
	*ChannelMeta
	*TaskRelayInfo
}

func (info *RelayInfo) InitChannelMeta(c *gin.Context) {
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	paramOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)
	headerOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelHeaderOverride)
	apiType, _ := common.ChannelType2APIType(channelType)
	channelMeta := &ChannelMeta{
		ChannelType:          channelType,
		ChannelId:            common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		ChannelIsMultiKey:    common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey),
		ChannelMultiKeyIndex: common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex),
		ChannelBaseUrl:       common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl),
		ApiType:              apiType,
		ApiVersion:           c.GetString("api_version"),
		ApiKey:               common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		Organization:         c.GetString("channel_organization"),
		ChannelCreateTime:    c.GetInt64("channel_create_time"),
		ParamOverride:        paramOverride,
		HeadersOverride:      headerOverride,
		UpstreamModelName:    common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		IsModelMapped:        false,
		SupportStreamOptions: false,
	}

	if channelType == constant.ChannelTypeAzure {
		channelMeta.ApiVersion = GetAPIVersion(c)
	}
	if channelType == constant.ChannelTypeVertexAi {
		channelMeta.ApiVersion = c.GetString("region")
	}

	channelSetting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	if ok {
		channelMeta.ChannelSetting = channelSetting
	}

	channelOtherSettings, ok := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	if ok {
		channelMeta.ChannelOtherSettings = channelOtherSettings
	}

	if streamSupportedChannels[channelMeta.ChannelType] {
		channelMeta.SupportStreamOptions = true
	}

	info.ChannelMeta = channelMeta

	// Channel identity feeds the converter options snapshot (e.g.
	// OpenRouterDialect); drop the cache so a cross-channel retry rebuilds it.
	info.convOptions = nil

	// reset some fields based on channel meta
	// 重置某些字段，例如模型名称等
	if info.Request != nil {
		info.Request.SetModelName(info.OriginModelName)
	}
}

func (info *RelayInfo) ToString() string {
	if info == nil {
		return "RelayInfo<nil>"
	}

	// Basic info
	b := &strings.Builder{}
	fmt.Fprintf(b, "RelayInfo{ ")
	fmt.Fprintf(b, "RelayFormat: %s, ", info.RelayFormat)
	fmt.Fprintf(b, "RelayMode: %d, ", info.RelayMode)
	fmt.Fprintf(b, "IsStream: %t, ", info.IsStream)
	fmt.Fprintf(b, "IsPlayground: %t, ", info.IsPlayground)
	fmt.Fprintf(b, "RequestURLPath: %q, ", info.RequestURLPath)
	fmt.Fprintf(b, "OriginModelName: %q, ", info.OriginModelName)
	fmt.Fprintf(b, "EstimatePromptTokens: %d, ", info.estimatePromptTokens)
	fmt.Fprintf(b, "ShouldIncludeUsage: %t, ", info.ShouldIncludeUsage)
	fmt.Fprintf(b, "DisablePing: %t, ", info.DisablePing)
	fmt.Fprintf(b, "SendResponseCount: %d, ", info.SendResponseCount)
	fmt.Fprintf(b, "FinalPreConsumedQuota: %d, ", info.FinalPreConsumedQuota)

	// User & token info (mask secrets)
	fmt.Fprintf(b, "User{ Id: %d, Email: %q, Group: %q, UsingGroup: %q, Quota: %d }, ",
		info.UserId, common.MaskEmail(info.UserEmail), info.UserGroup, info.UsingGroup, info.UserQuota)
	fmt.Fprintf(b, "Token{ Id: %d, Unlimited: %t, Key: ***masked*** }, ", info.TokenId, info.TokenUnlimited)

	// Time info
	latencyMs := info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	fmt.Fprintf(b, "Timing{ Start: %s, FirstResponse: %s, LatencyMs: %d }, ",
		info.StartTime.Format(time.RFC3339Nano), info.FirstResponseTime.Format(time.RFC3339Nano), latencyMs)

	// Audio / realtime
	if info.InputAudioFormat != "" || info.OutputAudioFormat != "" || len(info.RealtimeTools) > 0 || info.AudioUsage {
		fmt.Fprintf(b, "Realtime{ AudioUsage: %t, InFmt: %q, OutFmt: %q, Tools: %d }, ",
			info.AudioUsage, info.InputAudioFormat, info.OutputAudioFormat, len(info.RealtimeTools))
	}

	// Reasoning
	if info.ReasoningEffort != "" {
		fmt.Fprintf(b, "ReasoningEffort: %q, ", info.ReasoningEffort)
	}

	// Price data (non-sensitive)
	if info.PriceData.UsePrice {
		fmt.Fprintf(b, "PriceData{ %s }, ", info.PriceData.ToSetting())
	}

	// Channel metadata (mask ApiKey)
	if info.ChannelMeta != nil {
		cm := info.ChannelMeta
		fmt.Fprintf(b, "ChannelMeta{ Type: %d, Id: %d, IsMultiKey: %t, MultiKeyIndex: %d, BaseURL: %q, ApiType: %d, ApiVersion: %q, Organization: %q, CreateTime: %d, UpstreamModelName: %q, IsModelMapped: %t, SupportStreamOptions: %t, ApiKey: ***masked*** }, ",
			cm.ChannelType, cm.ChannelId, cm.ChannelIsMultiKey, cm.ChannelMultiKeyIndex, cm.ChannelBaseUrl, cm.ApiType, cm.ApiVersion, cm.Organization, cm.ChannelCreateTime, cm.UpstreamModelName, cm.IsModelMapped, cm.SupportStreamOptions)
	}

	// Responses usage info (non-sensitive)
	if info.ResponsesUsageInfo != nil && len(info.ResponsesUsageInfo.BuiltInTools) > 0 {
		fmt.Fprintf(b, "ResponsesTools{ ")
		first := true
		for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
			if !first {
				fmt.Fprintf(b, ", ")
			}
			first = false
			if tool != nil {
				fmt.Fprintf(b, "%s: calls=%d", name, tool.CallCount)
			} else {
				fmt.Fprintf(b, "%s: calls=0", name)
			}
		}
		fmt.Fprintf(b, " }, ")
	}

	fmt.Fprintf(b, "}")
	return b.String()
}

// 定义支持流式选项的通道类型
var streamSupportedChannels = map[int]bool{
	constant.ChannelTypeOpenAI:          true,
	constant.ChannelTypeAnthropic:       true,
	constant.ChannelTypeAws:             true,
	constant.ChannelTypeGemini:          true,
	constant.ChannelTypeAstraFlowGemini: true,
	constant.ChannelCloudflare:          true,
	constant.ChannelTypeAzure:           true,
	constant.ChannelTypeVolcEngine:      true,
	constant.ChannelTypeOllama:          true,
	constant.ChannelTypeXai:             true,
	constant.ChannelTypeDeepSeek:        true,
	constant.ChannelTypeBaiduV2:         true,
	constant.ChannelTypeZhipu_v4:        true,
	constant.ChannelTypeAli:             true,
	constant.ChannelTypeSubmodel:        true,
	constant.ChannelTypeCodex:           true,
	constant.ChannelTypeMoonshot:        true,
	constant.ChannelTypeMiniMax:         true,
	constant.ChannelTypeSiliconFlow:     true,
	constant.ChannelTypeAdvancedCustom:  true,
	constant.ChannelTypeSub2API:         true,
	constant.ChannelTypeNewAPI:          true,
	constant.ChannelTypeTencent:         true,
	constant.ChannelTypeXunfeiMaaS:      true,
}

func GenRelayInfoWs(c *gin.Context, ws *websocket.Conn) *RelayInfo {
	info := genBaseRelayInfo(c, nil)
	info.RelayFormat = types.RelayFormatOpenAIRealtime
	info.ClientWs = ws
	info.InputAudioFormat = "pcm16"
	info.OutputAudioFormat = "pcm16"
	info.IsFirstRequest = true
	return info
}

func GenRelayInfoClaude(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatClaude
	info.ShouldIncludeUsage = false
	info.ClaudeConvertInfo = &ClaudeConvertInfo{
		LastMessagesType: LastMessageTypeNone,
	}
	info.IsClaudeBetaQuery = c.Query("beta") == "true"
	return info
}

func GenRelayInfoRerank(c *gin.Context, request *dto.RerankRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayMode = relayconstant.RelayModeRerank
	info.RelayFormat = types.RelayFormatRerank
	info.RerankerInfo = &RerankerInfo{
		Documents:       request.Documents,
		ReturnDocuments: request.GetReturnDocuments(),
	}
	return info
}

func GenRelayInfoOpenAIAudio(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAIAudio
	return info
}

func GenRelayInfoEmbedding(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatEmbedding
	return info
}

func GenRelayInfoResponses(c *gin.Context, request *dto.OpenAIResponsesRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayMode = relayconstant.RelayModeResponses
	info.RelayFormat = types.RelayFormatOpenAIResponses

	info.ResponsesUsageInfo = &ResponsesUsageInfo{
		BuiltInTools: make(map[string]*BuildInToolInfo),
	}
	if len(request.Tools) > 0 {
		for _, tool := range request.GetToolsMap() {
			toolType := common.Interface2String(tool["type"])
			info.ResponsesUsageInfo.BuiltInTools[toolType] = &BuildInToolInfo{
				ToolName:  toolType,
				CallCount: 0,
			}
			switch toolType {
			case dto.BuildInToolWebSearchPreview:
				searchContextSize := common.Interface2String(tool["search_context_size"])
				if searchContextSize == "" {
					searchContextSize = "medium"
				}
				info.ResponsesUsageInfo.BuiltInTools[toolType].SearchContextSize = searchContextSize
			}
		}
	}
	return info
}

func GenRelayInfoGemini(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatGemini
	info.ShouldIncludeUsage = false

	return info
}

func GenRelayInfoImage(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAIImage
	return info
}

func GenRelayInfoOpenAI(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAI
	return info
}

func genBaseRelayInfo(c *gin.Context, request dto.Request) *RelayInfo {

	//channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	//channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	//paramOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)

	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	// 当令牌分组为空时，表示使用用户分组
	if tokenGroup == "" {
		tokenGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}

	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}

	isStream := false

	if request != nil {
		isStream = request.IsStream(c.Request)
	}
	c.Set(string(constant.ContextKeyIsStream), isStream)

	// firstResponseTime = time.Now() - 1 second

	reqId := common.GetContextKeyString(c, common.RequestIdKey)
	if reqId == "" {
		reqId = common.NewRequestId()
	}
	info := &RelayInfo{
		Request: request,

		RequestId:  reqId,
		UserId:     common.GetContextKeyInt(c, constant.ContextKeyUserId),
		UsingGroup: common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		UserGroup:  common.GetContextKeyString(c, constant.ContextKeyUserGroup),
		UserQuota:  common.GetContextKeyInt(c, constant.ContextKeyUserQuota),
		UserEmail:  common.GetContextKeyString(c, constant.ContextKeyUserEmail),

		OriginModelName: common.GetContextKeyString(c, constant.ContextKeyOriginalModel),

		TokenId:        common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenKey:       common.GetContextKeyString(c, constant.ContextKeyTokenKey),
		TokenUnlimited: common.GetContextKeyBool(c, constant.ContextKeyTokenUnlimited),
		TokenGroup:     tokenGroup,

		isFirstResponse: true,
		RelayMode:       relayconstant.Path2RelayMode(c.Request.URL.Path),
		RequestURLPath:  c.Request.URL.String(),
		RequestHeaders:  cloneRequestHeaders(c),
		IsStream:        isStream,

		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
		ThinkingContentInfo: ThinkingContentInfo{
			IsFirstThinkingContent:  true,
			SendLastThinkingContent: false,
		},
		TokenCountMeta: TokenCountMeta{
			//promptTokens: common.GetContextKeyInt(c, constant.ContextKeyPromptTokens),
			estimatePromptTokens: common.GetContextKeyInt(c, constant.ContextKeyEstimatedTokens),
		},
	}

	if info.RelayMode == relayconstant.RelayModeUnknown {
		info.RelayMode = c.GetInt("relay_mode")
	}

	if strings.HasPrefix(c.Request.URL.Path, "/pg") {
		info.IsPlayground = true
		info.RequestURLPath = strings.TrimPrefix(info.RequestURLPath, "/pg")
		info.RequestURLPath = "/v1" + info.RequestURLPath
	}

	userSetting, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
	if ok {
		info.UserSetting = userSetting
	}

	return info
}

func cloneRequestHeaders(c *gin.Context) map[string]string {
	if c == nil || c.Request == nil {
		return nil
	}
	if len(c.Request.Header) == 0 {
		return nil
	}
	headers := make(map[string]string, len(c.Request.Header))
	for key := range c.Request.Header {
		value := strings.TrimSpace(c.Request.Header.Get(key))
		if value == "" {
			continue
		}
		headers[key] = value
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func GenRelayInfo(c *gin.Context, relayFormat types.RelayFormat, request dto.Request, ws *websocket.Conn) (*RelayInfo, error) {
	var info *RelayInfo
	var err error
	switch relayFormat {
	case types.RelayFormatOpenAI:
		info = GenRelayInfoOpenAI(c, request)
	case types.RelayFormatOpenAIAudio:
		info = GenRelayInfoOpenAIAudio(c, request)
	case types.RelayFormatOpenAIImage:
		info = GenRelayInfoImage(c, request)
	case types.RelayFormatOpenAIRealtime:
		info = GenRelayInfoWs(c, ws)
	case types.RelayFormatClaude:
		info = GenRelayInfoClaude(c, request)
	case types.RelayFormatRerank:
		if request, ok := request.(*dto.RerankRequest); ok {
			info = GenRelayInfoRerank(c, request)
			break
		}
		err = errors.New("request is not a RerankRequest")
	case types.RelayFormatGemini:
		info = GenRelayInfoGemini(c, request)
	case types.RelayFormatEmbedding:
		info = GenRelayInfoEmbedding(c, request)
	case types.RelayFormatOpenAIResponses:
		if request, ok := request.(*dto.OpenAIResponsesRequest); ok {
			info = GenRelayInfoResponses(c, request)
			break
		}
		err = errors.New("request is not a OpenAIResponsesRequest")
	case types.RelayFormatOpenAIResponsesCompaction:
		if request, ok := request.(*dto.OpenAIResponsesCompactionRequest); ok {
			return GenRelayInfoResponsesCompaction(c, request), nil
		}
		return nil, errors.New("request is not a OpenAIResponsesCompactionRequest")
	case types.RelayFormatOpenAIAlphaSearch:
		if request, ok := request.(*dto.AlphaSearchRequest); ok {
			return GenRelayInfoAlphaSearch(c, request), nil
		}
		return nil, errors.New("request is not a AlphaSearchRequest")
	case types.RelayFormatTask:
		info = genBaseRelayInfo(c, nil)
		info.TaskRelayInfo = &TaskRelayInfo{}
	case types.RelayFormatMjProxy:
		info = genBaseRelayInfo(c, nil)
		info.TaskRelayInfo = &TaskRelayInfo{}
	default:
		err = errors.New("invalid relay format")
	}

	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errors.New("failed to build relay info")
	}

	info.InitRequestConversionChain()
	return info, nil
}

func (info *RelayInfo) InitRequestConversionChain() {
	if info == nil {
		return
	}
	if len(info.RequestConversionChain) > 0 {
		return
	}
	if info.RelayFormat == "" {
		return
	}
	info.RequestConversionChain = []types.RelayFormat{info.RelayFormat}
}

func (info *RelayInfo) AppendRequestConversion(format types.RelayFormat) {
	if info == nil {
		return
	}
	if format == "" {
		return
	}
	if len(info.RequestConversionChain) == 0 {
		info.RequestConversionChain = []types.RelayFormat{format}
		return
	}
	last := info.RequestConversionChain[len(info.RequestConversionChain)-1]
	if last == format {
		return
	}
	info.RequestConversionChain = append(info.RequestConversionChain, format)
}

func (info *RelayInfo) GetFinalRequestRelayFormat() types.RelayFormat {
	if info == nil {
		return ""
	}
	if info.FinalRequestRelayFormat != "" {
		return info.FinalRequestRelayFormat
	}
	if n := len(info.RequestConversionChain); n > 0 {
		return info.RequestConversionChain[n-1]
	}
	return info.RelayFormat
}

func GenRelayInfoResponsesCompaction(c *gin.Context, request *dto.OpenAIResponsesCompactionRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	if info.RelayMode == relayconstant.RelayModeUnknown {
		info.RelayMode = relayconstant.RelayModeResponsesCompact
	}
	info.RelayFormat = types.RelayFormatOpenAIResponsesCompaction
	return info
}

func GenRelayInfoAlphaSearch(c *gin.Context, request *dto.AlphaSearchRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	if info.RelayMode == relayconstant.RelayModeUnknown {
		info.RelayMode = relayconstant.RelayModeAlphaSearch
	}
	info.RelayFormat = types.RelayFormatOpenAIAlphaSearch
	info.ResponsesUsageInfo = &ResponsesUsageInfo{
		BuiltInTools: map[string]*BuildInToolInfo{
			dto.BuildInToolWebSearchPreview: {
				ToolName:  dto.BuildInToolWebSearchPreview,
				CallCount: 0,
			},
		},
	}
	return info
}

//func (info *RelayInfo) SetPromptTokens(promptTokens int) {
//	info.promptTokens = promptTokens
//}

func (info *RelayInfo) SetEstimatePromptTokens(promptTokens int) {
	if info == nil {
		return
	}
	info.estimatePromptTokens = promptTokens
}

func (info *RelayInfo) GetEstimatePromptTokens() int {
	if info == nil {
		return 0
	}
	return info.estimatePromptTokens
}

// ---------------------------------------------------------------------------
// convmeta.Meta implementation — the view format converters see. Keep these
// thin: they only expose protocol state, never billing/user fields.
// ---------------------------------------------------------------------------

var _ convmeta.Meta = (*RelayInfo)(nil)

func (info *RelayInfo) GetOriginModelName() string {
	if info == nil {
		return ""
	}
	return info.OriginModelName
}

func (info *RelayInfo) GetUpstreamModelName() string {
	if info == nil || info.ChannelMeta == nil {
		return ""
	}
	return info.UpstreamModelName
}

func (info *RelayInfo) HasChannelMeta() bool { return info != nil && info.ChannelMeta != nil }

func (info *RelayInfo) GetChannelID() int {
	if info == nil || info.ChannelMeta == nil {
		return 0
	}
	return info.ChannelId
}

func (info *RelayInfo) GetChannelType() int {
	if info == nil || info.ChannelMeta == nil {
		return 0
	}
	return info.ChannelType
}

func (info *RelayInfo) GetIsStream() bool {
	return info != nil && info.IsStream
}

func (info *RelayInfo) GetReasoningEffort() string {
	if info == nil {
		return ""
	}
	return info.ReasoningEffort
}

func (info *RelayInfo) SetReasoningEffort(effort string) {
	if info == nil {
		return
	}
	info.ReasoningEffort = effort
}

func (info *RelayInfo) EnsureClaudeConvertInfo() *convmeta.ClaudeConvertInfo {
	if info == nil {
		return &convmeta.ClaudeConvertInfo{
			LastMessagesType: convmeta.LastMessageTypeNone,
		}
	}
	if info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &convmeta.ClaudeConvertInfo{
			LastMessagesType: convmeta.LastMessageTypeNone,
		}
	}
	return info.ClaudeConvertInfo
}

func (info *RelayInfo) GetSendResponseCount() int {
	if info == nil {
		return 0
	}
	return info.SendResponseCount
}

func (info *RelayInfo) IncrSendResponseCount() {
	if info == nil {
		return
	}
	info.SendResponseCount++
}

// ConvOptions snapshots host settings for the converters. Rebuilt on each
// call site's first use; cached so one relay session sees one snapshot.
func (info *RelayInfo) ConvOptions() *convmeta.Options {
	if info != nil && info.convOptions != nil {
		return info.convOptions
	}

	claudeSettings := model_setting.GetClaudeSettings()
	geminiSettings := model_setting.GetGeminiSettings()
	options := &convmeta.Options{
		Claude: convmeta.ClaudeOptions{
			ThinkingAdapterEnabled:                claudeSettings.ThinkingAdapterEnabled,
			ThinkingAdapterBudgetTokensPercentage: claudeSettings.ThinkingAdapterBudgetTokensPercentage,
			DefaultMaxTokens:                      claudeSettings.GetDefaultMaxTokens,
		},
		Gemini: convmeta.GeminiOptions{
			ThinkingAdapterEnabled:                geminiSettings.ThinkingAdapterEnabled,
			ThinkingAdapterBudgetTokensPercentage: geminiSettings.ThinkingAdapterBudgetTokensPercentage,
			FunctionCallThoughtSignatureEnabled:   geminiSettings.FunctionCallThoughtSignatureEnabled,
			SupportsImagine:                       model_setting.IsGeminiModelSupportImagine,
			SafetySetting:                         model_setting.GetGeminiSafetySetting,
		},
		OpenRouterDialect:      info != nil && info.GetChannelType() == constant.ChannelTypeOpenRouter,
		PreserveThinkingSuffix: model_setting.ShouldPreserveThinkingSuffix,
	}
	if info != nil {
		info.convOptions = options
	}
	return options
}

// BeginDynamicRoutingAttempt resets only per-attempt telemetry. The selected
// channel type is explicit because ChannelMeta can still describe the previous
// retry until the handler rebuilds it. Request-wide timing such as StartTime
// and FirstResponseTime intentionally survives retries.
func (info *RelayInfo) BeginDynamicRoutingAttempt(channelID int, channelType int, publicModel string, enabled bool) {
	if info == nil {
		return
	}
	info.attemptMu.Lock()
	info.currentAttempt = dynamicRoutingAttempt{
		enabled:     enabled,
		channelID:   channelID,
		channelType: channelType,
		model:       publicModel,
	}
	if enabled {
		info.StreamStatus = nil
	}
	info.attemptMu.Unlock()
}

// DiscardDynamicRoutingAttempt clears only the current attempt. It is used
// when downstream cancellation makes every in-flight timing or failure signal
// ambiguous, while preserving request-wide timing across retries.
func (info *RelayInfo) DiscardDynamicRoutingAttempt() {
	if info == nil {
		return
	}
	info.attemptMu.Lock()
	info.currentAttempt = dynamicRoutingAttempt{}
	info.StreamStatus = nil
	info.attemptMu.Unlock()
}

// MarkAttemptUpstreamStarted marks the boundary immediately before the
// physical upstream call. Performance timing requires this boundary; proven
// channel config errors and explicitly marked channel-owned preflight failures
// may still contribute health-only observations before it.
func (info *RelayInfo) MarkAttemptUpstreamStarted() {
	if info == nil {
		return
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	if !info.currentAttempt.enabled || info.currentAttempt.reachedUpstream {
		return
	}
	info.currentAttempt.reachedUpstream = true
	info.currentAttempt.upstreamStarted = info.attemptTimeNowLocked()
}

// MarkDynamicRoutingAttemptPreUpstreamHard records a channel-owned preflight
// failure that happened before the physical model call. It is intentionally
// separate from channel-prefixed errors: transient token endpoint or network
// failures should affect dynamic health and retry, but must not force the
// permanent auto-disable policy used for proven credential/config errors.
func (info *RelayInfo) MarkDynamicRoutingAttemptPreUpstreamHard() {
	if info == nil {
		return
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	if info.currentAttempt.enabled && !info.currentAttempt.reachedUpstream {
		info.currentAttempt.preUpstreamHard = true
	}
}

// SetAttemptHTTPStatus preserves the raw upstream status before configured
// status-code mappings alter the client-facing error.
func (info *RelayInfo) SetAttemptHTTPStatus(status int) {
	if info == nil {
		return
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	if info.currentAttempt.enabled && info.currentAttempt.reachedUpstream {
		info.currentAttempt.httpStatus = status
	}
}

// SetAttemptCompletionTokens records the first trusted upstream or locally
// counted output token count. Timing comes only from upstream-visible content
// events, so billing/database work cannot extend TPOT.
func (info *RelayInfo) SetAttemptCompletionTokens(tokens int) {
	if info == nil || tokens < 0 {
		return
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	if info.currentAttempt.enabled && info.currentAttempt.reachedUpstream && !info.currentAttempt.completionTokensSet {
		info.currentAttempt.completionTokens = tokens
		info.currentAttempt.completionTokensSet = true
	}
}

// RecordAttemptVisibleText records both the timing and exact text of an
// upstream event known to contain user-visible model output. Reasoning, tools,
// audio, and image data must not be passed here.
func (info *RelayInfo) RecordAttemptVisibleText(text string) {
	if info == nil || text == "" {
		return
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	if info.currentAttempt.enabled && info.currentAttempt.reachedUpstream {
		now := info.attemptTimeNowLocked()
		if info.currentAttempt.firstResponseAt.IsZero() {
			// Visible output is itself a non-terminal upstream payload. Keep the
			// attempt health boundary valid even if a custom transport omitted the
			// separate request-wide first-event hook.
			info.currentAttempt.firstResponseAt = now
		}
		if info.currentAttempt.firstContentAt.IsZero() {
			info.currentAttempt.firstContentAt = now
		}
		info.currentAttempt.lastContentAt = now
		info.currentAttempt.visibleTextParts = append(info.currentAttempt.visibleTextParts, text)
	}
}

// DynamicRoutingAttemptVisibleText returns a stable copy of the visible text
// accumulated by the current attempt for local token counting.
func (info *RelayInfo) DynamicRoutingAttemptVisibleText() string {
	if info == nil {
		return ""
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	return strings.Join(info.currentAttempt.visibleTextParts, "")
}

// DynamicRoutingAttemptModel returns the immutable public model captured when
// the current attempt began. Billing aliases may mutate OriginModelName later
// in the relay without changing the observation key or tokenizer model.
func (info *RelayInfo) DynamicRoutingAttemptModel() string {
	if info == nil {
		return ""
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	return info.currentAttempt.model
}

// InvalidateDynamicRoutingAttemptTPOT marks only inter-token timing as
// unreliable. It is used by synchronous custom transports whose first
// upstream event remains trustworthy but whose later reads follow downstream
// writes.
func (info *RelayInfo) InvalidateDynamicRoutingAttemptTPOT() {
	if info == nil {
		return
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	if info.currentAttempt.enabled && info.currentAttempt.reachedUpstream {
		info.currentAttempt.tPOTInvalid = true
	}
}

// MarkDynamicRoutingAttemptBackpressure marks output timing that crossed a
// full downstream queue. TPOT is always invalid after this point. If the queue
// filled before any visible text arrived, TTFT is invalid too because reading
// the first visible upstream event may have been delayed by the client.
func (info *RelayInfo) MarkDynamicRoutingAttemptBackpressure() {
	if info == nil {
		return
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	if !info.currentAttempt.enabled || !info.currentAttempt.reachedUpstream {
		return
	}
	info.currentAttempt.tPOTInvalid = true
	if info.currentAttempt.firstContentAt.IsZero() {
		info.currentAttempt.tTFTInvalid = true
	}
}

// DynamicRoutingAttemptBackpressured reports whether the current bounded
// ingress has ever had to wait for downstream delivery. It is primarily
// useful for diagnostics and deterministic transport tests.
func (info *RelayInfo) DynamicRoutingAttemptBackpressured() bool {
	if info == nil {
		return false
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	return info.currentAttempt.tPOTInvalid
}

// FinishDynamicRoutingAttempt snapshots and clears the current attempt. The
// boolean is false for bypassed requests, unmarked local pre-upstream failures,
// and stream endings that describe downstream cancellation rather than
// upstream health.
func (info *RelayInfo) FinishDynamicRoutingAttempt(handlerErr *types.NewAPIError) (DynamicRoutingAttemptSample, bool) {
	if info == nil {
		return DynamicRoutingAttemptSample{}, false
	}

	info.attemptMu.Lock()
	attempt := info.currentAttempt
	info.currentAttempt = dynamicRoutingAttempt{}
	finishedAt := info.attemptTimeNowLocked()
	info.attemptMu.Unlock()

	if !attempt.enabled {
		return DynamicRoutingAttemptSample{}, false
	}

	sample := DynamicRoutingAttemptSample{
		ChannelID:         attempt.channelID,
		Model:             attempt.model,
		ObservedAt:        finishedAt,
		UpstreamStartedAt: attempt.upstreamStarted,
		FirstResponseAt:   attempt.firstResponseAt,
		FirstContentAt:    attempt.firstContentAt,
		LastContentAt:     attempt.lastContentAt,
		CompletionTokens:  attempt.completionTokens,
		TTFTInvalidated:   attempt.tTFTInvalid,
		TPOTInvalidated:   attempt.tPOTInvalid,
	}
	if !attempt.reachedUpstream {
		// Physical dispatch is required for performance timing, but an explicit
		// channel construction/authentication failure is already a trustworthy
		// health signal. Ordinary local conversion and client errors remain
		// unobserved.
		if handlerErr != nil && (types.IsChannelError(handlerErr) || attempt.preUpstreamHard) {
			sample.HardFailure = true
			return sample, true
		}
		return DynamicRoutingAttemptSample{}, false
	}

	if info.IsStream {
		reason := StreamEndReasonNone
		var streamEndErr error
		if info.StreamStatus != nil {
			reason, streamEndErr = info.StreamStatus.End()
		}
		switch reason {
		case StreamEndReasonTimeout, StreamEndReasonScannerErr:
			sample.HardFailure = true
		case StreamEndReasonDone, StreamEndReasonEOF:
			if isHardDynamicRoutingFailure(attempt.httpStatus, handlerErr, attempt.channelType) {
				sample.HardFailure = true
			} else if handlerErr == nil {
				sample.Success = true
			}
		case StreamEndReasonHandlerStop:
			if isHardDynamicRoutingFailure(attempt.httpStatus, handlerErr, attempt.channelType) {
				sample.HardFailure = true
			} else if handlerErr != nil {
				// A non-channel 4xx/local handler error is observed but is not an
				// upstream-health failure and must not be counted as success.
			} else if streamEndErr != nil {
				var streamAPIError *types.NewAPIError
				if errors.As(streamEndErr, &streamAPIError) {
					sample.HardFailure = isHardDynamicRoutingFailure(attempt.httpStatus, streamAPIError, attempt.channelType)
				}
			} else {
				sample.Success = true
			}
		case StreamEndReasonNone:
			// An immediate HTTP/channel failure happens before a stream scanner
			// exists. Preserve that strong upstream signal; ambiguous local errors
			// with no stream lifecycle remain discarded.
			if !isHardDynamicRoutingFailure(attempt.httpStatus, handlerErr, attempt.channelType) {
				return DynamicRoutingAttemptSample{}, false
			}
			sample.HardFailure = true
		case StreamEndReasonClientGone, StreamEndReasonPingFail, StreamEndReasonPanic:
			return DynamicRoutingAttemptSample{}, false
		default:
			return DynamicRoutingAttemptSample{}, false
		}
	} else {
		if isHardDynamicRoutingFailure(attempt.httpStatus, handlerErr, attempt.channelType) {
			sample.HardFailure = true
		} else if handlerErr == nil {
			sample.Success = true
		}
	}
	if info.IsStream && sample.Success && attempt.firstResponseAt.IsZero() {
		// An empty HTTP 200 body (or a terminal marker with no preceding payload)
		// is a broken stream, not a health success. Metadata and tool-only events
		// still set the first-response boundary and remain valid health-only
		// successes without entering the performance ring.
		sample.Success = false
		sample.HardFailure = true
	}

	if info.IsStream && sample.Success && !attempt.tTFTInvalid && !attempt.upstreamStarted.IsZero() &&
		!attempt.firstContentAt.IsZero() && attempt.firstContentAt.After(attempt.upstreamStarted) {
		sample.TTFT = attempt.firstContentAt.Sub(attempt.upstreamStarted)
		sample.HasTTFT = true
		if !attempt.tPOTInvalid && attempt.completionTokens > 1 && attempt.lastContentAt.After(attempt.firstContentAt) {
			sample.TPOT = attempt.lastContentAt.Sub(attempt.firstContentAt) / time.Duration(attempt.completionTokens-1)
			sample.HasTPOT = true
		}
	}

	return sample, true
}

func isHardDynamicRoutingFailure(upstreamStatus int, handlerErr *types.NewAPIError, channelType int) bool {
	if isDynamicRoutingModelUnavailable(handlerErr, channelType) || (handlerErr != nil && types.IsChannelError(handlerErr)) {
		return true
	}
	if upstreamStatus != 0 && upstreamStatus != http.StatusOK {
		return (upstreamStatus > 0 && upstreamStatus < http.StatusBadRequest) ||
			upstreamStatus == http.StatusPaymentRequired || upstreamStatus == http.StatusRequestTimeout || upstreamStatus == http.StatusUnauthorized || upstreamStatus == http.StatusForbidden ||
			upstreamStatus == http.StatusProxyAuthRequired || upstreamStatus == http.StatusTooManyRequests || upstreamStatus == 498 || upstreamStatus == 499 ||
			upstreamStatus >= http.StatusInternalServerError
	}
	if handlerErr == nil {
		return false
	}
	return (handlerErr.StatusCode >= http.StatusMultipleChoices && handlerErr.StatusCode < http.StatusBadRequest) ||
		handlerErr.StatusCode == http.StatusPaymentRequired || handlerErr.StatusCode == http.StatusRequestTimeout || handlerErr.StatusCode == http.StatusUnauthorized || handlerErr.StatusCode == http.StatusForbidden ||
		handlerErr.StatusCode == http.StatusProxyAuthRequired || handlerErr.StatusCode == http.StatusTooManyRequests || handlerErr.StatusCode == 498 || handlerErr.StatusCode == 499 ||
		handlerErr.StatusCode >= http.StatusInternalServerError
}

func isDynamicRoutingModelUnavailable(handlerErr *types.NewAPIError, channelType int) bool {
	if handlerErr == nil {
		return false
	}
	normalize := func(value any) string {
		return strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	}
	errorCode := normalize(handlerErr.GetErrorCode())
	if errorCode == string(types.ErrorCodeModelNotFound) || errorCode == "model_not_found_error" {
		return true
	}
	if channelType == constant.ChannelTypeAzure && (errorCode == "deploymentnotfound" || errorCode == "modelnotfound") {
		return true
	}
	if handlerErr.GetErrorType() == types.ErrorTypeClaudeError && errorCode == "not_found_error" {
		return true
	}
	if openAIError, ok := handlerErr.RelayError.(types.OpenAIError); ok {
		openAIErrorCode := normalize(openAIError.Code)
		if channelType == constant.ChannelTypeAzure && (openAIErrorCode == "deploymentnotfound" || openAIErrorCode == "modelnotfound") {
			return true
		}
		return openAIErrorCode == string(types.ErrorCodeModelNotFound) ||
			normalize(openAIError.Type) == string(types.ErrorCodeModelNotFound) ||
			normalize(openAIError.Type) == "model_not_found_error"
	}
	return false
}

func (info *RelayInfo) SetFirstResponseTime() {
	if info == nil {
		return
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	needsRequestTime := info.isFirstResponse
	needsAttemptTime := info.currentAttempt.enabled && info.currentAttempt.reachedUpstream && info.currentAttempt.firstResponseAt.IsZero()
	if !needsRequestTime && !needsAttemptTime {
		return
	}
	now := info.attemptTimeNowLocked()
	if needsRequestTime {
		info.FirstResponseTime = now
		info.isFirstResponse = false
	}
	if needsAttemptTime {
		info.currentAttempt.firstResponseAt = now
	}
}

func (info *RelayInfo) HasSendResponse() bool {
	if info == nil {
		return false
	}
	info.attemptMu.Lock()
	defer info.attemptMu.Unlock()
	return info.FirstResponseTime.After(info.StartTime)
}

func (info *RelayInfo) attemptTimeNowLocked() time.Time {
	if info.attemptNow != nil {
		return info.attemptNow()
	}
	return time.Now()
}

// setAttemptNowForTest gives same-package tests a deterministic clock without
// exposing a production timing override.
func (info *RelayInfo) setAttemptNowForTest(now func() time.Time) {
	info.attemptMu.Lock()
	info.attemptNow = now
	info.attemptMu.Unlock()
}

type TaskRelayInfo struct {
	Action       string
	OriginTaskID string
	// PublicTaskID 是提交时预生成的 task_xxxx 格式公开 ID，
	// 供 DoResponse 在返回给客户端时使用（避免暴露上游真实 ID）。
	PublicTaskID string

	ConsumeQuota bool

	// LockedChannel holds the full channel object when the request is bound to
	// a specific channel (e.g., remix on origin task's channel). Stored as any
	// to avoid an import cycle with model; callers type-assert to *model.Channel.
	LockedChannel any
}

type TaskSubmitReq struct {
	Prompt         string                 `json:"prompt"`
	Model          string                 `json:"model,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	Image          string                 `json:"image,omitempty"`
	Images         []string               `json:"images,omitempty"`
	Size           string                 `json:"size,omitempty"`
	Duration       int                    `json:"duration,omitempty"`
	Seconds        string                 `json:"seconds,omitempty"`
	InputReference string                 `json:"input_reference,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

func (t *TaskSubmitReq) GetPrompt() string {
	return t.Prompt
}

func (t *TaskSubmitReq) HasImage() bool {
	return len(t.Images) > 0
}

func (t *TaskSubmitReq) UnmarshalJSON(data []byte) error {
	type Alias TaskSubmitReq
	aux := &struct {
		Metadata json.RawMessage `json:"metadata,omitempty"`
		Duration json.RawMessage `json:"duration,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.Duration) > 0 {
		var durationInt int
		if err := common.Unmarshal(aux.Duration, &durationInt); err == nil {
			t.Duration = durationInt
		} else {
			var durationStr string
			if err := common.Unmarshal(aux.Duration, &durationStr); err == nil && durationStr != "" {
				if v, err := strconv.Atoi(durationStr); err == nil {
					t.Duration = v
				}
			}
		}
	}

	if len(aux.Metadata) > 0 {
		var metadataStr string
		if err := common.Unmarshal(aux.Metadata, &metadataStr); err == nil && metadataStr != "" {
			var metadataObj map[string]interface{}
			if err := common.Unmarshal([]byte(metadataStr), &metadataObj); err == nil {
				t.Metadata = metadataObj
				return nil
			}
		}

		var metadataObj map[string]interface{}
		if err := common.Unmarshal(aux.Metadata, &metadataObj); err == nil {
			t.Metadata = metadataObj
		}
	}

	return nil
}
func (t *TaskSubmitReq) UnmarshalMetadata(v any) error {
	metadata := t.Metadata
	if metadata != nil {
		metadataBytes, err := common.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata failed: %w", err)
		}
		err = common.Unmarshal(metadataBytes, v)
		if err != nil {
			return fmt.Errorf("unmarshal metadata to target failed: %w", err)
		}
	}
	return nil
}

type TaskInfo struct {
	Code             int    `json:"code"`
	TaskID           string `json:"task_id"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	Url              string `json:"url,omitempty"`
	RemoteUrl        string `json:"remote_url,omitempty"`
	Progress         string `json:"progress,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"` // 用于按倍率计费
	TotalTokens      int    `json:"total_tokens,omitempty"`      // 用于按倍率计费
}

func FailTaskInfo(reason string) *TaskInfo {
	return &TaskInfo{
		Status: "FAILURE",
		Reason: reason,
	}
}

// RemoveDisabledFields 从请求 JSON 数据中移除渠道设置中禁用的字段
// service_tier: 服务层级字段，可能导致额外计费（OpenAI、Claude、Responses API 支持）
// inference_geo: Claude 数据驻留推理区域字段（仅 Claude 支持，默认过滤）
// speed: Claude 推理速度模式字段（仅 Claude 支持，默认过滤）
// store: 数据存储授权字段，涉及用户隐私（仅 OpenAI、Responses API 支持，默认允许透传，禁用后可能导致 Codex 无法使用）
// safety_identifier: 安全标识符，用于向 OpenAI 报告违规用户（仅 OpenAI 支持，涉及用户隐私）
// stream_options.include_obfuscation: 响应流混淆控制字段（仅 OpenAI Responses API 支持）
func RemoveDisabledFields(jsonData []byte, channelOtherSettings dto.ChannelOtherSettings, channelPassThroughEnabled bool) ([]byte, error) {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || channelPassThroughEnabled {
		return jsonData, nil
	}
	if !hasRemovableDisabledField(jsonData, channelOtherSettings) {
		return jsonData, nil
	}

	var data map[string]interface{}
	if err := common.Unmarshal(jsonData, &data); err != nil {
		common.SysError("RemoveDisabledFields Unmarshal error :" + err.Error())
		return jsonData, nil
	}

	// 默认移除 service_tier，除非明确允许（避免额外计费风险）
	if !channelOtherSettings.AllowServiceTier {
		if _, exists := data["service_tier"]; exists {
			delete(data, "service_tier")
		}
	}

	// 默认移除 inference_geo，除非明确允许（避免在未授权情况下透传数据驻留区域）
	if !channelOtherSettings.AllowInferenceGeo {
		if _, exists := data["inference_geo"]; exists {
			delete(data, "inference_geo")
		}
	}

	// 默认移除 speed，除非明确允许（避免意外切换 Claude 推理速度模式）
	if !channelOtherSettings.AllowSpeed {
		if _, exists := data["speed"]; exists {
			delete(data, "speed")
		}
	}

	// 默认允许 store 透传，除非明确禁用（禁用可能影响 Codex 使用）
	if channelOtherSettings.DisableStore {
		if _, exists := data["store"]; exists {
			delete(data, "store")
		}
	}

	// 默认移除 safety_identifier，除非明确允许（保护用户隐私，避免向 OpenAI 报告用户信息）
	if !channelOtherSettings.AllowSafetyIdentifier {
		if _, exists := data["safety_identifier"]; exists {
			delete(data, "safety_identifier")
		}
	}

	// 默认移除 stream_options.include_obfuscation，除非明确允许（避免关闭响应流混淆保护）
	if !channelOtherSettings.AllowIncludeObfuscation {
		if streamOptionsAny, exists := data["stream_options"]; exists {
			if streamOptions, ok := streamOptionsAny.(map[string]interface{}); ok {
				if _, includeExists := streamOptions["include_obfuscation"]; includeExists {
					delete(streamOptions, "include_obfuscation")
				}
				if len(streamOptions) == 0 {
					delete(data, "stream_options")
				} else {
					data["stream_options"] = streamOptions
				}
			}
		}
	}

	jsonDataAfter, err := common.Marshal(data)
	if err != nil {
		common.SysError("RemoveDisabledFields Marshal error :" + err.Error())
		return jsonData, nil
	}
	return jsonDataAfter, nil
}

func hasRemovableDisabledField(jsonData []byte, channelOtherSettings dto.ChannelOtherSettings) bool {
	values := gjson.GetManyBytes(
		jsonData,
		"service_tier",
		"inference_geo",
		"speed",
		"store",
		"safety_identifier",
		"stream_options.include_obfuscation",
	)

	return (!channelOtherSettings.AllowServiceTier && values[0].Exists()) ||
		(!channelOtherSettings.AllowInferenceGeo && values[1].Exists()) ||
		(!channelOtherSettings.AllowSpeed && values[2].Exists()) ||
		(channelOtherSettings.DisableStore && values[3].Exists()) ||
		(!channelOtherSettings.AllowSafetyIdentifier && values[4].Exists()) ||
		(!channelOtherSettings.AllowIncludeObfuscation && values[5].Exists())
}

// RemoveGeminiDisabledFields removes disabled fields from Gemini request JSON data
// Currently supports removing functionResponse.id field which Vertex AI does not support
func RemoveGeminiDisabledFields(jsonData []byte) ([]byte, error) {
	if !model_setting.GetGeminiSettings().RemoveFunctionResponseIdEnabled {
		return jsonData, nil
	}

	var data map[string]interface{}
	if err := common.Unmarshal(jsonData, &data); err != nil {
		common.SysError("RemoveGeminiDisabledFields Unmarshal error: " + err.Error())
		return jsonData, nil
	}

	// Process contents array
	// Handle both camelCase (functionResponse) and snake_case (function_response)
	if contents, ok := data["contents"].([]interface{}); ok {
		for _, content := range contents {
			if contentMap, ok := content.(map[string]interface{}); ok {
				if parts, ok := contentMap["parts"].([]interface{}); ok {
					for _, part := range parts {
						if partMap, ok := part.(map[string]interface{}); ok {
							// Check functionResponse (camelCase)
							if funcResp, ok := partMap["functionResponse"].(map[string]interface{}); ok {
								delete(funcResp, "id")
							}
							// Check function_response (snake_case)
							if funcResp, ok := partMap["function_response"].(map[string]interface{}); ok {
								delete(funcResp, "id")
							}
						}
					}
				}
			}
		}
	}

	jsonDataAfter, err := common.Marshal(data)
	if err != nil {
		common.SysError("RemoveGeminiDisabledFields Marshal error: " + err.Error())
		return jsonData, nil
	}
	return jsonDataAfter, nil
}
