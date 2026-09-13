package service

import (
	"bytes"
	"context"
	"hash/crc64"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cosTestTransport func(*http.Request) (*http.Response, error)

func (f cosTestTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestCOSSDKUploadsVerifiedBytesAndSignsPrivateDownloads(t *testing.T) {
	data := []byte("a verified original")
	object := &model.AssetStoredObject{Bucket: "references-123456", Region: "ap-guangzhou", ObjectKey: "assets/1/image/digest", FileSize: int64(len(data)), SHA256: "digest", ContentType: "image/png"}
	puts, heads, deletes := 0, 0, 0
	store := &cosAssetObjectStore{config: system_setting.AssetStorageConfig{SecretID: "test-secret-id", SecretKey: "test-secret-key", SessionToken: "test-token", URLLifetime: time.Hour}}
	store.transport = cosTestTransport(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "references-123456.cos.ap-guangzhou.myqcloud.com", request.URL.Host)
		assert.Contains(t, request.Header.Get("Authorization"), "q-signature=")
		assert.Equal(t, "test-token", request.Header.Get("x-cos-security-token"))
		response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(nil)), Request: request}
		switch request.Method {
		case http.MethodHead:
			heads++
			if puts == 0 {
				response.StatusCode = 404
			} else {
				response.ContentLength = int64(len(data))
				response.Header.Set("x-cos-meta-sha256", "digest")
			}
		case http.MethodPut:
			puts++
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			assert.Equal(t, data, body)
			assert.Equal(t, "digest", request.Header.Get("x-cos-meta-sha256"))
			response.Header.Set("x-cos-hash-crc64ecma", strconv.FormatUint(crc64.Checksum(data, crc64.MakeTable(crc64.ECMA)), 10))
		case http.MethodGet:
			assert.Equal(t, "bytes=2-4", request.Header.Get("Range"))
			response.StatusCode = http.StatusPartialContent
			response.Body = io.NopCloser(bytes.NewReader(data[2:5]))
		case http.MethodDelete:
			deletes++
			response.StatusCode = 204
		}
		return response, nil
	})
	require.NoError(t, store.Put(context.Background(), object, bytes.NewReader(data)))
	require.NoError(t, store.Put(context.Background(), object, bytes.NewReader(data)))
	assert.Equal(t, 1, puts, "recovery must reuse an already verified object")
	assert.Equal(t, 3, heads)
	signed, err := store.Sign(context.Background(), object)
	require.NoError(t, err)
	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	assert.Equal(t, "/"+object.ObjectKey, parsed.Path)
	assert.NotEmpty(t, parsed.Query().Get("q-signature"))
	assert.Equal(t, "test-token", parsed.Query().Get("x-cos-security-token"))
	assert.NotContains(t, signed, "test-secret-key")
	response, err := store.Get(context.Background(), object, "bytes=2-4")
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, 206, response.StatusCode)
	require.NoError(t, store.Delete(context.Background(), object))
	assert.Equal(t, 1, deletes)
}

func TestCOSSDKRejectsAnExistingObjectWithDifferentContent(t *testing.T) {
	store := &cosAssetObjectStore{config: system_setting.AssetStorageConfig{SecretID: "id", SecretKey: "secret"}}
	store.transport = cosTestTransport(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodHead, request.Method, "an inconsistent original must never be overwritten")
		return &http.Response{StatusCode: 200, ContentLength: 1, Header: http.Header{"X-Cos-Meta-Sha256": []string{"wrong"}}, Body: io.NopCloser(bytes.NewReader(nil)), Request: request}, nil
	})
	err := store.Put(context.Background(), &model.AssetStoredObject{Bucket: "b-1", Region: "ap-guangzhou", ObjectKey: "key", FileSize: 1, SHA256: "expected"}, bytes.NewReader([]byte{1}))
	require.ErrorContains(t, err, "does not match")
}
