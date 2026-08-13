package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAssetLibraryModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&ChannelAssetConfig{}, &UserAssetGroup{}, &UserAsset{},
		&UserAssetGroupReplica{}, &UserAssetReplica{},
	))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestAssetReplicaIntersectionEnforcesOwnershipAndAllMappings(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.Create(&[]ChannelAssetConfig{
		{ChannelId: 1, Enabled: true},
		{ChannelId: 2, Enabled: true},
		{ChannelId: 3, Enabled: false},
	}).Error)
	require.NoError(t, db.Create(&[]UserAsset{
		{Id: "asset-na-one", UserId: 10, GroupId: "group-na-a", AssetType: "Image"},
		{Id: "asset-na-two", UserId: 10, GroupId: "group-na-a", AssetType: "Video"},
		{Id: "asset-na-other", UserId: 20, GroupId: "group-na-b", AssetType: "Image"},
	}).Error)
	require.NoError(t, db.Create(&[]UserAssetReplica{
		{AssetId: "asset-na-one", ChannelId: 1, UpstreamAssetId: "up-one-a"},
		{AssetId: "asset-na-two", ChannelId: 1, UpstreamAssetId: "up-two-a"},
		{AssetId: "asset-na-one", ChannelId: 2, UpstreamAssetId: "up-one-b"},
		{AssetId: "asset-na-two", ChannelId: 3, UpstreamAssetId: "up-two-c"},
	}).Error)

	allowed, err := GetAssetReplicaChannelIntersection(10, []string{"asset-na-one", "asset-na-two", "asset-na-one"})
	require.NoError(t, err)
	assert.Equal(t, map[int]struct{}{1: {}}, allowed)

	_, err = GetAssetReplicaChannelIntersection(10, []string{"asset-na-one", "asset-na-other"})
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestAssetReplicaUpsertKeepsSingleMapping(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, SaveUserAssetReplica(&UserAssetReplica{
		AssetId: "asset-na-one", ChannelId: 1, UpstreamAssetId: "first", State: AssetReplicaStateProcessing,
	}))
	require.NoError(t, SaveUserAssetReplica(&UserAssetReplica{
		AssetId: "asset-na-one", ChannelId: 1, UpstreamAssetId: "second", State: AssetReplicaStateReady,
	}))

	var count int64
	require.NoError(t, db.Model(&UserAssetReplica{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	replica, err := GetUserAssetReplica("asset-na-one", 1)
	require.NoError(t, err)
	assert.Equal(t, "second", replica.UpstreamAssetId)
	assert.Equal(t, AssetReplicaStateReady, replica.State)
}

func TestDeleteUserAssetLibraryDataRemovesOnlyOwnedLogicalData(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.Create(&[]UserAssetGroup{
		{Id: "group-na-owned", UserId: 10},
		{Id: "group-na-other", UserId: 20},
	}).Error)
	require.NoError(t, db.Create(&[]UserAsset{
		{Id: "asset-na-owned", UserId: 10, GroupId: "group-na-owned", AssetType: "Image"},
		{Id: "asset-na-other", UserId: 20, GroupId: "group-na-other", AssetType: "Image"},
	}).Error)
	require.NoError(t, db.Create(&[]UserAssetGroupReplica{
		{GroupId: "group-na-owned", ChannelId: 1},
		{GroupId: "group-na-other", ChannelId: 1},
	}).Error)
	require.NoError(t, db.Create(&[]UserAssetReplica{
		{AssetId: "asset-na-owned", ChannelId: 1},
		{AssetId: "asset-na-other", ChannelId: 1},
	}).Error)

	require.NoError(t, DeleteUserAssetLibraryData(db, 10))
	assert.ErrorIs(t, db.First(&UserAsset{}, "id = ?", "asset-na-owned").Error, gorm.ErrRecordNotFound)
	assert.ErrorIs(t, db.First(&UserAssetGroup{}, "id = ?", "group-na-owned").Error, gorm.ErrRecordNotFound)
	require.NoError(t, db.First(&UserAsset{}, "id = ?", "asset-na-other").Error)
	require.NoError(t, db.First(&UserAssetGroup{}, "id = ?", "group-na-other").Error)
}

func TestListUserAssetsFiltersStatusesFromEnabledChannelsOnly(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.Create(&[]ChannelAssetConfig{
		{ChannelId: 1, Enabled: true},
		{ChannelId: 2, Enabled: false},
	}).Error)
	require.NoError(t, db.Create(&[]UserAsset{
		{Id: "asset-na-active", UserId: 10, GroupId: "group-na-a", AssetType: "Image"},
		{Id: "asset-na-disabled", UserId: 10, GroupId: "group-na-a", AssetType: "Image"},
	}).Error)
	require.NoError(t, db.Create(&[]UserAssetReplica{
		{AssetId: "asset-na-active", ChannelId: 1, UpstreamStatus: "Active"},
		{AssetId: "asset-na-disabled", ChannelId: 2, UpstreamStatus: "Active"},
	}).Error)

	assets, total, err := ListUserAssets(10, AssetListParams{
		Statuses: []string{"Active"}, PageNumber: 1, PageSize: 100,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, assets, 1)
	assert.Equal(t, "asset-na-active", assets[0].Id)
}

func TestBatchDeleteChannelsRemovesAssetLibraryData(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	require.NoError(t, db.Create(&Channel{Id: 17, Name: "asset-channel", Key: "key"}).Error)
	require.NoError(t, db.Create(&ChannelAssetConfig{ChannelId: 17, Enabled: true}).Error)
	require.NoError(t, db.Create(&UserAssetGroupReplica{GroupId: "group-na-a", ChannelId: 17}).Error)
	require.NoError(t, db.Create(&UserAssetReplica{AssetId: "asset-na-a", ChannelId: 17}).Error)

	deleted, err := BatchDeleteChannels([]int{17})
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	assert.ErrorIs(t, db.First(&ChannelAssetConfig{}, "channel_id = ?", 17).Error, gorm.ErrRecordNotFound)
	assert.ErrorIs(t, db.First(&UserAssetGroupReplica{}, "channel_id = ?", 17).Error, gorm.ErrRecordNotFound)
	assert.ErrorIs(t, db.First(&UserAssetReplica{}, "channel_id = ?", 17).Error, gorm.ErrRecordNotFound)
}

func TestCountUserAssetsInGroupIsAccountScoped(t *testing.T) {
	db := setupAssetLibraryModelTestDB(t)
	require.NoError(t, db.Create(&[]UserAsset{
		{Id: "asset-na-owned", UserId: 10, GroupId: "group-na-shared", AssetType: "Image"},
		{Id: "asset-na-other", UserId: 20, GroupId: "group-na-shared", AssetType: "Image"},
	}).Error)

	count, err := CountUserAssetsInGroup(10, "group-na-shared")
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
}
