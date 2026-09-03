package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAssetLibraryImageMetadataEnforcesDocumentedLimits(t *testing.T) {
	testCases := []struct {
		name      string
		metadata  AssetMediaMetadata
		wantError string
	}{
		{
			name:     "valid image",
			metadata: AssetMediaMetadata{Format: "png", FileSize: 29 * 1024 * 1024, Width: 1200, Height: 800},
		},
		{
			name:      "unsupported format",
			metadata:  AssetMediaMetadata{Format: "svg", FileSize: 1024, Width: 1200, Height: 800},
			wantError: "image format",
		},
		{
			name:      "size must be strictly below thirty megabytes",
			metadata:  AssetMediaMetadata{Format: "png", FileSize: 30 * 1024 * 1024, Width: 1200, Height: 800},
			wantError: "image size",
		},
		{
			name:      "dimension lower bound is exclusive",
			metadata:  AssetMediaMetadata{Format: "jpeg", FileSize: 1024, Width: 300, Height: 800},
			wantError: "image width and height",
		},
		{
			name:      "dimension upper bound is exclusive",
			metadata:  AssetMediaMetadata{Format: "webp", FileSize: 1024, Width: 6000, Height: 2400},
			wantError: "image width and height",
		},
		{
			name:      "aspect ratio lower bound is exclusive",
			metadata:  AssetMediaMetadata{Format: "heic", FileSize: 1024, Width: 400, Height: 1000},
			wantError: "image aspect ratio",
		},
		{
			name:      "aspect ratio upper bound is exclusive",
			metadata:  AssetMediaMetadata{Format: "heif", FileSize: 1024, Width: 1000, Height: 400},
			wantError: "image aspect ratio",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateAssetLibraryMediaMetadata("Image", testCase.metadata)
			if testCase.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.wantError)
		})
	}
}

func TestValidateAssetLibraryVideoMetadataEnforcesDocumentedLimits(t *testing.T) {
	valid := AssetMediaMetadata{
		Format: "mp4", FileSize: 200 * 1024 * 1024,
		Width: 854, Height: 480, Duration: 2, FPS: 24,
	}
	testCases := []struct {
		name      string
		mutate    func(*AssetMediaMetadata)
		wantError string
	}{
		{name: "valid inclusive lower boundaries", mutate: func(*AssetMediaMetadata) {}},
		{name: "unsupported format", mutate: func(metadata *AssetMediaMetadata) { metadata.Format = "webm" }, wantError: "video format"},
		{name: "oversized file", mutate: func(metadata *AssetMediaMetadata) { metadata.FileSize++ }, wantError: "video size"},
		{name: "duration below minimum", mutate: func(metadata *AssetMediaMetadata) { metadata.Duration = 1.99 }, wantError: "video duration"},
		{name: "duration above maximum", mutate: func(metadata *AssetMediaMetadata) { metadata.Duration = 30.01 }, wantError: "video duration"},
		{name: "dimension below minimum", mutate: func(metadata *AssetMediaMetadata) { metadata.Width = 299 }, wantError: "video width and height"},
		{name: "dimension above maximum", mutate: func(metadata *AssetMediaMetadata) { metadata.Width = 6001 }, wantError: "video width and height"},
		{name: "aspect ratio below minimum", mutate: func(metadata *AssetMediaMetadata) { metadata.Width, metadata.Height = 400, 1001 }, wantError: "video aspect ratio"},
		{name: "aspect ratio above maximum", mutate: func(metadata *AssetMediaMetadata) { metadata.Width, metadata.Height = 1001, 400 }, wantError: "video aspect ratio"},
		{name: "pixel count below minimum", mutate: func(metadata *AssetMediaMetadata) { metadata.Width, metadata.Height = 638, 638 }, wantError: "video pixel count"},
		{name: "pixel count above maximum", mutate: func(metadata *AssetMediaMetadata) { metadata.Width, metadata.Height = 1921, 1087 }, wantError: "video pixel count"},
		{name: "frame rate below minimum", mutate: func(metadata *AssetMediaMetadata) { metadata.FPS = 23.99 }, wantError: "video frame rate"},
		{name: "frame rate above maximum", mutate: func(metadata *AssetMediaMetadata) { metadata.FPS = 60.01 }, wantError: "video frame rate"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			metadata := valid
			testCase.mutate(&metadata)
			err := validateAssetLibraryMediaMetadata("Video", metadata)
			if testCase.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.wantError)
		})
	}
}

func TestValidateAssetLibraryAudioMetadataEnforcesDocumentedLimits(t *testing.T) {
	valid := AssetMediaMetadata{Format: "wav", FileSize: 15 * 1024 * 1024, Duration: 2}
	testCases := []struct {
		name      string
		mutate    func(*AssetMediaMetadata)
		wantError string
	}{
		{name: "valid inclusive lower boundaries", mutate: func(*AssetMediaMetadata) {}},
		{name: "mp3 is supported", mutate: func(metadata *AssetMediaMetadata) { metadata.Format = "mp3" }},
		{name: "unsupported format", mutate: func(metadata *AssetMediaMetadata) { metadata.Format = "flac" }, wantError: "audio format"},
		{name: "oversized file", mutate: func(metadata *AssetMediaMetadata) { metadata.FileSize++ }, wantError: "audio size"},
		{name: "duration below minimum", mutate: func(metadata *AssetMediaMetadata) { metadata.Duration = 1.99 }, wantError: "audio duration"},
		{name: "duration above maximum", mutate: func(metadata *AssetMediaMetadata) { metadata.Duration = 30.01 }, wantError: "audio duration"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			metadata := valid
			testCase.mutate(&metadata)
			err := validateAssetLibraryMediaMetadata("Audio", metadata)
			if testCase.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.wantError)
		})
	}
}

