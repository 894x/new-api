package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeAdvancedCustomBalanceResponseDoesNotExposeValues(t *testing.T) {
	response := json.RawMessage(`{
		"api_key": "sk-sensitive",
		"account": {"email": "owner@example.com", "token": "secret-token"},
		"balance": 12345,
		"items": ["private-value"]
	}`)

	summary, err := summarizeAdvancedCustomBalanceResponse(response)
	require.NoError(t, err)
	assert.JSONEq(t, `{"response_type":"object","field_count":4}`, summary)
	assert.NotContains(t, summary, "sk-sensitive")
	assert.NotContains(t, summary, "owner@example.com")
	assert.NotContains(t, summary, "secret-token")
	assert.NotContains(t, summary, "12345")
	assert.NotContains(t, summary, "private-value")
}
