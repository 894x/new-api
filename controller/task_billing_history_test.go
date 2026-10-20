package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestTaskBillingHistoryDatabase(t *testing.T) {
	mainDB, dialect := openTaskDialectDatabase(t, &model.Task{})
	logDB, _ := openTaskDialectDatabase(t, &model.Log{})
	logDialect := dialect
	if dsn := os.Getenv("TEST_TASK_LOG_CLICKHOUSE_DSN"); dsn != "" {
		var err error
		logDB, err = gorm.Open(clickhouse.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: fmt.Sprintf("tbilling_%d_", time.Now().UnixNano())}})
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
	oldDB, oldLogs := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = mainDB, logDB
	common.SetDatabaseTypes(dialect, logDialect)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogs
		common.SetDatabaseTypes(oldMainType, oldLogType)
	})
	tasks := []model.Task{
		{TaskID: "task_refunded", UserId: 74, Status: model.TaskStatusFailure, Quota: 0, PrivateData: model.TaskPrivateData{Key: "secret", Execution: &model.TaskExecutionSnapshot{RequestID: "submit-refund"}}},
		{TaskID: "task_extra", UserId: 74, Status: model.TaskStatusSuccess, Quota: 214968},
		{TaskID: "task_missing", UserId: 74, Status: model.TaskStatusSuccess, Quota: 100},
		{TaskID: "task_refunded", UserId: 75, Status: model.TaskStatusSuccess, Quota: 999},
		{TaskID: "task_a%_!", UserId: 74, Status: model.TaskStatusSuccess, Quota: 1},
		{TaskID: "task_truncated", UserId: 74, Status: model.TaskStatusSuccess, Quota: 201},
		{TaskID: "task_pending", UserId: 74, Status: model.TaskStatusFailure, Quota: 0, BillingRecoveryPending: true},
	}
	for index := range tasks {
		tasks[index].CreatedAt, tasks[index].SubmitTime = 1, 1
	}
	require.NoError(t, mainDB.Create(&tasks).Error)
	logs := []model.Log{
		{UserId: 74, RequestId: "submit-refund", Type: model.LogTypeConsume, Quota: 506849, CreatedAt: 1, Other: `{"task_id":"task_refunded","admin_info":{"secret":"private"}}`},
		{UserId: 74, RequestId: "refund-new-id", Type: model.LogTypeRefund, Quota: 506849, CreatedAt: 2, Other: `{"task_id": "task_refunded", "reason":"provider-secret"}`},
		{UserId: 75, RequestId: "foreign", Type: model.LogTypeConsume, Quota: 999, CreatedAt: 3, Other: `{"task_id":"task_refunded"}`},
		{UserId: 74, RequestId: "submit-extra", Type: model.LogTypeConsume, Quota: 165410, CreatedAt: 1, Other: `{"task_id":"task_extra"}`},
		{UserId: 74, RequestId: "extra-new-id", Type: model.LogTypeConsume, Quota: 49558, CreatedAt: 2, Other: `{"task_id":"task_extra","pre_consumed_quota":165410}`},
		{UserId: 74, RequestId: "wrong-key", Type: model.LogTypeConsume, Quota: 99999, Other: `{"note":"task_refunded","task_id":"another"}`},
		{UserId: 74, RequestId: "malformed", Type: model.LogTypeConsume, Quota: 99999, Other: `{"task_id":"task_refunded"`},
		{UserId: 74, RequestId: "escaped", Type: model.LogTypeConsume, Quota: 1, Other: `{"task_id":"task_a%_!"}`},
		{UserId: 74, RequestId: "wildcard-neighbor", Type: model.LogTypeConsume, Quota: 999, Other: `{"task_id":"task_abcX!"}`},
		{UserId: 74, RequestId: "pending-charge", Type: model.LogTypeConsume, Quota: 1, Other: `{"task_id":"task_pending"}`},
		{UserId: 74, RequestId: "pending-refund", Type: model.LogTypeRefund, Quota: 1, Other: `{"task_id":"task_pending"}`},
	}
	for index := range 201 {
		logs = append(logs, model.Log{UserId: 74, RequestId: fmt.Sprintf("many-%03d", index), Type: model.LogTypeConsume, Quota: 1, Other: `{"task_id":"task_truncated"}`})
	}
	require.NoError(t, logDB.Create(&logs).Error)

	for _, tc := range []struct {
		name, query string
		user, role  int
		status      int
		net         int64
		entries     int
		reconciled  bool
	}{
		{"submit finds full refund", "request_id=submit-refund", 74, common.RoleCommonUser, 200, 0, 2, true},
		{"refund finds original charge", "request_id=refund-new-id", 74, common.RoleCommonUser, 200, 0, 2, true},
		{"task lookup ignores foreign user override", "task_id=task_refunded&user_id=75", 74, common.RoleCommonUser, 200, 0, 2, true},
		{"foreign request denied", "request_id=foreign&user_id=75", 74, common.RoleCommonUser, 404, 0, 0, false},
		{"admin selects task owner", "task_id=task_refunded&user_id=75", 1, common.RoleAdminUser, 200, 999, 1, true},
		{"admin ambiguous task fails closed", "task_id=task_refunded", 1, common.RoleAdminUser, 404, 0, 0, false},
		{"additional charge resolves historical task", "request_id=extra-new-id", 74, common.RoleCommonUser, 200, 214968, 2, true},
		{"missing logs cannot reconcile", "task_id=task_missing", 74, common.RoleCommonUser, 200, 0, 0, false},
		{"pending recovery cannot confirm refund", "task_id=task_pending", 74, common.RoleCommonUser, 200, 0, 2, false},
		{"literal wildcard task ID", "task_id=" + url.QueryEscape("task_a%_!"), 74, common.RoleCommonUser, 200, 1, 1, true},
		{"truncated history cannot reconcile", "task_id=task_truncated", 74, common.RoleCommonUser, 200, 200, 200, false},
		{"both identifiers rejected", "request_id=submit-refund&task_id=task_extra", 74, common.RoleCommonUser, 400, 0, 0, false},
		{"missing identity denied", "request_id=submit-refund", 0, common.RoleCommonUser, 401, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/log/task-billing?"+tc.query, nil)
			c.Set("id", tc.user)
			c.Set("role", tc.role)
			GetTaskBillingHistory(c)
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			if tc.status != http.StatusOK {
				return
			}
			var result struct {
				Success bool                     `json:"success"`
				Data    model.TaskBillingHistory `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
			require.True(t, result.Success)
			assert.Equal(t, tc.net, result.Data.NetQuota)
			assert.Len(t, result.Data.Entries, tc.entries)
			assert.Equal(t, tc.reconciled, result.Data.Reconciled)
			assert.NotContains(t, recorder.Body.String(), "secret")
			if tc.name == "additional charge resolves historical task" {
				assert.Equal(t, "submit-extra", result.Data.SubmitRequestID)
				assert.Equal(t, "additional_charge", result.Data.Entries[1].Kind)
			}
			if tc.name == "refund finds original charge" {
				assert.Equal(t, "submit-refund", result.Data.SubmitRequestID)
				assert.EqualValues(t, 506849, result.Data.RefundedQuota)
				assert.Equal(t, "refund", result.Data.Entries[1].Kind)
			}
		})
	}
}
