package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

// DoubaoVideoRequestConvert converts the provider-native video request into
// the common task request used by routing, validation, and billing. The
// original provider payload is retained in Metadata for lossless forwarding.
func DoubaoVideoRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatDoubaoVideo)
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		var nativeRequest map[string]any
		if err := common.UnmarshalBodyReusable(c, &nativeRequest); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
			return
		}

		modelName, _ := nativeRequest["model"].(string)
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
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to convert request body")
			return
		}
		common.CleanupBodyStorage(c)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Set(common.KeyRequestBody, body)
		c.Next()
	}
}
