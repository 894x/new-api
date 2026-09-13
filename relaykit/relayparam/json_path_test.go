package relayparam

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func TestResolveJSONPathsLiteralObjectKeys(t *testing.T) {
	data := []byte(`{"items":{"a.b":{"value":1},"*":{"value":2},"a\\b":{"value":3},"a(b":{"value":4},"a\"b":{"value":5},"#":{"value":6},"|":{"value":7},"@this":{"value":8}}}`)
	paths, err := ResolveJSONPaths(data, "items.*.value", false)
	require.NoError(t, err)
	require.Len(t, paths, 8)
	for _, path := range paths {
		original := gjson.GetBytes(data, path)
		require.True(t, original.Exists(), path)
		changed, err := sjson.SetBytes(data, path, original.Int()+10)
		require.NoError(t, err)
		assert.Equal(t, original.Int()+10, gjson.GetBytes(changed, path).Int(), path)
		assert.Len(t, gjson.GetBytes(changed, "items").Map(), 8, "must not create accidental nested keys")
	}
}

func TestResolveJSONPathsMissingAndNegativeIndices(t *testing.T) {
	data := []byte(`{"groups":[[{"value":1},{}],[{"value":2}],[],"text"]}`)
	existing, err := ResolveJSONPaths(data, "groups.*.-1.value", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"groups.1.0.value"}, existing)
	creatable, err := ResolveJSONPaths(data, "groups.*.-1.value", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"groups.0.1.value", "groups.1.0.value"}, creatable)
}
