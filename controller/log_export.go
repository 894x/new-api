package controller

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type logExportRequest = service.LogExportRequest

type selfLogExportRequest struct {
	View           service.LogExportView `json:"view"`
	Fields         []string              `json:"fields"`
	StartTimestamp int64                 `json:"start_timestamp"`
	EndTimestamp   int64                 `json:"end_timestamp"`
	ModelName      string                `json:"model_name"`
	TokenName      string                `json:"token_name"`
	RequestID      string                `json:"request_id"`
}

// Keep the existing synchronous API for clients; the dashboard queues jobs via
// /exports. Both paths use the same field allowlist and reconciliation rules.
func ExportSelfLogs(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	userID := c.GetInt("id")
	if userID <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication required"})
		return
	}
	var input selfLogExportRequest
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiError(c, errors.New("invalid export request"))
		return
	}
	if input.View != "" && input.View != service.LogExportViewSelf {
		common.ApiError(c, errors.New("only the self export view is available"))
		return
	}
	request := service.LogExportRequest{View: service.LogExportViewSelf, Fields: input.Fields, StartTimestamp: input.StartTimestamp, EndTimestamp: input.EndTimestamp, ModelName: input.ModelName, TokenName: input.TokenName, RequestID: input.RequestID}
	if err := request.Validate(); err != nil {
		common.ApiError(c, err)
		return
	}
	writeUsageLogArchive(c, request)
}

func ExportLogs(c *gin.Context) {
	var request logExportRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, errors.New("invalid export request"))
		return
	}
	if request.View != service.LogExportViewUpstream && request.View != service.LogExportViewDownstream {
		common.ApiError(c, errors.New("unsupported administrative export view"))
		return
	}
	if err := request.Validate(); err != nil {
		common.ApiError(c, err)
		return
	}
	writeUsageLogArchive(c, request)
}

func writeUsageLogArchive(c *gin.Context, request service.LogExportRequest) {
	file, err := os.CreateTemp("", "new-api-log-export-*.zip")
	if err != nil {
		common.ApiError(c, errors.New("unable to export consumption data; try again later"))
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	_, size, err := service.StreamLogExportArchive(c.Request.Context(), file, request, c.GetInt("id"), model.GetLogExportMaxRows(), nil)
	if err != nil {
		common.SysError("failed to export usage logs: " + err.Error())
		var limitError *service.LogExportLimitError
		if errors.As(err, &limitError) {
			common.ApiError(c, limitError)
			return
		}
		common.ApiError(c, errors.New("unable to export consumption data; try again later"))
		return
	}
	if _, err = file.Seek(0, 0); err != nil {
		common.ApiError(c, errors.New("unable to export consumption data; try again later"))
		return
	}
	filename := fmt.Sprintf("usage-%s-%s.zip", request.View, time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.DataFromReader(http.StatusOK, size, "application/zip", file, nil)
}
