package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetQuotaReservationsDeduplicateAndNeverExceedMBLimit(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.AutoMigrate(&AssetStoredObject{}, &AssetStorageAccount{}))
	previous := system_setting.GetAssetStorageSetting().DefaultQuotaMB
	system_setting.GetAssetStorageSetting().DefaultQuotaMB = 1
	t.Cleanup(func() { system_setting.GetAssetStorageSetting().DefaultQuotaMB = previous })
	first := &AssetStoredObject{Id: "first", UserId: 1, SHA256: "digest-one", AssetType: "Image", FileSize: 600000, State: "pending"}
	_, claimed, err := ClaimAssetStoredObject(first, "upload-one")
	require.NoError(t, err)
	require.True(t, claimed)
	duplicate := *first
	duplicate.Id = "duplicate"
	object, claimed, err := ClaimAssetStoredObject(&duplicate, "upload-duplicate")
	require.NoError(t, err)
	assert.False(t, claimed)
	assert.Equal(t, "first", object.Id)
	used, err := GetAssetStorageUsedBytes(1)
	require.NoError(t, err)
	assert.EqualValues(t, 600000, used)
	second := &AssetStoredObject{Id: "second", UserId: 1, SHA256: "digest-two", AssetType: "Video", FileSize: 400001, State: "pending"}
	_, _, err = ClaimAssetStoredObject(second, "upload-two")
	require.ErrorIs(t, err, ErrAssetQuotaExceeded)
	second.FileSize = 400000
	_, claimed, err = ClaimAssetStoredObject(second, "upload-two")
	require.NoError(t, err)
	require.True(t, claimed)
	used, err = GetAssetStorageUsedBytes(1)
	require.NoError(t, err)
	assert.EqualValues(t, 1000000, used)
	require.NoError(t, CompleteAssetStoredObject("second", "upload-two", errors.New("upload failed")))
	require.Error(t, CompleteAssetStoredObject("second", "upload-two", errors.New("duplicate failure")))
	used, err = GetAssetStorageUsedBytes(1)
	require.NoError(t, err)
	assert.EqualValues(t, 600000, used)
	require.NoError(t, CompleteAssetStoredObject("first", "upload-one", nil))
	require.NoError(t, CreateUserAsset(&UserAsset{Id: "asset-one", UserId: 1, StoredObjectId: "first"}))
	require.NoError(t, CreateUserAsset(&UserAsset{Id: "asset-two", UserId: 1, StoredObjectId: "first"}))
	require.NoError(t, DeleteUserAsset(1, "asset-one"))
	used, err = GetAssetStorageUsedBytes(1)
	require.NoError(t, err)
	assert.EqualValues(t, 600000, used)
	require.NoError(t, DeleteUserAsset(1, "asset-two"))
	used, err = GetAssetStorageUsedBytes(1)
	require.NoError(t, err)
	assert.Zero(t, used)
	retained, err := GetAssetStoredObject(1, "first")
	require.NoError(t, err)
	assert.Equal(t, "ready", retained.State, "deleting catalog references must not break already-issued task download URLs")
	system_setting.GetAssetStorageSetting().DefaultQuotaMB = 0
	require.ErrorIs(t, CreateUserAsset(&UserAsset{Id: "asset-three", UserId: 1, StoredObjectId: "first"}), ErrAssetQuotaExceeded)
}

func TestAssetQuotaBulkDeletionReleasesOnlyLastAccountReference(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.AutoMigrate(&AssetStoredObject{}, &AssetStorageAccount{}))
	object := &AssetStoredObject{Id: "original", UserId: 1, SHA256: "digest", AssetType: "Image", FileSize: 1000, State: "pending"}
	_, _, err := ClaimAssetStoredObject(object, "upload")
	require.NoError(t, err)
	require.NoError(t, CompleteAssetStoredObject(object.Id, "upload", nil))
	require.NoError(t, CreateUserAsset(&UserAsset{Id: "one", UserId: 1, GroupId: "group-one", StoredObjectId: object.Id}))
	require.NoError(t, CreateUserAsset(&UserAsset{Id: "two", UserId: 1, GroupId: "group-two", StoredObjectId: object.Id}))
	require.NoError(t, DeleteUserAssetGroup(1, "group-one"))
	used, err := GetAssetStorageUsedBytes(1)
	require.NoError(t, err)
	assert.EqualValues(t, 1000, used)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return DeleteUserAssetLibraryData(tx, 1) }))
	used, err = GetAssetStorageUsedBytes(1)
	require.NoError(t, err)
	assert.Zero(t, used)
}

