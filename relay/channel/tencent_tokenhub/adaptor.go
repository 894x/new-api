package tencent_tokenhub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
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
	ChannelName         = "tencent_tokenhub"
	ModelHYImageLite    = "hy-image-lite"
	ModelHYImageV3      = "hy-image-v3.0"
	defaultPollAttempts = 60
	defaultPollInterval = 5 * time.Second
)

var ModelList = []string{
	ModelHYImageLite,
	ModelHYImageV3,
	"hy-video-1.5",
	"yt-video-2.0",
	"yt-video-fx",
	"yt-video-humanactor",
	"kl-video-v3",
	"kl-video-v2-6",
	"kl-video-v2-5-turbo",
	"kl-video-v2-1-master",
	"kl-video-v2-1",
	"vd-video-q3-pro",
	"vd-video-q3-turbo",
}

type Adaptor struct {
	pollAttempts int
	pollInterval time.Duration
}

type imageRequest struct {
	Model          string                     `json:"model"`
	Prompt         string                     `json:"prompt"`
	Images         json.RawMessage            `json:"images,omitempty"`
	Resolution     string                     `json:"resolution,omitempty"`
	Style          json.RawMessage            `json:"style,omitempty"`
	RspImgType     string                     `json:"rsp_img_type,omitempty"`
	NegativePrompt json.RawMessage            `json:"negative_prompt,omitempty"`
	Seed           json.RawMessage            `json:"seed,omitempty"`
	LogoAdd        json.RawMessage            `json:"logo_add,omitempty"`
	LogoParam      json.RawMessage            `json:"logo_param,omitempty"`
	Revise         json.RawMessage            `json:"revise,omitempty"`
	Extra          map[string]json.RawMessage `json:"-"`
}

func (r imageRequest) MarshalJSON() ([]byte, error) {
	type alias imageRequest
	knownData, err := common.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	known := make(map[string]json.RawMessage)
	if err := common.Unmarshal(knownData, &known); err != nil {
		return nil, err
	}
	merged := make(map[string]json.RawMessage, len(r.Extra)+len(known))
	for key, value := range r.Extra {
		merged[key] = value
	}
	for key, value := range known {
		merged[key] = value
	}
	return common.Marshal(merged)
}

