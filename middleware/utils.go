package middleware

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	publicMessage := common.MessageWithRequestId(message, c.GetString(common.RequestIdKey))
	if service.ShouldHideErrorDetails(c) {
		publicMessage = service.PublicErrorMessage(c.GetString(common.RequestIdKey))
		codeStr = "request_failed"
	}
	userId := c.GetInt("id")
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": publicMessage,
			"type":    "new_api_error",
			"code":    codeStr,
		},
	})
	c.Abort()
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
}

func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	publicDescription := common.MessageWithRequestId(description, c.GetString(common.RequestIdKey))
	if service.ShouldHideErrorDetails(c) {
		publicDescription = service.PublicErrorMessage(c.GetString(common.RequestIdKey))
		code = 4
	}
	c.JSON(statusCode, gin.H{
		"description": publicDescription,
		"type":        "new_api_error",
		"code":        code,
	})
	c.Abort()
	logger.LogError(c.Request.Context(), description)
}
