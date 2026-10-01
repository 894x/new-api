package controller

import (
	"bytes"
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

func TestSeedanceMediaStorageOptionValidatesAndPersists(t *testing.T) {
	db := setupAssetLibraryControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	previousOptions := common.OptionMap
	previousSetting := *system_setting.GetAssetStorageSetting()
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		common.OptionMap = previousOptions
		*system_setting.GetAssetStorageSetting() = previousSetting
	})
	const key = "asset_storage_setting.seedance_media_max_mb"
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{value: "4000", valid: true},
		{value: "0"},
		{value: "-1"},
		{value: "1.5"},
		{value: "1000001"},
		{value: "invalid"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			body, err := common.Marshal(map[string]string{"key": key, "value": tc.value})
			require.NoError(t, err)
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(body))
			UpdateOption(c)
			assert.Equal(t, http.StatusOK, response.Code)
			var result struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, tc.valid, result.Success)
			if !tc.valid {
				assert.Contains(t, result.Message, "integer between 1 and 1000000 MB")
			}
			var option model.Option
			require.NoError(t, db.Where("key = ?", key).First(&option).Error)
			assert.Equal(t, "4000", option.Value, "invalid updates must not change the saved capacity")
			assert.Equal(t, int64(4000), system_setting.GetAssetStorageSetting().SeedanceMediaMaxMB)
		})
	}
}

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
