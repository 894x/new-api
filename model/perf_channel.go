package model

import "context"

// PerfChannelLog is the small read model used by channel performance
// analytics. The request timing payload lives in Log.Other, so the query
// intentionally selects only the fields needed for aggregation.
type PerfChannelLog struct {
	Id        int64  `gorm:"column:id"`
	UserId    int    `gorm:"column:user_id"`
	ChannelId int    `gorm:"column:channel_id"`
	CreatedAt int64  `gorm:"column:created_at"`
	Type      int    `gorm:"column:type"`
	UseTime   int    `gorm:"column:use_time"`
	Other     string `gorm:"column:other"`
}

// VisitPerfChannelLogs caps both rows and decoded payload bytes; pagination never
// retains a full day's Other payloads in memory. Truncation is explicit to callers.
func VisitPerfChannelLogs(ctx context.Context, channelID int, startTs, endTs int64, visit func(PerfChannelLog)) (int, bool, error) {
	return visitPerfLogs(ctx, perfLogFilter{
		ChannelId: channelID,
		StartTs:   startTs,
		EndTs:     endTs,
	}, visit)
}

// VisitPerfUserLogs applies the same bounded scan used by channel diagnostics
// to one user's error logs. It is intentionally read-only and suitable for
// short-window monitoring queries.
func VisitPerfUserLogs(ctx context.Context, userID int, modelName string, tokenID int, startTs, endTs int64, visit func(PerfChannelLog)) (int, bool, error) {
	return visitPerfLogs(ctx, perfLogFilter{
		UserId:    userID,
		ModelName: modelName,
		TokenId:   tokenID,
		StartTs:   startTs,
		EndTs:     endTs,
		ErrorOnly: true,
	}, visit)
}

type perfLogFilter struct {
	ChannelId int
	UserId    int
	ModelName string
	TokenId   int
	StartTs   int64
	EndTs     int64
	ErrorOnly bool
}

func visitPerfLogs(ctx context.Context, filter perfLogFilter, visit func(PerfChannelLog)) (int, bool, error) {
	var cursor int64
	count, payloadBytes := 0, 0
	for {
		query := LOG_DB.WithContext(ctx).Model(&Log{}).
			Select("id, user_id, channel_id, created_at, type, use_time, other")
		if filter.ErrorOnly {
			query = query.Where("type = ?", LogTypeError)
		} else {
			query = query.Where("type IN ?", []int{LogTypeConsume, LogTypeError})
		}
		query = query.
			Where("created_at >= ? AND created_at <= ?", filter.StartTs, filter.EndTs)
		if filter.ChannelId > 0 {
			query = query.Where("channel_id = ?", filter.ChannelId)
		}
		if filter.UserId > 0 {
			query = query.Where("user_id = ?", filter.UserId)
		}
		if filter.ModelName != "" {
			query = query.Where("model_name = ?", filter.ModelName)
		}
		if filter.TokenId > 0 {
			query = query.Where("token_id = ?", filter.TokenId)
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
