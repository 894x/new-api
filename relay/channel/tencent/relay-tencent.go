package tencent

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// https://cloud.tencent.com/document/product/1729/97732

func requestOpenAI2Tencent(a *Adaptor, request dto.GeneralOpenAIRequest) *TencentChatRequest {
	messages := make([]*TencentMessage, 0, len(request.Messages))
	for i := 0; i < len(request.Messages); i++ {
		message := request.Messages[i]
		messages = append(messages, &TencentMessage{
			Content: message.StringContent(),
			Role:    message.Role,
		})
	}
	var req = TencentChatRequest{
		Stream:   request.Stream,
		Messages: messages,
		Model:    &request.Model,
	}
	if request.TopP != nil {
		req.TopP = request.TopP
	}
	req.Temperature = request.Temperature
	return &req
}

func responseTencent2OpenAI(response *TencentChatResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Id,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Usage: dto.Usage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
	}
	if len(response.Choices) > 0 {
		choice := dto.OpenAITextResponseChoice{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: response.Choices[0].Messages.Content,
			},
			FinishReason: response.Choices[0].FinishReason,
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func streamResponseTencent2OpenAI(TencentResponse *TencentChatResponse) *dto.ChatCompletionsStreamResponse {
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "tencent-hunyuan",
	}
	if len(TencentResponse.Choices) > 0 {
		var choice dto.ChatCompletionsStreamResponseChoice
		choice.Delta.SetContentString(TencentResponse.Choices[0].Delta.Content)
		if TencentResponse.Choices[0].FinishReason == "stop" {
			choice.FinishReason = &constant.FinishReasonStop
		}
		response.Choices = append(response.Choices, choice)
	}
	return &response
}

func tencentStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	info.StreamStatus = relaycommon.NewStreamStatus()
	streamCtx, cancel := context.WithCancel(c.Request.Context())
	var responseText string
	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	dataChan := make(chan string, 10)
	producerDone := make(chan struct{})
	idleWatchdog := helper.NewConfiguredStreamIdleWatchdog()
	var producerErr *types.NewAPIError

	helper.SetEventStreamHeaders(c)
	go func() {
		defer close(producerDone)
		defer close(dataChan)
		for scanner.Scan() {
			idleWatchdog.Reset()
			data := strings.TrimSpace(scanner.Text())
			if helper.IsNullJSONStreamEvent(data) {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
				return
			}

			var response TencentChatResponse
			if strings.HasPrefix(data, "data:") {
				data = strings.TrimSpace(strings.TrimPrefix(data, "data:"))
				if helper.IsNullJSONStreamEvent(data) {
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
					return
				}
				if err := common.Unmarshal([]byte(data), &response); err != nil {
					common.SysLog("error unmarshalling stream response: " + err.Error())
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
					return
				}
			} else if strings.HasPrefix(data, "{") {
				var wrapped TencentChatResponseSB
				if err := common.Unmarshal([]byte(data), &wrapped); err != nil {
					common.SysLog("error unmarshalling stream error response: " + err.Error())
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
					return
				}
				response = wrapped.Response
				if _, ok := tencentResponseError(&response); !ok {
					continue
				}
			} else {
				continue
			}
			if responseError, ok := tencentResponseError(&response); ok {
				producerErr = types.WithOpenAIError(types.OpenAIError{
					Message: responseError.Message,
					Type:    "upstream_error",
					Code:    responseError.Code,
				}, tencentStreamErrorStatus(responseError.Code, data, resp.StatusCode))
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, producerErr)
				return
			}
			terminalOnly := len(response.Choices) > 0 && response.Choices[0].FinishReason == "stop" && response.Choices[0].Delta.Content == ""
			if !terminalOnly {
				info.SetFirstResponseTime()
			}
			if len(response.Choices) > 0 {
				info.RecordAttemptVisibleText(response.Choices[0].Delta.Content)
				if response.Choices[0].FinishReason == "stop" {
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				}
			}
			if !helper.EnqueueStreamDataWithBackpressure(streamCtx, dataChan, data, info) {
				return
			}
			if len(response.Choices) > 0 && response.Choices[0].FinishReason == "stop" {
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

	for data := range dataChan {
		var tencentResponse TencentChatResponse
		err := common.Unmarshal([]byte(data), &tencentResponse)
		if err != nil {
			continue
		}

		response := streamResponseTencent2OpenAI(&tencentResponse)
		if len(response.Choices) != 0 {
			responseText += response.Choices[0].Delta.GetContentString()
		}

		err = helper.ObjectData(c, response)
		if err != nil {
			common.SysLog(err.Error())
			info.StreamStatus.SetClientGone(err)
			return service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens()), types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
		}
	}

	if requestErr := c.Request.Context().Err(); requestErr != nil {
		info.StreamStatus.SetClientGone(requestErr)
	}
	if producerErr != nil {
		return service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens()), producerErr
	}

	if err := helper.Done(c); err != nil {
		info.StreamStatus.SetClientGone(err)
		return service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens()), types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}

	return service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens()), nil
}

func tencentResponseError(response *TencentChatResponse) (*TencentError, bool) {
	if response == nil {
		return nil, false
	}
	if tencentErrorPresent(&response.ErrorMsg) {
		return &response.ErrorMsg, true
	}
	if tencentErrorPresent(&response.Error) {
		return &response.Error, true
	}
	return nil, false
}

func tencentErrorPresent(responseError *TencentError) bool {
	if responseError == nil {
		return false
	}
	if responseError.Message != "" {
		return true
	}
	switch code := responseError.Code.(type) {
	case string:
		return strings.TrimSpace(code) != "" && strings.TrimSpace(code) != "0"
	case float64:
		return code != 0
	case int:
		return code != 0
	default:
		return code != nil
	}
}

