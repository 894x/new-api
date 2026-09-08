package aws

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrockruntimeTypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/auth/bearer"
)

// getAwsErrorStatusCode extracts HTTP status code from AWS SDK error
func getAwsErrorStatusCode(err error) int {
	// Check for HTTP response error which contains status code
	var httpErr interface{ HTTPStatusCode() int }
	if errors.As(err, &httpErr) {
		return httpErr.HTTPStatusCode()
	}
	// Default to 500 if we can't determine the status code
	return http.StatusInternalServerError
}

func newAwsInvokeContext(parent context.Context) (context.Context, context.CancelFunc) {
	if common.RelayTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, time.Duration(common.RelayTimeout)*time.Second)
}

func newAwsInvokeError(requestContext context.Context, err error, operation string) *types.NewAPIError {
	options := make([]types.NewAPIErrorOptions, 0, 1)
	if requestContext.Err() != nil {
		options = append(options, types.ErrOptionWithSkipRetry())
	}
	errorCode := types.ErrorCodeAwsInvokeError
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ResourceNotFoundException" {
		errorCode = types.ErrorCodeModelNotFound
	}
	return types.NewOpenAIError(
		errors.Wrap(err, operation),
		errorCode,
		getAwsErrorStatusCode(err),
		options...,
	)
}

func newAwsClient(c *gin.Context, info *relaycommon.RelayInfo) (*bedrockruntime.Client, error) {
	httpClient, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}

	awsSecret := strings.Split(info.ApiKey, "|")
	var client *bedrockruntime.Client
	switch len(awsSecret) {
	case 2:
		apiKey := awsSecret[0]
		region := awsSecret[1]
		client = bedrockruntime.New(bedrockruntime.Options{
			Region:                  region,
			BearerAuthTokenProvider: bearer.StaticTokenProvider{Token: bearer.Token{Value: apiKey}},
			HTTPClient:              httpClient,
		})
	case 3:
		ak := awsSecret[0]
		sk := awsSecret[1]
		region := awsSecret[2]
		client = bedrockruntime.New(bedrockruntime.Options{
			Region:      region,
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(ak, sk, "")),
			HTTPClient:  httpClient,
		})
	default:
		return nil, errors.New("invalid aws secret key")
	}

	return client, nil
}

func doAwsClientRequest(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor, requestBody io.Reader) (any, error) {
	awsCli, err := newAwsClient(c, info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelAwsClientError)
	}
	a.AwsClient = awsCli

	// 获取对应的AWS模型ID
	awsModelId := getAwsModelID(info.UpstreamModelName)

	awsRegionPrefix := getAwsRegionPrefix(awsCli.Options().Region)
	canCrossRegion := awsModelCanCrossRegion(awsModelId, awsRegionPrefix)
	if canCrossRegion {
		awsModelId = awsModelCrossRegion(awsModelId, awsRegionPrefix)
	}

	// init empty request.header
	requestHeader := http.Header{}
	a.SetupRequestHeader(c, &requestHeader, info)
	headerOverride, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range headerOverride {
		requestHeader.Set(key, value)
	}

	if isNovaModel(awsModelId) {
		var novaReq *NovaRequest
		err = common.DecodeJson(requestBody, &novaReq)
		if err != nil {
			return nil, types.NewError(errors.Wrap(err, "decode nova request fail"), types.ErrorCodeBadRequestBody)
		}

		// 使用InvokeModel API，但使用Nova格式的请求体
		awsReq := &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(awsModelId),
			Accept:      aws.String("application/json"),
			ContentType: aws.String("application/json"),
		}

		reqBody, err := common.Marshal(novaReq)
		if err != nil {
			return nil, types.NewError(errors.Wrap(err, "marshal nova request"), types.ErrorCodeBadResponseBody)
		}
		awsReq.Body = reqBody
		a.AwsReq = awsReq
		return nil, nil
	} else {
		awsClaudeReq, err := formatRequest(requestBody, requestHeader)
		if err != nil {
			return nil, types.NewError(errors.Wrap(err, "format aws request fail"), types.ErrorCodeBadRequestBody)
		}

		if info.IsStream {
			awsReq := &bedrockruntime.InvokeModelWithResponseStreamInput{
				ModelId:     aws.String(awsModelId),
				Accept:      aws.String("application/json"),
				ContentType: aws.String("application/json"),
			}
			awsReq.Body, err = buildAwsRequestBody(c, info, awsClaudeReq)
			if err != nil {
				return nil, types.NewError(errors.Wrap(err, "marshal aws request fail"), types.ErrorCodeBadRequestBody)
			}
			a.AwsReq = awsReq
			return nil, nil
		} else {
			awsReq := &bedrockruntime.InvokeModelInput{
				ModelId:     aws.String(awsModelId),
				Accept:      aws.String("application/json"),
				ContentType: aws.String("application/json"),
			}
			awsReq.Body, err = buildAwsRequestBody(c, info, awsClaudeReq)
			if err != nil {
				return nil, types.NewError(errors.Wrap(err, "marshal aws request fail"), types.ErrorCodeBadRequestBody)
			}
			a.AwsReq = awsReq
			return nil, nil
		}
	}
}

