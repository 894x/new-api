package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay"
	jspluginadaptor "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type taskSubmissionTestBilling struct {
	events     *[]string
	settleErr  error
	reserveErr error
	onSettle   func()
	refunds    int
}

func (b *taskSubmissionTestBilling) Settle(int) error {
	*b.events = append(*b.events, "settle")
	if b.onSettle != nil {
		b.onSettle()
	}
	return b.settleErr
}

func (b *taskSubmissionTestBilling) Refund(*gin.Context) {
	*b.events = append(*b.events, "refund")
	b.refunds++
}

func (b *taskSubmissionTestBilling) NeedsRefund() bool        { return b.refunds == 0 }
func (b *taskSubmissionTestBilling) GetPreConsumedQuota() int { return 0 }
func (b *taskSubmissionTestBilling) Reserve(int) error {
	*b.events = append(*b.events, "reserve")
	return b.reserveErr
}

func (b *taskSubmissionTestBilling) ReserveForAdmission(quota int) error {
	return b.Reserve(quota)
}

func TestPresentTaskSubmissionUsesNativePresenterAfterPersistence(t *testing.T) {
	plugin, err := pluginruntime.CompilePlugin(`
export const meta = {apiVersion:1,key:"presenter-test",name:"Presenter",version:"1.0.0",author:{name:"Test"},models:["model"],fetchMode:"per_task",routes:[{method:"POST",path:"/vendor/jobs",type:"submit",decode:"decode",render:"created"}]};
export const native = {decode:function(ctx){return {kind:"submit",model:"model",requestBody:ctx.body.value};},created:function(ctx,task){return {data:{task_id:task.task_id},upstream:task.data};}};
export function buildSubmitRequest(){return {}} export function parseSubmitResponse(){return {taskId:"upstream"}} export function buildQueryRequest(){return {}} export function parseTaskResult(){return {status:"SUCCESS"}}
`, pluginruntime.Options{})
	require.NoError(t, err)
	priceData := types.PriceData{}
	priceData.AddOtherRatio("seconds", 5)
	task := &model.Task{TaskID: "task_public", SubmitTime: 123}
	task.SetData(map[string]any{"task_id": "upstream_private"})
	outcome := &taskSubmissionOutcome{
		Result:    &relay.TaskSubmitResult{},
		Task:      task,
		RelayInfo: &relaycommon.RelayInfo{PriceData: priceData},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/vendor/jobs", strings.NewReader(`{"model":"model"}`))
	c.Set(pluginruntime.ContextKeyPinnedRoute, pluginruntime.PinnedRoute{Plugin: plugin, Route: plugin.Meta.Routes[0]})
	c.Set(pluginruntime.ContextKeyRouteRequest, pluginruntime.RouteRequestContext{Path: "/vendor/jobs", Method: http.MethodPost, Body: map[string]any{"kind": "json", "value": map[string]any{"model": "model"}}})

	presentTaskSubmission(c, outcome)

	assert.JSONEq(t, `{
		"data":{"task_id":"task_public"},
		"upstream":{"task_id":"upstream_private"}
	}`, recorder.Body.String())
	assert.JSONEq(t, `{"seconds":5}`, recorder.Header().Get("X-New-Api-Other-Ratios"))
}

func TestPresentTaskSubmissionAddsWanTaskIDForWan3(t *testing.T) {
	plugin, err := pluginruntime.CompilePlugin(`
export const meta = {apiVersion:1,key:"moyu-wan3",name:"Moyu Wan3",version:"1.0.0",author:{name:"Test"},models:["wan3.0-video"],fetchMode:"per_task",routes:[{method:"POST",path:"/moyu/jobs",type:"submit",decode:"decode",render:"created"}]};
export const native = {decode:function(ctx){return {kind:"submit",model:"wan3.0-video",requestBody:ctx.body.value};},created:function(_ctx,task){return {task_id:task.task_id};}};
export function buildSubmitRequest(){return {}} export function parseSubmitResponse(){return {taskId:"upstream"}} export function buildQueryRequest(){return {}} export function parseTaskResult(){return {status:"SUCCESS"}}
`, pluginruntime.Options{})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/moyu/jobs", strings.NewReader(`{"model":"wan3.0-video"}`))
	c.Set(pluginruntime.ContextKeyPinnedRoute, pluginruntime.PinnedRoute{Plugin: plugin, Route: plugin.Meta.Routes[0]})
	c.Set(pluginruntime.ContextKeyRouteRequest, pluginruntime.RouteRequestContext{Path: "/moyu/jobs", Method: http.MethodPost, Body: map[string]any{"kind": "json", "value": map[string]any{"model": "wan3.0-video"}}})

	presentTaskSubmission(c, &taskSubmissionOutcome{
		Result:    &relay.TaskSubmitResult{},
		Task:      &model.Task{TaskID: "task_public", PrivateData: model.TaskPrivateData{UpstreamTaskID: "wan-provider-task"}},
		RelayInfo: &relaycommon.RelayInfo{},
	})

	assert.JSONEq(t, `{"task_id":"task_public","wan_task_id":"wan-provider-task"}`, recorder.Body.String())
}

func TestPresentTaskSubmissionFallbackUsesPersistedPublicID(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	outcome := &taskSubmissionOutcome{
		Result:    &relay.TaskSubmitResult{},
		Task:      &model.Task{TaskID: "task_persisted", SubmitTime: 456},
		RelayInfo: &relaycommon.RelayInfo{OriginModelName: "video-model"},
	}

	presentTaskSubmission(c, outcome)

	assert.JSONEq(t, `{
		"id":"task_persisted",
		"task_id":"task_persisted",
		"status":"queued",
		"model":"video-model",
		"created_at":456
	}`, recorder.Body.String())
}

func TestPresentTaskSubmissionUsesHostOpenAIVideoCreateReceipt(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(pluginruntime.ContextKeyPinnedEndpoint, pluginruntime.PinnedEndpoint{
		Protocol:  "openai_video",
		Operation: pluginruntime.HostProtocolOperation{Name: "create"},
	})
	task := &model.Task{
		TaskID:     "task_public",
		Status:     model.TaskStatusSubmitted,
		Progress:   "0%",
		CreatedAt:  456,
		Properties: model.Properties{OriginModelName: "video-model"},
	}
	outcome := &taskSubmissionOutcome{Result: &relay.TaskSubmitResult{}, Task: task, RelayInfo: &relaycommon.RelayInfo{}}

	presentTaskSubmission(c, outcome)

	assert.JSONEq(t, `{"id":"task_public","object":"video","model":"video-model","status":"queued","progress":0,"created_at":456}`, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "task_id")
}

func TestExecuteTaskSubmissionRefundsWhenInsertFails(t *testing.T) {
	events := make([]string, 0, 3)
	database := setupTaskSubmissionDatabase(t, false, &events)
	_ = database
	billing := &taskSubmissionTestBilling{events: &events}
	c := taskSubmissionTestContext()
	info := taskSubmissionRelayInfo(billing)

	outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
		return &relay.TaskSubmitResult{
			UpstreamTaskID: "upstream_private",
			Platform:       constant.TaskPlatform("plugin"),
		}, nil
	})

	assert.Nil(t, outcome)
	require.NotNil(t, taskErr)
	assert.Equal(t, "task_insert_failed", taskErr.Code)
	assert.Equal(t, []string{"reserve", "insert", "refund"}, events)
	assert.Equal(t, 1, billing.refunds)
	assert.False(t, c.Writer.Written())
}

