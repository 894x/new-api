package astraflow_gemini

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	gemini.Adaptor
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if err := validateModel(info); err != nil {
		return nil, err
	}
	return a.Adaptor.ConvertGeminiRequest(c, info, request)
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if err := validateModel(info); err != nil {
		return nil, err
	}
	return a.Adaptor.ConvertOpenAIRequest(c, info, request)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits {
		return handleImageResponse(c, resp, info)
	}
	return a.Adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func validateModel(info *relaycommon.RelayInfo) error {
	if info == nil || info.ChannelMeta == nil {
		return fmt.Errorf("relay info is required")
	}
	if info.UpstreamModelName != ModelName {
		return fmt.Errorf("model %q is not supported by %s", info.UpstreamModelName, ChannelName)
	}
	return nil
}
