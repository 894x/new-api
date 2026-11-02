package service

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"

	"github.com/QuantumNous/new-api/model"
)

var ErrLogExportFileTooLarge = errors.New("Export file exceeds 1 GiB. Narrow the filters and try again.")

// LogExportArchiveWriter retains only grouped summary counters. Detail records
// are written to the ZIP stream as they arrive, never collected in a slice.
type LogExportArchiveWriter struct {
	archive  *zip.Writer
	details  *csv.Writer
	fields   []string
	summary  logExportSummaryAccumulator
	RowCount int
}

func NewLogExportArchiveWriter(writer io.Writer, view LogExportView, fields []string) (*LogExportArchiveWriter, error) {
	if err := ValidateLogExportFields(view, fields); err != nil {
		return nil, err
	}
	archive := zip.NewWriter(writer)
	file, err := archive.Create(fmt.Sprintf("%s-details.csv", view))
	if err != nil {
		return nil, err
	}
	if _, err = file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return nil, err
	}
	details := csv.NewWriter(file)
	if err = details.Write(fields); err != nil {
		return nil, err
	}
	return &LogExportArchiveWriter{archive: archive, details: details, fields: fields, summary: logExportSummaryAccumulator{view: view}}, nil
}

func (writer *LogExportArchiveWriter) WriteRows(rows []LogExportRow) error {
	if err := writeLogExportDetailRows(writer.details, writer.fields, rows); err != nil {
		return err
	}
	writer.details.Flush()
	if err := writer.details.Error(); err != nil {
		return err
	}
	writer.summary.Add(rows)
	writer.RowCount += len(rows)
	return nil
}

func (writer *LogExportArchiveWriter) Close() error {
	writer.details.Flush()
	if err := writer.details.Error(); err != nil {
		return err
	}
	file, err := writer.archive.Create(fmt.Sprintf("%s-summary.csv", writer.summary.view))
	if err != nil {
		return err
	}
	if err = writeLogExportSummary(file, writer.summary.view, writer.summary.Summaries()); err != nil {
		return err
	}
	return writer.archive.Close()
}

type logExportRowStream struct {
	request   LogExportRequest
	archive   *LogExportArchiveWriter
	requestID string
	terminal  *model.Log
}

func (stream *logExportRowStream) WriteLogs(logs []*model.Log) error {
	if stream.request.View != LogExportViewDownstream {
		return stream.archive.WriteRows(BuildLogExportRows(stream.request.View, logs))
	}
	for _, item := range logs {
		if item.RequestId != stream.requestID || item.RequestId == "" {
			if err := stream.FlushTerminal(); err != nil {
				return err
			}
			stream.requestID = item.RequestId
		}
		if !isExportableUsageLog(item) || isViolationFeeLog(item) {
			continue
		}
		if stream.terminal == nil || stream.terminal.Type != model.LogTypeConsume || item.Type == model.LogTypeConsume {
			stream.terminal = item
		}
		if item.RequestId == "" {
			if err := stream.FlushTerminal(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (stream *logExportRowStream) FlushTerminal() error {
	if stream.terminal == nil {
		return nil
	}
	item := stream.terminal
	stream.terminal = nil
	if stream.request.StartTimestamp != 0 && item.CreatedAt < stream.request.StartTimestamp {
		return nil
	}
	if stream.request.EndTimestamp != 0 && item.CreatedAt > stream.request.EndTimestamp {
		return nil
	}
	return stream.archive.WriteRows(BuildLogExportRows(LogExportViewDownstream, []*model.Log{item}))
}

func StreamLogExportArchive(ctx context.Context, writer io.Writer, request LogExportRequest, userID int, limit int) (rowCount int, fileSize int64, err error) {
	if err = request.Validate(); err != nil {
		return 0, 0, err
	}
	if request.View == LogExportViewSelf && userID <= 0 {
		return 0, 0, errors.New("authentication required")
	}
	bounded := &logExportFileWriter{ctx: ctx, writer: writer}
	archive, err := NewLogExportArchiveWriter(bounded, request.View, request.Fields)
	if err != nil {
		return 0, 0, err
	}
	stream := logExportRowStream{request: request, archive: archive}
	query := model.LogExportQuery{StartTimestamp: request.StartTimestamp, EndTimestamp: request.EndTimestamp, ModelName: request.ModelName, Username: request.Username, TokenName: request.TokenName, ChannelID: request.ChannelID, Group: request.Group, RequestID: request.RequestID, UpstreamRequestID: request.UpstreamRequestID}
	ownerID := 0
	if request.View == LogExportViewSelf {
		ownerID = userID
	}
	scanned, err := model.VisitLogsForExport(ctx, query, ownerID, request.View == LogExportViewDownstream, limit, stream.WriteLogs)
	if err != nil {
		return 0, bounded.written, err
	}
	if scanned > int64(limit) {
		return 0, bounded.written, &LogExportLimitError{Limit: limit}
	}
	if err = stream.FlushTerminal(); err != nil {
		return 0, bounded.written, err
	}
	if err = ctx.Err(); err != nil {
		return 0, bounded.written, err
	}
	if err = archive.Close(); err != nil {
		return 0, bounded.written, err
	}
	return archive.RowCount, bounded.written, nil
}
