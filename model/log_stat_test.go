package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSumUsedQuotaPreservesTotalsAndRateFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previousLogDB, previousType, previousGroup := LOG_DB, common.LogDatabaseType(), logGroupCol
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	logGroupCol = "`group`"
	t.Cleanup(func() {
		LOG_DB, logGroupCol = previousLogDB, previousGroup
		common.SetLogDatabaseType(previousType)
	})
	require.NoError(t, db.AutoMigrate(&Log{}))
	now := time.Now().Unix()
	rows := []Log{
		{CreatedAt: now - 120, Type: LogTypeConsume, Username: "alice", TokenName: "key", ChannelId: 7, Group: "team", ModelName: "custom-one", Quota: 40, PromptTokens: 10, CompletionTokens: 5},
		{CreatedAt: now - 5, Type: LogTypeConsume, Username: "alice", TokenName: "key", ChannelId: 7, Group: "team", ModelName: "custom-one", Quota: 60, PromptTokens: 20, CompletionTokens: 7},
		{CreatedAt: now - 5, Type: LogTypeConsume, Username: "alice", TokenName: "key", ChannelId: 7, Group: "team", ModelName: "custom-two", Quota: 90, PromptTokens: 30, CompletionTokens: 8},
		{CreatedAt: now - 5, Type: LogTypeConsume, Username: "bob", TokenName: "key", ChannelId: 7, Group: "team", ModelName: "custom-one", Quota: 9000, PromptTokens: 9000},
		{CreatedAt: now - 5, Type: LogTypeTopup, Username: "alice", TokenName: "key", ChannelId: 7, Group: "team", ModelName: "custom-one", Quota: 8000, PromptTokens: 8000},
		{CreatedAt: now - 120, Type: LogTypeConsume, Username: "alice", TokenName: "key", ChannelId: 7, Group: "team", ModelName: "archive-model", Quota: 123, PromptTokens: 50},
	}
	require.NoError(t, db.Create(&rows).Error)
	for _, tc := range []struct {
		name  string
		model string
		end   int64
		want  Stat
	}{
		{name: "exact model", model: "custom-one", end: now, want: Stat{Quota: 100, Rpm: 1, Tpm: 27}},
		{name: "explicit wildcard", model: "custom-%", end: now, want: Stat{Quota: 190, Rpm: 2, Tpm: 65}},
		{name: "historical total with live rate", model: "custom-one", end: now - 90, want: Stat{Quota: 40, Rpm: 1, Tpm: 27}},
		{name: "total survives empty rate window", model: "archive-model", end: now, want: Stat{Quota: 123}},
		{name: "no matching usage", model: "missing", end: now, want: Stat{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stat, err := SumUsedQuota(LogTypeConsume, now-300, tc.end, tc.model, "alice", "key", 7, "team")
			require.NoError(t, err)
			assert.Equal(t, tc.want, stat)
		})
	}
}
