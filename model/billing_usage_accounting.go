package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrBillingUsageTargetNotFound = errors.New("billing usage target not found")

// BillingUsageDelta is the durable accounting effect paired with one billing
// settlement. RequestCountDelta is one for a newly consumed request and zero
// for quota-only adjustments or refunds.
type BillingUsageDelta struct {
	UserID            int
	ChannelID         int
	QuotaDelta        int
	RequestCountDelta int
}

// ApplyBillingUsageDelta synchronously persists user and channel statistics in
// one primary-database transaction. It deliberately bypasses batch accounting:
// callers use its success as durable evidence before advancing billing state.
func ApplyBillingUsageDelta(delta BillingUsageDelta) error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return applyBillingUsageDelta(tx, delta)
	})
}

func applyBillingUsageDelta(tx *gorm.DB, delta BillingUsageDelta) error {
	if tx == nil || delta.UserID <= 0 || delta.ChannelID <= 0 ||
		delta.QuotaDelta < -common.MaxQuota || delta.QuotaDelta > common.MaxQuota ||
		(delta.RequestCountDelta != 0 && delta.RequestCountDelta != 1) {
		return errors.New("invalid billing usage delta")
	}

	userUpdate := tx.Model(&User{}).
		Where("id = ?", delta.UserID).
		Updates(map[string]any{
			"used_quota":    gorm.Expr("used_quota + ?", delta.QuotaDelta),
			"request_count": gorm.Expr("request_count + ?", delta.RequestCountDelta),
		})
	if userUpdate.Error != nil {
		return fmt.Errorf("update user billing usage: %w", userUpdate.Error)
	}
	if userUpdate.RowsAffected != 1 {
		return fmt.Errorf("%w: user %d", ErrBillingUsageTargetNotFound, delta.UserID)
	}

	channelUpdate := tx.Model(&Channel{}).
		Where("id = ?", delta.ChannelID).
		Update("used_quota", gorm.Expr("used_quota + ?", delta.QuotaDelta))
	if channelUpdate.Error != nil {
		return fmt.Errorf("update channel billing usage: %w", channelUpdate.Error)
	}
	if channelUpdate.RowsAffected != 1 {
		return fmt.Errorf("%w: channel %d", ErrBillingUsageTargetNotFound, delta.ChannelID)
	}
	return nil
}
