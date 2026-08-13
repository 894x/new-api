package xunfei_maas

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	ChannelName = "xunfei_maas"
	requestPath = "/v2/chat/completions"
)

var ModelList = []string{"xopdeepseekv4flash"}

type Adaptor struct {
	openaiAdaptor openai.Adaptor
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	info.SupportStreamOptions = true
	a.openaiAdaptor.Init(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := constant.ChannelBaseURLs[constant.ChannelTypeXunfeiMaaS]
	if info != nil && strings.TrimSpace(info.ChannelBaseUrl) != "" {
		baseURL = strings.TrimSpace(info.ChannelBaseUrl)
	}
	return strings.TrimRight(baseURL, "/") + requestPath, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	converted, err := a.openaiAdaptor.ConvertOpenAIRequest(c, info, request)
	if err != nil {
		return nil, err
	}
	chatRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, fmt.Errorf("unexpected converted request type %T", converted)
	}
	if info.IsStream {
		chatRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return chatRequest, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	chatRequest, err := service.ClaudeToOpenAIRequest(*request, info)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, chatRequest)
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	chatRequest, err := service.ResponsesRequestToChatCompletionsRequest(&request)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, chatRequest)
}

func (a *Adaptor) ConvertGeminiRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ *dto.GeminiChatRequest) (any, error) {
	return nil, fmt.Errorf("Xunfei MaaS does not support Gemini generateContent")
}

func (a *Adaptor) ConvertRerankRequest(_ *gin.Context, _ int, _ dto.RerankRequest) (any, error) {
	return nil, fmt.Errorf("Xunfei MaaS does not support rerank")
}

func (a *Adaptor) ConvertEmbeddingRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.EmbeddingRequest) (any, error) {
	return nil, fmt.Errorf("Xunfei MaaS does not support embeddings")
}

func (a *Adaptor) ConvertAudioRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.AudioRequest) (io.Reader, error) {
	return nil, fmt.Errorf("Xunfei MaaS does not support audio")
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, _ *relaycommon.RelayInfo, _ dto.ImageRequest) (any, error) {
	return nil, fmt.Errorf("Xunfei MaaS does not support images")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, newAPIError *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeResponses {
		if info.IsStream {
			usage, newAPIError = openai.OaiChatToResponsesStreamHandler(c, info, resp)
		} else {
			usage, newAPIError = openai.OaiChatToResponsesHandler(c, info, resp)
		}
	} else {
		usage, newAPIError = a.openaiAdaptor.DoResponse(c, resp, info)
	}
	if newAPIError != nil {
		return nil, a.NormalizeUpstreamError(newAPIError)
	}
	return usage, nil
}

func (a *Adaptor) NormalizeUpstreamError(err *types.NewAPIError) *types.NewAPIError {
	return classifyError(err)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) ChatCompletionsOnly() bool {
	return true
}
