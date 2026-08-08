package tencent_tokenhub

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestBodyPassesThroughTokenHubFields(t *testing.T) {
	data := validateAndBuildRequest(t, `{
		"model":"video-alias",
		"template":"hug",
		"images":[{"url":"https://example.com/portrait.png"}],
		"bgm":true,
		"future_provider_field":{"mode":"fast"}
	}`, ModelYTVideoFX)

	assert.JSONEq(t, `{
		"model":"yt-video-fx",
		"template":"hug",
		"images":[{"url":"https://example.com/portrait.png"}],
		"bgm":true,
		"future_provider_field":{"mode":"fast"}
	}`, string(data))
}

func TestValidateRequestRejectsUnboundedMetadataDuration(t *testing.T) {
	adaptor := &TaskAdaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"hy-video-1.5",
		"prompt":"a paper boat moving downstream",
		"metadata":{"duration":3601}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	taskErr := adaptor.ValidateRequestAndSetAction(c, tokenHubVideoRelayInfo("hy-video-1.5"))

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "invalid_duration", taskErr.Code)
}

func TestValidateRequestRejectsInvalidDirectDuration(t *testing.T) {
	tests := []string{
		`18446744073709551615`,
		`"18446744073709551615"`,
		`1.5`,
	}
	for _, duration := range tests {
		t.Run(duration, func(t *testing.T) {
			adaptor := &TaskAdaptor{}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
				"model":"hy-video-1.5",
				"prompt":"a paper boat moving downstream",
				"duration":`+duration+`
			}`))
			c.Request.Header.Set("Content-Type", "application/json")

			taskErr := adaptor.ValidateRequestAndSetAction(c, tokenHubVideoRelayInfo(ModelHYVideo15))

			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, "invalid_duration", taskErr.Code)
		})
	}
}

func TestValidateRequestAcceptsProviderSpecificVideoContracts(t *testing.T) {
	tests := []struct {
		name  string
		model string
		body  string
	}{
		{
			name:  "video fx metadata images",
			model: ModelYTVideoFX,
			body:  `{"model":"yt-video-fx","template":"hug","images":[{"url":"https://example.com/portrait.png"}]}`,
		},
		{
			name:  "human actor metadata image",
			model: ModelYTVideoHumanActor,
			body:  `{"model":"yt-video-humanactor","prompt":"the person speaks","audio_url":"https://example.com/voice.mp3","image_url":"https://example.com/portrait.png","metadata":{"billing_duration_seconds":12}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adaptor := &TaskAdaptor{}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(test.body))
			c.Request.Header.Set("Content-Type", "application/json")
			info := tokenHubVideoRelayInfo(test.model)

			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			require.Nil(t, taskErr)
			_, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
		})
	}
}

func TestValidateRequestDefersModelSpecificValidationToTokenHub(t *testing.T) {
	adaptor := &TaskAdaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"yt-video-fx",
		"future_provider_field":true
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	taskErr := adaptor.ValidateRequestAndSetAction(c, tokenHubVideoRelayInfo(ModelYTVideoFX))

	require.Nil(t, taskErr)
}

func TestEstimateBillingUsesDocumentedVideoVariants(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		body     string
		expected map[string]float64
	}{
		{
			name:     "HY is fixed per call",
			model:    ModelHYVideo15,
			body:     `{"model":"hy-video-1.5"}`,
			expected: nil,
		},
		{
			name:     "YT 2 uses 720p per call tier",
			model:    ModelYTVideo20,
			body:     `{"model":"yt-video-2.0","resolution":"720p"}`,
			expected: map[string]float64{"resolution": 2.5},
		},
		{
			name:     "human actor uses seconds and 1080p",
			model:    ModelYTVideoHumanActor,
			body:     `{"model":"yt-video-humanactor","metadata":{"billing_duration_seconds":12}}`,
			expected: map[string]float64{"seconds": 12, "resolution": 2},
		},
		{
			name:     "Kling v3 uses audio 1080p variant",
			model:    ModelKLVideoV3,
			body:     `{"model":"kl-video-v3","duration":5,"resolution":"1920x1080","sound":true}`,
			expected: map[string]float64{"seconds": 5, "resolution": 2},
		},
		{
			name:     "Kling 2.6 uses specified voice variant",
			model:    ModelKLVideoV26,
			body:     `{"model":"kl-video-v2-6","metadata":{"duration_seconds":8,"resolution":"1080p","generate_audio":true,"voice_id":"voice-1"}}`,
			expected: map[string]float64{"seconds": 8, "resolution": 4},
		},
		{
			name:     "Vidu q3 pro uses 720p ratio",
			model:    ModelVDVideoQ3Pro,
			body:     `{"model":"vd-video-q3-pro","seconds":"16","size":"1280x720"}`,
			expected: map[string]float64{"seconds": 16, "resolution": 20.0 / 9.0},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adaptor := &TaskAdaptor{}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(test.body))
			c.Request.Header.Set("Content-Type", "application/json")
			info := tokenHubVideoRelayInfo(test.model)

			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			assert.Equal(t, test.expected, adaptor.EstimateBilling(c, info))
		})
	}
}

