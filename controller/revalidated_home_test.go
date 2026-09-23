package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHomeContentETagIncludesEveryPublicField(t *testing.T) {
	fields := []struct{ option, json string }{
		{"HomePageContent", "data"},
		{"HomePageTemplate", "template"},
		{"BusinessContactEmail", "business_contact_email"},
		{"BusinessContactQRCode", "business_contact_qr_code"},
	}
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{
		"HomePageContent":       "Welcome",
		"HomePageTemplate":      "custom",
		"BusinessContactEmail":  "contact@example.test",
		"BusinessContactQRCode": "https://example.test/contact.png",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})

	router := gin.New()
	router.GET("/api/home_page_content", GetHomePageContent)
	initial := httptest.NewRecorder()
	router.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "/api/home_page_content", nil))
	require.Equal(t, http.StatusOK, initial.Code)
	etag := initial.Header().Get("ETag")
	require.NotEmpty(t, etag)
	assert.Equal(t, "no-cache", initial.Header().Get("Cache-Control"))
	assert.Equal(t, "Accept-Encoding", initial.Header().Get("Vary"))
	var envelope map[string]any
	require.NoError(t, common.Unmarshal(initial.Body.Bytes(), &envelope))
	assert.Equal(t, true, envelope["success"])
	for _, field := range fields {
		assert.Equal(t, common.OptionMap[field.option], envelope[field.json])
	}
	request := httptest.NewRequest(http.MethodGet, "/api/home_page_content", nil)
	request.Header.Set("If-None-Match", etag)
	unchanged := httptest.NewRecorder()
	router.ServeHTTP(unchanged, request)
	assert.Equal(t, http.StatusNotModified, unchanged.Code)
	assert.Empty(t, unchanged.Body.String())

	for _, field := range fields {
		t.Run(field.option, func(t *testing.T) {
			common.OptionMapRWMutex.Lock()
			original := common.OptionMap[field.option]
			common.OptionMap[field.option] = original + "-changed"
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				common.OptionMapRWMutex.Lock()
				common.OptionMap[field.option] = original
				common.OptionMapRWMutex.Unlock()
			})
			request := httptest.NewRequest(http.MethodGet, "/api/home_page_content", nil)
			request.Header.Set("If-None-Match", etag)
			updated := httptest.NewRecorder()
			router.ServeHTTP(updated, request)
			require.Equal(t, http.StatusOK, updated.Code)
			assert.NotEqual(t, etag, updated.Header().Get("ETag"))
			var response map[string]any
			require.NoError(t, common.Unmarshal(updated.Body.Bytes(), &response))
			assert.Equal(t, original+"-changed", response[field.json])
		})
	}
}
