package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIOCopyBytesGracefullyNormalizesChatCompletionIDBeforeContentLength(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(c, constant.ContextKeyResponseId, "550e8400-e29b-41d4-a716-446655440000")
	input := []byte(`{"id":"gen_upstream","choices":[{"message":{"tool_calls":[{"id":"call_keep"}]}}]}`)

	upstreamHeaders := http.Header{
		"ETag":           []string{`"upstream-body-hash"`},
		"Content-Md5":    []string{"upstream-content-md5"},
		"Digest":         []string{"sha-256=upstream-digest"},
		"Content-Digest": []string{"sha-256=:upstream-content-digest:"},
		"Repr-Digest":    []string{"sha-256=:upstream-repr-digest:"},
		"X-Provider":     []string{"keep"},
	}
	IOCopyBytesGracefully(c, &http.Response{StatusCode: http.StatusOK, Header: upstreamHeaders}, input)

	assert.JSONEq(t, `{"id":"550e8400-e29b-41d4-a716-446655440000","choices":[{"message":{"tool_calls":[{"id":"call_keep"}]}}]}`, recorder.Body.String())
	assert.Equal(t, int64(recorder.Body.Len()), recorder.Result().ContentLength)
	assert.Empty(t, recorder.Header().Get("ETag"))
	assert.Empty(t, recorder.Header().Get("Content-MD5"))
	assert.Empty(t, recorder.Header().Get("Digest"))
	assert.Empty(t, recorder.Header().Get("Content-Digest"))
	assert.Empty(t, recorder.Header().Get("Repr-Digest"))
	assert.Equal(t, "keep", recorder.Header().Get("X-Provider"))
	assert.Equal(t, "gen_upstream", c.GetString(common.UpstreamResponseIdKey))
}

func TestShouldCopyUpstreamHeaderBlocksConfiguredHeadersAndCapturesValues(t *testing.T) {
	original := append([]string(nil), operation_setting.GetErrorSetting().BlockedResponseHeaders...)
	require.NoError(t, operation_setting.UpdateBlockedResponseHeaders([]string{
		"X-Modelverse-Request-Id",
		"X-Request-Id",
		"X-Trace-Id",
		"X-Vendor-Debug-Id",
	}))
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdateBlockedResponseHeaders(original))
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	tests := []struct {
		name   string
		header string
		value  string
	}{
		{name: "modelverse", header: "x-modelverse-request-id", value: "modelverse-id"},
		{name: "request", header: "X-Request-ID", value: "request-id"},
		{name: "trace", header: "X-Trace-Id", value: "trace-id"},
		{name: "custom", header: "X-Vendor-Debug-Id", value: "vendor-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, ShouldCopyUpstreamHeader(c, tt.header, []string{tt.value}))
		})
	}
	assert.True(t, ShouldCopyUpstreamHeader(c, "X-Provider", []string{"safe"}))

	captured, ok := common.GetContextKeyType[map[string]string](c, common.UpstreamResponseHeadersKey)
	require.True(t, ok)
	assert.Equal(t, "modelverse-id", captured["X-Modelverse-Request-Id"])
	assert.Equal(t, "request-id", captured["X-Request-Id"])
	assert.Equal(t, "trace-id", captured["X-Trace-Id"])
	assert.Equal(t, "vendor-id", captured["X-Vendor-Debug-Id"])
}

func TestShouldCopyUpstreamHeaderAlwaysProtectsGatewayRequestID(t *testing.T) {
	original := append([]string(nil), operation_setting.GetErrorSetting().BlockedResponseHeaders...)
	require.NoError(t, operation_setting.UpdateBlockedResponseHeaders([]string{}))
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdateBlockedResponseHeaders(original))
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.False(t, ShouldCopyUpstreamHeader(c, common.RequestIdKey, []string{"upstream-oneapi-id"}))
	assert.Equal(t, "upstream-oneapi-id", c.GetString(common.UpstreamRequestIdKey))

	captured, ok := common.GetContextKeyType[map[string]string](c, common.UpstreamResponseHeadersKey)
	require.True(t, ok)
	assert.Equal(t, "upstream-oneapi-id", captured[http.CanonicalHeaderKey(common.RequestIdKey)])
}

func TestCaptureUpstreamResponseHeadersReplacesRetryAttempt(t *testing.T) {
	original := append([]string(nil), operation_setting.GetErrorSetting().BlockedResponseHeaders...)
	require.NoError(t, operation_setting.UpdateBlockedResponseHeaders([]string{"X-Trace-Id"}))
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdateBlockedResponseHeaders(original))
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	CaptureUpstreamResponseHeaders(c, http.Header{
		"X-Trace-Id": []string{"failed-attempt"},
	})
	ResetUpstreamResponseMetadata(c)
	CaptureUpstreamResponseHeaders(c, http.Header{
		"X-Request-Id": []string{"not-configured"},
	})

	captured, ok := common.GetContextKeyType[map[string]string](c, common.UpstreamResponseHeadersKey)
	require.True(t, ok)
	assert.Empty(t, captured)
}

func TestResetUpstreamResponseMetadataClearsPriorRetryAttempt(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.UpstreamRequestIdKey, "prior-request")
	c.Set(common.UpstreamResponseIdKey, "prior-response")
	c.Set(common.UpstreamResponseHeadersKey, map[string]string{"X-Trace-Id": "prior-trace"})

	ResetUpstreamResponseMetadata(c)

	assert.Empty(t, c.GetString(common.UpstreamRequestIdKey))
	assert.Empty(t, c.GetString(common.UpstreamResponseIdKey))
	captured, ok := common.GetContextKeyType[map[string]string](c, common.UpstreamResponseHeadersKey)
	require.True(t, ok)
	assert.Empty(t, captured)
}

func TestShouldCopyUpstreamHeaderLimitsCapturedValueSize(t *testing.T) {
	original := append([]string(nil), operation_setting.GetErrorSetting().BlockedResponseHeaders...)
	require.NoError(t, operation_setting.UpdateBlockedResponseHeaders([]string{"X-Trace-Id"}))
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdateBlockedResponseHeaders(original))
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.False(t, ShouldCopyUpstreamHeader(c, "X-Trace-Id", []string{
		strings.Repeat("t", common.MaxUpstreamIdentifierBytes+100),
	}))
	captured, ok := common.GetContextKeyType[map[string]string](c, common.UpstreamResponseHeadersKey)
	require.True(t, ok)
	assert.LessOrEqual(t, len(captured["X-Trace-Id"]), common.MaxUpstreamIdentifierBytes)
	assert.True(t, strings.HasSuffix(captured["X-Trace-Id"], "…"))
}

func TestShouldCopyUpstreamHeaderFitsLegacyLogColumn(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	assert.False(t, ShouldCopyUpstreamHeader(c, common.RequestIdKey, []string{
		strings.Repeat("r", common.MaxUpstreamRequestIdentifierBytes+100),
	}))

	captured := c.GetString(common.UpstreamRequestIdKey)
	assert.LessOrEqual(t, len(captured), common.MaxUpstreamRequestIdentifierBytes)
	assert.True(t, strings.HasSuffix(captured, "…"))
}
