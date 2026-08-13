package astraflow_gemini

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type openAIImageUsage struct {
	TotalTokens         int                    `json:"total_tokens"`
	InputTokens         int                    `json:"input_tokens"`
	OutputTokens        int                    `json:"output_tokens"`
	InputTokensDetails  dto.InputTokenDetails  `json:"input_tokens_details"`
	OutputTokensDetails dto.OutputTokenDetails `json:"output_tokens_details"`
}

type openAIImageResponse struct {
	Created int64            `json:"created"`
	Data    []dto.ImageData  `json:"data"`
	Usage   openAIImageUsage `json:"usage"`
}

func handleImageResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("AstraFlow Gemini returned an empty response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var geminiResponse dto.GeminiChatResponse
	if err := common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}

	metadata := geminiResponse.GetUsageMetadata()
	if !dto.HasGeminiUsageMetadataTokens(metadata) {
		return nil, types.NewOpenAIError(errors.New("AstraFlow Gemini response is missing billable usageMetadata"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	usage := relayconvert.UsageFromGeminiMetadata(metadata, info.GetEstimatePromptTokens())
	if usage == nil {
		return nil, types.NewOpenAIError(errors.New("failed to normalize AstraFlow Gemini usageMetadata"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	usage.InputTokens = usage.PromptTokens
	usage.OutputTokens = usage.CompletionTokens
	inputDetails := usage.PromptTokensDetails
	usage.InputTokensDetails = &inputDetails

	images := make([]dto.ImageData, 0)
	for _, candidate := range geminiResponse.Candidates {
		var revisedPrompt strings.Builder
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				revisedPrompt.WriteString(part.Text)
			}
		}
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil || part.InlineData.Data == "" || !strings.HasPrefix(part.InlineData.MimeType, "image/") {
				continue
			}
			images = append(images, dto.ImageData{
				B64Json:       part.InlineData.Data,
				RevisedPrompt: revisedPrompt.String(),
			})
		}
	}
	if len(images) == 0 {
		return nil, types.NewOpenAIError(errors.New("AstraFlow Gemini response contains no generated image"), types.ErrorCodeEmptyResponse, http.StatusBadGateway)
	}

	clientResponse := openAIImageResponse{
		Created: common.GetTimestamp(),
		Data:    images,
		Usage: openAIImageUsage{
			TotalTokens:         usage.TotalTokens,
			InputTokens:         usage.PromptTokens,
			OutputTokens:        usage.CompletionTokens,
			InputTokensDetails:  usage.PromptTokensDetails,
			OutputTokensDetails: usage.CompletionTokenDetails,
		},
	}
	clientBody, err := common.Marshal(clientResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, clientBody)
	return usage, nil
}
