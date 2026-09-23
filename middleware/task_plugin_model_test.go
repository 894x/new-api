package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPluginModelRewritePreservesOriginalSelectionSize(t *testing.T) {
	const original = `{"model":"\u0061LIAS","prompt":"test"}`
	for _, tc := range []struct {
		name     string
		recorded *int64
		wantSize int64
	}{
		{name: "capture before model spelling shortens JSON", wantSize: int64(len(original))},
		{name: "retain earlier native protocol capture", recorded: common.GetPointer(int64(123)), wantSize: 123},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(original))
			c.Request.Header.Set("Content-Type", "application/json")
			if tc.recorded != nil {
				common.SetContextKey(c, constant.ContextKeySelectionBodySize, *tc.recorded)
			}

			require.NoError(t, rewriteTaskPluginJSONModel(c, "alias"))

			originalSize, recorded := common.GetContextKeyType[int64](c, constant.ContextKeySelectionBodySize)
			assert.True(t, recorded)
			assert.Equal(t, tc.wantSize, originalSize)
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, storage.Close()) })
			raw, err := storage.Bytes()
			require.NoError(t, err)
			assert.JSONEq(t, `{"model":"alias","prompt":"test"}`, string(raw))
			assert.Equal(t, int64(len(raw)), c.Request.ContentLength)
			assert.Less(t, int64(len(raw)), int64(len(original)))
		})
	}
}
