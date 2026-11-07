package controller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Pinned representative log schema from QuantumNous/new-api v1.0.0-rc.48.
type releasedGrossProfitLog struct {
	Id                int    `json:"id" gorm:"index:idx_created_at_id,priority:2;index:idx_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

func (releasedGrossProfitLog) TableName() string { return "logs" }

// The previous local accounting schema had no VAT or net amounts.
type previousGrossProfitEvent struct {
	EventKey   string `gorm:"type:varchar(64);primaryKey"`
	Day        string `gorm:"type:varchar(10);not null"`
	ChannelID  int    `gorm:"not null"`
	Reason     string `gorm:"type:varchar(64);not null"`
	RevenueUSD string `gorm:"type:text;not null"`
	CostUSD    string `gorm:"type:text;not null"`
	AppliedAt  int64  `gorm:"bigint;not null"`
	CreatedAt  int64  `gorm:"bigint;not null"`
}

func (previousGrossProfitEvent) TableName() string { return "gross_profit_events" }

type previousGrossProfitState struct {
	Name              string `gorm:"type:varchar(32);primaryKey"`
	Timezone          string `gorm:"type:varchar(64);not null"`
	CursorAt          int64  `gorm:"bigint;not null"`
	CursorID          int    `gorm:"bigint;not null"`
	CursorOffset      int    `gorm:"bigint;not null"`
	CursorRequestID   string `gorm:"type:varchar(64);not null"`
	FullScanStartedAt int64  `gorm:"bigint;not null"`
	BackfillComplete  bool
	Revision          int64 `gorm:"bigint;not null"`
	UpdatedAt         int64 `gorm:"bigint;not null"`
}

func (previousGrossProfitState) TableName() string { return "gross_profit_rollup_states" }

func TestGrossProfitVATRateValidation(t *testing.T) {
	for _, rate := range []*float64{nil, common.GetPointer(0.0), common.GetPointer(0.06), common.GetPointer(1.0)} {
		require.NoError(t, model.ValidateChannelRoutingLimits(&model.Channel{Group: "default", InvoiceVATRate: rate}))
	}
	for _, rate := range []float64{-0.01, 1.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		assert.ErrorContains(t, model.ValidateChannelRoutingLimits(&model.Channel{Group: "default", InvoiceVATRate: &rate}), "invoice VAT rate")
	}
}

func TestGrossProfitRejectsNonRootBeforeReadingData(t *testing.T) {
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("role", role)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/gross-profit?start_timestamp=1&end_timestamp=2", nil)
			GetGrossProfit(ctx)
			assert.Equal(t, http.StatusForbidden, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "known_revenue_usd")
		})
	}
}

func TestGrossProfitDatabaseMatrix(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var driver gorm.Dialector
			databaseType := common.DatabaseTypeSQLite
			switch engine {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "profit.db"))
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
				databaseType = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
				databaseType = common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			var logDriver gorm.Dialector = driver
			if engine == "sqlite" {
				logDriver = sqlite.Open(filepath.Join(t.TempDir(), "separate-logs.db"))
			}
			if engine == "mysql" && os.Getenv("TEST_LOG_MYSQL_DSN") != "" {
				logDriver = mysql.Open(os.Getenv("TEST_LOG_MYSQL_DSN"))
			}
			if engine == "postgres" && os.Getenv("TEST_LOG_POSTGRES_DSN") != "" {
				logDriver = postgres.New(postgres.Config{DSN: os.Getenv("TEST_LOG_POSTGRES_DSN"), PreferSimpleProtocol: true})
			}
			logDB, err := gorm.Open(logDriver, &gorm.Config{})
			require.NoError(t, err)
			previousDB, previousLogDB := model.DB, model.LOG_DB
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			previousConsume, previousExport, previousRedis := common.LogConsumeEnabled, common.DataExportEnabled, common.RedisEnabled
			previousUnit := common.QuotaPerUnit
			model.DB, model.LOG_DB = db, logDB
			common.SetDatabaseTypes(databaseType, databaseType)
			common.LogConsumeEnabled, common.DataExportEnabled, common.RedisEnabled = true, false, false
			common.QuotaPerUnit = 500000
			t.Cleanup(func() {
				model.DB, model.LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMain, previousLog)
				common.LogConsumeEnabled, common.DataExportEnabled, common.RedisEnabled = previousConsume, previousExport, previousRedis
				common.QuotaPerUnit = previousUnit
				for _, handle := range []*gorm.DB{db, logDB} {
					sqlDB, err := handle.DB()
					require.NoError(t, err)
					require.NoError(t, sqlDB.Close())
				}
			})
			t.Setenv("TZ", "UTC")
			// Fresh empty database and a repeated startup.
			require.NoError(t, model.MigrateGrossProfit(db))
			require.NoError(t, model.MigrateGrossProfit(db))
			require.NoError(t, db.Migrator().DropTable(&model.GrossProfitDaily{}, &model.GrossProfitEvent{}, &model.GrossProfitRollupState{}))
			// Preserve the old durable ledger even if its original logs were purged.
			require.NoError(t, db.AutoMigrate(&previousGrossProfitEvent{}, &previousGrossProfitState{}))
			legacyTime := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -30)
			legacyDay := legacyTime.Format(time.DateOnly)
			legacy := []previousGrossProfitEvent{
				{EventKey: fmt.Sprintf("%x", sha256.Sum256([]byte("legacy-replayed"))), Day: legacyDay, ChannelID: 1, RevenueUSD: "1.06", CostUSD: "0.53", AppliedAt: 1},
				{EventKey: fmt.Sprintf("%x", sha256.Sum256([]byte("legacy-purged"))), Day: legacyDay, ChannelID: 1, RevenueUSD: "2.12", CostUSD: "1.03", AppliedAt: 1},
			}
			require.NoError(t, db.Create(&legacy).Error)
			require.NoError(t, db.Create(&previousGrossProfitState{Name: "consume", Timezone: "UTC", BackfillComplete: true}).Error)
			// Upgrade the latest released schema with a retained consume log.
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.User{}))
			require.NoError(t, logDB.AutoMigrate(&releasedGrossProfitLog{}))
			require.NoError(t, logDB.Create(&model.Log{Type: model.LogTypeConsume, CreatedAt: legacyTime.Unix(), ChannelId: 1, Quota: 530000,
				Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":1,"event_id":"legacy-replayed","cost_usd":"0.53"}}}`}).Error)
			// A representative released log row predates all rollup tables/snapshots.
			require.NoError(t, logDB.Create(&releasedGrossProfitLog{CreatedAt: time.Now().Unix(), Type: model.LogTypeConsume, ChannelId: 2, Quota: 100000, Other: `{"quota_per_unit":500000}`}).Error)
			require.NoError(t, model.MigrateGrossProfit(db))
			require.NoError(t, model.MigrateGrossProfit(db))
			var oldState previousGrossProfitState
			require.NoError(t, db.First(&oldState, "name = ?", "consume").Error)
			assert.True(t, oldState.BackfillComplete)
			t.Run("failed legacy ledger import does not advance migration progress", func(t *testing.T) {
				var before model.GrossProfitRollupState
				require.NoError(t, db.First(&before, "name = ?", model.GrossProfitRollupName).Error)
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("gross-profit-legacy-import-failure", func(tx *gorm.DB) {
					if tx.Statement.Table == (model.GrossProfitEvent{}).TableName() {
						tx.AddError(errors.New("legacy ledger import unavailable"))
					}
				}))
				err := model.ReconcileGrossProfit(context.Background(), 1, time.Now())
				require.NoError(t, db.Callback().Create().Remove("gross-profit-legacy-import-failure"))
				require.ErrorContains(t, err, "legacy ledger import unavailable")
				var after model.GrossProfitRollupState
				require.NoError(t, db.First(&after, "name = ?", model.GrossProfitRollupName).Error)
				assert.Equal(t, before, after)
			})
			var version string
			versionQuery := "SELECT VERSION()"
			if engine == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("%s version: %s; separate main/log connections", engine, version)
			channel := model.Channel{Id: 1, Name: "Discount channel", Models: "model-a,gpt-4o-gizmo-*", CostDiscount: common.GetPointer(0.3), InvoiceVATRate: common.GetPointer(0.03)}
			require.NoError(t, db.Create(&channel).Error)
			// Upgrade a populated pre-VAT channel schema twice, preserving costs.
			require.NoError(t, db.Migrator().DropColumn(&model.Channel{}, "invoice_vat_rate"))
			require.NoError(t, db.AutoMigrate(&model.Channel{}))
			require.NoError(t, db.AutoMigrate(&model.Channel{}))
			var upgraded model.Channel
			require.NoError(t, db.First(&upgraded, 1).Error)
			assert.Nil(t, upgraded.InvoiceVATRate)
			require.NotNil(t, upgraded.CostDiscount)
			assert.Equal(t, 0.3, *upgraded.CostDiscount)
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1).Update("invoice_vat_rate", 0.03).Error)
			require.NoError(t, db.Create(&model.Channel{Id: 2, Name: "Unconfigured", Models: "model-a"}).Error)
			require.NoError(t, db.Create(&[]model.ChannelModelOverride{
				{ChannelId: 1, Model: "model-free", CostDiscount: common.GetPointer(0.0)},
				{ChannelId: 1, Model: "model-cheap", CostDiscount: common.GetPointer(0.2)},
				{ChannelId: 1, Model: "gpt-4o-gizmo-*", CostDiscount: common.GetPointer(0.1)},
			}).Error)
			for _, tc := range []struct {
				name, model, cost, reason string
				channel, quota            int
				ratio                     any
				original                  *int
			}{
				{name: "formula", model: "model-a", channel: 1, quota: 400000, ratio: 0.8, cost: "0.3"},
				{name: "model override", model: "model-cheap", channel: 1, quota: 400000, ratio: 0.8, cost: "0.2"},
				{name: "explicit zero", model: "model-free", channel: 1, quota: 0, ratio: 0, cost: "0"},
				{name: "normalized model", model: "gpt-4o-gizmo-customer", channel: 1, quota: 400000, ratio: 0.8, cost: "0.1"},
				{name: "monthly tiers", model: "model-a", channel: 1, quota: 250000, ratio: 0.8, original: common.GetPointer(500000), cost: "0.3"},
				{name: "monthly free", model: "model-a", channel: 1, quota: 0, ratio: 0, original: common.GetPointer(500000), cost: "0.3"},
				{name: "zero customer discount", model: "model-a", channel: 1, quota: 0, ratio: 0, reason: "billing_metadata_missing"},
				{name: "missing channel discount", model: "model-a", channel: 2, quota: 100000, ratio: 0.8, reason: "discount_unconfigured"},
				{name: "invalid billing metadata", model: "model-a", channel: 1, quota: 100000, ratio: "invalid", reason: "billing_metadata_missing"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					other := model.NewLogOther()
					other.SetPublic("group_ratio", tc.ratio)
					other.SetPublic("user_group_ratio", tc.ratio)
					if tc.original != nil {
						other.SetPublic("group_model_discount", map[string]any{"original_quota": *tc.original})
					}
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					model.RecordConsumeLog(ctx, 1, model.RecordConsumeLogParams{ChannelId: tc.channel, ModelName: tc.model, Quota: tc.quota, Other: other})
					var entry model.Log
					require.NoError(t, logDB.Order("id DESC").First(&entry).Error)
					assert.Equal(t, tc.cost, gjson.Get(entry.Other, "root_info.gross_profit.cost_usd").String())
					assert.Equal(t, tc.reason, gjson.Get(entry.Other, "root_info.gross_profit.reason").String())
					assert.Equal(t, 0.06, gjson.Get(entry.Other, "root_info.gross_profit.output_vat_rate").Float())
					if tc.channel == 1 {
						assert.Equal(t, 0.03, gjson.Get(entry.Other, "root_info.gross_profit.input_vat_rate").Float())
					}
					adminEntry := entry
					model.FormatAdminLogs([]*model.Log{&adminEntry})
					assert.False(t, gjson.Get(adminEntry.Other, "root_info").Exists())
					model.FormatRootLogs([]*model.Log{&entry})
					assert.True(t, gjson.Get(entry.Other, "root_info.gross_profit").Exists())
				})
			}
			start := time.Now().UTC().Truncate(24 * time.Hour).Unix()

			// Editing/deleting routing metadata must not reprice recorded calls.
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1).Updates(map[string]any{"cost_discount": 0.9, "invoice_vat_rate": 0.13}).Error)
			require.NoError(t, db.Where("channel_id = ?", 1).Delete(&model.ChannelModelOverride{}).Error)
			require.NoError(t, logDB.Create(&[]model.Log{

				{CreatedAt: start - 1, Type: model.LogTypeConsume, ChannelId: 1, Quota: 9000000},
				{CreatedAt: start + 86400, Type: model.LogTypeConsume, ChannelId: 1, Quota: 9000000},
				{CreatedAt: start + 100, Type: model.LogTypeError, ChannelId: 1, Quota: 9000000},
			}).Error)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("role", common.RoleRootUser)
			ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/gross-profit?start_date=%s&end_date=%s", time.Unix(start, 0).UTC().Format(time.DateOnly), time.Unix(start+86400, 0).UTC().Format(time.DateOnly)), nil)
			require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1000, time.Now()))
			require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1000, time.Now()))
			require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1000, time.Now()))
			GetGrossProfit(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool
				Message string
				Data    model.GrossProfitReport
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			assert.EqualValues(t, 6, response.Data.Summary.KnownRequests)
			assert.InDelta(t, 2.9, response.Data.Summary.KnownRevenueUSD, 1e-12)
			assert.InDelta(t, 1.2, response.Data.Summary.CostUSD, 1e-12)
			assert.InDelta(t, 2.9/1.06, response.Data.Summary.NetRevenueUSD, 1e-12)
			assert.InDelta(t, 1.2/1.03, response.Data.Summary.NetCostUSD, 1e-12)
			assert.InDelta(t, 2.9/1.06-1.2/1.03, response.Data.Summary.ProfitUSD, 1e-12)
			assert.InDelta(t, 2.9-2.9/1.06, response.Data.Summary.OutputVATUSD, 1e-12)
			assert.InDelta(t, 1.2-1.2/1.03, response.Data.Summary.InputVATUSD, 1e-12)
			assert.EqualValues(t, 4, response.Data.Summary.UnknownRequests)
			assert.InDelta(t, 0.6, response.Data.Summary.UnknownRevenueUSD, 1e-12)
			require.NotNil(t, response.Data.Summary.RevenueCoverage)
			assert.InDelta(t, 2.9/3.5, *response.Data.Summary.RevenueCoverage, 1e-12)
			require.Len(t, response.Data.Channels, 1)
			assert.Len(t, response.Data.Unknown, 3)
			assert.Len(t, response.Data.Series, 1)
			historical, err := model.GetGrossProfitReport(context.Background(), legacyDay, legacyTime.AddDate(0, 0, 1).Format(time.DateOnly))
			require.NoError(t, err)
			assert.EqualValues(t, 2, historical.Summary.UnknownRequests, "ledger and retained log must deduplicate; purged log must remain accounted for")
			assert.InDelta(t, 3.18, historical.Summary.UnknownRevenueUSD, 1e-12)
			assert.Zero(t, historical.Summary.KnownRequests)
			require.Len(t, historical.Unknown, 1)
			assert.Equal(t, "tax_snapshot_missing", historical.Unknown[0].Reason)
			var retainedLegacy []previousGrossProfitEvent
			require.NoError(t, db.Order("event_key ASC").Find(&retainedLegacy).Error)
			assert.ElementsMatch(t, legacy, retainedLegacy)

			// Replaying every retained source row does not change the totals.
			var storedLogs []model.Log
			require.NoError(t, logDB.Where("type = ?", model.LogTypeConsume).Find(&storedLogs).Error)
			keys, err := model.QueueGrossProfitLogs(context.Background(), storedLogs)
			require.NoError(t, err)
			for _, key := range keys {
				require.NoError(t, model.ApplyGrossProfitEvent(context.Background(), key))
			}
			repeated, err := model.GetGrossProfitReport(context.Background(), time.Unix(start, 0).UTC().Format(time.DateOnly), time.Unix(start+86400, 0).UTC().Format(time.DateOnly))
			require.NoError(t, err)
			assert.Equal(t, response.Data.Summary, repeated.Summary)

			for index, stage := range []string{(model.GrossProfitDaily{}).TableName(), (model.GrossProfitEvent{}).TableName()} {
				t.Run("rollback and recover "+stage, func(t *testing.T) {
					entry := model.Log{Type: model.LogTypeConsume, CreatedAt: start + int64(10+index)*86400, ChannelId: 1, Quota: 400000, RequestId: "rollback-" + stage,
						Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":2,"output_vat_rate":0.06,"input_vat_rate":0.03,"event_id":"rollback-` + stage + `","cost_usd":"0.3"}}}`}
					keys, err := model.QueueGrossProfitLogs(context.Background(), []model.Log{entry})
					require.NoError(t, err)
					hook := "gross-profit-fail-" + stage
					require.NoError(t, db.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
						if tx.Statement.Table == stage {
							tx.AddError(errors.New("injected rollup failure"))
						}
					}))
					err = model.ApplyGrossProfitEvent(context.Background(), keys[0])
					require.ErrorContains(t, err, "injected rollup failure")
					require.NoError(t, db.Callback().Update().Remove(hook))
					var event model.GrossProfitEvent
					require.NoError(t, db.First(&event, "event_key = ?", keys[0]).Error)
					assert.Zero(t, event.AppliedAt)
					var buckets int64
					require.NoError(t, db.Model(&model.GrossProfitDaily{}).Where("day = ?", event.Day).Count(&buckets).Error)
					assert.Zero(t, buckets, "failed transaction must roll back bucket and marker together")
					require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1000, time.Now()))
					require.NoError(t, model.ApplyGrossProfitEvent(context.Background(), keys[0]))
					var bucket model.GrossProfitDaily
					require.NoError(t, db.First(&bucket, "day = ?", event.Day).Error)
					assert.EqualValues(t, 1, bucket.Requests)
					assert.Equal(t, "0.8", bucket.RevenueUSD)
					assert.Equal(t, "0.3", bucket.CostUSD)
				})
			}

			t.Run("known and unknown buckets for the same channel remain isolated", func(t *testing.T) {
				entries := []model.Log{
					{Type: model.LogTypeConsume, CreatedAt: start + 21*86400, ChannelId: 1, Quota: 100000, Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":2,"output_vat_rate":0.06,"input_vat_rate":0.03,"event_id":"isolated-unknown-a","reason":"discount_unconfigured"}}}`},
					{Type: model.LogTypeConsume, CreatedAt: start + 21*86400, ChannelId: 1, Quota: 100000, Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":2,"output_vat_rate":0.06,"input_vat_rate":0.03,"event_id":"isolated-unknown-b","reason":"billing_metadata_missing"}}}`},
					{Type: model.LogTypeConsume, CreatedAt: start + 21*86400, ChannelId: 1, Quota: 400000, Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":2,"output_vat_rate":0.06,"input_vat_rate":0.03,"event_id":"isolated-known","cost_usd":"0.3"}}}`},
				}
				keys, err := model.QueueGrossProfitLogs(context.Background(), entries)
				require.NoError(t, err)
				for _, key := range keys {
					require.NoError(t, model.ApplyGrossProfitEvent(context.Background(), key))
				}
				day := time.Unix(start+21*86400, 0).UTC().Format(time.DateOnly)
				next := time.Unix(start+22*86400, 0).UTC().Format(time.DateOnly)
				report, err := model.GetGrossProfitReport(context.Background(), day, next)
				require.NoError(t, err)
				assert.EqualValues(t, 1, report.Summary.KnownRequests)
				assert.EqualValues(t, 2, report.Summary.UnknownRequests)
				assert.InDelta(t, 0.8, report.Summary.KnownRevenueUSD, 1e-12)
				assert.InDelta(t, 0.4, report.Summary.UnknownRevenueUSD, 1e-12)
				assert.InDelta(t, 0.3, report.Summary.CostUSD, 1e-12)
			})
			t.Run("concurrent events in the same bucket and duplicate replay", func(t *testing.T) {
				entries := []model.Log{
					{Type: model.LogTypeConsume, CreatedAt: start + 20*86400, ChannelId: 1, Quota: 400000, Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":2,"output_vat_rate":0.06,"input_vat_rate":0.03,"event_id":"parallel-a","cost_usd":"0.1"}}}`},
					{Type: model.LogTypeConsume, CreatedAt: start + 20*86400, ChannelId: 1, Quota: 400000, Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":2,"output_vat_rate":0.06,"input_vat_rate":0.03,"event_id":"parallel-b","cost_usd":"0.2"}}}`},
				}
				keys, err := model.QueueGrossProfitLogs(context.Background(), entries)
				require.NoError(t, err)
				begin := make(chan struct{})
				failures := make(chan error, 2)
				var workers sync.WaitGroup
				for _, key := range keys {
					workers.Go(func() { <-begin; failures <- model.ApplyGrossProfitEvent(context.Background(), key) })
				}
				close(begin)
				workers.Wait()
				close(failures)
				for err := range failures {
					require.NoError(t, err)
				}
				for _, key := range keys {
					require.NoError(t, model.ApplyGrossProfitEvent(context.Background(), key))
				}
				var bucket model.GrossProfitDaily
				require.NoError(t, db.First(&bucket, "day = ?", time.Unix(start+20*86400, 0).UTC().Format(time.DateOnly)).Error)
				assert.EqualValues(t, 2, bucket.Requests)
				assert.Equal(t, "1.6", bucket.RevenueUSD)
				assert.Equal(t, "0.3", bucket.CostUSD)
			})
			t.Run("failed source import preserves cursor and late historical logs are repaired", func(t *testing.T) {
				var before model.GrossProfitRollupState
				require.NoError(t, db.First(&before, "name = ?", model.GrossProfitRollupName).Error)
				entry := model.Log{Type: model.LogTypeConsume, CreatedAt: start - 10*86400, ChannelId: 1, Quota: 400000, RequestId: "late-committed-log", Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":2,"output_vat_rate":0.06,"input_vat_rate":0.03,"event_id":"late-historical","cost_usd":"0.1"}}}`}
				require.NoError(t, logDB.Create(&entry).Error)
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("gross-profit-import-failure", func(tx *gorm.DB) {
					if tx.Statement.Table == (model.GrossProfitEvent{}).TableName() {
						tx.AddError(errors.New("outbox unavailable"))
					}
				}))
				err := model.ReconcileGrossProfit(context.Background(), 2, time.Now().Add(25*time.Hour))
				require.ErrorContains(t, err, "outbox unavailable")
				require.NoError(t, db.Callback().Create().Remove("gross-profit-import-failure"))
				var after model.GrossProfitRollupState
				require.NoError(t, db.First(&after, "name = ?", model.GrossProfitRollupName).Error)
				assert.Equal(t, before.Revision, after.Revision)
				assert.Equal(t, before.CursorAt, after.CursorAt)
				require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1000, time.Now().Add(25*time.Hour)))
				require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1000, time.Now().Add(25*time.Hour)))
				var bucket model.GrossProfitDaily
				require.NoError(t, db.First(&bucket, "day = ?", time.Unix(entry.CreatedAt, 0).UTC().Format(time.DateOnly)).Error)
				assert.EqualValues(t, 1, bucket.Requests)
				assert.Equal(t, "0.1", bucket.CostUSD)
			})

			t.Run("small pages and deleted cursor boundary do not skip new logs", func(t *testing.T) {
				require.NoError(t, db.Model(&model.GrossProfitRollupState{}).Where("name = ?", model.GrossProfitRollupName).Updates(map[string]any{"cursor_at": -1, "cursor_id": 0, "cursor_request_id": "", "cursor_offset": 0, "backfill_complete": false}).Error)
				var count int64
				require.NoError(t, logDB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&count).Error)
				for range count + 2 {
					require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1, time.Now().Add(25*time.Hour)))
				}
				var cursor model.GrossProfitRollupState
				require.NoError(t, db.First(&cursor, "name = ?", model.GrossProfitRollupName).Error)
				assert.True(t, cursor.BackfillComplete)
				require.NoError(t, logDB.Where("created_at = ? AND id = ? AND request_id = ?", cursor.CursorAt, cursor.CursorID, cursor.CursorRequestID).Delete(&model.Log{}).Error)
				entry := model.Log{Type: model.LogTypeConsume, CreatedAt: cursor.CursorAt + 1, ChannelId: 1, Quota: 400000, RequestId: "after-log-retention", Other: `{"quota_per_unit":500000,"root_info":{"gross_profit":{"version":2,"output_vat_rate":0.06,"input_vat_rate":0.03,"event_id":"after-log-retention","cost_usd":"0.1"}}}`}
				require.NoError(t, logDB.Create(&entry).Error)
				require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1, time.Now().Add(25*time.Hour)))
				require.NoError(t, model.ReconcileGrossProfit(context.Background(), 1, time.Now().Add(25*time.Hour)))
				keys, err := model.QueueGrossProfitLogs(context.Background(), []model.Log{entry})
				require.NoError(t, err)
				var event model.GrossProfitEvent
				require.NoError(t, db.First(&event, "event_key = ?", keys[0]).Error)
				assert.NotZero(t, event.AppliedAt)
			})
			t.Run("VAT-inclusive amounts split using frozen deductible invoice rates", func(t *testing.T) {
				for index, tc := range []struct {
					rate         *float64
					costDiscount float64
					netCost      float64
					reason       string
				}{
					{rate: common.GetPointer(0.06), costDiscount: 0.5, netCost: 0.5},
					{rate: common.GetPointer(0.03), costDiscount: 0.5, netCost: 0.53 / 1.03},
					{rate: common.GetPointer(0.01), costDiscount: 0.5, netCost: 0.53 / 1.01},
					{rate: common.GetPointer(0.0), costDiscount: 0.5, netCost: 0.53},
					{costDiscount: 0.5, reason: "tax_rate_unconfigured"},
					{costDiscount: 0, netCost: 0},
				} {
					require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1).Updates(map[string]any{"cost_discount": tc.costDiscount, "invoice_vat_rate": tc.rate}).Error)
					other := model.NewLogOther()
					other.SetPublic("group_ratio", 1)
					ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
					ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
					model.RecordConsumeLog(ctx, 1, model.RecordConsumeLogParams{ChannelId: 1, ModelName: "model-a", Quota: 530000, Other: other})
					var entry model.Log
					require.NoError(t, logDB.Order("id DESC").First(&entry).Error)
					// Give the immutable snapshot its own business day to inspect exact totals.
					entry.CreatedAt = start + int64(30+index)*86400
					var parsed map[string]any
					require.NoError(t, common.UnmarshalJsonStr(entry.Other, &parsed))
					parsed["root_info"].(map[string]any)["gross_profit"].(map[string]any)["event_id"] = fmt.Sprintf("vat-case-%d", index)
					encoded, err := common.Marshal(parsed)
					require.NoError(t, err)
					entry.Other = string(encoded)
					keys, err := model.QueueGrossProfitLogs(context.Background(), []model.Log{entry})
					require.NoError(t, err)
					require.NoError(t, model.ApplyGrossProfitEvent(context.Background(), keys[0]))
					require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1).Update("invoice_vat_rate", 0.13).Error)
					keys, err = model.QueueGrossProfitLogs(context.Background(), []model.Log{entry})
					require.NoError(t, err)
					require.NoError(t, model.ApplyGrossProfitEvent(context.Background(), keys[0]))
					report, err := model.GetGrossProfitReport(context.Background(), time.Unix(entry.CreatedAt, 0).UTC().Format(time.DateOnly), time.Unix(entry.CreatedAt+86400, 0).UTC().Format(time.DateOnly))
					require.NoError(t, err)
					if tc.reason != "" {
						assert.Zero(t, report.Summary.KnownRequests)
						assert.EqualValues(t, 1, report.Summary.UnknownRequests)
						require.Len(t, report.Unknown, 1)
						assert.Equal(t, tc.reason, report.Unknown[0].Reason)
						assert.InDelta(t, 1.06, report.Summary.UnknownRevenueUSD, 1e-12)
						continue
					}
					assert.EqualValues(t, 1, report.Summary.KnownRequests)
					assert.InDelta(t, 1.06, report.Summary.KnownRevenueUSD, 1e-12)
					assert.InDelta(t, 1, report.Summary.NetRevenueUSD, 1e-12)
					assert.InDelta(t, 0.06, report.Summary.OutputVATUSD, 1e-12)
					assert.InDelta(t, tc.netCost, report.Summary.NetCostUSD, 1e-12)
					assert.InDelta(t, 1-tc.netCost, report.Summary.ProfitUSD, 1e-12)
					assert.InDelta(t, report.Summary.CostUSD-tc.netCost, report.Summary.InputVATUSD, 1e-12)
					require.NotNil(t, report.Summary.ProfitMargin)
					assert.InDelta(t, 1-tc.netCost, *report.Summary.ProfitMargin, 1e-12)
				}
			})
			require.NoError(t, model.MigrateGrossProfit(db))
			require.NoError(t, model.MigrateGrossProfit(db))
			// A report remains available with the source log table gone: no query-time log scans.
			require.NoError(t, logDB.Migrator().DropTable(&model.Log{}))
			wide, err := model.GetGrossProfitReport(context.Background(), "1900-01-01", "2100-01-01")
			require.NoError(t, err)
			assert.True(t, wide.Aggregation.BackfillComplete)
			assert.Zero(t, wide.Aggregation.PendingEvents)
			for _, query := range []string{"start_date=bad&end_date=2026-11-08", "start_date=2026-11-08&end_date=2026-11-07", "start_date=2026-11-07&end_date=bad"} {
				r := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(r)
				c.Set("role", common.RoleRootUser)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/gross-profit?"+query, nil)
				GetGrossProfit(c)
				assert.False(t, gjson.Get(r.Body.String(), "success").Bool(), r.Body.String())
			}
		})
	}
}
