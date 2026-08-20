package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
}
