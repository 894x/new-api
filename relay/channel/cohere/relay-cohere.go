package cohere

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func requestOpenAI2Cohere(textRequest dto.GeneralOpenAIRequest) *CohereRequest {
	cohereReq := CohereRequest{
		Model:       textRequest.Model,
		ChatHistory: []ChatHistory{},
		Message:     "",
		Stream:      lo.FromPtrOr(textRequest.Stream, false),
		MaxTokens:   textRequest.GetMaxTokens(),
	}
	if common.CohereSafetySetting != "NONE" {
		cohereReq.SafetyMode = common.CohereSafetySetting
	}
	if cohereReq.MaxTokens == 0 {
		cohereReq.MaxTokens = 4000
	}
	for _, msg := range textRequest.Messages {
		if msg.Role == "user" {
			cohereReq.Message = msg.StringContent()
		} else {
			var role string
			if msg.Role == "assistant" {
				role = "CHATBOT"
			} else if msg.Role == "system" {
				role = "SYSTEM"
			} else {
				role = "USER"
			}
			cohereReq.ChatHistory = append(cohereReq.ChatHistory, ChatHistory{
				Role:    role,
				Message: msg.StringContent(),
			})
		}
	}

	return &cohereReq
}

func requestConvertRerank2Cohere(rerankRequest dto.RerankRequest) *CohereRerankRequest {
	topN := lo.FromPtrOr(rerankRequest.TopN, 1)
	if topN <= 0 {
		topN = 1
	}
	cohereReq := CohereRerankRequest{
		Query:           rerankRequest.Query,
		Documents:       rerankRequest.Documents,
		Model:           rerankRequest.Model,
		TopN:            topN,
		ReturnDocuments: true,
	}
	return &cohereReq
}

func stopReasonCohere2OpenAI(reason string) string {
	switch reason {
	case "COMPLETE":
		return "stop"
	case "MAX_TOKENS":
		return "max_tokens"
	default:
		return reason
	}
}

func isCohereStreamEnd(response *CohereResponse) bool {
	if response == nil {
		return false
	}
	return response.IsFinished || strings.EqualFold(strings.TrimSpace(response.EventType), "stream-end")
}

func cohereFinishReason(response *CohereResponse) string {
	if response == nil {
		return ""
	}
	reason := strings.ToUpper(strings.TrimSpace(response.FinishReason))
	if reason == "" && response.Response != nil {
		reason = strings.ToUpper(strings.TrimSpace(response.Response.FinishReason))
	}
	return reason
}

type cohereStreamAttemptObserver interface {
	SetFirstResponseTime()
	RecordAttemptVisibleText(string)
}

func observeCohereStreamAttempt(observer cohereStreamAttemptObserver, data string, firstResponseRecorded bool) (bool, error) {
	if !firstResponseRecorded {
		observer.SetFirstResponseTime()
		firstResponseRecorded = true
	}
	var response CohereResponse
	if err := common.Unmarshal([]byte(data), &response); err != nil {
		return firstResponseRecorded, err
	}
	if !isCohereStreamEnd(&response) && response.Text != "" {
		observer.RecordAttemptVisibleText(response.Text)
	}
	return firstResponseRecorded, nil
}

func cohereStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	info.StreamStatus = relaycommon.NewStreamStatus()
	streamCtx, cancel := context.WithCancel(c.Request.Context())
	responseId := helper.GetResponseID(c)
	createdTime := common.GetTimestamp()
	usage := &dto.Usage{}
	responseText := ""
	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := strings.Index(string(data), "\n"); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	dataChan := make(chan string, 10)
	producerDone := make(chan struct{})
	idleWatchdog := helper.NewConfiguredStreamIdleWatchdog()
	var producerErr *types.NewAPIError
	go func() {
		defer close(producerDone)
		defer close(dataChan)
		firstResponseRecorded := false
		for scanner.Scan() {
			idleWatchdog.Reset()
			data := strings.TrimSuffix(scanner.Text(), "\r")
			if helper.IsNullJSONStreamEvent(data) {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
				return
			}
			var envelope CohereResponse
			if err := common.Unmarshal([]byte(data), &envelope); err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
				return
			}
			if envelope.EventType == "stream-error" || envelope.EventType == "error" || envelope.Error != nil {
				message := envelope.Message
				if message == "" {
					message = "Cohere stream error"
				}
				producerErr = types.NewOpenAIError(
					fmt.Errorf("%s", message),
					types.ErrorCodeBadResponse,
					helper.ResolveUpstreamStreamErrorStatus(data, resp.StatusCode),
				)
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, producerErr)
				return
			}
			streamEnd := isCohereStreamEnd(&envelope)
			if streamEnd {
				finishReason := cohereFinishReason(&envelope)
				statusCode := 0
				switch finishReason {
				case "ERROR":
					statusCode = http.StatusBadGateway
				case "ERROR_TOXIC":
					statusCode = http.StatusUnprocessableEntity
				case "TIMEOUT":
					statusCode = http.StatusRequestTimeout
				}
				if statusCode != 0 {
					producerErr = types.NewOpenAIError(
						fmt.Errorf("Cohere stream ended with finish reason %s", finishReason),
						types.ErrorCodeBadResponse,
						statusCode,
					)
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, producerErr)
					return
				}
			}
			if !streamEnd {
				var observeErr error
				firstResponseRecorded, observeErr = observeCohereStreamAttempt(info, data, firstResponseRecorded)
				if observeErr != nil {
					common.SysLog("error unmarshalling stream response: " + observeErr.Error())
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, observeErr)
					return
				}
			}
			if !helper.EnqueueStreamDataWithBackpressure(streamCtx, dataChan, data, info) {
				info.StreamStatus.SetClientGone(c.Request.Context().Err())
				return
			}
			if streamEnd {
				return
			}
		}
		if requestErr := c.Request.Context().Err(); requestErr != nil {
			info.StreamStatus.SetClientGone(requestErr)
		} else if err := scanner.Err(); err != nil {
			common.SysLog("error reading stream: " + err.Error())
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
		}
	}()
	watcherDone := helper.CloseUpstreamOnContext(streamCtx, resp.Body, producerDone)
	idleWatcherDone := helper.CloseUpstreamOnIdleTimeout(streamCtx, info, resp.Body, producerDone, idleWatchdog)
	defer func() {
		cancel()
		idleWatchdog.Stop()
		service.CloseResponseBodyGracefully(resp)
		<-producerDone
		<-watcherDone
		<-idleWatcherDone
	}()
	helper.SetEventStreamHeaders(c)
	var clientWriteErr error
	clientGone := c.Stream(func(w io.Writer) bool {
		data, ok := <-dataChan
		if !ok {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
			if err := helper.StringData(c, "[DONE]"); err != nil {
				clientWriteErr = err
				info.StreamStatus.SetClientGone(err)
			}
			return false
		}
		var cohereResp CohereResponse
		err := common.Unmarshal([]byte(data), &cohereResp)
		if err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			info.StreamStatus.RecordError(err.Error())
			return true
		}
		var openaiResp dto.ChatCompletionsStreamResponse
		openaiResp.Id = responseId
		openaiResp.Created = createdTime
		openaiResp.Object = "chat.completion.chunk"
		openaiResp.Model = info.UpstreamModelName
		if isCohereStreamEnd(&cohereResp) {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			finishReason := stopReasonCohere2OpenAI(cohereFinishReason(&cohereResp))
			openaiResp.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{},
					Index:        0,
					FinishReason: &finishReason,
				},
			}
			if cohereResp.Response != nil {
				usage.PromptTokens = cohereResp.Response.Meta.BilledUnits.InputTokens
				usage.CompletionTokens = cohereResp.Response.Meta.BilledUnits.OutputTokens
			}
		} else {
			openaiResp.Choices = []dto.ChatCompletionsStreamResponseChoice{
				{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						Role:    "assistant",
						Content: &cohereResp.Text,
					},
					Index: 0,
				},
			}
			responseText += cohereResp.Text
		}
		jsonStr, err := common.Marshal(openaiResp)
		if err != nil {
			common.SysLog("error marshalling stream response: " + err.Error())
			info.StreamStatus.RecordError(err.Error())
			return true
		}
		if err := helper.StringData(c, string(jsonStr)); err != nil {
			clientWriteErr = err
			info.StreamStatus.SetClientGone(err)
			return false
		}
		return true
	})
	if clientGone && clientWriteErr == nil {
		info.StreamStatus.SetClientGone(c.Request.Context().Err())
	}
	if clientWriteErr != nil {
		return usage, types.NewError(clientWriteErr, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	if producerErr != nil {
		return usage, producerErr
	}
	if usage.PromptTokens == 0 {
		usage = service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	}
	return usage, nil
}

func cohereHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	createdTime := common.GetTimestamp()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp)
	var cohereResp CohereResponseResult
	err = common.Unmarshal(responseBody, &cohereResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	usage := dto.Usage{}
	usage.PromptTokens = cohereResp.Meta.BilledUnits.InputTokens
	usage.CompletionTokens = cohereResp.Meta.BilledUnits.OutputTokens
	usage.TotalTokens = cohereResp.Meta.BilledUnits.InputTokens + cohereResp.Meta.BilledUnits.OutputTokens

	var openaiResp dto.TextResponse
	openaiResp.Id = cohereResp.ResponseId
	openaiResp.Created = createdTime
	openaiResp.Object = "chat.completion"
	openaiResp.Model = info.UpstreamModelName
	openaiResp.Usage = usage

	openaiResp.Choices = []dto.OpenAITextResponseChoice{
		{
			Index:        0,
			Message:      dto.Message{Content: cohereResp.Text, Role: "assistant"},
			FinishReason: stopReasonCohere2OpenAI(cohereResp.FinishReason),
		},
	}

	jsonResponse, err := common.Marshal(openaiResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return &usage, nil
}

func cohereRerankHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp)
	var cohereResp CohereRerankResponseResult
	err = common.Unmarshal(responseBody, &cohereResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	usage := dto.Usage{}
	if cohereResp.Meta.BilledUnits.InputTokens == 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
		usage.CompletionTokens = 0
		usage.TotalTokens = info.GetEstimatePromptTokens()
	} else {
		usage.PromptTokens = cohereResp.Meta.BilledUnits.InputTokens
		usage.CompletionTokens = cohereResp.Meta.BilledUnits.OutputTokens
		usage.TotalTokens = cohereResp.Meta.BilledUnits.InputTokens + cohereResp.Meta.BilledUnits.OutputTokens
	}

	var rerankResp dto.RerankResponse
	rerankResp.Results = cohereResp.Results
	rerankResp.Usage = usage

	jsonResponse, err := common.Marshal(rerankResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return &usage, nil
}
