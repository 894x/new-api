package model

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOptionPrimaryKeyMigrationPreservesLegacyRowsAndUpserts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "options.db")), &gorm.Config{})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	require.NoError(t, db.Exec("CREATE TABLE options (`key` TEXT, value TEXT)").Error)
	legacy := []Option{{Key: "setting", Value: "old"}, {Key: "setting", Value: "new"}, {Key: "empty", Value: ""}, {Key: "", Value: "orphan"}}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, migrateOptionPrimaryKey(db))
	var rows []Option
	require.NoError(t, db.Order("`key`").Find(&rows).Error)
	assert.Equal(t, []Option{{Key: "empty", Value: ""}, {Key: "setting", Value: "new"}}, rows)
	tables, err := db.Migrator().GetTables()
	require.NoError(t, err)
	var backups []string
	for _, table := range tables {
		if strings.HasPrefix(table, optionLegacyTablePrefix) {
			backups = append(backups, table)
		}
	}
	require.Len(t, backups, 1)
	var backupRows []Option
	require.NoError(t, db.Table(backups[0]).Find(&backupRows).Error)
	assert.ElementsMatch(t, legacy, backupRows, "rebuild must retain recoverable original rows")
	require.NoError(t, db.Save(&Option{Key: "setting", Value: "updated"}).Error)
	require.NoError(t, db.Save(&Option{Key: "new-key", Value: "created"}).Error)
	require.NoError(t, migrateOptionPrimaryKey(db))
	var updated []Option
	require.NoError(t, db.Order("`key`").Find(&updated).Error)
	assert.Equal(t, []Option{{Key: "empty", Value: ""}, {Key: "new-key", Value: "created"}, {Key: "setting", Value: "updated"}}, updated)
	after, err := db.Migrator().GetTables()
	require.NoError(t, err)
	assert.ElementsMatch(t, tables, after, "repeated startup must not rebuild an already unique table")
}
