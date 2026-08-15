package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func videoResolutionTestContext(t *testing.T, path string, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func TestGetModelRequestExtractsCanonicalVideoResolution(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		body   string
		doubao bool
		want   string
	}{
		{
			name: "unified video metadata resolution",
			path: "/v1/video/generations",
			body: `{"model":"video-model","metadata":{"resolution":"1920x1080"}}`,
			want: "1080p",
		},
		{
			name: "openai video size",
			path: "/v1/videos",
			body: `{"model":"video-model","size":"1280x720"}`,
			want: "720p",
		},
		{
			name:   "provider native video resolution",
			path:   "/api/v3/contents/generations/tasks",
			body:   `{"model":"video-model","metadata":{"resolution":"1080P"}}`,
			doubao: true,
			want:   "1080p",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := videoResolutionTestContext(t, test.path, test.body)
			if test.doubao {
				common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
			}

			request, shouldSelect, err := getModelRequest(ctx)
			require.NoError(t, err)
			assert.True(t, shouldSelect)
			assert.Equal(t, "video-model", request.Model)
			assert.Equal(t, test.want, request.VideoResolution)
		})
	}
}

func TestGetModelRequestDoesNotApplyResolutionToNonVideoRequests(t *testing.T) {
	ctx := videoResolutionTestContext(t, "/v1/chat/completions", `{"model":"chat-model","size":"1920x1080"}`)

	request, shouldSelect, err := getModelRequest(ctx)
	require.NoError(t, err)
	assert.True(t, shouldSelect)
	assert.Empty(t, request.VideoResolution)
}

func TestGetModelRequestRejectsInvalidVideoResolution(t *testing.T) {
	ctx := videoResolutionTestContext(t, "/v1/video/generations", `{"model":"video-model","size":"high"}`)

	_, _, err := getModelRequest(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid video resolution")
}

func TestGetModelRequestExtractsResolutionFromMultipartVideoRequest(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "video-model"))
	require.NoError(t, writer.WriteField("size", "720x1280"))
	require.NoError(t, writer.Close())

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	request, shouldSelect, err := getModelRequest(ctx)
	require.NoError(t, err)
	assert.True(t, shouldSelect)
	assert.Equal(t, "video-model", request.Model)
	assert.Equal(t, "720p", request.VideoResolution)
}
