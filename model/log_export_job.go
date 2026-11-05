package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	SystemTaskTypeLogExport   = "log_export"
	LogExportMaxRowsOptionKey = "LogExportMaxRows"
	DefaultLogExportMaxRows   = 100000
	LogExportRowSafetyLimit   = 10000000
	LogExportRetentionSeconds = 7 * 24 * 60 * 60
	LogExportChunkSize        = 512 * 1024
	LogExportMaxFileBytes     = 1024 * 1024 * 1024
)

// LogExportRecord and its private file chunks live in the primary database,
// so a download works on every gateway node without shared local disks.
type LogExportRecord struct {
	TaskID    string `json:"task_id" gorm:"primaryKey;type:varchar(64)"`
	UserID    int    `json:"-" gorm:"index"`
	View      string `json:"view" gorm:"type:varchar(32)"`
	MaxRows   int    `json:"max_rows"`
	Filename  string `json:"filename" gorm:"type:varchar(128)"`
	RowCount  int    `json:"row_count"`
	FileSize  int64  `json:"file_size"`
	ExpiresAt int64  `json:"expires_at" gorm:"index"`
	CreatedAt int64  `json:"created_at" gorm:"index"`
}

type LogExportChunk struct {
	TaskID   string `gorm:"primaryKey;type:varchar(64)"`
	Sequence int    `gorm:"primaryKey;autoIncrement:false"`
	Data     []byte
}

type LogExportRecordResponse struct {
	LogExportRecord
	Status    SystemTaskStatus  `json:"status"`
	Error     string            `json:"error"`
	Request   string            `json:"-" gorm:"column:request"`
	Filters   any               `json:"filters" gorm:"-"`
	UpdatedAt int64             `json:"updated_at"`
	State     string            `json:"-"`
	Progress  LogExportProgress `json:"progress" gorm:"-"`
}

// LogExportProgress is stored in the existing system-task state, not the schema.
// Percent reserves 90-99 for artifact storage; only a succeeded task is 100%.
type LogExportProgress struct {
	Stage       string `json:"stage"`
	Percent     int    `json:"percent"`
	Processed   int64  `json:"processed"`
	Total       int64  `json:"total"`
	RowCount    int    `json:"row_count"`
	StoredBytes int64  `json:"stored_bytes"`
	TotalBytes  int64  `json:"total_bytes"`
}

func (record *LogExportRecordResponse) loadProgress() {
	var progress LogExportProgress
	if err := common.UnmarshalJsonStr(record.State, &progress); err != nil {
		progress = LogExportProgress{}
	}
	progress.Percent = min(99, max(0, progress.Percent))
	switch record.Status {
	case SystemTaskStatusPending:
		progress = LogExportProgress{Stage: "queued"}
	case SystemTaskStatusSucceeded:
		progress.Stage, progress.Percent, progress.RowCount = "completed", 100, record.RowCount
	}
	record.Progress = progress
}

func ParseLogExportMaxRows(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > LogExportRowSafetyLimit {
		return 0, fmt.Errorf("maximum export rows must be an integer between 1 and %d", LogExportRowSafetyLimit)
	}
	return limit, nil
}

func GetLogExportMaxRows() int {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[LogExportMaxRowsOptionKey]
	common.OptionMapRWMutex.RUnlock()
	limit, err := ParseLogExportMaxRows(value)
	if err != nil {
		return DefaultLogExportMaxRows
	}
	return limit
}

func CreateLogExportRecord(userID int, view string, request any) (*LogExportRecord, error) {
	if userID <= 0 {
		return nil, errors.New("authentication required")
	}
	payload, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	taskID, err := GenerateSystemTaskID()
	if err != nil {
		return nil, err
	}
	activeKey := fmt.Sprintf("log_export:%d", userID)
	now := common.GetTimestamp()
	record := &LogExportRecord{TaskID: taskID, UserID: userID, View: view, MaxRows: GetLogExportMaxRows(), CreatedAt: now, ExpiresAt: now + LogExportRetentionSeconds}
	task := &SystemTask{TaskID: taskID, Type: SystemTaskTypeLogExport, Status: SystemTaskStatusPending, ActiveKey: &activeKey, Payload: string(payload)}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		return tx.Create(record).Error
	})
	return record, err
}

func ListLogExportRecords(ctx context.Context, userID int, self bool, offset int, limit int) ([]LogExportRecordResponse, int64, error) {
	query := DB.WithContext(ctx).Model(&LogExportRecord{}).Where("log_export_records.user_id = ?", userID)
	if self {
		query = query.Where("log_export_records.view = ?", "self")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []LogExportRecordResponse
	err := query.Joins("JOIN system_tasks ON system_tasks.task_id = log_export_records.task_id").
		Select("log_export_records.*, system_tasks.status, system_tasks.error, system_tasks.payload AS request, system_tasks.updated_at, system_tasks.state").
		Order("log_export_records.created_at DESC, log_export_records.task_id DESC").Offset(max(0, offset)).Limit(min(max(1, limit), 50)).Scan(&records).Error
	for i := range records {
		records[i].loadProgress()
	}
	return records, total, err
}

func GetLogExportRecord(ctx context.Context, taskID string, userID int, self bool) (*LogExportRecordResponse, error) {
	query := DB.WithContext(ctx).Model(&LogExportRecord{}).
		Joins("JOIN system_tasks ON system_tasks.task_id = log_export_records.task_id").
		Where("log_export_records.task_id = ? AND log_export_records.user_id = ?", taskID, userID)
	if self {
		query = query.Where("log_export_records.view = ?", "self")
	}
	var record LogExportRecordResponse
	err := query.Select("log_export_records.*, system_tasks.status, system_tasks.error, system_tasks.updated_at, system_tasks.state").Take(&record).Error
	record.loadProgress()
	return &record, err
}

// ExpireLogExportFiles keeps the history while removing expired/abandoned bytes.
func ExpireLogExportFiles(ctx context.Context) error {
	var records []LogExportRecord
	if err := DB.WithContext(ctx).Where("expires_at > 0 AND expires_at <= ?", common.GetTimestamp()).
		Where("EXISTS (SELECT 1 FROM log_export_chunks WHERE log_export_chunks.task_id = log_export_records.task_id)").
		Limit(100).Find(&records).Error; err != nil {
		return err
	}
	for _, record := range records {
		if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("task_id = ?", record.TaskID).Delete(&LogExportChunk{}).Error; err != nil {
				return err
			}
			return tx.Model(&LogExportRecord{}).Where("task_id = ?", record.TaskID).Update("file_size", 0).Error
		}); err != nil {
			return err
		}
	}
	return nil
}
