package service

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

const (
	assetLibraryImageMaxBytes = int64(30 * 1024 * 1024)
	assetLibraryVideoMaxBytes = int64(200 * 1024 * 1024)
	assetLibraryAudioMaxBytes = int64(15 * 1024 * 1024)

	assetLibraryMinDuration = 2.0
	assetLibraryMaxDuration = 30.0
)

// AssetMediaMetadata is the verified metadata persisted for a logical asset.
type AssetMediaMetadata struct {
	Format   string
	FileSize int64
	Width    int
	Height   int
	Duration float64
	FPS      float64
}

// ValidateAssetLibraryMedia downloads a URL-backed asset through the protected
// fetch path, inspects its content, and enforces the documented media limits.
func ValidateAssetLibraryMedia(ctx context.Context, sourceURL string, assetType string) (AssetMediaMetadata, error) {
	if err := ctx.Err(); err != nil {
		return AssetMediaMetadata{}, err
	}
	response, err := DoDownloadRequest(sourceURL, "asset_library_validation")
	if err != nil {
		return AssetMediaMetadata{}, errors.New("media URL could not be downloaded")
	}
	defer response.Body.Close()
	return validateAssetLibraryMediaResponse(ctx, assetType, response)
}

func validateAssetLibraryMediaResponse(ctx context.Context, assetType string, response *http.Response) (AssetMediaMetadata, error) {
	if response.StatusCode != http.StatusOK {
		return AssetMediaMetadata{}, fmt.Errorf("media URL returned HTTP %d", response.StatusCode)
	}
	maxBytes, strictMaximum, err := assetLibraryMediaSizeLimit(assetType)
	if err != nil {
		return AssetMediaMetadata{}, err
	}
	if response.ContentLength >= 0 && assetLibraryMediaSizeExceeded(response.ContentLength, maxBytes, strictMaximum) {
		return AssetMediaMetadata{}, assetLibraryMediaSizeError(assetType)
	}

	temporaryFile, err := os.CreateTemp("", "new-api-asset-media-*")
	if err != nil {
		return AssetMediaMetadata{}, errors.New("media could not be inspected")
	}
	defer os.Remove(temporaryFile.Name())
	defer temporaryFile.Close()

	readLimit := maxBytes + 1
	if strictMaximum {
		readLimit = maxBytes
	}
	written, err := io.Copy(temporaryFile, io.LimitReader(response.Body, readLimit))
	if err != nil {
		return AssetMediaMetadata{}, errors.New("media URL could not be read")
	}
	if assetLibraryMediaSizeExceeded(written, maxBytes, strictMaximum) {
		return AssetMediaMetadata{}, assetLibraryMediaSizeError(assetType)
	}
	if written == 0 {
		return AssetMediaMetadata{}, errors.New("media file is empty")
	}
	if _, err := temporaryFile.Seek(0, io.SeekStart); err != nil {
		return AssetMediaMetadata{}, errors.New("media could not be inspected")
	}

	metadata, err := inspectAssetLibraryMedia(ctx, assetType, temporaryFile, written)
	if err != nil {
		return AssetMediaMetadata{}, err
	}
	if err := validateAssetLibraryMediaMetadata(assetType, metadata); err != nil {
		return AssetMediaMetadata{}, err
	}
	return metadata, nil
}

func inspectAssetLibraryMedia(ctx context.Context, assetType string, input io.ReadSeeker, fileSize int64) (AssetMediaMetadata, error) {
	if err := ctx.Err(); err != nil {
		return AssetMediaMetadata{}, err
	}
	metadata := AssetMediaMetadata{FileSize: fileSize}
	switch assetType {
	case "Image":
		data, err := io.ReadAll(input)
		if err != nil {
			return AssetMediaMetadata{}, errors.New("image could not be read")
		}
		config, format, err := decodeImageConfig(data)
		if err != nil {
			return AssetMediaMetadata{}, errors.New("image format or dimensions could not be detected")
		}
		metadata.Format = strings.ToLower(format)
		metadata.Width = config.Width
		metadata.Height = config.Height
	case "Video":
		readerAt, ok := input.(io.ReaderAt)
		if !ok {
			return AssetMediaMetadata{}, errors.New("video could not be inspected")
		}
		videoMetadata, err := inspectAssetLibraryISOBaseMedia(readerAt, fileSize)
		if err != nil {
			return AssetMediaMetadata{}, err
		}
		metadata.Format = videoMetadata.Format
		metadata.Width = videoMetadata.Width
		metadata.Height = videoMetadata.Height
		metadata.Duration = videoMetadata.Duration
		metadata.FPS = videoMetadata.FPS
	case "Audio":
		format, err := detectAssetLibraryAudioFormat(input)
		if err != nil {
			return AssetMediaMetadata{}, err
		}
		if _, err := input.Seek(0, io.SeekStart); err != nil {
			return AssetMediaMetadata{}, errors.New("audio could not be inspected")
		}
		duration, err := common.GetAudioDuration(ctx, input, "."+format)
		if err != nil {
			return AssetMediaMetadata{}, errors.New("audio duration could not be detected")
		}
		metadata.Format = format
		metadata.Duration = duration
	default:
		return AssetMediaMetadata{}, errors.New("AssetType must be Image, Video, or Audio")
	}
	return metadata, nil
}

