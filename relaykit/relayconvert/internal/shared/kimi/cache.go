package kimi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

// Kimi expresses the same cache-write policy with different top-level fields
// in Messages and OpenAI APIs. Other providers retain their own cache semantics.
func PromptCacheOptions(model string, info convmeta.Meta, raw json.RawMessage) (json.RawMessage, error) {
	empty := len(raw) == 0 || string(raw) == "null"
	if !isKimiModel(model, info) || empty {
		return nil, nil
	}
	var control struct {
		Type string `json:"type"`
		TTL  string `json:"ttl,omitempty"`
	}
	if err := kitutil.Unmarshal(raw, &control); err != nil {
		return nil, fmt.Errorf("invalid Kimi cache_control: %w", err)
	}
	if control.Type != "ephemeral" {
		return nil, fmt.Errorf("Kimi cache_control.type must be ephemeral")
	}
	switch control.TTL {
	case "", "5m", "1h":
	default:
		return nil, fmt.Errorf("Kimi cache_control.ttl must be 5m or 1h")
	}
	return kitutil.Marshal(struct {
		Mode string `json:"mode"`
		TTL  string `json:"ttl,omitempty"`
	}{Mode: "implicit", TTL: control.TTL})
}

func CacheControl(model string, info convmeta.Meta, raw json.RawMessage) (json.RawMessage, error) {
	empty := len(raw) == 0 || string(raw) == "null"
	if !isKimiModel(model, info) || empty {
		return nil, nil
	}
	var options struct {
		Mode string `json:"mode"`
		TTL  string `json:"ttl,omitempty"`
	}
	if err := kitutil.Unmarshal(raw, &options); err != nil {
		return nil, fmt.Errorf("invalid Kimi prompt_cache_options: %w", err)
	}
	if options.Mode != "implicit" {
		return nil, fmt.Errorf("Kimi prompt_cache_options.mode must be implicit")
	}
	switch options.TTL {
	case "", "5m", "1h":
	default:
		return nil, fmt.Errorf("Kimi prompt_cache_options.ttl must be 5m or 1h")
	}
	return kitutil.Marshal(struct {
		Type string `json:"type"`
		TTL  string `json:"ttl,omitempty"`
	}{Type: "ephemeral", TTL: options.TTL})
}

func isKimiModel(model string, info convmeta.Meta) bool {
	if upstream := convmeta.UpstreamModelName(info); upstream != "" {
		model = upstream
	}
	return strings.HasPrefix(strings.ToLower(model), "kimi-")
}