// buildAwsRequestBody prepares the payload for AWS requests, applying passthrough rules when enabled.
func buildAwsRequestBody(c *gin.Context, info *relaycommon.RelayInfo, awsClaudeReq any) ([]byte, error) {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, errors.Wrap(err, "get request body for pass-through fail")
		}
		body, err := storage.Bytes()
		if err != nil {
			return nil, errors.Wrap(err, "get request body bytes fail")
		}
		var data map[string]interface{}
		if err := common.Unmarshal(body, &data); err != nil {
			return nil, errors.Wrap(err, "pass-through unmarshal request body fail")
		}
		delete(data, "model")
		delete(data, "stream")
		return common.Marshal(data)
	}
	return common.Marshal(awsClaudeReq)
}

func getAwsRegionPrefix(awsRegionId string) string {
	parts := strings.Split(awsRegionId, "-")
	regionPrefix := ""
	if len(parts) > 0 {
		regionPrefix = parts[0]
	}
	return regionPrefix
}

func awsModelCanCrossRegion(awsModelId, awsRegionPrefix string) bool {
	regionSet, exists := awsModelCanCrossRegionMap[awsModelId]
	return exists && regionSet[awsRegionPrefix]
}

func awsModelCrossRegion(awsModelId, awsRegionPrefix string) string {
	modelPrefix, find := awsRegionCrossModelPrefixMap[awsRegionPrefix]
	if !find {
		return awsModelId
	}
	return modelPrefix + "." + awsModelId
}

func getAwsModelID(requestModel string) string {
	if awsModelIDName, ok := awsModelIDMap[requestModel]; ok {
		return awsModelIDName
	}
	return requestModel
}

func awsHandler(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.NewAPIError, *dto.Usage) {

	requestContext := c.Request.Context()
	ctx, cancel := newAwsInvokeContext(requestContext)
	defer cancel()

	request := a.AwsReq.(*bedrockruntime.InvokeModelInput)
	info.MarkAttemptUpstreamStarted()
	awsResp, err := a.AwsClient.InvokeModel(ctx, request)
	if err != nil {
		return newAwsInvokeError(requestContext, err, "InvokeModel"), nil
	}

	claudeInfo := &claude.ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}

	// 复制上游 Content-Type 到客户端响应头
	if awsResp.ContentType != nil && *awsResp.ContentType != "" {
		c.Writer.Header().Set("Content-Type", *awsResp.ContentType)
	}

	handlerErr := claude.HandleClaudeResponseData(c, info, claudeInfo, nil, awsResp.Body)
	if handlerErr != nil {
		return handlerErr, nil
	}
	return nil, claudeInfo.Usage
}

func awsStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.NewAPIError, *dto.Usage) {
	requestContext := c.Request.Context()
	ctx, cancel := newAwsInvokeContext(requestContext)
	defer cancel()

	request := a.AwsReq.(*bedrockruntime.InvokeModelWithResponseStreamInput)
	info.MarkAttemptUpstreamStarted()
	awsResp, err := a.AwsClient.InvokeModelWithResponseStream(ctx, request)
	if err != nil {
		return newAwsInvokeError(requestContext, err, "InvokeModelWithResponseStream"), nil
	}
	stream := awsResp.GetStream()
	info.StreamStatus = relaycommon.NewStreamStatus()
	ingressCtx, cancelIngress := context.WithCancel(ctx)
	dataChan := make(chan string, 10)
	producerDone := make(chan struct{})
	idleWatchdog := helper.NewConfiguredStreamIdleWatchdog()
	var producerErr *types.NewAPIError

	claudeInfo := &claude.ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}

	go func() {
		defer close(producerDone)
		defer close(dataChan)
		events := stream.Events()
		for {
			select {
			case <-ingressCtx.Done():
				if requestContext.Err() != nil {
					info.StreamStatus.SetClientGone(requestContext.Err())
				} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, ctx.Err())
				} else {
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, ingressCtx.Err())
				}
				return
			case event, ok := <-events:
				if !ok {
					if requestContext.Err() != nil {
						info.StreamStatus.SetClientGone(requestContext.Err())
					} else if streamErr := stream.Err(); streamErr != nil {
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, streamErr)
					} else {
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
					}
					return
				}
				idleWatchdog.Reset()
				switch value := event.(type) {
				case *bedrockruntimeTypes.ResponseStreamMemberChunk:
					data := string(value.Value.Bytes)
					if helper.IsNullJSONStreamEvent(data) {
						info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
						return
					}
					visibleText := helper.StreamChunkVisibleText(data)
					var eventHeader struct {
						Type string `json:"type"`
					}
					_ = common.Unmarshal([]byte(data), &eventHeader)
					if eventHeader.Type != "message_stop" && eventHeader.Type != "content_block_stop" {
						info.SetFirstResponseTime()
					}
					info.RecordAttemptVisibleText(visibleText)
					if !helper.EnqueueStreamDataWithBackpressure(ingressCtx, dataChan, data, info) {
						return
					}
				case *bedrockruntimeTypes.UnknownUnionMember:
					fmt.Println("unknown tag:", value.Tag)
					producerErr = types.NewError(errors.New("unknown response type"), types.ErrorCodeInvalidRequest)
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, producerErr)
					return
				default:
					fmt.Println("union is nil or unknown type")
					producerErr = types.NewError(errors.New("nil or unknown response type"), types.ErrorCodeInvalidRequest)
					info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, producerErr)
					return
				}
			}
		}
	}()
	watcherDone := helper.CloseUpstreamOnContext(requestContext, stream, producerDone)
	idleWatcherDone := helper.CloseUpstreamOnIdleTimeout(requestContext, info, stream, producerDone, idleWatchdog)
	defer func() {
		cancelIngress()
		idleWatchdog.Stop()
		_ = stream.Close()
		<-producerDone
		<-watcherDone
		<-idleWatcherDone
	}()

	for data := range dataChan {
		respErr := claude.HandleStreamResponseData(c, info, claudeInfo, data)
		if respErr != nil {
			if helper.IsDownstreamWriteError(respErr) {
				info.StreamStatus.SetClientGone(respErr)
			} else {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, respErr)
			}
			return respErr, nil
		}
	}
	if requestContext.Err() != nil {
		info.StreamStatus.SetClientGone(requestContext.Err())
		return nil, claudeInfo.Usage
	}
	if producerErr != nil {
		return producerErr, nil
	}
	if finalErr := claude.HandleStreamFinalResponse(c, info, claudeInfo); finalErr != nil {
		if helper.IsDownstreamWriteError(finalErr) {
			info.StreamStatus.SetClientGone(finalErr)
		}
		return types.NewError(finalErr, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry()), claudeInfo.Usage
	}
	return nil, claudeInfo.Usage
}

