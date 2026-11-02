package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetLogExportConfig(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"max_rows": model.GetLogExportMaxRows(), "retention_days": model.LogExportRetentionSeconds / 86400, "max_file_bytes": model.LogExportMaxFileBytes}})
}

func CreateLogExportJob(c *gin.Context)     { createLogExportJob(c, false) }
func CreateSelfLogExportJob(c *gin.Context) { createLogExportJob(c, true) }

func createLogExportJob(c *gin.Context, self bool) {
	c.Header("Cache-Control", "private, no-store")
	userID := c.GetInt("id")
	if userID <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication required"})
		return
	}
	var request service.LogExportRequest
	if err := common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024), &request); err != nil {
		common.ApiError(c, errors.New("invalid export request"))
		return
	}
	if self {
		if request.View != "" && request.View != service.LogExportViewSelf {
			common.ApiError(c, errors.New("only the self export view is available"))
			return
		}
		request = service.LogExportRequest{View: service.LogExportViewSelf, Fields: request.Fields, StartTimestamp: request.StartTimestamp, EndTimestamp: request.EndTimestamp, ModelName: request.ModelName, TokenName: request.TokenName, RequestID: request.RequestID}
	} else if request.View != service.LogExportViewUpstream && request.View != service.LogExportViewDownstream {
		common.ApiError(c, errors.New("unsupported administrative export view"))
		return
	}
	if err := request.Validate(); err != nil {
		common.ApiError(c, err)
		return
	}
	record, err := service.QueueLogExport(userID, request)
	if err != nil {
		common.SysError("failed to queue log export: " + err.Error())
		common.ApiError(c, errors.New("Unable to queue export. Wait for your active export to finish, then try again."))
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": record})
}

func ListLogExportJobs(c *gin.Context)     { listLogExportJobs(c, false) }
func ListSelfLogExportJobs(c *gin.Context) { listLogExportJobs(c, true) }

func listLogExportJobs(c *gin.Context, self bool) {
	c.Header("Cache-Control", "private, no-store")
	userID := c.GetInt("id")
	if userID <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication required"})
		return
	}
	offset, _ := strconv.Atoi(c.Query("offset"))
	records, total, err := model.ListLogExportRecords(c.Request.Context(), userID, self, offset, 20)
	if err != nil {
		common.SysError(err.Error())
		common.ApiError(c, errors.New("Unable to load export history."))
		return
	}
	for i := range records {
		var request service.LogExportRequest
		if err := common.UnmarshalJsonStr(records[i].Request, &request); err == nil {
			records[i].Filters = request
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": records, "total": total})
}

func DownloadLogExportFile(c *gin.Context)     { downloadLogExportFile(c, false) }
func DownloadSelfLogExportFile(c *gin.Context) { downloadLogExportFile(c, true) }

func downloadLogExportFile(c *gin.Context, self bool) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	userID := c.GetInt("id")
	if userID <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication required"})
		return
	}
	record, err := model.GetLogExportRecord(c.Request.Context(), c.Param("task_id"), userID, self)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Export not found."})
		return
	}
	if record.Status != model.SystemTaskStatusSucceeded {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Export is not ready."})
		return
	}
	if record.ExpiresAt <= common.GetTimestamp() || record.FileSize <= 0 {
		c.JSON(http.StatusGone, gin.H{"success": false, "message": "Export file has expired. Create a new export."})
		return
	}
	// Validate availability before sending headers, then stream one bounded chunk
	// at a time. No public URL or untrusted filesystem path is exposed.
	var count int64
	if err = model.DB.WithContext(c.Request.Context()).Model(&model.LogExportChunk{}).Where("task_id = ?", record.TaskID).Count(&count).Error; err != nil || count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Export file is unavailable."})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, record.Filename))
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Length", strconv.FormatInt(record.FileSize, 10))
	for sequence := 0; int64(sequence) < count; sequence++ {
		var chunk model.LogExportChunk
		err = model.DB.WithContext(c.Request.Context()).Where("task_id = ? AND sequence = ?", record.TaskID, sequence).Take(&chunk).Error
		if err != nil {
			common.SysError("failed to read log export chunk: " + err.Error())
			return
		}
		if _, err = c.Writer.Write(chunk.Data); err != nil {
			return
		}
	}
}
