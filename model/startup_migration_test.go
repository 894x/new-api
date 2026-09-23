package model

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Exercise the canonical startup migration after removing its unused parallel
// alternative. For cross-release checks, run this same test source first on the
// preceding checkout with NEW_API_MIGRATION_FIXTURE_PHASE=seed, then on the new
// checkout with phase=upgrade and the same NEW_API_MIGRATION_FIXTURE_DIR. Seed
// refuses to overwrite either fixture file. This is not a server matrix test.
func TestStartupMigrationSQLitePreservesForkDataAndSeparateLogs(t *testing.T) {
	fixtureDirectory := os.Getenv("NEW_API_MIGRATION_FIXTURE_DIR")
	phase := os.Getenv("NEW_API_MIGRATION_FIXTURE_PHASE")
	if fixtureDirectory == "" {
		require.Empty(t, phase, "persistent fixture phase requires a directory")
		fixtureDirectory = t.TempDir()
	} else {
		require.Contains(t, []string{"seed", "upgrade"}, phase)
		for _, name := range []string{"main", "logs"} {
			info, err := os.Stat(filepath.Join(fixtureDirectory, "rc-startup-"+name+".sqlite"))
			if phase == "seed" {
				require.True(t, os.IsNotExist(err), "refuse to overwrite an existing fixture")
			} else {
				require.NoError(t, err, "upgrade requires an existing previous-release fixture")
				require.True(t, info.Mode().IsRegular())
			}
		}
	}
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousGroup, previousKey, previousTrue, previousFalse := commonGroupCol, commonKeyCol, commonTrueVal, commonFalseVal
	previousLogKey, previousLogGroup := logKeyCol, logGroupCol
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		commonGroupCol, commonKeyCol, commonTrueVal, commonFalseVal = previousGroup, previousKey, previousTrue, previousFalse
		logKeyCol, logGroupCol = previousLogKey, previousLogGroup
	})
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	initCol()

	for _, target := range []struct {
		name string
		db   **gorm.DB
	}{{"main", &DB}, {"logs", &LOG_DB}} {
		dsn := filepath.ToSlash(filepath.Join(fixtureDirectory, "rc-startup-"+target.name+".sqlite")) + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"
		db, err := gorm.Open(sqlite.Open(dsn), newGormConfig(true))
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		*target.db = db
	}
	var sqliteVersion string
	require.NoError(t, DB.Raw("select sqlite_version()").Scan(&sqliteVersion).Error)
	t.Logf("SQLite %s", sqliteVersion)
	require.NoError(t, migrateDB())
	require.NoError(t, migrateLOGDB())

	user := User{Id: 101, Username: "migration-user", Password: "fixture", AffCode: "migration-aff", Quota: 12345, QuotaVersion: 7}
	token := Token{Id: 102, UserId: user.Id, Key: "migration-token", RemainQuota: 7890, QuotaVersion: 9}
	document := ModelDocument{ModelId: 44, Slug: "migration-document", Title: "Preserved document", DraftHTML: "<p>draft</p>", PublishedHTML: "<p>published</p>"}
	plugin := TaskPlugin{Id: 103, Key: "migration-provider", APIVersion: 1, Version: "1.0", Source: "preserved source", SourceHash: "fixture-hash", Enabled: true, Active: true}
	group := PrefillGroup{Id: 104, Name: "migration-group", Type: "model", Items: JSONValue(`["custom-model"]`)}
	entry := Log{Id: 105, UserId: user.Id, Type: 2, Content: "separate log survives migration"}
	if phase != "upgrade" {
		require.NoError(t, DB.Create(&user).Error)
		require.NoError(t, DB.Model(&user).Update("auth_version", 0).Error)
		require.NoError(t, DB.Create(&token).Error)
		require.NoError(t, DB.Create(&document).Error)
		require.NoError(t, DB.Create(&plugin).Error)
		require.NoError(t, DB.Create(&group).Error)
		require.NoError(t, LOG_DB.Create(&entry).Error)
	}
	if phase == "seed" {
		t.Log("previous-release fixture created; run phase=upgrade from the target checkout")
		return
	}

	for range 2 {
		require.NoError(t, migrateDB())
		require.NoError(t, migrateLOGDB())
	}
	var preservedUser User
	require.NoError(t, DB.First(&preservedUser, user.Id).Error)
	assert.Equal(t, 12345, preservedUser.Quota)
	assert.EqualValues(t, 7, preservedUser.QuotaVersion)
	assert.EqualValues(t, 1, preservedUser.AuthVersion)
	var preservedToken Token
	require.NoError(t, DB.First(&preservedToken, token.Id).Error)
	assert.Equal(t, 7890, preservedToken.RemainQuota)
	assert.EqualValues(t, 9, preservedToken.QuotaVersion)
	var variants []ModelDocumentVariant
	require.NoError(t, DB.Where("model_id = ?", document.ModelId).Find(&variants).Error)
	require.Len(t, variants, 1)
	assert.Equal(t, document.DraftHTML, variants[0].DraftHTML)
	assert.Equal(t, document.PublishedHTML, variants[0].PublishedHTML)
	assert.Equal(t, ModelDocumentDefaultInterfaceKey, variants[0].InterfaceKey)
	var preservedPlugin TaskPlugin
	require.NoError(t, DB.First(&preservedPlugin, plugin.Id).Error)
	assert.Equal(t, plugin.Source, preservedPlugin.Source)
	assert.True(t, preservedPlugin.Active)
	duplicatePlugin := plugin
	duplicatePlugin.Id = 0
	require.Error(t, DB.Create(&duplicatePlugin).Error, "plugin key/version must remain unique")
	var preservedGroup PrefillGroup
	require.NoError(t, DB.First(&preservedGroup, group.Id).Error)
	assert.JSONEq(t, string(group.Items), string(preservedGroup.Items))
	rollbackProbe := errors.New("rollback uniqueness probe")
	require.ErrorIs(t, DB.Transaction(func(tx *gorm.DB) error {
		duplicateGroup := PrefillGroup{Name: group.Name, Type: group.Type, Items: JSONValue(`[]`)}
		require.Error(t, tx.Create(&duplicateGroup).Error, "active prefill names must remain unique")
		require.NoError(t, tx.Delete(&group).Error)
		duplicateGroup.Id = 0
		require.NoError(t, tx.Create(&duplicateGroup).Error, "soft-deleted names can be reused on SQLite")
		return rollbackProbe
	}), rollbackProbe)
	var preservedLog Log
	require.NoError(t, LOG_DB.First(&preservedLog, entry.Id).Error)
	assert.Equal(t, entry.Content, preservedLog.Content)
	var mainLogCount int64
	require.NoError(t, DB.Model(&Log{}).Count(&mainLogCount).Error)
	assert.Zero(t, mainLogCount, "separate logs must not move into the primary database")
}
