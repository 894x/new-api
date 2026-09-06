package hailuo

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestH3BuildRequestUsesV2MultimodalContract(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt:   "A paper boat crossing a rain puddle",
		Duration: 7,
		Metadata: map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "A paper boat crossing a rain puddle"},
				map[string]any{
					"type": "image_url", "role": "reference_image",
					"image_url": map[string]any{"url": "https://cdn.example/reference.png"},
				},
			},
			"resolution":     "2K",
			"ratio":          "16:9",
			"aigc_watermark": false,
		},
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.minimax.example"},
	}
	info.UpstreamModelName = H3Model
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.minimax.example/v2/video_generation", requestURL)

	requestBody, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.DecodeJson(requestBody, &payload))
	assert.Equal(t, H3Model, payload["model"])
	assert.Equal(t, "2K", payload["resolution"])
	assert.Equal(t, float64(7), payload["duration"])
	assert.Equal(t, "16:9", payload["ratio"])
	watermark, exists := payload["aigc_watermark"]
	assert.True(t, exists)
	assert.Equal(t, false, watermark)
	content, ok := payload["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
}

func TestH3ValidationRejectsOutOfContractDuration(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v2/video_generation",
		bytes.NewBufferString(`{"model":"MiniMax-H3","prompt":"p","duration":16}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: H3Model,
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Contains(t, taskErr.Message, "between 4 and 15")
}

func TestH3RequestDefaultsPreserveUpstreamCompatibility(t *testing.T) {
	request, err := buildH3VideoRequest(&relaycommon.TaskSubmitReq{Prompt: "a boy playing basketball"}, H3Model, false)
	require.NoError(t, err)
	payload, err := common.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"MiniMax-H3",
		"content":[{"type":"text","text":"a boy playing basketball"}],
		"resolution":"768P",
		"duration":5,
		"ratio":"16:9"
	}`, string(payload))

	request, err = buildH3VideoRequest(&relaycommon.TaskSubmitReq{
		Prompt: "animate",
		Images: []string{"first.png", "last.png"},
	}, H3Model, false)
	require.NoError(t, err)
	payload, err = common.Marshal(request)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"MiniMax-H3",
		"content":[
			{"type":"text","text":"animate"},
			{"type":"image_url","role":"first_frame","image_url":{"url":"first.png"}},
			{"type":"image_url","role":"last_frame","image_url":{"url":"last.png"}}
		],
		"resolution":"768P",
		"duration":5,
		"ratio":"adaptive"
	}`, string(payload))
}

func TestH3RatioFollowsOfficialScenarioRules(t *testing.T) {
	t.Run("frame input is always adaptive", func(t *testing.T) {
		request, err := buildH3VideoRequest(&relaycommon.TaskSubmitReq{
			Prompt: "animate",
			Images: []string{"first.png"},
			Metadata: map[string]any{
				"ratio": "16:9",
			},
		}, H3Model, false)
		require.NoError(t, err)
		assert.Equal(t, "adaptive", request.Ratio)
	})

	t.Run("audio reference defaults to adaptive", func(t *testing.T) {
		request, err := buildH3VideoRequest(&relaycommon.TaskSubmitReq{
			Prompt: "follow the voice",
			Metadata: map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "follow the voice"},
					map[string]any{
						"type": "audio_url", "role": "reference_audio",
						"audio_url": map[string]any{"url": "https://cdn.example/reference.mp3"},
					},
				},
			},
		}, H3Model, false)
		require.NoError(t, err)
		assert.Equal(t, "adaptive", request.Ratio)
	})
}

func TestH3MappedModelStillUsesV2Protocol(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("task_request", relaycommon.TaskSubmitReq{Prompt: "mapped model"})
	info := &relaycommon.RelayInfo{
		OriginModelName: H3Model,
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.minimax.example"},
	}
	info.UpstreamModelName = "vendor-h3"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.minimax.example/v2/video_generation", requestURL)

	requestBody, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.DecodeJson(requestBody, &payload))
	assert.Equal(t, "vendor-h3", payload["model"])
}

func TestH3RequestRejectsDocumentedInputBounds(t *testing.T) {
	tenReferenceImages := make([]any, 0, 10)
	for range 10 {
		tenReferenceImages = append(tenReferenceImages, map[string]any{
			"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "image.png"},
		})
	}
	tests := []struct {
		name    string
		request relaycommon.TaskSubmitReq
		want    string
	}{
		{name: "duration below minimum", request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"duration": 3}}, want: "between 4 and 15"},
		{name: "fractional duration", request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"duration": 5.5}}, want: "between 4 and 15"},
		{name: "unsupported resolution", request: relaycommon.TaskSubmitReq{Prompt: "p", Size: "1080P"}, want: "resolution must be 768P or 2K"},
		{name: "unsupported ratio", request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"ratio": "16:10"}}, want: "ratio must be one of"},
		{name: "adaptive ratio without visual input", request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"ratio": "adaptive"}}, want: "requires an image"},
		{name: "too many frame images", request: relaycommon.TaskSubmitReq{Prompt: "p", Images: []string{"a", "b", "c"}}, want: "at most 2 frame images"},
		{name: "content is not an array", request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"content": "invalid"}}, want: "metadata.content must be an array"},
		{name: "media has no text", request: relaycommon.TaskSubmitReq{Images: []string{"frame.png"}}, want: "requires a non-empty text item"},
		{
			name: "multiple text items",
			request: relaycommon.TaskSubmitReq{Metadata: map[string]any{"content": []any{
				map[string]any{"type": "text", "text": "first prompt"},
				map[string]any{"type": "text", "text": "second prompt"},
			}}},
			want: "exactly one text item",
		},
		{
			name: "frame and reference media cannot mix",
			request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"content": []any{
				map[string]any{"type": "image_url", "role": "first_frame", "image_url": map[string]any{"url": "frame.png"}},
				map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "reference.png"}},
			}}},
			want: "cannot mix frame images with reference media",
		},
		{name: "too many reference images", request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"content": tenReferenceImages}}, want: "at most 9 reference images"},
		{name: "too many reference videos", request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"reference_video": []any{"a", "b", "c", "d"}}}, want: "at most 3 reference videos"},
		{name: "too many reference audios", request: relaycommon.TaskSubmitReq{Prompt: "p", Metadata: map[string]any{"reference_audio": []any{"a", "b", "c", "d"}}}, want: "at most 3 reference audios"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildH3VideoRequest(&test.request, H3Model, false)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestH3FetchAndParseTaskResultUseV2Contract(t *testing.T) {
	type observation struct {
		path          string
		authorization string
	}
	observed := make(chan observation, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observed <- observation{path: request.URL.EscapedPath(), authorization: request.Header.Get("Authorization")}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"task":{"id":"task/1","status":"succeeded","content":{"url":"https://cdn.example/h3.mp4"}}}`)
	}))
	t.Cleanup(server.Close)

	adaptor := &TaskAdaptor{}
	response, err := adaptor.FetchTask(server.URL, "sk-test", map[string]any{
		"task_id": "task/1",
		"model":   H3Model,
	}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	request := <-observed
	assert.Equal(t, "/v2/query/video_generation/task%2F1", request.path)
	assert.Equal(t, "Bearer sk-test", request.authorization)

	result, err := adaptor.ParseTaskResult(responseBody)
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Equal(t, "https://cdn.example/h3.mp4", result.Url)
}

