package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParameterMediaDownloaderCachesAndValidatesContent(t *testing.T) {
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	wavBytes := []byte{'R', 'I', 'F', 'F', 38, 0, 0, 0, 'W', 'A', 'V', 'E', 'f', 'm', 't', ' ', 16, 0, 0, 0, 1, 0, 1, 0, 0x40, 0x1f, 0, 0, 0x80, 0x3e, 0, 0, 2, 0, 16, 0, 'd', 'a', 't', 'a', 2, 0, 0, 0, 0, 0}
	mp4Header := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2', 'i', 's', 'o', 'm'}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/image":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes.Bytes())
		case "/fake":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("<html>not media</html>"))
		case "/large":
			w.Header().Set("Content-Length", "67108865")
		case "/audio":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(wavBytes)
		case "/video":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(mp4Header)
		case "/chunked":
			w.(http.Flusher).Flush()
			_, _ = w.Write(bytes.Repeat([]byte{0}, (1<<20)+1))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	fetch := system_setting.GetFetchSetting()
	originalFetch, originalClient, originalLimit := *fetch, httpClient, constant.MaxFileDownloadMB
	t.Cleanup(func() {
		*fetch = originalFetch
		httpClient = originalClient
		constant.MaxFileDownloadMB = originalLimit
	})
	fetch.EnableSSRFProtection = false
	httpClient = server.Client()
	constant.MaxFileDownloadMB = 64
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	transform := NewParameterMediaTransformer(c)
	encoded, err := transform(server.URL+"/image", "image")
	require.NoError(t, err)
	assert.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(pngBytes.Bytes()), encoded)
	// Re-creating the processor models a retry selecting another channel.
	again, err := NewParameterMediaTransformer(c)(server.URL+"/image", "image")
	require.NoError(t, err)
	assert.Equal(t, encoded, again)
	assert.Equal(t, 1, hits)
	for _, tc := range []struct {
		path, kind, mime string
		body             []byte
	}{
		{"/audio", "audio", "audio/wave", wavBytes},
		{"/video", "video", "video/mp4", mp4Header},
	} {
		data, err := transform(server.URL+tc.path, tc.kind)
		require.NoError(t, err)
		assert.Equal(t, "data:"+tc.mime+";base64,"+base64.StdEncoding.EncodeToString(tc.body), data)
	}
	_, err = transform(server.URL+"/fake", "image")
	require.ErrorContains(t, err, "not image")
	_, err = transform(server.URL+"/image", "audio")
	require.ErrorContains(t, err, "not audio")
	_, err = transform(server.URL+"/large", "video")
	require.ErrorContains(t, err, "size limit")
	_, err = transform(server.URL+"/missing?token=secret", "image")
	require.ErrorContains(t, err, "HTTP 404")
	assert.NotContains(t, err.Error(), "secret")
	constant.MaxFileDownloadMB = 1
	_, err = transform(server.URL+"/chunked", "video")
	require.ErrorContains(t, err, "size limit")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	before := hits
	_, err = transform(server.URL+"/cancel", "image")
	require.ErrorContains(t, err, "cancelled")
	assert.Equal(t, before, hits)
}

func TestParameterMediaDownloaderHonorsFetchPolicy(t *testing.T) {
	originalLimit := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 64
	t.Cleanup(func() { constant.MaxFileDownloadMB = originalLimit })
	fetch := system_setting.GetFetchSetting()
	original := *fetch
	t.Cleanup(func() { *fetch = original })
	fetch.EnableSSRFProtection = true
	fetch.AllowPrivateIp = false
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	_, err := NewParameterMediaTransformer(c)("http://127.0.0.1/media", "image")
	require.ErrorContains(t, err, "fetch policy")
}
