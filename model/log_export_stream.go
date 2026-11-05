package model

import (
	"context"
	"database/sql"
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// VisitLogsForExport releases the SQL connection between bounded keyset pages.
// Downstream histories are adjacent by request ID, including attempts outside
// the report filters, so callers only retain one request's terminal record.
func VisitLogsForExport(ctx context.Context, query LogExportQuery, userID int, downstream bool, limit int, visit func([]*Log) error, progress func(processed, total int64) error) (int64, error) {
	if limit <= 0 || userID < 0 {
		return 0, errors.New("invalid log export scope")
	}
	var scope *gorm.DB
	var err error
	if userID > 0 {
		scope, err = userConsumptionExportScope(ctx, userID, SelfLogExportQuery{StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp, ModelName: query.ModelName, TokenName: query.TokenName, RequestID: query.RequestID})
	} else {
		if downstream {
			query.ChannelID = 0
			query.UpstreamRequestID = ""
		}
		scope, err = administrativeLogExportScope(ctx, query)
	}
	if err != nil {
		return 0, err
	}
	logTable := LOG_DB.NamingStrategy.TableName("Log")
	scope = scope.Table(logTable + " AS logs")
	requestKey := "logs.request_id"
	if common.UsingLogDatabase(common.DatabaseTypeMySQL) {
		requestKey = "BINARY logs.request_id"
	}
	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		requestKey = `logs.request_id COLLATE "C"`
	}
	clickHouse := common.UsingLogDatabase(common.DatabaseTypeClickHouse)
	var snapshotID int
	if !clickHouse {
		if err = LOG_DB.WithContext(ctx).Model(&Log{}).Select("COALESCE(MAX(id), 0)").Scan(&snapshotID).Error; err != nil {
			return 0, err
		}
		scope = scope.Where("logs.id <= ?", snapshotID)
	}
	var total int64
	if err = scope.Count(&total).Error; err != nil || total > int64(limit) {
		return total, err
	}
	if downstream && !clickHouse {
		if progress != nil {
			if err = progress(0, total); err != nil {
				return 0, err
			}
		}
		return visitDownstreamLogsForExport(ctx, scope, logTable, requestKey, snapshotID, limit, visit, total, progress)
	}
	source := scope
	if downstream {
		matching := scope.Session(&gorm.Session{}).Select("logs.request_id").Where("logs.request_id <> ?", "")
		emptyMatches := scope.Session(&gorm.Session{}).Where("logs.request_id = ?", "")
		condition := LOG_DB.WithContext(ctx).Where("logs.request_id <> ? AND logs.request_id IN (?)", "", matching).Or(emptyMatches)
		source = LOG_DB.WithContext(ctx).Model(&Log{}).Table(logTable+" AS logs").Where("logs.type IN ?", []int{LogTypeConsume, LogTypeError}).Where(condition)
		if !clickHouse {
			source = source.Where("logs.id <= ?", snapshotID)
		}
	}
	channelNames := make(map[int]string)
	if userID == 0 && !downstream {
		var channels []struct {
			ID   int
			Name string
		}
		if err = DB.WithContext(ctx).Table(DB.NamingStrategy.TableName("Channel")).Select("id, name").Find(&channels).Error; err != nil {
			return 0, err
		}
		for _, channel := range channels {
			channelNames[channel.ID] = channel.Name
		}
	}
	if userID > 0 {
		columns := []string{"created_at", "type", "request_id", "token_name", "model_name", "prompt_tokens", "completion_tokens", "quota", "other"}
		if !clickHouse {
			columns = append(columns, "id")
		}
		source = source.Select(columns)
	}
	var cursor *sql.Rows
	if progress != nil {
		if clickHouse && downstream {
			// Expanded retry histories can exceed the matching report scope.
			// Count the actual cursor source so its percentage has a real denominator.
			if err = source.Session(&gorm.Session{}).Count(&total).Error; err != nil || total > int64(limit) {
				return total, err
			}
		}
		if err = progress(0, total); err != nil {
			return 0, err
		}
	}
	if clickHouse {
		order := "logs.created_at asc, logs.request_id asc"
		if downstream {
			order = "logs.request_id asc, logs.created_at asc"
		}
		cursor, err = source.Order(order).Limit(limit + 1).Rows()
		if err != nil {
			return 0, err
		}
		defer cursor.Close()
	}
	var scanned int64
	var last Log
	for {
		if err = ctx.Err(); err != nil {
			return scanned, err
		}
		batch := make([]*Log, 0, 1000)
		if cursor != nil {
			for len(batch) < cap(batch) && cursor.Next() {
				var item Log
				if err = LOG_DB.ScanRows(cursor, &item); err != nil {
					return scanned, err
				}
				batch = append(batch, &item)
			}
			if err = cursor.Err(); err != nil {
				return scanned, err
			}
		} else {
			page := source.Session(&gorm.Session{}).Where("logs.id > ?", last.Id).Order("logs.id asc")
			if err = page.Limit(min(1000, limit+1-int(scanned))).Find(&batch).Error; err != nil {
				return scanned, err
			}
		}
		if len(batch) == 0 {
			return scanned, nil
		}
		scanned += int64(len(batch))
		if scanned > int64(limit) {
			return scanned, nil
		}
		for _, item := range batch {
			if userID == 0 && !downstream {
				item.ChannelName = channelNames[item.ChannelId]
			}
		}
		if err = visit(batch); err != nil {
			return scanned, err
		}
		if progress != nil {
			if err = progress(scanned, total); err != nil {
				return scanned, err
			}
		}
		last = *batch[len(batch)-1]
	}
}

