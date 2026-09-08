package aws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrockruntimeTypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const awsTestModel = "anthropic.claude-3-5-sonnet-20240620-v1:0"

type awsHTTPClientFunc func(*http.Request) (*http.Response, error)

func (f awsHTTPClientFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

type awsNotifyingResponseWriter struct {
	*httptest.ResponseRecorder
	notifyOn []byte
	notified chan int
	once     sync.Once
}

type awsInspectingResponseWriter struct {
	*httptest.ResponseRecorder
	onVisibleWrite func()
	once           sync.Once
}

type awsBlockingResponseWriter struct {
	*httptest.ResponseRecorder
	blocked chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *awsBlockingResponseWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("token_0")) {
		w.once.Do(func() { close(w.blocked) })
		<-w.release
	}
	return w.ResponseRecorder.Write(data)
}

func (w *awsInspectingResponseWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("hello")) {
		w.once.Do(w.onVisibleWrite)
	}
	return w.ResponseRecorder.Write(data)
}

func newAwsNotifyingResponseWriter(notifyOn string) *awsNotifyingResponseWriter {
	return &awsNotifyingResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		notifyOn:         []byte(notifyOn),
		notified:         make(chan int, 1),
	}
}

func (w *awsNotifyingResponseWriter) Write(data []byte) (int, error) {
	return w.ResponseRecorder.Write(data)
}

func (w *awsNotifyingResponseWriter) Flush() {
	w.ResponseRecorder.Flush()
	if bytes.Contains(w.Body.Bytes(), w.notifyOn) {
		w.once.Do(func() {
			w.notified <- w.Body.Len()
		})
	}
}

func newAwsTestClient(httpClient bedrockruntime.HTTPClient) *bedrockruntime.Client {
	return bedrockruntime.New(bedrockruntime.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String("https://bedrock.test"),
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
			"access-key", "secret-key", "",
		)),
		HTTPClient: httpClient,
		Retryer:    aws.NopRetryer{},
	})
}

func newAwsTestContext(writer http.ResponseWriter, requestContext context.Context) *gin.Context {
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestContext)
	return c
}

func newAwsTestRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		StartTime:          time.Now(),
		IsStream:           true,
		OriginModelName:    awsTestModel,
		RelayFormat:        relaytypes.RelayFormatOpenAI,
		ShouldIncludeUsage: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: awsTestModel,
		},
	}
}

func newAwsInvokeModelInput() *bedrockruntime.InvokeModelInput {
	return &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(awsTestModel),
		Body:        []byte(`{}`),
		Accept:      aws.String("application/json"),
		ContentType: aws.String("application/json"),
	}
}

func newAwsStreamInput() *bedrockruntime.InvokeModelWithResponseStreamInput {
	return &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId:     aws.String(awsTestModel),
		Body:        []byte(`{}`),
		Accept:      aws.String("application/json"),
		ContentType: aws.String("application/json"),
	}
}

func writeAwsStreamEvent(writer io.Writer, data string) error {
	payload, err := common.Marshal(struct {
		Bytes []byte `json:"bytes"`
	}{Bytes: []byte(data)})
	if err != nil {
		return err
	}

	return eventstream.NewEncoder().Encode(writer, eventstream.Message{
		Headers: eventstream.Headers{
			{Name: eventstreamapi.MessageTypeHeader, Value: eventstream.StringValue(eventstreamapi.EventMessageType)},
			{Name: eventstreamapi.EventTypeHeader, Value: eventstream.StringValue("chunk")},
			{Name: eventstreamapi.ContentTypeHeader, Value: eventstream.StringValue("application/json")},
		},
		Payload: payload,
	})
}

func newAwsStreamResponse(request *http.Request, body io.ReadCloser) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Type":                []string{"application/vnd.amazon.eventstream"},
			"X-Amzn-Bedrock-Content-Type": []string{"application/json"},
		},
		Body:    body,
		Request: request,
	}
}

