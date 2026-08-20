package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newChatCompletionResponseIDTestContext(t *testing.T, body string, isStream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, common.NewRequestId())

	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
	require.NoError(t, err)
	info.ChannelMeta = &relaycommon.ChannelMeta{
		UpstreamModelName: "kimi-k3",
		ChannelSetting: dto.ChannelSettings{
			ForceFormat: true,
		},
	}
	info.IsStream = isStream
	info.ShouldIncludeUsage = isStream
	info.DisablePing = true

	contentType := "application/json"
	if isStream {
		contentType = "text/event-stream"
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{contentType}},
	}
	return c, recorder, resp, info
}

func TestOpenaiHandlerReplacesUpstreamChatCompletionIDWithUUID(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"id":"gen_upstream","object":"chat.completion","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"ok","tool_calls":[{"id":"call_keep","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, false)

	usage, apiErr := OpenaiHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	responseBody := recorder.Body.Bytes()
	responseID := gjson.GetBytes(responseBody, "id").String()
	parsedID, err := uuid.Parse(responseID)
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(4), parsedID.Version())
	assert.NotEqual(t, "gen_upstream", responseID)
	assert.Equal(t, "call_keep", gjson.GetBytes(responseBody, "choices.0.message.tool_calls.0.id").String())
}

func TestOaiStreamHandlerUsesOneUUIDForEveryChatCompletionChunk(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"id":"gen_upstream","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"gen_upstream","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"delta":{"content":"ok","tool_calls":[{"index":0,"id":"call_keep","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":null}]}`,
		`data: {"id":"gen_upstream","object":"chat.completion.chunk","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, true)

	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	var responseIDs []string
	var toolCallID string
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		responseID := gjson.Get(payload, "id").String()
		if responseID != "" {
			responseIDs = append(responseIDs, responseID)
		}
		if candidate := gjson.Get(payload, "choices.0.delta.tool_calls.0.id").String(); candidate != "" {
			toolCallID = candidate
		}
	}

	require.GreaterOrEqual(t, len(responseIDs), 4)
	parsedID, err := uuid.Parse(responseIDs[0])
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(4), parsedID.Version())
	for _, responseID := range responseIDs[1:] {
		assert.Equal(t, responseIDs[0], responseID)
	}
	assert.Equal(t, "call_keep", toolCallID)
	assert.Contains(t, recorder.Body.String(), "data: [DONE]")
}

func TestOpenaiHandlerGeneratesDifferentChatCompletionUUIDPerRequest(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"id":"same_upstream","object":"chat.completion","created":1710000000,"model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`
	responseIDs := make([]string, 0, 2)
	for range 2 {
		c, recorder, resp, info := newChatCompletionResponseIDTestContext(t, body, false)
		_, apiErr := OpenaiHandler(c, info, resp)
		require.Nil(t, apiErr)
		responseIDs = append(responseIDs, gjson.GetBytes(recorder.Body.Bytes(), "id").String())
	}

	require.NoError(t, uuid.Validate(responseIDs[0]))
	require.NoError(t, uuid.Validate(responseIDs[1]))
	assert.NotEqual(t, responseIDs[0], responseIDs[1])
}