// visitDownstreamLogsForExport expands only a bounded batch of matching request
// IDs. A global IN subquery combined with empty-ID matches prevents a cheap
// first page on large histories. The earliest matching log owns each request,
// so retries in later candidate pages cannot export that request again.
func visitDownstreamLogsForExport(ctx context.Context, scope *gorm.DB, logTable, requestKey string, snapshotID, limit int, visit func([]*Log) error, total int64, progress func(processed, total int64) error) (int64, error) {
	var scanned int64
	var processed int64
	lastCandidateID := 0
	for {
		var candidates []Log
		err := scope.Session(&gorm.Session{}).Select("logs.id, logs.request_id").
			Where("logs.id > ?", lastCandidateID).Order("logs.id asc").Limit(1000).Find(&candidates).Error
		if err != nil || len(candidates) == 0 {
			return scanned, err
		}
		candidateEnd := candidates[len(candidates)-1].Id
		requestIDs := make([]string, 0, len(candidates))
		emptyIDs := make([]int, 0)
		seen := make(map[string]bool, len(candidates))
		for _, item := range candidates {
			if item.RequestId == "" {
				emptyIDs = append(emptyIDs, item.Id)
			} else if !seen[item.RequestId] {
				seen[item.RequestId] = true
				requestIDs = append(requestIDs, item.RequestId)
			}
		}
		ownedIDs := make([]string, 0, len(requestIDs))
		if len(requestIDs) > 0 {
			var firstMatches []Log
			matching := scope.Session(&gorm.Session{}).
				Select("MIN(logs.id) AS id, "+requestKey+" AS request_id").
				Where("logs.request_id IN ?", requestIDs)
			if common.UsingLogDatabase(common.DatabaseTypeMySQL) {
				// Keep the indexed predicate as well as exact, case-sensitive IDs.
				matching = matching.Where(requestKey+" IN ?", requestIDs)
			}
			if err = matching.Group(requestKey).Find(&firstMatches).Error; err != nil {
				return scanned, err
			}
			for _, item := range firstMatches {
				if item.Id > lastCandidateID && item.Id <= candidateEnd {
					ownedIDs = append(ownedIDs, item.RequestId)
				}
			}
		}
		lastCandidateID = candidateEnd
		if len(ownedIDs) == 0 && len(emptyIDs) == 0 {
			processed += int64(len(candidates))
			if progress != nil {
				if err = progress(processed, total); err != nil {
					return scanned, err
				}
			}
			continue
		}
		histories := LOG_DB.WithContext(ctx).Model(&Log{}).Table(logTable+" AS logs").
			Where("logs.type IN ?", []int{LogTypeConsume, LogTypeError}).Where("logs.id <= ?", snapshotID)
		switch {
		case len(ownedIDs) == 0:
			histories = histories.Where("logs.id IN ?", emptyIDs)
		case len(emptyIDs) == 0:
			histories = histories.Where("logs.request_id IN ?", ownedIDs)
		default:
			histories = histories.Where("logs.request_id IN ? OR logs.id IN ?", ownedIDs, emptyIDs)
		}
		if len(ownedIDs) > 0 && common.UsingLogDatabase(common.DatabaseTypeMySQL) {
			exact := LOG_DB.WithContext(ctx).Where(requestKey+" IN ?", ownedIDs)
			if len(emptyIDs) > 0 {
				exact = exact.Or("logs.id IN ?", emptyIDs)
			}
			histories = histories.Where(exact)
		}
		var last *Log
		for {
			page := histories.Session(&gorm.Session{})
			if last != nil {
				page = page.Where(requestKey+" > ? OR ("+requestKey+" = ? AND logs.created_at > ?) OR ("+requestKey+" = ? AND logs.created_at = ? AND logs.id > ?)", last.RequestId, last.RequestId, last.CreatedAt, last.RequestId, last.CreatedAt, last.Id)
			}
			var batch []*Log
			if err = page.Order(requestKey + " asc, logs.created_at asc, logs.id asc").
				Limit(min(1000, limit+1-int(scanned))).Find(&batch).Error; err != nil {
				return scanned, err
			}
			if len(batch) == 0 {
				break
			}
			scanned += int64(len(batch))
			if scanned > int64(limit) {
				return scanned, nil
			}
			if err = visit(batch); err != nil {
				return scanned, err
			}
			last = batch[len(batch)-1]
		}
		processed += int64(len(candidates))
		if progress != nil {
			if err = progress(processed, total); err != nil {
				return scanned, err
			}
		}
	}
}

