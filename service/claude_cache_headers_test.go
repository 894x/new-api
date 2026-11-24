package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestIOCopySetsClaudeCacheHeadersFromFinalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		path         string
		body         string
		wantRead     string
		wantCreation string
		wantHeaders  bool
	}{
		{
			name:         "messages usage",
			path:         "/v1/messages",
			body:         `{"id":"msg_1","usage":{"input_tokens":1,"cache_read_input_tokens":9,"cache_creation_input_tokens":4}}`,
			wantRead:     "9",
			wantCreation: "4",
			wantHeaders:  true,
		},
		{
			name:         "missing cache fields are explicit zeros",
			path:         "/v1/messages",
			body:         `{"usage":{"input_tokens":2,"output_tokens":1}}`,
			wantRead:     "0",
			wantCreation: "0",
			wantHeaders:  true,
		},
		{
			name:        "error without usage",
			path:        "/v1/messages",
			body:        `{"type":"error","error":{"type":"api_error","message":"nope"}}`,
			wantHeaders: false,
		},
		{
			name:        "other protocol",
			path:        "/v1/chat/completions",
			body:        `{"usage":{"prompt_tokens":3,"cache_read_input_tokens":9,"cache_creation_input_tokens":4}}`,
			wantHeaders: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)

			IOCopyBytesGracefully(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, []byte(tt.body))

			if tt.wantHeaders {
				assert.Equal(t, tt.wantRead, recorder.Header().Get("x-cache-read-input-tokens"))
				assert.Equal(t, tt.wantCreation, recorder.Header().Get("x-cache-creation-input-tokens"))
				return
			}
			assert.Empty(t, recorder.Header().Get("x-cache-read-input-tokens"))
			assert.Empty(t, recorder.Header().Get("x-cache-creation-input-tokens"))
		})
	}
}
