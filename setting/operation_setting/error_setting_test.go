package operation_setting

import (
	"fmt"
	"net/http"
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

func TestDefaultErrorResponseReplacementRules(t *testing.T) {
	moonshotMessage, matched := MatchErrorResponseReplacement(
		http.StatusInternalServerError,
		"请联系 Moonshot AI 技术支持。",
	)
	assert.True(t, matched)
	assert.Equal(t, "请联系 rhzs 技术支持。", moonshotMessage)

	tencentMessage, matched := MatchErrorResponseReplacement(
		http.StatusTooManyRequests,
		"Please contact Tencent Cloud support to request a higher limit.",
	)
	assert.True(t, matched)
	assert.Equal(t, "Please contact rhzs support to request a higher limit.", tencentMessage)
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

func TestErrorResponseReplacementRulesMatchStatusAndMessage(t *testing.T) {
	original := append([]ErrorResponseReplacementRule(nil), GetErrorSetting().ResponseReplacementRules...)
	t.Cleanup(func() {
		require.NoError(t, UpdateErrorResponseReplacementRules(original))
	})

	require.NoError(t, UpdateErrorResponseReplacementRulesFromJSON(`[
		{"status_code":500,"match":"Moonshot AI","replacement":"rhzs"},
		{"status_code":429,"match":"rate limit","replacement":"请求过于频繁。"}
	]`))

	replacement, matched := MatchErrorResponseReplacement(
		http.StatusInternalServerError,
		"服务处理请求时发生内部错误，请稍后重试。若持续出现该问题，请联系 Moonshot AI 技术支持。",
	)
	require.True(t, matched)
	assert.Equal(t, "服务处理请求时发生内部错误，请稍后重试。若持续出现该问题，请联系 rhzs 技术支持。", replacement)
	_, matched = MatchErrorResponseReplacement(http.StatusBadGateway, "Moonshot AI 技术支持")
	assert.False(t, matched)
	_, matched = MatchErrorResponseReplacement(http.StatusInternalServerError, "unrelated error")
	assert.False(t, matched)
}

func TestErrorResponseReplacementRulesApplyInOrder(t *testing.T) {
	original := append([]ErrorResponseReplacementRule(nil), GetErrorSetting().ResponseReplacementRules...)
	t.Cleanup(func() {
		require.NoError(t, UpdateErrorResponseReplacementRules(original))
	})

	require.NoError(t, UpdateErrorResponseReplacementRules([]ErrorResponseReplacementRule{
		{StatusCode: 500, Match: "Moonshot AI", Replacement: "rhzs"},
		{StatusCode: 500, Match: "技术支持", Replacement: "support"},
	}))

	replacement, matched := MatchErrorResponseReplacement(500, "请联系 Moonshot AI 技术支持。")
	require.True(t, matched)
	assert.Equal(t, "请联系 rhzs support。", replacement)
}

func TestErrorResponseReplacementRulesUseRegexWithLiteralReplacement(t *testing.T) {
	original := append([]ErrorResponseReplacementRule(nil), GetErrorSetting().ResponseReplacementRules...)
	t.Cleanup(func() {
		require.NoError(t, UpdateErrorResponseReplacementRules(original))
	})

	require.NoError(t, UpdateErrorResponseReplacementRules([]ErrorResponseReplacementRule{{
		StatusCode:  http.StatusTooManyRequests,
		Match:       `(?i)(moonshot|tencent) cloud`,
		Replacement: `${1} {{provider}} $1`,
	}}))

	replacement, matched := MatchErrorResponseReplacement(
		http.StatusTooManyRequests,
		"Contact Moonshot Cloud or TENCENT CLOUD support.",
	)
	require.True(t, matched)
	assert.Equal(t, "Contact ${1} {{provider}} $1 or ${1} {{provider}} $1 support.", replacement)
}

func TestValidateErrorResponseReplacementRulesRejectsInvalidRules(t *testing.T) {
	tests := []string{
		`not-json`,
		`null`,
		`[{"status_code":200,"match":"error","replacement":"retry"}]`,
		`[{"status_code":500,"match":"","replacement":"retry"}]`,
		`[{"status_code":500,"match":"(","replacement":"retry"}]`,
		`[{"status_code":500,"match":"` + strings.Repeat("a", MaxErrorResponseReplacementTextLength+1) + `","replacement":"retry"}]`,
	}
	for _, input := range tests {
		_, err := ValidateErrorResponseReplacementRulesJSON(input)
		assert.Error(t, err, input)
	}
}

func TestRegisteredConfigUpdateRollsBackInvalidReplacementRules(t *testing.T) {
	original := GetErrorSetting()
	registered := config.GlobalConfig.Get("error_setting")

	err := config.UpdateConfigFromMap(registered, map[string]string{
		"hide_error_details":         "false",
		"response_replacement_rules": `[{"status_code":500,"match":"(","replacement":"retry"}]`,
	})
	require.Error(t, err)
	assert.Equal(t, original, GetErrorSetting())
}

func TestGetErrorSettingReturnsReplacementRuleSnapshot(t *testing.T) {
	original := append([]ErrorResponseReplacementRule(nil), GetErrorSetting().ResponseReplacementRules...)
	t.Cleanup(func() {
		require.NoError(t, UpdateErrorResponseReplacementRules(original))
	})
	require.NoError(t, UpdateErrorResponseReplacementRules([]ErrorResponseReplacementRule{{
		StatusCode: 500, Match: "error", Replacement: "retry",
	}}))

	snapshot := GetErrorSetting()
	snapshot.ResponseReplacementRules[0].Replacement = "mutated"
	replacement, matched := MatchErrorResponseReplacement(500, "error")

	require.True(t, matched)
	assert.Equal(t, "retry", replacement)
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
