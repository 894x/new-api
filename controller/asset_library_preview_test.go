package controller

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetImagePreviewEnforcesOwnershipAndValidatesRemoteContent(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	var imageBytes bytes.Buffer
	require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		if r.URL.Path == "/bad" {
			_, _ = w.Write([]byte("<html>not an image</html>"))
			return
		}
		if r.URL.Path == "/large" {
			w.Header().Set("Content-Length", "31457280")
			return
		}
		if r.URL.Path == "/unavailable" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(imageBytes.Bytes())
	}))
	t.Cleanup(server.Close)
	fetch := system_setting.GetFetchSetting()
	previous := *fetch
	fetch.EnableSSRFProtection = false
	service.InitHttpClient()
	t.Cleanup(func() { *fetch = previous; service.InitHttpClient() })
	require.NoError(t, db.Create(&model.UserAsset{Id: "asset-na-image", UserId: 1, GroupId: "group-na-1", AssetType: "Image", SourceURL: server.URL + "/image"}).Error)
	for _, tc := range []struct {
		name   string
		userID int
		path   string
		status int
	}{
		{"owner can view failed or unsynchronized image", 1, "/image", http.StatusOK},
		{"other user cannot download image", 2, "/image", http.StatusNotFound},
		{"anonymous user cannot download image", 0, "/image", http.StatusUnauthorized},
		{"reject HTML even if stored as an image", 1, "/bad", http.StatusBadGateway},
		{"reject oversized image before reading its body", 1, "/large", http.StatusBadGateway},
		{"unavailable source produces a preview failure", 1, "/unavailable", http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Model(&model.UserAsset{}).Where("id = ?", "asset-na-image").Update("source_url", server.URL+tc.path).Error)
			before := requests
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("id", tc.userID)
			c.Params = gin.Params{{Key: "id", Value: "asset-na-image"}}
			c.Request = httptest.NewRequest(http.MethodGet, "/api/asset-library/assets/asset-na-image/preview", nil)
			c.Request.Header.Set("Authorization", "Bearer private-session")
			GetAssetLibraryImagePreview(c)
			assert.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				assert.Equal(t, imageBytes.Bytes(), w.Body.Bytes())
				assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
				assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
			}
			if tc.userID != 1 {
				assert.Equal(t, before, requests)
			}
		})
	}
}

func TestAssetImagePreviewAdminScopeAndPrivateURLProtection(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "owner", AffCode: "owner", Role: common.RoleCommonUser}).Error)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "admin", AffCode: "admin", Role: common.RoleRootUser}).Error)
	require.NoError(t, db.Create(&model.UserAsset{Id: "asset-na-private", UserId: 1, GroupId: "group-na-1", AssetType: "Image", SourceURL: "http://127.0.0.1/image"}).Error)
	fetch := system_setting.GetFetchSetting()
	previous := *fetch
	fetch.EnableSSRFProtection = true
	fetch.AllowPrivateIp = false
	service.InitHttpClient()
	t.Cleanup(func() { *fetch = previous; service.InitHttpClient() })
	for _, tc := range []struct {
		userID   int
		target   string
		expected int
	}{
		{1, "2", http.StatusForbidden},
		{2, "2", http.StatusNotFound},
		{2, "1", http.StatusBadGateway},
		{2, "invalid", http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("id", tc.userID)
		c.Params = gin.Params{{Key: "id", Value: "asset-na-private"}, {Key: "user_id", Value: tc.target}}
		c.Request = httptest.NewRequest(http.MethodGet, "/preview", nil)
		GetAssetLibraryImagePreview(c)
		assert.Equal(t, tc.expected, w.Code)
		assert.NotContains(t, w.Body.String(), "127.0.0.1")
	}
}
