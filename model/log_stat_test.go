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

func TestGetUserChannelStatsAggregatesAndLiveRates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previousLogDB := LOG_DB
	LOG_DB = db
	t.Cleanup(func() { LOG_DB = previousLogDB })
	require.NoError(t, db.AutoMigrate(&Log{}))
	now := time.Now().Unix()
	rows := []Log{
		{CreatedAt: now - 300, Type: LogTypeConsume, Username: "alice", ChannelId: 1, Quota: 10, PromptTokens: 100, CompletionTokens: 20},
		{CreatedAt: now - 5, Type: LogTypeConsume, Username: "alice", ChannelId: 1, Quota: 15, PromptTokens: 30, CompletionTokens: 10},
		{CreatedAt: now - 5, Type: LogTypeConsume, Username: "alice", ChannelId: 2, Quota: 7, PromptTokens: 40, CompletionTokens: 8},
		{CreatedAt: now - 5, Type: LogTypeConsume, Username: "alice", ChannelId: 2, Quota: 3, PromptTokens: 10, CompletionTokens: 2},
		{CreatedAt: now - 200, Type: LogTypeConsume, Username: "alice", ChannelId: 2, Quota: 2, PromptTokens: 5, CompletionTokens: 1},
		{CreatedAt: now - 5, Type: LogTypeConsume, Username: "bob", ChannelId: 1, Quota: 9999, PromptTokens: 9999},
		{CreatedAt: now - 5, Type: LogTypeTopup, Username: "alice", ChannelId: 1, Quota: 8000},
	}
	require.NoError(t, db.Create(&rows).Error)

	stats, err := GetUserChannelStats("alice", now-600, now)
	require.NoError(t, err)
	// 渠道 2 请求更多（3 > 2）仍按渠道 ID 升序输出；RPM/TPM 只统计最近 60 秒
	require.Len(t, stats, 2)
	assert.Equal(t, 1, stats[0].ChannelId)
	assert.Equal(t, int64(2), stats[0].Count)
	assert.Equal(t, int64(25), stats[0].Quota)
	assert.Equal(t, int64(130), stats[0].PromptTokens)
	assert.Equal(t, int64(30), stats[0].CompletionTokens)
	assert.Equal(t, int64(1), stats[0].Rpm)
	assert.Equal(t, int64(40), stats[0].Tpm)
	assert.Equal(t, now-300, stats[0].FirstAt)
	assert.Equal(t, now-5, stats[0].LastAt)
	assert.Equal(t, 2, stats[1].ChannelId)
	assert.Equal(t, int64(3), stats[1].Count)
	assert.Equal(t, int64(2), stats[1].Rpm)
	assert.Equal(t, int64(60), stats[1].Tpm)

	// 时间下界排除旧请求后渠道 1 的窗口聚合随之缩小，实时速率不变
	stats, err = GetUserChannelStats("alice", now-100, now)
	require.NoError(t, err)
	require.Len(t, stats, 2)
	assert.Equal(t, int64(1), stats[0].Count)
	assert.Equal(t, int64(1), stats[0].Rpm)

	// 用户名与其他统计接口同口径：默认精确匹配，显式 % 才走模糊匹配
	stats, err = GetUserChannelStats("ali", now-600, now)
	require.NoError(t, err)
	assert.Empty(t, stats)

	stats, err = GetUserChannelStats("al%", now-600, now)
	require.NoError(t, err)
	require.Len(t, stats, 2)

	stats, err = GetUserChannelStats("nobody", now-600, now)
	require.NoError(t, err)
	assert.Empty(t, stats)
}
