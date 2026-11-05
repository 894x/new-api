package controller

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestAsyncLogExportDurableLifecycle(t *testing.T) {
	var driver gorm.Dialector
	dialect := common.DatabaseType(os.Getenv("TEST_TASK_DB_DIALECT"))
	switch dialect {
	case "", common.DatabaseTypeSQLite:
		dialect = common.DatabaseTypeSQLite
		driver = sqlite.Open(filepath.Join(t.TempDir(), "async-export.db"))
	case common.DatabaseTypeMySQL:
		require.NotEmpty(t, os.Getenv("TEST_MYSQL_DSN"))
		driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
	case common.DatabaseTypePostgreSQL:
		require.NotEmpty(t, os.Getenv("TEST_POSTGRES_DSN"))
		driver = postgres.New(postgres.Config{DSN: os.Getenv("TEST_POSTGRES_DSN"), PreferSimpleProtocol: true})
	default:
		t.Fatalf("unexpected dialect %q", dialect)
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	logDB := db
	logDSN := os.Getenv("TEST_EXPORT_LOG_DSN")
	if dialect == common.DatabaseTypeSQLite {
		logDB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "async-export-logs.db")), &gorm.Config{})
	} else if logDSN != "" {
		if dialect == common.DatabaseTypeMySQL {
			logDB, err = gorm.Open(mysql.Open(logDSN), &gorm.Config{})
		} else {
			logDB, err = gorm.Open(postgres.New(postgres.Config{DSN: logDSN, PreferSimpleProtocol: true}), &gorm.Config{})
		}
	}
	require.NoError(t, err)
	if logDB != db {
		logSQLDB, err := logDB.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, logSQLDB.Close()) })
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{model.LogExportMaxRowsOptionKey: "10"}
	common.OptionMapRWMutex.Unlock()
	model.DB, model.LOG_DB = db, logDB
	common.SetDatabaseTypes(dialect, dialect)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	var version string
	if dialect == common.DatabaseTypeSQLite {
		require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
	} else {
		require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
	}
	t.Logf("database: %s %s", dialect, version)
	models := []any{&model.LogExportChunk{}, &model.LogExportRecord{}, &model.SystemTaskLock{}, &model.SystemTask{}, &model.Option{}, &model.Log{}}
	for _, mode := range []string{"fresh", "upgrade"} {
		t.Run(mode, func(t *testing.T) {
			require.NoError(t, db.Migrator().DropTable(models...))
			if logDB != db {
				require.NoError(t, logDB.Migrator().DropTable(&model.Log{}))
				for range 2 {
					require.NoError(t, logDB.AutoMigrate(&model.Log{}))
				}
			}
			if mode == "upgrade" {
				// The existing release's task and option models are unchanged by
				// this additive migration; seed them before adding export tables.
				require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}, &model.Option{}, &model.Log{}))
				_, err := model.CreateSystemTask("legacy-export-fixture", map[string]string{"existing": "preserved"}, nil)
				require.NoError(t, err)
			}
			for range 2 {
				require.NoError(t, db.AutoMigrate(models...))
			}
			require.True(t, db.Migrator().HasIndex(&model.SystemTask{}, "ActiveKey"))
			require.True(t, db.Migrator().HasIndex(&model.LogExportRecord{}, "UserID"))
			if mode == "upgrade" {
				legacy, err := model.GetActiveSystemTask("legacy-export-fixture")
				require.NoError(t, err)
				require.NotNil(t, legacy)
				assert.Contains(t, legacy.Payload, "preserved")
				_, err = model.CreateSystemTask("legacy-export-fixture", nil, nil)
				require.Error(t, err, "existing uniqueness must survive repeated migration")
			}
			logs := []model.Log{
				{UserId: 41, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "mine", ModelName: "model-a", Quota: 10},
				{UserId: 42, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "OTHER_USER_SECRET", ModelName: "model-a", Quota: 99},
			}
			require.NoError(t, logDB.Create(&logs).Error)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("id", 41) })
			router.POST("/self/exports", CreateSelfLogExportJob)
			router.GET("/self/exports", ListSelfLogExportJobs)
			router.GET("/self/exports/:task_id/file", DownloadSelfLogExportFile)
			body := `{"view":"self","fields":["request_id","charged_quota"],"username":"other","channel":99,"upstream_request_id":"private","start_timestamp":1,"end_timestamp":200}`
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/self/exports", strings.NewReader(body)))
			require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
			var queued struct {
				Data model.LogExportRecord `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &queued))
			require.NotEmpty(t, queued.Data.TaskID)
			assert.Equal(t, 10, queued.Data.MaxRows)
			task, err := model.GetSystemTaskByTaskID(queued.Data.TaskID)
			require.NoError(t, err)
			assert.Equal(t, model.SystemTaskStatusPending, task.Status)
			assert.NotContains(t, task.Payload, "private")
			assert.NotContains(t, task.Payload, "other")
			_, err = service.QueueLogExport(41, service.LogExportRequest{View: service.LogExportViewSelf})
			require.Error(t, err, "one active export per account")
			// Snapshot the configured limit and keep a durable pending record even
			// when no worker/request remains attached to the creation call.
			require.NoError(t, model.UpdateOption(model.LogExportMaxRowsOptionKey, "1"))
			pending := httptest.NewRecorder()
			router.ServeHTTP(pending, httptest.NewRequest(http.MethodGet, "/self/exports/"+task.TaskID+"/file", nil))
			assert.Equal(t, http.StatusConflict, pending.Code)
			claimed, ok, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeLogExport, "export-test", common.GetTimestamp()+60)
			require.NoError(t, err)
			require.True(t, ok)
			require.NoError(t, model.UpdateSystemTaskState(task.TaskID, "export-test", model.LogExportProgress{Stage: "exporting", Percent: 45, Processed: 45, Total: 100}))
			running := httptest.NewRecorder()
			router.ServeHTTP(running, httptest.NewRequest(http.MethodGet, "/self/exports", nil))
			var runningHistory struct {
				Data []model.LogExportRecordResponse `json:"data"`
			}
			require.NoError(t, common.Unmarshal(running.Body.Bytes(), &runningHistory))
			require.Len(t, runningHistory.Data, 1)
			assert.Equal(t, 45, runningHistory.Data[0].Progress.Percent)
			assert.EqualValues(t, 45, runningHistory.Data[0].Progress.Processed)
			assert.NotContains(t, running.Body.String(), "locked_by")
			service.RunLogExportJob(context.Background(), claimed, "export-test")
			record, err := model.GetLogExportRecord(context.Background(), task.TaskID, 41, true)
			require.NoError(t, err)
			assert.Equal(t, model.SystemTaskStatusSucceeded, record.Status, record.Error)
			assert.Equal(t, 1, record.RowCount)
			assert.Equal(t, 100, record.Progress.Percent)
			assert.Equal(t, "completed", record.Progress.Stage)
			var savedProgress model.LogExportProgress
			finishedTask, err := model.GetSystemTaskByTaskID(task.TaskID)
			require.NoError(t, err)
			require.NoError(t, common.UnmarshalJsonStr(finishedTask.State, &savedProgress))
			assert.Equal(t, 99, savedProgress.Percent, "storage must not advertise 100 before task completion")
			assert.EqualValues(t, 1, savedProgress.Processed)
			assert.EqualValues(t, 1, savedProgress.Total)
			assert.Equal(t, record.FileSize, savedProgress.StoredBytes)
			assert.Equal(t, record.FileSize, savedProgress.TotalBytes)
			// Reopening/migrating the application must preserve finished artifacts.
			for range 2 {
				require.NoError(t, db.AutoMigrate(models...))
			}
			download := httptest.NewRecorder()
			router.ServeHTTP(download, httptest.NewRequest(http.MethodGet, "/self/exports/"+task.TaskID+"/file", nil))
			files := readSelfLogExportArchive(t, download)
			assert.Equal(t, []string{"mine", "10"}, files["self-details.csv"][1])
			// Exercise a real file crossing the storage chunk boundary, without
			// relying on compression ratios or synthetic performance workloads.
			var largeArchive bytes.Buffer
			archiveWriter := zip.NewWriter(&largeArchive)
			entry, err := archiveWriter.CreateHeader(&zip.FileHeader{Name: "boundary.csv", Method: zip.Store})
			require.NoError(t, err)
			_, err = entry.Write(bytes.Repeat([]byte("a"), model.LogExportChunkSize+17))
			require.NoError(t, err)
			require.NoError(t, archiveWriter.Close())
			require.NoError(t, db.Where("task_id = ?", task.TaskID).Delete(&model.LogExportChunk{}).Error)
			for sequence, start := 0, 0; start < largeArchive.Len(); sequence, start = sequence+1, start+model.LogExportChunkSize {
				end := min(start+model.LogExportChunkSize, largeArchive.Len())
				require.NoError(t, db.Create(&model.LogExportChunk{TaskID: task.TaskID, Sequence: sequence, Data: largeArchive.Bytes()[start:end]}).Error)
			}
			require.NoError(t, db.Model(&model.LogExportRecord{}).Where("task_id = ?", task.TaskID).Update("file_size", largeArchive.Len()).Error)
			chunkedDownload := httptest.NewRecorder()
			router.ServeHTTP(chunkedDownload, httptest.NewRequest(http.MethodGet, "/self/exports/"+task.TaskID+"/file", nil))
			require.Equal(t, http.StatusOK, chunkedDownload.Code)
			assert.Equal(t, strconv.Itoa(largeArchive.Len()), chunkedDownload.Header().Get("Content-Length"))
			assert.Equal(t, largeArchive.Bytes(), chunkedDownload.Body.Bytes())
			list := httptest.NewRecorder()
			router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/self/exports", nil))
			assert.Contains(t, list.Body.String(), `"status":"succeeded"`)
			assert.NotContains(t, list.Body.String(), "locked_by")
			_, err = model.GetLogExportRecord(context.Background(), task.TaskID, 42, true)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			denied := httptest.NewRecorder()
			deniedContext, _ := gin.CreateTestContext(denied)
			deniedContext.Request = httptest.NewRequest(http.MethodGet, "/self/exports/"+task.TaskID+"/file", nil)
			deniedContext.Set("id", 42)
			deniedContext.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			DownloadSelfLogExportFile(deniedContext)
			assert.Equal(t, http.StatusNotFound, denied.Code)
			assert.NotContains(t, denied.Body.String(), "mine")
			// Retain history when the administrator clears system-task history.
			_, err = model.CreateSystemTask("newer-history-fixture", nil, nil)
			require.NoError(t, err)
			_, err = model.DeleteSystemTaskHistory(model.SystemTaskFilter{})
			require.NoError(t, err)
			_, err = model.GetLogExportRecord(context.Background(), task.TaskID, 41, true)
			require.NoError(t, err)
			require.NoError(t, db.Model(&model.LogExportRecord{}).Where("task_id = ?", task.TaskID).Update("expires_at", common.GetTimestamp()-1).Error)
			expired := httptest.NewRecorder()
			router.ServeHTTP(expired, httptest.NewRequest(http.MethodGet, "/self/exports/"+task.TaskID+"/file", nil))
			assert.Equal(t, http.StatusGone, expired.Code)
			require.NoError(t, model.ExpireLogExportFiles(context.Background()))
			var chunks int64
			require.NoError(t, db.Model(&model.LogExportChunk{}).Where("task_id = ?", task.TaskID).Count(&chunks).Error)
			assert.Zero(t, chunks)
			history, total, err := model.ListLogExportRecords(context.Background(), 41, true, 0, 20)
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, history, 1)
			// A limit violation becomes a safe failure record without a file.
			require.NoError(t, logDB.Create(&model.Log{UserId: 41, CreatedAt: 101, Type: model.LogTypeRefund, RequestId: "refund", ModelName: "model-a"}).Error)
			failed, err := service.QueueLogExport(41, service.LogExportRequest{View: service.LogExportViewSelf})
			require.NoError(t, err)
			failedTask, err := model.GetSystemTaskByTaskID(failed.TaskID)
			require.NoError(t, err)
			failedTask, ok, err = model.ClaimSystemTask(failedTask.ID, model.SystemTaskTypeLogExport, "export-test", common.GetTimestamp()+60)
			require.NoError(t, err)
			require.True(t, ok)
			service.RunLogExportJob(context.Background(), failedTask, "export-test")
			failedRecord, err := model.GetLogExportRecord(context.Background(), failed.TaskID, 41, true)
			require.NoError(t, err)
			assert.Equal(t, model.SystemTaskStatusFailed, failedRecord.Status)
			assert.Equal(t, "export exceeds 1 records; narrow the filters", failedRecord.Error)
			assert.Zero(t, failedRecord.FileSize)
			assert.Zero(t, failedRecord.Progress.Percent)
			for _, tc := range []struct {
				status  model.SystemTaskStatus
				state   string
				percent int
			}{
				{model.SystemTaskStatusPending, `{"percent":90}`, 0},
				{model.SystemTaskStatusRunning, `{"stage":"storing","percent":100}`, 99},
				{model.SystemTaskStatusFailed, `{"stage":"exporting","percent":45}`, 45},
				{model.SystemTaskStatusSucceeded, `{"row_count":1}`, 100},
				{model.SystemTaskStatusRunning, `invalid legacy state`, 0},
			} {
				require.NoError(t, db.Model(&model.SystemTask{}).Where("task_id = ?", task.TaskID).Updates(map[string]any{"status": tc.status, "state": tc.state}).Error)
				progressRecord, err := model.GetLogExportRecord(context.Background(), task.TaskID, 41, true)
				require.NoError(t, err)
				assert.Equal(t, tc.percent, progressRecord.Progress.Percent)
			}
			require.NoError(t, db.Model(&model.SystemTask{}).Where("task_id = ?", task.TaskID).Updates(map[string]any{"status": model.SystemTaskStatusSucceeded, "state": finishedTask.State}).Error)
			for _, invalid := range []string{"0", "-1", "10000001", "1.5", ""} {
				require.Error(t, model.UpdateOption(model.LogExportMaxRowsOptionKey, invalid))
			}
			require.NoError(t, model.UpdateOption(model.LogExportMaxRowsOptionKey, "10000000"))
			assert.Equal(t, 10000000, model.GetLogExportMaxRows())
			// A retry chain straddles read batches; a later fee cannot replace its
			// success. Case-distinct request IDs and unfiltered empty IDs stay separate.
			chain := make([]model.Log, 1003)
			for i := range chain {
				requestID := "CaseChain"
				if i%2 != 0 {
					requestID = "casechain"
				}
				chain[i] = model.Log{UserId: 77, Username: "stream-user", Type: model.LogTypeError, CreatedAt: int64(1000 + i), RequestId: requestID, ModelName: "stream-model"}
			}
			require.NoError(t, logDB.CreateInBatches(&chain, 100).Error)
			require.NoError(t, logDB.Create(&[]model.Log{
				{UserId: 77, Username: "stream-user", Type: model.LogTypeConsume, CreatedAt: 2500, RequestId: "CaseChain", ModelName: "stream-model"},
				{UserId: 77, Username: "stream-user", Type: model.LogTypeConsume, CreatedAt: 2003, RequestId: "casechain", ModelName: "stream-model", PromptTokens: 123},
				{UserId: 77, Username: "stream-user", Type: model.LogTypeConsume, CreatedAt: 2004, RequestId: "casechain", ModelName: "stream-model", Other: `{"violation_fee":true}`},
				{UserId: 77, Username: "stream-user", Type: model.LogTypeError, CreatedAt: 1500, ModelName: "stream-model"},
				{UserId: 78, Username: "other-user", Type: model.LogTypeError, CreatedAt: 1500, ModelName: "OUTSIDE_SCOPE"},
			}).Error)
			var streamed bytes.Buffer
			request := service.LogExportRequest{View: service.LogExportViewDownstream, Fields: []string{"request_id", "status", "input_tokens"}, Username: "stream-user", StartTimestamp: 1000, EndTimestamp: 2004}
			rows, size, err := service.StreamLogExportArchive(context.Background(), &streamed, request, 0, 10000000, nil)
			require.NoError(t, err)
			assert.Equal(t, 2, rows)
			assert.EqualValues(t, streamed.Len(), size)
			zipReader, err := zip.NewReader(bytes.NewReader(streamed.Bytes()), int64(streamed.Len()))
			require.NoError(t, err)
			detailReader, err := zipReader.File[0].Open()
			require.NoError(t, err)
			detail, err := io.ReadAll(detailReader)
			require.NoError(t, err)
			require.NoError(t, detailReader.Close())
			assert.Contains(t, string(detail), "casechain,success,123")
			assert.NotContains(t, string(detail), "CaseChain")
			assert.NotContains(t, string(detail), "OUTSIDE_SCOPE")
			// The visitor returns each SQL connection before delivering a batch,
			// so another main/log operation can run even with a single connection.
			logConnection, err := logDB.DB()
			require.NoError(t, err)
			logConnection.SetMaxOpenConns(1)
			var visited int
			_, err = model.VisitLogsForExport(context.Background(), model.LogExportQuery{}, 41, false, 10, func(batch []*model.Log) error {
				visited += len(batch)
				return logDB.Exec("SELECT 1").Error
			}, nil)
			require.NoError(t, err)
			assert.Equal(t, 2, visited)
			require.NoError(t, model.UpdateOption(model.LogExportMaxRowsOptionKey, "10"))
		})
	}
}

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

func selfLogExportTestRequest(t *testing.T, userID int, request any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", userID)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/log/self/export", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	ExportSelfLogs(context)
	return recorder
}

func readSelfLogExportArchive(t *testing.T, recorder *httptest.ResponseRecorder) map[string][][]string {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/zip", recorder.Header().Get("Content-Type"), recorder.Body.String())
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	archive, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	require.NoError(t, err)
	require.Len(t, archive.File, 2)
	files := make(map[string][][]string, len(archive.File))
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		for _, secret := range []string{"CHANNEL_SECRET", "UPSTREAM_SECRET", "DIAGNOSTIC_SECRET", "OTHER_USER_SECRET", "PRICE_SECRET", "GROUP_SECRET", "USERNAME_SECRET"} {
			assert.NotContains(t, string(data), secret, file.Name)
		}
		records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
		require.NoError(t, err)
		files[file.Name] = records
	}
	return files
}

func TestExportSelfLogsOwnConsumptionLedgerAndRedaction(t *testing.T) {
	db, dialect := openTaskDialectDatabase(t, &model.Log{})
	logDB, logDialect := db, dialect
	if dsn := os.Getenv("TEST_TASK_LOG_CLICKHOUSE_DSN"); dsn != "" {
		var err error
		logDB, err = gorm.Open(clickhouse.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("selfexport_%d_", time.Now().UnixNano())}})
		require.NoError(t, err)
		require.NoError(t, logDB.Set("gorm:table_options", "ENGINE=MergeTree() ORDER BY (created_at, request_id)").AutoMigrate(&model.Log{}))
		sqlDB, err := logDB.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		t.Cleanup(func() { require.NoError(t, logDB.Migrator().DropTable(&model.Log{})) })
		var version string
		require.NoError(t, logDB.Raw("SELECT version()").Scan(&version).Error)
		t.Logf("log database: clickhouse %s", version)
		logDialect = common.DatabaseTypeClickHouse
	}
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousQuotaPerUnit := common.QuotaPerUnit
	model.DB, model.LOG_DB = db, logDB
	common.SetDatabaseTypes(dialect, logDialect)
	common.QuotaPerUnit = 1 // Current settings must never reprice historical entries.
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	other := map[string]any{
		"quota_per_unit": 500000, "cache_tokens": 40, "group_ratio": 0.5,
		"channel_name": "CHANNEL_SECRET", "upstream_model_name": "UPSTREAM_SECRET",
		"admin_info":         map[string]any{"api_key": "DIAGNOSTIC_SECRET"},
		"billing_expression": "PRICE_SECRET", "model_price": 999,
	}
	logs := []*model.Log{
		{UserId: 7, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "=client-model", TokenName: "=token", Quota: 1000, PromptTokens: 100, CompletionTokens: 20, Other: common.MapToJsonStr(other)},
		{UserId: 7, CreatedAt: 101, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "=client-model", TokenName: "=token", Quota: 250, PromptTokens: 10, CompletionTokens: 5, Other: `{"quota_per_unit":500000}`},
		{UserId: 7, CreatedAt: 102, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "=client-model", TokenName: "=token", Quota: 100, PromptTokens: 100, CompletionTokens: 20, Other: `{"quota_per_unit":500000,"violation_fee":true,"cache_tokens":40}`},
		{UserId: 7, CreatedAt: 103, Type: model.LogTypeRefund, RequestId: "shared-request", ModelName: "=client-model", TokenName: "=token", Quota: 200, PromptTokens: 100, CompletionTokens: 20, Other: `{"quota_per_unit":500000,"cache_tokens":40}`},
		{UserId: 7, CreatedAt: 104, Type: model.LogTypeConsume, RequestId: "unknown-snapshot", ModelName: "=client-model", TokenName: "=token", Quota: 50, PromptTokens: 1},
		{UserId: 7, CreatedAt: 105, Type: model.LogTypeError, RequestId: "failed-request", ModelName: "DIAGNOSTIC_SECRET", Content: "DIAGNOSTIC_SECRET"},
		{UserId: 7, CreatedAt: 106, Type: model.LogTypeSystem, RequestId: "system-request", ModelName: "DIAGNOSTIC_SECRET"},
		{UserId: 7, CreatedAt: 107, Type: model.LogTypeTopup, RequestId: "topup-request", ModelName: "DIAGNOSTIC_SECRET"},
		{UserId: 7, CreatedAt: 300, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "DIAGNOSTIC_SECRET", Quota: 999999},
		{UserId: 8, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "OTHER_USER_SECRET", TokenName: "OTHER_USER_SECRET", Quota: 999999},
		{UserId: 8, CreatedAt: 103, Type: model.LogTypeRefund, RequestId: "shared-request", ModelName: "OTHER_USER_SECRET", Quota: 999999},
	}
	for _, entry := range logs {
		entry.Username = "USERNAME_SECRET"
		entry.ChannelId = 987654
		entry.UpstreamRequestId = "UPSTREAM_SECRET"
		entry.Group = "GROUP_SECRET"
		entry.Content = "DIAGNOSTIC_SECRET"
	}
	require.NoError(t, logDB.Create(&logs).Error)

	gin.SetMode(gin.TestMode)
	request := map[string]any{
		"start_timestamp": 90, "end_timestamp": 200,
		"user_id": 8, "username": "OTHER_USER_SECRET", "channel": 987654,
		"channel_id": 987654, "group": "OTHER_USER_SECRET", "upstream_request_id": "UPSTREAM_SECRET",
		"type": model.LogTypeError,
	}
	files := readSelfLogExportArchive(t, selfLogExportTestRequest(t, 7, request))
	assert.Equal(t, [][]string{
		{"created_at", "request_id", "consumption_type", "token_name", "model_name", "input_tokens", "cached_input_tokens", "output_tokens", "charged_quota", "actual_amount_usd"},
		{"1970-01-01T00:01:40Z", "shared-request", "consumption", "'=token", "'=client-model", "100", "40", "20", "1000", "0.002"},
		{"1970-01-01T00:01:41Z", "shared-request", "consumption", "'=token", "'=client-model", "10", "0", "5", "250", "0.0005"},
		{"1970-01-01T00:01:42Z", "shared-request", "violation_fee", "'=token", "'=client-model", "0", "0", "0", "100", "0.0002"},
		{"1970-01-01T00:01:43Z", "shared-request", "refund", "'=token", "'=client-model", "0", "0", "0", "-200", "-0.0004"},
		{"1970-01-01T00:01:44Z", "unknown-snapshot", "consumption", "'=token", "'=client-model", "1", "0", "0", "50", ""},
	}, files["self-details.csv"])
	assert.Equal(t, [][]string{
		{"model_name", "entry_count", "consumption_count", "violation_fee_count", "refund_count", "input_tokens", "cached_input_tokens", "output_tokens", "charged_quota", "known_actual_amount_usd", "unknown_amount_count"},
		{"'=client-model", "5", "3", "1", "1", "111", "40", "25", "1200", "0.0023", "1"},
	}, files["self-summary.csv"])

	for _, view := range []service.LogExportView{service.LogExportViewSelf, service.LogExportViewDownstream} {
		var output bytes.Buffer
		var updates [][2]int64
		_, _, err := service.StreamLogExportArchive(context.Background(), &output, service.LogExportRequest{View: view, StartTimestamp: 90, EndTimestamp: 200, Fields: []string{"request_id"}}, 7, 100, func(processed, total int64) error {
			updates = append(updates, [2]int64{processed, total})
			return nil
		})
		require.NoError(t, err)
		total := int64(5)
		if view == service.LogExportViewDownstream {
			total = 6
			if logDialect == common.DatabaseTypeClickHouse {
				total = 7 // The cursor includes the later retry outside the report window.
			}
		}
		assert.Equal(t, [][2]int64{{0, total}, {total, total}}, updates)
	}

	for _, tc := range []struct {
		name    string
		filters map[string]any
		count   int
	}{
		{"shared request cannot expand to other users or dates", map[string]any{"request_id": "shared-request"}, 4},
		{"model filter", map[string]any{"model_name": "=client-model"}, 5},
		{"token filter", map[string]any{"token_name": "missing-token"}, 0},
		{"request filter", map[string]any{"request_id": "unknown-snapshot"}, 1},
		{"bounded date filter", map[string]any{"start_timestamp": 102, "end_timestamp": 103}, 2},
		{"other user model", map[string]any{"model_name": "OTHER_USER_SECRET"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filters := map[string]any{"view": "self", "start_timestamp": 90, "end_timestamp": 200}
			for key, value := range tc.filters {
				filters[key] = value
			}
			files := readSelfLogExportArchive(t, selfLogExportTestRequest(t, 7, filters))
			assert.Len(t, files["self-details.csv"], tc.count+1)
		})
	}
}

func TestExportSelfLogsRejectsUnsafeViewsFieldsAndMissingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, field := range []string{
		"user_id", "username", "token_id", "group", "status", "channel", "channel_id", "channel_name",
		"upstream_request_id", "upstream_model_name", "error_message", "content", "other", "ip",
		"model_price", "model_ratio", "completion_ratio", "cache_ratio", "group_ratio", "quota_per_unit", "original_amount_usd",
	} {
		t.Run(field, func(t *testing.T) {
			recorder := selfLogExportTestRequest(t, 7, map[string]any{"fields": []string{"request_id", field}})
			assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
			assert.Contains(t, recorder.Body.String(), `"success":false`)
			var archive bytes.Buffer
			assert.Error(t, service.WriteLogExportArchive(&archive, service.LogExportViewSelf, []string{field}, nil, nil))
			assert.Zero(t, archive.Len())
		})
	}
	for _, view := range []string{"upstream", "downstream", "unknown"} {
		t.Run(view, func(t *testing.T) {
			recorder := selfLogExportTestRequest(t, 7, map[string]any{"view": view})
			assert.Contains(t, recorder.Body.String(), `"success":false`)
			assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
		})
	}
	for _, userID := range []int{0, -1} {
		recorder := selfLogExportTestRequest(t, userID, map[string]any{"user_id": 7})
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
	}
	for _, request := range []map[string]any{
		{"fields": []string{"request_id", "request_id"}},
		{"start_timestamp": 200, "end_timestamp": 100},
	} {
		recorder := selfLogExportTestRequest(t, 7, request)
		assert.Contains(t, recorder.Body.String(), `"success":false`)
	}
}

func TestExportSelfLogsHistoricalAmountsAndSafeErrors(t *testing.T) {
	for _, prefix := range []string{"=", "+", "-", "@", "\t", "\r", "\n", "\x00"} {
		t.Run("CSV prefix "+prefix, func(t *testing.T) {
			text := prefix + "formula"
			rows := service.BuildLogExportRows(service.LogExportViewSelf, []*model.Log{{Type: model.LogTypeConsume, RequestId: text, ModelName: text, TokenName: text}})
			var archive bytes.Buffer
			require.NoError(t, service.WriteLogExportArchive(&archive, service.LogExportViewSelf, []string{"request_id", "model_name", "token_name"}, rows, service.BuildLogExportSummary(service.LogExportViewSelf, rows)))
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Type", "application/zip")
			recorder.Header().Set("Cache-Control", "private, no-store")
			_, err := recorder.Write(archive.Bytes())
			require.NoError(t, err)
			files := readSelfLogExportArchive(t, recorder)
			assert.Equal(t, []string{"'" + text, "'" + text, "'" + text}, files["self-details.csv"][1])
			assert.Equal(t, "'"+text, files["self-summary.csv"][1][0])
		})
	}
	for _, snapshot := range []string{"", `{}`, `{"quota_per_unit":0}`, `{"quota_per_unit":-1}`, `{"quota_per_unit":"invalid"}`, `{"quota_per_unit":1e309}`, `{"quota_per_unit":1e-320}`} {
		t.Run(snapshot, func(t *testing.T) {
			rows := service.BuildLogExportRows(service.LogExportViewSelf, []*model.Log{{Type: model.LogTypeConsume, Quota: 1000, Other: snapshot}})
			require.Len(t, rows, 1)
			assert.Nil(t, rows[0].ActualAmountUSD)
			summary := service.BuildLogExportSummary(service.LogExportViewSelf, rows)
			require.Len(t, summary, 1)
			assert.Equal(t, 1, summary[0].UnpricedRequestCount)
		})
	}
	db, _ := openTaskDialectDatabase(t, &model.Log{})
	previousLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previousLogDB })
	// Force a driver error with a private table name; only the safe API error may reach the user.
	model.LOG_DB = db.Table("CHANNEL_SECRET_PRIVATE_TABLE")
	recorder := selfLogExportTestRequest(t, 7, map[string]any{})
	assert.Contains(t, recorder.Body.String(), "unable to export consumption data")
	assert.NotContains(t, recorder.Body.String(), "CHANNEL_SECRET")
	assert.NotContains(t, recorder.Body.String(), "SELECT")
}

func TestExportSelfLogsDashboardAuthenticationAndAdminBoundary(t *testing.T) {
	db, dialect := openTaskDialectDatabase(t, &model.User{}, &model.UserSession{}, &model.Log{})
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousSearchLimit := common.RedisEnabled, common.SearchRateLimitEnable
	previousSecret := common.SessionSecret
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(dialect, dialect)
	common.RedisEnabled, common.SearchRateLimitEnable = false, false
	common.SessionSecret = "self-consumption-export-test-secret"
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.SearchRateLimitEnable = previousRedis, previousSearchLimit
		common.SessionSecret = previousSecret
	})
	user := &model.User{Username: "export-user", Password: "unused-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, AffCode: "export-user-aff"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&[]*model.Log{
		{UserId: user.Id, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "shared-request", Quota: 123},
		{UserId: user.Id + 1, CreatedAt: 100, Type: model.LogTypeConsume, RequestId: "shared-request", ModelName: "OTHER_USER_SECRET", Quota: 999},
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/log/self/export", middleware.UserAuth(), middleware.DisableCache(), middleware.SearchRateLimit(), ExportSelfLogs)
	router.POST("/api/log/export", middleware.AdminAuth(), ExportLogs)

	var activeToken string
	for _, tc := range []struct {
		name      string
		status    string
		expiresAt int64
		want      int
	}{
		{"active", model.UserSessionStatusActive, time.Now().Add(time.Hour).Unix(), http.StatusOK},
		{"expired", model.UserSessionStatusActive, time.Now().Add(-time.Hour).Unix(), http.StatusUnauthorized},
		{"revoked", model.UserSessionStatusRevoked, time.Now().Add(time.Hour).Unix(), http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &model.UserSession{SID: "export-session-" + tc.name, UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: tc.status, RefreshHash: strings.Repeat("x", 64), LoginMethod: "password", ExpiresAt: tc.expiresAt}
			require.NoError(t, db.Create(session).Error)
			token, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1})
			require.NoError(t, err)
			if tc.name == "active" {
				activeToken = token
			}
			request := httptest.NewRequest(http.MethodPost, "/api/log/self/export", strings.NewReader(`{"request_id":"shared-request","user_id":999}`))
			request.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			require.Equal(t, tc.want, recorder.Code, recorder.Body.String())
			if tc.want == http.StatusOK {
				files := readSelfLogExportArchive(t, recorder)
				require.Len(t, files["self-details.csv"], 2)
				assert.Equal(t, "123", files["self-details.csv"][1][8])
			} else {
				assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
			}
		})
	}
	for _, token := range []string{"", "invalid-token", activeToken[:len(activeToken)-2] + "!!"} {
		request := httptest.NewRequest(http.MethodPost, "/api/log/self/export", strings.NewReader(`{}`))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
	}
	request := httptest.NewRequest(http.MethodPost, "/api/log/export", strings.NewReader(`{"view":"upstream"}`))
	request.Header.Set("Authorization", "Bearer "+activeToken)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.NotEqual(t, "application/zip", recorder.Header().Get("Content-Type"))
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

func TestDownstreamLogExportDatabasePagination(t *testing.T) {
	db, dialect := openTaskDialectDatabase(t, &model.Log{})
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(dialect, dialect)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
	})
	// More distinct requests than one candidate page, followed by retries of
	// requests from both pages. Input order differs from request-ID ordering.
	logs := make([]model.Log, 1001)
	for i := range logs {
		requestID := fmt.Sprintf("paged-%04d", 1000-i)
		if i == 0 {
			requestID = "CasePage"
		} else if i == 1 {
			requestID = "casepage"
		}
		logs[i] = model.Log{Username: "paging-user", ModelName: "selected-model", CreatedAt: 1500, Type: model.LogTypeError, RequestId: requestID}
	}
	require.NoError(t, db.CreateInBatches(&logs, 100).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{Username: "paging-user", ModelName: "selected-model", CreatedAt: 1501, Type: model.LogTypeError, RequestId: "CasePage"},
		{Username: "other-user", ModelName: "terminal-model", CreatedAt: 1502, Type: model.LogTypeConsume, RequestId: "CasePage", PromptTokens: 17},
		{Username: "paging-user", ModelName: "selected-model", CreatedAt: 2500, Type: model.LogTypeConsume, RequestId: "casepage"},
		{Username: "paging-user", ModelName: "selected-model", CreatedAt: 1500, Type: model.LogTypeConsume, RequestId: "paged-0000", PromptTokens: 23},
		{Username: "other-user", ModelName: "terminal-model", CreatedAt: 1503, Type: model.LogTypeConsume, RequestId: "CasePage", Other: `{"violation_fee":true}`},
		{Username: "paging-user", ModelName: "selected-model", CreatedAt: 1500, Type: model.LogTypeError},
		{Username: "other-user", ModelName: "selected-model", CreatedAt: 1500, Type: model.LogTypeError},
	}).Error)
	request := service.LogExportRequest{View: service.LogExportViewDownstream, Username: "paging-%", ModelName: "selected-model", StartTimestamp: 1000, EndTimestamp: 2000, Fields: []string{"request_id", "status", "input_tokens"}}
	var output bytes.Buffer
	rows, _, err := service.StreamLogExportArchive(context.Background(), &output, request, 0, 10000, nil)
	require.NoError(t, err)
	assert.Equal(t, 1001, rows)
	archive, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	require.NoError(t, err)
	reader, err := archive.File[0].Open()
	require.NoError(t, err)
	detail, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(detail, []byte{0xEF, 0xBB, 0xBF}))).ReadAll()
	require.NoError(t, err)
	actual := make(map[string][]string, rows)
	for _, row := range records[1:] {
		require.NotContains(t, actual, row[0], "a retry in a later page must not duplicate a request")
		actual[row[0]] = row
	}
	assert.Equal(t, []string{"CasePage", "success", "17"}, actual["CasePage"])
	assert.Equal(t, []string{"paged-0000", "success", "23"}, actual["paged-0000"])
	assert.Equal(t, []string{"", "failed", "0"}, actual[""])
	assert.NotContains(t, actual, "casepage", "the real terminal record lies outside the report window")
	// Expanded histories also count toward the safety limit, including retries
	// outside the report filters. The archive must fail rather than truncate.
	output.Reset()
	_, _, err = service.StreamLogExportArchive(context.Background(), &output, request, 0, 1004, nil)
	var limitError *service.LogExportLimitError
	require.ErrorAs(t, err, &limitError)
	assert.Equal(t, 1004, limitError.Limit)
	// Callbacks may use the same one-connection database; rows appended while
	// exporting must remain outside the captured ID snapshot.
	var appended model.Log
	var visited []int
	var progressUpdates [][2]int64
	_, err = model.VisitLogsForExport(context.Background(), model.LogExportQuery{Username: "paging-%", StartTimestamp: 1000, EndTimestamp: 2000}, 0, true, 10000, func(batch []*model.Log) error {
		if appended.Id == 0 {
			appended = model.Log{Username: "paging-user", CreatedAt: 1504, Type: model.LogTypeConsume, RequestId: "CasePage"}
			if err := db.Create(&appended).Error; err != nil {
				return err
			}
		}
		for _, item := range batch {
			visited = append(visited, item.Id)
		}
		return db.Exec("SELECT 1").Error
	}, func(processed, total int64) error {
		progressUpdates = append(progressUpdates, [2]int64{processed, total})
		return db.Exec("SELECT 1").Error
	})
	require.NoError(t, err)
	require.Positive(t, appended.Id)
	assert.NotContains(t, visited, appended.Id)
	assert.Equal(t, [][2]int64{{0, 1004}, {1000, 1004}, {1004, 1004}}, progressUpdates, "progress counts matching candidates, not expanded or duplicate histories")
}
