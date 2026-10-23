package relayconvert

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestKimiCacheTTLRequestConversion(t *testing.T) {
	formats := []types.RelayFormat{
		types.RelayFormatOpenAI,
		types.RelayFormatOpenAIResponses,
		types.RelayFormatClaude,
	}
	for _, ttl := range []string{"5m", "1h"} {
		for _, from := range formats {
			for _, to := range formats {
				t.Run(fmt.Sprintf("%s/%s/%s", ttl, from, to), func(t *testing.T) {
					options := []byte(fmt.Sprintf(`{"mode":"implicit","ttl":%q}`, ttl))
					control := []byte(fmt.Sprintf(`{"type":"ephemeral","ttl":%q}`, ttl))
					var request any
					maxTokens := uint(64)
					switch from {
					case types.RelayFormatOpenAI:
						// Exercise the actual typed decode/encode boundary that used to drop the field.
						var chat dto.GeneralOpenAIRequest
						raw := fmt.Sprintf(`{"model":"kimi-k3","max_tokens":64,"messages":[{"role":"user","content":"hi"}],"prompt_cache_options":%s}`, options)
						require.NoError(t, kitutil.Unmarshal([]byte(raw), &chat))
						request = &chat
					case types.RelayFormatOpenAIResponses:
						request = &dto.OpenAIResponsesRequest{
							Model: "kimi-k3", Input: []byte(`"hi"`), MaxOutputTokens: &maxTokens,
							PromptCacheOptions: options,
						}
					case types.RelayFormatClaude:
						request = &dto.ClaudeRequest{
							Model: "kimi-k3", MaxTokens: &maxTokens, CacheControl: control,
							Messages: []dto.ClaudeMessage{{Role: "user", Content: "hi"}},
						}
					}
					result, err := ConvertRequest(context.Background(), &convmeta.Values{}, to, request)
					require.NoError(t, err)
					raw, err := kitutil.Marshal(result.Value)
					require.NoError(t, err)
					field := "prompt_cache_options"
					if to == types.RelayFormatClaude {
						field = "cache_control"
						require.Equal(t, "ephemeral", gjson.GetBytes(raw, field+".type").String())
					} else {
						require.Equal(t, "implicit", gjson.GetBytes(raw, field+".mode").String())
					}
					require.Equal(t, ttl, gjson.GetBytes(raw, field+".ttl").String())
				})
			}
		}
	}
}

func TestKimiCacheTTLUsesMappedModel(t *testing.T) {
	for _, upstream := range []string{"kimi-k3", "claude-sonnet-4"} {
		t.Run(upstream, func(t *testing.T) {
			req := &dto.ClaudeRequest{
				Model: "public-alias", CacheControl: []byte(`{"type":"ephemeral","ttl":"1h"}`),
				Messages: []dto.ClaudeMessage{{Role: "user", Content: "hi"}},
			}
			info := &convmeta.Values{ChannelMetaAttached: true, UpstreamModelName: upstream}
			result, err := ConvertRequest(context.Background(), info, types.RelayFormatOpenAI, req)
			require.NoError(t, err)
			chat := result.Value.(*dto.GeneralOpenAIRequest)
			if upstream == "kimi-k3" {
				require.JSONEq(t, `{"mode":"implicit","ttl":"1h"}`, string(chat.PromptCacheOptions))
			} else {
				require.Empty(t, chat.PromptCacheOptions)
			}
		})
	}
}
