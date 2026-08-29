package controller

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestExportLogsRejectsUpstreamFieldsInDownstreamView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/log/export", strings.NewReader(`{"view":"downstream","fields":["request_id","channel_id"]}`))
	context.Request.Header.Set("Content-Type", "application/json")

	ExportLogs(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "channel_id")
}

func TestExportLogsDownstreamUsesTrueTerminalRecordBeforeReportFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "controller-log-export.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	require.NoError(t, db.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY, name TEXT)").Error)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB, model.LOG_DB = db, db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	logs := []*model.Log{
		{Id: 1, CreatedAt: 100, Type: model.LogTypeError, RequestId: "req-success", ChannelId: 10, ModelName: "gpt-x"},
		{Id: 2, CreatedAt: 101, Type: model.LogTypeConsume, RequestId: "req-success", ChannelId: 12, ModelName: "gpt-x"},
		{Id: 3, CreatedAt: 150, Type: model.LogTypeError, RequestId: "req-outside", ChannelId: 10, ModelName: "gpt-x"},
		{Id: 4, CreatedAt: 250, Type: model.LogTypeConsume, RequestId: "req-outside", ChannelId: 12, ModelName: "gpt-x"},
	}
	require.NoError(t, db.Create(&logs).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/log/export", strings.NewReader(`{"view":"downstream","fields":["request_id","status"],"start_timestamp":90,"end_timestamp":200,"channel":10}`))
	context.Request.Header.Set("Content-Type", "application/json")

	ExportLogs(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/zip", recorder.Header().Get("Content-Type"))
	archive, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	require.NoError(t, err)
	require.NotEmpty(t, archive.File)
	detail, err := archive.File[0].Open()
	require.NoError(t, err)
	defer detail.Close()
	detailBytes, err := io.ReadAll(detail)
	require.NoError(t, err)
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(detailBytes, []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"request_id", "status"}, {"req-success", "success"}}, records)
}
