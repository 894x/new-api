package astraflow_gemini

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertImageGenerationRequestUsesGeminiGenerateContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelName,
		},
	}

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  ModelName,
		Prompt: "draw a red panda",
		N:      rootcommon.GetPointer(uint(1)),
		Size:   "1024x1536",
	})

	require.NoError(t, err)
	request, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, request.Contents, 1)
	assert.Equal(t, "draw a red panda", request.Contents[0].Parts[0].Text)
	assert.Equal(t, []string{"IMAGE"}, request.GenerationConfig.ResponseModalities)
	assert.JSONEq(t, `{"aspectRatio":"2:3"}`, string(request.GenerationConfig.ImageConfig))
}

func TestConvertOpenAIChatRequestEnablesImageOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: ModelName}}

	converted, err := adaptor.ConvertOpenAIRequest(c, info, &dto.GeneralOpenAIRequest{
		Model: ModelName,
		Messages: []dto.Message{{
			Role:    "user",
			Content: "draw a red panda",
		}},
	})

	require.NoError(t, err)
	request, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	assert.Equal(t, []string{"TEXT", "IMAGE"}, request.GenerationConfig.ResponseModalities)
}

func TestConvertImageEditRequestEmbedsMultipartImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", ModelName))
	require.NoError(t, writer.WriteField("prompt", "turn it into a watercolor"))
	require.NoError(t, writer.WriteField("n", "1"))
	file, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("\x89PNG\r\n\x1a\nimage"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, request.ParseMultipartForm(32<<20))
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelName,
		},
	}, dto.ImageRequest{
		Model:  ModelName,
		Prompt: "turn it into a watercolor",
		N:      rootcommon.GetPointer(uint(1)),
	})

	require.NoError(t, err)
	geminiRequest, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, geminiRequest.Contents[0].Parts, 2)
	require.NotNil(t, geminiRequest.Contents[0].Parts[1].InlineData)
	assert.Equal(t, "image/png", geminiRequest.Contents[0].Parts[1].InlineData.MimeType)
	assert.NotEmpty(t, geminiRequest.Contents[0].Parts[1].InlineData.Data)
}

func TestHandleImageResponseConvertsImagesAndPreservesGeminiBillingUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"candidates":[{"content":{"parts":[{"text":"refined prompt"},{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}],
			"usageMetadata":{
				"promptTokenCount":1332,
				"candidatesTokenCount":3888,
				"totalTokenCount":5220,
				"promptTokensDetails":[{"modality":"TEXT","tokenCount":42},{"modality":"IMAGE","tokenCount":1290}],
				"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":18},{"modality":"IMAGE","tokenCount":3870}]
			}
		}`)),
	}

	usage, apiErr := handleImageResponse(c, resp, &relaycommon.RelayInfo{})

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 1290, usage.PromptTokensDetails.ImageTokens)
	assert.Equal(t, 3870, usage.CompletionTokenDetails.ImageTokens)
	require.NotNil(t, usage.BillingUsage)
	assert.Equal(t, dto.BillingUsageSemanticGemini, usage.BillingUsage.Semantic)
	var clientResponse openAIImageResponse
	require.NoError(t, rootcommon.UnmarshalJsonStr(recorder.Body.String(), &clientResponse))
	require.Len(t, clientResponse.Data, 1)
	assert.Equal(t, "aW1hZ2U=", clientResponse.Data[0].B64Json)
	assert.Equal(t, "refined prompt", clientResponse.Data[0].RevisedPrompt)
	assert.Equal(t, 5220, clientResponse.Usage.TotalTokens)
	assert.Equal(t, 1290, clientResponse.Usage.InputTokensDetails.ImageTokens)
	assert.Equal(t, 3870, clientResponse.Usage.OutputTokensDetails.ImageTokens)
}

func TestAstraFlowGeminiAdaptorRoundTripsOpenAIImageRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1beta/models/gemini-2.5-flash-image:generateContent", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("x-goog-api-key"))
		var request dto.GeminiChatRequest
		require.NoError(t, rootcommon.DecodeJson(r.Body, &request))
		assert.Equal(t, []string{"IMAGE"}, request.GenerationConfig.ResponseModalities)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aW1hZ2U="}}]}}],
			"usageMetadata":{
				"promptTokenCount":10,
				"candidatesTokenCount":1290,
				"totalTokenCount":1300,
				"promptTokensDetails":[{"modality":"TEXT","tokenCount":10}],
				"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1290}]
			}
		}`)
	}))
	defer upstream.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: ModelName,
			ChannelBaseUrl:    upstream.URL,
			ApiKey:            "test-key",
		},
	}
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  ModelName,
		Prompt: "draw a red panda",
		N:      rootcommon.GetPointer(uint(1)),
	})
	require.NoError(t, err)
	requestBody, err := rootcommon.Marshal(converted)
	require.NoError(t, err)

	response, err := adaptor.DoRequest(c, info, bytes.NewReader(requestBody))
	require.NoError(t, err)
	usage, apiErr := adaptor.DoResponse(c, response.(*http.Response), info)

	require.Nil(t, apiErr)
	assert.Equal(t, 1290, usage.(*dto.Usage).CompletionTokenDetails.ImageTokens)
	assert.Contains(t, recorder.Body.String(), `"b64_json":"aW1hZ2U="`)
}
