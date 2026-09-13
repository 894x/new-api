package relay

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestManagedVideoRequestRewritesActualBodyAndKeepsRetrySnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous; sqlDB, _ := db.DB(); sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.UserAsset{}, &model.AssetStoredObject{}))
	object := &model.AssetStoredObject{Id: "original", UserId: 7, SHA256: "hash", AssetType: "Image", Bucket: "references-123456", Region: "ap-guangzhou", ObjectKey: "assets/7/image/hash", State: "ready"}
	require.NoError(t, db.Create(object).Error)
	require.NoError(t, db.Create(&model.UserAsset{Id: "asset-na-0123456789abcdef0123456789abcdef", UserId: 7, AssetType: "Image", StoredObjectId: object.Id}).Error)
	t.Setenv("ASSET_STORAGE_ENABLED", "true")
	t.Setenv("COS_BUCKET", object.Bucket)
	t.Setenv("COS_REGION", object.Region)
	t.Setenv("COS_SECRET_ID", "id")
	t.Setenv("COS_SECRET_KEY", "secret")
	for _, format := range []string{"json", "multipart"} {
		t.Run(format, func(t *testing.T) {
			var body bytes.Buffer
			contentType := "application/json"
			if format == "json" {
				body.WriteString(`{"model":"video-model","prompt":"https://example.com/prompt","image":"asset://asset-na-0123456789abcdef0123456789abcdef"}`)
			} else {
				writer := multipart.NewWriter(&body)
				require.NoError(t, writer.WriteField("model", "video-model"))
				require.NoError(t, writer.WriteField("image", "asset://asset-na-0123456789abcdef0123456789abcdef"))
				require.NoError(t, writer.Close())
				contentType = writer.FormDataContentType()
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/videos", &body)
			c.Request.Header.Set("Content-Type", contentType)
			defer common.CleanupBodyStorage(c)
			require.NoError(t, prepareManagedVideoRequest(c, 7))
			actual, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			assert.Contains(t, string(actual), "https://references-123456.cos.ap-guangzhou.myqcloud.com/assets/7/image/hash?")
			assert.NotContains(t, string(actual), "asset://")
			assert.Equal(t, "asset-na-0123456789abcdef0123456789abcdef", recorder.Header().Get("X-New-Api-Asset-Ids"))
			refs := ManagedVideoTaskReferences(c)
			require.NotNil(t, refs)
			assert.Equal(t, []model.TaskAssetReference{{AssetID: "asset-na-0123456789abcdef0123456789abcdef", StoredObjectID: "original"}}, refs.Items)
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			cached, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, actual, cached)
			c.Request.Body = io.NopCloser(bytes.NewReader(cached))
			require.NoError(t, prepareManagedVideoRequest(c, 7))
			assert.Equal(t, refs, ManagedVideoTaskReferences(c))
		})
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"image":"asset://asset-na-0123456789abcdef0123456789abcdef"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)
	require.ErrorContains(t, prepareManagedVideoRequest(c, 8), "does not belong")
}
