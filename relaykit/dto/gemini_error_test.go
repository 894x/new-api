package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiChatResponseUnmarshalPreservesErrorEnvelope(t *testing.T) {
	var response GeminiChatResponse
	require.NoError(t, response.UnmarshalJSON([]byte(`{"error":{"code":429,"message":"limited","status":"RESOURCE_EXHAUSTED"}}`)))
	require.NotNil(t, response.Error)
	assert.Equal(t, 429, response.Error.Code)
	assert.Equal(t, "limited", response.Error.Message)
	assert.Equal(t, "RESOURCE_EXHAUSTED", response.Error.Status)
}
