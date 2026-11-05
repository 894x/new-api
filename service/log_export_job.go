package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type LogExportRequest struct {
	View              LogExportView `json:"view"`
	Fields            []string      `json:"fields"`
	StartTimestamp    int64         `json:"start_timestamp"`
	EndTimestamp      int64         `json:"end_timestamp"`
	ModelName         string        `json:"model_name"`
	Username          string        `json:"username,omitempty"`
	TokenName         string        `json:"token_name"`
	ChannelID         int           `json:"channel,omitempty"`
	Group             string        `json:"group,omitempty"`
	RequestID         string        `json:"request_id"`
	UpstreamRequestID string        `json:"upstream_request_id,omitempty"`
}

func (request *LogExportRequest) Validate() error {
	if request.View != LogExportViewSelf && request.View != LogExportViewUpstream && request.View != LogExportViewDownstream {
		return errors.New("unsupported export view")
	}
	if request.StartTimestamp < 0 || request.EndTimestamp < 0 {
		return errors.New("invalid export time range")
	}
	if request.StartTimestamp != 0 && request.EndTimestamp != 0 && request.StartTimestamp > request.EndTimestamp {
		return errors.New("start timestamp must not be later than end timestamp")
	}
	if len(request.Fields) == 0 {
		request.Fields = DefaultLogExportFields(request.View)
	}
	return ValidateLogExportFields(request.View, request.Fields)
}

type LogExportLimitError struct{ Limit int }

func (err *LogExportLimitError) Error() string {
	return fmt.Sprintf("export exceeds %d records; narrow the filters", err.Limit)
}

type logExportHandler struct{}

func (logExportHandler) Type() string { return model.SystemTaskTypeLogExport }
func (logExportHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	RunLogExportJob(ctx, task, runnerID)
}

// RunLogExportJob executes an already claimed export under the system task lease.
func RunLogExportJob(ctx context.Context, task *model.SystemTask, runnerID string) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Hour)
	defer cancel()
	defer notifySystemTaskRunner()
	var request LogExportRequest
	var record model.LogExportRecord
	err := task.DecodePayload(&request)
	if err == nil {
		err = model.DB.WithContext(ctx).Where("task_id = ?", task.TaskID).Take(&record).Error
	}
	if err == nil {
		err = request.Validate()
	}
	if err == nil {
		err = generateLogExport(ctx, task, runnerID, record, request)
	}
	if err == nil {
		return
	}
	common.SysError(fmt.Sprintf("log export %s failed: %v", task.TaskID, err))
	message := "Unable to generate export; please try again."
	var limitError *LogExportLimitError
	if errors.As(err, &limitError) {
		message = limitError.Error()
	}
	if errors.Is(err, ErrLogExportFileTooLarge) {
		message = err.Error()
	}
	if finishErr := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusFailed, nil, message); finishErr != nil {
		common.SysError(fmt.Sprintf("failed to finish log export %s: %v", task.TaskID, finishErr))
	}
}

