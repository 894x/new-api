package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/relayparam"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type videoInspection struct {
	info relayparam.MediaInfo
	err  error
}

type videoMediaRequest struct {
	c             *gin.Context
	inspections   map[string]videoInspection
	converted     map[string]string
	probeDeadline time.Time
	uploadedBytes int64
	descriptions  map[videoDescriptionKey]videoDescription
}

// The pointer pins the immutable request body for this request only. Rewritten
// outbound bodies have a different identity and are always validated afresh.
type videoDescriptionKey struct {
	first  *byte
	length int
	path   string
}
type videoDescription struct {
	inputs []relayparam.MediaInput
	err    error
}

func (m *videoMediaRequest) Describe(data []byte, path string) ([]relayparam.MediaInput, error) {
	if len(data) == 0 {
		return nil, nil
	}
	key := videoDescriptionKey{&data[0], len(data), path}
	if cached, ok := m.descriptions[key]; ok {
		return cached.inputs, cached.err
	}
	inputs, err := relayparam.DescribeMediaInputs(data, path, m)
	if m.descriptions == nil {
		m.descriptions = make(map[videoDescriptionKey]videoDescription)
	}
	m.descriptions[key] = videoDescription{inputs, err}
	return inputs, err
}

func requestVideoMedia(c *gin.Context) *videoMediaRequest {
	const key = "video_media_request"
	if c != nil {
		if value, ok := c.Get(key); ok {
			return value.(*videoMediaRequest)
		}
	}
	media := &videoMediaRequest{c: c, inspections: make(map[string]videoInspection), converted: make(map[string]string), probeDeadline: time.Now().Add(10 * time.Second)}
	if c != nil {
		c.Set(key, media)
	}
	return media
}

func (m *videoMediaRequest) Inspect(source string) (relayparam.MediaInfo, error) {
	if cached, ok := m.inspections[source]; ok {
		return cached.info, cached.err
	}
	if len(m.inspections) >= parameterMediaMaxDownloads {
		return relayparam.MediaInfo{}, errors.New("too many distinct video sources")
	}
	info, err := m.inspect(source)
	m.inspections[source] = videoInspection{info, err}
	return info, err
}

func (m *videoMediaRequest) inspect(source string) (relayparam.MediaInfo, error) {
	if strings.HasPrefix(source, "data:") {
		return relayparam.Base64VideoInfo(source)
	}
	info := relayparam.MediaInfo{Format: "url", Bytes: -1}
	parsed, err := url.Parse(source)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		return info, errors.New("video source must be an HTTP(S) URL without credentials or a video Base64 data URI")
	}
	if m.c == nil || m.c.Request == nil {
		return info, errors.New("request context is unavailable")
	}
	if err := ValidateSSRFProtectedFetchURL(source); err != nil {
		return info, errors.New("video URL blocked by fetch policy")
	}
	deadline := time.Now().Add(2 * time.Second)
	if m.probeDeadline.Before(deadline) {
		deadline = m.probeDeadline
	}
	ctx, cancel := context.WithDeadline(m.c.Request.Context(), deadline)
	defer cancel()
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		if ctx.Err() != nil {
			break
		}
		req, err := http.NewRequestWithContext(ctx, method, source, nil)
		if err != nil {
			break
		}
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("User-Agent", "new-api/1.0 (media metadata)")
		if method == http.MethodGet {
			req.Header.Set("Range", "bytes=0-0")
		}
		resp, err := GetSSRFProtectedHTTPClient().Do(req)
		if err != nil {
			continue
		}
		// Never drain a body: some hosts ignore Range and return the entire video.
		resp.Body.Close()
		if resp.Header.Get("Content-Encoding") != "" && resp.Header.Get("Content-Encoding") != "identity" {
			continue
		}
		if resp.StatusCode == http.StatusPartialContent {
			prefix, total, ok := strings.Cut(resp.Header.Get("Content-Range"), "/")
			if ok && prefix == "bytes 0-0" {
				if size, err := strconv.ParseInt(total, 10, 64); err == nil && size > 0 {
					info.Bytes = size
					return info, nil
				}
			}
		} else if resp.StatusCode == http.StatusOK && resp.ContentLength >= 0 {
			info.Bytes = resp.ContentLength
			return info, nil
		}
	}
	// Missing or inaccessible metadata is unknown, never a zero-byte video.
	return info, nil
}

