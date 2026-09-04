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

func TestAliVideoRequestConvertPreservesNativeWan3Request(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := []byte(`{
		"model":"wan3.0-video",
		"input":{
			"prompt":"Generate a tracking shot",
			"media":[{"type":"reference_video","url":"https://example.com/input.mp4"}]
		},
		"parameters":{
			"resolution":"1080P",
			"ratio":"adaptive",
			"duration":-1,
			"audio":false,
			"seed":0,
			"prompt_extend":false,
			"watermark":false
		}
	}`)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", io.NopCloser(bytes.NewReader(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AliVideoRequestConvert()(ctx)

	var request relaycommon.TaskSubmitReq
	require.NoError(t, common.UnmarshalBodyReusable(ctx, &request))
	assert.Equal(t, constant.TaskResponseFormatAliVideo, common.GetContextKeyString(ctx, constant.ContextKeyTaskResponseFormat))
	assert.Equal(t, "wan3.0-video", request.Model)
	assert.Equal(t, "Generate a tracking shot", request.Prompt)
	assert.Equal(t, "1080P", request.Size)
	assert.Equal(t, -1, request.Duration)

	parameters, ok := request.Metadata["parameters"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, parameters["audio"])
	assert.Equal(t, float64(0), parameters["seed"])
	assert.Equal(t, false, parameters["prompt_extend"])
	assert.Equal(t, false, parameters["watermark"])
}

func TestAliVideoRequestConvertLeavesFetchBodyUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task_public", nil)

	AliVideoRequestConvert()(ctx)

	assert.Equal(t, constant.TaskResponseFormatAliVideo, common.GetContextKeyString(ctx, constant.ContextKeyTaskResponseFormat))
	assert.Equal(t, http.NoBody, ctx.Request.Body)
}
