package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetStorageUsageReportsAccountMBAndClampsRemaining(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.AssetStorageAccount{}))
	require.NoError(t, db.Create(&model.AssetStorageAccount{UserId: 7, UsedBytes: 1500000}).Error)
	require.NoError(t, db.Create(&model.AssetStorageAccount{UserId: 8, UsedBytes: 3000000}).Error)
	previous := system_setting.GetAssetStorageSetting().DefaultQuotaMB
	system_setting.GetAssetStorageSetting().DefaultQuotaMB = 1
	t.Cleanup(func() { system_setting.GetAssetStorageSetting().DefaultQuotaMB = previous })
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Set("id", 7)
	GetAssetStorageUsage(c)
	var result struct {
		Success bool
		Data    struct {
			QuotaMB        int64 `json:"quota_mb"`
			UsedBytes      int64 `json:"used_bytes"`
			RemainingBytes int64 `json:"remaining_bytes"`
			BytesPerMB     int64 `json:"bytes_per_mb"`
		}
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.True(t, result.Success)
	assert.EqualValues(t, 1, result.Data.QuotaMB)
	assert.EqualValues(t, 1500000, result.Data.UsedBytes)
	assert.Zero(t, result.Data.RemainingBytes)
	assert.EqualValues(t, 1000000, result.Data.BytesPerMB)
}

func TestAssetStorageContentRejectsAnotherOwnerBeforeDownload(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	require.NoError(t, db.Create(&model.UserAsset{Id: "owned", UserId: 7, StoredObjectId: "original"}).Error)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/asset-library/assets/owned/content", nil)
	c.Set("id", 8)
	c.Params = gin.Params{{Key: "id", Value: "owned"}}
	GetAssetLibraryContent(c)
	assert.Equal(t, http.StatusNotFound, response.Code)
}
