package ali

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestConvertToAliRequestWan27I2VBuildsMediaFromImage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:    "wan2.7-i2v",
		Prompt:   "animate the first frame",
		Image:    "https://example.com/first.png",
		Size:     "720p",
		Duration: 10,
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, "wan2.7-i2v", aliReq.Model)
	require.NotNil(t, aliReq.Parameters.Resolution)
	require.Equal(t, "720P", *aliReq.Parameters.Resolution)
	require.NotNil(t, aliReq.Parameters.Duration)
	require.Equal(t, 10, *aliReq.Parameters.Duration)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
	}, aliReq.Input.Media)
	require.Empty(t, aliReq.Input.ImgURL)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"media"`)
	require.NotContains(t, string(body), `"img_url"`)
}

func TestConvertToAliRequestWan27I2VBuildsFirstAndLastFrameFromImages(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "interpolate between frames",
		Images: []string{
			"https://example.com/first.png",
			"https://example.com/last.png",
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VPrefersImageBeforeImagesAndInputReference(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "use the direct image",
		Image:          " https://example.com/direct.png ",
		Images:         []string{"https://example.com/images-first.png", " https://example.com/images-last.png "},
		InputReference: "https://example.com/input-reference.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/direct.png"},
		{Type: "last_frame", URL: "https://example.com/images-last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VFallsBackToFirstNonEmptyImage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "skip blank images",
		Image:  " ",
		Images: []string{
			" ",
			" https://example.com/first.png ",
			" https://example.com/last.png ",
		},
		InputReference: "https://example.com/input-reference.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VKeepsExplicitMetadataMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "continue the clip",
		Image:          "https://example.com/direct.png",
		Images:         []string{"https://example.com/images-first.png", "https://example.com/images-last.png"},
		InputReference: "https://example.com/input-reference.png",
		Metadata: map[string]interface{}{
			"input": map[string]interface{}{
				"media": []interface{}{
					map[string]interface{}{
						"type": "first_clip",
						"url":  "https://example.com/input.mp4",
					},
				},
			},
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_clip", URL: "https://example.com/input.mp4"},
	}, aliReq.Input.Media)
	require.Empty(t, aliReq.Input.ImgURL)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"media"`)
	require.NotContains(t, string(body), `"img_url"`)
}

func TestConvertToAliRequestWan27I2VRequiresMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "animate without a frame",
	}

	_, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "requires image"))
}

