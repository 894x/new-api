package zhipu

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// https://open.bigmodel.cn/doc/api#chatglm_std
// chatglm_std, chatglm_lite
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/invoke
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/sse-invoke

var zhipuTokens sync.Map
var expSeconds int64 = 24 * 3600

func getZhipuToken(apikey string) string {
	data, ok := zhipuTokens.Load(apikey)
	if ok {
		tokenData := data.(zhipuTokenData)
		if time.Now().Before(tokenData.ExpiryTime) {
			return tokenData.Token
		}
	}

	split := strings.Split(apikey, ".")
	if len(split) != 2 {
		common.SysLog("invalid zhipu key: " + apikey)
		return ""
	}

	id := split[0]
	secret := split[1]

	expMillis := time.Now().Add(time.Duration(expSeconds)*time.Second).UnixNano() / 1e6
	expiryTime := time.Now().Add(time.Duration(expSeconds) * time.Second)

	timestamp := time.Now().UnixNano() / 1e6

	payload := jwt.MapClaims{
		"api_key":   id,
		"exp":       expMillis,
		"timestamp": timestamp,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)

	token.Header["alg"] = "HS256"
	token.Header["sign_type"] = "SIGN"

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return ""
	}

	zhipuTokens.Store(apikey, zhipuTokenData{
		Token:      tokenString,
		ExpiryTime: expiryTime,
	})

	return tokenString
}

func requestOpenAI2Zhipu(request dto.GeneralOpenAIRequest) *ZhipuRequest {
	messages := make([]ZhipuMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == "system" {
			messages = append(messages, ZhipuMessage{
				Role:    "system",
				Content: message.StringContent(),
			})
			messages = append(messages, ZhipuMessage{
				Role:    "user",
				Content: "Okay",
			})
		} else {
			messages = append(messages, ZhipuMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	return &ZhipuRequest{
		Prompt:      messages,
		Temperature: request.Temperature,
		TopP:        lo.FromPtrOr(request.TopP, 0),
		Incremental: false,
	}
}

func responseZhipu2OpenAI(response *ZhipuResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Data.TaskId,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(response.Data.Choices)),
		Usage:   response.Data.Usage,
	}
	for i, choice := range response.Data.Choices {
		openaiChoice := dto.OpenAITextResponseChoice{
			Index: i,
			Message: dto.Message{
				Role:    choice.Role,
				Content: strings.Trim(choice.Content, "\""),
			},
			FinishReason: "",
		}
		if i == len(response.Data.Choices)-1 {
			openaiChoice.FinishReason = "stop"
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, openaiChoice)
	}
	return &fullTextResponse
}

func streamResponseZhipu2OpenAI(zhipuResponse string) *dto.ChatCompletionsStreamResponse {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString(zhipuResponse)
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "chatglm",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

func streamMetaResponseZhipu2OpenAI(zhipuResponse *ZhipuStreamMetaResponse) (*dto.ChatCompletionsStreamResponse, *dto.Usage) {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString("")
	choice.FinishReason = &constant.FinishReasonStop
	response := dto.ChatCompletionsStreamResponse{
		Id:      zhipuResponse.RequestId,
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "chatglm",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response, &zhipuResponse.Usage
}

func zhipuStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	info.StreamStatus = relaycommon.NewStreamStatus()
	streamCtx, cancel := context.WithCancel(c.Request.Context())
	var usage *dto.Usage
	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	eventChan := make(chan zhipuStreamEvent, 10)
	producerDone := make(chan struct{})
	idleWatchdog := helper.NewConfiguredStreamIdleWatchdog()
	var producerErr *types.NewAPIError
	go func() {
		defer close(producerDone)
		defer close(eventChan)
		nextEventKind := ""
		for scanner.Scan() {
			idleWatchdog.Reset()
			line := scanner.Text()
			if strings.HasPrefix(line, "event:") {
				nextEventKind = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if len(line) < 5 {
				continue
			}
			var event zhipuStreamEvent
			switch line[:5] {
			case "data:":
				kind := "data"
				if nextEventKind == "error" {
					kind = "error"
				}
				nextEventKind = ""
				event = zhipuStreamEvent{kind: kind, data: line[5:]}
				info.SetFirstResponseTime()
				if event.kind == "error" {
					producerErr = types.NewOpenAIError(
						fmt.Errorf("zhipu stream error: %s", common.LocalLogPreview(event.data)),
						types.ErrorCodeBadResponse,
						helper.ResolveUpstreamStreamErrorStatus(event.data, resp.StatusCode),
					)
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, producerErr)
					return
				}
				info.RecordAttemptVisibleText(event.data)
			case "meta:":
				event = zhipuStreamEvent{kind: "meta", data: line[5:]}
				if helper.IsNullJSONStreamEvent(event.data) {
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
					return
				}
				var meta ZhipuStreamMetaResponse
				if err := common.Unmarshal([]byte(event.data), &meta); err != nil {
					producerErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
					return
				}
				if strings.EqualFold(meta.TaskStatus, "FAIL") || strings.EqualFold(meta.TaskStatus, "FAILED") {
					producerErr = types.NewOpenAIError(
						fmt.Errorf("zhipu stream task failed: %s", meta.TaskStatus),
						types.ErrorCodeBadResponse,
						http.StatusBadGateway,
					)
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, producerErr)
					return
				}
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			default:
				continue
			}
			if !helper.EnqueueStreamItemWithBackpressure(streamCtx, eventChan, event, info) {
				return
			}
			if event.kind == "meta" {
				return
			}
		}
		if requestErr := c.Request.Context().Err(); requestErr != nil {
			info.StreamStatus.SetClientGone(requestErr)
		} else if err := scanner.Err(); err != nil {
			common.SysLog("error reading stream: " + err.Error())
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
	helper.SetEventStreamHeaders(c)
	for event := range eventChan {
		switch event.kind {
		case "data":
			data := event.data
			response := streamResponseZhipu2OpenAI(data)
			if err := helper.ObjectData(c, response); err != nil {
				info.StreamStatus.SetClientGone(err)
				return usage, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
			}
		case "meta":
			data := event.data
			var zhipuResponse ZhipuStreamMetaResponse
			err := common.Unmarshal([]byte(data), &zhipuResponse)
			if err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
				return usage, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
			response, zhipuUsage := streamMetaResponseZhipu2OpenAI(&zhipuResponse)
			usage = zhipuUsage
			if err = helper.ObjectData(c, response); err != nil {
				info.StreamStatus.SetClientGone(err)
				return usage, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
			}
		}
	}
	if requestErr := c.Request.Context().Err(); requestErr != nil {
		info.StreamStatus.SetClientGone(requestErr)
	}
	if producerErr != nil {
		return usage, producerErr
	}
	if err := helper.Done(c); err != nil {
		info.StreamStatus.SetClientGone(err)
		return usage, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	return usage, nil
}

type zhipuStreamEvent struct {
	kind string
	data string
}

func zhipuHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var zhipuResponse ZhipuResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &zhipuResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if !zhipuResponse.Success {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResponse.Msg,
			Code:    zhipuResponse.Code,
		}, resp.StatusCode)
	}
	fullTextResponse := responseZhipu2OpenAI(&zhipuResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return &fullTextResponse.Usage, nil
}
