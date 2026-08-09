package astraflow_image

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPointer(value int) *int {
	return &value
}

func uintPointer(value uint) *uint {
	return &value
}

func TestNormalizeUsagePreservesAstraFlowBillingCategories(t *testing.T) {
	tests := []struct {
		name            string
		raw             *astraFlowUsage
		wantPromptText  int
		wantPromptImage int
		wantOutputText  int
		wantOutputImage int
	}{
		{
			name: "documented aggregate output is image output",
			raw: &astraFlowUsage{
				TotalTokens:  intPointer(4169),
				InputTokens:  intPointer(9),
				OutputTokens: intPointer(4160),
				InputTokensDetails: &tokenDetails{
					TextTokens: intPointer(9),
				},
			},
			wantPromptText:  9,
			wantOutputImage: 4160,
		},
		{
			name: "explicit output breakdown preserves text and image tokens",
			raw: &astraFlowUsage{
				TotalTokens:  intPointer(410),
				InputTokens:  intPointer(18),
				OutputTokens: intPointer(392),
				InputTokensDetails: &tokenDetails{
					TextTokens:  intPointer(18),
					ImageTokens: intPointer(0),
				},
				OutputTokensDetails: &tokenDetails{
					TextTokens:  intPointer(120),
					ImageTokens: intPointer(272),
				},
			},
			wantPromptText:  18,
			wantOutputText:  120,
			wantOutputImage: 272,
		},
		{
			name: "unclassified edit input is conservatively image input",
			raw: &astraFlowUsage{
				TotalTokens:  intPointer(200),
				InputTokens:  intPointer(100),
				OutputTokens: intPointer(100),
				InputTokensDetails: &tokenDetails{
					TextTokens: intPointer(10),
				},
			},
			wantPromptText:  10,
			wantPromptImage: 90,
			wantOutputImage: 100,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usage, err := normalizeUsage(test.raw)

			require.NoError(t, err)
			assert.Equal(t, test.wantPromptText, usage.PromptTokensDetails.TextTokens)
			assert.Equal(t, test.wantPromptImage, usage.PromptTokensDetails.ImageTokens)
			assert.Equal(t, test.wantOutputText, usage.CompletionTokenDetails.TextTokens)
			assert.Equal(t, test.wantOutputImage, usage.CompletionTokenDetails.ImageTokens)
			assert.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
		})
	}
}

func TestNormalizeUsageRejectsInvalidBillingUsage(t *testing.T) {
	tests := []struct {
		name string
		raw  *astraFlowUsage
	}{
		{
			name: "missing totals",
			raw:  &astraFlowUsage{},
		},
		{
			name: "negative tokens",
			raw: &astraFlowUsage{
				InputTokens:  intPointer(-1),
				OutputTokens: intPointer(10),
			},
		},
		{
			name: "details exceed total",
			raw: &astraFlowUsage{
				InputTokens:  intPointer(10),
				OutputTokens: intPointer(20),
				InputTokensDetails: &tokenDetails{
					TextTokens:  intPointer(8),
					ImageTokens: intPointer(3),
				},
			},
		},
		{
			name: "inconsistent total",
			raw: &astraFlowUsage{
				TotalTokens:  intPointer(31),
				InputTokens:  intPointer(10),
				OutputTokens: intPointer(20),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeUsage(test.raw)
			require.Error(t, err)
		})
	}
}