type imageTaskResponse struct {
	ID        string          `json:"id"`
	Status    string          `json:"status"`
	Message   string          `json:"message,omitempty"`
	Error     json.RawMessage `json:"error,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
}

func (a *Adaptor) Init(_ *relaycommon.RelayInfo) {
	if a.pollAttempts == 0 {
		a.pollAttempts = defaultPollAttempts
	}
	if a.pollInterval == 0 {
		a.pollInterval = defaultPollInterval
	}
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("missing relay info")
	}
	switch info.UpstreamModelName {
	case ModelHYImageLite:
		return strings.TrimRight(info.ChannelBaseUrl, "/") + "/v1/api/image/lite", nil
	case ModelHYImageV3:
		return strings.TrimRight(info.ChannelBaseUrl, "/") + "/v1/api/image/submit", nil
	default:
		return "", fmt.Errorf("model %q does not support the image generation endpoint", info.UpstreamModelName)
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	return nil
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil || info.RelayMode != relayconstant.RelayModeImagesGenerations {
		return nil, errors.New("Tencent TokenHub only supports image generations")
	}
	if request.N != nil && *request.N != 1 {
		return nil, errors.New("Tencent TokenHub image models support exactly one image per request")
	}
	if !IsImageModel(info.UpstreamModelName) {
		return nil, fmt.Errorf("unsupported Tencent TokenHub image model: %s", info.UpstreamModelName)
	}

	converted := imageRequest{
		Model:      info.UpstreamModelName,
		Prompt:     request.Prompt,
		Resolution: strings.Replace(request.Size, "x", ":", 1),
		Images:     request.Images,
		Style:      request.Style,
		Extra:      request.Extra,
	}
	if info.UpstreamModelName == ModelHYImageLite {
		converted.RspImgType = "url"
		if request.ResponseFormat == "b64_json" {
			converted.RspImgType = "base64"
		}
	}
	if len(converted.Images) == 0 && len(request.Image) > 0 {
		var imageURL string
		if err := common.Unmarshal(request.Image, &imageURL); err == nil && imageURL != "" {
			converted.Images, _ = common.Marshal([]string{imageURL})
		}
	}
	if raw, ok := request.Extra["resolution"]; ok {
		var resolution string
		if err := common.Unmarshal(raw, &resolution); err != nil {
			return nil, fmt.Errorf("resolution must be a string: %w", err)
		}
		converted.Resolution = resolution
	}
	converted.NegativePrompt = request.Extra["negative_prompt"]
	converted.Seed = request.Extra["seed"]
	converted.LogoAdd = request.Extra["logo_add"]
	converted.LogoParam = request.Extra["logo_param"]
	converted.Revise = request.Extra["revise"]
	if request.Watermark != nil {
		logoAdd := 0
		if *request.Watermark {
			logoAdd = 1
		}
		converted.LogoAdd, _ = common.Marshal(logoAdd)
	}
	return &converted, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	response, err := channel.DoApiRequest(a, c, info, requestBody)
	if err != nil || response == nil || info.UpstreamModelName != ModelHYImageV3 || response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response, err
	}

	submitBody, err := io.ReadAll(response.Body)
	service.CloseResponseBodyGracefully(response)
	if err != nil {
		return nil, fmt.Errorf("read Tencent TokenHub image submit response: %w", err)
	}
	var submit imageTaskResponse
	if err := common.Unmarshal(submitBody, &submit); err != nil {
		return nil, fmt.Errorf("decode Tencent TokenHub image submit response: %w", err)
	}
	if submit.ID == "" {
		return nil, fmt.Errorf("Tencent TokenHub image submit response is missing id: %s", submitBody)
	}

	for attempt := 0; attempt < a.pollAttempts; attempt++ {
		queryResponse, queryBody, err := a.queryImageTask(c, info, submit.ID)
		if err != nil {
			return nil, err
		}
		if queryResponse.StatusCode < http.StatusOK || queryResponse.StatusCode >= http.StatusMultipleChoices {
			queryResponse.Body = io.NopCloser(bytes.NewReader(queryBody))
			return queryResponse, nil
		}

		var query imageTaskResponse
		if err := common.Unmarshal(queryBody, &query); err != nil {
			return nil, fmt.Errorf("decode Tencent TokenHub image query response: %w", err)
		}
		switch query.Status {
		case "completed":
			queryResponse.Body = io.NopCloser(bytes.NewReader(queryBody))
			return queryResponse, nil
		case "failed", "cancelled":
			return nil, fmt.Errorf("Tencent TokenHub image task %s: %s", query.Status, tokenHubTaskMessage(query, queryBody))
		case "queued", "pending", "processing", "in_progress":
		default:
			return nil, fmt.Errorf("unknown Tencent TokenHub image task status %q", query.Status)
		}
		if attempt == a.pollAttempts-1 {
			break
		}

		timer := time.NewTimer(a.pollInterval)
		select {
		case <-c.Request.Context().Done():
			timer.Stop()
			return nil, c.Request.Context().Err()
		case <-timer.C:
		}
	}
	return nil, errors.New("Tencent TokenHub image task polling timed out")
}

func (a *Adaptor) queryImageTask(c *gin.Context, info *relaycommon.RelayInfo, taskID string) (*http.Response, []byte, error) {
	payload, err := common.Marshal(map[string]string{"model": info.UpstreamModelName, "id": taskID})
	if err != nil {
		return nil, nil, err
	}
	url := strings.TrimRight(info.ChannelBaseUrl, "/") + "/v1/api/image/query"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	body, err := io.ReadAll(resp.Body)
	service.CloseResponseBodyGracefully(resp)
	if err != nil {
		return nil, nil, err
	}
	return resp, body, nil
}

func tokenHubTaskMessage(response imageTaskResponse, raw []byte) string {
	if response.Message != "" {
		return response.Message
	}
	if len(response.Error) > 0 {
		var detail struct {
			Message string `json:"message"`
		}
		if err := common.Unmarshal(response.Error, &detail); err == nil && detail.Message != "" {
			return detail.Message
		}
	}
	return string(raw)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if info.RelayMode != relayconstant.RelayModeImagesGenerations {
		return nil, types.NewError(errors.New("Tencent TokenHub only supports image generations"), types.ErrorCodeInvalidRequest)
	}
	return openai.OpenaiImageHandler(c, info, resp)
}

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("Tencent TokenHub does not support chat completions")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("Tencent TokenHub does not support Claude messages")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("Tencent TokenHub does not support Gemini requests")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("Tencent TokenHub does not support OpenAI responses")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("Tencent TokenHub does not support embeddings")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("Tencent TokenHub does not support audio")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("Tencent TokenHub does not support rerank")
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func IsImageModel(modelName string) bool {
	return modelName == ModelHYImageLite || modelName == ModelHYImageV3
}
