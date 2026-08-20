package xunfei

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	request          *XunfeiChatRequest
	requestBody      []byte
	authURL          string
	capacityAdmitter func(*gin.Context, *relaycommon.RelayInfo, io.Reader) error
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return "", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) PrepareFinalOutboundRequest(c *gin.Context, info *relaycommon.RelayInfo, request any) (any, error) {
	openAIRequest, ok := request.(*dto.GeneralOpenAIRequest)
	if !ok || openAIRequest == nil {
		return nil, fmt.Errorf("invalid Xunfei request type %T", request)
	}
	splits := strings.Split(info.ApiKey, "|")
	if len(splits) != 3 {
		info.MarkDynamicRoutingAttemptPreUpstreamHard()
		return nil, types.NewError(errors.New("invalid auth"), types.ErrorCodeChannelInvalidKey)
	}
	domain, authURL := getXunfeiAuthUrl(c, splits[2], splits[1], openAIRequest.Model)
	a.authURL = authURL
	return requestOpenAI2Xunfei(*openAIRequest, splits[0], domain), nil
}

func (a *Adaptor) DeferChannelModelCapacityAdmission() bool {
	return true
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if requestBody == nil {
		return nil, errors.New("request body is nil")
	}
	data, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	var request XunfeiChatRequest
	if err := common.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	a.request = &request
	a.requestBody = data
	// Xunfei dispatches through a WebSocket in DoResponse.
	dummyResp := &http.Response{}
	dummyResp.StatusCode = http.StatusOK
	return dummyResp, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if a.request == nil {
		return nil, types.NewError(errors.New("request is nil"), types.ErrorCodeInvalidRequest)
	}
	body, closer, bodyErr := relaycommon.NewOutboundJSONBody(a.requestBody)
	if bodyErr != nil {
		return nil, types.NewError(bodyErr, types.ErrorCodeCountTokenFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	admit := a.capacityAdmitter
	if admit == nil {
		admit = service.AdmitFinalChannelModelCapacity
	}
	if capacityErr := admit(c, info, body); capacityErr != nil {
		var admissionErr *service.ChannelModelCapacityAdmissionError
		if errors.As(capacityErr, &admissionErr) {
			return nil, types.NewErrorWithStatusCode(
				capacityErr,
				types.ErrorCodeChannelModelCapacityExhausted,
				http.StatusTooManyRequests,
				types.ErrOptionWithSkipRetry(),
			)
		}
		return nil, types.NewError(capacityErr, types.ErrorCodeCountTokenFailed, types.ErrOptionWithSkipRetry())
	}
	if info.IsStream {
		usage, err = xunfeiStreamHandler(c, info, *a.request, a.authURL)
	} else {
		usage, err = xunfeiHandler(c, info, *a.request, a.authURL)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