func (m *videoMediaRequest) Convert(source, target string, limit int64) (string, error) {
	info, err := m.Inspect(source)
	if err != nil {
		return "", err
	}
	if target == "base64" {
		encoded, err := NewParameterMediaTransformer(m.c)(source, "video")
		if err != nil {
			return "", err
		}
		actual, err := relayparam.Base64VideoInfo(encoded)
		if err != nil {
			return "", err
		}
		// Replace declared metadata with the bytes actually downloaded, including on retries.
		m.inspections[source] = videoInspection{info: relayparam.MediaInfo{Format: "url", Bytes: actual.Bytes, Exact: true}}
		for _, description := range m.descriptions {
			for i := range description.inputs {
				if description.inputs[i].Source == source {
					description.inputs[i].Info = m.inspections[source].info
				}
			}
		}
		if limit > 0 && actual.Bytes > limit {
			return "", relayparam.ErrActualMediaSize
		}
		return encoded, nil
	}
	if target != "url" || info.Format != "base64" {
		return "", errors.New("unsupported video conversion")
	}
	if limit > 0 && info.Bytes > limit {
		return "", errors.New("video exceeds the channel media size limit")
	}
	if cached, ok := m.converted[source]; ok {
		return cached, nil
	}
	if info.Bytes > parameterMediaMaxBytes-m.uploadedBytes {
		return "", errors.New("video upload request exceeds 64 MiB")
	}
	config, err := system_setting.LoadAssetStorageConfig()
	if err != nil || !config.Enabled {
		return "", errors.New("video URL conversion requires configured platform object storage")
	}
	header, payload, _ := strings.Cut(source, ",")
	encoding := base64.StdEncoding
	if !strings.HasSuffix(payload, "=") {
		encoding = base64.RawStdEncoding
	}
	body, err := encoding.DecodeString(payload)
	if err != nil {
		return "", errors.New("invalid video Base64")
	}
	contentType := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	if actualType := http.DetectContentType(body); !strings.HasPrefix(actualType, "video/") || actualType != contentType {
		return "", errors.New("Base64 bytes do not match the declared video MIME type")
	}
	m.uploadedBytes += int64(len(body))
	checksum := sha256.Sum256(body)
	id := common.GetUUID()
	object := &model.AssetStoredObject{Id: id, Bucket: config.Bucket, Region: config.Region, ObjectKey: "relay-media/" + id, FileSize: int64(len(body)), ContentType: contentType, SHA256: hex.EncodeToString(checksum[:])}
	ledger := &model.RelayMediaObject{ID: id, Bucket: object.Bucket, Region: object.Region, ObjectKey: object.ObjectKey, ExpiresAt: time.Now().Add(config.URLLifetime + time.Hour).Unix()}
	if err := model.DB.Create(ledger).Error; err != nil {
		return "", errors.New("cannot schedule temporary video cleanup")
	}
	ctx, cancel := context.WithTimeout(m.c.Request.Context(), 60*time.Second)
	defer cancel()
	store := assetObjectStoreFactory(config)
	if err := store.Put(ctx, object, bytes.NewReader(body)); err != nil {
		return "", errors.New("temporary video upload failed")
	}
	signed, err := store.Sign(ctx, object)
	if err != nil {
		return "", errors.New("temporary video URL signing failed")
	}
	m.converted[source] = signed
	return signed, nil
}

// Cleanup is retryable across restarts; concurrent deletions are idempotent.
func DeleteExpiredRelayMedia(ctx context.Context) error {
	config, err := system_setting.LoadAssetStorageConfig()
	if err != nil || !config.Enabled {
		return err
	}
	var objects []model.RelayMediaObject
	if err := model.DB.Where("expires_at <= ?", time.Now().Unix()).Order("expires_at").Limit(100).Find(&objects).Error; err != nil {
		return err
	}
	for _, object := range objects {
		deleteCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := assetObjectStoreFactory(config).Delete(deleteCtx, &model.AssetStoredObject{Bucket: object.Bucket, Region: object.Region, ObjectKey: object.ObjectKey})
		cancel()
		if err != nil {
			return err
		}
		if err := model.DB.Delete(&object).Error; err != nil {
			return err
		}
	}
	return nil
}