func userConsumptionExportScope(ctx context.Context, userID int, query SelfLogExportQuery) (tx *gorm.DB, err error) {
	tx = LOG_DB.WithContext(ctx).Model(&Log{}).
		Where("user_id = ?", userID).
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund})
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", query.ModelName); err != nil {
		return nil, err
	}
	if query.TokenName != "" {
		tx = tx.Where("token_name = ?", query.TokenName)
	}
	if query.RequestID != "" {
		tx = tx.Where("request_id = ?", query.RequestID)
	}
	if query.StartTimestamp != 0 {
		tx = tx.Where("created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp != 0 {
		tx = tx.Where("created_at <= ?", query.EndTimestamp)
	}
	return tx, nil
}

func administrativeLogExportScope(ctx context.Context, query LogExportQuery) (tx *gorm.DB, err error) {
	tx = LOG_DB.WithContext(ctx).Model(&Log{}).Where("logs.type IN ?", []int{LogTypeConsume, LogTypeError})
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", query.ModelName); err != nil {
		return nil, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", query.Username); err != nil {
		return nil, err
	}
	if query.TokenName != "" {
		tx = tx.Where("logs.token_name = ?", query.TokenName)
	}
	if query.RequestID != "" {
		tx = tx.Where("logs.request_id = ?", query.RequestID)
	}
	if query.UpstreamRequestID != "" {
		tx = tx.Where("logs.upstream_request_id = ?", query.UpstreamRequestID)
	}
	if query.StartTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", query.EndTimestamp)
	}
	if query.ChannelID != 0 {
		tx = tx.Where("logs.channel_id = ?", query.ChannelID)
	}
	if query.Group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", query.Group)
	}
	return tx, nil
}
