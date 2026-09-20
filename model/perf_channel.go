package model

import "context"

// PerfChannelLog is the small read model used by channel performance
// analytics. The request timing payload lives in Log.Other, so the query
// intentionally selects only the fields needed for aggregation.
type PerfChannelLog struct {
	Id        int64  `gorm:"column:id"`
	ChannelId int    `gorm:"column:channel_id"`
	CreatedAt int64  `gorm:"column:created_at"`
	Type      int    `gorm:"column:type"`
	UseTime   int    `gorm:"column:use_time"`
	Other     string `gorm:"column:other"`
}

// VisitPerfChannelLogs caps both rows and decoded payload bytes; pagination never
// retains a full day's Other payloads in memory. Truncation is explicit to callers.
func VisitPerfChannelLogs(ctx context.Context, channelID int, startTs, endTs int64, visit func(PerfChannelLog)) (int, bool, error) {
	var cursor int64
	count, payloadBytes := 0, 0
	for {
		query := LOG_DB.WithContext(ctx).Model(&Log{}).
			Select("id, channel_id, created_at, type, use_time, other").
			Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
			Where("created_at >= ? AND created_at <= ?", startTs, endTs)
		if channelID > 0 {
			query = query.Where("channel_id = ?", channelID)
		}
		if cursor > 0 {
			query = query.Where("id < ?", cursor)
		}
		rows, err := query.Order("id DESC").Limit(500).Rows()
		if err != nil {
			return count, false, err
		}
		pageCount := 0
		for rows.Next() {
			var row PerfChannelLog
			if err := LOG_DB.ScanRows(rows, &row); err != nil {
				_ = rows.Close()
				return count, false, err
			}
			if err := ctx.Err(); err != nil {
				_ = rows.Close()
				return count, false, err
			}
			if count >= 50000 || payloadBytes+len(row.Other) > 32<<20 {
				_ = rows.Close()
				return count, true, nil
			}
			pageCount++
			count++
			payloadBytes += len(row.Other)
			visit(row)
			cursor = row.Id
		}
		rowErr := rows.Err()
		_ = rows.Close()
		if rowErr != nil {
			return count, false, rowErr
		}
		if pageCount < 500 {
			return count, false, nil
		}
	}
}
