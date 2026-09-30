package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const seedanceVideoURLPath = "/v1/seedance-media/"
const seedanceVideoUploadGrace = 15 * time.Minute

var seedanceVideoStorageMu sync.Mutex

// ConvertSeedanceBase64Media rewrites Seedance image_url and video_url content
// entries before media validation and asset-library synchronization.
func ConvertSeedanceBase64Media(ctx context.Context, payload map[string]any, enabled bool) (map[string]any, error) {
	content, ok := payload["content"].([]any)
	if !ok {
		return payload, nil
	}
	converted := make(map[string]string)
	result := make([]any, len(content))
	copy(result, content)
	changed := false
	for i, value := range content {
		item, ok := value.(map[string]any)
		if !ok || item["type"] != "video_url" && item["type"] != "image_url" {
			continue
		}
		field, ok := item["type"].(string)
		if !ok {
			continue
		}
		media, ok := item[field].(map[string]any)
		if !ok {
			continue
		}
		source, ok := media["url"].(string)
		if !ok || source == "" || strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "asset://") {
			continue
		}
		if !enabled {
			return nil, fmt.Errorf("content[%d].%s: channel base64 media conversion is disabled", i, field)
		}
		assetType := "Image"
		if field == "video_url" {
			assetType = "Video"
		}
		conversionKey := assetType + "\x00" + source
		publicURL := converted[conversionKey]
		if publicURL == "" {
			var err error
			publicURL, err = StoreSeedanceBase64Media(ctx, source, assetType)
			if err != nil {
				return nil, fmt.Errorf("content[%d].%s: %w", i, field, err)
			}
			converted[conversionKey] = publicURL
		}
		copiedMedia := make(map[string]any, len(media))
		for key, child := range media {
			copiedMedia[key] = child
		}
		copiedMedia["url"] = publicURL
		copiedItem := make(map[string]any, len(item))
		for key, child := range item {
			copiedItem[key] = child
		}
		copiedItem[field] = copiedMedia
		result[i] = copiedItem
		changed = true
	}
	if !changed {
		return payload, nil
	}
	prepared := make(map[string]any, len(payload))
	for key, child := range payload {
		prepared[key] = child
	}
	prepared["content"] = result
	return prepared, nil
}

// ConvertSeedanceBase64Videos is kept for callers that use the old name. It
// now converts both image_url and video_url media entries.
func ConvertSeedanceBase64Videos(ctx context.Context, payload map[string]any, enabled bool) (map[string]any, error) {
	return ConvertSeedanceBase64Media(ctx, payload, enabled)
}

