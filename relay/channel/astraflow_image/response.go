package astraflow_image

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type tokenDetails struct {
	TextTokens  *int `json:"text_tokens"`
	ImageTokens *int `json:"image_tokens"`
}

type astraFlowUsage struct {
	TotalTokens         *int          `json:"total_tokens"`
	InputTokens         *int          `json:"input_tokens"`
	OutputTokens        *int          `json:"output_tokens"`
	InputTokensDetails  *tokenDetails `json:"input_tokens_details"`
	OutputTokensDetails *tokenDetails `json:"output_tokens_details"`
}

type astraFlowImageResponse struct {
	Usage *astraFlowUsage `json:"usage"`
	Error any             `json:"error"`
}

func handleImageResponse(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var upstreamResponse astraFlowImageResponse
	if err := common.Unmarshal(responseBody, &upstreamResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if upstreamResponse.Error != nil {
		simpleResponse := dto.SimpleResponse{Error: upstreamResponse.Error}
		if openAIError := simpleResponse.GetOpenAIError(); openAIError != nil && openAIError.Message != "" {
			return nil, types.WithOpenAIError(*openAIError, resp.StatusCode)
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, types.NewOpenAIError(
			fmt.Errorf("AstraFlow returned HTTP %d", resp.StatusCode),
			types.ErrorCodeBadResponse,
			resp.StatusCode,
		)
	}
	if upstreamResponse.Usage == nil {
		return nil, types.NewOpenAIError(
			fmt.Errorf("AstraFlow image response is missing usage"),
			types.ErrorCodeBadResponseBody,
			http.StatusBadGateway,
		)
	}

	usage, err := normalizeUsage(upstreamResponse.Usage)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	if info != nil && info.PriceData.UsePrice {
		count := gjson.GetBytes(responseBody, "data.#").Int()
		if count > 0 && count <= int64(dto.MaxImageN) {
			info.PriceData.AddOtherRatio("n", float64(count))
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func normalizeUsage(raw *astraFlowUsage) (*dto.Usage, error) {
	if raw.InputTokens == nil || raw.OutputTokens == nil {
		return nil, fmt.Errorf("AstraFlow usage must include input_tokens and output_tokens")
	}
	if *raw.InputTokens < 0 || *raw.OutputTokens < 0 {
		return nil, fmt.Errorf("AstraFlow usage token counts must not be negative")
	}

	inputText, inputImage, err := normalizeDetails(*raw.InputTokens, raw.InputTokensDetails, "input")
	if err != nil {
		return nil, err
	}
	outputText, outputImage, err := normalizeDetails(*raw.OutputTokens, raw.OutputTokensDetails, "output")
	if err != nil {
		return nil, err
	}
	if raw.OutputTokensDetails == nil || outputText+outputImage == 0 {
		outputText = 0
		outputImage = *raw.OutputTokens
	}

	totalTokens := *raw.InputTokens + *raw.OutputTokens
	if raw.TotalTokens != nil {
		if *raw.TotalTokens < 0 {
			return nil, fmt.Errorf("AstraFlow total_tokens must not be negative")
		}
		if *raw.TotalTokens != totalTokens {
			return nil, fmt.Errorf("AstraFlow total_tokens does not equal input_tokens plus output_tokens")
		}
	}
	if totalTokens == 0 {
		return nil, fmt.Errorf("AstraFlow usage contains no billable tokens")
	}

	usage := &dto.Usage{
		PromptTokens:     *raw.InputTokens,
		CompletionTokens: *raw.OutputTokens,
		TotalTokens:      totalTokens,
		InputTokens:      *raw.InputTokens,
		OutputTokens:     *raw.OutputTokens,
		PromptTokensDetails: dto.InputTokenDetails{
			TextTokens:  inputText,
			ImageTokens: inputImage,
		},
		CompletionTokenDetails: dto.OutputTokenDetails{
			TextTokens:  outputText,
			ImageTokens: outputImage,
		},
	}
	usage.InputTokensDetails = &dto.InputTokenDetails{
		TextTokens:  inputText,
		ImageTokens: inputImage,
	}
	return usage, nil
}

func normalizeDetails(total int, details *tokenDetails, side string) (textTokens int, imageTokens int, err error) {
	if details != nil {
		if details.TextTokens != nil {
			textTokens = *details.TextTokens
		}
		if details.ImageTokens != nil {
			imageTokens = *details.ImageTokens
		}
	}
	if textTokens < 0 || imageTokens < 0 {
		return 0, 0, fmt.Errorf("AstraFlow %s token details must not be negative", side)
	}
	if textTokens+imageTokens > total {
		return 0, 0, fmt.Errorf("AstraFlow %s token details exceed the total", side)
	}
	imageTokens += total - textTokens - imageTokens
	return textTokens, imageTokens, nil
}
