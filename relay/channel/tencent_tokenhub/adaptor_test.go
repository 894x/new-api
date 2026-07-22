package tencent_tokenhub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLSelectsImageEndpoint(t *testing.T) {
	adaptor := &Adaptor{}
	tests := []struct {
		model string
		path  string
	}{
		{model: ModelHYImageLite, path: "/v1/api/image/lite"},
		{model: ModelHYImageV3, path: "/v1/api/image/submit"},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			info := tokenHubImageRelayInfo(test.model)
			requestURL, err := adaptor.GetRequestURL(info)

			require.NoError(t, err)
			assert.Equal(t, "https://tokenhub.example"+test.path, requestURL)
		})
	}
}

func TestConvertImageRequestUsesMappedModelAndProviderFields(t *testing.T) {
	adaptor := &Adaptor{}
	n := uint(1)
	watermark := true
	request := dto.ImageRequest{
		Model:          "image-alias",
		Prompt:         "a lighthouse in fog",
		N:              &n,
		Size:           "1024x1024",
		ResponseFormat: "b64_json",
		Style:          []byte(`"riman"`),
		Watermark:      &watermark,
		Extra: map[string]json.RawMessage{
			"negative_prompt": []byte(`"low quality"`),
			"seed":            []byte(`42`),
			"provider_option": []byte(`{"enabled":true}`),
		},
	}

	converted, err := adaptor.ConvertImageRequest(nil, tokenHubImageRelayInfo(ModelHYImageLite), request)

	require.NoError(t, err)
	image, ok := converted.(*imageRequest)
	require.True(t, ok)
	assert.Equal(t, ModelHYImageLite, image.Model)
	assert.Equal(t, "1024:1024", image.Resolution)
	assert.Equal(t, "base64", image.RspImgType)
	assert.JSONEq(t, `"riman"`, string(image.Style))
	assert.JSONEq(t, `"low quality"`, string(image.NegativePrompt))
	assert.JSONEq(t, `42`, string(image.Seed))
	assert.JSONEq(t, `1`, string(image.LogoAdd))
	encoded, err := common.Marshal(image)
	require.NoError(t, err)
	var encodedPayload map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(encoded, &encodedPayload))
	assert.JSONEq(t, `{"enabled":true}`, string(encodedPayload["provider_option"]))
}

func TestConvertImageRequestRejectsMultipleImages(t *testing.T) {
	adaptor := &Adaptor{}
	n := uint(2)

	_, err := adaptor.ConvertImageRequest(nil, tokenHubImageRelayInfo(ModelHYImageLite), dto.ImageRequest{
		Model:  ModelHYImageLite,
		Prompt: "a lighthouse in fog",
		N:      &n,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one image")
}

func TestSetupRequestHeaderUsesBearerToken(t *testing.T) {
	adaptor := &Adaptor{}
	header := http.Header{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	require.NoError(t, adaptor.SetupRequestHeader(c, &header, tokenHubImageRelayInfo(ModelHYImageLite)))
	assert.Equal(t, "Bearer sk-test", header.Get("Authorization"))
	assert.Equal(t, "application/json", header.Get("Content-Type"))
}

func TestDoRequestPollsAsyncImageTaskToCompletion(t *testing.T) {
	service.InitHttpClient()
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			http.Error(w, "failed to read request", http.StatusInternalServerError)
			return
		}
		requests = append(requests, r.URL.Path+" "+string(body))
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/api/image/submit":
			w.WriteHeader(http.StatusAccepted)
			_, err = w.Write([]byte(`{"id":"image-job-id","status":"queued"}`))
		case "/v1/api/image/query":
			_, err = w.Write([]byte(`{"status":"completed","data":[{"url":"https://example.com/image.png"}]}`))
		default:
			assert.Failf(t, "unexpected path", "path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		assert.NoError(t, err)
	}))
	defer server.Close()

	adaptor := &Adaptor{pollAttempts: 1, pollInterval: time.Millisecond}
	info := tokenHubImageRelayInfo(ModelHYImageV3)
	info.ChannelBaseUrl = server.URL
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	result, err := adaptor.DoRequest(c, info, bytes.NewBufferString(`{"model":"hy-image-v3.0","prompt":"a lighthouse"}`))

	require.NoError(t, err)
	response, ok := result.(*http.Response)
	require.True(t, ok)
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	response.Body.Close()
	assert.JSONEq(t, `{"status":"completed","data":[{"url":"https://example.com/image.png"}]}`, string(responseBody))
	require.Len(t, requests, 2)
	assert.Contains(t, requests[0], "/v1/api/image/submit")
	assert.Equal(t, `/v1/api/image/query {"id":"image-job-id","model":"hy-image-v3.0"}`, requests[1])
}

func tokenHubImageRelayInfo(model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://tokenhub.example",
			ApiKey:            "sk-test",
			UpstreamModelName: model,
		},
	}
}
