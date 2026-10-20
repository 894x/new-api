package model

import (
	"context"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// TaskBillingEntry exposes accounting facts only, never raw provider or admin data.
type TaskBillingEntry struct {
	RequestID string `json:"request_id"`
	CreatedAt int64  `json:"created_at"`
	Kind      string `json:"kind"`
	Quota     int    `json:"quota"`
}

type TaskBillingHistory struct {
	TaskID          string             `json:"task_id"`
	SubmitRequestID string             `json:"submit_request_id"`
	Status          TaskStatus         `json:"status"`
	TaskQuota       int                `json:"task_quota"`
	ChargedQuota    int64              `json:"charged_quota"`
	RefundedQuota   int64              `json:"refunded_quota"`
	NetQuota        int64              `json:"net_quota"`
	Truncated       bool               `json:"truncated"`
	Reconciled      bool               `json:"reconciled"`
	Entries         []TaskBillingEntry `json:"entries"`
}

// GetTaskBillingHistory queries main and log databases separately. userID == 0
// is reserved for administrators; every subsequent log read is pinned to the
// resolved task owner, including when historical task identifiers collide.
func GetTaskBillingHistory(ctx context.Context, taskID, requestID string, userID int) (*TaskBillingHistory, error) {
	if requestID != "" {
		query := LOG_DB.WithContext(ctx).Where("request_id = ? AND type IN ?", requestID, []int{LogTypeConsume, LogTypeRefund})
		if userID > 0 {
			query = query.Where("user_id = ?", userID)
		}
		var references []Log
		if err := query.Select("user_id", "other").Limit(2).Find(&references).Error; err != nil {
			return nil, err
		}
		if len(references) != 1 {
			return nil, nil
		}
		var reference struct {
			TaskID string `json:"task_id"`
		}
		if common.UnmarshalJsonStr(references[0].Other, &reference) != nil || reference.TaskID == "" {
			return nil, nil
		}
		taskID, userID = reference.TaskID, references[0].UserId
	}
	if taskID == "" {
		return nil, nil
	}
	query := DB.WithContext(ctx).Where("task_id = ?", taskID)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	var tasks []Task
	if err := query.Select("task_id", "user_id", "status", "quota", "private_data", "created_at", "submit_time", "billing_ready", "billing_recovery_pending").Limit(2).Find(&tasks).Error; err != nil {
		return nil, err
	}
	if len(tasks) != 1 || tasks[0].UserId <= 0 {
		return nil, nil
	}
	task := &tasks[0]

	// Narrow historical TEXT metadata with a literal LIKE, then verify the JSON
	// key exactly in Go. This also works on a separately configured ClickHouse
	// log store and tolerates legacy malformed JSON without dialect-specific casts.
	needle, err := common.Marshal(task.TaskID)
	if err != nil {
		return nil, err
	}
	condition := "other LIKE ? ESCAPE '!'"
	pattern := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(string(needle))
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		condition = "other LIKE ?"
		pattern = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(string(needle))
	}
	const entryLimit = 200
	var logs []Log
	logQuery := LOG_DB.WithContext(ctx).
		Where("user_id = ? AND type IN ?", task.UserId, []int{LogTypeConsume, LogTypeRefund}).
		Where(condition, "%"+pattern+"%")
	startTime := task.CreatedAt
	if task.SubmitTime > 0 && (startTime <= 0 || task.SubmitTime < startTime) {
		startTime = task.SubmitTime
	}
	if startTime > 0 {
		// Include a clock-skew margin, with no upper bound: late refunds remain visible.
		logQuery = logQuery.Where("created_at >= ?", max(int64(0), startTime-300))
	}
	err = logQuery.
		Select("created_at", "request_id", "type", "quota", "other").
		Order("created_at asc, request_id asc").Limit(entryLimit + 1).Find(&logs).Error
	if err != nil {
		return nil, err
	}
	history := &TaskBillingHistory{
		TaskID: task.TaskID, Status: task.Status, TaskQuota: task.Quota,
		Truncated: len(logs) > entryLimit, Entries: make([]TaskBillingEntry, 0),
	}
	if execution := task.PrivateData.Execution; execution != nil {
		history.SubmitRequestID = execution.RequestID
	}
	for _, log := range logs[:min(len(logs), entryLimit)] {
		var other struct {
			TaskID           string `json:"task_id"`
			PreConsumedQuota *int   `json:"pre_consumed_quota"`
		}
		if common.UnmarshalJsonStr(log.Other, &other) != nil || other.TaskID != task.TaskID {
			continue
		}
		if log.Quota < 0 || log.Quota > common.MaxWalletQuota {
			history.Truncated = true // Invalid historical facts cannot establish a balance.
			continue
		}
		if (log.Type == LogTypeRefund && history.RefundedQuota > common.MaxWalletQuota-int64(log.Quota)) ||
			(log.Type == LogTypeConsume && history.ChargedQuota > common.MaxWalletQuota-int64(log.Quota)) {
			history.Truncated = true
			continue
		}
		kind := "initial_charge"
		if log.Type == LogTypeRefund {
			kind = "refund"
			history.RefundedQuota += int64(log.Quota)
		} else {
			history.ChargedQuota += int64(log.Quota)
			if other.PreConsumedQuota != nil {
				kind = "additional_charge"
			} else if history.SubmitRequestID == "" {
				history.SubmitRequestID = log.RequestId
			}
		}
		history.Entries = append(history.Entries, TaskBillingEntry{
			RequestID: log.RequestId, CreatedAt: log.CreatedAt, Kind: kind, Quota: log.Quota,
		})
	}
	history.NetQuota = history.ChargedQuota - history.RefundedQuota
	history.Reconciled = !history.Truncated && len(history.Entries) > 0 &&
		(task.BillingReady == nil || *task.BillingReady) && !task.BillingRecoveryPending &&
		(task.Status == TaskStatusSuccess || task.Status == TaskStatusFailure) &&
		history.NetQuota == int64(task.Quota)
	if billing := task.PrivateData.BillingContext; billing != nil && billing.RefundState != "" && billing.RefundState != TaskRefundStateCommitted {
		history.Reconciled = false
	}
	return history, nil
}
