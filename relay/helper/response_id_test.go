package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringDataNormalizesOnlyTopLevelChatCompletionID(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyResponseId, "550e8400-e29b-41d4-a716-446655440000")

	err := StringData(c, `{"id":"gen_upstream","choices":[{"delta":{"tool_calls":[{"id":"call_keep"}]}}]}`)
	require.NoError(t, err)

	body := recorder.Body.String()
	assert.Contains(t, body, `"id":"550e8400-e29b-41d4-a716-446655440000"`)
	assert.Contains(t, body, `"id":"call_keep"`)
	assert.False(t, strings.Contains(body, "gen_upstream"))
}

func TestGetResponseIDUsesGatewayChatCompletionID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.RequestIdKey, "request-id")
	common.SetContextKey(c, constant.ContextKeyResponseId, "550e8400-e29b-41d4-a716-446655440000")

	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", GetResponseID(c))
	assert.Equal(t, "chatcmpl-request-id", GetDefaultUpstreamUserID(c))
}

func TestWriteJSONNormalizesDirectChatCompletionOutput(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(c, constant.ContextKeyResponseId, "550e8400-e29b-41d4-a716-446655440000")

	_, err := WriteJSON(c, []byte(`{"id":"provider-id","choices":[]}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"550e8400-e29b-41d4-a716-446655440000","choices":[]}`, recorder.Body.String())
}