// Nova模型处理函数
func handleNovaRequest(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.NewAPIError, *dto.Usage) {

	requestContext := c.Request.Context()
	if info.IsStream {
		info.StreamStatus = relaycommon.NewStreamStatus()
	}
	ctx, cancel := newAwsInvokeContext(requestContext)
	defer cancel()

	request := a.AwsReq.(*bedrockruntime.InvokeModelInput)
	info.MarkAttemptUpstreamStarted()
	awsResp, err := a.AwsClient.InvokeModel(ctx, request)
	if err != nil {
		apiErr := newAwsInvokeError(requestContext, err, "InvokeModel")
		if info.IsStream {
			if requestErr := requestContext.Err(); requestErr != nil {
				info.StreamStatus.SetClientGone(requestErr)
			} else if errors.Is(err, context.DeadlineExceeded) {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, err)
			} else {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, apiErr)
			}
		}
		return apiErr, nil
	}

	// 解析Nova响应
	var novaResp struct {
		Output struct {
			Message struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
			TotalTokens  int `json:"totalTokens"`
		} `json:"usage"`
	}

	if helper.IsNullJSONStreamEvent(string(awsResp.Body)) {
		if info.IsStream {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, helper.ErrNullJSONStreamEvent)
		}
		return types.NewError(helper.ErrNullJSONStreamEvent, types.ErrorCodeBadResponseBody), nil
	}
	if err := common.Unmarshal(awsResp.Body, &novaResp); err != nil {
		wrappedErr := errors.Wrap(err, "unmarshal nova response")
		if info.IsStream {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonScannerErr, wrappedErr)
		}
		return types.NewError(wrappedErr, types.ErrorCodeBadResponseBody), nil
	}
	responseText := ""
	if len(novaResp.Output.Message.Content) > 0 {
		responseText = novaResp.Output.Message.Content[0].Text
	}
	if info.IsStream {
		// The complete SDK body has now been decoded and visible output is known.
		// Capture upstream timing before local DTO conversion and serialization.
		info.SetFirstResponseTime()
		info.RecordAttemptVisibleText(responseText)
	}

	// 构造OpenAI格式响应
	response := dto.OpenAITextResponse{
		Id:      helper.GetResponseID(c),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Model:   info.UpstreamModelName,
		Choices: []dto.OpenAITextResponseChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: responseText,
			},
			FinishReason: "stop",
		}},
		Usage: dto.Usage{
			PromptTokens:     novaResp.Usage.InputTokens,
			CompletionTokens: novaResp.Usage.OutputTokens,
			TotalTokens:      novaResp.Usage.TotalTokens,
		},
	}

	responseBody, err := common.Marshal(response)
	if err != nil {
		info.DiscardDynamicRoutingAttempt()
		return types.NewError(err, types.ErrorCodeJsonMarshalFailed), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(http.StatusOK)
	if _, err = c.Writer.Write(responseBody); err != nil {
		if info.IsStream {
			info.StreamStatus.SetClientGone(err)
		}
		return types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry()), nil
	}
	if info.IsStream {
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
	return nil, &response.Usage
}
