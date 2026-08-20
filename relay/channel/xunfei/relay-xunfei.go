package xunfei

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// https://console.xfyun.cn/services/cbm
// https://www.xfyun.cn/doc/spark/Web.html

func requestOpenAI2Xunfei(request dto.GeneralOpenAIRequest, xunfeiAppId string, domain string) *XunfeiChatRequest {
	messages := make([]XunfeiMessage, 0, len(request.Messages))
	shouldCovertSystemMessage := !strings.HasSuffix(request.Model, "3.5")
	for _, message := range request.Messages {
		if message.Role == "system" && shouldCovertSystemMessage {
			messages = append(messages, XunfeiMessage{
				Role:    "user",
				Content: message.StringContent(),
			})
			messages = append(messages, XunfeiMessage{
				Role:    "assistant",
				Content: "Okay",
			})
		} else {
			messages = append(messages, XunfeiMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	xunfeiRequest := XunfeiChatRequest{}
	xunfeiRequest.Header.AppId = xunfeiAppId
	xunfeiRequest.Parameter.Chat.Domain = domain
	xunfeiRequest.Parameter.Chat.Temperature = request.Temperature
	xunfeiRequest.Parameter.Chat.TopK = lo.FromPtrOr(request.N, 0)
	xunfeiRequest.Parameter.Chat.MaxTokens = request.GetMaxTokens()
	xunfeiRequest.Payload.Message.Text = messages
	return &xunfeiRequest
}

func responseXunfei2OpenAI(response *XunfeiChatResponse) *dto.OpenAITextResponse {
	if len(response.Payload.Choices.Text) == 0 {
		response.Payload.Choices.Text = []XunfeiChatResponseTextItem{
			{
				Content: "",
			},
		}
	}
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role:    "assistant",
			Content: response.Payload.Choices.Text[0].Content,
		},
		FinishReason: constant.FinishReasonStop,
	}
	fullTextResponse := dto.OpenAITextResponse{
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Choices: []dto.OpenAITextResponseChoice{choice},
		Usage:   response.Payload.Usage.Text,
	}
	return &fullTextResponse
}

func streamResponseXunfei2OpenAI(xunfeiResponse *XunfeiChatResponse) *dto.ChatCompletionsStreamResponse {
	if len(xunfeiResponse.Payload.Choices.Text) == 0 {
		xunfeiResponse.Payload.Choices.Text = []XunfeiChatResponseTextItem{
			{
				Content: "",
			},
		}
	}
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString(xunfeiResponse.Payload.Choices.Text[0].Content)
	if xunfeiResponse.Payload.Choices.Status == 2 {
		choice.FinishReason = &constant.FinishReasonStop
	}
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "SparkDesk",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

func xunfeiResponseVisibleText(response *XunfeiChatResponse) string {
	if response == nil || len(response.Payload.Choices.Text) == 0 {
		return ""
	}
	return response.Payload.Choices.Text[0].Content
}

func buildXunfeiAuthUrl(hostUrl string, apiKey, apiSecret string) string {
	HmacWithShaToBase64 := func(algorithm, data, key string) string {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(data))
		encodeData := mac.Sum(nil)
		return base64.StdEncoding.EncodeToString(encodeData)
	}
	ul, err := url.Parse(hostUrl)
	if err != nil {
		fmt.Println(err)
	}
	date := time.Now().UTC().Format(time.RFC1123)
	signString := []string{"host: " + ul.Host, "date: " + date, "GET " + ul.Path + " HTTP/1.1"}
	sign := strings.Join(signString, "\n")
	sha := HmacWithShaToBase64("hmac-sha256", sign, apiSecret)
	authUrl := fmt.Sprintf("hmac username=\"%s\", algorithm=\"%s\", headers=\"%s\", signature=\"%s\"", apiKey,
		"hmac-sha256", "host date request-line", sha)
	authorization := base64.StdEncoding.EncodeToString([]byte(authUrl))
	v := url.Values{}
	v.Add("host", ul.Host)
	v.Add("date", date)
	v.Add("authorization", authorization)
	callUrl := hostUrl + "?" + v.Encode()
	return callUrl
}

func xunfeiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, textRequest dto.GeneralOpenAIRequest, appId string, apiSecret string, apiKey string) (*dto.Usage, *types.NewAPIError) {
	info.StreamStatus = relaycommon.NewStreamStatus()
	domain, authUrl := getXunfeiAuthUrl(c, apiKey, apiSecret, textRequest.Model)
	streamCtx, cancel := context.WithCancel(c.Request.Context())
	stream, err := xunfeiMakeRequest(streamCtx, info, textRequest, domain, authUrl, appId)
	if err != nil {
		cancel()
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed)
	}
	watcherDone := helper.CloseUpstreamOnContext(c.Request.Context(), stream.conn, stream.done)
	defer func() {
		cancel()
		stream.idleWatchdog.Stop()
		_ = stream.conn.Close()
		<-stream.done
		<-watcherDone
		<-stream.idleWatcherDone
	}()
	helper.SetEventStreamHeaders(c)
	var usage dto.Usage
	for xunfeiResponse := range stream.responses {
		if responseErr := newXunfeiStreamError(&xunfeiResponse); responseErr != nil {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, responseErr)
			return &usage, responseErr
		}
		usage.PromptTokens += xunfeiResponse.Payload.Usage.Text.PromptTokens
		usage.CompletionTokens += xunfeiResponse.Payload.Usage.Text.CompletionTokens
		usage.TotalTokens += xunfeiResponse.Payload.Usage.Text.TotalTokens
		response := streamResponseXunfei2OpenAI(&xunfeiResponse)
		if err := helper.ObjectData(c, response); err != nil {
			info.StreamStatus.SetClientGone(err)
			return &usage, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
		}
	}
	if requestErr := c.Request.Context().Err(); requestErr != nil {
		info.StreamStatus.SetClientGone(requestErr)
	}
	if err := helper.Done(c); err != nil {
		info.StreamStatus.SetClientGone(err)
		return &usage, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	return &usage, nil
}

