package cloudflare

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

func convertCf2CompletionsRequest(textRequest dto.GeneralOpenAIRequest) *CfRequest {
	p, _ := textRequest.Prompt.(string)
	return &CfRequest{
		Prompt:      p,
		MaxTokens:   textRequest.GetMaxTokens(),
		Stream:      lo.FromPtrOr(textRequest.Stream, false),
		Temperature: textRequest.Temperature,
	}
}

func cfStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NewAPIError, *dto.Usage) {
	info.StreamStatus = relaycommon.NewStreamStatus()
	streamCtx, cancel := context.WithCancel(c.Request.Context())
	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	dataChan := make(chan string, 10)
	producerDone := make(chan struct{})
	idleWatchdog := helper.NewConfiguredStreamIdleWatchdog()
	var producerErr *types.NewAPIError

	helper.SetEventStreamHeaders(c)
	id := helper.GetResponseID(c)
	var responseText string
	go func() {
		defer close(producerDone)
		defer close(dataChan)
		for scanner.Scan() {
			idleWatchdog.Reset()
			data := scanner.Text()
			if len(data) < len("data: ") {
				continue
			}
			data = strings.TrimPrefix(data, "data: ")
			data = strings.TrimSuffix(data, "\r")
			if data == "[DONE]" {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				return
			}
			if helper.IsNullJSONStreamEvent(data) {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
				return
			}
			info.SetFirstResponseTime()
			var response dto.ChatCompletionsStreamResponse
			if err := common.Unmarshal([]byte(data), &response); err != nil {
				logger.LogError(c, "error_unmarshalling_stream_response: "+err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
				return
			}
			var errorEnvelope struct {
				Success *bool `json:"success"`
				Errors  []struct {
					Code    any    `json:"code"`
					Message string `json:"message"`
				} `json:"errors"`
				Error any `json:"error"`
			}
			if err := common.Unmarshal([]byte(data), &errorEnvelope); err == nil &&
				((errorEnvelope.Success != nil && !*errorEnvelope.Success) || len(errorEnvelope.Errors) > 0 || errorEnvelope.Error != nil) {
				message := "Cloudflare stream error"
				var code any = "upstream_error"
				if len(errorEnvelope.Errors) > 0 {
					if errorEnvelope.Errors[0].Message != "" {
						message = errorEnvelope.Errors[0].Message
					}
					code = errorEnvelope.Errors[0].Code
				}
				producerErr = types.WithOpenAIError(types.OpenAIError{
					Message: message,
					Type:    "upstream_error",
					Code:    code,
				}, helper.ResolveUpstreamStreamErrorStatus(data, resp.StatusCode))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, producerErr)
				return
			}
			var visibleText strings.Builder
			for _, choice := range response.Choices {
				visibleText.WriteString(choice.Delta.GetContentString())
			}
			info.RecordAttemptVisibleText(visibleText.String())
			if !helper.EnqueueStreamDataWithBackpressure(streamCtx, dataChan, data, info) {
				return
			}
		}
		if requestErr := c.Request.Context().Err(); requestErr != nil {
			info.StreamStatus.SetClientGone(requestErr)
		} else if err := scanner.Err(); err != nil {
			logger.LogError(c, "error_scanning_stream_response: "+err.Error())
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
		} else {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
		}
	}()
	watcherDone := helper.CloseUpstreamOnContext(c.Request.Context(), resp.Body, producerDone)
	idleWatcherDone := helper.CloseUpstreamOnIdleTimeout(c.Request.Context(), info, resp.Body, producerDone, idleWatchdog)
	defer func() {
		cancel()
		idleWatchdog.Stop()
		service.CloseResponseBodyGracefully(resp)
		<-producerDone
		<-watcherDone
		<-idleWatcherDone
	}()

	var clientWriteErr error
	for data := range dataChan {
		var response dto.ChatCompletionsStreamResponse
		err := json.Unmarshal([]byte(data), &response)
		if err != nil {
			logger.LogError(c, "error_unmarshalling_stream_response: "+err.Error())
			continue
		}
		for _, choice := range response.Choices {
			choice.Delta.Role = "assistant"
			content := choice.Delta.GetContentString()
			responseText += content
		}
		response.Id = id
		response.Model = info.UpstreamModelName
		err = helper.ObjectData(c, response)
		if err != nil {
			logger.LogError(c, "error_rendering_stream_response: "+err.Error())
			info.StreamStatus.SetClientGone(err)
			clientWriteErr = err
			break
		}
	}

	if requestErr := c.Request.Context().Err(); requestErr != nil {
		info.StreamStatus.SetClientGone(requestErr)
	}
	if clientWriteErr != nil {
		return types.NewError(clientWriteErr, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry()), nil
	}
	if producerErr != nil {
		return producerErr, nil
	}
	usage := service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	if info.ShouldIncludeUsage {
		response := helper.GenerateFinalUsageResponse(id, info.StartTime.Unix(), info.UpstreamModelName, *usage)
		err := helper.ObjectData(c, response)
		if err != nil {
			logger.LogError(c, "error_rendering_final_usage_response: "+err.Error())
			info.StreamStatus.SetClientGone(err)
			return types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry()), usage
		}
	}
	if err := helper.StringData(c, "[DONE]"); err != nil {
		info.StreamStatus.SetClientGone(err)
		return types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry()), usage
	}

	return nil, usage
}

func cfHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NewAPIError, *dto.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.CloseResponseBodyGracefully(resp)
	var response dto.TextResponse
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	response.Model = info.UpstreamModelName
	var responseText string
	for _, choice := range response.Choices {
		responseText += choice.Message.StringContent()
	}
	usage := service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	response.Usage = *usage
	response.Id = helper.GetResponseID(c)
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, usage
}

func cfSTTHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NewAPIError, *dto.Usage) {
	var cfResp CfAudioResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &cfResp)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}

	audioResp := &dto.AudioResponse{
		Text: cfResp.Result.Text,
	}

	jsonResponse, err := json.Marshal(audioResp)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)

	usage := service.ResponseText2Usage(c, cfResp.Result.Text, info.UpstreamModelName, info.GetEstimatePromptTokens())
	return nil, usage
}
