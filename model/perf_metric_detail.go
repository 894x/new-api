package model

import (
	"database/sql"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PerfMetricDetail struct {
	Id              int    `json:"id" gorm:"primaryKey"`
	ModelName       string `json:"model_name" gorm:"size:128;uniqueIndex:idx_perf_detail_dimension_bucket,priority:1;index:idx_perf_detail_model_bucket,priority:1;index:idx_perf_detail_user_model_bucket,priority:2"`
	UserId          int    `json:"user_id" gorm:"uniqueIndex:idx_perf_detail_dimension_bucket,priority:2;index:idx_perf_detail_user_model_bucket,priority:1"`
	TokenId         int    `json:"token_id" gorm:"uniqueIndex:idx_perf_detail_dimension_bucket,priority:3"`
	BucketTs        int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_perf_detail_dimension_bucket,priority:4;index:idx_perf_detail_model_bucket,priority:2;index:idx_perf_detail_user_model_bucket,priority:3;index:idx_perf_detail_bucket"`
	RequestCount    int64  `json:"-" gorm:"default:0"`
	SuccessCount    int64  `json:"-" gorm:"default:0"`
	TtftCount       int64  `json:"-" gorm:"default:0"`
	TpotCount       int64  `json:"-" gorm:"default:0"`
	InputTokens     int64  `json:"-" gorm:"default:0"`
	OutputTokens    int64  `json:"-" gorm:"default:0"`
	TotalTokens     int64  `json:"-" gorm:"default:0"`
	CacheReadTokens int64  `json:"-" gorm:"default:0"`
}

func (PerfMetricDetail) TableName() string {
	return "perf_metric_details"
}

type PerfMetricHistogram struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	ModelName    string `json:"model_name" gorm:"size:128;uniqueIndex:idx_perf_hist_dimension_bucket,priority:1;index:idx_perf_hist_model_bucket,priority:1;index:idx_perf_hist_user_model_bucket,priority:2"`
	UserId       int    `json:"user_id" gorm:"uniqueIndex:idx_perf_hist_dimension_bucket,priority:2;index:idx_perf_hist_user_model_bucket,priority:1"`
	TokenId      int    `json:"token_id" gorm:"uniqueIndex:idx_perf_hist_dimension_bucket,priority:3"`
	BucketTs     int64  `json:"bucket_ts" gorm:"uniqueIndex:idx_perf_hist_dimension_bucket,priority:4;index:idx_perf_hist_model_bucket,priority:2;index:idx_perf_hist_user_model_bucket,priority:3;index:idx_perf_hist_bucket"`
	Metric       string `json:"metric" gorm:"size:8;uniqueIndex:idx_perf_hist_dimension_bucket,priority:5"`
	UpperBoundMs int64  `json:"upper_bound_ms" gorm:"uniqueIndex:idx_perf_hist_dimension_bucket,priority:6"`
	Count        int64  `json:"-" gorm:"default:0"`
}

func (PerfMetricHistogram) TableName() string {
	return "perf_metric_histograms"
}

type PerfMetricDetailFilter struct {
	ModelName     string
	UserId        int
	TokenId       int
	StartTs       int64
	EndTs         int64
	BucketSeconds int64
}

type PerfAnalyticsUserOption struct {
	Id       int    `json:"id"`
	Username string `json:"username"`
}

type PerfAnalyticsTokenOption struct {
	Id     int    `json:"id"`
	UserId int    `json:"user_id"`
	Name   string `json:"name"`
}

type PerfAnalyticsOptions struct {
	Models []string                   `json:"models"`
	Users  []PerfAnalyticsUserOption  `json:"users"`
	Tokens []PerfAnalyticsTokenOption `json:"tokens"`
}

func UpsertPerfAnalyticsBucket(detail *PerfMetricDetail, histograms []PerfMetricHistogram) error {
	if detail == nil || detail.RequestCount == 0 {
		return nil
	}
	return UpsertPerfAnalyticsBuckets([]PerfMetricDetail{*detail}, histograms)
}

func UpsertPerfAnalyticsBuckets(details []PerfMetricDetail, histograms []PerfMetricHistogram) error {
	if len(details) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "model_name"},
				{Name: "user_id"},
				{Name: "token_id"},
				{Name: "bucket_ts"},
			},
			DoUpdates: perfMetricAdditiveAssignments("perf_metric_details", []string{
				"request_count", "success_count", "ttft_count", "tpot_count",
				"input_tokens", "output_tokens", "total_tokens", "cache_read_tokens",
			}),
		}).CreateInBatches(&details, 100).Error; err != nil {
			return err
		}

		if len(histograms) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "model_name"},
					{Name: "user_id"},
					{Name: "token_id"},
					{Name: "bucket_ts"},
					{Name: "metric"},
					{Name: "upper_bound_ms"},
				},
				DoUpdates: perfMetricAdditiveAssignments("perf_metric_histograms", []string{"count"}),
			}).CreateInBatches(&histograms, 100).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func perfMetricAdditiveAssignments(tableName string, columns []string) clause.Set {
	assignments := make([]clause.Assignment, 0, len(columns))
	for _, column := range columns {
		insertedValue := "excluded." + column
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			insertedValue = "VALUES(" + column + ")"
		}
		assignments = append(assignments, clause.Assignment{
			Column: clause.Column{Name: column},
			Value:  gorm.Expr(tableName + "." + column + " + " + insertedValue),
		})
	}
	return assignments
}