func TestConvertToAliRequestWan25I2VKeepsLegacyImgURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.5-i2v-preview",
		Prompt: "animate the first frame",
		Image:  "https://example.com/first.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, "https://example.com/first.png", aliReq.Input.ImgURL)
	require.Empty(t, aliReq.Input.Media)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"img_url"`)
	require.NotContains(t, string(body), `"media"`)
}

func TestWan3ModelsAreBuiltIn(t *testing.T) {
	require.Contains(t, ModelList, "wan3.0-video")
	require.Contains(t, ModelList, "wan3.0-video-prime")
	defaults := ratio_setting.GetDefaultModelRatioMap()
	assert.InDelta(t, 2*0.3/ratio_setting.USD2RMB, defaults["wan3.0-video"], 1e-12)
	assert.InDelta(t, 2*0.45/ratio_setting.USD2RMB, defaults["wan3.0-video-prime"], 1e-12)
}

func TestValidateAliNativeWan3AcceptsMediaOnlyAndSmartDuration(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Metadata: map[string]any{
		"model": "wan3.0-video",
		"input": map[string]any{
			"media": []any{map[string]any{"type": "reference_video", "url": "https://example.com/input.mp4"}},
		},
		"parameters": map[string]any{
			"resolution":    "1080P",
			"ratio":         "adaptive",
			"duration":      -1,
			"audio":         false,
			"seed":          0,
			"prompt_extend": false,
			"watermark":     false,
		},
	}}

	nativeReq, err := validateAliNativeRequest(req, "", false)

	require.NoError(t, err)
	require.Len(t, nativeReq.Input.Media, 1)
	require.NotNil(t, nativeReq.Parameters.Duration)
	assert.Equal(t, -1, *nativeReq.Parameters.Duration)
	require.NotNil(t, nativeReq.Parameters.Audio)
	assert.False(t, *nativeReq.Parameters.Audio)
	require.NotNil(t, nativeReq.Parameters.Seed)
	assert.Zero(t, *nativeReq.Parameters.Seed)
	require.NotNil(t, nativeReq.Parameters.PromptExtend)
	assert.False(t, *nativeReq.Parameters.PromptExtend)
	require.NotNil(t, nativeReq.Parameters.Watermark)
	assert.False(t, *nativeReq.Parameters.Watermark)
}

func TestValidateAliNativeWan3RejectsUnsafeDuration(t *testing.T) {
	for _, duration := range []int{-2, 0, 1, 31, relaycommon.MaxTaskDurationSeconds} {
		t.Run(strconv.Itoa(duration), func(t *testing.T) {
			_, err := validateAliNativeRequest(relaycommon.TaskSubmitReq{Metadata: map[string]any{
				"model":      "wan3.0-video",
				"input":      map[string]any{"prompt": "animate"},
				"parameters": map[string]any{"duration": duration},
			}}, "", false)
			require.Error(t, err)
		})
	}
}

func TestBuildAliNativeRequestPreservesOfficialFieldsAndMapsModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatAliVideo)
	ctx.Set("task_request", relaycommon.TaskSubmitReq{Metadata: map[string]any{
		"model": "wan3-alias",
		"input": map[string]any{
			"prompt": "animate",
			"media":  []any{map[string]any{"type": "reference_image", "url": "https://example.com/ref.png"}},
		},
		"parameters": map[string]any{
			"resolution":    "480P",
			"ratio":         "1:1",
			"duration":      2,
			"audio":         false,
			"seed":          0,
			"prompt_extend": false,
			"watermark":     false,
		},
	}})
	info := testRelayInfo()
	info.UpstreamModelName = "wan3.0-video"

	body, err := (&TaskAdaptor{}).BuildRequestBody(ctx, info)

	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(encoded, &payload))
	assert.Equal(t, "wan3.0-video", payload["model"])
	parameters := payload["parameters"].(map[string]any)
	assert.Equal(t, false, parameters["audio"])
	assert.Equal(t, float64(0), parameters["seed"])
	assert.Equal(t, false, parameters["prompt_extend"])
	assert.Equal(t, false, parameters["watermark"])
}

func TestEstimateBillingWan3SmartDurationReservesMaximum(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatAliVideo)
	ctx.Set("task_request", relaycommon.TaskSubmitReq{Metadata: map[string]any{
		"model":      "wan3.0-video",
		"input":      map[string]any{"prompt": "animate"},
		"parameters": map[string]any{"resolution": "1080P", "duration": -1},
	}})
	info := testRelayInfo()
	info.UpstreamModelName = "wan3.0-video"

	ratios := (&TaskAdaptor{}).EstimateBilling(ctx, info)

	assert.Equal(t, 30.0, ratios["seconds"])
	assert.Equal(t, 4.0, ratios["resolution-1080P"])
}

func TestEstimateBillingWan3OfficialDefaultsUse1080P(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatAliVideo)
	ctx.Set("task_request", relaycommon.TaskSubmitReq{Metadata: map[string]any{
		"model": "wan3.0-video",
		"input": map[string]any{"prompt": "animate"},
	}})
	info := testRelayInfo()
	info.UpstreamModelName = "wan3.0-video"

	ratios := (&TaskAdaptor{}).EstimateBilling(ctx, info)

	assert.Equal(t, 5.0, ratios["seconds"])
	assert.Equal(t, 4.0, ratios["resolution-1080P"])
}

func TestValidateAliNativeWan3UsesMappedUpstreamModel(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Metadata: map[string]any{
		"model":      "wan3-alias",
		"input":      map[string]any{"prompt": "animate"},
		"parameters": map[string]any{"duration": -1},
	}}

	_, err := validateAliNativeRequest(req, "", true)
	require.NoError(t, err)
	_, err = validateAliNativeRequest(req, "wan3.0-video", false)
	require.NoError(t, err)
	_, err = validateAliNativeRequest(req, "wan2.7-i2v", false)
	require.Error(t, err)
}

func TestDoResponseAliNativeReturnsPublicTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	common.SetContextKey(ctx, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatAliVideo)
	upstreamBody := `{"output":{"task_status":"PENDING","task_id":"upstream-id"},"request_id":"request-id"}`
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(upstreamBody))}
	info := testRelayInfo()
	info.PublicTaskID = "task_public"

	upstreamID, storedData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, response, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-id", upstreamID)
	assert.JSONEq(t, upstreamBody, string(storedData))
	assert.JSONEq(t, `{"output":{"task_status":"PENDING","task_id":"task_public"},"request_id":"request-id"}`, recorder.Body.String())
}

func TestConvertToAliNativeVideoPreservesUsageAndHidesUpstreamTaskID(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		Data: []byte(`{
			"request_id":"request-id",
			"output":{"task_id":"upstream-id","task_status":"SUCCEEDED","video_url":"https://example.com/video.mp4"},
			"usage":{"duration":5.0,"output_video_duration":5.0,"SR":720,"ratio":"16:9"}
		}`),
	}

	encoded, err := (&TaskAdaptor{}).ConvertToAliNativeVideo(task)

	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(encoded, &payload))
	output := payload["output"].(map[string]any)
	assert.Equal(t, "task_public", output["task_id"])
	assert.Equal(t, "https://example.com/video.mp4", output["video_url"])
	usage := payload["usage"].(map[string]any)
	assert.Equal(t, float64(5), usage["output_video_duration"])
	assert.Equal(t, "16:9", usage["ratio"])
}

func TestConvertToAliNativeVideoFillsEmptyTaskStatus(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusInProgress,
		Data:   []byte(`{"output":{"task_id":"upstream-id","task_status":""}}`),
	}

	encoded, err := (&TaskAdaptor{}).ConvertToAliNativeVideo(task)

	require.NoError(t, err)
	assert.JSONEq(t, `{"output":{"task_id":"task_public","task_status":"RUNNING"}}`, string(encoded))
}

func TestAdjustBillingOnCompleteWan3UsesActualOutputDuration(t *testing.T) {
	modelRatio := 2 * 0.3 / ratio_setting.USD2RMB
	task := &model.Task{
		Status:     model.TaskStatusSuccess,
		Properties: model.Properties{OriginModelName: "wan3.0-video"},
		Data:       []byte(`{"usage":{"duration":5,"output_video_duration":5.0}}`),
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			ModelPrice:  -1,
			ModelRatio:  modelRatio,
			GroupRatio:  1,
			OtherRatios: map[string]float64{"seconds": 30, "resolution-1080P": 4},
		}},
	}
	taskResult := &relaycommon.TaskInfo{}

	actual := (&TaskAdaptor{}).AdjustBillingOnComplete(task, taskResult)

	assert.Zero(t, actual)
	assert.Equal(t, common.QuotaRound(5*common.QuotaPerUnit/2), taskResult.TotalTokens)
	assert.Equal(t, 1.0, task.PrivateData.BillingContext.OtherRatios["seconds"])
	assert.Equal(t, 4.0, task.PrivateData.BillingContext.OtherRatios["resolution-1080P"])
}

func TestConvertToAliRequestWan3BuildsOfficialMediaProtocol(t *testing.T) {
	aliReq, err := (&TaskAdaptor{}).convertToAliRequest(testRelayInfo(), relaycommon.TaskSubmitReq{
		Model:    "wan3.0-video-prime",
		Prompt:   "animate",
		Images:   []string{"https://example.com/first.png", "https://example.com/last.png"},
		Size:     "720p",
		Duration: 10,
		Metadata: map[string]any{"parameters": map[string]any{"audio": false, "seed": 0, "prompt_extend": false}},
	})

	require.NoError(t, err)
	assert.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
	assert.Empty(t, aliReq.Input.ImgURL)
	require.NotNil(t, aliReq.Parameters.Audio)
	assert.False(t, *aliReq.Parameters.Audio)
	require.NotNil(t, aliReq.Parameters.Seed)
	assert.Zero(t, *aliReq.Parameters.Seed)
	require.NotNil(t, aliReq.Parameters.PromptExtend)
	assert.False(t, *aliReq.Parameters.PromptExtend)
	assert.Equal(t, "720P", lo.FromPtr(aliReq.Parameters.Resolution))
}
