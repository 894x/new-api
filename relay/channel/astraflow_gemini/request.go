package astraflow_gemini

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

const maxEditInputImages = 3

var aspectRatioBySize = map[string]string{
	"1024x1024": "1:1",
	"1024x1536": "2:3",
	"1536x1024": "3:2",
	"832x1248":  "2:3",
	"1248x832":  "3:2",
	"864x1184":  "3:4",
	"1184x864":  "4:3",
	"896x1152":  "4:5",
	"1152x896":  "5:4",
	"768x1344":  "9:16",
	"1344x768":  "16:9",
	"1536x672":  "21:9",
	"1:1":       "1:1",
	"2:3":       "2:3",
	"3:2":       "3:2",
	"3:4":       "3:4",
	"4:3":       "4:3",
	"4:5":       "4:5",
	"5:4":       "5:4",
	"9:16":      "9:16",
	"16:9":      "16:9",
	"21:9":      "21:9",
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if err := validateModel(info); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	if request.N != nil && *request.N != 1 {
		return nil, errors.New("n must be 1 for gemini-2.5-flash-image")
	}
	if request.Stream != nil && *request.Stream {
		return nil, errors.New("stream is not supported by the OpenAI-compatible image endpoint")
	}
	if request.Quality != "" && request.Quality != "auto" && request.Quality != "standard" {
		return nil, errors.New("quality must be auto or standard for gemini-2.5-flash-image")
	}
	if request.ResponseFormat != "" && request.ResponseFormat != "b64_json" {
		return nil, errors.New("response_format must be b64_json for gemini-2.5-flash-image")
	}
	if len(request.Style) > 0 || len(request.ExtraFields) > 0 || len(request.Background) > 0 ||
		len(request.Moderation) > 0 || len(request.User) > 0 || len(request.OutputFormat) > 0 || len(request.OutputCompression) > 0 ||
		len(request.PartialImages) > 0 || len(request.Mask) > 0 || len(request.InputFidelity) > 0 ||
		request.Watermark != nil || len(request.WatermarkEnabled) > 0 || len(request.UserId) > 0 ||
		len(request.Image) > 0 || len(request.Images) > 0 || len(request.Extra) > 0 {
		return nil, errors.New("request contains fields that are not supported by the AstraFlow Gemini image protocol")
	}

	aspectRatio := ""
	if request.Size != "" {
		var ok bool
		aspectRatio, ok = aspectRatioBySize[request.Size]
		if !ok {
			return nil, fmt.Errorf("size %q is not supported by gemini-2.5-flash-image", request.Size)
		}
	}

	parts := []dto.GeminiPart{{Text: request.Prompt}}
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations:
	case relayconstant.RelayModeImagesEdits:
		imageParts, err := imageEditParts(c)
		if err != nil {
			return nil, err
		}
		parts = append(parts, imageParts...)
	default:
		return nil, errors.New("endpoint is not supported by AstraFlow Gemini")
	}

	generationConfig := dto.GeminiChatGenerationConfig{
		ResponseModalities: []string{"IMAGE"},
	}
	if aspectRatio != "" {
		imageConfig, err := common.Marshal(map[string]string{"aspectRatio": aspectRatio})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Gemini image config: %w", err)
		}
		generationConfig.ImageConfig = imageConfig
	}

	return &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{
			Role:  "user",
			Parts: parts,
		}},
		GenerationConfig: generationConfig,
	}, nil
}

func imageEditParts(c *gin.Context) ([]dto.GeminiPart, error) {
	if c == nil || c.Request == nil || !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		return nil, errors.New("image edits require multipart/form-data")
	}
	form := c.Request.MultipartForm
	if form == nil {
		var err error
		form, err = common.ParseMultipartFormReusable(c)
		if err != nil {
			return nil, fmt.Errorf("failed to parse image edit form: %w", err)
		}
		c.Request.MultipartForm = form
	}

	allowedValues := map[string]struct{}{
		"model": {}, "prompt": {}, "n": {}, "size": {}, "quality": {}, "response_format": {}, "stream": {},
	}
	for field := range form.Value {
		if _, ok := allowedValues[field]; !ok {
			return nil, fmt.Errorf("multipart field %q is not supported by AstraFlow Gemini", field)
		}
	}
	if value := strings.TrimSpace(firstFormValue(form, "response_format")); value != "" && value != "b64_json" {
		return nil, errors.New("response_format must be b64_json for gemini-2.5-flash-image")
	}

	files, err := collectImageFiles(form)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("image is required")
	}
	if len(files) > maxEditInputImages {
		return nil, fmt.Errorf("gemini-2.5-flash-image accepts at most %d input images", maxEditInputImages)
	}

	parts := make([]dto.GeminiPart, 0, len(files))
	for index, fileHeader := range files {
		part, err := imageFilePart(fileHeader)
		if err != nil {
			return nil, fmt.Errorf("invalid input image %d: %w", index+1, err)
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func firstFormValue(form *multipart.Form, field string) string {
	if form == nil || len(form.Value[field]) == 0 {
		return ""
	}
	return form.Value[field][0]
}

func collectImageFiles(form *multipart.Form) ([]*multipart.FileHeader, error) {
	files := make([]*multipart.FileHeader, 0)
	files = append(files, form.File["image"]...)
	files = append(files, form.File["image[]"]...)

	indexedFields := make([]string, 0)
	for field := range form.File {
		if field == "image" || field == "image[]" {
			continue
		}
		if strings.HasPrefix(field, "image[") {
			indexedFields = append(indexedFields, field)
			continue
		}
		return nil, fmt.Errorf("multipart file field %q is not supported by AstraFlow Gemini", field)
	}
	sort.Strings(indexedFields)
	for _, field := range indexedFields {
		files = append(files, form.File[field]...)
	}
	return files, nil
}

func imageFilePart(fileHeader *multipart.FileHeader) (dto.GeminiPart, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return dto.GeminiPart{}, fmt.Errorf("failed to open %q: %w", fileHeader.Filename, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return dto.GeminiPart{}, fmt.Errorf("failed to read %q: %w", fileHeader.Filename, err)
	}
	if len(data) == 0 {
		return dto.GeminiPart{}, errors.New("image file is empty")
	}

	mimeType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return dto.GeminiPart{}, fmt.Errorf("%q is not an image", filepath.Base(fileHeader.Filename))
	}

	return dto.GeminiPart{InlineData: &dto.GeminiInlineData{
		MimeType: mimeType,
		Data:     base64.StdEncoding.EncodeToString(data),
	}}, nil
}