func TestExecuteTaskSubmissionSettlementFailureStaysDurableAndWritesNothing(t *testing.T) {
	events := make([]string, 0, 3)
	database := setupTaskSubmissionDatabase(t, true, &events)
	billing := &taskSubmissionTestBilling{events: &events, settleErr: errors.New("settlement failed")}
	c := taskSubmissionTestContext()
	info := taskSubmissionRelayInfo(billing)

	outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
		return &relay.TaskSubmitResult{
			UpstreamTaskID: "upstream_private",
			Platform:       constant.TaskPlatform("plugin"),
		}, nil
	})

	assert.Nil(t, outcome)
	require.NotNil(t, taskErr)
	assert.Equal(t, "task_billing_settlement_failed", taskErr.Code)
	assert.Equal(t, []string{"reserve", "insert", "settle"}, events)
	assert.Zero(t, billing.refunds)
	var count int64
	require.NoError(t, database.Model(&model.Task{}).Where("task_id = ?", "task_public").Count(&count).Error)
	assert.Equal(t, int64(1), count)
	assert.False(t, c.Writer.Written())
}

func TestExecuteTaskSubmissionPersistsPinnedPluginProvenance(t *testing.T) {
	events := make([]string, 0, 3)
	database := setupTaskSubmissionDatabase(t, true, &events)
	previousLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = previousLogConsumeEnabled })

	c := taskSubmissionTestContext()
	c.Set(common.RequestIdKey, "request-public")
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{
		Generation: &pluginruntime.RoutingGeneration{Number: 42},
		Plugin: &pluginruntime.LoadedPlugin{Meta: pluginruntime.Meta{
			Key:        "document-parser",
			Name:       "Document Parser",
			Version:    "1.2.3",
			APIVersion: 1,
			Author: pluginruntime.AuthorMeta{
				Name: "Community Author",
				URL:  "https://plugins.example/author",
			},
		}},
	})
	billing := &taskSubmissionTestBilling{events: &events}
	info := taskSubmissionRelayInfo(billing)

	outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
		return &relay.TaskSubmitResult{
			UpstreamTaskID: "upstream-private",
			Platform:       constant.TaskPlatform("document-parser"),
		}, nil
	})

	require.Nil(t, taskErr)
	require.NotNil(t, outcome)
	require.NotNil(t, outcome.Task.PrivateData.Execution)
	require.NotNil(t, outcome.Task.PrivateData.Execution.TaskPlugin)
	assert.Equal(t, "request-public", outcome.Task.PrivateData.Execution.RequestID)
	assert.Equal(t, "/plugin/submit", outcome.Task.PrivateData.Execution.RequestPath)
	assert.Equal(t, "1.2.3", outcome.Task.PrivateData.Execution.TaskPlugin.Version)
	assert.Equal(t, uint64(42), outcome.Task.PrivateData.Execution.TaskPlugin.Generation)
	require.NotNil(t, outcome.Task.PrivateData.Execution.TaskPlugin.Author)
	assert.Equal(t, "Community Author", outcome.Task.PrivateData.Execution.TaskPlugin.Author.Name)
	assert.Equal(t, "https://plugins.example/author", outcome.Task.PrivateData.Execution.TaskPlugin.Author.URL)

	var stored model.Task
	require.NoError(t, database.Where("task_id = ?", "task_public").First(&stored).Error)
	require.NotNil(t, stored.PrivateData.Execution)
	require.NotNil(t, stored.PrivateData.Execution.TaskPlugin)
	assert.Equal(t, "document-parser", stored.PrivateData.Execution.TaskPlugin.Key)
	require.NotNil(t, stored.PrivateData.Execution.TaskPlugin.Author)
	assert.Equal(t, "Community Author", stored.PrivateData.Execution.TaskPlugin.Author.Name)
	assert.Equal(t, "upstream-private", stored.PrivateData.UpstreamTaskID)
}

