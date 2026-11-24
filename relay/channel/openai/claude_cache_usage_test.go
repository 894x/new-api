package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeCachedTokenAliasesPriority(t *testing.T) {
	t.Parallel()

	usage := &dto.Usage{
		CachedTokens:         7,
		PromptCacheHitTokens: 3,
		InputTokensDetails:   &dto.InputTokenDetails{CachedTokens: 40},
	}
	normalizeCachedTokenAliases(usage)
	assert.Equal(t, 40, usage.PromptTokensDetails.CachedTokens)

	usage = &dto.Usage{CachedTokens: 7, PromptCacheHitTokens: 3}
	normalizeCachedTokenAliases(usage)
	assert.Equal(t, 7, usage.PromptTokensDetails.CachedTokens)

	usage = &dto.Usage{PromptCacheHitTokens: 3}
	normalizeCachedTokenAliases(usage)
	assert.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)

	usage = &dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 11}, PromptCacheHitTokens: 3}
	normalizeCachedTokenAliases(usage)
	assert.Equal(t, 11, usage.PromptTokensDetails.CachedTokens)
}

func TestOpenaiHandlerClaudeMessagesExposesCacheAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		usage        string
		wantRead     int64
		wantCreation int64
	}{
		{
			name:         "prompt cache hit tokens",
			usage:        `{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_cache_hit_tokens":33}`,
			wantRead:     33,
			wantCreation: 0,
		},
		{
			name:         "top-level cached tokens beat prompt cache hit tokens",
			usage:        `{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"cached_tokens":21,"prompt_cache_hit_tokens":9}`,
			wantRead:     21,
			wantCreation: 0,
		},
		{
			name:         "input token details beat other aliases",
			usage:        `{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"input_tokens_details":{"cached_tokens":40},"cached_tokens":7,"prompt_cache_hit_tokens":3}`,
			wantRead:     40,
			wantCreation: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"id":"chatcmpl-cache","object":"chat.completion","created":1710000000,"model":"alias-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":` + tt.usage + `}`
			c, recorder, resp, info := newClaudeMessagesResponseContext(t, body, false)
			info.ChannelType = constant.ChannelTypeAli

			usage, apiErr := OpenaiHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, int(tt.wantRead), usage.PromptTokensDetails.CachedTokens)

			responseUsage := gjson.GetBytes(recorder.Body.Bytes(), "usage")
			assert.Equal(t, tt.wantRead, responseUsage.Get("cache_read_input_tokens").Int())
			assert.Equal(t, tt.wantCreation, responseUsage.Get("cache_creation_input_tokens").Int())
			assert.True(t, responseUsage.Get("cache_read_input_tokens").Exists())
			assert.True(t, responseUsage.Get("cache_creation_input_tokens").Exists())
			assert.Equal(t, strconv.FormatInt(tt.wantRead, 10), recorder.Header().Get("x-cache-read-input-tokens"))
			assert.Equal(t, strconv.FormatInt(tt.wantCreation, 10), recorder.Header().Get("x-cache-creation-input-tokens"))
		})
	}
}

func TestOaiStreamHandlerClaudeMessagesExposesCacheAlias(t *testing.T) {
	prepareDynamicRoutingStreamTest(t)

	withUsage := `{"id":"chat_cache","model":"alias-model","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}],"usage":{"prompt_tokens":100,"completion_tokens":2,"total_tokens":102,"prompt_cache_hit_tokens":55}}`
	finish := `{"id":"chat_cache","model":"alias-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
	combined := `{"id":"chat_cache","model":"alias-model","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":2,"total_tokens":102,"prompt_cache_hit_tokens":55}}`

	tests := []struct {
		name   string
		frames []string
	}{
		{name: "usage before final frame", frames: []string{withUsage, finish}},
		{name: "usage on final frame", frames: []string{combined}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body strings.Builder
			for _, frame := range tt.frames {
				body.WriteString("data: " + frame + "\n\n")
			}
			body.WriteString("data: [DONE]\n\n")
			c, recorder, resp, info := newClaudeMessagesResponseContext(t, body.String(), true)
			info.ChannelType = constant.ChannelTypeAli

			usage, apiErr := OaiStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 55, usage.PromptTokensDetails.CachedTokens)

			var startUsage, deltaUsage gjson.Result
			for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
				payload, ok := strings.CutPrefix(line, "data: ")
				if !ok {
					continue
				}
				event := gjson.Parse(payload)
				switch event.Get("type").String() {
				case "message_start":
					startUsage = event.Get("message.usage")
				case "message_delta":
					deltaUsage = event.Get("usage")
				}
			}
			require.True(t, startUsage.Exists())
			require.True(t, deltaUsage.Exists())
			assert.Equal(t, int64(55), startUsage.Get("cache_read_input_tokens").Int())
			assert.Equal(t, int64(0), startUsage.Get("cache_creation_input_tokens").Int())
			assert.Equal(t, int64(55), deltaUsage.Get("cache_read_input_tokens").Int())
			assert.Equal(t, int64(0), deltaUsage.Get("cache_creation_input_tokens").Int())
			assert.Empty(t, recorder.Header().Get("x-cache-read-input-tokens"))
			assert.Empty(t, recorder.Header().Get("x-cache-creation-input-tokens"))
		})
	}
}

func newClaudeMessagesResponseContext(t *testing.T, body string, isStream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(common.RequestIdKey, common.NewRequestId())

	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatClaude, nil, nil)
	require.NoError(t, err)
	info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "alias-model"}
	info.IsStream = isStream
	info.DisablePing = true
	info.SetEstimatePromptTokens(100)

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
