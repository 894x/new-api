package plugins_test

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceSLSPluginMappedPoliciesFreezeBillingAndWirePayload(t *testing.T) {
	plugin := seedanceSLSPlugin(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"doubao-seedance-2-5-260628","content":[{"type":"text","text":"a fox"}],"duration":-1,"resolution":"720p","seed":0,"generate_audio":false,"future":{"enabled":false}}`))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-5-260628", TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://sls.example", UpstreamModelName: "doubao-seedance-2-5-260628",
			ParamOverride: map[string]any{"resolution": "1080p", "duration": 5},
		},
	}
	adaptor := taskplugin.New(plugin)
	adaptor.Init(info)
	assert.True(t, adaptor.SupportsNativeTaskFormat(constant.TaskResponseFormatDoubaoVideo))
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Nil(t, adaptor.ValidateMappedRequest(c, info))
	ratios, err := adaptor.EstimateBillingValidated(c, info)
	require.NoError(t, err)
	assert.InDelta(t, 11.7/10.7, ratios["video_input"], 1e-12)
	facts, err := adaptor.ExtractUsageFactsValidated(c, info)
	require.NoError(t, err)
	assert.Equal(t, float64(243000), facts["tokens"])
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.DecodeJson(body, &payload))
	assert.Equal(t, "1080p", payload["resolution"])
	assert.Equal(t, float64(5), payload["duration"])
	assert.Equal(t, float64(0), payload["seed"])
	assert.Equal(t, false, payload["generate_audio"])
	assert.Equal(t, map[string]any{"enabled": false}, payload["future"])

	// A retry with another channel must start from the original request, not
	// the first channel's resolution/duration overrides or prepared descriptor.
	info.ParamOverride = nil
	require.Nil(t, adaptor.ValidateMappedRequest(c, info))
	ratios, err = adaptor.EstimateBillingValidated(c, info)
	require.NoError(t, err)
	assert.Empty(t, ratios)
	body, err = adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	payload = nil
	require.NoError(t, common.DecodeJson(body, &payload))
	assert.Equal(t, "720p", payload["resolution"])
	assert.Equal(t, float64(-1), payload["duration"])
}

func TestSeedanceSLSPluginRejectsInvalidFinalPolicies(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override map[string]any
		message  string
	}{
		{"oversized duration", map[string]any{"duration": 3601}, "duration"},
		{"negative duration", map[string]any{"duration": -2}, "duration"},
		{"model override", map[string]any{"model": "other-model"}, "model mapping"},
		{"missing text", map[string]any{"content": []any{}}, "non-empty text"},
		{"boolean frames", map[string]any{"frames": true}, "frames"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"doubao-seedance-2-5-260628","prompt":"a fox","seconds":5}`))
			c.Request.Header.Set("Content-Type", gin.MIMEJSON)
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-5-260628", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: "https://sls.example", UpstreamModelName: "doubao-seedance-2-5-260628", ParamOverride: tc.override,
			}}
			adaptor := taskplugin.New(seedanceSLSPlugin(t))
			adaptor.Init(info)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			taskErr := adaptor.ValidateMappedRequest(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.True(t, taskErr.LocalError)
			assert.Contains(t, taskErr.Message, tc.message)
		})
	}
}

func TestSeedanceSLSPluginMediaValidationUsesFinalMappedPayload(t *testing.T) {
	for _, removeInvalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject final invalid audio", true: "override removes invalid audio"}[removeInvalid], func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"public-alias","content":[{"type":"text","text":"a fox"},{"type":"audio_url","audio_url":{"url":"data:audio/wav;base64,YQ=="}}]}`))
			c.Request.Header.Set("Content-Type", gin.MIMEJSON)
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			info := &relaycommon.RelayInfo{UserId: 7, OriginModelName: "public-alias", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: "https://sls.example", UpstreamModelName: "doubao-seedance-2-5-260628",
			}}
			if removeInvalid {
				info.ParamOverride = map[string]any{"content": []any{map[string]any{"type": "text", "text": "a fox"}}}
			}
			adaptor := taskplugin.New(seedanceSLSPlugin(t))
			adaptor.Init(info)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			taskErr := adaptor.ValidateMappedRequest(c, info)
			if removeInvalid {
				require.Nil(t, taskErr)
			} else {
				require.NotNil(t, taskErr)
				assert.Contains(t, taskErr.Message, "content[1].audio_url")
			}
		})
	}
}

