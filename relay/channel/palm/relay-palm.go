package palm

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// https://developers.generativeai.google/api/rest/generativelanguage/models/generateMessage#request-body
// https://developers.generativeai.google/api/rest/generativelanguage/models/generateMessage#response-body

func responsePaLM2OpenAI(response *PaLMChatResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(response.Candidates)),
	}
	for i, candidate := range response.Candidates {
		choice := dto.OpenAITextResponseChoice{
			Index: i,
			Message: dto.Message{
				Role:    "assistant",
				Content: candidate.Content,
			},
			FinishReason: "stop",
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func streamResponsePaLM2OpenAI(palmResponse *PaLMChatResponse) *dto.ChatCompletionsStreamResponse {
	var choice dto.ChatCompletionsStreamResponseChoice
	if len(palmResponse.Candidates) > 0 {
		choice.Delta.SetContentString(palmResponse.Candidates[0].Content)
	}
	choice.FinishReason = &constant.FinishReasonStop
	var response dto.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Model = "palm2"
	response.Choices = []dto.ChatCompletionsStreamResponseChoice{choice}
	return &response
}

func palmStreamErrorStatus(code int, data string, responseStatus int) int {
	switch code {
	case 1, 2: // CANCELLED, UNKNOWN
		return http.StatusBadGateway
	case 3: // INVALID_ARGUMENT
		return http.StatusBadRequest
	case 4: // DEADLINE_EXCEEDED
		return http.StatusGatewayTimeout
	case 5: // NOT_FOUND: this adapter calls a fixed model endpoint
		return http.StatusBadGateway
	case 6: // ALREADY_EXISTS
		return http.StatusConflict
	case 7: // PERMISSION_DENIED
		return http.StatusForbidden
	case 8: // RESOURCE_EXHAUSTED
		return http.StatusTooManyRequests
	case 9: // FAILED_PRECONDITION
		return http.StatusPreconditionFailed
	case 10: // ABORTED: retryable upstream concurrency failure
		return http.StatusServiceUnavailable
	case 11: // OUT_OF_RANGE
		return http.StatusBadRequest
	case 12: // UNIMPLEMENTED
		return http.StatusNotImplemented
	case 13: // INTERNAL
		return http.StatusInternalServerError
	case 14: // UNAVAILABLE
		return http.StatusServiceUnavailable
	case 15: // DATA_LOSS
		return http.StatusInternalServerError
	case 16: // UNAUTHENTICATED
		return http.StatusUnauthorized
	default:
		return helper.ResolveUpstreamStreamErrorStatus(data, responseStatus)
	}
}

func palmStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NewAPIError, string) {
	info.StreamStatus = relaycommon.NewStreamStatus()
	responseId := helper.GetResponseID(c)
	createdTime := common.GetTimestamp()
	readCompleted := make(chan struct{})
	idleWatchdog := helper.NewConfiguredStreamIdleWatchdog()
	watcherDone := helper.CloseUpstreamOnContext(c.Request.Context(), resp.Body, readCompleted)
	idleWatcherDone := helper.CloseUpstreamOnIdleTimeout(c.Request.Context(), info, resp.Body, readCompleted, idleWatchdog)
	responseBody, err := io.ReadAll(idleWatchdog.WrapReadCloser(resp.Body))
	close(readCompleted)
	idleWatchdog.Stop()
	<-watcherDone
	<-idleWatcherDone
	if err != nil {
		if requestErr := c.Request.Context().Err(); requestErr != nil {
			info.StreamStatus.SetClientGone(requestErr)
		} else {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
		}
		return types.NewError(err, types.ErrorCodeBadResponseBody), ""
	}
	service.CloseResponseBodyGracefully(resp)
	if helper.IsNullJSONStreamEvent(string(responseBody)) {
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
		return types.NewError(helper.ErrNullJSONStreamEvent, types.ErrorCodeBadResponseBody), ""
	}
	var palmResponse PaLMChatResponse
	if err = common.Unmarshal(responseBody, &palmResponse); err != nil {
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
		return types.NewError(err, types.ErrorCodeBadResponseBody), ""
	}
	info.SetFirstResponseTime()
	if palmResponse.Error.Code != 0 || palmResponse.Error.Message != "" {
		statusCode := palmStreamErrorStatus(palmResponse.Error.Code, string(responseBody), resp.StatusCode)
		errorCode := any(palmResponse.Error.Code)
		if palmResponse.Error.Code == 5 {
			errorCode = types.ErrorCodeModelNotFound
		}
		apiErr := types.WithOpenAIError(types.OpenAIError{
			Message: palmResponse.Error.Message,
			Type:    palmResponse.Error.Status,
			Code:    errorCode,
		}, statusCode)
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, apiErr)
		return apiErr, ""
	}
	responseText := ""
	if len(palmResponse.Candidates) > 0 {
		responseText = palmResponse.Candidates[0].Content
		info.RecordAttemptVisibleText(responseText)
	}
	fullTextResponse := streamResponsePaLM2OpenAI(&palmResponse)
	fullTextResponse.Id = responseId
	fullTextResponse.Created = createdTime
	helper.SetEventStreamHeaders(c)
	if err = helper.ObjectData(c, fullTextResponse); err != nil {
		info.StreamStatus.SetClientGone(err)
		return types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry()), responseText
	}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	if err = helper.Done(c); err != nil {
		info.StreamStatus.SetClientGone(err)
		return types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry()), responseText
	}
	return nil, responseText
}

func palmHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	var palmResponse PaLMChatResponse
	err = json.Unmarshal(responseBody, &palmResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if palmResponse.Error.Code != 0 || len(palmResponse.Candidates) == 0 {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: palmResponse.Error.Message,
			Type:    palmResponse.Error.Status,
			Param:   "",
			Code:    palmResponse.Error.Code,
		}, resp.StatusCode)
	}
	fullTextResponse := responsePaLM2OpenAI(&palmResponse)
	usage := service.ResponseText2Usage(c, palmResponse.Candidates[0].Content, info.UpstreamModelName, info.GetEstimatePromptTokens())
	fullTextResponse.Usage = *usage
	jsonResponse, err := common.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return usage, nil
}
