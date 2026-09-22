package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWanMediaReferencesKeepTypesAndLeaveNonMediaURLsUntouched(t *testing.T) {
	for _, tc := range []struct{ role, kind string }{
		{"first_frame", "Image"}, {"last_frame", "Image"}, {"reference_image", "Image"},
		{"reference_video", "Video"}, {"reference_audio", "Audio"}, {"driving_audio", "Audio"},
	} {
		t.Run(tc.role, func(t *testing.T) {
			payload := map[string]any{"input": map[string]any{
				"prompt": "https://prompt.example/leave-alone", "callback_url": "https://callback.example/leave-alone",
				"media": []any{map[string]any{"type": tc.role, "url": "https://source.example/reference"}},
			}}
			calls := 0
			value, err := walkVideoAssetReferences(payload, "", func(source, kind string) (string, error) {
				calls++
				assert.Equal(t, "https://source.example/reference", source)
				assert.Equal(t, tc.kind, kind)
				return "https://owned.example/reference", nil
			})
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
			input := value.(map[string]any)["input"].(map[string]any)
			assert.Equal(t, "https://prompt.example/leave-alone", input["prompt"])
			assert.Equal(t, "https://callback.example/leave-alone", input["callback_url"])
			assert.Equal(t, []any{map[string]any{"type": tc.role, "url": "https://owned.example/reference"}}, input["media"])
			assert.Equal(t, []any{map[string]any{"type": tc.role, "url": "https://source.example/reference"}}, payload["input"].(map[string]any)["media"], "retry source snapshot must not be mutated")
		})
	}
}
