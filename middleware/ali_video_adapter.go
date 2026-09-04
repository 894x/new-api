package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

// AliVideoRequestConvert retains the DashScope request as metadata while
// exposing the model and billing inputs to the common task relay pipeline.
func AliVideoRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTaskResponseFormat, constant.TaskResponseFormatAliVideo)
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
		if input, ok := nativeRequest["input"].(map[string]any); ok {
			prompt, _ = input["prompt"].(string)
		}

		unifiedRequest := map[string]any{
			"model":    modelName,
			"prompt":   prompt,
			"metadata": nativeRequest,
		}
		if parameters, ok := nativeRequest["parameters"].(map[string]any); ok {
			if duration, exists := parameters["duration"]; exists {
				unifiedRequest["duration"] = duration
			}
			if resolution, ok := parameters["resolution"].(string); ok {
				unifiedRequest["size"] = resolution
			}
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
