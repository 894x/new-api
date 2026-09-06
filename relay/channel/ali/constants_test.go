package ali

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWan3ModelsAreBuiltIntoAliChannel(t *testing.T) {
	require.Contains(t, ModelList, "wan3.0-video")
	require.Contains(t, ModelList, "wan3.0-video-prime")
}