func GetPerfMetricDetails(filter PerfMetricDetailFilter) ([]PerfMetricDetail, error) {
	return getPerfMetricDetails(DB, filter)
}

func getPerfMetricDetails(db *gorm.DB, filter PerfMetricDetailFilter) ([]PerfMetricDetail, error) {
	var details []PerfMetricDetail
	query := applyPerfMetricDetailFilter(db.Model(&PerfMetricDetail{}), filter)
	bucketExpression := perfMetricBucketExpression(filter.BucketSeconds)
	return details, query.
		Select(bucketExpression + " AS bucket_ts, SUM(request_count) AS request_count, SUM(success_count) AS success_count, SUM(ttft_count) AS ttft_count, SUM(tpot_count) AS tpot_count, SUM(input_tokens) AS input_tokens, SUM(output_tokens) AS output_tokens, SUM(total_tokens) AS total_tokens, SUM(cache_read_tokens) AS cache_read_tokens").
		Group(bucketExpression).
		Order("bucket_ts ASC").
		Find(&details).Error
}

func GetPerfMetricHistograms(filter PerfMetricDetailFilter) ([]PerfMetricHistogram, error) {
	return getPerfMetricHistograms(DB, filter)
}

func getPerfMetricHistograms(db *gorm.DB, filter PerfMetricDetailFilter) ([]PerfMetricHistogram, error) {
	var histograms []PerfMetricHistogram
	query := applyPerfMetricDetailFilter(db.Model(&PerfMetricHistogram{}), filter)
	bucketExpression := perfMetricBucketExpression(filter.BucketSeconds)
	return histograms, query.
		Select(bucketExpression + " AS bucket_ts, metric, upper_bound_ms, SUM(count) AS count").
		Group(bucketExpression + ", metric, upper_bound_ms").
		Order("bucket_ts ASC, metric ASC, upper_bound_ms ASC").
		Find(&histograms).Error
}

func GetPerfAnalyticsBuckets(filter PerfMetricDetailFilter) ([]PerfMetricDetail, []PerfMetricHistogram, error) {
	var details []PerfMetricDetail
	var histograms []PerfMetricHistogram
	queryBuckets := func(tx *gorm.DB) error {
		var err error
		details, err = getPerfMetricDetails(tx, filter)
		if err != nil {
			return err
		}
		histograms, err = getPerfMetricHistograms(tx, filter)
		return err
	}

	var err error
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		err = DB.Transaction(queryBuckets)
	} else {
		err = DB.Transaction(queryBuckets, &sql.TxOptions{
			Isolation: sql.LevelRepeatableRead,
			ReadOnly:  true,
		})
	}
	return details, histograms, err
}

func perfMetricBucketExpression(bucketSeconds int64) string {
	if bucketSeconds <= 1 {
		return "bucket_ts"
	}
	return fmt.Sprintf("bucket_ts - (bucket_ts %% %d)", bucketSeconds)
}

func applyPerfMetricDetailFilter(query *gorm.DB, filter PerfMetricDetailFilter) *gorm.DB {
	query = query.Where("model_name = ? AND bucket_ts >= ? AND bucket_ts <= ?", filter.ModelName, filter.StartTs, filter.EndTs)
	if filter.UserId > 0 {
		query = query.Where("user_id = ?", filter.UserId)
	}
	if filter.TokenId > 0 {
		query = query.Where("token_id = ?", filter.TokenId)
	}
	return query
}

func DeletePerfAnalyticsBefore(cutoffTs int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("bucket_ts < ?", cutoffTs).Delete(&PerfMetricHistogram{}).Error; err != nil {
			return err
		}
		return tx.Where("bucket_ts < ?", cutoffTs).Delete(&PerfMetricDetail{}).Error
	})
}

func PerfAnalyticsTokenBelongsToUser(tokenId int, userId int) (bool, error) {
	if tokenId <= 0 || userId <= 0 {
		return false, nil
	}
	var count int64
	err := DB.Unscoped().Model(&Token{}).
		Where("id = ? AND user_id = ?", tokenId, userId).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count == 1, nil
}

func GetPerfAnalyticsOptions(userId int, includeUsers bool) (PerfAnalyticsOptions, error) {
	options := PerfAnalyticsOptions{
		Models: make([]string, 0),
		Users:  make([]PerfAnalyticsUserOption, 0),
		Tokens: make([]PerfAnalyticsTokenOption, 0),
	}

	modelQuery := DB.Model(&PerfMetricDetail{}).Distinct("model_name")
	if userId > 0 {
		modelQuery = modelQuery.Where("user_id = ?", userId)
	}
	if err := modelQuery.Order("model_name ASC").Pluck("model_name", &options.Models).Error; err != nil {
		return options, err
	}

	if includeUsers {
		userIds := DB.Model(&PerfMetricDetail{}).Distinct("user_id").Select("user_id")
		if err := DB.Model(&User{}).
			Select("id, username").
			Where("id IN (?)", userIds).
			Order("username ASC, id ASC").
			Scan(&options.Users).Error; err != nil {
			return options, err
		}
	}

	if userId > 0 {
		tokenIds := DB.Model(&PerfMetricDetail{}).
			Distinct("token_id").
			Select("token_id").
			Where("user_id = ? AND token_id > 0", userId)
		if err := DB.Unscoped().Model(&Token{}).
			Select("id, user_id, name").
			Where("user_id = ? AND id IN (?)", userId, tokenIds).
			Order("name ASC, id ASC").
			Scan(&options.Tokens).Error; err != nil {
			return options, err
		}
	}

	return options, nil
}
