package model

import (
	"errors"

	"gorm.io/gorm"
)

type Midjourney struct {
	Id          int    `json:"id"`
	Code        int    `json:"code"`
	UserId      int    `json:"user_id" gorm:"index"`
	Action      string `json:"action" gorm:"type:varchar(40);index"`
	MjId        string `json:"mj_id" gorm:"index"`
	Prompt      string `json:"prompt"`
	PromptEn    string `json:"prompt_en"`
	Description string `json:"description"`
	State       string `json:"state"`
	SubmitTime  int64  `json:"submit_time" gorm:"index"`
	StartTime   int64  `json:"start_time" gorm:"index"`
	FinishTime  int64  `json:"finish_time" gorm:"index"`
	ImageUrl    string `json:"image_url"`
	VideoUrl    string `json:"video_url"`
	VideoUrls   string `json:"video_urls"`
	Status      string `json:"status" gorm:"type:varchar(20);index"`
	Progress    string `json:"progress" gorm:"type:varchar(30);index"`
	FailReason  string `json:"fail_reason"`
	ChannelId   int    `json:"channel_id"`
	Quota       int    `json:"quota"`
	Buttons     string `json:"buttons"`
	Properties  string `json:"properties"`

	TokenId          int `json:"-" gorm:"default:0"`
	BillingChannelId int `json:"-" gorm:"default:0"`

	OriginalQuota          int    `json:"-"`
	RefundedQuota          int    `json:"-"`
	ChargeState            string `json:"-" gorm:"type:varchar(32)"`
	RefundState            string `json:"-" gorm:"type:varchar(32)"`
	BillingSource          string `json:"-" gorm:"type:varchar(32)"`
	SubscriptionId         int    `json:"-"`
	UsingGroup             string `json:"-" gorm:"type:varchar(128)"`
	OriginModelName        string `json:"-" gorm:"type:varchar(255)"`
	DiscountSettlementID   string `json:"-" gorm:"type:varchar(191);index"`
	DiscountPolicySnapshot string `json:"-" gorm:"type:text"`
	BillingReady           *bool  `json:"-" gorm:"index"`
	BillingRecoveryPending bool   `json:"-" gorm:"index"`
}

// TaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type TaskQueryParams struct {
	ChannelID      string
	MjID           string
	StartTimestamp string
	EndTimestamp   string
}