func TestAssetQuotaRecoveryReleasesOnlyAbandonedReservations(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.AutoMigrate(&AssetStoredObject{}, &AssetStorageAccount{}))
	object := &AssetStoredObject{Id: "abandoned", UserId: 1, SHA256: "digest", AssetType: "Image", FileSize: 1000, State: "pending"}
	_, claimed, err := ClaimAssetStoredObject(object, "dead-worker")
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, db.Model(&AssetStoredObject{}).Where("id = ?", object.Id).UpdateColumns(map[string]any{"lease_until": time.Now().Unix() - 1800, "updated_time": time.Now().Unix() - 1800}).Error)
	require.NoError(t, RecoverAssetStorageReservations())
	used, err := GetAssetStorageUsedBytes(1)
	require.NoError(t, err)
	assert.Zero(t, used)
	require.Error(t, CompleteAssetStoredObject(object.Id, "dead-worker", nil))
}

func TestAssetQuotaOverridePrecedenceAndResetPreserveUsage(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.AutoMigrate(&AssetStoredObject{}, &AssetStorageAccount{}))
	previous := system_setting.GetAssetStorageSetting().DefaultQuotaMB
	system_setting.GetAssetStorageSetting().DefaultQuotaMB = 0
	t.Cleanup(func() { system_setting.GetAssetStorageSetting().DefaultQuotaMB = previous })
	custom := int64(2)
	require.NoError(t, SetAssetStorageQuotaOverride(7, &custom))
	first := &AssetStoredObject{Id: "custom", UserId: 7, SHA256: "large", AssetType: "Image", FileSize: 1500000, State: "pending"}
	_, claimed, err := ClaimAssetStoredObject(first, "upload")
	require.NoError(t, err)
	require.True(t, claimed)
	account, err := GetAssetStorageAccount(7)
	require.NoError(t, err)
	quota, err := account.QuotaBytes()
	require.NoError(t, err)
	assert.EqualValues(t, 2000000, quota)
	assert.EqualValues(t, 1500000, account.UsedBytes)
	other := &AssetStoredObject{Id: "other", UserId: 8, SHA256: "small", AssetType: "Image", FileSize: 1, State: "pending"}
	_, _, err = ClaimAssetStoredObject(other, "other-upload")
	require.ErrorIs(t, err, ErrAssetQuotaExceeded)
	zero := int64(0)
	require.NoError(t, SetAssetStorageQuotaOverride(7, &zero))
	_, _, err = ClaimAssetStoredObject(first, "reuse")
	require.NoError(t, err, "quota reduction must preserve already-reserved originals")
	newObject := &AssetStoredObject{Id: "new", UserId: 7, SHA256: "new", AssetType: "Image", FileSize: 1, State: "pending"}
	_, _, err = ClaimAssetStoredObject(newObject, "new-upload")
	require.ErrorIs(t, err, ErrAssetQuotaExceeded)
	system_setting.GetAssetStorageSetting().DefaultQuotaMB = 3
	_, _, err = ClaimAssetStoredObject(newObject, "new-upload")
	require.ErrorIs(t, err, ErrAssetQuotaExceeded, "explicit zero must not inherit the default")
	require.NoError(t, SetAssetStorageQuotaOverride(7, nil))
	account, err = GetAssetStorageAccount(7)
	require.NoError(t, err)
	assert.Nil(t, account.QuotaOverrideMB)
	assert.EqualValues(t, 1500000, account.UsedBytes)
	quota, err = account.QuotaBytes()
	require.NoError(t, err)
	assert.EqualValues(t, 3000000, quota)
	_, claimed, err = ClaimAssetStoredObject(newObject, "new-upload")
	require.NoError(t, err)
	assert.True(t, claimed)
	for _, invalid := range []int64{-1, system_setting.MaxAssetQuotaMB + 1, 9223372036854775807} {
		require.Error(t, SetAssetStorageQuotaOverride(7, &invalid))
	}
	account, err = GetAssetStorageAccount(7)
	require.NoError(t, err)
	assert.Nil(t, account.QuotaOverrideMB)
	assert.EqualValues(t, 1500001, account.UsedBytes)
}
