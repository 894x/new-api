package model

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestChannelMonitoringDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "channel_monitor_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB := DB
			DB = db
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&ChannelMonitorRecord{}, &Channel{}))
				DB = previousDB
				require.NoError(t, sqlDB.Close())
			})
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			for _, upgrade := range []bool{false, true} {
				require.NoError(t, db.Migrator().DropTable(&ChannelMonitorRecord{}, &Channel{}))
				if upgrade {
					// Reproduce the preceding release's channel schema: all existing
					// fields and indexes, without the new monitoring column/table.
					require.NoError(t, db.AutoMigrate(&Channel{}))
					require.NoError(t, db.Migrator().DropColumn(&Channel{}, "MonitorConfig"))
					require.NoError(t, db.Model(&Channel{}).Create(map[string]any{"id": 7101, "name": "existing channel", "key": "fixture", "models": "model-a,model-b", "status": common.ChannelStatusEnabled}).Error)
				}
				if upgrade {
					// Also exercise upgrading this feature's preceding schema.
					require.NoError(t, db.AutoMigrate(&ChannelMonitorRecord{}))
					for _, field := range []string{"Source", "Count", "BucketAt", "ObservedAtMs"} {
						require.NoError(t, db.Migrator().DropColumn(&ChannelMonitorRecord{}, field))
					}
					require.NoError(t, db.Table("channel_monitor_test_channel_monitor_records").Create(map[string]any{"sample_key": "legacy", "channel_id": 7101, "model": "model-a", "state": "healthy", "created_at": int64(1)}).Error)
				}
				for range 2 {
					require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelMonitorRecord{}))
				}
				if upgrade {
					var preserved Channel
					require.NoError(t, db.First(&preserved, 7101).Error)
					assert.Equal(t, "existing channel", preserved.Name)
					assert.Equal(t, "fixture", preserved.Key)
					assert.Equal(t, "model-a,model-b", preserved.Models)
					enabled, targets, err := preserved.MonitorTargets()
					require.NoError(t, err)
					assert.True(t, enabled)
					assert.Equal(t, []ChannelMonitorTarget{{Model: "model-a"}}, targets)
				}
				if upgrade {
					var legacy ChannelMonitorRecord
					require.NoError(t, db.Where("sample_key = ?", "legacy").First(&legacy).Error)
					assert.Equal(t, "healthy", legacy.State)
					assert.Empty(t, legacy.Source)
					assert.Zero(t, legacy.Count)
					require.NoError(t, db.Delete(&legacy).Error)
				}
				assert.True(t, db.Migrator().HasIndex(&ChannelMonitorRecord{}, "idx_channel_monitor_window"))
				assert.True(t, db.Migrator().HasIndex(&Channel{}, "Name"))
			}

			now := time.Unix(1700000000, 0)
			channel := &Channel{Id: 7101, Models: "model-a,model-b", Status: common.ChannelStatusEnabled, MonitorConfig: `{"targets":[{"model":"model-a","endpoint_type":"openai"}]}`}
			ctx := context.Background()
			records := []ChannelMonitorRecord{
				{SampleKey: "ok", ChannelID: 7101, Model: "model-a", EndpointType: "openai", CreatedAt: now.Unix() - 600, State: "healthy", LatencyMs: 680},
				{SampleKey: "rejected-probe", ChannelID: 7101, Model: "model-a", EndpointType: "openai", CreatedAt: now.Unix() - 400, State: "unavailable", StatusCode: 400},
				{SampleKey: "slow", ChannelID: 7101, Model: "model-a", EndpointType: "openai", CreatedAt: now.Unix() - 300, State: "slow", LatencyMs: 4000},
				{SampleKey: "failure", ChannelID: 7101, Model: "model-a", EndpointType: "openai", CreatedAt: now.Unix() - 1, State: "unavailable", LatencyMs: 100, StatusCode: 503},
				{SampleKey: "other-model", ChannelID: 7101, Model: "model-b", EndpointType: "openai", CreatedAt: now.Unix(), State: "healthy"},
				{SampleKey: "other-channel", ChannelID: 7102, Model: "model-a", EndpointType: "openai", CreatedAt: now.Unix(), State: "healthy"},
				{SampleKey: "old", ChannelID: 7101, Model: "model-a", EndpointType: "openai", CreatedAt: now.Add(-8 * 24 * time.Hour).Unix(), State: "healthy"},
			}
			for i := range records {
				require.NoError(t, SaveChannelMonitorRecord(ctx, &records[i]))
			}
			duplicate := records[0]
			duplicate.ID = 0
			require.NoError(t, SaveChannelMonitorRecord(ctx, &duplicate))
			var count int64
			require.NoError(t, db.Model(&ChannelMonitorRecord{}).Count(&count).Error)
			assert.Equal(t, int64(7), count, "retried writes do not inflate probe success rates")
			excluded := []operation_setting.StatusCodeRange{{Start: 400, End: 400}}
			summaries, err := ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-a", true, 5*time.Minute, excluded, now)
			require.NoError(t, err)
			require.Len(t, summaries, 1)
			summary := summaries[0]
			assert.Equal(t, "unavailable", summary.State)
			assert.Equal(t, 4, summary.SampleCount)
			require.NotNil(t, summary.SuccessRate)
			assert.InDelta(t, 200.0/3, *summary.SuccessRate, 0.0001, "a rejected probe does not enter the availability denominator")
			require.Len(t, summary.Buckets, 96)
			assert.Equal(t, "empty", summary.Buckets[0].State)
			assert.Equal(t, "unavailable", summary.Buckets[95].State)
			assert.Equal(t, map[string]int{"healthy": 1, "slow": 1, "unavailable": 1, "excluded": 1}, summary.Buckets[95].StateCounts, "mixed outcomes retain their proportions within one interval")
			assert.Equal(t, 503, summary.Latest.StatusCode)
			unfiltered, err := ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-a", true, 5*time.Minute, nil, now)
			require.NoError(t, err)
			require.NotNil(t, unfiltered[0].SuccessRate)
			assert.Equal(t, 50.0, *unfiltered[0].SuccessRate, "clearing exclusions recalculates existing observations")
			otherModel, err := ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-b", true, 5*time.Minute, excluded, now)
			require.NoError(t, err)
			assert.Equal(t, "unregistered", otherModel[0].State)
			assert.Zero(t, otherModel[0].SampleCount)
			stale, err := ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-a", true, 5*time.Minute, excluded, now.Add(time.Hour))
			require.NoError(t, err)
			assert.Equal(t, "stale", stale[0].State)
			paused, err := ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-a", false, 5*time.Minute, excluded, now)
			require.NoError(t, err)
			assert.Equal(t, "paused", paused[0].State)
			traffic := ChannelMonitorRecord{SampleKey: "traffic", ChannelID: 7101, Model: "model-a", EndpointType: "openai", CreatedAt: now.Unix(), BucketAt: now.Unix() / 900 * 900, State: "healthy", StatusCode: 200, Source: "traffic", Count: 100}
			require.NoError(t, SaveChannelTrafficRecords(ctx, []ChannelMonitorRecord{traffic}))
			traffic.Count = 200
			require.NoError(t, SaveChannelTrafficRecords(ctx, []ChannelMonitorRecord{traffic}))
			require.NoError(t, SaveChannelTrafficRecords(ctx, []ChannelMonitorRecord{traffic}))
			mixed, err := ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-a", true, 5*time.Minute, excluded, now)
			require.NoError(t, err)
			assert.Equal(t, "healthy", mixed[0].State, "fresh traffic supersedes an earlier probe for the same protocol")
			assert.Equal(t, 204, mixed[0].SampleCount, "cumulative flush retries do not double count")
			assert.Equal(t, map[string]int{"probe": 4, "traffic": 200}, mixed[0].SourceCounts)
			assert.InDelta(t, 20200.0/203, *mixed[0].SuccessRate, 0.0001, "availability is weighted by attempts rather than averaging sources")
			traffic.SampleKey, traffic.StatusCode, traffic.State, traffic.Count = "traffic-400", 400, "unavailable", 20
			require.NoError(t, SaveChannelTrafficRecords(ctx, []ChannelMonitorRecord{traffic}))
			mixed, err = ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-a", true, 5*time.Minute, excluded, now)
			require.NoError(t, err)
			assert.Equal(t, 21, mixed[0].Buckets[95].StateCounts["excluded"])
			unfiltered, err = ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-a", true, 5*time.Minute, nil, now)
			require.NoError(t, err)
			assert.InDelta(t, 20200.0/224, *unfiltered[0].SuccessRate, 0.0001, "clearing exclusions includes upstream 400s from both sources")
			traffic.SampleKey, traffic.Model, traffic.State, traffic.StatusCode = "traffic-other-model", "model-b", "healthy", 200
			require.NoError(t, SaveChannelTrafficRecords(ctx, []ChannelMonitorRecord{traffic}))
			otherModel, err = ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-b", true, 5*time.Minute, excluded, now)
			require.NoError(t, err)
			assert.Equal(t, "healthy", otherModel[0].State, "traffic contributes without scheduling a synthetic target for this model")
			assert.Equal(t, 20, otherModel[0].SampleCount)
			require.NoError(t, SaveChannelTrafficRecords(ctx, []ChannelMonitorRecord{{SampleKey: "gap", Source: "gap", State: "collection_gap", CreatedAt: now.Unix()}}))
			partial, err := ChannelMonitorSummaries(ctx, []*Channel{channel}, "model-a", true, 5*time.Minute, excluded, now)
			require.NoError(t, err)
			assert.True(t, partial[0].Truncated)
			assert.Nil(t, partial[0].SuccessRate, "known collection gaps cannot show a complete availability rate")
			require.NoError(t, db.Where("source = ?", "gap").Delete(&ChannelMonitorRecord{}).Error)
			require.NoError(t, db.Where("source = ?", "traffic").Delete(&ChannelMonitorRecord{}).Error)
			require.NoError(t, PruneChannelMonitorRecords(ctx, now))
			require.NoError(t, db.Model(&ChannelMonitorRecord{}).Count(&count).Error)
			assert.Equal(t, int64(6), count)
		})
	}
}