func TestConvertImageRequestValidatesPerModelGenerationContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &common.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}
	adaptor := &Adaptor{}

	tests := []struct {
		name    string
		request dto.ImageRequest
		wantErr string
	}{
		{
			name: "gpt image 1 accepts its documented limit",
			request: dto.ImageRequest{
				Model: "gpt-image-1", Prompt: "draw a flower", N: uintPointer(4),
				Size: "1024x1536", Quality: "high", OutputFormat: json.RawMessage(`"jpeg"`),
			},
		},
		{
			name: "gpt image 1 rejects too many images",
			request: dto.ImageRequest{
				Model: "gpt-image-1", Prompt: "draw a flower", N: uintPointer(5),
			},
			wantErr: "between 1 and 4",
		},
		{
			name: "gpt image 1 rejects webp",
			request: dto.ImageRequest{
				Model: "gpt-image-1", Prompt: "draw a flower", N: uintPointer(1),
				OutputFormat: json.RawMessage(`"webp"`),
			},
			wantErr: "not supported",
		},
		{
			name: "gpt image 1 point 5 accepts webp",
			request: dto.ImageRequest{
				Model: "gpt-image-1.5", Prompt: "draw a flower", N: uintPointer(1),
				OutputFormat: json.RawMessage(`"webp"`),
			},
		},
		{
			name: "mini accepts documented optional fields",
			request: dto.ImageRequest{
				Model: "gpt-image-1-mini", Prompt: "draw a flower", N: uintPointer(10),
				Background: json.RawMessage(`"transparent"`), Moderation: json.RawMessage(`"low"`),
				User: json.RawMessage(`"customer-1"`),
			},
		},
		{
			name: "gpt image 2 accepts maximum documented size",
			request: dto.ImageRequest{
				Model: "gpt-image-2", Prompt: "draw a flower", N: uintPointer(10), Size: "2160x3840",
			},
		},
		{
			name: "gpt image 2 rejects excessive aspect ratio",
			request: dto.ImageRequest{
				Model: "gpt-image-2", Prompt: "draw a flower", N: uintPointer(1), Size: "2048x512",
			},
			wantErr: "aspect ratio",
		},
		{
			name: "gpt image 2 rejects non aligned dimensions",
			request: dto.ImageRequest{
				Model: "gpt-image-2", Prompt: "draw a flower", N: uintPointer(1), Size: "1025x1024",
			},
			wantErr: "multiples of 16",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := adaptor.ConvertImageRequest(c, info, test.request)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateEditFilesEnforcesProviderContract(t *testing.T) {
	png := &multipart.FileHeader{Filename: "input.png"}
	secondPNG := &multipart.FileHeader{Filename: "reference.png"}
	jpeg := &multipart.FileHeader{Filename: "input.jpg"}

	require.NoError(t, validateEditFiles(&multipart.Form{
		File: map[string][]*multipart.FileHeader{"image[]": {png, secondPNG}},
	}, "gpt-image-2", imageModelSpecs["gpt-image-2"]))
	require.NoError(t, validateEditFiles(&multipart.Form{
		File: map[string][]*multipart.FileHeader{"image": {jpeg}},
	}, "gpt-image-1.5", imageModelSpecs["gpt-image-1.5"]))

	err := validateEditFiles(&multipart.Form{
		File: map[string][]*multipart.FileHeader{"image": {png, secondPNG}},
	}, "gpt-image-1.5", imageModelSpecs["gpt-image-1.5"])
	require.ErrorContains(t, err, "exactly one image")

	err = validateEditFiles(&multipart.Form{
		File: map[string][]*multipart.FileHeader{"image": {{Filename: "input.gif"}}},
	}, "gpt-image-1", imageModelSpecs["gpt-image-1"])
	require.ErrorContains(t, err, "PNG, JPEG, or WebP")
}

func TestHandleImageResponseReturnsRawBodyAndNormalizedUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"created":1750667997,"data":[{"b64_json":"image"}],"usage":{"total_tokens":4169,"input_tokens":9,"output_tokens":4160,"input_tokens_details":{"text_tokens":9}}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}

	usage, apiErr := handleImageResponse(c, &common.RelayInfo{}, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 4160, usage.CompletionTokenDetails.ImageTokens)
	assert.Equal(t, body, recorder.Body.String())
}

func TestHandleImageResponseRejectsSuccessWithoutUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"data":[{"b64_json":"image"}]}`)),
	}

	usage, apiErr := handleImageResponse(c, &common.RelayInfo{}, resp)

	assert.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "missing usage")
	assert.Empty(t, recorder.Body.String())
}