func validateAssetLibraryMediaMetadata(assetType string, metadata AssetMediaMetadata) error {
	switch assetType {
	case "Image":
		if !isSupportedAssetLibraryImageFormat(metadata.Format) {
			return errors.New("image format must be jpeg, png, webp, bmp, tiff, gif, heic, or heif")
		}
		if metadata.FileSize <= 0 || metadata.FileSize >= assetLibraryImageMaxBytes {
			return errors.New("image size must be less than 30 MB")
		}
		if metadata.Width <= 300 || metadata.Width >= 6000 || metadata.Height <= 300 || metadata.Height >= 6000 {
			return errors.New("image width and height must each be greater than 300 px and less than 6000 px")
		}
		aspectRatio := float64(metadata.Width) / float64(metadata.Height)
		if aspectRatio <= 0.4 || aspectRatio >= 2.5 {
			return errors.New("image aspect ratio must be greater than 0.4 and less than 2.5")
		}
	case "Video":
		if metadata.Format != "mp4" && metadata.Format != "mov" {
			return errors.New("video format must be mp4 or mov")
		}
		if metadata.FileSize <= 0 || metadata.FileSize > assetLibraryVideoMaxBytes {
			return errors.New("video size must not exceed 200 MB")
		}
		if !isFiniteAssetLibraryNumber(metadata.Duration) || metadata.Duration < assetLibraryMinDuration || metadata.Duration > assetLibraryMaxDuration {
			return errors.New("video duration must be between 2 and 30 seconds")
		}
		if metadata.Width < 300 || metadata.Width > 6000 || metadata.Height < 300 || metadata.Height > 6000 {
			return errors.New("video width and height must each be between 300 px and 6000 px")
		}
		aspectRatio := float64(metadata.Width) / float64(metadata.Height)
		if aspectRatio < 0.4 || aspectRatio > 2.5 {
			return errors.New("video aspect ratio must be between 0.4 and 2.5")
		}
		pixelCount := int64(metadata.Width) * int64(metadata.Height)
		if pixelCount < 407696 || pixelCount > 2086876 {
			return errors.New("video pixel count must be between 407696 and 2086876")
		}
		if !isFiniteAssetLibraryNumber(metadata.FPS) || metadata.FPS < 24 || metadata.FPS > 60 {
			return errors.New("video frame rate must be between 24 and 60 FPS")
		}
	case "Audio":
		if metadata.Format != "wav" && metadata.Format != "mp3" {
			return errors.New("audio format must be wav or mp3")
		}
		if metadata.FileSize <= 0 || metadata.FileSize > assetLibraryAudioMaxBytes {
			return errors.New("audio size must not exceed 15 MB")
		}
		if !isFiniteAssetLibraryNumber(metadata.Duration) || metadata.Duration < assetLibraryMinDuration || metadata.Duration > assetLibraryMaxDuration {
			return errors.New("audio duration must be between 2 and 30 seconds")
		}
	default:
		return errors.New("AssetType must be Image, Video, or Audio")
	}
	return nil
}

func assetLibraryMediaSizeLimit(assetType string) (int64, bool, error) {
	switch assetType {
	case "Image":
		return assetLibraryImageMaxBytes, true, nil
	case "Video":
		return assetLibraryVideoMaxBytes, false, nil
	case "Audio":
		return assetLibraryAudioMaxBytes, false, nil
	default:
		return 0, false, errors.New("AssetType must be Image, Video, or Audio")
	}
}

