package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/relayparam"
	"github.com/gin-gonic/gin"
)

// Limits apply across fields and channel retries in one request. This feature
// downloads locally; it does not send arbitrary client URLs or credentials to a worker.
const parameterMediaMaxBytes int64 = 64 << 20
const parameterMediaMaxDownloads = 16

type parameterMediaCache struct {
	data      map[string]string
	bytes     int64
	downloads int
	deadline  time.Time
}

// NewParameterMediaTransformer creates a lazy request-scoped downloader. No I/O
// or cache allocation occurs for requests without a matching remote media field.
func NewParameterMediaTransformer(c *gin.Context) relayparam.MediaTransformer {
	return func(source, mediaType string) (string, error) {
		if c == nil || c.Request == nil {
			return "", errors.New("request context is unavailable")
		}
		const cacheKey = "parameter_media_downloads"
		cached, exists := c.Get(cacheKey)
		var cache *parameterMediaCache
		if exists {
			cache = cached.(*parameterMediaCache)
		} else {
			cache = &parameterMediaCache{data: make(map[string]string), deadline: time.Now().Add(60 * time.Second)}
			c.Set(cacheKey, cache)
		}
		key := mediaType + ":" + source
		if data, ok := cache.data[key]; ok {
			return data, nil
		}
		if cache.downloads >= parameterMediaMaxDownloads {
			return "", errors.New("too many media downloads in one request")
		}
		remaining := parameterMediaMaxBytes - cache.bytes
		fileLimit := int64(constant.MaxFileDownloadMB) * 1024 * 1024
		if fileLimit <= 0 {
			return "", errors.New("media downloads are disabled by the size limit")
		}
		if remaining > fileLimit {
			remaining = fileLimit
		}
		if remaining <= 0 {
			return "", errors.New("media download size limit exceeded")
		}
		cache.downloads++
		ctx, cancel := context.WithDeadline(c.Request.Context(), cache.deadline)
		defer cancel()
		if err := ctx.Err(); err != nil {
			return "", errors.New("media download cancelled or timed out")
		}
		if err := ValidateSSRFProtectedFetchURL(source); err != nil {
			return "", errors.New("media URL blocked by fetch policy")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return "", errors.New("invalid media URL")
		}
		resp, err := GetSSRFProtectedHTTPClient().Do(req)
		if err != nil {
			return "", errors.New("media download failed or timed out")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("media server returned HTTP %d", resp.StatusCode)
		}
		if resp.ContentLength > remaining {
			return "", errors.New("media download size limit exceeded")
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, remaining+1))
		cache.bytes += int64(len(body))
		if err != nil {
			return "", errors.New("failed to read media response")
		}
		if int64(len(body)) > remaining {
			return "", errors.New("media download size limit exceeded")
		}
		if len(body) == 0 {
			return "", errors.New("media response is empty")
		}
		// Prefer the actual bytes; use the existing MIME detector for formats
		// without a standard-library signature, but never accept HTML/text as media.
		mimeType, _, _ := mime.ParseMediaType(http.DetectContentType(body))
		if mimeType == "application/octet-stream" || mimeType == "application/ogg" {
			mimeType, _, _ = mime.ParseMediaType(smartDetectMimeType(resp, source, body))
		}
		// MP4 and WebM containers can carry audio-only streams.
		declared, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if mediaType == "audio" && ((mimeType == "video/mp4" && declared == "audio/mp4") || (mimeType == "video/webm" && declared == "audio/webm")) {
			mimeType = declared
		}
		if !strings.HasPrefix(mimeType, mediaType+"/") {
			return "", fmt.Errorf("downloaded content is not %s media", mediaType)
		}
		data := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(body)
		cache.data[key] = data
		return data, nil
	}
}
