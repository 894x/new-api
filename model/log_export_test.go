package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetLogsForExportUsesUsageTypesFiltersAndLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "log-export.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, db.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY, name TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, name) VALUES (?, ?)", 10, "supplier-a").Error)

	previousDB, previousLogDB := DB, LOG_DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	DB, LOG_DB = db, db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	logs := []*Log{
		{Id: 1, CreatedAt: 100, Type: LogTypeConsume, Username: "alice", ModelName: "gpt-x", ChannelId: 10, RequestId: "req-1"},
		{Id: 2, CreatedAt: 101, Type: LogTypeError, Username: "alice", ModelName: "gpt-x", ChannelId: 10, RequestId: "req-1"},
		{Id: 3, CreatedAt: 102, Type: LogTypeManage, Username: "alice", ModelName: "gpt-x", ChannelId: 10, RequestId: "req-1"},
		{Id: 4, CreatedAt: 103, Type: LogTypeConsume, Username: "bob", ModelName: "gpt-x", ChannelId: 10, RequestId: "req-2"},
	}
	require.NoError(t, db.Create(&logs).Error)

	result, total, err := GetLogsForExport(LogExportQuery{
		StartTimestamp: 99,
		EndTimestamp:   102,
		ModelName:      "gpt-x",
		Username:       "alice",
		ChannelID:      10,
		RequestID:      "req-1",
	}, 1)

	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, result, 2)
	assert.Equal(t, 2, result[0].Id)
	assert.Equal(t, "supplier-a", result[0].ChannelName)
}
