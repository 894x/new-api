package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
)

type PendingTaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	PluginState    []byte
}

type PendingTaskSubmitError struct {
	Err     error
	Unknown bool
}

func (e *PendingTaskSubmitError) Error() string { return e.Err.Error() }
func (e *PendingTaskSubmitError) Unwrap() error { return e.Err }

type pendingTaskSubmitter interface {
	SubmitPendingTask(context.Context, *model.Task) (*PendingTaskSubmitResult, error)
}

func submitPendingTask(ctx context.Context, adaptor TaskPollingAdaptor, task *model.Task, admissionErr error) error {
	claimedTask, claimed, err := model.ClaimTaskSubmission(ctx, task.ID, time.Now().Unix())
	if err != nil || !claimed {
		return err
	}
	task = claimedTask
	if task.PrivateData.Execution != nil && task.PrivateData.Execution.RequestID != "" {
		ctx = context.WithValue(ctx, common.RequestIdKey, task.PrivateData.Execution.RequestID)
	}
	pending := task.PrivateData.PendingSubmission
	leaseID := pending.LeaseID
	unknown := pending.Stage == model.TaskSubmissionUpstream
	var result *PendingTaskSubmitResult
	if unknown {
		err = errors.New(model.TaskSubmissionUnknownReason)
	} else if admissionErr != nil {
		err = admissionErr
	} else if submitter, ok := adaptor.(pendingTaskSubmitter); ok {
		executionCtx, cancel := context.WithTimeout(ctx, model.TaskSubmissionTimeout)
		result, err = submitter.SubmitPendingTask(executionCtx, task)
		cancel()
		var failure *PendingTaskSubmitError
		if errors.As(err, &failure) {
			unknown = failure.Unknown
		}
	} else {
		err = errors.New("task adaptor does not support pending submissions")
	}
	// A runner losing its lease during asset preparation leaves resumable work.
	// Cancellation after dispatch is an uncertain provider outcome instead.
	if ctx.Err() != nil && pending.Stage == model.TaskSubmissionAssets {
		pending.LeaseUntil = 0
		_, updateErr := model.UpdateClaimedTaskSubmission(context.WithoutCancel(ctx), task, leaseID)
		return updateErr
	}
	if err == nil && (result == nil || result.UpstreamTaskID == "") {
		err = errors.New("upstream returned no task ID")
		unknown = true
	}
	if err != nil {
		stage := pending.Stage
		code := stage + "_failed"
		reason := "资产准备失败：" + err.Error()
		if stage == model.TaskSubmissionUpstream {
			reason = "上游任务提交失败：" + err.Error()
		}
		if unknown {
			code = "upstream_submit_unknown"
			reason = model.TaskSubmissionUnknownReason
		}
		reason = common.MaskSensitiveInfo(reason)
		if task.PrivateData.Key != "" {
			reason = strings.ReplaceAll(reason, task.PrivateData.Key, "[redacted]")
		}
		if len([]rune(reason)) > 1024 {
			reason = string([]rune(reason)[:1024])
		}
		task.Status, task.Progress = model.TaskStatusFailure, taskcommon.ProgressComplete
		task.FailReason, task.FinishTime = reason, time.Now().Unix()
		task.SetData(map[string]any{"error": map[string]any{"code": code, "stage": stage, "message": reason}})
		pending.Body = nil
		pending.MediaPolicy = nil
		if !unknown {
			task.PrivateData.PendingSubmission = nil
		}
		won, updateErr := model.UpdateClaimedTaskSubmission(context.WithoutCancel(ctx), task, leaseID)
		if updateErr != nil || !won {
			return updateErr
		}
		if !unknown && taskNeedsBillingRefund(task) {
			RefundTaskQuota(context.WithoutCancel(ctx), task, reason)
		}
		return nil
	}
	task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
	task.PrivateData.PluginState = result.PluginState
	task.PrivateData.PendingSubmission = nil
	task.Data = result.TaskData
	task.Status, task.Progress = model.TaskStatusSubmitted, taskcommon.ProgressSubmitted
	won, err := model.UpdateClaimedTaskSubmission(context.WithoutCancel(ctx), task, leaseID)
	if err != nil {
		return err
	}
	if !won {
		return fmt.Errorf("task %s submission receipt lost its lease", task.TaskID)
	}
	return nil
}