// A submit route declaring retainResult: false persists the task row for
// billing but never writes the upstream snapshot for an immediate terminal
// result, while the in-memory task still carries it for the presenter. An
// asynchronous result on the same route is retained because polling and
// retrieval depend on it.
func TestExecuteTaskSubmissionHonorsRouteRetainResult(t *testing.T) {
	retainFalse := false
	for _, tc := range []struct {
		name          string
		immediate     *relaycommon.TaskInfo
		wantDiscarded bool
	}{
		{"immediate success is discarded", &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Progress: "100%"}, true},
		{"immediate failure is discarded", &relaycommon.TaskInfo{Status: model.TaskStatusFailure, Reason: "rejected"}, true},
		{"asynchronous result is retained", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := make([]string, 0, 3)
			database, _ := openTaskDialectDatabase(t, &model.Task{}, &model.User{}, &model.Channel{})
			previousDB := model.DB
			model.DB = database
			t.Cleanup(func() { model.DB = previousDB })
			previousLogConsumeEnabled := common.LogConsumeEnabled
			common.LogConsumeEnabled = false
			t.Cleanup(func() { common.LogConsumeEnabled = previousLogConsumeEnabled })

			c := taskSubmissionTestContext()
			c.Set(pluginruntime.ContextKeyPinnedRoute, pluginruntime.PinnedRoute{
				Plugin: &pluginruntime.LoadedPlugin{Meta: pluginruntime.Meta{Key: "sync-images"}},
				Route:  pluginruntime.Route{Method: http.MethodPost, Path: "/sync/images", Type: pluginruntime.RouteTypeSubmit, RetainResult: &retainFalse},
			})
			info := taskSubmissionRelayInfo(&taskSubmissionTestBilling{events: &events})
			upstream := []byte(`{"data":[{"url":"https://cdn.example/a.png"}]}`)

			outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
				return &relay.TaskSubmitResult{
					UpstreamTaskID: "task_public",
					Platform:       constant.TaskPlatform("sync-images"),
					TaskData:       upstream,
					Immediate:      tc.immediate,
				}, nil
			})
			require.Nil(t, taskErr)
			require.NotNil(t, outcome)
			assert.JSONEq(t, string(upstream), string(outcome.Task.Data), "presenter keeps the in-memory snapshot")
			assert.Equal(t, tc.wantDiscarded, outcome.Task.PrivateData.ResultDiscarded)
			assert.Equal(t, !tc.wantDiscarded, outcome.Task.ResultRetrievable())

			var stored model.Task
			require.NoError(t, database.Where("task_id = ?", "task_public").First(&stored).Error)
			assert.Equal(t, tc.wantDiscarded, stored.PrivateData.ResultDiscarded)
			var nullCount int64
			require.NoError(t, database.Model(&model.Task{}).Where("task_id = ? AND data IS NULL", "task_public").Count(&nullCount).Error)
			if tc.wantDiscarded {
				assert.Equal(t, int64(1), nullCount, "snapshot column must stay NULL")
				artifacts, err := projectTaskArtifacts(&stored)
				require.NoError(t, err)
				assert.Empty(t, artifacts)
			} else {
				assert.Equal(t, int64(0), nullCount)
				assert.JSONEq(t, string(upstream), string(stored.Data))
			}
			listed := model.TaskGetAllUserTask(1, 0, 10, model.SyncTaskQueryParams{})
			require.Len(t, listed, 1)
			assert.Empty(t, listed[0].Data, "task lists never select the snapshot column")
		})
	}
}

func TestExecuteTaskSubmissionRefundsCancellationBeforeDurableBarrier(t *testing.T) {
	events := make([]string, 0, 2)
	setupTaskSubmissionDatabase(t, true, &events)
	billing := &taskSubmissionTestBilling{events: &events}
	c := taskSubmissionTestContext()
	requestContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestContext)
	info := taskSubmissionRelayInfo(billing)

	outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
		cancel()
		return &relay.TaskSubmitResult{
			UpstreamTaskID: "upstream_private",
			Platform:       constant.TaskPlatform("plugin"),
		}, nil
	})

	assert.Nil(t, outcome)
	require.NotNil(t, taskErr)
	assert.Equal(t, "request_cancelled", taskErr.Code)
	assert.Equal(t, []string{"refund"}, events)
	assert.Equal(t, 1, billing.refunds)
	assert.False(t, c.Writer.Written())
}

func TestExecuteTaskSubmissionDisconnectBeforeUpstreamAcceptanceSkipsSubmitAndRefunds(t *testing.T) {
	events := make([]string, 0, 1)
	setupTaskSubmissionDatabase(t, true, &events)
	billing := &taskSubmissionTestBilling{events: &events}
	c := taskSubmissionTestContext()
	requestContext, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(requestContext)
	info := taskSubmissionRelayInfo(billing)
	submitted := false

	outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
		submitted = true
		return nil, nil
	})

	assert.Nil(t, outcome)
	require.NotNil(t, taskErr)
	assert.Equal(t, "request_cancelled", taskErr.Code)
	assert.False(t, submitted)
	assert.Equal(t, []string{"refund"}, events)
	assert.Equal(t, 1, billing.refunds)
	assert.False(t, c.Writer.Written())
}

func TestExecuteTaskSubmissionCallerCancellationDuringSubmitRefundsBeforeDurableBarrier(t *testing.T) {
	events := make([]string, 0, 1)
	setupTaskSubmissionDatabase(t, true, &events)
	billing := &taskSubmissionTestBilling{events: &events}
	c := taskSubmissionTestContext()
	requestContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestContext)
	info := taskSubmissionRelayInfo(billing)
	submitStarted := make(chan struct{})
	done := make(chan struct{})
	var outcome *taskSubmissionOutcome
	var taskErr *dto.TaskError

	go func() {
		defer close(done)
		outcome, taskErr = executeTaskSubmissionWith(c, info, func(c *gin.Context, _ *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
			close(submitStarted)
			<-c.Request.Context().Done()
			return nil, service.TaskErrorWrapperLocal(c.Request.Context().Err(), "do_request_failed", http.StatusInternalServerError)
		})
	}()
	select {
	case <-submitStarted:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "submission did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "submission did not stop after disconnect")
	}

	assert.Nil(t, outcome)
	require.NotNil(t, taskErr)
	assert.Equal(t, "request_cancelled", taskErr.Code)
	assert.Equal(t, []string{"refund"}, events)
	assert.Equal(t, 1, billing.refunds)
	assert.False(t, c.Writer.Written())
}

