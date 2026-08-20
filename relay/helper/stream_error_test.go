package helper

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveUpstreamStreamErrorStatus(t *testing.T) {
	tests := []struct {
		name           string
		payload        string
		responseStatus int
		want           int
	}{
		{name: "top level status", payload: `{"status":400}`, responseStatus: http.StatusOK, want: http.StatusBadRequest},
		{name: "top level status code", payload: `{"status_code":429}`, responseStatus: http.StatusOK, want: http.StatusTooManyRequests},
		{name: "nested error code", payload: `{"error":{"code":403}}`, responseStatus: http.StatusOK, want: http.StatusForbidden},
		{name: "nested client status", payload: `{"error":{"status_code":400}}`, responseStatus: http.StatusOK, want: http.StatusBadRequest},
		{name: "nested numeric string rate limit", payload: `{"error":{"code":"429"}}`, responseStatus: http.StatusOK, want: http.StatusTooManyRequests},
		{name: "provider top level code", payload: `{"code":400}`, responseStatus: http.StatusOK, want: http.StatusBadRequest},
		{name: "uppercase provider error", payload: `{"Error":{"Code":429}}`, responseStatus: http.StatusOK, want: http.StatusTooManyRequests},
		{name: "cloudflare errors array", payload: `{"errors":[{"code":400}]}`, responseStatus: http.StatusOK, want: http.StatusBadRequest},
		{name: "nested response error status", payload: `{"response":{"error":{"status":500}}}`, responseStatus: http.StatusOK, want: http.StatusInternalServerError},
		{name: "string status", payload: `{"error":{"status_code":"401"}}`, responseStatus: http.StatusOK, want: http.StatusUnauthorized},
		{name: "actual upstream error status", payload: `{}`, responseStatus: http.StatusBadRequest, want: http.StatusBadRequest},
		{name: "unknown embedded error", payload: `{"error":{"code":"provider_code"}}`, responseStatus: http.StatusOK, want: http.StatusBadGateway},
		{name: "invalid payload", payload: `{`, responseStatus: http.StatusOK, want: http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveUpstreamStreamErrorStatus(tt.payload, tt.responseStatus))
		})
	}
}

func TestIsNullJSONStreamEvent(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{name: "literal null", data: "null", want: true},
		{name: "whitespace", data: " \r\n null \t", want: true},
		{name: "case insensitive", data: "NULL", want: true},
		{name: "empty object remains compatible", data: "{}"},
		{name: "quoted null is data", data: `"null"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNullJSONStreamEvent(tt.data))
		})
	}
}
