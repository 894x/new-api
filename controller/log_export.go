package controller

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const maxLogExportRows = 100000

type logExportRequest struct {
	View              service.LogExportView `json:"view"`
	Fields            []string              `json:"fields"`
	StartTimestamp    int64                 `json:"start_timestamp"`
	EndTimestamp      int64                 `json:"end_timestamp"`
	ModelName         string                `json:"model_name"`
	Username          string                `json:"username"`
	TokenName         string                `json:"token_name"`
	ChannelID         int                   `json:"channel"`
	Group             string                `json:"group"`
	RequestID         string                `json:"request_id"`
	UpstreamRequestID string                `json:"upstream_request_id"`
}

type selfLogExportRequest struct {
	View           service.LogExportView `json:"view"`
	Fields         []string              `json:"fields"`
	StartTimestamp int64                 `json:"start_timestamp"`
	EndTimestamp   int64                 `json:"end_timestamp"`
	ModelName      string                `json:"model_name"`
	TokenName      string                `json:"token_name"`
	RequestID      string                `json:"request_id"`
}

func ExportSelfLogs(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	userID := c.GetInt("id")
	if userID <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication required"})
		return
	}

	var request selfLogExportRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, errors.New("invalid export request"))
		return
	}
	if request.View != "" && request.View != service.LogExportViewSelf {
		common.ApiError(c, errors.New("only the self export view is available"))
		return
	}
	if len(request.Fields) == 0 {
		request.Fields = service.DefaultLogExportFields(service.LogExportViewSelf)
	}
	if err := service.ValidateLogExportFields(service.LogExportViewSelf, request.Fields); err != nil {
		common.ApiError(c, err)
		return
	}
	if request.StartTimestamp != 0 && request.EndTimestamp != 0 && request.StartTimestamp > request.EndTimestamp {
		common.ApiError(c, errors.New("start timestamp must not be later than end timestamp"))
		return
	}

	logs, total, err := model.GetUserConsumptionLogsForExport(userID, model.SelfLogExportQuery{
		StartTimestamp: request.StartTimestamp,
		EndTimestamp:   request.EndTimestamp,
		ModelName:      request.ModelName,
		TokenName:      request.TokenName,
		RequestID:      request.RequestID,
	}, maxLogExportRows)
	if err != nil {
		common.SysError("failed to export user consumption logs: " + err.Error())
		common.ApiError(c, errors.New("unable to export consumption data; try again later"))
		return
	}
	if total > maxLogExportRows || len(logs) > maxLogExportRows {
		common.ApiError(c, fmt.Errorf("export exceeds %d records; narrow the filters", maxLogExportRows))
		return
	}
	rows := service.BuildLogExportRows(service.LogExportViewSelf, logs)
	summaries := service.BuildLogExportSummary(service.LogExportViewSelf, rows)
	var archive bytes.Buffer
	if err := service.WriteLogExportArchive(&archive, service.LogExportViewSelf, request.Fields, rows, summaries); err != nil {
		common.SysError("failed to write user consumption export: " + err.Error())
		common.ApiError(c, errors.New("unable to export consumption data; try again later"))
		return
	}
	filename := fmt.Sprintf("usage-self-%s.zip", time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/zip", archive.Bytes())
}

func ExportLogs(c *gin.Context) {
	var request logExportRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, fmt.Errorf("invalid export request: %w", err))
		return
	}
	if request.View != service.LogExportViewUpstream && request.View != service.LogExportViewDownstream {
		common.ApiError(c, errors.New("unsupported administrative export view"))
		return
	}
	if len(request.Fields) == 0 {
		request.Fields = service.DefaultLogExportFields(request.View)
	}
	if err := service.ValidateLogExportFields(request.View, request.Fields); err != nil {
		common.ApiError(c, err)
		return
	}
	if request.StartTimestamp != 0 && request.EndTimestamp != 0 && request.StartTimestamp > request.EndTimestamp {
		common.ApiError(c, errors.New("start timestamp must not be later than end timestamp"))
		return
	}

	query := model.LogExportQuery{
		StartTimestamp:    request.StartTimestamp,
		EndTimestamp:      request.EndTimestamp,
		ModelName:         request.ModelName,
		Username:          request.Username,
		TokenName:         request.TokenName,
		ChannelID:         request.ChannelID,
		Group:             request.Group,
		RequestID:         request.RequestID,
		UpstreamRequestID: request.UpstreamRequestID,
	}
	if request.View == service.LogExportViewDownstream {
		query.ChannelID = 0
		query.UpstreamRequestID = ""
	}
	logs, total, err := model.GetLogsForExport(query, maxLogExportRows)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if total > maxLogExportRows || len(logs) > maxLogExportRows {
		observedTotal := total
		if int64(len(logs)) > observedTotal {
			observedTotal = int64(len(logs))
		}
		common.ApiError(c, fmt.Errorf("export contains at least %d log records; narrow the filters to at most %d records", observedTotal, maxLogExportRows))
		return
	}
	if request.View == service.LogExportViewDownstream {
		requestIDs := make([]string, 0, len(logs))
		requestIDSet := make(map[string]struct{}, len(logs))
		logsWithoutRequestID := make([]*model.Log, 0)
		for _, logItem := range logs {
			if logItem.RequestId == "" {
				logsWithoutRequestID = append(logsWithoutRequestID, logItem)
				continue
			}
			if _, ok := requestIDSet[logItem.RequestId]; ok {
				continue
			}
			requestIDSet[logItem.RequestId] = struct{}{}
			requestIDs = append(requestIDs, logItem.RequestId)
		}
		completeLogs, truncated, loadErr := model.GetUsageLogsByRequestIDsForExport(requestIDs, maxLogExportRows-len(logsWithoutRequestID))
		if loadErr != nil {
			common.ApiError(c, loadErr)
			return
		}
		if truncated {
			common.ApiError(c, fmt.Errorf("expanded request histories exceed %d log records; narrow the filters", maxLogExportRows))
			return
		}
		logs = append(logsWithoutRequestID, completeLogs...)
	}

	rows := service.BuildLogExportRows(request.View, logs)
	if request.View == service.LogExportViewDownstream {
		filteredRows := rows[:0]
		for _, row := range rows {
			if request.StartTimestamp != 0 && row.CreatedAt < request.StartTimestamp {
				continue
			}
			if request.EndTimestamp != 0 && row.CreatedAt > request.EndTimestamp {
				continue
			}
			filteredRows = append(filteredRows, row)
		}
		rows = filteredRows
	}
	summaries := service.BuildLogExportSummary(request.View, rows)
	var archive bytes.Buffer
	if err = service.WriteLogExportArchive(&archive, request.View, request.Fields, rows, summaries); err != nil {
		common.ApiError(c, err)
		return
	}

	filename := fmt.Sprintf("usage-%s-%s.zip", request.View, time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/zip", archive.Bytes())
}
