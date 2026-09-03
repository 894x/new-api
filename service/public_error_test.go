package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

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

func TestTaskErrorForClientReplacesUpstreamRequestID(t *testing.T) {
	setHideErrorDetails(t, false)
	c := newErrorClientContext(common.RoleCommonUser, "request-local")
	taskErr := &dto.TaskError{
		Code:       "provider_error",
		Message:    "provider failed. Request id: upstream-request-id",
		StatusCode: http.StatusBadGateway,
	}

	result := TaskErrorForClient(c, taskErr)

	assert.Equal(t, "provider failed. request id: request-local", result.Message)
	assert.NotContains(t, result.Message, "upstream-request-id")
	assert.Equal(t, "provider failed. Request id: upstream-request-id", taskErr.Message)
}

func TestTaskFailReasonForClientReplacesUpstreamRequestID(t *testing.T) {
	setHideErrorDetails(t, false)
	c := newErrorClientContext(common.RoleCommonUser, "request-local")
	failReason := "The parameter ratio specified in the request is not valid. Request id: 0217882828433820e23c04e8b740c94d8512f0597aa5fa6ac2318 (code=InvalidParameter.TaskTypeConstraint)"

	result := TaskFailReasonForClient(c, failReason)

	assert.Equal(t, "The parameter ratio specified in the request is not valid. request id: request-local (code=InvalidParameter.TaskTypeConstraint)", result)
	assert.NotContains(t, result, "0217882828433820e23c04e8b740c94d8512f0597aa5fa6ac2318")
}

func TestTaskResponseDataForClientReplacesNestedUpstreamRequestID(t *testing.T) {
	c := newErrorClientContext(common.RoleCommonUser, "request-local")
	responseBody := []byte(`{"code":"success","data":{"status":"FAILURE","result_url":"ratio is invalid. Request id: upstream-request-123 (code=InvalidParameter.TaskTypeConstraint)"}}`)

	result := TaskResponseDataForClient(c, responseBody)

	assert.JSONEq(t, `{"code":"success","data":{"status":"FAILURE","result_url":"ratio is invalid. request id: request-local (code=InvalidParameter.TaskTypeConstraint)"}}`, string(result))
	assert.NotContains(t, string(result), "upstream-request-123")
}

func TestTaskResponseDataForClientKeepsRequestIDURLQuery(t *testing.T) {
	c := newErrorClientContext(common.RoleCommonUser, "request-local")
	responseBody := []byte(`{"status":"succeeded","content":{"video_url":"https://example.com/video.mp4?request_id=upstream-signed-value&token=abc"}}`)

	result := TaskResponseDataForClient(c, responseBody)

	assert.Equal(t, string(responseBody), string(result))
}

func TestTaskResponseDataForClientPreservesLargeJSONIntegers(t *testing.T) {
	c := newErrorClientContext(common.RoleCommonUser, "request-local")
	responseBody := []byte(`{"id":9007199254740993,"error":{"message":"failed. Request id: upstream-request-id"}}`)

	result := TaskResponseDataForClient(c, responseBody)

	assert.Contains(t, string(result), `"id":9007199254740993`)
	assert.NotContains(t, string(result), `"id":9007199254740992`)
	assert.Contains(t, string(result), "request id: request-local")
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
	original := operation_setting.GetErrorSetting().HideErrorDetails
	operation_setting.UpdateHideErrorDetails(enabled)
	t.Cleanup(func() {
		operation_setting.UpdateHideErrorDetails(original)
	})
}

func newErrorClientContext(role int, requestId string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", role)
	c.Set(common.RequestIdKey, requestId)
	return c
}
