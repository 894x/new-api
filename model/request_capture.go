package model

import (
	"context"

	"gorm.io/gorm/clause"
)

const requestCaptureCleanupLockPrefix = "request_capture_cleanup:"

// RequestCapturePolicy is administrator-owned; it is deliberately separate
// from user-editable settings and defaults to disabled when no row exists.
type RequestCapturePolicy struct {
	UserID  int  `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	Enabled bool `json:"enabled"`
}

// RequestCapture indexes local compressed evidence in the primary database,
// independently of the configured consumption-log database.
type RequestCapture struct {
	ID          string `json:"id" gorm:"primaryKey;type:varchar(32)"`
	RequestID   string `json:"request_id" gorm:"type:varchar(64);index"`
	UserID      int    `json:"user_id" gorm:"index"`
	StoreID     string `json:"-" gorm:"type:varchar(32);index:idx_capture_store_time,priority:1;index:idx_capture_store_expiry,priority:1"`
	CreatedAt   int64  `json:"created_at" gorm:"index:idx_capture_store_time,priority:2"`
	ExpiresAt   int64  `json:"expires_at" gorm:"index;index:idx_capture_store_expiry,priority:2"`
	Status      string `json:"status" gorm:"type:varchar(32)"`
	Reason      string `json:"reason,omitempty" gorm:"type:varchar(64)"`
	StatusCode  int    `json:"status_code"`
	RawBytes    int64  `json:"raw_bytes"`
	StoredBytes int64  `json:"stored_bytes"`
}

func GetRequestCapturePolicy(ctx context.Context, userID int) (RequestCapturePolicy, error) {
	policy := RequestCapturePolicy{UserID: userID}
	err := DB.WithContext(ctx).Where("user_id = ?", userID).Limit(1).Find(&policy).Error
	return policy, err
}

func SetRequestCapturePolicy(ctx context.Context, policy RequestCapturePolicy) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled"}),
	}).Create(&policy).Error
}

func SaveRequestCapture(ctx context.Context, capture *RequestCapture) error {
	return DB.WithContext(ctx).Save(capture).Error
}

func FindRequestCapture(ctx context.Context, requestID string) (*RequestCapture, error) {
	var capture RequestCapture
	err := DB.WithContext(ctx).Where("request_id = ?", requestID).Order("created_at DESC").Take(&capture).Error
	return &capture, err
}

func ListStoredRequestCaptures(ctx context.Context, storeID string) ([]RequestCapture, error) {
	var captures []RequestCapture
	err := DB.WithContext(ctx).Where("store_id = ?", storeID).Order("created_at ASC").Find(&captures).Error
	return captures, err
}

func ListExpiredRequestCaptures(ctx context.Context, storeID string, now int64, pruneBefore int64, limit int) ([]RequestCapture, error) {
	var captures []RequestCapture
	err := DB.WithContext(ctx).
		Where("store_id = ? AND expires_at <= ?", storeID, now).
		Where("expires_at < ? OR stored_bytes > 0 OR status <> ?", pruneBefore, "expired").
		Order("created_at ASC").
		Limit(limit).
		Find(&captures).Error
	return captures, err
}

func ListRequestCapturesForEviction(ctx context.Context, storeID string, limit int) ([]RequestCapture, error) {
	var captures []RequestCapture
	err := DB.WithContext(ctx).
		Where("store_id = ? AND stored_bytes > 0", storeID).
		Order("created_at ASC").
		Limit(limit).
		Find(&captures).Error
	return captures, err
}

func DeleteRequestCapture(ctx context.Context, id string) error {
	return DB.WithContext(ctx).Where("id = ?", id).Delete(&RequestCapture{}).Error
}

func RequestCaptureStoredBytes(ctx context.Context, storeID string) (int64, error) {
	var total int64
	err := DB.WithContext(ctx).Model(&RequestCapture{}).Where("store_id = ?", storeID).
		Select("COALESCE(SUM(stored_bytes), 0)").Scan(&total).Error
	return total, err
}

func AcquireRequestCaptureCleanupLease(ctx context.Context, storeID string, owner string, now int64, lockedUntil int64) (bool, error) {
	lockType := requestCaptureCleanupLockPrefix + storeID
	result := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&SystemTaskLock{
		Type:        lockType,
		TaskID:      storeID,
		LockedBy:    owner,
		LockedUntil: lockedUntil,
		UpdatedAt:   now,
	})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	result = DB.WithContext(ctx).Model(&SystemTaskLock{}).
		Where("type = ? AND locked_until < ?", lockType, now).
		Updates(map[string]any{
			"task_id":      storeID,
			"locked_by":    owner,
			"locked_until": lockedUntil,
			"updated_at":   now,
		})
	return result.RowsAffected == 1, result.Error
}

func ReleaseRequestCaptureCleanupLease(ctx context.Context, storeID string, owner string) error {
	return DB.WithContext(ctx).Where("type = ? AND task_id = ? AND locked_by = ?", requestCaptureCleanupLockPrefix+storeID, storeID, owner).
		Delete(&SystemTaskLock{}).Error
}
