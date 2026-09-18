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

func TestOpenAIErrorForClientUsesConfiguredReplacementForRegularUsers(t *testing.T) {
	setHideErrorDetails(t, true)
	setErrorResponseReplacementRules(t, []operation_setting.ErrorResponseReplacementRule{{
		StatusCode:  http.StatusInternalServerError,
		Match:       "Moonshot AI",
		Replacement: "rhzs",
	}})
	c := newErrorClientContext(common.RoleCommonUser, "request-replaced")
	err := types.NewOpenAIError(
		errors.New("服务处理请求时发生内部错误，请稍后重试。若持续出现该问题，请联系 Moonshot AI 技术支持。"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	result := OpenAIErrorForClient(c, err)

	assert.Contains(t, result.Message, "请联系 rhzs 技术支持")
	assert.Contains(t, result.Message, "request-replaced")
	assert.NotContains(t, result.Message, "Moonshot AI")
	assert.Equal(t, "request_failed", result.Code)
	assert.Equal(t, "new_api_error", result.Type)
	assert.Contains(t, err.Error(), "Moonshot AI")
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
}

func TestOpenAIErrorForClientPreservesErrorShapeWhenDetailsAreVisible(t *testing.T) {
	setHideErrorDetails(t, false)
	setErrorResponseReplacementRules(t, []operation_setting.ErrorResponseReplacementRule{{
		StatusCode:  http.StatusBadGateway,
		Match:       "provider failure",
		Replacement: "service unavailable",
	}})
	c := newErrorClientContext(common.RoleCommonUser, "request-visible-replaced")
	err := types.NewOpenAIError(
		errors.New("provider failure"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadGateway,
	)

	result := OpenAIErrorForClient(c, err)

	assert.Contains(t, result.Message, "service unavailable")
	assert.Contains(t, result.Message, "request-visible-replaced")
	assert.Equal(t, types.ErrorCodeBadResponseStatusCode, result.Code)
}

func TestOpenAIErrorForClientKeepsOriginalMessageForAdministrators(t *testing.T) {
	setHideErrorDetails(t, true)
	setErrorResponseReplacementRules(t, []operation_setting.ErrorResponseReplacementRule{{
		StatusCode:  http.StatusInternalServerError,
		Match:       "provider failure",
		Replacement: "service unavailable",
	}})
	c := newErrorClientContext(common.RoleAdminUser, "request-admin-original")
	err := types.NewOpenAIError(
		errors.New("provider failure"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	result := OpenAIErrorForClient(c, err)

	assert.Contains(t, result.Message, "provider failure")
	assert.NotContains(t, result.Message, "service unavailable")
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

func TestTaskErrorForClientUsesConfiguredReplacement(t *testing.T) {
	setHideErrorDetails(t, true)
	setErrorResponseReplacementRules(t, []operation_setting.ErrorResponseReplacementRule{{
		StatusCode:  http.StatusBadGateway,
		Match:       "provider task failure",
		Replacement: "任务服务暂时不可用。",
	}})
	c := newErrorClientContext(common.RoleCommonUser, "request-task-replaced")
	taskErr := &dto.TaskError{
		Code:       "provider_error",
		Message:    "provider task failure",
		Data:       map[string]any{"provider": "private"},
		StatusCode: http.StatusBadGateway,
	}

	result := TaskErrorForClient(c, taskErr)

	assert.Contains(t, result.Message, "任务服务暂时不可用。")
	assert.Contains(t, result.Message, "request-task-replaced")
	assert.Equal(t, "request_failed", result.Code)
	assert.Nil(t, result.Data)
	assert.Equal(t, "provider task failure", taskErr.Message)
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

func TestTaskErrorForClientWithSeparateRequestIDKeepsMessageStructured(t *testing.T) {
	setHideErrorDetails(t, false)
	c := newErrorClientContext(common.RoleCommonUser, "request-local")
	taskErr := &dto.TaskError{
		Code:       "unprocessable_entity_error",
		Message:    "video description contains sensitive content (1026)",
		StatusCode: http.StatusUnprocessableEntity,
	}

	result := TaskErrorForClientWithSeparateRequestID(c, taskErr)

	assert.Equal(t, taskErr.Message, result.Message)
	assert.NotContains(t, result.Message, "request-local")
	assert.Equal(t, taskErr.Code, result.Code)
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

func setErrorResponseReplacementRules(t *testing.T, rules []operation_setting.ErrorResponseReplacementRule) {
	t.Helper()
	original := append([]operation_setting.ErrorResponseReplacementRule(nil), operation_setting.GetErrorSetting().ResponseReplacementRules...)
	require.NoError(t, operation_setting.UpdateErrorResponseReplacementRules(rules))
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdateErrorResponseReplacementRules(original))
	})
}

func newErrorClientContext(role int, requestId string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", role)
	c.Set(common.RequestIdKey, requestId)
	return c
}
