package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
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
	EventKey   string `gorm:"type:varchar(64);primaryKey"`
	Day        string `gorm:"type:varchar(10);not null"`
	ChannelID  int    `gorm:"not null"`
	Reason     string `gorm:"type:varchar(64);not null"`
	RevenueUSD string `gorm:"type:text;not null"`
	CostUSD    string `gorm:"type:text;not null"`
	AppliedAt  int64  `gorm:"bigint;not null;index:idx_gross_profit_pending,priority:1"`
	CreatedAt  int64  `gorm:"bigint;not null;index:idx_gross_profit_pending,priority:2"`
}

// Text decimal amounts preserve the same precision on SQLite, MySQL and
// PostgreSQL. Updates take a row lock and add with decimal, rather than floats.
type GrossProfitDaily struct {
	Day        string `gorm:"type:varchar(10);primaryKey"`
	ChannelID  int    `gorm:"primaryKey;autoIncrement:false"`
	Reason     string `gorm:"type:varchar(64);primaryKey"` // empty means known cost
	Requests   int64  `gorm:"bigint;not null"`
	RevenueUSD string `gorm:"type:text;not null"`
	CostUSD    string `gorm:"type:text;not null"`
}

// The timezone is frozen once so nodes with different host timezones agree on
// day boundaries. Explicit TZ is respected; an unset TZ defaults to UTC.
type GrossProfitRollupState struct {
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

var grossProfitSQLiteWriter sync.Mutex

func MigrateGrossProfit(db *gorm.DB) error {
	if err := db.AutoMigrate(&GrossProfitEvent{}, &GrossProfitDaily{}, &GrossProfitRollupState{}); err != nil {
		return err
	}
	timezone := os.Getenv("TZ")
	if timezone == "" {
		timezone = "UTC"
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return fmt.Errorf("gross profit timezone: %w", err)
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&GrossProfitRollupState{
		Name: "consume", Timezone: timezone, CursorAt: -1, FullScanStartedAt: common.GetTimestamp(),
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
	if err := DB.WithContext(ctx).First(&state, "name = ?", "consume").Error; err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(state.Timezone)
	if err != nil {
		return nil, err
	}
	events := make([]GrossProfitEvent, 0, len(logs))
	keys := make([]string, 0, len(logs))
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
			Day:       time.Unix(entry.CreatedAt, 0).In(location).Format(time.DateOnly),
			ChannelID: entry.ChannelId, Reason: "snapshot_missing", CostUSD: "0",
			RevenueUSD: decimal.NewFromInt(int64(entry.Quota)).Div(unit).String(), CreatedAt: common.GetTimestamp(),
		}
		var snapshot consumeGrossProfit
		if raw := metadata.Get("root_info.gross_profit"); raw.Exists() && common.UnmarshalJsonStr(raw.Raw, &snapshot) == nil && snapshot.Version == 1 {
			event.Reason = snapshot.Reason
			if snapshot.CostUSD != nil {
				cost, err := decimal.NewFromString(*snapshot.CostUSD)
				if err == nil && !cost.IsNegative() {
					event.Reason, event.CostUSD = "", cost.String()
				} else {
					event.Reason = "billing_metadata_missing"
				}
			} else if event.Reason == "" {
				event.Reason = "billing_metadata_missing"
			}
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
		keys = append(keys, event.EventKey)
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, nil
	}
	if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&events, 100).Error; err != nil {
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
		if event.AppliedAt != 0 {
			return nil
		}
		bucket := GrossProfitDaily{Day: event.Day, ChannelID: event.ChannelID, Reason: event.Reason, RevenueUSD: "0", CostUSD: "0"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&bucket).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&bucket, "day = ? AND channel_id = ? AND reason = ?", event.Day, event.ChannelID, event.Reason).Error; err != nil {
			return err
		}
		revenue, err := decimal.NewFromString(bucket.RevenueUSD)
		if err != nil {
			return err
		}
		cost, err := decimal.NewFromString(bucket.CostUSD)
		if err != nil {
			return err
		}
		eventRevenue, err := decimal.NewFromString(event.RevenueUSD)
		if err != nil {
			return err
		}
		eventCost, err := decimal.NewFromString(event.CostUSD)
		if err != nil {
			return err
		}
		// Reason is an empty string for known costs. Explicitly bind every key:
		// GORM omits zero-valued composite primary keys from Model updates.
		if err := tx.Model(&GrossProfitDaily{}).Where("day = ? AND channel_id = ? AND reason = ?", event.Day, event.ChannelID, event.Reason).Updates(map[string]any{
			"requests":    bucket.Requests + 1,
			"revenue_usd": revenue.Add(eventRevenue).String(), "cost_usd": cost.Add(eventCost).String(),
		}).Error; err != nil {
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
	if err := DB.WithContext(ctx).Where("applied_at = ?", 0).Order("created_at ASC").Limit(batchSize).Find(&pending).Error; err != nil {
		return err
	}
	var failures error
	for _, event := range pending {
		if err := ApplyGrossProfitEvent(ctx, event.EventKey); err != nil {
			failures = errors.Join(failures, err)
		}
	}
	var state GrossProfitRollupState
	if err := DB.WithContext(ctx).First(&state, "name = ?", "consume").Error; err != nil {
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
