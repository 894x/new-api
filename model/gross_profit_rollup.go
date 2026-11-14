package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GrossProfitEvent is a durable outbox and deduplication ledger in the primary
// database. Keep applied events: removing them would allow a replay to double count.
type GrossProfitEvent struct {
	EventKey        string `gorm:"type:varchar(64);primaryKey"`
	Day             string `gorm:"type:varchar(10);not null"`
	ChannelID       int    `gorm:"not null"`
	Reason          string `gorm:"type:varchar(64);not null"`
	RevenueUSD      string `gorm:"type:text;not null"`
	CostUSD         string `gorm:"type:text;not null"`
	NetRevenueUSD   string `gorm:"type:text;not null"`
	NetCostUSD      string `gorm:"type:text;not null"`
	AppliedAt       int64  `gorm:"bigint;not null;index:idx_gross_profit_time_pending,priority:1"`
	SourceCreatedAt *int64 `gorm:"bigint;index:idx_gross_profit_time_source;index:idx_gross_profit_time_pending,priority:2"`
	CreatedAt       int64  `gorm:"bigint;not null;index:idx_gross_profit_time_pending,priority:3"`
}

// Preserve previous daily ledgers while recovering exact source timestamps.
func (GrossProfitEvent) TableName() string { return "gross_profit_time_events" }

// Text decimal amounts preserve the same precision on SQLite, MySQL and
// PostgreSQL. Updates take a row lock and add with decimal, rather than floats.
type GrossProfitBucket struct {
	BucketTs      int64  `gorm:"bigint;primaryKey;autoIncrement:false"`
	ChannelID     int    `gorm:"primaryKey;autoIncrement:false"`
	Reason        string `gorm:"type:varchar(64);primaryKey"` // empty means known cost and VAT
	Requests      int64  `gorm:"bigint;not null"`
	RevenueUSD    string `gorm:"type:text;not null"`
	CostUSD       string `gorm:"type:text;not null"`
	NetRevenueUSD string `gorm:"type:text;not null"`
	NetCostUSD    string `gorm:"type:text;not null"`
}

func (GrossProfitBucket) TableName() string { return "gross_profit_time_buckets" }

const GrossProfitRollupName = "consume_vat_time_v3"
const grossProfitBucketSeconds int64 = 15 * 60

// Storage uses UTC buckets. Each report chooses the browser's business timezone.
type GrossProfitRollupState struct {
	Name                   string `gorm:"type:varchar(32);primaryKey"`
	Timezone               string `gorm:"type:varchar(64);not null"`
	CursorAt               int64  `gorm:"bigint;not null"`
	CursorID               int    `gorm:"bigint;not null"`
	CursorOffset           int    `gorm:"bigint;not null"`
	CursorRequestID        string `gorm:"type:varchar(64);not null"`
	FullScanStartedAt      int64  `gorm:"bigint;not null"`
	BackfillComplete       bool
	LegacyCursor           string `gorm:"type:varchar(64);not null;default:''"`
	LegacyBackfillComplete bool
	VATCursor              string `gorm:"type:varchar(64);not null;default:''"`
	VATBackfillComplete    bool
	Revision               int64 `gorm:"bigint;not null"`
	UpdatedAt              int64 `gorm:"bigint;not null"`
}

var grossProfitSQLiteWriter sync.Mutex

func MigrateGrossProfit(db *gorm.DB) error {
	if err := db.AutoMigrate(&GrossProfitEvent{}, &GrossProfitBucket{}, &GrossProfitRollupState{}); err != nil {
		return err
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&GrossProfitRollupState{
		Name: GrossProfitRollupName, Timezone: "UTC", CursorAt: -1, FullScanStartedAt: common.GetTimestamp(),
		LegacyBackfillComplete: !db.Migrator().HasTable("gross_profit_events"),
		VATBackfillComplete:    !db.Migrator().HasTable("gross_profit_vat_events"),
	}).Error
}

