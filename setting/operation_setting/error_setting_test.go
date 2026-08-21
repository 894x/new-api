package operation_setting

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorDetailsAreHiddenByDefault(t *testing.T) {
	assert.True(t, ShouldHideErrorDetails())
}

func TestDefaultBlockedResponseHeaders(t *testing.T) {
	assert.Equal(t, []string{
		"X-Modelverse-Request-Id",
		"X-Request-Id",
		"X-Trace-Id",
	}, GetErrorSetting().BlockedResponseHeaders)
}

func TestShouldBlockUpstreamResponseHeaderIsCaseInsensitive(t *testing.T) {
	original := append([]string(nil), GetErrorSetting().BlockedResponseHeaders...)
	require.NoError(t, UpdateBlockedResponseHeaders([]string{" X-Custom-Trace "}))
	t.Cleanup(func() {
		require.NoError(t, UpdateBlockedResponseHeaders(original))
	})

	assert.True(t, ShouldBlockUpstreamResponseHeader("x-custom-trace"))
	assert.False(t, ShouldBlockUpstreamResponseHeader("X-Request-Id"))
}

func TestUpdateBlockedResponseHeadersFromJSONNormalizesValues(t *testing.T) {
	original := append([]string(nil), GetErrorSetting().BlockedResponseHeaders...)
	t.Cleanup(func() {
		require.NoError(t, UpdateBlockedResponseHeaders(original))
	})

	require.NoError(t, UpdateBlockedResponseHeadersFromJSON(
		`["x-custom-request-id"," X-Debug-Trace ","X-Custom-Request-Id"]`,
	))
	assert.Equal(t, []string{"X-Custom-Request-Id", "X-Debug-Trace"}, GetErrorSetting().BlockedResponseHeaders)
	assert.True(t, ShouldBlockUpstreamResponseHeader("X-CUSTOM-REQUEST-ID"))
}

func TestUpdateBlockedResponseHeadersFromJSONAllowsEmptyList(t *testing.T) {
	original := append([]string(nil), GetErrorSetting().BlockedResponseHeaders...)
	t.Cleanup(func() {
		require.NoError(t, UpdateBlockedResponseHeaders(original))
	})

	require.NoError(t, UpdateBlockedResponseHeadersFromJSON(`[]`))
	assert.Empty(t, GetErrorSetting().BlockedResponseHeaders)
	assert.False(t, ShouldBlockUpstreamResponseHeader("X-Request-Id"))
}

func TestRegisteredConfigUpdatePublishesBlockedHeaderIndex(t *testing.T) {
	original := GetErrorSetting()
	t.Cleanup(func() {
		UpdateHideErrorDetails(original.HideErrorDetails)
		require.NoError(t, UpdateBlockedResponseHeaders(original.BlockedResponseHeaders))
	})

	registered := config.GlobalConfig.Get("error_setting")
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{
		"blocked_response_headers": `["X-Loaded-Trace-Id"]`,
		"hide_error_details":       "false",
	}))
	assert.True(t, ShouldBlockUpstreamResponseHeader("x-loaded-trace-id"))
	assert.False(t, ShouldHideErrorDetails())
	assert.Equal(t, []string{"X-Loaded-Trace-Id"}, GetErrorSetting().BlockedResponseHeaders)
}

func TestRegisteredConfigUpdateRollsBackInvalidBlockedHeaders(t *testing.T) {
	original := GetErrorSetting()
	registered := config.GlobalConfig.Get("error_setting")

	err := config.UpdateConfigFromMap(registered, map[string]string{
		"blocked_response_headers": `["X Invalid"]`,
		"hide_error_details":       "false",
	})
	require.Error(t, err)
	assert.Equal(t, original, GetErrorSetting())
}

func TestGetErrorSettingReturnsBlockedHeaderSnapshot(t *testing.T) {
	original := GetErrorSetting()
	require.NotEmpty(t, original.BlockedResponseHeaders)
	original.BlockedResponseHeaders[0] = "X-Mutated"

	assert.False(t, ShouldBlockUpstreamResponseHeader("X-Mutated"))
	assert.NotEqual(t, "X-Mutated", GetErrorSetting().BlockedResponseHeaders[0])
}

func TestValidateBlockedResponseHeadersJSONRejectsInvalidLists(t *testing.T) {
	tooMany := make([]byte, 0, 512)
	tooMany = append(tooMany, '[')
	for i := 0; i <= MaxBlockedResponseHeaderCount; i++ {
		if i > 0 {
			tooMany = append(tooMany, ',')
		}
		tooMany = append(tooMany, fmt.Sprintf(`"X-Test-%d"`, i)...)
	}
	tooMany = append(tooMany, ']')

	tests := []string{
		`not-json`,
		`null`,
		`["X Good"]`,
		`["` + strings.Repeat("A", MaxBlockedResponseHeaderNameLength+1) + `"]`,
		string(tooMany),
	}
	for _, input := range tests {
		_, err := ValidateBlockedResponseHeadersJSON(input)
		assert.Error(t, err, input)
	}
}