func newXunfeiStreamError(response *XunfeiChatResponse) *types.NewAPIError {
	if response == nil || response.Header.Code == 0 {
		return nil
	}
	return types.NewOpenAIError(
		fmt.Errorf("xunfei error %d: %s", response.Header.Code, response.Header.Message),
		types.ErrorCodeBadResponse,
		xunfeiStreamErrorStatus(response.Header.Code),
	)
}

func xunfeiStreamErrorStatus(code int) int {
	switch code {
	case 10003, 10004, 10005, 10049:
		return http.StatusBadRequest
	case 10907:
		return http.StatusRequestEntityTooLarge
	case 10013, 10014, 10019, 10021, 10022, 10163:
		return http.StatusUnprocessableEntity
	case 10007, 10028, 11201, 11202, 11203:
		return http.StatusTooManyRequests
	case 10015, 10016, 11200:
		return http.StatusForbidden
	case 10008, 10110:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

func xunfeiHandler(c *gin.Context, info *relaycommon.RelayInfo, textRequest dto.GeneralOpenAIRequest, appId string, apiSecret string, apiKey string) (*dto.Usage, *types.NewAPIError) {
	// Xunfei uses the same upstream WebSocket reader for buffered responses.
	// Keep its lifecycle sink initialized even though non-stream requests are
	// not eligible for dynamic-routing performance samples.
	info.StreamStatus = relaycommon.NewStreamStatus()
	domain, authUrl := getXunfeiAuthUrl(c, apiKey, apiSecret, textRequest.Model)
	streamCtx, cancel := context.WithCancel(c.Request.Context())
	stream, err := xunfeiMakeRequest(streamCtx, info, textRequest, domain, authUrl, appId)
	if err != nil {
		cancel()
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed)
	}
	watcherDone := helper.CloseUpstreamOnContext(c.Request.Context(), stream.conn, stream.done)
	defer func() {
		cancel()
		stream.idleWatchdog.Stop()
		_ = stream.conn.Close()
		<-stream.done
		<-watcherDone
		<-stream.idleWatcherDone
	}()
	var usage dto.Usage
	var content string
	var xunfeiResponse XunfeiChatResponse
	for response := range stream.responses {
		xunfeiResponse = response
		if len(xunfeiResponse.Payload.Choices.Text) == 0 {
			continue
		}
		content += xunfeiResponse.Payload.Choices.Text[0].Content
		usage.PromptTokens += xunfeiResponse.Payload.Usage.Text.PromptTokens
		usage.CompletionTokens += xunfeiResponse.Payload.Usage.Text.CompletionTokens
		usage.TotalTokens += xunfeiResponse.Payload.Usage.Text.TotalTokens
	}
	if len(xunfeiResponse.Payload.Choices.Text) == 0 {
		xunfeiResponse.Payload.Choices.Text = []XunfeiChatResponseTextItem{
			{
				Content: "",
			},
		}
	}
	xunfeiResponse.Payload.Choices.Text[0].Content = content

	response := responseXunfei2OpenAI(&xunfeiResponse)
	jsonResponse, err := common.Marshal(response)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	_, _ = c.Writer.Write(jsonResponse)
	return &usage, nil
}

type xunfeiDialer interface {
	DialContext(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error)
}

type xunfeiResponseStream struct {
	responses       <-chan XunfeiChatResponse
	done            <-chan struct{}
	conn            *websocket.Conn
	idleWatchdog    *helper.StreamIdleWatchdog
	idleWatcherDone <-chan struct{}
}

func dialXunfeiUpstream(ctx context.Context, info *relaycommon.RelayInfo, dialer xunfeiDialer, authUrl string) (*websocket.Conn, error) {
	info.MarkAttemptUpstreamStarted()
	conn, resp, err := dialer.DialContext(ctx, authUrl, nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	if resp == nil || resp.StatusCode != http.StatusSwitchingProtocols {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		_ = conn.Close()
		return nil, fmt.Errorf("xunfei websocket handshake did not switch protocols")
	}
	return conn, nil
}

func xunfeiMakeRequest(ctx context.Context, info *relaycommon.RelayInfo, textRequest dto.GeneralOpenAIRequest, domain, authUrl, appId string) (*xunfeiResponseStream, error) {
	dialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	data := requestOpenAI2Xunfei(textRequest, appId, domain)
	conn, err := dialXunfeiUpstream(ctx, info, dialer, authUrl)
	if err != nil {
		return nil, err
	}
	err = conn.WriteJSON(data)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	dataChan := make(chan XunfeiChatResponse, 10)
	done := make(chan struct{})
	idleWatchdog := helper.NewConfiguredStreamIdleWatchdog()
	go func() {
		defer close(done)
		defer close(dataChan)
		defer func() {
			_ = conn.Close()
		}()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if ctx.Err() != nil {
					info.StreamStatus.SetClientGone(ctx.Err())
				} else {
					common.SysLog("error reading stream response: " + err.Error())
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
				}
				return
			}
			idleWatchdog.Reset()
			if helper.IsNullJSONStreamEvent(string(msg)) {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
				return
			}
			var response XunfeiChatResponse
			err = common.Unmarshal(msg, &response)
			if err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, err)
				return
			}
			visibleText := xunfeiResponseVisibleText(&response)
			if response.Payload.Choices.Status != 2 {
				info.SetFirstResponseTime()
			}
			info.RecordAttemptVisibleText(visibleText)
			if response.Payload.Choices.Status == 2 {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
			}
			if !helper.EnqueueStreamItemWithBackpressure(ctx, dataChan, response, info) {
				return
			}
			if response.Payload.Choices.Status == 2 {
				return
			}
		}
	}()
	idleWatcherDone := helper.CloseUpstreamOnIdleTimeout(ctx, info, conn, done, idleWatchdog)

	return &xunfeiResponseStream{
		responses:       dataChan,
		done:            done,
		conn:            conn,
		idleWatchdog:    idleWatchdog,
		idleWatcherDone: idleWatcherDone,
	}, nil
}

