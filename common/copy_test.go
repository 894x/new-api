package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepCopyKeepsMutableRequestFieldsIndependent(t *testing.T) {
	type request struct {
		Raw     json.RawMessage
		Bytes   []byte
		Nested  []json.RawMessage
		Content []any
		Enabled *bool
		Count   *uint
	}
	enabled, count := false, uint(0)
	src := request{
		Raw:   json.RawMessage(`{"value":false,"id":9007199254740993}`),
		Bytes: []byte("media"), Nested: []json.RawMessage{json.RawMessage(`{"n":0}`)},
		Content: []any{map[string]any{"image_url": "data:image/png;base64,AAAA"}},
		Enabled: &enabled, Count: &count,
	}
	dst, err := DeepCopy(&src)
	require.NoError(t, err)
	require.Equal(t, src, *dst)
	dst.Raw[2] = 'V'
	dst.Bytes[0] = 'M'
	dst.Nested[0][2] = 'N'
	dst.Content[0].(map[string]any)["image_url"] = "changed"
	*dst.Enabled = true
	*dst.Count = 1
	assert.Equal(t, `{"value":false,"id":9007199254740993}`, string(src.Raw))
	assert.Equal(t, "media", string(src.Bytes))
	assert.Equal(t, `{"n":0}`, string(src.Nested[0]))
	assert.Equal(t, "data:image/png;base64,AAAA", src.Content[0].(map[string]any)["image_url"])
	assert.False(t, *src.Enabled)
	assert.Zero(t, *src.Count)
}
