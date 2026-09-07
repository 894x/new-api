package xunfei

import (
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	request   *dto.GeneralOpenAIRequest
	finalBody []byte
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
	a.request = request
	return request, nil
}

func (a *Adaptor) PrepareFinalOutboundRequest(c *gin.Context, info *relaycommon.RelayInfo, request any) (any, error) {
	openAIRequest, ok := request.(*dto.GeneralOpenAIRequest)
	if !ok || openAIRequest == nil {
		return nil, fmt.Errorf("invalid Xunfei request type %T", request)
	}
	credentials := strings.Split(info.ApiKey, "|")
	if len(credentials) != 3 {
		return nil, errors.New("invalid Xunfei credentials")
	}
	domain, _ := getXunfeiAuthUrl(c, credentials[2], credentials[1], openAIRequest.Model)
	return requestOpenAI2Xunfei(*openAIRequest, credentials[0], domain), nil
}
func (a *Adaptor) DeferChannelModelCapacityAdmission() bool { return true }

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
	// Preserve the exact body after shared policies for the WebSocket dispatch.
	data, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	var payload XunfeiChatRequest
	if err = common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	a.finalBody = data
	dummyResp := &http.Response{}
	dummyResp.StatusCode = http.StatusOK
	return dummyResp, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	splits := strings.Split(info.ApiKey, "|")
	if len(splits) != 3 {
		return nil, types.NewError(errors.New("invalid auth"), types.ErrorCodeChannelInvalidKey)
	}
	if a.request == nil {
		return nil, types.NewError(errors.New("request is nil"), types.ErrorCodeInvalidRequest)
	}
	body, closer, bodyErr := relaycommon.NewOutboundJSONBody(a.finalBody)
	if bodyErr != nil {
		return nil, types.NewError(bodyErr, types.ErrorCodeConvertRequestFailed)
	}
	defer closer.Close()
	if admissionErr := service.AdmitFinalChannelModelCapacity(c, info, body); admissionErr != nil {
		var denied *service.ChannelModelCapacityError
		if errors.As(admissionErr, &denied) {
			return nil, types.NewErrorWithStatusCode(admissionErr, types.ErrorCodeChannelModelCapacityExhausted, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
		}
		return nil, types.NewError(admissionErr, types.ErrorCodeCountTokenFailed, types.ErrOptionWithSkipRetry())
	}
	if info.IsStream {
		usage, err = xunfeiStreamHandler(c, *a.request, splits[0], splits[1], splits[2], a.finalBody)
	} else {
		usage, err = xunfeiHandler(c, *a.request, splits[0], splits[1], splits[2], a.finalBody)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