func apiVersion2domain(apiVersion string) string {
	switch apiVersion {
	case "v1.1":
		return "lite"
	case "v2.1":
		return "generalv2"
	case "v3.1":
		return "generalv3"
	case "v3.5":
		return "generalv3.5"
	case "v4.0":
		return "4.0Ultra"
	}
	return "general" + apiVersion
}

func getXunfeiAuthUrl(c *gin.Context, apiKey string, apiSecret string, modelName string) (string, string) {
	apiVersion := getAPIVersion(c, modelName)
	domain := apiVersion2domain(apiVersion)
	authUrl := buildXunfeiAuthUrl(fmt.Sprintf("wss://spark-api.xf-yun.com/%s/chat", apiVersion), apiKey, apiSecret)
	return domain, authUrl
}

func getAPIVersion(c *gin.Context, modelName string) string {
	query := c.Request.URL.Query()
	apiVersion := query.Get("api-version")
	if apiVersion != "" {
		return apiVersion
	}
	parts := strings.Split(modelName, "-")
	if len(parts) == 2 {
		apiVersion = parts[1]
		return apiVersion

	}
	apiVersion = c.GetString("api_version")
	if apiVersion != "" {
		return apiVersion
	}
	apiVersion = "v1.1"
	common.SysLog("api_version not found, using default: " + apiVersion)
	return apiVersion
}
