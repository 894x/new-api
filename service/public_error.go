package service

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const publicErrorMessage = "请求处理失败，请稍后重试。如果多次尝试仍然失败，请联系管理员。"

func ShouldHideErrorDetails(c *gin.Context) bool {
	if !operation_setting.ShouldHideErrorDetails() {
		return false
	}
	if c == nil {
		return true
	}
	if roleValue, exists := c.Get("role"); exists {
		if role, ok := roleValue.(int); ok {
			return role < common.RoleAdminUser
		}
	}
	return !model.IsAdmin(c.GetInt("id"))
}

func PublicErrorMessage(requestId string) string {
	return common.MessageWithRequestId(publicErrorMessage, requestId)
}

func OpenAIErrorForClient(c *gin.Context, err *types.NewAPIError) types.OpenAIError {
	if ShouldHideErrorDetails(c) {
		return types.OpenAIError{
			Message: PublicErrorMessage(c.GetString(common.RequestIdKey)),
			Type:    "new_api_error",
			Param:   "",
			Code:    "request_failed",
		}
	}
	result := err.ToOpenAIError()
	result.Message = common.MessageWithRequestId(result.Message, c.GetString(common.RequestIdKey))
	return result
}

func ClaudeErrorForClient(c *gin.Context, err *types.NewAPIError) types.ClaudeError {
	if ShouldHideErrorDetails(c) {
		return types.ClaudeError{
			Type:    "request_failed",
			Message: PublicErrorMessage(c.GetString(common.RequestIdKey)),
		}
	}
	result := err.ToClaudeError()
	result.Message = common.MessageWithRequestId(result.Message, c.GetString(common.RequestIdKey))
	return result
}

func TaskErrorForClient(c *gin.Context, taskErr *dto.TaskError) *dto.TaskError {
	if taskErr == nil {
		return nil
	}
	result := *taskErr
	if ShouldHideErrorDetails(c) {
		result.Code = "request_failed"
		result.Message = PublicErrorMessage(c.GetString(common.RequestIdKey))
		result.Data = nil
		return &result
	}
	result.Message = common.MessageWithRequestId(result.Message, c.GetString(common.RequestIdKey))
	return &result
}

func TaskFailReasonForClient(c *gin.Context, failReason string) string {
	if failReason == "" || !ShouldHideErrorDetails(c) {
		return failReason
	}
	return PublicErrorMessage(c.GetString(common.RequestIdKey))
}

func StreamErrorDataForClient(c *gin.Context, data string) (string, bool) {
	var payload struct {
		Type  string          `json:"type"`
		Error json.RawMessage `json:"error"`
	}
	if err := common.UnmarshalJsonStr(data, &payload); err != nil {
		return data, false
	}
	payloadType := strings.ToLower(strings.TrimSpace(payload.Type))
	errorText := strings.TrimSpace(string(payload.Error))
	hasError := errorText != "" && errorText != "null" && errorText != "{}"
	isError := payloadType == "error" || payloadType == "upstream_error" ||
		payloadType == "response.error" || payloadType == "response.failed" || hasError
	if !isError || !ShouldHideErrorDetails(c) {
		return data, isError
	}
	publicType := payload.Type
	if strings.TrimSpace(publicType) == "" {
		publicType = "error"
	}
	publicData, err := common.Marshal(map[string]any{
		"type": publicType,
		"error": types.OpenAIError{
			Message: PublicErrorMessage(c.GetString(common.RequestIdKey)),
			Type:    "new_api_error",
			Param:   "",
			Code:    "request_failed",
		},
	})
	if err != nil {
		return data, true
	}
	return string(publicData), true
}