func TestExecuteTaskSubmissionDisconnectAfterDurableInsertDoesNotRefund(t *testing.T) {
	events := make([]string, 0, 3)
	database := setupTaskSubmissionDatabase(t, true, &events)
	previousLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = previousLogConsumeEnabled })
	c := taskSubmissionTestContext()
	requestContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestContext)
	billing := &taskSubmissionTestBilling{
		events:   &events,
		onSettle: cancel,
	}
	info := taskSubmissionRelayInfo(billing)

	outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
		return &relay.TaskSubmitResult{
			UpstreamTaskID: "upstream_private",
			Platform:       constant.TaskPlatform("plugin"),
		}, nil
	})

	require.Nil(t, taskErr)
	require.NotNil(t, outcome)
	assert.Equal(t, "task_public", outcome.Task.TaskID)
	assert.Equal(t, []string{"reserve", "insert", "settle"}, events)
	assert.Zero(t, billing.refunds)
	var count int64
	require.NoError(t, database.Model(&model.Task{}).Where("task_id = ?", "task_public").Count(&count).Error)
	assert.Equal(t, int64(1), count)
	assert.False(t, c.Writer.Written())
}

// setupTaskSubmissionDatabase opens the dialect selected by
// TEST_TASK_DB_DIALECT (SQLite in memory by default) and records every task
// INSERT in events so tests can assert the reserve → insert → settle order.
// Without migrate the task table does not exist and inserts fail.
func setupTaskSubmissionDatabase(t *testing.T, migrate bool, events *[]string) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	var models []any
	if migrate {
		models = append(models, &model.Task{})
	}
	database, _ := openTaskDialectDatabase(t, models...)
	require.NoError(t, database.Callback().Create().Before("gorm:create").Register("test:task-submit-order", func(*gorm.DB) {
		*events = append(*events, "insert")
	}))
	model.DB = database
	t.Cleanup(func() { model.DB = previousDB })
	return database
}

func taskSubmissionTestContext() *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/plugin/submit", strings.NewReader(`{}`))
	return c
}

func taskSubmissionRelayInfo(billing relaycommon.BillingSettler) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		UserId:          1,
		UsingGroup:      "default",
		OriginModelName: "plugin-model",
		Billing:         billing,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID:  "task_public",
			LockedChannel: &model.Channel{Id: 1, Type: constant.ChannelTypeTaskPlugin, Name: "plugin"},
		},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1, ChannelType: constant.ChannelTypeTaskPlugin},
	}
}

// Uses the real submit adaptor, expression evaluator, BillingSession, task row
// and consume log. Set TEST_TASK_DB_DIALECT plus TEST_MYSQL_DSN or
// TEST_POSTGRES_DSN to exercise the same contract on an external test database.
// Unique table prefixes keep the fixture isolated from all existing tables.
// openTaskDialectDatabase opens the engine selected by TEST_TASK_DB_DIALECT
// (default SQLite in memory; MySQL and PostgreSQL through TEST_MYSQL_DSN and
// TEST_POSTGRES_DSN) with a unique table prefix, migrates the given models and
// drops them on cleanup. It logs the engine version so database verification
// runs leave a record.
func openTaskDialectDatabase(t *testing.T, models ...any) (*gorm.DB, common.DatabaseType) {
	t.Helper()
	dialect := common.DatabaseType(os.Getenv("TEST_TASK_DB_DIALECT"))
	var driver gorm.Dialector
	switch dialect {
	case "", common.DatabaseTypeSQLite:
		dialect = common.DatabaseTypeSQLite
		driver = sqlite.Open(":memory:")
	case common.DatabaseTypeMySQL:
		require.NotEmpty(t, os.Getenv("TEST_MYSQL_DSN"))
		driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
	case common.DatabaseTypePostgreSQL:
		require.NotEmpty(t, os.Getenv("TEST_POSTGRES_DSN"))
		driver = postgres.New(postgres.Config{DSN: os.Getenv("TEST_POSTGRES_DSN"), PreferSimpleProtocol: true})
	default:
		t.Fatalf("unsupported test dialect %q", dialect)
	}
	db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("tsubmit_%d_", time.Now().UnixNano())}})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(models...))
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(models...)) })
	var version string
	if dialect == common.DatabaseTypeSQLite {
		require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
	} else {
		require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
	}
	t.Logf("database: %s %s", dialect, version)
	return db, dialect
}

