package perfmetrics

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummaryHourlySuccessWeightsRequestsAndFiltersGroups(t *testing.T) {
	previousDB := model.DB
	previousPath, previousMaster := common.SQLitePath, common.IsMasterNode
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	t.Setenv("SQL_DSN", "")
	t.Setenv("LOG_SQL_DSN", "")
	common.SQLitePath, common.IsMasterNode = ":memory:", false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SQLitePath, common.IsMasterNode = previousPath, previousMaster
		common.SetMainDatabaseType(previousMain)
		common.SetLogDatabaseType(previousLog)
		hotBuckets = sync.Map{}
	})
	require.NoError(t, model.InitDB())
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}))
	hotBuckets = sync.Map{}
	hour := time.Now().Truncate(time.Hour).Add(-4 * time.Hour).Unix()
	rows := []model.PerfMetric{
		{ModelName: "wan", Group: "visible", BucketTs: hour, RequestCount: 1, SuccessCount: 1},
		{ModelName: "wan", Group: "visible", BucketTs: hour + 300, RequestCount: 3, SuccessCount: 1},
		{ModelName: "wan", Group: "hidden", BucketTs: hour + 600, RequestCount: 100, SuccessCount: 100},
		{ModelName: "wan", Group: "visible", BucketTs: hour + 3600, RequestCount: 2, SuccessCount: 0},
		{ModelName: "wan", Group: "visible", BucketTs: hour + 7200, RequestCount: 3, SuccessCount: 3},
		{ModelName: "wan", Group: "visible", BucketTs: hour + 10800, RequestCount: 1, SuccessCount: 1},
	}
	require.NoError(t, db.Create(&rows).Error)
	result, err := QuerySummaryAll(24, []string{"visible"})
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	assert.Equal(t, int64(10), result.Models[0].RequestCount)
	assert.Equal(t, 60.0, result.Models[0].SuccessRate)
	assert.Equal(t, []SuccessRatePoint{
		{Ts: hour, SuccessRate: 50}, {Ts: hour + 3600, SuccessRate: 0},
		{Ts: hour + 7200, SuccessRate: 100}, {Ts: hour + 10800, SuccessRate: 100},
	}, result.Models[0].RecentSuccessSeries)
	result, err = QuerySummaryAll(24, []string{})
	require.NoError(t, err)
	assert.Empty(t, result.Models)
}