func TestSeedanceSLSPluginPollingPreservesNestedUsageAndFailures(t *testing.T) {
	adaptor := taskplugin.New(seedanceSLSPlugin(t))
	for _, tc := range []struct {
		name, body, status, progress, reason, url string
		tokens                                    int
	}{
		{"nested success", `{"code":"success","data":{"task_id":"gateway","status":"SUCCESS","data":{"code":"success","data":{"task_id":"provider","status":"SUCCESS","result_url":"https://cdn.example/video.mp4","total_tokens":87300}}}}`, "SUCCESS", "100%", "", "https://cdn.example/video.mp4", 87300},
		{"numeric progress", `{"task_id":"provider","status":"RUNNING","progress":50}`, "IN_PROGRESS", "50%", "", "", 0},
		{"numeric zero", `{"task_id":"provider","status":"RUNNING","progress":0}`, "IN_PROGRESS", "0%", "", "", 0},
		{"submitted default", `{"status":"SUBMITTED"}`, "SUBMITTED", "10%", "", "", 0},
		{"queued default", `{"status":"QUEUED","progress":null}`, "QUEUED", "20%", "", "", 0},
		{"running default", `{"status":"RUNNING"}`, "IN_PROGRESS", "30%", "", "", 0},
		{"failure detail", `{"status":"FAILED","fail_reason":"generation failed","result_url":"provider safety detail"}`, "FAILURE", "100%", "generation failed\nprovider safety detail", "", 0},
		{"duplicate detail", `{"status":"FAILED","fail_reason":"generation failed","result_url":"generation failed"}`, "FAILURE", "100%", "generation failed", "", 0},
		{"real URL", `{"status":"FAILED","fail_reason":"generation failed","result_url":"https://cdn.example/video.mp4"}`, "FAILURE", "100%", "generation failed", "https://cdn.example/video.mp4", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := adaptor.ParseTaskResult(&model.Task{}, &http.Response{StatusCode: http.StatusOK}, []byte(tc.body))
			require.NoError(t, err)
			assert.Equal(t, tc.status, result.Status)
			assert.Equal(t, tc.progress, result.Progress)
			assert.Equal(t, tc.reason, result.Reason)
			assert.Equal(t, tc.url, result.Url)
			assert.Equal(t, tc.tokens, result.TotalTokens)
		})
	}
	_, err := adaptor.ParseTaskResult(&model.Task{}, &http.Response{StatusCode: http.StatusOK}, []byte(`{"code":"failure","message":"try again"}`))
	require.Error(t, err)
	_, err = adaptor.ParseTaskResult(&model.Task{}, &http.Response{StatusCode: http.StatusOK}, []byte(`{"status":"RUNNING","progress":true}`))
	require.Error(t, err)
}

func TestSeedanceSLSPluginTransformsMediaBeforeValidation(t *testing.T) {
	previousLimit := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 64
	fetch := system_setting.GetFetchSetting()
	previousFetch := *fetch
	fetch.EnableSSRFProtection = false
	service.InitHttpClient()
	t.Cleanup(func() { constant.MaxFileDownloadMB = previousLimit; *fetch = previousFetch; service.InitHttpClient() })
	var content bytes.Buffer
	require.NoError(t, png.Encode(&content, image.NewRGBA(image.Rect(0, 0, 400, 400))))
	downloads := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads <- struct{}{}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(content.Bytes())
	}))
	t.Cleanup(server.Close)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(fmt.Sprintf(`{"model":"public-alias","content":[{"type":"text","text":"a fox"},{"type":"image_url","image_url":{"url":%q},"role":"first_frame"}]}`, server.URL)))
	c.Request.Header.Set("Content-Type", gin.MIMEJSON)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{OriginModelName: "public-alias", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl: "https://sls.example", UpstreamModelName: "doubao-seedance-2-0-260128",
		ChannelOtherSettings: kitdto.ChannelOtherSettings{ParameterCapabilities: &kitdto.ParameterCapabilityConfig{
			Defaults: map[string]kitdto.ParameterCapability{"content.*.image_url": {Transform: "image_url_to_base64"}},
		}},
	}}
	adaptor := taskplugin.New(seedanceSLSPlugin(t))
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Nil(t, adaptor.ValidateMappedRequest(c, info))
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.DecodeJson(body, &payload))
	media := payload["content"].([]any)[1].(map[string]any)
	assert.Equal(t, "first_frame", media["role"])
	assert.Contains(t, media["image_url"].(map[string]any)["url"], "data:image/png;base64,")
	assert.Len(t, downloads, 1)
	require.Len(t, info.ParameterCapabilityAudit, 1)
	assert.Equal(t, "image_url_to_base64", info.ParameterCapabilityAudit[0].Action)
}

