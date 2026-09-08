package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// LoadAssetLibraryImagePreview fetches only an already-owned asset's source.
// Validate the URL again because DNS and redirects may have changed since upload.
func LoadAssetLibraryImagePreview(ctx context.Context, sourceURL string) ([]byte, string, error) {
	if err := ValidateSSRFProtectedFetchURL(sourceURL); err != nil {
		return nil, "", errors.New("image preview URL is not allowed")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, "", errors.New("invalid image preview URL")
	}
	response, err := GetSSRFProtectedHTTPClient().Do(request)
	if err != nil {
		return nil, "", errors.New("image preview could not be downloaded")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength >= assetLibraryImageMaxBytes {
		return nil, "", errors.New("image preview is unavailable or too large")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, assetLibraryImageMaxBytes))
	if err != nil || len(data) == 0 || int64(len(data)) >= assetLibraryImageMaxBytes {
		return nil, "", errors.New("image preview is unavailable or too large")
	}
	// Never serve upstream HTML or SVG on the application's origin, even if the
	// remote Content-Type header or persisted asset type claims it is an image.
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/x-icon":
	default:
		return nil, "", errors.New("image preview format is unsupported")
	}
	if _, _, err := decodeImageConfig(data); err != nil {
		return nil, "", errors.New("image preview is invalid")
	}
	return data, contentType, nil
}
