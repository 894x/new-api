package helper

import (
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/tidwall/sjson"
)

// SanitizeResponseBody removes internal billing metadata and usage fields owned
// by other client protocols. Billing must be captured before calling this.
func SanitizeResponseBody(body []byte, relayFormat types.RelayFormat) ([]byte, error) {
	var paths []string
	switch relayFormat {
	case types.RelayFormatOpenAI:
		paths = []string{
			"usage.billing_usage",
			"usage.usage_semantic",
			"usage.usage_source",
			"usage.input_tokens",
			"usage.output_tokens",
			"usage.input_tokens_details",
			"usage.output_tokens_details",
			"usage.claude_cache_creation_5_m_tokens",
			"usage.claude_cache_creation_1_h_tokens",
			"usage.prompt_tokens_details.cached_creation_tokens",
		}
	case types.RelayFormatOpenAIResponses:
		paths = []string{
			"usage.billing_usage",
			"usage.usage_semantic",
			"usage.usage_source",
			"usage.prompt_tokens",
			"usage.completion_tokens",
			"usage.cached_tokens",
			"usage.prompt_cache_hit_tokens",
			"usage.prompt_tokens_details",
			"usage.completion_tokens_details",
			"usage.claude_cache_creation_5_m_tokens",
			"usage.claude_cache_creation_1_h_tokens",
			"usage.input_tokens_details.cached_creation_tokens",
			"response.usage.billing_usage",
			"response.usage.usage_semantic",
			"response.usage.usage_source",
			"response.usage.prompt_tokens",
			"response.usage.completion_tokens",
			"response.usage.cached_tokens",
			"response.usage.prompt_cache_hit_tokens",
			"response.usage.prompt_tokens_details",
			"response.usage.completion_tokens_details",
			"response.usage.claude_cache_creation_5_m_tokens",
			"response.usage.claude_cache_creation_1_h_tokens",
			"response.usage.input_tokens_details.cached_creation_tokens",
		}
	case types.RelayFormatClaude:
		paths = []string{
			"usage.billing_usage",
			"usage.usage_semantic",
			"usage.usage_source",
			"usage.prompt_tokens",
			"usage.completion_tokens",
			"usage.total_tokens",
			"usage.cached_tokens",
			"usage.prompt_cache_hit_tokens",
			"usage.prompt_tokens_details",
			"usage.completion_tokens_details",
			"usage.input_tokens_details",
			"usage.output_tokens_details",
			"message.usage.billing_usage",
			"message.usage.usage_semantic",
			"message.usage.usage_source",
			"message.usage.prompt_tokens",
			"message.usage.completion_tokens",
			"message.usage.total_tokens",
			"message.usage.cached_tokens",
			"message.usage.prompt_cache_hit_tokens",
			"message.usage.prompt_tokens_details",
			"message.usage.completion_tokens_details",
			"message.usage.input_tokens_details",
			"message.usage.output_tokens_details",
		}
	default:
		return body, nil
	}

	var err error
	for _, path := range paths {
		body, err = sjson.DeleteBytes(body, path)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}