func TestDoAwsClientRequest_AppliesRuntimeHeaderOverrideToAnthropicBeta(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName:           "claude-3-5-sonnet-20240620",
		IsStream:                  false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"anthropic-beta": "computer-use-2025-01-24",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "access-key|secret-key|us-east-1",
			UpstreamModelName: "claude-3-5-sonnet-20240620",
		},
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	adaptor := &Adaptor{}

	_, err := doAwsClientRequest(ctx, info, adaptor, requestBody)
	require.NoError(t, err)

	awsReq, ok := adaptor.AwsReq.(*bedrockruntime.InvokeModelInput)
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(awsReq.Body, &payload))

	anthropicBeta, exists := payload["anthropic_beta"]
	require.True(t, exists)

	values, ok := anthropicBeta.([]any)
	require.True(t, ok)
	require.Equal(t, []any{"computer-use-2025-01-24"}, values)
}

func TestNewAwsInvokeContextInheritsParent(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
	})

	tests := []struct {
		name         string
		relayTimeout int
		wantDeadline bool
	}{
		{name: "without relay timeout", relayTimeout: 0, wantDeadline: false},
		{name: "with relay timeout", relayTimeout: 30, wantDeadline: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			common.RelayTimeout = test.relayTimeout
			parent, cancelParent := context.WithCancel(context.Background())
			invokeContext, cancelInvoke := newAwsInvokeContext(parent)
			defer cancelInvoke()

			_, hasDeadline := invokeContext.Deadline()
			assert.Equal(t, test.wantDeadline, hasDeadline)

			cancelParent()
			require.ErrorIs(t, invokeContext.Err(), context.Canceled)
		})
	}
}

func TestNewAwsInvokeErrorSkipsRetryOnlyForClientCancellation(t *testing.T) {
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name           string
		requestContext context.Context
		err            error
		wantSkipRetry  bool
	}{
		{
			name:           "client context canceled",
			requestContext: canceledContext,
			err:            context.Canceled,
			wantSkipRetry:  true,
		},
		{
			name:           "relay timeout with live client context",
			requestContext: context.Background(),
			err:            context.DeadlineExceeded,
			wantSkipRetry:  false,
		},
		{
			name:           "upstream error with live client context",
			requestContext: context.Background(),
			err:            errors.New("upstream failed"),
			wantSkipRetry:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := newAwsInvokeError(test.requestContext, test.err, "InvokeModel")
			assert.Equal(t, test.wantSkipRetry, relaytypes.IsSkipRetryError(err))
		})
	}
}

func TestAwsResourceNotFoundIsChannelModelHard(t *testing.T) {
	message := "model resource not found"
	info := newAwsTestRelayInfo()
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)
	info.MarkAttemptUpstreamStarted()
	handlerErr := newAwsInvokeError(context.Background(), &bedrockruntimeTypes.ResourceNotFoundException{Message: &message}, "InvokeModel")

	assert.Equal(t, relaytypes.ErrorCodeModelNotFound, handlerErr.GetErrorCode())
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestAwsHandlersCancelSdkRequestAndSkipRetry(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
	})

	tests := []struct {
		name    string
		request any
		handle  func(*gin.Context, *relaycommon.RelayInfo, *Adaptor) (*relaytypes.NewAPIError, *dto.Usage)
	}{
		{name: "non-stream", request: newAwsInvokeModelInput(), handle: awsHandler},
		{name: "stream", request: newAwsStreamInput(), handle: awsStreamHandler},
		{name: "nova", request: newAwsInvokeModelInput(), handle: handleNovaRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestContext, cancelRequest := context.WithCancel(context.Background())
			t.Cleanup(cancelRequest)

			upstreamContexts := make(chan context.Context, 1)
			client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
				upstreamContexts <- request.Context()
				<-request.Context().Done()
				return nil, request.Context().Err()
			}))
			adaptor := &Adaptor{AwsClient: client, AwsReq: test.request}
			c := newAwsTestContext(httptest.NewRecorder(), requestContext)
			info := newAwsTestRelayInfo()

			type handlerResult struct {
				err   *relaytypes.NewAPIError
				usage *dto.Usage
			}
			results := make(chan handlerResult, 1)
			go func() {
				err, usage := test.handle(c, info, adaptor)
				results <- handlerResult{err: err, usage: usage}
			}()

			var upstreamContext context.Context
			select {
			case upstreamContext = <-upstreamContexts:
			case result := <-results:
				t.Fatalf("handler returned before issuing AWS request: %v", result.err)
			case <-time.After(5 * time.Second):
				t.Fatal("AWS request did not start")
			}

			cancelRequest()

			var result handlerResult
			select {
			case result = <-results:
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not stop after client cancellation")
			}

			require.ErrorIs(t, upstreamContext.Err(), context.Canceled)
			require.NotNil(t, result.err)
			assert.True(t, relaytypes.IsSkipRetryError(result.err))
			assert.Nil(t, result.usage)
		})
	}
}

