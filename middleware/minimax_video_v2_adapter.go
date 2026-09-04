package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const miniMaxVideoV2MaxBodyBytes int64 = 64 << 20

// MiniMaxVideoV2RequestConvert retains the provider-native payload in task
// metadata while exposing the common fields needed by routing and billing.
func MiniMaxVideoV2RequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatMiniMaxVideoV2)
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		storage, err := common.GetBodyStorage(c)
		if err != nil {
			abortWithMiniMaxVideoV2Error(c, http.StatusBadRequest, "bad_request_error", "Invalid request body")
			return
		}
		if storage.Size() > miniMaxVideoV2MaxBodyBytes {
			common.CleanupBodyStorage(c)
			abortWithMiniMaxVideoV2Error(c, http.StatusBadRequest, "bad_request_error", "Request body must not exceed 64 MB")
			return
		}

		var nativeRequest map[string]any
		if err := common.UnmarshalBodyReusable(c, &nativeRequest); err != nil {
			common.CleanupBodyStorage(c)
			abortWithMiniMaxVideoV2Error(c, http.StatusBadRequest, "bad_request_error", "Invalid request body")
			return
		}

		modelName, _ := nativeRequest["model"].(string)
		if modelName != "MiniMax-H3" {
			common.CleanupBodyStorage(c)
			abortWithMiniMaxVideoV2Error(c, http.StatusBadRequest, "bad_request_error", "MiniMax video V2 currently supports model MiniMax-H3")
			return
		}
		prompt := ""
		if content, ok := nativeRequest["content"].([]any); ok {
			for _, item := range content {
				contentItem, ok := item.(map[string]any)
				if !ok || contentItem["type"] != "text" {
					continue
				}
				prompt, _ = contentItem["text"].(string)
				if prompt != "" {
					break
				}
			}
		}

		unifiedRequest := map[string]any{
			"model":    modelName,
			"prompt":   prompt,
			"metadata": nativeRequest,
		}
		if duration, ok := nativeRequest["duration"]; ok {
			unifiedRequest["duration"] = duration
		}

		body, err := common.Marshal(unifiedRequest)
		if err != nil {
			common.CleanupBodyStorage(c)
			abortWithMiniMaxVideoV2Error(c, http.StatusInternalServerError, "server_error", "Failed to convert request body")
			return
		}
		common.CleanupBodyStorage(c)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Set(common.KeyRequestBody, body)
		c.Next()
	}
}

func abortWithMiniMaxVideoV2Error(c *gin.Context, status int, errorType string, message string) {
	clientError := service.TaskErrorForClientWithSeparateRequestID(c, &taskdto.TaskError{
		Code:       errorType,
		Message:    message,
		StatusCode: status,
	})
	c.AbortWithStatusJSON(status, MiniMaxVideoV2ErrorPayload(
		status,
		clientError.Code,
		clientError.Message,
		c.GetString(common.RequestIdKey),
	))
}

func MiniMaxVideoV2ErrorPayload(status int, errorType string, message string, requestID string) gin.H {
	payload := gin.H{
		"type": "error",
		"error": gin.H{
			"type":      MiniMaxVideoV2ErrorType(status, errorType),
			"message":   message,
			"http_code": strconv.Itoa(status),
		},
	}
	if requestID != "" {
		payload["request_id"] = requestID
	}
	return payload
}

func MiniMaxVideoV2ErrorType(status int, errorType string) string {
	switch errorType {
	case "authorized_error",
		"bad_request_error",
		"rate_limit_error",
		"insufficient_balance_error",
		"unprocessable_entity_error",
		"overloaded_error",
		"server_error":
		return errorType
	}
	switch status {
	case http.StatusBadRequest:
		return "bad_request_error"
	case http.StatusUnauthorized, http.StatusForbidden:
		return "authorized_error"
	case http.StatusPaymentRequired:
		return "insufficient_balance_error"
	case http.StatusUnprocessableEntity:
		return "unprocessable_entity_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case 529:
		return "overloaded_error"
	default:
		if status >= http.StatusInternalServerError {
			return "server_error"
		}
		return "api_error"
	}
}
