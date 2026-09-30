package openai

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	info.ObserveResponseModel(responsesResponse.Model)
	responseBody = rewriteSGLangResponsesCreatedAt(info, responseBody, "created_at", responsesResponse.CreatedAt)
	if info.ChannelMeta != nil && info.ChannelSetting.ForceFormat && responsesResponse.Usage != nil {
		responseBody, err = addChatUsageAliasesToResponsesBody(responseBody, "usage", responsesResponse.Usage)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := &dto.Usage{}
	service.ApplyResponsesUsage(usage, responsesResponse.Usage)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	return usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	accumulator := service.NewResponsesUsageAccumulator(info)
	var streamErr *types.NewAPIError

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		clientData, isStreamError := service.StreamErrorDataForClient(c, data)

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.ScannerError(err)
			return
		}
		if streamResponse.Response != nil {
			clientData = string(rewriteSGLangResponsesCreatedAt(info, []byte(clientData), "response.created_at", streamResponse.Response.CreatedAt))
			if info.ChannelMeta != nil && info.ChannelSetting.ForceFormat && !isStreamError && streamResponse.Response.Usage != nil {
				clientDataBytes, err := addChatUsageAliasesToResponsesBody([]byte(clientData), "response.usage", streamResponse.Response.Usage)
				if err != nil {
					sr.ScannerError(err)
					return
				}
				clientData = string(clientDataBytes)
			}
		}
		accumulator.Observe(&streamResponse)
		if isStreamError || streamResponse.Type == "response.failed" || streamResponse.Type == "response.error" {
			// Classify the upstream event before attempting delivery: a failed
			// write must not replace a known provider failure with client_gone.
			streamErr = newResponsesStreamError(data, resp.StatusCode)
		}
		if err := sendResponsesStreamData(c, streamResponse, clientData); err != nil {
			if streamErr != nil {
				streamErr = markStreamErrorIfCommitted(c, streamErr)
				// Observe already recorded the provider's precise code. The
				// transport wrapper must not overwrite it with bad_response.
				sr.Stop(nil)
				return
			}
			if !helper.IsDownstreamWriteError(err) {
				streamErr = types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
				sr.Stop(streamErr)
				return
			}
			streamErr = markStreamErrorIfCommitted(c, types.NewClientGoneError(err))
			sr.ClientGone(err)
			return
		}
		if streamErr != nil {
			streamErr = markStreamErrorIfCommitted(c, streamErr)
			sr.Stop(nil)
			return
		}
		if streamResponse.Type == "response.completed" || streamResponse.Type == "response.done" {
			sr.Done()
		}
	})
	common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, info.StreamStatus)
	info.StreamStatus.RequireTerminal()
	if reason, err := info.StreamStatus.End(); streamErr == nil && reason == relaycommon.StreamEndReasonClientGone {
		streamErr = markStreamErrorIfCommitted(c, types.NewClientGoneError(err))
	}
	if streamErr != nil {
		return accumulator.Finish(), streamErr
	}

	return accumulator.Finish(), nil
}

func rewriteSGLangResponsesCreatedAt(info *relaycommon.RelayInfo, payload []byte, path string, createdAt dto.IntValue) []byte {
	if info.GetChannelType() != constant.ChannelTypeSGLang {
		return payload
	}
	if !gjson.GetBytes(payload, path).Exists() {
		return payload
	}
	patched, err := sjson.SetBytes(payload, path, int(createdAt))
	if err != nil {
		return payload
	}
	return patched
}
