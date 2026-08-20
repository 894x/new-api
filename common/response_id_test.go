package common

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestReplaceTopLevelJSONIDRewritesOnlyProtocolID(t *testing.T) {
	input := []byte(`{"id":"gen_upstream","object":"chat.completion","choices":[{"message":{"tool_calls":[{"id":"call_keep"}]}}]}`)

	output, err := ReplaceTopLevelJSONID(input, "550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", gjson.GetBytes(output, "id").String())
	assert.Equal(t, "call_keep", gjson.GetBytes(output, "choices.0.message.tool_calls.0.id").String())
}

func TestReplaceTopLevelJSONIDAddsMissingProtocolID(t *testing.T) {
	input := []byte(`{"object":"chat.completion.chunk","choices":[]}`)

	output, err := ReplaceTopLevelJSONID(input, "550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", gjson.GetBytes(output, "id").String())
}

func TestReplaceTopLevelJSONIDAddsIDToCompletionShapeWithoutObject(t *testing.T) {
	input := []byte(`{"choices":[{"message":{"content":"ok"}}]}`)

	output, err := ReplaceTopLevelJSONID(input, "550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", gjson.GetBytes(output, "id").String())
}

func TestReplaceTopLevelJSONIDReturnsExistingGatewayPayloadUnchanged(t *testing.T) {
	input := []byte(`{"id":"550e8400-e29b-41d4-a716-446655440000","object":"chat.completion.chunk","choices":[]}`)

	output, err := ReplaceTopLevelJSONID(input, "550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.Equal(t, input, output)
}

func TestReplaceTopLevelJSONIDLeavesErrorPayloadUntouched(t *testing.T) {
	input := []byte(`{"error":{"message":"upstream failed"}}`)

	output, err := ReplaceTopLevelJSONID(input, "550e8400-e29b-41d4-a716-446655440000")
	require.NoError(t, err)
	assert.Equal(t, input, output)
}

func TestReplaceTopLevelJSONIDAllowsEmptyErrorFields(t *testing.T) {
	for _, errorValue := range []string{"null", "{}"} {
		t.Run(errorValue, func(t *testing.T) {
			input := []byte(`{"id":"provider-id","object":"chat.completion","error":` + errorValue + `,"choices":[]}`)

			output, err := ReplaceTopLevelJSONID(input, "550e8400-e29b-41d4-a716-446655440000")
			require.NoError(t, err)
			assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", gjson.GetBytes(output, "id").String())
		})
	}
}

func TestCaptureUpstreamResponseIDLimitsUntrustedIdentifierSize(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	upstreamID := strings.Repeat("a", MaxUpstreamIdentifierBytes+100)
	payload := []byte(`{"id":"` + upstreamID + `","choices":[]}`)

	CaptureUpstreamResponseID(c, payload, "public-id")

	captured := c.GetString(UpstreamResponseIdKey)
	assert.LessOrEqual(t, len(captured), MaxUpstreamIdentifierBytes)
	assert.True(t, strings.HasSuffix(captured, "…"))
}