func generateLogExport(ctx context.Context, task *model.SystemTask, runnerID string, record model.LogExportRecord, request LogExportRequest) error {
	progress := logExportProgressReporter{taskID: task.TaskID, runnerID: runnerID, state: model.LogExportProgress{Stage: "counting"}}
	if err := progress.persist(true); err != nil {
		return err
	}
	file, err := os.CreateTemp("", "new-api-log-export-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	rowCount, fileSize, err := StreamLogExportArchive(ctx, file, request, record.UserID, record.MaxRows, progress.reportRows)
	if err != nil {
		return err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	progress.state.Stage, progress.state.Percent = "storing", 90
	progress.state.RowCount, progress.state.TotalBytes = rowCount, fileSize
	if err = progress.persist(true); err != nil {
		return err
	}
	buffer := make([]byte, model.LogExportChunkSize)
	for sequence := 0; ; sequence++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := io.ReadFull(file, buffer)
		if n > 0 {
			chunk := model.LogExportChunk{TaskID: task.TaskID, Sequence: sequence, Data: buffer[:n]}
			if err = model.DB.WithContext(ctx).Create(&chunk).Error; err != nil {
				return err
			}
			progress.state.StoredBytes += int64(n)
			progress.state.Percent = 90 + int(progress.state.StoredBytes*9/max(1, fileSize))
			if err = progress.persist(progress.state.StoredBytes == fileSize); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	filename := fmt.Sprintf("usage-%s-%s.zip", request.View, time.Unix(record.CreatedAt, 0).UTC().Format("20060102-150405"))
	err = model.DB.WithContext(ctx).Model(&model.LogExportRecord{}).Where("task_id = ?", task.TaskID).Updates(map[string]any{"filename": filename, "row_count": rowCount, "file_size": fileSize, "expires_at": common.GetTimestamp() + model.LogExportRetentionSeconds}).Error
	if err != nil {
		return err
	}
	return model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, map[string]int{"row_count": rowCount}, "")
}

// Export progress includes row generation and byte storage. Unlike the generic
// task reporter, it reserves 100% for the committed terminal task status and
// propagates lease/persist failures before more artifact data is written.
type logExportProgressReporter struct {
	taskID    string
	runnerID  string
	state     model.LogExportProgress
	lastWrite time.Time
	lastStage string
}

func (reporter *logExportProgressReporter) reportRows(processed, total int64) error {
	reporter.state.Stage = "exporting"
	reporter.state.Processed, reporter.state.Total = processed, total
	reporter.state.Percent = 90
	if total > 0 {
		reporter.state.Percent = int(min(processed, total) * 90 / total)
	}
	return reporter.persist(processed == total)
}

func (reporter *logExportProgressReporter) persist(force bool) error {
	now := time.Now()
	if !force && reporter.lastStage == reporter.state.Stage && now.Sub(reporter.lastWrite) < 2*time.Second {
		return nil
	}
	if err := model.UpdateSystemTaskState(reporter.taskID, reporter.runnerID, reporter.state); err != nil {
		return err
	}
	reporter.lastWrite, reporter.lastStage = now, reporter.state.Stage
	return nil
}

type logExportFileWriter struct {
	ctx     context.Context
	writer  io.Writer
	written int64
}

func (writer *logExportFileWriter) Write(data []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	if writer.written+int64(len(data)) > model.LogExportMaxFileBytes {
		return 0, ErrLogExportFileTooLarge
	}
	n, err := writer.writer.Write(data)
	writer.written += int64(n)
	return n, err
}

type logExportCleanupHandler struct{}

func (logExportCleanupHandler) Type() string            { return "log_export_cleanup" }
func (logExportCleanupHandler) Enabled() bool           { return true }
func (logExportCleanupHandler) Interval() time.Duration { return time.Hour }
func (logExportCleanupHandler) NewPayload() any         { return nil }
func (logExportCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	err := model.ExpireLogExportFiles(ctx)
	status, message := model.SystemTaskStatusSucceeded, ""
	if err != nil {
		status = model.SystemTaskStatusFailed
		message = "Unable to clean up expired exports."
		common.SysError(err.Error())
	}
	if err = model.FinishSystemTask(task.TaskID, runnerID, status, nil, message); err != nil {
		common.SysError(err.Error())
	}
}

func init() {
	RegisterSystemTaskHandler(logExportHandler{})
	RegisterSystemTaskHandler(logExportCleanupHandler{})
}

func QueueLogExport(userID int, request LogExportRequest) (*model.LogExportRecord, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	// Freeze the time window before queuing so later requests do not enter it.
	if request.EndTimestamp == 0 {
		request.EndTimestamp = common.GetTimestamp()
	}
	record, err := model.CreateLogExportRecord(userID, string(request.View), request)
	if err == nil {
		notifySystemTaskRunner()
	}
	return record, err
}