func assetLibraryMediaSizeExceeded(size int64, maximum int64, strictMaximum bool) bool {
	if strictMaximum {
		return size >= maximum
	}
	return size > maximum
}

func assetLibraryMediaSizeError(assetType string) error {
	switch assetType {
	case "Image":
		return errors.New("image size must be less than 30 MB")
	case "Video":
		return errors.New("video size must not exceed 200 MB")
	case "Audio":
		return errors.New("audio size must not exceed 15 MB")
	default:
		return errors.New("unsupported asset type")
	}
}

func isSupportedAssetLibraryImageFormat(format string) bool {
	switch format {
	case "jpeg", "png", "webp", "bmp", "tiff", "gif", "heic", "heif":
		return true
	default:
		return false
	}
}

func isFiniteAssetLibraryNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func detectAssetLibraryAudioFormat(input io.ReadSeeker) (string, error) {
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return "", errors.New("audio could not be inspected")
	}
	header := make([]byte, 12)
	read, err := io.ReadFull(input, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", errors.New("audio could not be inspected")
	}
	if read >= 12 && string(header[0:4]) == "RIFF" && string(header[8:12]) == "WAVE" {
		return "wav", nil
	}
	if read >= 3 && string(header[0:3]) == "ID3" {
		return "mp3", nil
	}
	if read >= 2 && header[0] == 0xff && header[1]&0xe0 == 0xe0 {
		return "mp3", nil
	}
	return "", errors.New("audio format must be wav or mp3")
}

type assetLibraryISOBox struct {
	boxType     string
	payloadFrom int64
	to          int64
}

type assetLibraryVideoTrack struct {
	handlerType string
	width       int
	height      int
	timescale   uint64
	duration    uint64
	sampleCount uint64
}

func inspectAssetLibraryISOBaseMedia(input io.ReaderAt, fileSize int64) (AssetMediaMetadata, error) {
	metadata := AssetMediaMetadata{}
	foundFormat := false
	for offset := int64(0); offset < fileSize; {
		box, nextOffset, err := readAssetLibraryISOBox(input, offset, fileSize)
		if err != nil {
			return AssetMediaMetadata{}, errors.New("video container could not be parsed")
		}
		switch box.boxType {
		case "ftyp":
			brand, err := readAssetLibraryISOBrand(input, box)
			if err != nil {
				return AssetMediaMetadata{}, errors.New("video format could not be detected")
			}
			format, supported := assetLibraryVideoFormatFromBrand(brand)
			if !supported {
				return AssetMediaMetadata{}, errors.New("video format must be mp4 or mov")
			}
			metadata.Format = format
			foundFormat = true
		case "moov":
			track, ok, err := inspectAssetLibraryMovieBox(input, box)
			if err != nil {
				return AssetMediaMetadata{}, err
			}
			if ok {
				metadata.Width = track.width
				metadata.Height = track.height
				metadata.Duration = float64(track.duration) / float64(track.timescale)
				metadata.FPS = float64(track.sampleCount) / metadata.Duration
			}
		}
		offset = nextOffset
	}
	if !foundFormat || metadata.Width == 0 || metadata.Height == 0 || metadata.Duration <= 0 || metadata.FPS <= 0 {
		return AssetMediaMetadata{}, errors.New("video format, dimensions, duration, or frame rate could not be detected")
	}
	return metadata, nil
}

func inspectAssetLibraryMovieBox(input io.ReaderAt, movieBox assetLibraryISOBox) (assetLibraryVideoTrack, bool, error) {
	for offset := movieBox.payloadFrom; offset < movieBox.to; {
		box, nextOffset, err := readAssetLibraryISOBox(input, offset, movieBox.to)
		if err != nil {
			return assetLibraryVideoTrack{}, false, errors.New("video metadata could not be parsed")
		}
		if box.boxType == "trak" {
			track, err := inspectAssetLibraryTrackBox(input, box)
			if err != nil {
				return assetLibraryVideoTrack{}, false, err
			}
			if track.handlerType == "vide" && track.width > 0 && track.height > 0 && track.timescale > 0 && track.duration > 0 && track.sampleCount > 0 {
				return track, true, nil
			}
		}
		offset = nextOffset
	}
	return assetLibraryVideoTrack{}, false, nil
}

