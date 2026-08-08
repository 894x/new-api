package xunfei_maas

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLUsesConfiguredMaaSBaseURL(t *testing.T) {
	adaptor := &Adaptor{}
	tests := []struct {
		name    string
		baseURL string
		wantURL string
	}{
		{name: "official default", wantURL: constant.ChannelBaseURLs[constant.ChannelTypeXunfeiMaaS] + requestPath},
		{name: "custom override", baseURL: "https://maas-proxy.example.com/root/", wantURL: "https://maas-proxy.example.com/root" + requestPath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			url, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: test.baseURL}})
			require.NoError(t, err)
			assert.Equal(t, test.wantURL, url)
		})
	}
}

func TestConvertClaudeRequestKeepsXunfeiStreamUsageOption(t *testing.T) {
	stream := true
	maxTokens := uint(128)
	message := dto.ClaudeMessage{Role: "user"}
	message.SetStringContent("hello")
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeXunfeiMaaS,
			UpstreamModelName: "xopdeepseekv4flash",
		},
		IsStream: true,
	}
	adaptor := &Adaptor{}
	adaptor.Init(info)

	converted, err := adaptor.ConvertClaudeRequest(nil, info, &dto.ClaudeRequest{
		Model:     "xopdeepseekv4flash",
		MaxTokens: &maxTokens,
		Stream:    &stream,
		Messages:  []dto.ClaudeMessage{message},
	})
	require.NoError(t, err)

	chatRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, chatRequest.StreamOptions)
	assert.True(t, chatRequest.StreamOptions.IncludeUsage)
}

func TestConvertResponsesRequestToNativeChatRequest(t *testing.T) {
	stream := true
	input, err := common.Marshal("hello")
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeXunfeiMaaS,
			UpstreamModelName: "xopdeepseekv4flash",
		},
		IsStream: true,
	}
	adaptor := &Adaptor{}
	adaptor.Init(info)

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model:  "xopdeepseekv4flash",
		Input:  input,
		Stream: &stream,
	})
	require.NoError(t, err)

	chatRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, chatRequest.Messages, 1)
	assert.Equal(t, "user", chatRequest.Messages[0].Role)
	assert.Equal(t, "hello", chatRequest.Messages[0].Content)
	require.NotNil(t, chatRequest.StreamOptions)
	assert.True(t, chatRequest.StreamOptions.IncludeUsage)
}

func TestSetupRequestHeaderUsesBearerAuthentication(t *testing.T) {
	recorder := &headerOnlyResponseWriter{header: http.Header{}}
	c, _ := gin.CreateTestContext(recorder)
	c.Request, _ = http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	header := http.Header{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "test-key"}}

	err := (&Adaptor{}).SetupRequestHeader(c, &header, info)

	require.NoError(t, err)
	assert.Equal(t, "Bearer test-key", header.Get("Authorization"))
}

func TestClassifyErrorRetryPolicy(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		wantStatus int
		wantSkip   bool
	}{
		{name: "tpm limit", code: "11210", wantStatus: http.StatusTooManyRequests},
		{name: "capacity", code: "10008", wantStatus: http.StatusServiceUnavailable},
		{name: "transport failure", code: "10002", wantStatus: http.StatusServiceUnavailable},
		{name: "context too long", code: "10907", wantStatus: http.StatusBadRequest, wantSkip: true},
		{name: "model unauthorized", code: "11221", wantStatus: http.StatusForbidden, wantSkip: true},
		{name: "unknown HTTP 200 body error", code: "19999", wantStatus: http.StatusBadGateway},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := types.WithOpenAIError(types.OpenAIError{
				Message: "upstream failed",
				Code:    test.code,
			}, http.StatusOK)

			got := classifyError(err)

			require.NotNil(t, got)
			assert.Equal(t, test.wantStatus, got.StatusCode)
			assert.Equal(t, test.wantSkip, types.IsSkipRetryError(got))
			assert.Equal(t, types.ErrorCode(test.code), got.GetErrorCode())
		})
	}
}

func TestClassifyErrorPreservesCommittedResponse(t *testing.T) {
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "upstream failed",
		Code:    "11210",
	}, http.StatusOK, types.ErrOptionWithResponseCommitted())

	got := classifyError(err)

	require.NotNil(t, got)
	assert.Equal(t, http.StatusTooManyRequests, got.StatusCode)
	assert.True(t, types.IsResponseCommittedError(got))
	assert.True(t, types.IsSkipRetryError(got))
}

func TestNormalizeUpstreamErrorAppliesToNonOKHTTPResponses(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"code":10907,"message":"context too long"}}`,
		)),
	}
	err := service.RelayErrorHandler(context.Background(), resp, false)

	got := channel.NormalizeUpstreamError(&Adaptor{}, err)

	require.NotNil(t, got)
	assert.Equal(t, http.StatusBadRequest, got.StatusCode)
	assert.True(t, types.IsSkipRetryError(got))
	assert.Equal(t, types.ErrorCode("10907"), got.GetErrorCode())
}

func TestDoResponseRejectsHTTP200BusinessErrorBeforeBilling(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	tests := []struct {
		name        string
		relayMode   int
		relayFormat types.RelayFormat
		stream      bool
	}{
		{name: "chat non-stream", relayMode: relayconstant.RelayModeChatCompletions, relayFormat: types.RelayFormatOpenAI},
		{name: "chat stream", relayMode: relayconstant.RelayModeChatCompletions, relayFormat: types.RelayFormatOpenAI, stream: true},
		{name: "responses non-stream", relayMode: relayconstant.RelayModeResponses, relayFormat: types.RelayFormatOpenAIResponses},
		{name: "responses stream", relayMode: relayconstant.RelayModeResponses, relayFormat: types.RelayFormatOpenAIResponses, stream: true},
		{name: "messages non-stream", relayMode: relayconstant.RelayModeChatCompletions, relayFormat: types.RelayFormatClaude},
		{name: "messages stream", relayMode: relayconstant.RelayModeChatCompletions, relayFormat: types.RelayFormatClaude, stream: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			body := `{"error":{"code":11210,"message":"NotEnoughCvError"}}`
			contentType := "application/json"
			if test.stream {
				body = "data: " + body + "\n\n"
				contentType = "text/event-stream"
			}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{"Content-Type": []string{contentType}},
			}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       constant.ChannelTypeXunfeiMaaS,
					UpstreamModelName: "xopdeepseekv4flash",
				},
				RelayMode:   test.relayMode,
				RelayFormat: test.relayFormat,
				IsStream:    test.stream,
				DisablePing: true,
			}
			adaptor := &Adaptor{}
			adaptor.Init(info)

			usage, err := adaptor.DoResponse(c, resp, info)

			require.Nil(t, usage)
			require.NotNil(t, err)
			assert.Equal(t, types.ErrorCode("11210"), err.GetErrorCode())
			assert.Equal(t, http.StatusTooManyRequests, err.StatusCode)
			assert.False(t, types.IsSkipRetryError(err))
			assert.Empty(t, recorder.Body.String())
		})
	}
}

type headerOnlyResponseWriter struct {
	header http.Header
}

func (w *headerOnlyResponseWriter) Header() http.Header            { return w.header }
func (w *headerOnlyResponseWriter) Write(data []byte) (int, error) { return len(data), nil }
func (w *headerOnlyResponseWriter) WriteHeader(_ int)              {}
