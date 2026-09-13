package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayRequestValidationHTTPStatus(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		format      types.RelayFormat
		body        string
		bodyLimit   int64
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "empty chat messages",
			path:        "/v1/chat/completions",
			format:      types.RelayFormatOpenAI,
			body:        `{"model":"kimi-k3","messages":[]}`,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "field messages is required",
		},
		{
			name:        "missing chat model",
			path:        "/v1/chat/completions",
			format:      types.RelayFormatOpenAI,
			body:        `{"messages":[{"role":"user","content":"hi"}]}`,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "model is required",
		},
		{
			name:       "malformed JSON",
			path:       "/v1/chat/completions",
			format:     types.RelayFormatOpenAI,
			body:       `{"model":"kimi-k3","messages":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "empty Claude messages",
			path:        "/v1/messages",
			format:      types.RelayFormatClaude,
			body:        `{"model":"kimi-k3","messages":[],"max_tokens":1}`,
			wantStatus:  http.StatusBadRequest,
			wantMessage: "field messages is required",
		},
		{
			name:        "oversized body remains 413",
			path:        "/v1/chat/completions",
			format:      types.RelayFormatOpenAI,
			body:        `{"model":"kimi-k3","messages":[]}`,
			bodyLimit:   1,
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantMessage: "request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.POST(tt.path, func(c *gin.Context) {
				defer common.CleanupBodyStorage(c)
				c.Set("role", common.RoleAdminUser)
				if tt.bodyLimit > 0 {
					c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, tt.bodyLimit)
				}
				Relay(c, tt.format)
			})

			request := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			assert.Equal(t, tt.wantStatus, recorder.Code, recorder.Body.String())
			var response struct {
				Type  string `json:"type"`
				Error struct {
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.NotEmpty(t, response.Error.Message)
			if tt.wantMessage != "" {
				assert.Contains(t, response.Error.Message, tt.wantMessage)
			}
			if tt.format == types.RelayFormatClaude {
				assert.Equal(t, "error", response.Type)
			} else if tt.wantStatus == http.StatusBadRequest {
				assert.Equal(t, string(types.ErrorCodeInvalidRequest), response.Error.Code)
			}
		})
	}
}
