package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiChatHandlerClaudeMessagesExposesCachedContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatClaude,
		OriginModelName: "gemini-2.5-flash",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gemini-2.5-flash"},
	}
	body, err := common.Marshal(dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{{
			Content: dto.GeminiChatContent{Role: "model", Parts: []dto.GeminiPart{{Text: "ok"}}},
		}},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        200,
			CandidatesTokenCount:    4,
			TotalTokenCount:         204,
			CachedContentTokenCount: 128,
		},
	})
	require.NoError(t, err)

	usage, apiErr := GeminiChatHandler(c, info, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 128, usage.PromptTokensDetails.CachedTokens)

	responseUsage := gjson.GetBytes(recorder.Body.Bytes(), "usage")
	assert.Equal(t, int64(128), responseUsage.Get("cache_read_input_tokens").Int())
	assert.Equal(t, int64(0), responseUsage.Get("cache_creation_input_tokens").Int())
	assert.True(t, responseUsage.Get("cache_creation_input_tokens").Exists())
	assert.False(t, responseUsage.Get("cache_creation").Exists())
	assert.Equal(t, "128", recorder.Header().Get("x-cache-read-input-tokens"))
	assert.Equal(t, "0", recorder.Header().Get("x-cache-creation-input-tokens"))
}
