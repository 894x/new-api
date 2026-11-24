package claude

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeMessagesPassthroughKeepsCacheUsageAndHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet",
		RelayFormat:     types.RelayFormatClaude,
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "claude-sonnet"},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}
	data := []byte(`{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"claude-sonnet",
		"content":[{"type":"text","text":"ok"}],
		"stop_reason":"end_turn",
		"usage":{
			"input_tokens":10,
			"output_tokens":4,
			"cache_read_input_tokens":80,
			"cache_creation_input_tokens":20,
			"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":0}
		}
	}`)

	apiErr := HandleClaudeResponseData(c, info, claudeInfo, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, data)
	require.Nil(t, apiErr)

	responseUsage := gjson.GetBytes(recorder.Body.Bytes(), "usage")
	assert.Equal(t, int64(80), responseUsage.Get("cache_read_input_tokens").Int())
	assert.Equal(t, int64(20), responseUsage.Get("cache_creation_input_tokens").Int())
	assert.Equal(t, int64(20), responseUsage.Get("cache_creation.ephemeral_5m_input_tokens").Int())
	assert.Equal(t, int64(0), responseUsage.Get("cache_creation.ephemeral_1h_input_tokens").Int())
	assert.False(t, responseUsage.Get("prompt_tokens").Exists())
	assert.Equal(t, "80", recorder.Header().Get("x-cache-read-input-tokens"))
	assert.Equal(t, "20", recorder.Header().Get("x-cache-creation-input-tokens"))
}
