package kimi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCachePolicyConversion(t *testing.T) {
	for _, raw := range []string{"", "null"} {
		got, err := PromptCacheOptions("kimi-k3", nil, []byte(raw))
		require.NoError(t, err)
		require.Empty(t, got)
		got, err = CacheControl("kimi-k3", nil, []byte(raw))
		require.NoError(t, err)
		require.Empty(t, got)
	}
	got, err := PromptCacheOptions("kimi-k3", nil, []byte(`{"type":"ephemeral"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"mode":"implicit"}`, string(got))
	got, err = CacheControl("kimi-k3", nil, []byte(`{"mode":"implicit"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"ephemeral"}`, string(got))
	for _, raw := range []string{`{`, `[]`, `{"type":"permanent"}`, `{"type":"ephemeral","ttl":"2h"}`, `{"type":"ephemeral","ttl":5}`} {
		_, err := PromptCacheOptions("kimi-k3", nil, []byte(raw))
		require.Error(t, err, raw)
	}
	for _, raw := range []string{`{`, `[]`, `{"mode":"explicit"}`, `{"mode":"implicit","ttl":"2h"}`, `{"mode":"implicit","ttl":5}`} {
		_, err := CacheControl("kimi-k3", nil, []byte(raw))
		require.Error(t, err, raw)
	}
}