// QueueGrossProfitLogs persists before attempting rollup updates. OnConflict
// never replaces an applied event with a pending one. Historical log repair
// uses the same operation and never looks up today's discounts.
func QueueGrossProfitLogs(ctx context.Context, logs []Log) ([]string, error) {
	if len(logs) == 0 {
		return nil, nil
	}
	var state GrossProfitRollupState
	if err := DB.WithContext(ctx).First(&state, "name = ?", GrossProfitRollupName).Error; err != nil {
		return nil, err
	}
	events := make([]GrossProfitEvent, 0, len(logs))
	keys := make([]string, 0, len(logs))
	seen := make(map[string]struct{}, len(logs))
	for _, entry := range logs {
		if entry.Type != LogTypeConsume {
			continue
		}
		metadata := gjson.Parse(entry.Other)
		unit := decimal.NewFromFloat(common.QuotaPerUnit)
		if raw := metadata.Get("quota_per_unit"); raw.Exists() {
			parsed, err := decimal.NewFromString(raw.Raw)
			if err == nil && parsed.IsPositive() {
				unit = parsed
			}
		}
		if !unit.IsPositive() {
			return nil, errors.New("invalid quota unit for gross profit")
		}
		event := GrossProfitEvent{
			Day:             time.Unix(entry.CreatedAt, 0).UTC().Format(time.DateOnly),
			SourceCreatedAt: common.GetPointer(entry.CreatedAt),
			ChannelID:       entry.ChannelId, Reason: "snapshot_missing", CostUSD: "0",
			RevenueUSD: decimal.NewFromInt(int64(entry.Quota)).Div(unit).String(), CreatedAt: common.GetTimestamp(),
			NetRevenueUSD: "0", NetCostUSD: "0",
		}
		var snapshot consumeGrossProfit
		raw := metadata.Get("root_info.gross_profit")
		snapshotKnown := raw.Exists() && common.UnmarshalJsonStr(raw.Raw, &snapshot) == nil && (snapshot.Version == 1 || snapshot.Version == 2)
		cost := decimal.Zero
		var costErr error
		if snapshot.CostUSD != nil {
			cost, costErr = decimal.NewFromString(*snapshot.CostUSD)
		}
		if snapshotKnown && snapshot.CostUSD != nil && costErr == nil && !cost.IsNegative() {
			event.CostUSD = cost.String()
		}
		switch {
		case !snapshotKnown:
			event.Reason = "snapshot_missing"
		case snapshot.CostUSD == nil:
			event.Reason = snapshot.Reason
			if event.Reason == "" {
				event.Reason = "billing_metadata_missing"
			}
		case costErr != nil || cost.IsNegative():
			event.Reason = "billing_metadata_missing"
		case snapshot.Version != 2 || snapshot.OutputVATRate == nil:
			event.Reason = "tax_snapshot_missing"
		case ValidateChannelInvoiceVATRate(snapshot.OutputVATRate) != nil || ValidateChannelInvoiceVATRate(snapshot.InputVATRate) != nil:
			event.Reason = "billing_metadata_missing"
		case snapshot.InputVATRate == nil && !cost.IsZero():
			event.Reason = "tax_rate_unconfigured"
		default:
			revenue := decimal.NewFromInt(int64(entry.Quota)).Div(unit)
			event.NetRevenueUSD = revenue.Div(decimal.NewFromInt(1).Add(decimal.NewFromFloat(*snapshot.OutputVATRate))).String()
			inputRate := decimal.Zero
			if snapshot.InputVATRate != nil {
				inputRate = decimal.NewFromFloat(*snapshot.InputVATRate)
			}
			event.NetCostUSD = cost.Div(decimal.NewFromInt(1).Add(inputRate)).String()
			event.Reason = ""
		}
		identity := snapshot.EventID
		if identity == "" {
			// Old logs have no explicit event identity. Include the full immutable
			// row, not just request_id: batch/task calls can share request IDs.
			data, err := common.Marshal([]any{entry.Id, entry.CreatedAt, entry.UserId, entry.RequestId, entry.ChannelId, entry.Quota, entry.ModelName, entry.Other})
			if err != nil {
				return nil, err
			}
			identity = string(data)
		}
		event.EventKey = fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
		if _, duplicate := seen[event.EventKey]; duplicate {
			continue
		}
		seen[event.EventKey] = struct{}{}
		keys = append(keys, event.EventKey)
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, nil
	}
	// Imported ledgers retain their immutable monetary snapshots. Replaying an
	// original log supplies only its missing timestamp and source date.
	source := "excluded.source_created_at"
	day := "excluded.day"
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		source, day = "VALUES(source_created_at)", "VALUES(day)"
	}
	table := (GrossProfitEvent{}).TableName()
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_key"}},
		DoUpdates: clause.Set{
			{Column: clause.Column{Name: "day"}, Value: gorm.Expr("CASE WHEN " + table + ".source_created_at IS NULL THEN " + day + " ELSE " + table + ".day END")},
			{Column: clause.Column{Name: "source_created_at"}, Value: gorm.Expr("COALESCE(" + table + ".source_created_at, " + source + ")")},
		},
	}).CreateInBatches(&events, 100).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// ApplyGrossProfitEvent atomically updates the daily bucket and marks the
