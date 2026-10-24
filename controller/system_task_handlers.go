package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/wecomaibot"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// RegisterScheduledSystemTasks wires the periodic channel test, upstream model
// update, async task polling (Midjourney / Suno / video), and balance patrol
// (channel upstream balance + user quota) jobs into the system task framework
// so a DB lease dedups execution across multiple master instances and each
// run is recorded as one task row. Call this before service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})
	service.RegisterSystemTaskHandler(channelBalanceHandler{})
	service.RegisterSystemTaskHandler(userQuotaPatrolHandler{})
	service.RegisterSystemTaskHandler(wecomDailyReportHandler{})
	service.RegisterSystemTaskHandler(channelWatchdogHandler{})
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		channelUpstreamModelUpdateTaskDefaultIntervalMinutes,
	)
	if intervalMinutes < 1 {
		intervalMinutes = channelUpstreamModelUpdateTaskDefaultIntervalMinutes
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// channelBalanceHandler 定时巡检渠道上游余额：更新余额、按需禁用、低余额预警。
// 手动的全量刷新走 HTTP 接口，不经过本任务。
type channelBalanceHandler struct{}

func (channelBalanceHandler) Type() string { return model.SystemTaskTypeChannelBalance }

func (channelBalanceHandler) Enabled() bool {
	return operation_setting.GetBalancePatrolSetting().ChannelEnabled
}

func (channelBalanceHandler) Interval() time.Duration {
	minutes := operation_setting.NormalizeBalancePatrolMinutes(operation_setting.GetBalancePatrolSetting().ChannelMinutes)
	return time.Duration(minutes * float64(time.Minute))
}

func (channelBalanceHandler) NewPayload() any { return nil }

func (channelBalanceHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := runChannelBalanceTask(ctx, service.NewSystemTaskProgressReporter(task, runnerID), true)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// userQuotaPatrolHandler 定时巡检用户剩余额度，聚合推送低余额预警。
type userQuotaPatrolHandler struct{}

func (userQuotaPatrolHandler) Type() string { return model.SystemTaskTypeUserQuota }

func (userQuotaPatrolHandler) Enabled() bool {
	return operation_setting.GetBalancePatrolSetting().UserEnabled
}

func (userQuotaPatrolHandler) Interval() time.Duration {
	minutes := operation_setting.NormalizeBalancePatrolMinutes(operation_setting.GetBalancePatrolSetting().UserMinutes)
	return time.Duration(minutes * float64(time.Minute))
}

func (userQuotaPatrolHandler) NewPayload() any { return nil }

func (userQuotaPatrolHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := service.RunUserQuotaPatrolCheck(ctx)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// wecomDailyReportHandler 企微智能机器人每日运营日报。Enabled() 收窄到报时
// 后 10 分钟窗口，避免全天产生空调度行；Run 内按日期幂等去重。
type wecomDailyReportHandler struct{}

func (wecomDailyReportHandler) Type() string { return model.SystemTaskTypeDailyReport }

func (wecomDailyReportHandler) Enabled() bool {
	setting := operation_setting.GetWecomAIBotSetting()
	if !setting.Enabled || !setting.ReportEnabled {
		return false
	}
	now := time.Now()
	return now.Hour() == setting.ReportHour && now.Minute() < 10
}

func (wecomDailyReportHandler) Interval() time.Duration { return 10 * time.Minute }

type wecomDailyReportTaskPayload struct {
	Hour int `json:"hour"`
}

func (wecomDailyReportHandler) NewPayload() any {
	return wecomDailyReportTaskPayload{Hour: operation_setting.GetWecomAIBotSetting().ReportHour}
}

func (wecomDailyReportHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := wecomDailyReportTaskPayload{Hour: -1}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	hour := operation_setting.NormalizeWecomAIBotReportHour(payload.Hour)
	summary, err := wecomaibot.RunDailyReportTask(ctx, hour)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// channelWatchdogHandler 渠道健康看门狗：按窗口聚合错误日志，超阈值推送告警。
type channelWatchdogHandler struct{}

func (channelWatchdogHandler) Type() string { return model.SystemTaskTypeWatchdog }

func (channelWatchdogHandler) Enabled() bool {
	return operation_setting.GetBalancePatrolSetting().WatchdogEnabled
}

func (channelWatchdogHandler) Interval() time.Duration {
	minutes := operation_setting.NormalizeWatchdogMinutes(operation_setting.GetBalancePatrolSetting().WatchdogMinutes)
	return time.Duration(minutes) * time.Minute
}

func (channelWatchdogHandler) NewPayload() any { return nil }

func (channelWatchdogHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := service.RunChannelHealthWatchdog()
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}