func TestDoAwsClientRequestChannelConstructionFailurePublishesHealthFailure(t *testing.T) {
	info := newAwsTestRelayInfo()
	info.ApiKey = "invalid-aws-secret"
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)
	c := newAwsTestContext(httptest.NewRecorder(), context.Background())

	_, err := doAwsClientRequest(c, info, &Adaptor{}, bytes.NewBufferString(`{}`))

	require.Error(t, err)
	apiErr, ok := err.(*relaytypes.NewAPIError)
	require.True(t, ok)
	sample, observed := info.FinishDynamicRoutingAttempt(apiErr)
	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.True(t, sample.UpstreamStartedAt.IsZero())
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}

func TestAwsAPIKeyConfigurationFailurePublishesPreDispatchHealthFailure(t *testing.T) {
	info := newAwsTestRelayInfo()
	info.ApiKey = "invalid-api-key-without-region"
	info.ChannelOtherSettings.AwsKeyType = dto.AwsKeyTypeApiKey
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)

	_, err := (&Adaptor{}).GetRequestURL(info)
	require.Error(t, err)
	apiErr, ok := err.(*relaytypes.NewAPIError)
	require.True(t, ok)
	sample, observed := info.FinishDynamicRoutingAttempt(apiErr)

	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.True(t, sample.UpstreamStartedAt.IsZero())
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}

func TestAwsNovaSyntheticStreamRecordsVisibleTTFTWithoutTPOT(t *testing.T) {
	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"output":{"message":{"content":[{"text":"hello from nova"}]}},"usage":{"inputTokens":2,"outputTokens":3,"totalTokens":5}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	}))
	info := newAwsTestRelayInfo()
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)

	handlerErr, usage := handleNovaRequest(
		newAwsTestContext(httptest.NewRecorder(), context.Background()),
		info,
		&Adaptor{AwsClient: client, AwsReq: newAwsInvokeModelInput()},
	)
	require.Nil(t, handlerErr)
	require.NotNil(t, usage)
	assert.Equal(t, "hello from nova", info.DynamicRoutingAttemptVisibleText())
	info.SetAttemptCompletionTokens(usage.CompletionTokens)
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.True(t, observed)
	assert.True(t, sample.Success)
	assert.False(t, sample.FirstContentAt.IsZero())
	assert.False(t, sample.HasTPOT)
}

func TestAwsStreamHandlerUsesFinalUpstreamUsage(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
	})

	events := []string{
		`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-test","content":[],"usage":{"input_tokens":100,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":423}}`,
		`{"type":"message_stop"}`,
	}
	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		var body bytes.Buffer
		for _, event := range events {
			if err := writeAwsStreamEvent(&body, event); err != nil {
				return nil, err
			}
		}
		return newAwsStreamResponse(request, io.NopCloser(bytes.NewReader(body.Bytes()))), nil
	}))
	adaptor := &Adaptor{AwsClient: client, AwsReq: newAwsStreamInput()}
	recorder := httptest.NewRecorder()
	c := newAwsTestContext(recorder, context.Background())

	handlerErr, usage := awsStreamHandler(c, newAwsTestRelayInfo(), adaptor)

	require.Nil(t, handlerErr)
	require.NotNil(t, usage)
	require.NotNil(t, usage.BillingUsage)
	require.NotNil(t, usage.BillingUsage.ClaudeUsage)
	assert.Equal(t, 100, usage.BillingUsage.ClaudeUsage.InputTokens)
	assert.Equal(t, 423, usage.BillingUsage.ClaudeUsage.OutputTokens)
	assert.Contains(t, recorder.Body.String(), "[DONE]")
}

