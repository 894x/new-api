package model

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVisitPerfChannelLogsCrossesPageWithoutDuplicates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	original := LOG_DB
	LOG_DB = db
	t.Cleanup(func() { LOG_DB = original })
	// Exactly one more than a page protects the keyset boundary, not throughput.
	logs := make([]Log, 501)
	for i := range logs {
		logs[i] = Log{ChannelId: 15, Type: LogTypeConsume, CreatedAt: 100, Other: "{}"}
	}
	require.NoError(t, db.CreateInBatches(&logs, 100).Error)
	require.NoError(t, db.Create(&Log{ChannelId: 16, Type: LogTypeConsume, CreatedAt: 100}).Error)
	var ids []int64
	count, truncated, err := VisitPerfChannelLogs(context.Background(), 15, 90, 110, func(row PerfChannelLog) { ids = append(ids, row.Id) })
	require.NoError(t, err)
	assert.Equal(t, 501, count)
	assert.False(t, truncated)
	want := make([]int64, len(logs))
	for i := range logs {
		want[i] = int64(logs[len(logs)-1-i].Id)
	}
	assert.Equal(t, want, ids)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, err = VisitPerfChannelLogs(ctx, 15, 90, 110, func(PerfChannelLog) { cancel() })
	assert.ErrorIs(t, err, context.Canceled)
}
