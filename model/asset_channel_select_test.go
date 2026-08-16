package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAssetChannelSelectTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db

	highPriority := int64(100)
	lowPriority := int64(10)
	weight := uint(100)
	for _, fixture := range []struct {
		id       int
		priority *int64
	}{
		{id: 4101, priority: &highPriority},
		{id: 4102, priority: &lowPriority},
	} {
		require.NoError(t, db.Create(&Channel{
			Id:       fixture.id,
			Type:     constant.ChannelTypeOpenAI,
			Key:      fmt.Sprintf("key-%d", fixture.id),
			Status:   common.ChannelStatusEnabled,
			Name:     fmt.Sprintf("channel-%d", fixture.id),
			Weight:   &weight,
			Models:   "asset-video-model",
			Group:    "default",
			Priority: fixture.priority,
		}).Error)
		require.NoError(t, db.Create(&Ability{
			Group:     "default",
			Model:     "asset-video-model",
			ChannelId: fixture.id,
			Enabled:   true,
			Priority:  fixture.priority,
			Weight:    weight,
		}).Error)
	}

	t.Cleanup(func() {
		DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&Channel{}) && originalDB.Migrator().HasTable(&Ability{}) {
			InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func TestAssetAllowedChannelsFilterBeforePrioritySelection(t *testing.T) {
	setupAssetChannelSelectTest(t)
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled
			if memoryCacheEnabled {
				InitChannelCache()
			}

			ordinary, err := GetRandomSatisfiedChannel("default", "asset-video-model", 0, "/api/v3/contents/generations/tasks")
			require.NoError(t, err)
			require.NotNil(t, ordinary)
			assert.Equal(t, 4101, ordinary.Id)

			allowed := map[int]struct{}{4102: {}}
			selected, err := GetRandomSatisfiedChannelWithFilter("default", "asset-video-model", 0, "/api/v3/contents/generations/tasks", allowed)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 4102, selected.Id)

			selected, err = GetRandomSatisfiedChannelWithFilter("default", "asset-video-model", 0, "/api/v3/contents/generations/tasks", map[int]struct{}{})
			require.NoError(t, err)
			assert.Nil(t, selected)
		})
	}
}
