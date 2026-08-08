package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeUserErrorLogsHidesProviderDetails(t *testing.T) {
	setting := operation_setting.GetErrorSetting()
	original := setting.HideErrorDetails
	setting.HideErrorDetails = true
	t.Cleanup(func() {
		setting.HideErrorDetails = original
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", common.RoleCommonUser)
	logs := []*model.Log{
		{
			Type:              model.LogTypeError,
			Content:           "status_code=500, xunfei response error",
			RequestId:         "request-log",
			UpstreamRequestId: "upstream-secret",
			Other: common.MapToJsonStr(map[string]any{
				"error_code":   "11200",
				"channel_name": "from xunfei",
				"status_code":  500,
			}),
		},
	}

	sanitizeUserErrorLogs(c, logs)

	assert.Contains(t, logs[0].Content, "request-log")
	assert.NotContains(t, logs[0].Content, "xunfei")
	assert.Empty(t, logs[0].UpstreamRequestId)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, other, "error_code")
	assert.NotContains(t, other, "channel_name")
	assert.EqualValues(t, 500, other["status_code"])
}

func TestSanitizeUserErrorLogsKeepsAdministratorDetails(t *testing.T) {
	setting := operation_setting.GetErrorSetting()
	original := setting.HideErrorDetails
	setting.HideErrorDetails = true
	t.Cleanup(func() {
		setting.HideErrorDetails = original
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", common.RoleAdminUser)
	logs := []*model.Log{{
		Type:              model.LogTypeError,
		Content:           "full upstream error",
		UpstreamRequestId: "upstream-request",
	}}

	sanitizeUserErrorLogs(c, logs)

	assert.Equal(t, "full upstream error", logs[0].Content)
	assert.Equal(t, "upstream-request", logs[0].UpstreamRequestId)
}