func TestImmediateTaskSettlementDatabase(t *testing.T) {
	db, dialect := openTaskDialectDatabase(t, &model.User{}, &model.Channel{}, &model.Task{}, &model.Log{}, &model.BillingAdmissionReserveOperation{}, &model.BillingRefundOperation{})
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldMemory, oldBatch, oldConsume, oldExport := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(dialect, dialect)
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled = false, false, false, true, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, common.DataExportEnabled = oldRedis, oldMemory, oldBatch, oldConsume, oldExport
	})

	const expression = `u("units") == 7 ? tier("missing", u("missing") * 1.0) : u("units") == 8 ? tier("invalid", -1.0) : u("units") > 3 ? tier("bulk", u("units") * 0.01) : tier("small", u("units") * 0.01)`
	withTieredBillingConfig(t, map[string]string{"document-model": "tiered_expr"}, map[string]string{"document-model": expression})
	const source = `
export const meta={apiVersion:1,key:"generic-settlement",name:"Generic settlement",version:"1.0.0",author:{name:"Test"},models:["document-model"],fetchMode:"per_task",usageSchema:{units:{type:"number",unit:"count"}}};
export function buildSubmitRequest(ctx){return {url:ctx.baseUrl+"/compile",body:ctx.requestBody};}
export function parseSubmitResponse(ctx,resp){return {taskId:"vendor-job",taskData:resp.body,immediate:{status:resp.body.status,reason:"provider rejected job"}};}
export function extractUsage(){return {units:4};}
export function extractUsageOnComplete(ctx,result,body){return body.usage;}
export function parseTaskResult(){throw new Error("completed submissions must not poll");}
export function buildQueryRequest(){throw new Error("completed submissions must not poll");}
`
	plugin, err := pluginruntime.CompilePlugin(source, pluginruntime.Options{})
	require.NoError(t, err)
	for index, tc := range []struct {
		name, status string
		actual       any
		count        float64
	}{
		{"partial", "SUCCESS", 2, 2}, {"zero", "SUCCESS", 0, 0}, {"larger", "SUCCESS", 6, 6},
		{"invalid usage", "SUCCESS", -1, 4}, {"expression failure", "SUCCESS", 7, 4}, {"negative result", "SUCCESS", 8, 4}, {"missing usage", "SUCCESS", nil, 4}, {"failed", "FAILURE", 9, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				body := map[string]any{"status": tc.status}
				if tc.actual != nil {
					body["usage"] = map[string]any{"units": tc.actual}
				}
				encoded, err := common.Marshal(body)
				if err != nil {
					panic(err)
				}
				_, _ = w.Write(encoded)
			}))
			defer server.Close()
			initial := int(20 * common.QuotaPerUnit)
			user := model.User{Username: fmt.Sprintf("task_user_%d", index), AffCode: fmt.Sprintf("task_aff_%d", index), Quota: initial}
			require.NoError(t, db.Create(&user).Error)
			ch := model.Channel{Name: "test provider", Type: constant.ChannelTypeTaskPlugin}
			require.NoError(t, db.Create(&ch).Error)
			c := taskSubmissionTestContext()
			c.Set("group", "default")
			c.Set("username", user.Username)
			c.Set("task_request", map[string]any{"model": "document-model"})
			c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "document-model")
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(c, constant.ContextKeyChannelId, ch.Id)
			common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
			info := taskSubmissionRelayInfo(nil)
			info.UserId = user.Id
			info.OriginModelName = "document-model"
			info.UserGroup = "default"
			info.UsingGroup = "default"
			info.IsPlayground = true
			info.UserSetting.BillingPreference = "wallet_only"
			info.PublicTaskID = model.GenerateTaskID()
			info.LockedChannel = &ch
			outcome, taskErr := executeTaskSubmission(c, info)
			require.Nil(t, taskErr)
			require.NotNil(t, outcome)
			want := common.QuotaRound(tc.count * 0.01 * common.QuotaPerUnit)
			assert.Equal(t, want, outcome.Result.Quota)
			assert.Equal(t, want, info.PriceData.Quota)
			var stored model.Task
			require.NoError(t, db.Where("task_id = ?", info.PublicTaskID).First(&stored).Error)
			assert.Equal(t, want, stored.Quota)
			assert.Equal(t, model.TaskStatus(tc.status), stored.Status)
			assert.Positive(t, stored.FinishTime)
			assert.Equal(t, float64(4), info.TieredBillingSnapshot.EstimatedQuotaBeforeGroup/(0.01*common.QuotaPerUnit))
			if tc.status == "SUCCESS" {
				assert.Equal(t, tc.count, stored.PrivateData.BillingContext.TieredSnapshot.UsageFacts["units"])
			}
			var updated model.User
			require.NoError(t, db.First(&updated, user.Id).Error)
			assert.Equal(t, initial-want, updated.Quota)
			assert.Equal(t, want, updated.UsedQuota)
			var logs []model.Log
			require.NoError(t, db.Where("user_id = ?", user.Id).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, want, logs[0].Quota)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			if tc.status == "SUCCESS" {
				assert.Equal(t, tc.count, other["usage_facts"].(map[string]any)["units"])
			}
			assert.False(t, c.Writer.Written(), "presentation must follow persistence and settlement")
			require.NoError(t, info.Billing.Settle(want))
			info.Billing.Refund(c)
			require.NoError(t, db.First(&updated, user.Id).Error)
			assert.Equal(t, initial-want, updated.Quota, "terminal settlement is idempotent")
		})
	}
}

func TestAcceptedSubmitStreamNeverRetries(t *testing.T) {
	c := taskSubmissionTestContext()
	assert.Equal(t, service.PolicyDecision{Action: "stop", Reason: "task_accepted", Source: "system"}, decideTaskRetry(c, &dto.TaskError{StatusCode: 502, LocalError: true, NoRetry: true}, 3))
}

// Local task rejections carry a message but no cause; the response and the
// decision record must still be produced.
func TestRespondTaskSubmissionErrorWithoutCause(t *testing.T) {
	previousErrorLog := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false
	t.Cleanup(func() { constant.ErrorLogEnabled = previousErrorLog })
	c := taskSubmissionTestContext()
	taskErr := &dto.TaskError{Code: "get_channel_failed", Message: "no channel", StatusCode: http.StatusServiceUnavailable, LocalError: true}
	require.NotPanics(t, func() { respondTaskSubmissionError(c, taskErr) })
	assert.Equal(t, http.StatusServiceUnavailable, c.Writer.Status())
	events := service.RequestPolicy(c).Events()
	require.Len(t, events, 1)
	assert.Equal(t, service.PolicyDecision{Action: "stop", Reason: "request_failed", Source: "system"}, events[0].Decision)
	assert.Equal(t, http.StatusServiceUnavailable, events[0].Status)
	assert.Equal(t, service.PolicyDecision{Action: "stop", Reason: "local_rejection", Source: "system"}, decideTaskRetry(c, &dto.TaskError{StatusCode: http.StatusForbidden, LocalError: true, Message: "billing"}, 2), "local errors stop once the status rules do not force a retry")
}

