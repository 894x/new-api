package openai

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	syntheticStream := info != nil && info.IsStream
	if syntheticStream {
		info.StreamStatus = relaycommon.NewStreamStatus()
	}
	if resp == nil || resp.Body == nil {
		handlerErr := types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusBadGateway)
		if syntheticStream {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, handlerErr)
		}
		return nil, handlerErr
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		handlerErr := types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
		if syntheticStream {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, handlerErr)
		}
		return nil, handlerErr
	}
	if helper.IsNullJSONStreamEvent(string(body)) {
		handlerErr := types.NewOpenAIError(helper.ErrNullJSONStreamEvent, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		if syntheticStream {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, handlerErr)
		}
		return nil, handlerErr
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		handlerErr := types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		if syntheticStream {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, handlerErr)
		}
		return nil, handlerErr
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.IsPresent() {
		handlerErr := newResponsesStreamError(string(body), resp.StatusCode)
		if syntheticStream {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, handlerErr)
		}
		return nil, handlerErr
	}
	if syntheticStream {
		// This compatibility path received a complete JSON response for a client
		// stream request. Reading and validating that body is the first moment at
		// which upstream output can be known. Capture it before local conversion,
		// fallback token counting, serialization, or any downstream write.
		info.SetFirstResponseTime()
		info.RecordAttemptVisibleText(service.ExtractOutputTextFromResponses(&responsesResp))
	}

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, &responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	for key, values := range resp.Header {
		if !service.ShouldCopyUpstreamHeader(c, key, values) || len(values) == 0 {
			continue
		}
		c.Writer.Header().Set(key, values[0])
	}
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(responseBody)))
	c.Writer.WriteHeader(resp.StatusCode)
	helper.ExtendWriteDeadline(c)
	if _, err := c.Writer.Write(responseBody); err != nil {
		if syntheticStream {
			info.StreamStatus.SetClientGone(err)
		}
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	if err := http.NewResponseController(c.Writer).Flush(); err != nil {
		if syntheticStream {
			info.StreamStatus.SetClientGone(err)
		}
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	if syntheticStream {
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
	return usage, nil
}

func OaiResponsesToChatBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	accumulator := relayconvert.NewResponsesBufferedAccumulator()
	var finalResponse *dto.OpenAIResponsesResponse
	var streamErr *types.NewAPIError

	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 6 || line[:5] != "data:" {
			continue
		}
		data := line[5:]
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				break
			}
			continue
		}
		if helper.IsNullJSONStreamEvent(data) {
			streamErr = types.NewOpenAIError(helper.ErrNullJSONStreamEvent, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			break
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal buffered responses stream event: "+err.Error())
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			break
		}
		accumulator.ProcessEvent(&streamResp)
		switch streamResp.Type {
		case "response.completed", "response.done", "response.incomplete":
			finalResponse = streamResp.Response
			if streamResp.Type == "response.incomplete" {
				if finalResponse == nil {
					finalResponse = &dto.OpenAIResponsesResponse{}
				}
				if len(finalResponse.Status) == 0 {
					finalResponse.Status = []byte(`"incomplete"`)
				}
			}
		case "response.failed", "response.error":
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					break
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if streamErr != nil || finalResponse != nil {
			break
		}
	}
	if streamErr != nil {
		return nil, streamErr
	}
	if err := scanner.Err(); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if finalResponse == nil {
		finalResponse = &dto.OpenAIResponsesResponse{
			ID:        helper.GetResponseID(c),
			CreatedAt: int(time.Now().Unix()),
			Model:     info.UpstreamModelName,
			Status:    []byte(`"completed"`),
		}
	}
	accumulator.SupplementResponseOutput(finalResponse)

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, finalResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(finalResponse)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAIResponses, info.RelayFormat, relayconvert.ResponseStreamOptions{
		ID:      responseId,
		Model:   info.UpstreamModelName,
		Created: createAt,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	streamErr := (*types.NewAPIError)(nil)
	var clientWriteErr error

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendGeminiResponse := func(geminiResponse *dto.GeminiChatResponse) bool {
		if geminiResponse == nil {
			return true
		}
		geminiResponseStr, err := common.Marshal(geminiResponse)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := helper.StringData(c, string(geminiResponseStr)); err != nil {
			clientWriteErr = err
			streamErr = types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
			return false
		}
		return true
	}

	sendStreamResult := func(result relayconvert.ResponseResult) bool {
		switch value := result.Value.(type) {
		case dto.ChatCompletionsStreamResponse:
			if len(value.Choices) == 0 && value.Usage == nil {
				return true
			}
			if err := helper.ObjectData(c, &value); err != nil {
				clientWriteErr = err
				streamErr = types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
				return false
			}
			return true
		case *dto.ChatCompletionsStreamResponse:
			if value == nil || (len(value.Choices) == 0 && value.Usage == nil) {
				return true
			}
			if err := helper.ObjectData(c, value); err != nil {
				clientWriteErr = err
				streamErr = types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
				return false
			}
			return true
		case dto.ClaudeResponse:
			if err := helper.ClaudeData(c, value); err != nil {
				clientWriteErr = err
				streamErr = types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
				return false
			}
			return true
		case *dto.ClaudeResponse:
			if value == nil {
				return true
			}
			if err := helper.ClaudeData(c, *value); err != nil {
				clientWriteErr = err
				streamErr = types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
				return false
			}
			return true
		case dto.GeminiChatResponse:
			return sendGeminiResponse(&value)
		case *dto.GeminiChatResponse:
			return sendGeminiResponse(value)
		default:
			streamErr = types.NewOpenAIError(fmt.Errorf("unsupported converted stream response type %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		_, isStreamError := service.StreamErrorDataForClient(c, data)
		if isStreamError {
			streamErr = markStreamErrorIfCommitted(c, newResponsesStreamError(data, resp.StatusCode))
			sr.Stop(streamErr)
			return
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			sr.ScannerError(err)
			return
		}

		if streamResp.Type == "response.error" || streamResp.Type == "response.failed" {
			streamErr = newResponsesStreamError(data, resp.StatusCode)
			sr.Stop(streamErr)
			return
		}

		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &streamResp)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			if !sendStreamResult(result) {
				if clientWriteErr != nil {
					sr.ClientGone(clientWriteErr)
				} else {
					sr.Stop(streamErr)
				}
				return
			}
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}

	usage := state.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
		info.ClaudeConvertInfo.Usage = usage
	}
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		if !sendStreamResult(result) {
			if clientWriteErr != nil && info.StreamStatus != nil {
				info.StreamStatus.SetClientGone(clientWriteErr)
			}
			return nil, streamErr
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, info.UpstreamModelName, *usage)); err != nil {
			if info.StreamStatus != nil {
				info.StreamStatus.SetClientGone(err)
			}
			return nil, types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		if err := helper.Done(c); err != nil {
			if info.StreamStatus != nil {
				info.StreamStatus.SetClientGone(err)
			}
			return nil, types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
		}
	}
	return usage, nil
}
