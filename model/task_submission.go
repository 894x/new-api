package model

import (
	"context"
	"encoding/json"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	TaskSubmissionAssets        = "asset_prepare"
	TaskSubmissionUpstream      = "upstream_submit"
	TaskSubmissionUnknownReason = "上游提交结果未知，请联系管理员核对；系统不会重复提交"
	// Execution is bounded to ten minutes; an expired asset-preparation lease
	// can be reclaimed without overlapping the previous executor.
	TaskSubmissionTimeout = 10 * time.Minute
	taskSubmissionLease   = TaskSubmissionTimeout + time.Minute
)

// TaskPendingSubmission belongs to the host and is never included in TaskView.
// Only the frozen provider payload and transport target are retained here;
// authentication stays in the task's existing private key field.
type TaskPendingSubmission struct {
	Body               json.RawMessage `json:"body,omitempty"`
	MediaPolicy        json.RawMessage `json:"media_policy,omitempty"`
	ConvertBase64Media *bool           `json:"convert_base64_media,omitempty"`
	BaseURL            string          `json:"base_url"`
	Proxy              string          `json:"proxy,omitempty"`
	Stage              string          `json:"stage,omitempty"`
	LeaseID            string          `json:"lease_id,omitempty"`
	LeaseUntil         int64           `json:"lease_until,omitempty"`
}

// ExpireTaskSubmission rechecks the dispatch stage under the same row lock as
// submission receipts. A stale timeout snapshot cannot refund a dispatched
// request or overwrite its upstream ID.
func ExpireTaskSubmission(ctx context.Context, id, cutoff, now int64) (*Task, bool, error) {
	var task Task
	won := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&task, id).Error; err != nil {
			return err
		}
		pending := task.PrivateData.PendingSubmission
		if pending == nil || task.SubmitTime >= cutoff || task.Progress == "100%" || task.Status == TaskStatusSuccess || task.Status == TaskStatusFailure ||
			(task.BillingReady != nil && !*task.BillingReady) {
			return nil
		}
		task.Status, task.Progress, task.FinishTime = TaskStatusFailure, "100%", now
		code := "asset_prepare_timeout"
		task.FailReason = "资产准备超时"
		if pending.Stage == "" {
			pending.Stage = TaskSubmissionAssets
		}
		if pending.Stage == TaskSubmissionUpstream {
			code = "upstream_submit_unknown"
			task.FailReason = TaskSubmissionUnknownReason
		}
		task.SetData(map[string]any{"error": map[string]any{"code": code, "stage": pending.Stage, "message": task.FailReason}})
		pending.Body, pending.MediaPolicy = nil, nil
		result := tx.Model(&task).Select("status", "progress", "finish_time", "fail_reason", "private_data", "data").Updates(&task)
		won = result.Error == nil && result.RowsAffected == 1
		return result.Error
	})
	return &task, won, err
}

// ClaimTaskSubmission serializes dispatch across polling passes and nodes.
// Asset preparation is resumable. An expired upstream_submit marker is returned
// for reconciliation, never for another provider POST.
func ClaimTaskSubmission(ctx context.Context, id int64, now int64) (*Task, bool, error) {
	var task Task
	claimed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&task, id).Error; err != nil {
			return err
		}
		pending := task.PrivateData.PendingSubmission
		if pending == nil || task.Status == TaskStatusFailure || task.Status == TaskStatusSuccess ||
			(task.BillingReady != nil && !*task.BillingReady) || pending.LeaseUntil > now {
			return nil
		}
		pending.LeaseID = common.GetRandomString(32)
		pending.LeaseUntil = now + int64(taskSubmissionLease/time.Second)
		if pending.Stage == "" {
			pending.Stage = TaskSubmissionAssets
		}
		task.Status = TaskStatusQueued
		result := tx.Model(&task).Select("status", "private_data").Updates(&task)
		claimed = result.Error == nil && result.RowsAffected == 1
		return result.Error
	})
	return &task, claimed, err
}

// UpdateClaimedTaskSubmission rejects a stale executor or a timeout winner.
// No provider I/O occurs inside the database transaction.
func UpdateClaimedTaskSubmission(ctx context.Context, task *Task, leaseID string) (bool, error) {
	won := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Task
		if err := lockForUpdate(tx).First(&current, task.ID).Error; err != nil {
			return err
		}
		pending := current.PrivateData.PendingSubmission
		if pending == nil || pending.LeaseID != leaseID ||
			current.Status == TaskStatusFailure || current.Status == TaskStatusSuccess {
			return nil
		}
		result := tx.Model(task).Select("status", "progress", "private_data", "data", "start_time", "finish_time", "fail_reason").Updates(task)
		won = result.Error == nil && result.RowsAffected == 1
		return result.Error
	})
	return won, err
}