func TestH3TaskResultMapsFailureAndRetryableErrors(t *testing.T) {
	adaptor := &TaskAdaptor{}
	tests := []struct {
		name       string
		body       string
		wantStatus string
		wantReason string
	}{
		{name: "queued", body: `{"task":{"id":"1","status":"queued"}}`, wantStatus: string(model.TaskStatusQueued)},
		{name: "running", body: `{"task":{"id":"1","status":"running"}}`, wantStatus: string(model.TaskStatusInProgress)},
		{name: "failed", body: `{"task":{"id":"1","status":"failed","error":{"message":"sensitive content"}}}`, wantStatus: string(model.TaskStatusFailure), wantReason: "sensitive content"},
		{name: "cancelled", body: `{"task":{"id":"1","status":"cancelled"}}`, wantStatus: string(model.TaskStatusFailure), wantReason: "task cancelled"},
		{name: "permanent API error", body: `{"type":"error","error":{"type":"authorized_error","message":"login failed","http_code":"401"}}`, wantStatus: string(model.TaskStatusFailure), wantReason: "login failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := adaptor.ParseTaskResult([]byte(test.body))
			require.NoError(t, err)
			assert.Equal(t, test.wantStatus, result.Status)
			assert.Equal(t, test.wantReason, result.Reason)
		})
	}

	_, err := adaptor.ParseTaskResult([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"retry later","http_code":"429"}}`))
	require.ErrorContains(t, err, "retry later")
}

func TestH3TaskDataSanitizationReplacesUpstreamIDs(t *testing.T) {
	adaptor := &TaskAdaptor{}
	assert.JSONEq(t,
		`{"task_id":"task_public"}`,
		string(adaptor.SanitizeTaskData([]byte(`{"task_id":"task_upstream"}`), "task_public")),
	)
	assert.JSONEq(t,
		`{"task":{"id":"task_public","status":"succeeded","content":{"url":"https://cdn.example/h3.mp4"}}}`,
		string(adaptor.SanitizeTaskData(
			[]byte(`{"task":{"id":"task_upstream","status":"succeeded","content":{"url":"https://cdn.example/h3.mp4"}}}`),
			"task_public",
		)),
	)
}

func TestMiniMaxVideoV2ConversionRejectsLegacyTasks(t *testing.T) {
	adaptor := &TaskAdaptor{}
	legacyTask := &model.Task{
		TaskID: "task_legacy",
		Properties: model.Properties{
			OriginModelName:   "MiniMax-Hailuo-2.3",
			UpstreamModelName: "MiniMax-Hailuo-2.3",
		},
	}

	assert.False(t, adaptor.IsMiniMaxVideoV2Task(legacyTask))
	_, err := adaptor.ConvertToMiniMaxVideoV2(legacyTask)
	require.ErrorContains(t, err, "was not created with")
}

func TestLegacyHailuoRequestPathRemainsV1(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.minimax.example"},
	}
	info.UpstreamModelName = "MiniMax-Hailuo-2.3"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://api.minimax.example/v1/video_generation", requestURL)
}
