package astraflow_image

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

type imageModelSpec struct {
	maxImages             uint
	fixedSizes            map[string]struct{}
	customSizes           bool
	outputFormats         map[string]struct{}
	allowBackground       bool
	allowModeration       bool
	allowUser             bool
	allowMultipleEditImgs bool
}

var fixedImageSizes = map[string]struct{}{
	"1024x1024": {},
	"1024x1536": {},
	"1536x1024": {},
}

var pngJpegFormats = map[string]struct{}{
	"png":  {},
	"jpeg": {},
}

var editImageExtensions = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".webp": {},
}

var imageModelSpecs = map[string]imageModelSpec{
	"gpt-image-1": {
		maxImages:     4,
		fixedSizes:    fixedImageSizes,
		outputFormats: pngJpegFormats,
	},
	"gpt-image-1-mini": {
		maxImages:       10,
		fixedSizes:      fixedImageSizes,
		outputFormats:   pngJpegFormats,
		allowBackground: true,
		allowModeration: true,
		allowUser:       true,
	},
	"gpt-image-1.5": {
		maxImages:  4,
		fixedSizes: fixedImageSizes,
		outputFormats: map[string]struct{}{
			"png":  {},
			"jpeg": {},
			"webp": {},
		},
	},
	"gpt-image-2": {
		maxImages:             10,
		customSizes:           true,
		outputFormats:         pngJpegFormats,
		allowMultipleEditImgs: true,
	},
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	spec, ok := imageModelSpecs[request.Model]
	if !ok {
		return nil, fmt.Errorf("model %q is not supported by %s", request.Model, ChannelName)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	if request.N == nil || *request.N < 1 || *request.N > spec.maxImages {
		return nil, fmt.Errorf("n must be an integer between 1 and %d for %s", spec.maxImages, request.Model)
	}
	if request.Stream != nil {
		return nil, errors.New("stream is not supported by AstraFlow image models")
	}

	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations:
		if err := validateGenerationRequest(request, spec); err != nil {
			return nil, err
		}
	case relayconstant.RelayModeImagesEdits:
		if c == nil || c.Request == nil || !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			return nil, errors.New("AstraFlow image edits require multipart/form-data")
		}
		if err := validateEditRequest(c, request, spec); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("endpoint is not supported by AstraFlow Image")
	}

	return a.Adaptor.ConvertImageRequest(c, info, request)
}

func validateGenerationRequest(request dto.ImageRequest, spec imageModelSpec) error {
	if request.ResponseFormat != "" || len(request.Style) > 0 || len(request.ExtraFields) > 0 ||
		len(request.PartialImages) > 0 || len(request.Images) > 0 || len(request.Mask) > 0 ||
		len(request.InputFidelity) > 0 || request.Watermark != nil || len(request.WatermarkEnabled) > 0 ||
		len(request.UserId) > 0 || len(request.Image) > 0 {
		return errors.New("request contains fields that are not supported by the AstraFlow image generation protocol")
	}
	if len(request.Extra) > 0 {
		return errors.New("request contains unknown fields")
	}
	if err := validateCommonFields(request.Size, request.Quality, request.OutputFormat, request.OutputCompression, spec); err != nil {
		return err
	}
	if err := validateOptionalMiniFields(request.Background, request.Moderation, request.User, spec); err != nil {
		return err
	}
	return nil
}

func validateEditRequest(c *gin.Context, request dto.ImageRequest, spec imageModelSpec) error {
	form := c.Request.MultipartForm
	if form == nil {
		return errors.New("multipart form is required")
	}

	allowedFields := map[string]struct{}{
		"model": {}, "prompt": {}, "n": {}, "size": {}, "quality": {},
		"output_format": {}, "output_compression": {},
	}
	if spec.allowBackground {
		allowedFields["background"] = struct{}{}
	}
	if spec.allowModeration {
		allowedFields["moderation"] = struct{}{}
	}
	if spec.allowUser {
		allowedFields["user"] = struct{}{}
	}
	for field, values := range form.Value {
		if _, ok := allowedFields[field]; !ok {
			return fmt.Errorf("field %q is not supported by %s", field, request.Model)
		}
		if len(values) > 1 {
			return fmt.Errorf("field %q may only be provided once", field)
		}
	}

	if err := validateCommonFieldStrings(
		firstFormValue(form, "size"),
		firstFormValue(form, "quality"),
		firstFormValue(form, "output_format"),
		firstFormValue(form, "output_compression"),
		spec,
	); err != nil {
		return err
	}
	if err := validateMiniFieldStrings(
		firstFormValue(form, "background"),
		firstFormValue(form, "moderation"),
		firstFormValue(form, "user"),
		spec,
	); err != nil {
		return err
	}
	if nValue := strings.TrimSpace(firstFormValue(form, "n")); nValue != "" {
		n, err := strconv.Atoi(nValue)
		if err != nil || n < 1 || uint(n) > spec.maxImages {
			return fmt.Errorf("n must be an integer between 1 and %d for %s", spec.maxImages, request.Model)
		}
	}

	return validateEditFiles(form, request.Model, spec)
}