func TestValidateRequestRequiresHumanActorBillingDuration(t *testing.T) {
	adaptor := &TaskAdaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"yt-video-humanactor",
		"audio_url":"https://example.com/voice.mp3",
		"image_url":"https://example.com/portrait.png"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	taskErr := adaptor.ValidateRequestAndSetAction(c, tokenHubVideoRelayInfo(ModelYTVideoHumanActor))

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Equal(t, "invalid_duration", taskErr.Code)
}

func TestBuildRequestBodyFlattensCompatibilityMetadataAndProtectsReservedFields(t *testing.T) {
	data := validateAndBuildRequest(t, `{
		"model":"video-alias",
		"prompt":"the person speaks to the camera",
		"size":"1080p",
		"id":"attacker-id",
		"metadata":{
			"model":"attacker-model",
			"id":"attacker-id",
			"audio_url":"https://example.com/voice.mp3",
			"image_url":"https://example.com/portrait.png",
			"frame_rate":50
		}
	}`, ModelYTVideoHumanActor)

	assert.JSONEq(t, `{
		"model":"yt-video-humanactor",
		"prompt":"the person speaks to the camera",
		"audio_url":"https://example.com/voice.mp3",
		"image_url":"https://example.com/portrait.png",
		"resolution":"1080p",
		"frame_rate":50
	}`, string(data))
}

func TestBuildRequestBodyNormalizesOnlyCommonWrapperFields(t *testing.T) {
	data := validateAndBuildRequest(t, `{
		"model":"video-alias",
		"seconds":"5",
		"image":"data:image/png;base64,c2Vjb25k",
		"images":["https://example.com/first.png",{"base64":"c2Vjb25k"}]
	}`, ModelHYVideo15)

	assert.JSONEq(t, `{
		"model":"hy-video-1.5",
		"duration":5,
		"image":{"base64":"c2Vjb25k"},
		"images":[{"url":"https://example.com/first.png"},{"base64":"c2Vjb25k"}]
	}`, string(data))
}

func TestFetchTaskUsesPostQueryContract(t *testing.T) {
	service.InitHttpClient()
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/api/video/query", r.URL.Path)
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		var err error
		requestBody, err = io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			http.Error(w, "failed to read request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"status":"completed","data":{"url":"https://example.com/video.mp4"}}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	response, err := adaptor.FetchTask(server.URL, "sk-test", map[string]any{
		"task_id": "upstream-task-id",
		"model":   "hy-video-1.5",
	}, "")

	require.NoError(t, err)
	require.NotNil(t, response)
	response.Body.Close()
	assert.JSONEq(t, `{"model":"hy-video-1.5","id":"upstream-task-id"}`, string(requestBody))
}

func TestParseTaskResultMapsCompletedVideo(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{
		"status":"completed",
		"progress":100,
		"data":{"url":"https://example.com/video.mp4"}
	}`))

	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Equal(t, "https://example.com/video.mp4", result.Url)
}

func tokenHubVideoRelayInfo(model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://tokenhub.example",
			ApiKey:            "sk-test",
			UpstreamModelName: model,
		},
	}
}

func validateAndBuildRequest(t *testing.T, requestBody, upstreamModel string) []byte {
	t.Helper()
	adaptor := &TaskAdaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	info := tokenHubVideoRelayInfo(upstreamModel)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	return data
}