func TestChannelMonitorRegistrationValidation(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		valid        bool
	}{
		{"default registration", "", true},
		{"disabled", `{"enabled":false}`, true},
		{"configured model and protocol", `{"targets":[{"model":"model-a","endpoint_type":"openai"}]}`, true},
		{"unknown model", `{"targets":[{"model":"missing","endpoint_type":"openai"}]}`, false},
		{"unsupported protocol", `{"targets":[{"model":"model-a","endpoint_type":"images"}]}`, false},
		{"duplicate target", `{"targets":[{"model":"model-a","endpoint_type":"openai"},{"model":"model-a","endpoint_type":"openai"}]}`, false},
		{"malformed configuration", "[", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := Channel{Models: "model-a,model-b", MonitorConfig: tc.config}
			_, err := channel.GetMonitorConfig()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	channel := Channel{Models: "model-a", MonitorConfig: `{"enabled":false}`}
	enabled, targets, err := channel.MonitorTargets()
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Empty(t, targets)
	options := requestPolicyDefaultOptions()
	options["monitor_setting.channel_monitoring_prompt"] = " \t\n"
	_, err = BuildRequestPolicy(options)
	require.Error(t, err, "a blank probe prompt must not be saved")
	options["monitor_setting.channel_monitoring_prompt"] = "Reply with OK."
	for _, key := range []string{"monitor_setting.channel_monitoring_minutes", "monitor_setting.channel_monitoring_timeout_seconds", "monitor_setting.channel_monitoring_max_tokens"} {
		original := options[key]
		options[key] = "0"
		_, err := BuildRequestPolicy(options)
		require.Error(t, err)
		options[key] = original
	}
}