func firstFormValue(form *multipart.Form, field string) string {
	values := form.Value[field]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func validateEditFiles(form *multipart.Form, model string, spec imageModelSpec) error {
	for field := range form.File {
		if field != "image" && field != "image[]" && field != "mask" {
			return fmt.Errorf("file field %q is not supported by %s", field, model)
		}
	}
	if len(form.File["image"]) > 0 && len(form.File["image[]"]) > 0 {
		return errors.New("use either image or image[], not both")
	}
	images := append([]*multipart.FileHeader{}, form.File["image"]...)
	images = append(images, form.File["image[]"]...)
	if len(images) == 0 {
		return errors.New("image is required")
	}
	if !spec.allowMultipleEditImgs && (len(images) != 1 || len(form.File["image[]"]) > 0) {
		return fmt.Errorf("%s accepts exactly one image file", model)
	}
	for _, image := range images {
		if _, ok := editImageExtensions[strings.ToLower(filepath.Ext(image.Filename))]; !ok {
			return errors.New("AstraFlow image edit files must be PNG, JPEG, or WebP")
		}
	}
	if masks := form.File["mask"]; len(masks) > 1 {
		return errors.New("only one mask file is supported")
	} else if len(masks) == 1 && !strings.EqualFold(filepath.Ext(masks[0].Filename), ".png") {
		return errors.New("AstraFlow mask files must be PNG")
	}
	return nil
}

func validateCommonFields(size, quality string, outputFormat, outputCompression json.RawMessage, spec imageModelSpec) error {
	format, _, err := optionalJSONString("output_format", outputFormat)
	if err != nil {
		return err
	}
	compression, hasCompression, err := optionalJSONInt("output_compression", outputCompression)
	if err != nil {
		return err
	}
	compressionValue := ""
	if hasCompression {
		compressionValue = strconv.Itoa(compression)
	}
	return validateCommonFieldStrings(size, quality, format, compressionValue, spec)
}

func validateCommonFieldStrings(size, quality, outputFormat, outputCompression string, spec imageModelSpec) error {
	if quality != "" && quality != "low" && quality != "medium" && quality != "high" {
		return errors.New("quality must be one of low, medium, or high")
	}
	if outputFormat != "" {
		if _, ok := spec.outputFormats[outputFormat]; !ok {
			return fmt.Errorf("output_format %q is not supported by this model", outputFormat)
		}
	}
	if outputCompression != "" {
		compression, err := strconv.Atoi(outputCompression)
		if err != nil || compression < 0 || compression > 100 {
			return errors.New("output_compression must be an integer between 0 and 100")
		}
	}
	if size == "" {
		return nil
	}
	if spec.customSizes {
		return validateGPTImage2Size(size)
	}
	if _, ok := spec.fixedSizes[size]; !ok {
		return errors.New("size must be one of 1024x1024, 1024x1536, or 1536x1024")
	}
	return nil
}

func validateGPTImage2Size(size string) error {
	if size == "auto" {
		return nil
	}
	widthText, heightText, ok := strings.Cut(size, "x")
	if !ok || widthText == "" || heightText == "" || strings.Contains(heightText, "x") {
		return errors.New("gpt-image-2 size must be auto or WIDTHxHEIGHT")
	}
	width, widthErr := strconv.Atoi(widthText)
	height, heightErr := strconv.Atoi(heightText)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return errors.New("gpt-image-2 size must contain positive integer dimensions")
	}
	if width%16 != 0 || height%16 != 0 {
		return errors.New("gpt-image-2 width and height must be multiples of 16")
	}
	if width > 3840 || height > 3840 {
		return errors.New("gpt-image-2 width and height must not exceed 3840")
	}
	smaller, larger := width, height
	if smaller > larger {
		smaller, larger = larger, smaller
	}
	if larger > smaller*3 {
		return errors.New("gpt-image-2 aspect ratio must not exceed 3:1")
	}
	pixels := width * height
	if pixels < 655360 || pixels > 8294400 {
		return errors.New("gpt-image-2 total pixels must be between 655360 and 8294400")
	}
	return nil
}

func validateOptionalMiniFields(background, moderation, user json.RawMessage, spec imageModelSpec) error {
	backgroundValue, _, err := optionalJSONString("background", background)
	if err != nil {
		return err
	}
	moderationValue, _, err := optionalJSONString("moderation", moderation)
	if err != nil {
		return err
	}
	userValue, _, err := optionalJSONString("user", user)
	if err != nil {
		return err
	}
	return validateMiniFieldStrings(backgroundValue, moderationValue, userValue, spec)
}

func validateMiniFieldStrings(background, moderation, user string, spec imageModelSpec) error {
	if background != "" {
		if !spec.allowBackground {
			return errors.New("background is only supported by gpt-image-1-mini")
		}
		if background != "transparent" && background != "opaque" && background != "auto" {
			return errors.New("background must be one of transparent, opaque, or auto")
		}
	}
	if moderation != "" {
		if !spec.allowModeration {
			return errors.New("moderation is only supported by gpt-image-1-mini")
		}
		if moderation != "auto" && moderation != "low" {
			return errors.New("moderation must be auto or low")
		}
	}
	if user != "" && !spec.allowUser {
		return errors.New("user is only supported by gpt-image-1-mini")
	}
	return nil
}

func optionalJSONString(name string, raw json.RawMessage) (string, bool, error) {
	if len(raw) == 0 {
		return "", false, nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", false, fmt.Errorf("%s must be a string", name)
	}
	return value, true, nil
}

func optionalJSONInt(name string, raw json.RawMessage) (int, bool, error) {
	if len(raw) == 0 {
		return 0, false, nil
	}
	var value int
	if err := common.Unmarshal(raw, &value); err != nil {
		return 0, false, fmt.Errorf("%s must be an integer", name)
	}
	return value, true, nil
}