func tencentStreamErrorStatus(code any, data string, responseStatus int) int {
	codeText := strings.TrimSpace(fmt.Sprint(code))
	switch {
	case strings.HasPrefix(codeText, "InvalidParameter"), strings.HasPrefix(codeText, "UnsupportedOperation"),
		strings.HasPrefix(codeText, "MissingParameter"), strings.HasPrefix(codeText, "UnknownParameter"),
		strings.HasPrefix(codeText, "InvalidAction"), strings.HasPrefix(codeText, "NoSuchVersion"):
		return http.StatusBadRequest
	case strings.HasPrefix(codeText, "FailedOperation.SensitiveContent"),
		strings.HasPrefix(codeText, "OperationDenied.TextIllegalDetected"),
		strings.HasPrefix(codeText, "OperationDenied.ImageIllegalDetected"):
		return http.StatusUnprocessableEntity
	case strings.HasPrefix(codeText, "FailedOperation.EngineRequestTimeout"):
		return http.StatusRequestTimeout
	case strings.HasPrefix(codeText, "FailedOperation.EngineServerLimitExceeded"),
		strings.HasPrefix(codeText, "FailedOperation.FreeResourcePackExhausted"),
		strings.HasPrefix(codeText, "FailedOperation.ResourcePackExhausted"):
		return http.StatusTooManyRequests
	case strings.HasPrefix(codeText, "FailedOperation.EngineServerError"),
		strings.HasPrefix(codeText, "FailedOperation.ConsoleServerError"):
		return http.StatusInternalServerError
	case strings.HasPrefix(codeText, "FailedOperation.ServiceNotActivated"),
		strings.HasPrefix(codeText, "FailedOperation.ServiceStop"),
		strings.HasPrefix(codeText, "FailedOperation.PartnerAccountUnSupport"),
		strings.HasPrefix(codeText, "FailedOperation.SetPayModeExceed"):
		return http.StatusServiceUnavailable
	case strings.HasPrefix(codeText, "FailedOperation.UserUnAuthError"):
		return http.StatusForbidden
	case strings.HasPrefix(codeText, "FailedOperation"):
		return http.StatusBadGateway
	case strings.HasPrefix(codeText, "AuthFailure"):
		return http.StatusUnauthorized
	case strings.HasPrefix(codeText, "UnauthorizedOperation"):
		return http.StatusForbidden
	case strings.HasPrefix(codeText, "RequestLimitExceeded"), strings.HasPrefix(codeText, "LimitExceeded"):
		return http.StatusTooManyRequests
	case strings.HasPrefix(codeText, "InternalError"):
		return http.StatusInternalServerError
	case strings.HasPrefix(codeText, "ResourceUnavailable"), strings.HasPrefix(codeText, "ResourceInsufficient"),
		strings.HasPrefix(codeText, "ResourceNotFound"):
		return http.StatusServiceUnavailable
	default:
		return helper.ResolveUpstreamStreamErrorStatus(data, responseStatus)
	}
}

func tencentHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var tencentSb TencentChatResponseSB
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &tencentSb)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if responseError, ok := tencentResponseError(&tencentSb.Response); ok {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: responseError.Message,
			Code:    responseError.Code,
		}, tencentStreamErrorStatus(responseError.Code, string(responseBody), resp.StatusCode))
	}
	fullTextResponse := responseTencent2OpenAI(&tencentSb.Response)
	jsonResponse, err := common.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return &fullTextResponse.Usage, nil
}

func parseTencentConfig(config string) (appId int64, secretId string, secretKey string, err error) {
	parts := strings.Split(config, "|")
	if len(parts) != 3 {
		err = errors.New("invalid tencent config")
		return
	}
	appId, err = strconv.ParseInt(parts[0], 10, 64)
	secretId = parts[1]
	secretKey = parts[2]
	return
}

func sha256hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func hmacSha256(s, key string) string {
	hashed := hmac.New(sha256.New, []byte(key))
	hashed.Write([]byte(s))
	return string(hashed.Sum(nil))
}

func getTencentSign(req TencentChatRequest, adaptor *Adaptor, secId, secKey string) string {
	// build canonical request string
	host := "hunyuan.tencentcloudapi.com"
	httpRequestMethod := "POST"
	canonicalURI := "/"
	canonicalQueryString := ""
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-tc-action:%s\n",
		"application/json", host, strings.ToLower(adaptor.Action))
	signedHeaders := "content-type;host;x-tc-action"
	payload, _ := json.Marshal(req)
	hashedRequestPayload := sha256hex(string(payload))
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpRequestMethod,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedRequestPayload)
	// build string to sign
	algorithm := "TC3-HMAC-SHA256"
	requestTimestamp := strconv.FormatInt(adaptor.Timestamp, 10)
	timestamp, _ := strconv.ParseInt(requestTimestamp, 10, 64)
	t := time.Unix(timestamp, 0).UTC()
	// must be the format 2006-01-02, ref to package time for more info
	date := t.Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, "hunyuan")
	hashedCanonicalRequest := sha256hex(canonicalRequest)
	string2sign := fmt.Sprintf("%s\n%s\n%s\n%s",
		algorithm,
		requestTimestamp,
		credentialScope,
		hashedCanonicalRequest)

	// sign string
	secretDate := hmacSha256(date, "TC3"+secKey)
	secretService := hmacSha256("hunyuan", secretDate)
	secretKey := hmacSha256("tc3_request", secretService)
	signature := hex.EncodeToString([]byte(hmacSha256(string2sign, secretKey)))

	// build authorization
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		secId,
		credentialScope,
		signedHeaders,
		signature)
	return authorization
}