// StoreSeedanceBase64Media keeps a bounded, locally served staging copy until
// the upstream asset library downloads it. Files are named with opaque IDs.
func StoreSeedanceBase64Media(ctx context.Context, source, assetType string) (string, error) {
	encoded, contentType, err := parseSeedanceBase64Media(source, assetType)
	if err != nil {
		return "", err
	}
	maxBytes, strictMaximum, maxMB, err := seedanceBase64MediaLimit(assetType)
	if err != nil {
		return "", err
	}
	if encoded == "" || int64(len(encoded)) > (maxBytes+2)/3*4+4 {
		return "", fmt.Errorf("%s exceeds the configured storage limit", strings.ToLower(assetType))
	}
	baseAddress := strings.TrimSpace(system_setting.TaskPublicAddress)
	if baseAddress == "" {
		baseAddress = strings.TrimSpace(system_setting.ServerAddress)
	}
	if err := ValidateTaskArtifactBaseURL(baseAddress); err != nil {
		return "", fmt.Errorf("public media address: %w", err)
	}
	baseURL, err := url.Parse(baseAddress)
	if err != nil {
		return "", err
	}
	host := strings.ToLower(baseURL.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return "", fmt.Errorf("configure a publicly reachable TaskPublicAddress for Seedance %ss", strings.ToLower(assetType))
	}
	if address := net.ParseIP(host); address != nil && (address.IsPrivate() || address.IsLoopback() || address.IsUnspecified() || address.IsLinkLocalUnicast()) {
		return "", fmt.Errorf("configure a publicly reachable TaskPublicAddress for Seedance %ss", strings.ToLower(assetType))
	}
	seedanceVideoStorageMu.Lock()
	defer seedanceVideoStorageMu.Unlock()
	directory := seedanceVideoDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", fmt.Errorf("could not create Seedance %s directory", strings.ToLower(assetType))
	}
	temporary, err := os.CreateTemp(directory, ".upload-*")
	if err != nil {
		return "", fmt.Errorf("could not stage Seedance %s", strings.ToLower(assetType))
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	encoding := base64.StdEncoding.Strict()
	if !strings.HasSuffix(encoded, "=") {
		encoding = base64.RawStdEncoding.Strict()
	}
	written, err := io.Copy(temporary, io.LimitReader(base64.NewDecoder(encoding, strings.NewReader(encoded)), maxBytes+1))
	if err != nil || written == 0 {
		return "", fmt.Errorf("invalid %s base64", strings.ToLower(assetType))
	}
	if assetLibraryMediaSizeExceeded(written, maxBytes, strictMaximum) {
		return "", assetLibraryMediaSizeError(assetType)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	metadata, err := inspectAssetLibraryMedia(ctx, assetType, temporary, written)
	if err != nil {
		return "", err
	}
	if assetType == "Image" {
		if err := validateAssetLibraryMediaMetadata(assetType, metadata); err != nil {
			return "", err
		}
	} else if metadata.Format != "mp4" && metadata.Format != "mov" {
		return "", errors.New("video bytes do not match the declared format")
	}
	if !seedanceDeclaredMediaMatches(contentType, assetType, metadata.Format) {
		return "", fmt.Errorf("%s bytes do not match the declared format", strings.ToLower(assetType))
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	name := common.GetUUID() + "." + metadata.Format
	if err := os.Rename(temporary.Name(), filepath.Join(directory, name)); err != nil {
		return "", fmt.Errorf("could not publish Seedance %s", strings.ToLower(assetType))
	}
	if err := pruneSeedanceMedia(directory, maxMB*system_setting.AssetQuotaMB, name); err != nil {
		_ = os.Remove(filepath.Join(directory, name))
		return "", err
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + seedanceVideoURLPath + name
	baseURL.RawPath = ""
	return baseURL.String(), nil
}

// StoreSeedanceBase64Video keeps the existing video-only API for callers that
// do not need to select an asset type.
func StoreSeedanceBase64Video(ctx context.Context, source string) (string, error) {
	return StoreSeedanceBase64Media(ctx, source, "Video")
}

func StoreSeedanceBase64Image(ctx context.Context, source string) (string, error) {
	return StoreSeedanceBase64Media(ctx, source, "Image")
}

func parseSeedanceBase64Media(source, assetType string) (string, string, error) {
	encoded := source
	contentType := ""
	if strings.HasPrefix(source, "data:") {
		header, data, ok := strings.Cut(source, ",")
		if !ok || !strings.HasSuffix(header, ";base64") {
			return "", "", fmt.Errorf("%s must use base64 encoding", strings.ToLower(assetType))
		}
		contentType = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64"))
		if assetType == "Video" && contentType != "video/mp4" && contentType != "video/quicktime" && contentType != "application/octet-stream" {
			return "", "", errors.New("video must be MP4 or MOV")
		}
		if assetType == "Image" && !strings.HasPrefix(contentType, "image/") && contentType != "application/octet-stream" {
			return "", "", errors.New("image must use an image MIME type")
		}
		encoded = data
	}
	return encoded, contentType, nil
}

func seedanceBase64MediaLimit(assetType string) (int64, bool, int64, error) {
	if assetType == "Image" {
		return assetLibraryImageMaxBytes, true, assetLibraryImageMaxBytes / system_setting.AssetQuotaMB, nil
	}
	maxMB := system_setting.GetAssetStorageSetting().SeedanceVideoMaxMB
	if maxMB < 1 || maxMB > system_setting.MaxAssetQuotaMB {
		return 0, false, 0, errors.New("Seedance video storage limit is invalid")
	}
	limit := maxMB * system_setting.AssetQuotaMB
	if limit > assetLibraryVideoMaxBytes {
		limit = assetLibraryVideoMaxBytes
	}
	return limit, false, maxMB, nil
}

func seedanceDeclaredMediaMatches(contentType, assetType, format string) bool {
	if contentType == "" || contentType == "application/octet-stream" {
		return true
	}
	if assetType == "Video" {
		return contentType == "video/mp4" && format == "mp4" || contentType == "video/quicktime" && format == "mov"
	}
	if !strings.HasPrefix(contentType, "image/") {
		return false
	}
	declared := strings.TrimPrefix(contentType, "image/")
	if declared == "jpg" {
		declared = "jpeg"
	}
	return declared == format
}

func seedanceVideoDirectory() string {
	if configured := strings.TrimSpace(os.Getenv("SEEDANCE_VIDEO_DIR")); configured != "" {
		return configured
	}
	return filepath.Join("data", "seedance-video")
}

func pruneSeedanceVideos(directory string, limit int64, current string) error {
	return pruneSeedanceMedia(directory, limit, current)
}

func pruneSeedanceMedia(directory string, limit int64, current string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	type storedMedia struct {
		name    string
		size    int64
		modTime int64
	}
	files := make([]storedMedia, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !validSeedanceMediaName(entry.Name()) {
			if strings.HasPrefix(entry.Name(), ".upload-") {
				if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > seedanceVideoUploadGrace {
					_ = os.Remove(filepath.Join(directory, entry.Name()))
				}
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, storedMedia{entry.Name(), info.Size(), info.ModTime().UnixNano()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime < files[j].modTime })
	for _, file := range files {
		if total <= limit {
			break
		}
		if file.name == current || time.Since(time.Unix(0, file.modTime)) < seedanceVideoUploadGrace {
			continue
		}
		if err := os.Remove(filepath.Join(directory, file.name)); err != nil {
			return err
		}
		total -= file.size
	}
	if total > limit {
		return errors.New("Seedance media storage limit cannot accommodate this upload")
	}
	return nil
}

func validSeedanceVideoName(name string) bool {
	return validSeedanceMediaNameWithExtensions(name, ".mp4", ".mov")
}

func validSeedanceMediaName(name string) bool {
	return validSeedanceMediaNameWithExtensions(name, ".mp4", ".mov", ".jpeg", ".png", ".webp", ".bmp", ".tiff", ".gif", ".heic", ".heif")
}

func validSeedanceMediaNameWithExtensions(name string, extensions ...string) bool {
	stem, extension := "", ""
	for _, candidate := range extensions {
		if candidateStem, ok := strings.CutSuffix(name, candidate); ok {
			stem, extension = candidateStem, candidate
			break
		}
	}
	if extension == "" || len(stem) != 32 {
		return false
	}
	for _, char := range stem {
		if char < '0' || char > '9' && char < 'a' || char > 'f' {
			return false
		}
	}
	return true
}

func OpenSeedanceVideo(name string) (*os.File, error) {
	if !validSeedanceVideoName(name) {
		return nil, os.ErrNotExist
	}
	file, _, err := OpenSeedanceMedia(name)
	return file, err
}

func OpenSeedanceMedia(name string) (*os.File, string, error) {
	if !validSeedanceMediaName(name) {
		return nil, "", os.ErrNotExist
	}
	path := filepath.Join(seedanceVideoDirectory(), name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, "", os.ErrNotExist
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return file, contentType, nil
}