func TestAwsStreamHandlerRecordsOnlyProtocolVisibleTextForDynamicRouting(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
	})

	tests := []struct {
		name               string
		events             []string
		wantFirstContentAt bool
	}{
		{
			name: "anthropic text delta is visible",
			events: []string{
				`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[]}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
				`{"type":"message_stop"}`,
			},
			wantFirstContentAt: true,
		},
		{
			name: "metadata and thinking remain unmeasured",
			events: []string{
				`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[]}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hidden"}}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
				`{"type":"message_stop"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
				var body bytes.Buffer
				for _, event := range tt.events {
					require.NoError(t, writeAwsStreamEvent(&body, event))
				}
				return newAwsStreamResponse(request, io.NopCloser(bytes.NewReader(body.Bytes()))), nil
			}))
			adaptor := &Adaptor{AwsClient: client, AwsReq: newAwsStreamInput()}
			info := newAwsTestRelayInfo()
			info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)

			handlerErr, usage := awsStreamHandler(
				newAwsTestContext(httptest.NewRecorder(), context.Background()),
				info,
				adaptor,
			)

			require.Nil(t, handlerErr)
			require.NotNil(t, usage)
			require.NotNil(t, info.StreamStatus)
			assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)

			sample, observed := info.FinishDynamicRoutingAttempt(nil)
			require.True(t, observed)
			assert.True(t, sample.Success)
			assert.False(t, sample.UpstreamStartedAt.IsZero(), "InvokeModelWithResponseStream is the AWS dispatch boundary")
			assert.Equal(t, tt.wantFirstContentAt, !sample.FirstContentAt.IsZero())
			if !tt.wantFirstContentAt {
				assert.False(t, sample.HasTTFT)
			}
		})
	}
}

func TestAwsTerminalMarkerOnlyStreamIsProtocolFailure(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() { common.RelayTimeout = originalRelayTimeout })

	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		var body bytes.Buffer
		require.NoError(t, writeAwsStreamEvent(&body, `{"type":"message_stop"}`))
		return newAwsStreamResponse(request, io.NopCloser(bytes.NewReader(body.Bytes()))), nil
	}))
	info := newAwsTestRelayInfo()
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)

	handlerErr, usage := awsStreamHandler(
		newAwsTestContext(httptest.NewRecorder(), context.Background()),
		info,
		&Adaptor{AwsClient: client, AwsReq: newAwsStreamInput()},
	)

	require.Nil(t, handlerErr)
	require.NotNil(t, usage)
	sample, observed := info.FinishDynamicRoutingAttempt(nil)
	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
	assert.False(t, sample.HasTTFT)
	assert.False(t, sample.HasTPOT)
}

func TestAwsStreamHandlerRecordsVisibleContentBeforeDownstreamWrite(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
	})

	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		var body bytes.Buffer
		require.NoError(t, writeAwsStreamEvent(&body, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`))
		require.NoError(t, writeAwsStreamEvent(&body, `{"type":"message_stop"}`))
		return newAwsStreamResponse(request, io.NopCloser(bytes.NewReader(body.Bytes()))), nil
	}))
	info := newAwsTestRelayInfo()
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)
	var sample relaycommon.DynamicRoutingAttemptSample
	var observed bool
	writer := &awsInspectingResponseWriter{ResponseRecorder: httptest.NewRecorder()}
	writer.onVisibleWrite = func() {
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
		sample, observed = info.FinishDynamicRoutingAttempt(nil)
	}

	handlerErr, usage := awsStreamHandler(
		newAwsTestContext(writer, context.Background()),
		info,
		&Adaptor{AwsClient: client, AwsReq: newAwsStreamInput()},
	)

	require.Nil(t, handlerErr)
	require.NotNil(t, usage)
	require.True(t, observed)
	assert.False(t, sample.FirstContentAt.IsZero())
	assert.False(t, sample.TTFTInvalidated)
	assert.False(t, sample.TPOTInvalidated)
}