// event applied. A rollback leaves it pending, and concurrent/repeated workers
// cannot add the same event twice.
func ApplyGrossProfitEvent(ctx context.Context, key string) error {
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		grossProfitSQLiteWriter.Lock()
		defer grossProfitSQLiteWriter.Unlock()
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event GrossProfitEvent
		if err := lockForUpdate(tx).First(&event, "event_key = ?", key).Error; err != nil {
			return err
		}
		if event.AppliedAt != 0 || event.SourceCreatedAt == nil {
			return nil
		}
		bucket := GrossProfitBucket{BucketTs: grossProfitBucketStart(*event.SourceCreatedAt), ChannelID: event.ChannelID, Reason: event.Reason, RevenueUSD: "0", CostUSD: "0", NetRevenueUSD: "0", NetCostUSD: "0"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&bucket).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&bucket, "bucket_ts = ? AND channel_id = ? AND reason = ?", bucket.BucketTs, event.ChannelID, event.Reason).Error; err != nil {
			return err
		}
		updates := map[string]any{"requests": bucket.Requests + 1}
		for _, amount := range []struct{ column, bucket, event string }{
			{"revenue_usd", bucket.RevenueUSD, event.RevenueUSD}, {"cost_usd", bucket.CostUSD, event.CostUSD},
			{"net_revenue_usd", bucket.NetRevenueUSD, event.NetRevenueUSD}, {"net_cost_usd", bucket.NetCostUSD, event.NetCostUSD},
		} {
			current, err := decimal.NewFromString(amount.bucket)
			if err != nil {
				return err
			}
			increment, err := decimal.NewFromString(amount.event)
			if err != nil {
				return err
			}
			updates[amount.column] = current.Add(increment).String()
		}
		// Reason is an empty string for known costs. Explicitly bind every key:
		// GORM omits zero-valued composite primary keys from Model updates.
		if err := tx.Model(&GrossProfitBucket{}).Where("bucket_ts = ? AND channel_id = ? AND reason = ?", bucket.BucketTs, event.ChannelID, event.Reason).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&event).Update("applied_at", common.GetTimestamp()).Error
	})
}