func inspectAssetLibraryTrackBox(input io.ReaderAt, trackBox assetLibraryISOBox) (assetLibraryVideoTrack, error) {
	track := assetLibraryVideoTrack{}
	for offset := trackBox.payloadFrom; offset < trackBox.to; {
		box, nextOffset, err := readAssetLibraryISOBox(input, offset, trackBox.to)
		if err != nil {
			return assetLibraryVideoTrack{}, errors.New("video track metadata could not be parsed")
		}
		switch box.boxType {
		case "tkhd":
			track.width, track.height, err = readAssetLibraryTrackDimensions(input, box)
		case "mdia":
			err = inspectAssetLibraryMediaBox(input, box, &track)
		}
		if err != nil {
			return assetLibraryVideoTrack{}, err
		}
		offset = nextOffset
	}
	return track, nil
}

func inspectAssetLibraryMediaBox(input io.ReaderAt, mediaBox assetLibraryISOBox, track *assetLibraryVideoTrack) error {
	for offset := mediaBox.payloadFrom; offset < mediaBox.to; {
		box, nextOffset, err := readAssetLibraryISOBox(input, offset, mediaBox.to)
		if err != nil {
			return errors.New("video media metadata could not be parsed")
		}
		switch box.boxType {
		case "mdhd":
			track.timescale, track.duration, err = readAssetLibraryMediaDuration(input, box)
		case "hdlr":
			track.handlerType, err = readAssetLibraryHandlerType(input, box)
		case "minf":
			track.sampleCount, err = readAssetLibraryMediaSampleCount(input, box)
		}
		if err != nil {
			return err
		}
		offset = nextOffset
	}
	return nil
}

func readAssetLibraryMediaSampleCount(input io.ReaderAt, mediaInfoBox assetLibraryISOBox) (uint64, error) {
	for offset := mediaInfoBox.payloadFrom; offset < mediaInfoBox.to; {
		box, nextOffset, err := readAssetLibraryISOBox(input, offset, mediaInfoBox.to)
		if err != nil {
			return 0, errors.New("video sample metadata could not be parsed")
		}
		if box.boxType == "stbl" {
			return readAssetLibrarySampleTable(input, box)
		}
		offset = nextOffset
	}
	return 0, nil
}

func readAssetLibrarySampleTable(input io.ReaderAt, sampleTableBox assetLibraryISOBox) (uint64, error) {
	for offset := sampleTableBox.payloadFrom; offset < sampleTableBox.to; {
		box, nextOffset, err := readAssetLibraryISOBox(input, offset, sampleTableBox.to)
		if err != nil {
			return 0, errors.New("video sample table could not be parsed")
		}
		if box.boxType == "stts" {
			return readAssetLibraryTimeToSample(input, box)
		}
		offset = nextOffset
	}
	return 0, nil
}

func readAssetLibraryTimeToSample(input io.ReaderAt, box assetLibraryISOBox) (uint64, error) {
	header := make([]byte, 8)
	if box.to-box.payloadFrom < int64(len(header)) {
		return 0, errors.New("video sample table is invalid")
	}
	if _, err := input.ReadAt(header, box.payloadFrom); err != nil {
		return 0, errors.New("video sample table could not be read")
	}
	entryCount := uint64(binary.BigEndian.Uint32(header[4:8]))
	payloadSize := uint64(box.to - box.payloadFrom)
	if entryCount > (payloadSize-8)/8 {
		return 0, errors.New("video sample table is invalid")
	}
	var sampleCount uint64
	entry := make([]byte, 8)
	for index := uint64(0); index < entryCount; index++ {
		if _, err := input.ReadAt(entry, box.payloadFrom+8+int64(index*8)); err != nil {
			return 0, errors.New("video sample table could not be read")
		}
		count := uint64(binary.BigEndian.Uint32(entry[0:4]))
		if math.MaxUint64-sampleCount < count {
			return 0, errors.New("video sample count is invalid")
		}
		sampleCount += count
	}
	return sampleCount, nil
}

