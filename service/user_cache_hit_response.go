package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Transform only the client copy, never the upstream DTO used by metrics and
// settlement. Repeated stream events reuse the same request decision.
func TransformUserCacheHitResponse(c *gin.Context, data []byte) []byte {
	s, ok := common.GetContextKeyType[*cacheHitPolicyState](c, cacheHitPolicyContextKey)
	if !ok || !gjson.ValidBytes(data) {
		return data
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, prefix := range []string{"usage", "message.usage", "response.usage"} {
		u := gjson.GetBytes(data, prefix)
		if !u.IsObject() {
			continue
		}
		claude := u.Get("cache_read_input_tokens").Exists()
		input := u.Get("prompt_tokens")
		if !input.Exists() {
			input = u.Get("input_tokens")
		}
		cachePaths := []string{"prompt_tokens_details.cached_tokens", "input_tokens_details.cached_tokens", "cached_tokens", "prompt_cache_hit_tokens"}
		if claude {
			cachePaths = []string{"cache_read_input_tokens"}
		}
		var cached gjson.Result
		for _, path := range cachePaths {
			if value := u.Get(path); value.Exists() {
				cached = value
				break
			}
		}
		if !cached.Exists() {
			continue
		}
		// Multimodal cache breakdowns have separate expression prices; retain
		// their real facts until all categories can be transformed consistently.
		if u.Get("prompt_tokens_details.cached_tokens_details.audio_tokens").Int() > 0 || u.Get("prompt_tokens_details.cached_tokens_details.image_tokens").Int() > 0 {
			if s.decision == nil {
				s.failure = "multimodal_cache_breakdown"
			}
			return data
		}
		r := cached.Int()
		p := input.Int()
		if claude {
			writes := u.Get("cache_creation_input_tokens").Int()
			if writes == 0 {
				writes = u.Get("cache_creation.ephemeral_5m_input_tokens").Int() + u.Get("cache_creation.ephemeral_1h_input_tokens").Int()
			}
			p += r + writes
		}
		if cached.Type != gjson.Number || cached.Float() != float64(r) || r < 0 || r > 2147483647 {
			return data
		}
		decision := s.decision
		if !input.Exists() && decision == nil {
			s.failure = "incomplete_client_cache_usage"
			return data
		}
		if input.Exists() {
			if input.Type != gjson.Number || input.Float() != float64(input.Int()) || p <= 0 || p > 2147483647 || r > p {
				continue
			}
			decision = s.allocate(int(p), int(r))
		}
		if decision == nil {
			continue
		}
		gap := min(int64(decision.GapTokens), r)
		updated := data
		for _, path := range cachePaths {
			if u.Get(path).Exists() {
				var err error
				updated, err = sjson.SetBytes(updated, prefix+"."+path, r-gap)
				if err != nil {
					return data
				}
			}
		}
		if claude && input.Exists() {
			var err error
			updated, err = sjson.SetBytes(updated, prefix+".input_tokens", input.Int()+gap)
			if err != nil {
				return data
			}
		}
		if u.Get("prompt_tokens_details.cached_tokens_details.text_tokens").Exists() {
			var err error
			updated, err = sjson.SetBytes(updated, prefix+".prompt_tokens_details.cached_tokens_details.text_tokens", r-gap)
			if err != nil {
				return data
			}
		}
		return updated
	}
	return data
}