// ReconcileGrossProfit retries durable pending events, then imports one log
// page. The cursor advances only after every source row is durably queued.
// Daily replay from the beginning also catches late commits/imported old logs.
// All work is bounded per pass; report queries never invoke this scanner.
func ReconcileGrossProfit(ctx context.Context, batchSize int, now time.Time) error {
	if batchSize <= 0 || batchSize > 1000 {
		return errors.New("invalid gross profit reconciliation batch size")
	}
	var pending []GrossProfitEvent
	if err := DB.WithContext(ctx).Where("applied_at = ? AND source_created_at IS NOT NULL", 0).Order("source_created_at ASC, created_at ASC").Limit(batchSize).Find(&pending).Error; err != nil {
		return err
	}
	var failures error
	for _, event := range pending {
		if err := ApplyGrossProfitEvent(ctx, event.EventKey); err != nil {
			failures = errors.Join(failures, err)
		}
	}
	var state GrossProfitRollupState
	if err := DB.WithContext(ctx).First(&state, "name = ?", GrossProfitRollupName).Error; err != nil {
		return errors.Join(failures, err)
	}
	if !state.VATBackfillComplete || !state.LegacyBackfillComplete {
		// Import the VAT ledger first so a pre-VAT duplicate cannot replace its
		// richer snapshot. Original logs later supply exact timestamps; a
		// purged source stays preserved and explicitly unassigned, never guessed.
		sourceTable, cursor := "gross_profit_vat_events", state.VATCursor
		columns := []string{"event_key", "day", "channel_id", "reason", "revenue_usd", "cost_usd", "net_revenue_usd", "net_cost_usd", "created_at"}
		if state.VATBackfillComplete {
			sourceTable, cursor = "gross_profit_events", state.LegacyCursor
			columns = []string{"event_key", "day", "channel_id", "reason", "revenue_usd", "cost_usd", "created_at"}
		}
		var legacy []GrossProfitEvent
		if err := DB.WithContext(ctx).Table(sourceTable).Select(columns).
			Where("event_key > ?", cursor).Order("event_key ASC").Limit(batchSize).Find(&legacy).Error; err != nil {
			return errors.Join(failures, err)
		}
		for i := range legacy {
			if state.VATBackfillComplete {
				if legacy[i].Reason == "" {
					legacy[i].Reason = "tax_snapshot_missing"
				}
				legacy[i].NetRevenueUSD, legacy[i].NetCostUSD = "0", "0"
			}
		}
		err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if len(legacy) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&legacy, 100).Error; err != nil {
					return err
				}
				cursor = legacy[len(legacy)-1].EventKey
			}
			updates := map[string]any{
				"revision": state.Revision + 1, "updated_at": now.Unix(),
			}
			if state.VATBackfillComplete {
				updates["legacy_cursor"], updates["legacy_backfill_complete"] = cursor, len(legacy) < batchSize
			} else {
				updates["vat_cursor"], updates["vat_backfill_complete"] = cursor, len(legacy) < batchSize
			}
			return tx.Model(&GrossProfitRollupState{}).Where("name = ? AND revision = ?", state.Name, state.Revision).Updates(updates).Error
		})
		return errors.Join(failures, err)
	}
	if state.BackfillComplete && now.Unix()-state.FullScanStartedAt >= 86400 {
		state.CursorAt, state.CursorID, state.CursorRequestID = -1, 0, ""
		state.CursorOffset = 0
		state.FullScanStartedAt = now.Unix()
	}
	var logs []Log
	query := LOG_DB.WithContext(ctx).Model(&Log{}).Select("id", "user_id", "created_at", "type", "model_name", "quota", "channel_id", "request_id", "other").Where("type = ?", LogTypeConsume)
	if state.CursorAt >= 0 {
		// Offset applies only to equal-key rows, never to newer rows. Retention
		// may delete the previous boundary row while new logs are arriving.
		if err := query.Session(&gorm.Session{}).Where("created_at = ? AND id = ? AND request_id = ?", state.CursorAt, state.CursorID, state.CursorRequestID).
			Order("other ASC").Offset(state.CursorOffset).Limit(batchSize).Find(&logs).Error; err != nil {
			return errors.Join(failures, err)
		}
		if len(logs) < batchSize {
			var newer []Log
			if err := query.Session(&gorm.Session{}).Where("created_at > ? OR (created_at = ? AND id > ?) OR (created_at = ? AND id = ? AND request_id > ?)",
				state.CursorAt, state.CursorAt, state.CursorID, state.CursorAt, state.CursorID, state.CursorRequestID).
				Order("created_at ASC, id ASC, request_id ASC, other ASC").Limit(batchSize - len(logs)).Find(&newer).Error; err != nil {
				return errors.Join(failures, err)
			}
			logs = append(logs, newer...)
		}
	} else if err := query.Order("created_at ASC, id ASC, request_id ASC, other ASC").Limit(batchSize).Find(&logs).Error; err != nil {
		return errors.Join(failures, err)
	}
	if _, err := QueueGrossProfitLogs(ctx, logs); err != nil {
		return errors.Join(failures, err)
	}
	if len(logs) > 0 {
		last := logs[len(logs)-1]
		offset := 0
		for i := len(logs) - 1; i >= 0; i-- {
			if logs[i].CreatedAt != last.CreatedAt || logs[i].Id != last.Id || logs[i].RequestId != last.RequestId {
				break
			}
			offset++
		}
		if last.CreatedAt == state.CursorAt && last.Id == state.CursorID && last.RequestId == state.CursorRequestID {
			offset += state.CursorOffset
		}
		state.CursorOffset = offset
		state.CursorAt, state.CursorID, state.CursorRequestID = last.CreatedAt, last.Id, last.RequestId
	}
	if len(logs) < batchSize {
		state.BackfillComplete = true
	}
	result := DB.WithContext(ctx).Model(&GrossProfitRollupState{}).Where("name = ? AND revision = ?", state.Name, state.Revision).Updates(map[string]any{
		"cursor_at": state.CursorAt, "cursor_id": state.CursorID, "cursor_request_id": state.CursorRequestID, "cursor_offset": state.CursorOffset,
		"full_scan_started_at": state.FullScanStartedAt, "backfill_complete": state.BackfillComplete,
		"revision": state.Revision + 1, "updated_at": now.Unix(),
	})
	return errors.Join(failures, result.Error)
}
