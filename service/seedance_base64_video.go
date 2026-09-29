package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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

// ConvertSeedanceBase64Videos rewrites only Seedance video_url content entries.
// Conversion runs before media validation and asset-library synchronization.
func ConvertSeedanceBase64Videos(ctx context.Context, payload map[string]any, enabled bool) (map[string]any, error) {
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
		if !ok || item["type"] != "video_url" {
			continue
		}
		media, ok := item["video_url"].(map[string]any)
		if !ok {
			continue
		}
		source, ok := media["url"].(string)
		if !ok || source == "" || strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "asset://") {
			continue
		}
		if !enabled {
			return nil, fmt.Errorf("content[%d].video_url: channel base64 video conversion is disabled", i)
		}
		publicURL := converted[source]
		if publicURL == "" {
			var err error
			publicURL, err = StoreSeedanceBase64Video(ctx, source)
			if err != nil {
				return nil, fmt.Errorf("content[%d].video_url: %w", i, err)
			}
			converted[source] = publicURL
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
		copiedItem["video_url"] = copiedMedia
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

// StoreSeedanceBase64Video keeps a bounded, locally served staging copy until
// the upstream asset library downloads it. Files are named with opaque IDs.
func StoreSeedanceBase64Video(ctx context.Context, source string) (string, error) {
	encoded := source
	contentType := ""
	if strings.HasPrefix(source, "data:") {
		header, data, ok := strings.Cut(source, ",")
		if !ok || !strings.HasSuffix(header, ";base64") {
			return "", errors.New("video must use base64 encoding")
		}
		contentType = strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
		if contentType != "video/mp4" && contentType != "video/quicktime" && contentType != "application/octet-stream" {
			return "", errors.New("video must be MP4 or MOV")
		}
		encoded = data
	}
	maxMB := system_setting.GetAssetStorageSetting().SeedanceVideoMaxMB
	if maxMB < 1 || maxMB > system_setting.MaxAssetQuotaMB {
		return "", errors.New("Seedance video storage limit is invalid")
	}
	limit := maxMB * system_setting.AssetQuotaMB
	if limit > assetLibraryVideoMaxBytes {
		limit = assetLibraryVideoMaxBytes
	}
	if encoded == "" || int64(len(encoded)) > (limit+2)/3*4+4 {
		return "", errors.New("video exceeds the configured storage limit")
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
		return "", errors.New("configure a publicly reachable TaskPublicAddress for Seedance videos")
	}
	if address := net.ParseIP(host); address != nil && (address.IsPrivate() || address.IsLoopback() || address.IsUnspecified() || address.IsLinkLocalUnicast()) {
		return "", errors.New("configure a publicly reachable TaskPublicAddress for Seedance videos")
	}
	seedanceVideoStorageMu.Lock()
	defer seedanceVideoStorageMu.Unlock()
	directory := seedanceVideoDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", errors.New("could not create Seedance video directory")
	}
	temporary, err := os.CreateTemp(directory, ".upload-*")
	if err != nil {
		return "", errors.New("could not stage Seedance video")
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	encoding := base64.StdEncoding.Strict()
	if !strings.HasSuffix(encoded, "=") {
		encoding = base64.RawStdEncoding.Strict()
	}
	written, err := io.Copy(temporary, io.LimitReader(base64.NewDecoder(encoding, strings.NewReader(encoded)), limit+1))
	if err != nil || written == 0 {
		return "", errors.New("invalid video base64")
	}
	if written > limit {
		return "", errors.New("video exceeds the configured storage limit")
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	metadata, err := inspectAssetLibraryMedia(ctx, "Video", temporary, written)
	if err != nil || metadata.Format != "mp4" && metadata.Format != "mov" ||
		contentType == "video/mp4" && metadata.Format != "mp4" ||
		contentType == "video/quicktime" && metadata.Format != "mov" {
		return "", errors.New("video bytes do not match the declared format")
	}
	extension := "." + metadata.Format
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	name := common.GetUUID() + extension
	if err := os.Rename(temporary.Name(), filepath.Join(directory, name)); err != nil {
		return "", errors.New("could not publish Seedance video")
	}
	if err := pruneSeedanceVideos(directory, maxMB*system_setting.AssetQuotaMB, name); err != nil {
		_ = os.Remove(filepath.Join(directory, name))
		return "", err
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + seedanceVideoURLPath + name
	baseURL.RawPath = ""
	return baseURL.String(), nil
}

func seedanceVideoDirectory() string {
	if configured := strings.TrimSpace(os.Getenv("SEEDANCE_VIDEO_DIR")); configured != "" {
		return configured
	}
	return filepath.Join("data", "seedance-video")
}

func pruneSeedanceVideos(directory string, limit int64, current string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	type storedVideo struct {
		name    string
		size    int64
		modTime int64
	}
	files := make([]storedVideo, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !validSeedanceVideoName(entry.Name()) {
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
		files = append(files, storedVideo{entry.Name(), info.Size(), info.ModTime().UnixNano()})
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
		return errors.New("Seedance video storage limit cannot accommodate this upload")
	}
	return nil
}

func validSeedanceVideoName(name string) bool {
	stem, ext := strings.CutSuffix(name, ".mp4")
	if !ext {
		stem, ext = strings.CutSuffix(name, ".mov")
	}
	if !ext || len(stem) != 32 {
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
	path := filepath.Join(seedanceVideoDirectory(), name)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	return os.Open(path)
}