func readAssetLibraryTrackDimensions(input io.ReaderAt, box assetLibraryISOBox) (int, int, error) {
	if box.to-box.payloadFrom < 8 {
		return 0, 0, errors.New("video dimensions are invalid")
	}
	data := make([]byte, 8)
	if _, err := input.ReadAt(data, box.to-8); err != nil {
		return 0, 0, errors.New("video dimensions could not be read")
	}
	return int(binary.BigEndian.Uint32(data[0:4]) >> 16), int(binary.BigEndian.Uint32(data[4:8]) >> 16), nil
}

func readAssetLibraryMediaDuration(input io.ReaderAt, box assetLibraryISOBox) (uint64, uint64, error) {
	version := make([]byte, 1)
	if _, err := input.ReadAt(version, box.payloadFrom); err != nil {
		return 0, 0, errors.New("video duration could not be read")
	}
	if version[0] == 0 {
		data := make([]byte, 20)
		if box.to-box.payloadFrom < int64(len(data)) {
			return 0, 0, errors.New("video duration metadata is invalid")
		}
		if _, err := input.ReadAt(data, box.payloadFrom); err != nil {
			return 0, 0, errors.New("video duration could not be read")
		}
		return uint64(binary.BigEndian.Uint32(data[12:16])), uint64(binary.BigEndian.Uint32(data[16:20])), nil
	}
	if version[0] == 1 {
		data := make([]byte, 32)
		if box.to-box.payloadFrom < int64(len(data)) {
			return 0, 0, errors.New("video duration metadata is invalid")
		}
		if _, err := input.ReadAt(data, box.payloadFrom); err != nil {
			return 0, 0, errors.New("video duration could not be read")
		}
		return uint64(binary.BigEndian.Uint32(data[20:24])), binary.BigEndian.Uint64(data[24:32]), nil
	}
	return 0, 0, errors.New("video duration metadata version is unsupported")
}

func readAssetLibraryHandlerType(input io.ReaderAt, box assetLibraryISOBox) (string, error) {
	data := make([]byte, 12)
	if box.to-box.payloadFrom < int64(len(data)) {
		return "", errors.New("video handler metadata is invalid")
	}
	if _, err := input.ReadAt(data, box.payloadFrom); err != nil {
		return "", errors.New("video handler metadata could not be read")
	}
	return string(data[8:12]), nil
}

func readAssetLibraryISOBrand(input io.ReaderAt, box assetLibraryISOBox) (string, error) {
	if box.to-box.payloadFrom < 8 {
		return "", errors.New("invalid file type box")
	}
	brand := make([]byte, 4)
	if _, err := input.ReadAt(brand, box.payloadFrom); err != nil {
		return "", err
	}
	return string(brand), nil
}

func assetLibraryVideoFormatFromBrand(brand string) (string, bool) {
	switch brand {
	case "qt  ":
		return "mov", true
	case "isom", "iso2", "iso3", "iso4", "iso5", "iso6",
		"mp41", "mp42", "avc1", "hvc1", "hev1", "av01", "vp09",
		"dash", "cmfc", "cmfs", "M4V ", "F4V ", "MSNV":
		return "mp4", true
	default:
		return "", false
	}
}

func readAssetLibraryISOBox(input io.ReaderAt, from int64, parentTo int64) (assetLibraryISOBox, int64, error) {
	if from < 0 || parentTo-from < 8 {
		return assetLibraryISOBox{}, 0, errors.New("invalid box header")
	}
	header := make([]byte, 8)
	if _, err := input.ReadAt(header, from); err != nil {
		return assetLibraryISOBox{}, 0, err
	}
	boxSize := uint64(binary.BigEndian.Uint32(header[0:4]))
	headerSize := uint64(8)
	if boxSize == 1 {
		extendedSize := make([]byte, 8)
		if parentTo-from < 16 {
			return assetLibraryISOBox{}, 0, errors.New("invalid extended box header")
		}
		if _, err := input.ReadAt(extendedSize, from+8); err != nil {
			return assetLibraryISOBox{}, 0, err
		}
		boxSize = binary.BigEndian.Uint64(extendedSize)
		headerSize = 16
	} else if boxSize == 0 {
		boxSize = uint64(parentTo - from)
	}
	if boxSize < headerSize || boxSize > uint64(parentTo-from) {
		return assetLibraryISOBox{}, 0, errors.New("invalid box size")
	}
	to := from + int64(boxSize)
	return assetLibraryISOBox{
		boxType:     string(header[4:8]),
		payloadFrom: from + int64(headerSize),
		to:          to,
	}, to, nil
}
