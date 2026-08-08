package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const xunfeiHTTP200Error = `{"error":{"code":11210,"message":"NotEnoughCvError"}}`

func newHTTP200ErrorTestContext(t *testing.T, body, contentType string, stream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{contentType}},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "xopdeepseekv4flash"},
		RelayFormat: types.RelayFormatOpenAI,
		IsStream:    stream,
	}
	return c, recorder, resp, info
}

func TestOpenAIChatHandlersRejectHTTP200BusinessError(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	t.Run("non-stream", func(t *testing.T) {
		c, recorder, resp, info := newHTTP200ErrorTestContext(t, xunfeiHTTP200Error, "application/json", false)

		usage, err := OpenaiHandler(c, info, resp)

		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, "11210", string(err.GetErrorCode()))
		require.Empty(t, recorder.Body.String())
	})

	t.Run("stream before output", func(t *testing.T) {
		body := "data: " + xunfeiHTTP200Error + "\n\n"
		c, recorder, resp, info := newHTTP200ErrorTestContext(t, body, "text/event-stream", true)

		usage, err := OaiStreamHandler(c, info, resp)

		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, "11210", string(err.GetErrorCode()))
		require.Empty(t, recorder.Body.String())
		require.NotNil(t, info.StreamStatus)
		require.True(t, info.StreamStatus.HasErrors())
	})

	t.Run("stream after output", func(t *testing.T) {
		chunk := `{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test","choices":[{"index":0,"delta":{"content":"partial"}}]}`
		body := "data: " + chunk + "\n\ndata: " + xunfeiHTTP200Error + "\n\n"
		c, recorder, resp, info := newHTTP200ErrorTestContext(t, body, "text/event-stream", true)

		usage, err := OaiStreamHandler(c, info, resp)

		require.Nil(t, usage)
		require.NotNil(t, err)
		require.True(t, types.IsResponseCommittedError(err), "body=%q written=%t size=%d", recorder.Body.String(), c.Writer.Written(), c.Writer.Size())
		require.True(t, types.IsSkipRetryError(err))
		require.Contains(t, recorder.Body.String(), "partial")
		require.NotContains(t, recorder.Body.String(), "NotEnoughCvError")
	})

	t.Run("stream after keepalive ping", func(t *testing.T) {
		body := "data: " + xunfeiHTTP200Error + "\n\n"
		c, recorder, resp, info := newHTTP200ErrorTestContext(t, body, "text/event-stream", true)
		require.NoError(t, helper.PingData(c))

		usage, err := OaiStreamHandler(c, info, resp)

		require.Nil(t, usage)
		require.NotNil(t, err)
		require.True(t, types.IsResponseCommittedError(err))
		require.True(t, types.IsSkipRetryError(err))
		require.Contains(t, recorder.Body.String(), ": PING")
		require.NotContains(t, recorder.Body.String(), "NotEnoughCvError")
	})
}

func TestChatToResponsesHandlersRejectHTTP200BusinessError(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	t.Run("non-stream", func(t *testing.T) {
		c, recorder, resp, info := newHTTP200ErrorTestContext(t, xunfeiHTTP200Error, "application/json", false)

		usage, err := OaiChatToResponsesHandler(c, info, resp)

		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, "11210", string(err.GetErrorCode()))
		require.Empty(t, recorder.Body.String())
	})

	t.Run("stream before output", func(t *testing.T) {
		body := "data: " + xunfeiHTTP200Error + "\n\n"
		c, recorder, resp, info := newHTTP200ErrorTestContext(t, body, "text/event-stream", true)

		usage, err := OaiChatToResponsesStreamHandler(c, info, resp)

		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, "11210", string(err.GetErrorCode()))
		require.Empty(t, recorder.Body.String())
		require.NotNil(t, info.StreamStatus)
		require.True(t, info.StreamStatus.HasErrors())
	})

	t.Run("stream after output", func(t *testing.T) {
		chunk := `{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"test","choices":[{"index":0,"delta":{"content":"partial"}}]}`
		body := "data: " + chunk + "\n\ndata: " + xunfeiHTTP200Error + "\n\n"
		c, recorder, resp, info := newHTTP200ErrorTestContext(t, body, "text/event-stream", true)

		usage, err := OaiChatToResponsesStreamHandler(c, info, resp)

		require.Nil(t, usage)
		require.NotNil(t, err)
		require.True(t, types.IsResponseCommittedError(err))
		require.True(t, types.IsSkipRetryError(err))
		require.Contains(t, recorder.Body.String(), "partial")
		require.NotContains(t, recorder.Body.String(), "NotEnoughCvError")
	})

	t.Run("stream after keepalive ping", func(t *testing.T) {
		body := "data: " + xunfeiHTTP200Error + "\n\n"
		c, recorder, resp, info := newHTTP200ErrorTestContext(t, body, "text/event-stream", true)
		require.NoError(t, helper.PingData(c))

		usage, err := OaiChatToResponsesStreamHandler(c, info, resp)

		require.Nil(t, usage)
		require.NotNil(t, err)
		require.True(t, types.IsResponseCommittedError(err))
		require.True(t, types.IsSkipRetryError(err))
		require.Contains(t, recorder.Body.String(), ": PING")
		require.NotContains(t, recorder.Body.String(), "NotEnoughCvError")
	})
}
