package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoubaoVideoRequestConvertPreservesNativeBillingInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Body = http.NoBody

	body := []byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"video_url","video_url":{"url":"https://example.com/input.mp4"}},
			{"type":"text","text":"Generate a tracking shot"}
		],
		"resolution":"1080p",
		"duration":5,
		"generate_audio":false
	}`)
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	ctx.Request.ContentLength = int64(len(body))

	handler := DoubaoVideoRequestConvert()
	handler(ctx)

	var request relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(ctx, &request))

	assert.Equal(t, constant.TaskResponseFormatDoubaoVideo, common.GetContextKeyString(ctx, constant.ContextKeyTaskResponseFormat))
	assert.Equal(t, "doubao-seedance-2-0-260128", request.Model)
	assert.Equal(t, "Generate a tracking shot", request.Prompt)
	assert.Equal(t, 5, request.Duration)
	assert.Equal(t, "1080p", request.Metadata["resolution"])
	assert.Equal(t, false, request.Metadata["generate_audio"])
	content, ok := request.Metadata["content"].([]any)
	require.True(t, ok)
	assert.Len(t, content, 2)
}