func TestAwsSlowDownstreamInvalidatesTPOTAfterBoundedIngressFills(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() { common.RelayTimeout = originalRelayTimeout })
	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		var body bytes.Buffer
		for i := 0; i < 12; i++ {
			require.NoError(t, writeAwsStreamEvent(&body, fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"token_%d"}}`, i)))
		}
		require.NoError(t, writeAwsStreamEvent(&body, `{"type":"message_stop"}`))
		return newAwsStreamResponse(request, io.NopCloser(bytes.NewReader(body.Bytes()))), nil
	}))
	writer := &awsBlockingResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		blocked:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	info := newAwsTestRelayInfo()
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)
	type result struct {
		err   *relaytypes.NewAPIError
		usage *dto.Usage
	}
	resultChan := make(chan result, 1)
	go func() {
		err, usage := awsStreamHandler(newAwsTestContext(writer, context.Background()), info, &Adaptor{AwsClient: client, AwsReq: newAwsStreamInput()})
		resultChan <- result{err: err, usage: usage}
	}()

	select {
	case <-writer.blocked:
	case <-time.After(time.Second):
		t.Fatal("downstream writer did not block on first visible chunk")
	}
	backpressured := make(chan struct{})
	stopPolling := make(chan struct{})
	defer close(stopPolling)
	go func() {
		for {
			if info.DynamicRoutingAttemptBackpressured() {
				close(backpressured)
				return
			}
			select {
			case <-stopPolling:
				return
			default:
				runtime.Gosched()
			}
		}
	}()
	select {
	case <-backpressured:
	case <-time.After(time.Second):
		t.Fatal("bounded AWS ingress did not mark downstream backpressure")
	}
	close(writer.release)
	var handlerResult result
	select {
	case handlerResult = <-resultChan:
	case <-time.After(time.Second):
		t.Fatal("AWS handler did not finish after downstream resumed")
	}
	require.Nil(t, handlerResult.err)
	require.NotNil(t, handlerResult.usage)

	sample, observed := info.FinishDynamicRoutingAttempt(nil)
	require.True(t, observed)
	assert.False(t, sample.TTFTInvalidated)
	assert.True(t, sample.TPOTInvalidated)
}

func TestAwsStreamHandlerClassifiesBinaryEventDecodeFailure(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
	})

	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		return newAwsStreamResponse(request, io.NopCloser(bytes.NewReader([]byte("invalid-event-stream")))), nil
	}))
	info := newAwsTestRelayInfo()
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)

	handlerErr, usage := awsStreamHandler(
		newAwsTestContext(httptest.NewRecorder(), context.Background()),
		info,
		&Adaptor{AwsClient: client, AwsReq: newAwsStreamInput()},
	)

	require.Nil(t, handlerErr)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonScannerErr, info.StreamStatus.EndReason)
	require.Error(t, info.StreamStatus.EndError)

	sample, observed := info.FinishDynamicRoutingAttempt(nil)
	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
}

func TestAwsSilentEventStreamTriggersDynamicRoutingTimeout(t *testing.T) {
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		return newAwsStreamResponse(request, reader), nil
	}))
	info := newAwsTestRelayInfo()
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), awsTestModel, true)

	type result struct {
		err   *relaytypes.NewAPIError
		usage *dto.Usage
	}
	resultChan := make(chan result, 1)
	go func() {
		handlerErr, usage := awsStreamHandler(
			newAwsTestContext(httptest.NewRecorder(), context.Background()),
			info,
			&Adaptor{AwsClient: client, AwsReq: newAwsStreamInput()},
		)
		resultChan <- result{err: handlerErr, usage: usage}
	}()
	var handlerResult result
	select {
	case handlerResult = <-resultChan:
	case <-time.After(2 * time.Second):
		_ = writer.CloseWithError(errors.New("release silent AWS stream"))
		select {
		case <-resultChan:
		case <-time.After(time.Second):
		}
		t.Fatal("silent AWS event stream did not trigger streaming timeout")
	}
	handlerErr, usage := handlerResult.err, handlerResult.usage
	sample, observed := info.FinishDynamicRoutingAttempt(handlerErr)

	require.Nil(t, handlerErr)
	require.NotNil(t, usage)
	require.True(t, observed)
	assert.True(t, sample.HardFailure)
	assert.False(t, sample.Success)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason)
}

func TestAwsStreamHandlerStopsAtClientCancellationAndKeepsPartialBillingUsage(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
	})

	requestContext, cancelRequest := context.WithCancel(context.Background())
	t.Cleanup(cancelRequest)
	releaseFinal := make(chan struct{})
	var releaseFinalOnce sync.Once
	release := func() {
		releaseFinalOnce.Do(func() {
			close(releaseFinal)
		})
	}
	t.Cleanup(release)

	producerResults := make(chan error, 1)
	upstreamContexts := make(chan context.Context, 1)
	client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
		upstreamContexts <- request.Context()
		reader, writer := io.Pipe()
		go func() {
			defer writer.Close()
			initialEvents := []string{
				`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-test","content":[],"usage":{"input_tokens":100,"output_tokens":1}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}`,
			}
			for _, event := range initialEvents {
				if err := writeAwsStreamEvent(writer, event); err != nil {
					producerResults <- err
					return
				}
			}

			<-releaseFinal
			producerResults <- writeAwsStreamEvent(writer, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":423}}`)
		}()
		return newAwsStreamResponse(request, reader), nil
	}))

	responseWriter := newAwsNotifyingResponseWriter("partial")
	c := newAwsTestContext(responseWriter, requestContext)
	adaptor := &Adaptor{AwsClient: client, AwsReq: newAwsStreamInput()}
	info := newAwsTestRelayInfo()

	type handlerResult struct {
		err   *relaytypes.NewAPIError
		usage *dto.Usage
	}
	results := make(chan handlerResult, 1)
	go func() {
		err, usage := awsStreamHandler(c, info, adaptor)
		results <- handlerResult{err: err, usage: usage}
	}()

	var upstreamContext context.Context
	select {
	case upstreamContext = <-upstreamContexts:
	case <-time.After(5 * time.Second):
		t.Fatal("AWS stream request did not start")
	}

	var bodyLengthBeforeCancel int
	select {
	case bodyLengthBeforeCancel = <-responseWriter.notified:
	case <-time.After(5 * time.Second):
		t.Fatal("partial response was not written")
	}
	cancelRequest()

	var result handlerResult
	select {
	case result = <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("stream handler did not stop after client cancellation")
	}

	require.ErrorIs(t, upstreamContext.Err(), context.Canceled)
	require.Nil(t, result.err)
	require.NotNil(t, result.usage)
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	require.NotNil(t, result.usage.BillingUsage)
	require.NotNil(t, result.usage.BillingUsage.ClaudeUsage)
	assert.Equal(t, dto.BillingUsageSourceClaudeMessages, result.usage.BillingUsage.Source)
	assert.Equal(t, dto.BillingUsageSemanticAnthropic, result.usage.BillingUsage.Semantic)
	assert.Equal(t, 100, result.usage.BillingUsage.ClaudeUsage.InputTokens)
	assert.Equal(t, 1, result.usage.BillingUsage.ClaudeUsage.OutputTokens)
	assert.Equal(t, bodyLengthBeforeCancel, responseWriter.Body.Len())
	assert.NotContains(t, responseWriter.Body.String(), "[DONE]")

	release()
	select {
	case producerErr := <-producerResults:
		require.Error(t, producerErr)
	case <-time.After(5 * time.Second):
		t.Fatal("upstream producer did not observe the closed stream")
	}
}