func TestExecuteTaskSubmissionRefundsWhenFinalReserveFails(t *testing.T) {
	events := []string{}
	setupTaskSubmissionDatabase(t, true, &events)
	billing := &taskSubmissionTestBilling{events: &events, reserveErr: errors.New("insufficient funds")}
	c := taskSubmissionTestContext()
	info := taskSubmissionRelayInfo(billing)
	outcome, taskErr := executeTaskSubmissionWith(c, info, func(*gin.Context, *relaycommon.RelayInfo) (*relay.TaskSubmitResult, *dto.TaskError) {
		return &relay.TaskSubmitResult{Platform: "plugin", Quota: 600, Immediate: &relaycommon.TaskInfo{Status: "SUCCESS"}}, nil
	})
	require.Nil(t, outcome)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusForbidden, taskErr.StatusCode)
	assert.Equal(t, []string{"reserve", "refund"}, events)
	assert.Equal(t, 1, billing.refunds)
	assert.False(t, c.Writer.Written())
}

// Exercises the real SLS admission, asset import, background dispatch, public
// presenters and refund chain on every dialect supported by this fixture.
func TestSLSDeferredSubmissionDatabase(t *testing.T) {
	db, dialect := openTaskDialectDatabase(t, &model.User{}, &model.Channel{}, &model.Task{}, &model.Log{},
		&model.BillingAdmissionReserveOperation{}, &model.BillingRefundOperation{}, &model.AuditLog{},
		&model.ChannelAssetConfig{}, &model.UserAssetGroup{}, &model.UserAsset{},
		&model.UserAssetGroupReplica{}, &model.UserAssetReplica{})
	previousDB, previousLog := model.DB, model.LOG_DB
	previousMain, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemory, previousBatch := common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled
	previousConsume, previousExport := common.LogConsumeEnabled, common.DataExportEnabled
	previousFactory := service.GetTaskAdaptorFunc
	previousQueryLimit, previousTimeout, previousDownloadLimit := constant.TaskQueryLimit, constant.TaskTimeoutMinutes, constant.MaxFileDownloadMB
	previousFetch := *system_setting.GetFetchSetting()
	previousAddress := system_setting.TaskPublicAddress
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:isolated-audit-table", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.Statement.Table = tx.NamingStrategy.TableName("audit_logs")
			tx.Statement.TableExpr = nil
		}
	}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(dialect, dialect)
	common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = false, false, false
	common.LogConsumeEnabled, common.DataExportEnabled = true, false
	constant.TaskQueryLimit, constant.TaskTimeoutMinutes = 100, 0
	constant.MaxFileDownloadMB = 64
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	service.InitHttpClient()
	t.Setenv("ASSET_STORAGE_ENABLED", "false")
	t.Setenv("SEEDANCE_VIDEO_DIR", t.TempDir())
	system_setting.TaskPublicAddress = "http://media.example"
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLog
		common.SetDatabaseTypes(previousMain, previousLogType)
		common.RedisEnabled, common.MemoryCacheEnabled, common.BatchUpdateEnabled = previousRedis, previousMemory, previousBatch
		common.LogConsumeEnabled, common.DataExportEnabled = previousConsume, previousExport
		service.GetTaskAdaptorFunc = previousFactory
		constant.TaskQueryLimit, constant.TaskTimeoutMinutes = previousQueryLimit, previousTimeout
		constant.MaxFileDownloadMB = previousDownloadLimit
		*system_setting.GetFetchSetting() = previousFetch
		system_setting.TaskPublicAddress = previousAddress
		service.InitHttpClient()
	})
	const modelName = "doubao-seedance-2-0-260128"
	withTieredBillingConfig(t, map[string]string{modelName: "tiered_expr"}, map[string]string{modelName: `u("tokens") * 0.000001`})
	source, err := os.ReadFile("../plugins/tasks/seedance-sls/plugin.js")
	require.NoError(t, err)
	plugin, err := pluginruntime.CompilePlugin(string(source), pluginruntime.Options{})
	require.NoError(t, err)
	service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { return jspluginadaptor.New(plugin) }
	legacy := model.Task{TaskID: "legacy-sls-task", Status: model.TaskStatusSuccess, Progress: "100%"}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.Model(&legacy).Update("private_data", `{"upstream_task_id":"legacy-vendor","key":"legacy-key"}`).Error)
	require.NoError(t, db.First(&legacy, legacy.ID).Error)
	assert.Nil(t, legacy.PrivateData.PendingSubmission)
	assert.Equal(t, "legacy-vendor", legacy.PrivateData.UpstreamTaskID)
	_, claimed, err := model.ClaimTaskSubmission(t.Context(), legacy.ID, time.Now().Unix())
	require.NoError(t, err)
	assert.False(t, claimed, "historical rows do not enter the new submission queue")
	t.Run("lease ownership", func(t *testing.T) {
		ready := false
		task := model.Task{TaskID: "unfunded-sls-task", Status: model.TaskStatusNotStart, BillingReady: &ready,
			PrivateData: model.TaskPrivateData{PendingSubmission: &model.TaskPendingSubmission{Body: []byte(`{}`)}}}
		require.NoError(t, db.Create(&task).Error)
		_, claimed, err := model.ClaimTaskSubmission(t.Context(), task.ID, time.Now().Unix())
		require.NoError(t, err)
		assert.False(t, claimed, "an unconfirmed charge cannot reach the provider")
		require.NoError(t, db.Model(&task).Update("billing_ready", true).Error)
		first, claimed, err := model.ClaimTaskSubmission(t.Context(), task.ID, time.Now().Unix())
		require.NoError(t, err)
		require.True(t, claimed)
		leaseID := first.PrivateData.PendingSubmission.LeaseID
		second, claimed, err := model.ClaimTaskSubmission(t.Context(), task.ID, first.PrivateData.PendingSubmission.LeaseUntil)
		require.NoError(t, err)
		require.True(t, claimed)
		assert.NotEqual(t, leaseID, second.PrivateData.PendingSubmission.LeaseID)
		first.Status = model.TaskStatusSubmitted
		won, err := model.UpdateClaimedTaskSubmission(t.Context(), first, leaseID)
		require.NoError(t, err)
		assert.False(t, won, "a stale executor cannot overwrite a replacement")
		second.Status = model.TaskStatusSuccess
		second.Progress = "100%"
		secondLeaseID := second.PrivateData.PendingSubmission.LeaseID
		second.PrivateData.PendingSubmission = nil
		won, err = model.UpdateClaimedTaskSubmission(t.Context(), second, secondLeaseID)
		require.NoError(t, err)
		assert.True(t, won)
	})
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 512, 512))))
	for index, mode := range []string{"disconnect", "asset failure", "invalid media", "media conversion", "restart during assets", "unknown submit", "concurrent poll", "disabled channel", "deleted channel", "upstream rejection", "transport failure", "timeout during assets", "timeout during upstream"} {
		t.Run(mode, func(t *testing.T) {
			t.Cleanup(func() { constant.TaskTimeoutMinutes = 0 })
			var downloads, uploads, submissions, queries atomic.Int32
			assetEntered, assetContinue := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v1/seedance-media/") {
					file, contentType, err := service.OpenSeedanceMedia(strings.TrimPrefix(r.URL.Path, "/v1/seedance-media/"))
					if !assert.NoError(t, err) {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					defer file.Close()
					w.Header().Set("Content-Type", contentType)
					_, _ = io.Copy(w, file)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /reference.png":
					downloads.Add(1)
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write(pngBytes.Bytes())
				case "POST /v1/volcengine/assets":
					uploads.Add(1)
					if mode == "concurrent poll" || mode == "timeout during assets" {
						close(assetEntered)
						<-assetContinue
					}
					if mode == "asset failure" {
						_, _ = w.Write([]byte(`{"success":false,"message":"asset rejected"}`))
						return
					}
					_, _ = w.Write([]byte(`{"success":true,"data":{"logical_id":"lass_ref","logical_group_id":"lasg_ref","status":"Active"}}`))
				case "POST /v1/video/generations":
					submissions.Add(1)
					assert.Equal(t, "Bearer sls-test-key", r.Header.Get("Authorization"))
					var body map[string]any
					if assert.NoError(t, common.DecodeJson(r.Body, &body)) {
						assert.Equal(t, modelName, body["model"])
						items := body["content"].([]any)
						assert.Equal(t, "asset://lass_ref", items[0].(map[string]any)["image_url"].(map[string]any)["url"])
					}
					if mode == "upstream rejection" {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{"error":{"message":"generation rejected"}}`))
						return
					}
					if mode == "timeout during upstream" {
						close(assetEntered)
						<-assetContinue
					} else if mode == "transport failure" {
						conn, _, err := w.(http.Hijacker).Hijack()
						if assert.NoError(t, err) {
							_ = conn.Close()
						}
						return
					}
					_, _ = w.Write([]byte(`{"task_id":"vendor-private-id","status":"queued"}`))
				case "GET /v1/video/generations/vendor-private-id":
					queries.Add(1)
					_, _ = w.Write([]byte(`{"task_id":"vendor-private-id","status":"succeeded","total_tokens":10,"result_url":"https://example.com/result.mp4"}`))
				default:
					t.Errorf("unexpected provider request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			if mode == "media conversion" {
				transport := http.DefaultTransport.(*http.Transport).Clone()
				transport.Proxy = nil
				dialer := (&net.Dialer{}).DialContext
				transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					if address == "media.example:80" {
						address = server.Listener.Addr().String()
					}
					return dialer(ctx, network, address)
				}
				previousTransport := service.GetHttpClient().Transport
				service.GetHttpClient().Transport = transport
				t.Cleanup(func() { service.GetHttpClient().Transport = previousTransport; transport.CloseIdleConnections() })
			}
			initialQuota := int(20 * common.QuotaPerUnit)
			user := model.User{Username: fmt.Sprintf("sls_user_%d", index), AffCode: fmt.Sprintf("sls_aff_%d", index), Quota: initialQuota}
			require.NoError(t, db.Create(&user).Error)
			baseURL := server.URL
			ch := model.Channel{Name: "SLS test", Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeSeedanceSLS, Key: "sls-test-key", BaseURL: &baseURL}
			require.NoError(t, db.Create(&ch).Error)
			require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: ch.Id, Enabled: true,
				Backend: service.AssetLibraryBackendSeedanceSLS, BaseURL: baseURL, AuthType: service.AssetLibraryAuthBearer, APIKey: "sls-test-key"}).Error)
			c := taskSubmissionTestContext()
			requestCtx, cancelCaller := context.WithCancel(c.Request.Context())
			defer cancelCaller()
			c.Request = c.Request.WithContext(requestCtx)
			c.Set("group", "default")
			c.Set("username", user.Username)
			c.Set("task_request", map[string]any{"model": modelName, "payload": map[string]any{"model": modelName,
				"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": baseURL + "/reference.png"}},
					map[string]any{"type": "text", "text": "animate"}}, "duration": 5}})
			if mode == "invalid media" {
				c.Set("task_request", map[string]any{"model": modelName, "payload": map[string]any{"model": modelName,
					"content": []any{map[string]any{"type": "text", "text": "animate"},
						map[string]any{"type": "audio_url", "audio_url": map[string]any{"url": "data:audio/wav;base64,YQ=="}}}, "duration": 5}})
			}
			c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
			common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, baseURL)
			common.SetContextKey(c, constant.ContextKeyChannelKey, ch.Key)
			common.SetContextKey(c, constant.ContextKeyChannelId, ch.Id)
			common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
			if mode == "media conversion" {
				common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, kitdto.ChannelOtherSettings{
					ParameterCapabilities: &kitdto.ParameterCapabilityConfig{Defaults: map[string]kitdto.ParameterCapability{
						"content.*.image_url": {Transform: "image_url_to_base64"},
					}},
				})
			}
			info := taskSubmissionRelayInfo(nil)
			info.UserId, info.OriginModelName, info.UserGroup, info.UsingGroup = user.Id, modelName, "default", "default"
			info.IsPlayground = true
			info.UserSetting.BillingPreference = "wallet_only"
			info.PublicTaskID, info.LockedChannel = model.GenerateTaskID(), &ch
			outcome, taskErr := executeTaskSubmission(c, info)
			require.Nil(t, taskErr)
			require.NotNil(t, outcome)
			assert.Zero(t, downloads.Load(), "receipt does not wait for asset I/O")
			assert.Zero(t, uploads.Load())
			assert.Zero(t, submissions.Load())
			var stored model.Task
			require.NoError(t, db.First(&stored, outcome.Task.ID).Error)
			require.NotNil(t, stored.PrivateData.PendingSubmission)
			assert.Empty(t, stored.PrivateData.UpstreamTaskID)
			assert.Equal(t, model.TaskChargeStateCharged, stored.PrivateData.BillingContext.ChargeState)
			assert.True(t, *stored.BillingReady)
			assert.Positive(t, stored.Quota)
			view, err := service.BuildTaskPluginView(&stored)
			require.NoError(t, err)
			viewBytes, err := common.Marshal(view)
			require.NoError(t, err)
			var publicView map[string]any
			require.NoError(t, common.Unmarshal(viewBytes, &publicView))
			receipt, err := plugin.Engine.CallMember(t.Context(), "native", "taskCreated", map[string]any{}, publicView)
			require.NoError(t, err)
			assert.Equal(t, stored.TaskID, receipt.(map[string]any)["id"])
			cancelCaller()
			if mode == "disabled channel" {
				require.NoError(t, db.Model(&ch).Update("status", common.ChannelStatusManuallyDisabled).Error)
			} else if mode == "deleted channel" {
				require.NoError(t, db.Delete(&ch).Error)
			}
			if mode == "restart during assets" || mode == "unknown submit" {
				stored.PrivateData.PendingSubmission.Stage = model.TaskSubmissionAssets
				if mode == "unknown submit" {
					stored.PrivateData.PendingSubmission.Stage = model.TaskSubmissionUpstream
				}
				stored.PrivateData.PendingSubmission.LeaseUntil = time.Now().Add(-time.Minute).Unix()
				require.NoError(t, stored.Update())
			}
			if mode == "concurrent poll" || mode == "timeout during assets" || mode == "timeout during upstream" {
				completed := make(chan struct{})
				go func() { service.RunTaskPollingOnce(context.Background(), nil); close(completed) }()
				select {
				case <-assetEntered:
				case <-time.After(10 * time.Second):
					t.Fatal("background asset upload did not start")
				}
				if mode != "concurrent poll" {
					require.NoError(t, db.Model(&stored).Update("submit_time", time.Now().Add(-2*time.Minute).Unix()).Error)
					constant.TaskTimeoutMinutes = 1
				}
				service.RunTaskPollingOnce(context.Background(), nil)
				close(assetContinue)
				<-completed
			} else {
				service.RunTaskPollingOnce(context.Background(), nil)
			}
			require.NoError(t, db.First(&stored, outcome.Task.ID).Error)
			var account model.User
			require.NoError(t, db.First(&account, user.Id).Error)
			unknownFailure := mode == "unknown submit" || mode == "transport failure" || mode == "timeout during upstream"
			if unknownFailure || mode == "asset failure" || mode == "invalid media" || mode == "disabled channel" || mode == "deleted channel" || mode == "upstream rejection" || mode == "timeout during assets" {
				require.Equal(t, model.TaskStatus(model.TaskStatusFailure), stored.Status)
				view, err = service.BuildTaskPluginView(&stored)
				require.NoError(t, err)
				viewBytes, err = common.Marshal(view)
				require.NoError(t, err)
				require.NoError(t, common.Unmarshal(viewBytes, &publicView))
				queried, err := plugin.Engine.CallMember(t.Context(), "native", "taskStatus", map[string]any{}, publicView)
				require.NoError(t, err)
				publicError := queried.(map[string]any)["error"].(map[string]any)
				if !unknownFailure {
					stage := model.TaskSubmissionAssets
					if mode == "upstream rejection" {
						stage = model.TaskSubmissionUpstream
					}
					code := stage + "_failed"
					if mode == "timeout during assets" {
						code = "asset_prepare_timeout"
					}
					assert.Equal(t, code, publicError["code"])
					assert.Equal(t, stage, publicError["stage"])
					assert.NotEmpty(t, publicError["message"])
					if mode == "asset failure" {
						assert.Contains(t, publicError["message"], "asset rejected")
					}
					assert.Equal(t, initialQuota, account.Quota)
					assert.Zero(t, account.UsedQuota)
					assert.Zero(t, stored.Quota)
				} else {
					assert.Equal(t, "upstream_submit_unknown", publicError["code"])
					assert.Equal(t, initialQuota-outcome.Task.Quota, account.Quota)
					if mode == "unknown submit" {
						assert.Zero(t, uploads.Load())
					}
				}
				service.RunTaskPollingOnce(context.Background(), nil)
				wantSubmissions := int32(0)
				if mode == "upstream rejection" || mode == "transport failure" || mode == "timeout during upstream" {
					wantSubmissions = 1
				}
				assert.Equal(t, wantSubmissions, submissions.Load(), "failed or uncertain work never submits again")
			} else {
				require.Equal(t, model.TaskStatus(model.TaskStatusSubmitted), stored.Status, stored.FailReason)
				assert.Nil(t, stored.PrivateData.PendingSubmission)
				assert.Equal(t, "vendor-private-id", stored.PrivateData.UpstreamTaskID)
				assert.Equal(t, int32(1), uploads.Load())
				assert.Equal(t, int32(1), submissions.Load())
				service.RunTaskPollingOnce(context.Background(), nil)
				require.NoError(t, db.First(&stored, outcome.Task.ID).Error)
				assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stored.Status)
				assert.Equal(t, int32(1), queries.Load())
				assert.Equal(t, int32(1), submissions.Load())
				assert.NotContains(t, string(stored.Data), "vendor-private-id")
			}
		})
	}
}
