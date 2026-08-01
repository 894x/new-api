package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIErrorForClientHidesDetailsFromRegularUsers(t *testing.T) {
	setHideErrorDetails(t, true)
	c := newErrorClientContext(common.RoleCommonUser, "request-123")
	err := types.NewOpenAIError(
		errors.New("xunfei response error: AppIdNoAuthError"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	result := OpenAIErrorForClient(c, err)

	require.Equal(t, "request_failed", result.Code)
	assert.Equal(t, "new_api_error", result.Type)
	assert.Contains(t, result.Message, "请稍后重试")
	assert.Contains(t, result.Message, "request-123")
	assert.NotContains(t, result.Message, "xunfei")
	assert.Equal(t, "xunfei response error: AppIdNoAuthError", err.Error())
}

func TestOpenAIErrorForClientShowsDetailsToAdministrators(t *testing.T) {
	setHideErrorDetails(t, true)
	c := newErrorClientContext(common.RoleAdminUser, "request-admin")
	err := types.NewOpenAIError(
		errors.New("upstream authentication failed"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	result := OpenAIErrorForClient(c, err)

	assert.Contains(t, result.Message, "upstream authentication failed")
	assert.Contains(t, result.Message, "request-admin")
	assert.Equal(t, types.ErrorCodeBadResponseStatusCode, result.Code)
}

func TestOpenAIErrorForClientShowsDetailsWhenSettingDisabled(t *testing.T) {
	setHideErrorDetails(t, false)
	c := newErrorClientContext(common.RoleCommonUser, "request-visible")
	err := types.NewOpenAIError(
		errors.New("provider-specific failure"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadGateway,
	)

	result := OpenAIErrorForClient(c, err)

	assert.Contains(t, result.Message, "provider-specific failure")
	assert.Contains(t, result.Message, "request-visible")
}

func TestTaskErrorForClientDoesNotMutateInternalError(t *testing.T) {
	setHideErrorDetails(t, true)
	c := newErrorClientContext(common.RoleCommonUser, "request-task")
	taskErr := &dto.TaskError{
		Code:       "provider_error",
		Message:    "secret provider reason",
		Data:       map[string]any{"provider": "secret"},
		StatusCode: http.StatusBadGateway,
	}

	result := TaskErrorForClient(c, taskErr)

	require.NotSame(t, taskErr, result)
	assert.Equal(t, "request_failed", result.Code)
	assert.Nil(t, result.Data)
	assert.Contains(t, result.Message, "request-task")
	assert.Equal(t, "secret provider reason", taskErr.Message)
	assert.NotNil(t, taskErr.Data)
}

func TestStreamErrorDataForClientHidesErrorEventsOnly(t *testing.T) {
	setHideErrorDetails(t, true)
	c := newErrorClientContext(common.RoleCommonUser, "request-stream")
	rawError := `{"type":"response.failed","error":{"message":"secret stream failure"}}`
	normalChunk := `{"choices":[{"delta":{"content":"hello"}}]}`
	nullErrorChunk := `{"type":"response.completed","error":null}`

	publicData, isError := StreamErrorDataForClient(c, rawError)
	unchangedData, normalIsError := StreamErrorDataForClient(c, normalChunk)
	unchangedNullErrorData, nullErrorIsError := StreamErrorDataForClient(c, nullErrorChunk)

	require.True(t, isError)
	assert.Contains(t, publicData, "request-stream")
	assert.NotContains(t, publicData, "secret stream failure")
	assert.False(t, normalIsError)
	assert.Equal(t, normalChunk, unchangedData)
	assert.False(t, nullErrorIsError)
	assert.Equal(t, nullErrorChunk, unchangedNullErrorData)
}

func setHideErrorDetails(t *testing.T, enabled bool) {
	t.Helper()
	setting := operation_setting.GetErrorSetting()
	original := setting.HideErrorDetails
	setting.HideErrorDetails = enabled
	t.Cleanup(func() {
		setting.HideErrorDetails = original
	})
}

func newErrorClientContext(role int, requestId string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", role)
	c.Set(common.RequestIdKey, requestId)
	return c
}