func TestInspectAssetLibraryMediaReadsRealImageMetadata(t *testing.T) {
	var body bytes.Buffer
	source := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	source.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&body, source))

	metadata, err := inspectAssetLibraryMedia(context.Background(), "Image", bytes.NewReader(body.Bytes()), int64(body.Len()))

	require.NoError(t, err)
	assert.Equal(t, "png", metadata.Format)
	assert.Equal(t, 1200, metadata.Width)
	assert.Equal(t, 800, metadata.Height)
	assert.Equal(t, int64(body.Len()), metadata.FileSize)
}

func TestInspectAssetLibraryMediaReadsRealWAVDuration(t *testing.T) {
	body := buildAssetLibraryTestWAV(8000, 2)

	metadata, err := inspectAssetLibraryMedia(context.Background(), "Audio", bytes.NewReader(body), int64(len(body)))

	require.NoError(t, err)
	assert.Equal(t, "wav", metadata.Format)
	assert.InDelta(t, 2.0, metadata.Duration, 0.001)
}

func TestInspectAssetLibraryMediaReadsMP4TrackMetadata(t *testing.T) {
	body := buildAssetLibraryTestMP4("isom", 854, 480, 1000, 2000, 48)

	metadata, err := inspectAssetLibraryMedia(context.Background(), "Video", bytes.NewReader(body), int64(len(body)))

	require.NoError(t, err)
	assert.Equal(t, "mp4", metadata.Format)
	assert.Equal(t, 854, metadata.Width)
	assert.Equal(t, 480, metadata.Height)
	assert.InDelta(t, 2.0, metadata.Duration, 0.001)
	assert.InDelta(t, 24.0, metadata.FPS, 0.001)
}

func TestInspectAssetLibraryMediaRejectsNonMP4AndNonMOVContainerBrands(t *testing.T) {
	body := buildAssetLibraryTestMP4("3gp4", 854, 480, 1000, 2000, 48)

	_, err := inspectAssetLibraryMedia(context.Background(), "Video", bytes.NewReader(body), int64(len(body)))

	require.ErrorContains(t, err, "video format must be mp4 or mov")
}

func TestValidateAssetLibraryMediaResponseRejectsDeclaredOversizeBeforeReading(t *testing.T) {
	response := &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: 30 * 1024 * 1024,
		Body:          io.NopCloser(bytes.NewReader(nil)),
	}

	_, err := validateAssetLibraryMediaResponse(context.Background(), "Image", response)

	require.ErrorContains(t, err, "image size")
}

func buildAssetLibraryTestWAV(sampleRate uint32, seconds uint32) []byte {
	dataSize := sampleRate * seconds * 2
	body := make([]byte, 44+dataSize)
	copy(body[0:4], "RIFF")
	binary.LittleEndian.PutUint32(body[4:8], uint32(len(body)-8))
	copy(body[8:12], "WAVE")
	copy(body[12:16], "fmt ")
	binary.LittleEndian.PutUint32(body[16:20], 16)
	binary.LittleEndian.PutUint16(body[20:22], 1)
	binary.LittleEndian.PutUint16(body[22:24], 1)
	binary.LittleEndian.PutUint32(body[24:28], sampleRate)
	binary.LittleEndian.PutUint32(body[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(body[32:34], 2)
	binary.LittleEndian.PutUint16(body[34:36], 16)
	copy(body[36:40], "data")
	binary.LittleEndian.PutUint32(body[40:44], dataSize)
	return body
}

func buildAssetLibraryTestMP4(brand string, width, height uint32, timescale, duration, samples uint32) []byte {
	ftypPayload := make([]byte, 12)
	copy(ftypPayload[0:4], brand)
	copy(ftypPayload[8:12], brand)

	tkhdPayload := make([]byte, 84)
	binary.BigEndian.PutUint32(tkhdPayload[len(tkhdPayload)-8:len(tkhdPayload)-4], width<<16)
	binary.BigEndian.PutUint32(tkhdPayload[len(tkhdPayload)-4:], height<<16)

	hdlrPayload := make([]byte, 12)
	copy(hdlrPayload[8:12], "vide")
	mdhdPayload := make([]byte, 24)
	binary.BigEndian.PutUint32(mdhdPayload[12:16], timescale)
	binary.BigEndian.PutUint32(mdhdPayload[16:20], duration)
	sttsPayload := make([]byte, 16)
	binary.BigEndian.PutUint32(sttsPayload[4:8], 1)
	binary.BigEndian.PutUint32(sttsPayload[8:12], samples)
	if samples > 0 {
		binary.BigEndian.PutUint32(sttsPayload[12:16], duration/samples)
	}

	stbl := assetLibraryTestBox("stbl", assetLibraryTestBox("stts", sttsPayload))
	minf := assetLibraryTestBox("minf", stbl)
	mdia := assetLibraryTestBox("mdia", assetLibraryTestBox("mdhd", mdhdPayload), assetLibraryTestBox("hdlr", hdlrPayload), minf)
	trak := assetLibraryTestBox("trak", assetLibraryTestBox("tkhd", tkhdPayload), mdia)
	moov := assetLibraryTestBox("moov", trak)
	return append(assetLibraryTestBox("ftyp", ftypPayload), moov...)
}

func assetLibraryTestBox(boxType string, payloads ...[]byte) []byte {
	size := 8
	for _, payload := range payloads {
		size += len(payload)
	}
	body := make([]byte, size)
	binary.BigEndian.PutUint32(body[0:4], uint32(size))
	copy(body[4:8], boxType)
	offset := 8
	for _, payload := range payloads {
		copy(body[offset:], payload)
		offset += len(payload)
	}
	return body
}