func GetAllUserTask(userId int, startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB.Where("user_id = ?", userId)

	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		// 假设您已将前端传来的时间戳转换为数据库所需的时间格式，并处理了时间戳的验证和解析
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllTasks(startIdx int, num int, queryParams TaskQueryParams) []*Midjourney {
	var tasks []*Midjourney
	var err error

	// 初始化查询构建器
	query := DB

	// 添加过滤条件
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}

	// 获取数据
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func GetAllUnFinishTasks() []*Midjourney {
	recoverMidjourneyBillingRowsBeforePolling()
	var tasks []*Midjourney
	var err error
	// get all tasks progress is not 100%
	err = DB.Where("progress != ?", "100%").
		Where("(billing_ready IS NULL OR billing_ready = ?)", true).
		Where("(billing_ready IS NOT NULL OR charge_state IS NULL OR charge_state = ? OR charge_state NOT IN ?)",
			"", []string{TaskChargeStatePrepared, TaskChargeStatePendingReconcile}).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// HasUnfinishedMidjourneyTasks reports whether at least one Midjourney task is
// still in progress. It is a cheap existence check (LIMIT 1) used to decide
// whether the midjourney_poll system task needs to run; when no task is pending
// the scheduler skips creating a row entirely.
func HasUnfinishedMidjourneyTasks() bool {
	recoverMidjourneyBillingRowsBeforePolling()
	var id int
	err := DB.Model(&Midjourney{}).
		Where("progress != ?", "100%").
		Where("(billing_ready IS NULL OR billing_ready = ?)", true).
		Where("(billing_ready IS NOT NULL OR charge_state IS NULL OR charge_state = ? OR charge_state NOT IN ?)",
			"", []string{TaskChargeStatePrepared, TaskChargeStatePendingReconcile}).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func GetByOnlyMJId(mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("mj_id = ?", mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJId(userId int, mjId string) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("user_id = ? and mj_id = ?", userId, mjId).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetByMJIds(userId int, mjIds []string) []*Midjourney {
	var mj []*Midjourney
	var err error
	err = DB.Where("user_id = ? and mj_id in (?)", userId, mjIds).Find(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func GetMjByuId(id int) *Midjourney {
	var mj *Midjourney
	var err error
	err = DB.Where("id = ?", id).First(&mj).Error
	if err != nil {
		return nil
	}
	return mj
}

func UpdateProgress(id int, progress string) error {
	return DB.Model(&Midjourney{}).Where("id = ?", id).Update("progress", progress).Error
}

func (midjourney *Midjourney) Insert() error {
	var err error
	err = DB.Create(midjourney).Error
	return err
}

func (midjourney *Midjourney) BeforeCreate(_ *gorm.DB) error {
	midjourney.syncBillingReady()
	return nil
}

func (midjourney *Midjourney) syncBillingReady() {
	if midjourney.ChargeState == "" {
		return
	}
	ready := midjourney.ChargeState != TaskChargeStatePrepared &&
		midjourney.ChargeState != TaskChargeStatePendingReconcile &&
		!midjourney.BillingRecoveryPending
	midjourney.BillingReady = &ready
}

func (midjourney *Midjourney) Update() error {
	var err error
	err = DB.Save(midjourney).Error
	return err
}

func (midjourney *Midjourney) UpdateBillingState() error {
	midjourney.syncBillingReady()
	return DB.Model(midjourney).
		Select(
			"quota", "token_id", "billing_channel_id", "original_quota", "refunded_quota",
			"charge_state", "refund_state",
			"billing_source", "subscription_id", "using_group", "origin_model_name",
			"discount_settlement_id", "discount_policy_snapshot", "billing_ready", "billing_recovery_pending",
		).
		Updates(midjourney).Error
}

// ClaimBillingState persists the pending intent only while this row still has
// the caller's observed charge state. A stale concurrent owner must not move
// the same external funding/token charge twice.
func (midjourney *Midjourney) ClaimBillingState(fromState string) (bool, error) {
	midjourney.syncBillingReady()
	result := DB.Model(&Midjourney{}).
		Where("id = ? AND charge_state = ? AND billing_recovery_pending = ?", midjourney.Id, fromState, false).
		Select(
			"quota", "token_id", "billing_channel_id", "original_quota", "refunded_quota",
			"charge_state", "refund_state",
			"billing_source", "subscription_id", "using_group", "origin_model_name",
			"discount_settlement_id", "discount_policy_snapshot", "billing_ready", "billing_recovery_pending",
		).
		Updates(midjourney)
	return result.RowsAffected == 1, result.Error
}

// MarkBillingRecoveryPending hands a settled dynamic ledger to the local
// recovery pass after the final Midjourney snapshot write failed.
func (midjourney *Midjourney) MarkBillingRecoveryPending(fromState string) error {
	result := DB.Model(&Midjourney{}).
		Where(
			"id = ? AND charge_state = ? AND billing_recovery_pending = ?",
			midjourney.Id, fromState, false,
		).
		Updates(map[string]any{
			"billing_ready":            false,
			"billing_recovery_pending": true,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("Midjourney billing recovery handoff was not claimed")
	}
	ready := false
	midjourney.BillingReady = &ready
	midjourney.BillingRecoveryPending = true
	return nil
}

// RecoverSettledInitialBilling promotes only an explicitly handed-off dynamic
// settlement and never performs upstream, funding, token, or statistics work.
func (midjourney *Midjourney) RecoverSettledInitialBilling() bool {
	if midjourney == nil || midjourney.Id == 0 || !midjourney.BillingRecoveryPending ||
		midjourney.DiscountSettlementID == "" ||
		(midjourney.ChargeState != TaskChargeStatePrepared &&
			midjourney.ChargeState != TaskChargeStatePendingReconcile) {
		return false
	}
	settlement, err := GetGroupModelDiscountSettlement(midjourney.DiscountSettlementID)
	billingChannelID := midjourney.GetBillingChannelId()
	if err != nil || settlement.Status != GroupModelDiscountStatusSettled ||
		!settlement.AccountingApplied || settlement.UserID != midjourney.UserId ||
		settlement.AccountingUserID != midjourney.UserId || settlement.AccountingChannelID != billingChannelID ||
		settlement.AccountingQuotaDelta < 0 || settlement.AccountingRequestCountDelta != 1 ||
		settlement.AccountingQuotaDelta != int(settlement.ChargedQuota) ||
		settlement.OriginalQuota != int64(midjourney.OriginalQuota) {
		return false
	}

	previousQuota := midjourney.Quota
	previousChargeState := midjourney.ChargeState
	previousRecoveryPending := midjourney.BillingRecoveryPending
	previousBillingReady := midjourney.BillingReady
	midjourney.Quota = settlement.AccountingQuotaDelta
	midjourney.ChargeState = TaskChargeStateCharged
	midjourney.BillingRecoveryPending = false
	midjourney.syncBillingReady()
	result := DB.Model(&Midjourney{}).
		Where(
			"id = ? AND charge_state = ? AND billing_ready = ? AND billing_recovery_pending = ?",
			midjourney.Id, previousChargeState, false, true,
		).
		Select(
			"quota", "token_id", "billing_channel_id", "original_quota", "refunded_quota",
			"charge_state", "refund_state", "billing_source", "subscription_id", "using_group",
			"origin_model_name", "discount_settlement_id", "discount_policy_snapshot",
			"billing_ready", "billing_recovery_pending",
		).
		Updates(midjourney)
	if result.Error != nil || result.RowsAffected != 1 {
		midjourney.Quota = previousQuota
		midjourney.ChargeState = previousChargeState
		midjourney.BillingRecoveryPending = previousRecoveryPending
		midjourney.BillingReady = previousBillingReady
		return false
	}
	return true
}

func recoverMidjourneyBillingRowsBeforePolling() {
	var tasks []*Midjourney
	if err := DB.Where("billing_recovery_pending = ?", true).Find(&tasks).Error; err != nil {
		return
	}
	for _, task := range tasks {
		task.RecoverSettledInitialBilling()
	}
}

func (midjourney *Midjourney) GetBillingChannelId() int {
	if midjourney.BillingChannelId > 0 {
		return midjourney.BillingChannelId
	}
	return midjourney.ChannelId
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Returns (true, nil) if this caller won the update, (false, nil) if
// another process already moved the task out of fromStatus.
// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Uses Model().Select("*").Updates() to avoid GORM Save()'s INSERT fallback.
func (midjourney *Midjourney) UpdateWithStatus(fromStatus string) (bool, error) {
	result := DB.Model(midjourney).Where("status = ?", fromStatus).Select("*").Updates(midjourney)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func MjBulkUpdateByTaskIds(taskIDs []int, params map[string]any) error {
	return DB.Model(&Midjourney{}).
		Where("id in (?)", taskIDs).
		Updates(params).Error
}

// CountAllTasks returns total midjourney tasks for admin query
func CountAllTasks(queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{})
	if queryParams.ChannelID != "" {
		query = query.Where("channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}

// CountAllUserTask returns total midjourney tasks for user
func CountAllUserTask(userId int, queryParams TaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Midjourney{}).Where("user_id = ?", userId)
	if queryParams.MjID != "" {
		query = query.Where("mj_id = ?", queryParams.MjID)
	}
	if queryParams.StartTimestamp != "" {
		query = query.Where("submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != "" {
		query = query.Where("submit_time <= ?", queryParams.EndTimestamp)
	}
	_ = query.Count(&total).Error
	return total
}