func TestSeedanceSLSPluginArtifactFallbackAndLastFrame(t *testing.T) {
	adaptor := taskplugin.New(seedanceSLSPlugin(t))
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://sls.example", ApiKey: "private-key"}})
	for _, tc := range []struct {
		name, data, key, wantURL string
		wantArtifacts            int
	}{
		{"legacy stored URL", "", "video", "https://legacy.example/video.mp4", 1},
		{"provider URL wins", `{"result_url":"https://cdn.example/video.mp4"}`, "video", "https://cdn.example/video.mp4", 1},
		{"last frame", `{"code":"success","data":{"result_url":"https://cdn.example/video.mp4","last_frame_url":"https://cdn.example/frame.png"}}`, "last_frame", "https://cdn.example/frame.png", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, Data: []byte(tc.data),
				PrivateData: model.TaskPrivateData{ResultURL: "https://legacy.example/video.mp4"}}
			artifacts, err := adaptor.ListArtifacts(task)
			require.NoError(t, err)
			require.Len(t, artifacts, tc.wantArtifacts)
			descriptor, err := adaptor.BuildContentRequest(task, tc.key, channel.TaskArtifactClientRequest{Method: http.MethodHead})
			require.NoError(t, err)
			assert.Equal(t, tc.wantURL, descriptor.URL)
			assert.Equal(t, http.MethodHead, descriptor.Method)
			assert.True(t, descriptor.Credentialless)
			assert.Empty(t, descriptor.Headers)
			assert.Empty(t, descriptor.Body)
			task.Status = model.TaskStatusFailure
			artifacts, err = adaptor.ListArtifacts(task)
			require.NoError(t, err)
			assert.Empty(t, artifacts)
		})
	}
}

func TestSeedanceSLSPluginMappedCapabilitiesPreserveConfiguredRules(t *testing.T) {
	for _, tc := range []struct {
		name, duration string
		override       map[string]any
		allowed        []string
		want           float64
		wantError      bool
	}{
		{"automatic", "-1", nil, []string{"-1", "4", "15"}, -1, false},
		{"automatic disabled", "-1", nil, []string{"4", "15"}, 0, true},
		{"duration rejected", "16", nil, []string{"4", "15"}, 0, true},
		{"override before validation", "16", map[string]any{"duration": 4}, []string{"4"}, 4, false},
		{"override cannot bypass validation", "4", map[string]any{"duration": 16}, []string{"4"}, 0, true},
		{"typed value validation", `"4"`, nil, []string{"4"}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"public-model","content":[{"type":"text","text":"A cat"}],"duration":`+tc.duration+`,"seed":0,"generate_audio":false}`))
			c.Request.Header.Set("Content-Type", gin.MIMEJSON)
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			minimum, maximum := -1.0, 15.0
			info := &relaycommon.RelayInfo{OriginModelName: "public-model", TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: "https://sls.example", UpstreamModelName: "upstream-model", ParamOverride: tc.override,
				ChannelOtherSettings: kitdto.ChannelOtherSettings{ParameterCapabilities: &kitdto.ParameterCapabilityConfig{Rules: []kitdto.ModelParameterCapabilityRule{{
					Selector:   kitdto.ParameterCapabilitySelector{Type: "exact", Value: "upstream-model"},
					Parameters: map[string]kitdto.ParameterCapability{"duration": {Min: &minimum, Max: &maximum, AllowedValues: tc.allowed}},
				}}}},
			}}
			adaptor := taskplugin.New(seedanceSLSPlugin(t))
			adaptor.Init(info)
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			taskErr := adaptor.ValidateMappedRequest(c, info)
			if tc.wantError {
				require.NotNil(t, taskErr)
				assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
				assert.True(t, taskErr.LocalError)
				assert.Contains(t, taskErr.Message, "duration")
				return
			}
			require.Nil(t, taskErr)
			reader, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, common.DecodeJson(reader, &body))
			assert.Equal(t, tc.want, body["duration"])
			assert.Equal(t, "upstream-model", body["model"])
			assert.Equal(t, float64(0), body["seed"])
			assert.Equal(t, false, body["generate_audio"])
		})
	}
}

func TestSeedanceSLSPluginRetainsPublicModelsAndDefaultPrices(t *testing.T) {
	assert.Equal(t, []string{
		"doubao-seedance-1-0-pro-250528", "doubao-seedance-1-0-lite-t2v", "doubao-seedance-1-0-lite-i2v", "doubao-seedance-1-5-pro-251215",
		"doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615", "doubao-seedance-2-5-260628",
	}, taskplugin.New(seedanceSLSPlugin(t)).GetModelList())
	defaults := ratio_setting.GetDefaultModelRatioMap()
	for alias, versioned := range map[string]string{
		"doubao-seedance-2-0": "doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast": "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini": "doubao-seedance-2-0-mini-260615",
	} {
		assert.Positive(t, defaults[alias], alias)
		assert.Equal(t, defaults[versioned], defaults[alias], alias)
	}
	assert.InDelta(t, 4.794520547945205, defaults["doubao-seedance-2-5-260628"], 1e-12)
}
