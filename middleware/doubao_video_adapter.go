package middleware

import (
	"bytes"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service"
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
		var originIDs []string
		if content, ok := nativeRequest["content"].([]any); ok {
			for _, item := range content {
				contentItem, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if contentItem["type"] == "text" && prompt == "" {
					prompt, _ = contentItem["text"].(string)
				}
				if contentItem["type"] == "draft_task" {
					draft, _ := contentItem["draft_task"].(map[string]any)
					id, _ := draft["id"].(string)
					if id = strings.TrimSpace(id); id != "" {
						originIDs = append(originIDs, id)
					}
				}
			}
		}
		// Resolve draft ownership and its channel before distribution. Both the
		// historical numeric platform and canonical plugin key are supported.
		// The late adaptor decoder cannot safely add a new pin after routing.
		if len(originIDs) > 0 {
			if len(originIDs) > maxOriginTaskIDs || len(originIDs[0]) > 4*maxOriginTaskIDLen {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "origin task ids are invalid")
				return
			}
			origin, exists, err := model.GetByTaskId(common.GetContextKeyInt(c, constant.ContextKeyUserId), originIDs[0])
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to resolve origin task")
				return
			}
			if !exists || origin == nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "origin task not found or not owned by you")
				return
			}
			generation := pluginruntime.DefaultRegistry.Generation()
			var plugin *pluginruntime.LoadedPlugin
			for _, key := range []string{"doubao", "seedance-sls"} {
				candidate, found := generation.Get(key)
				if found && slices.Contains(taskPluginLegacyPlatforms(candidate.Meta), origin.Platform) {
					plugin = candidate
					break
				}
			}
			if plugin == nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "origin task does not belong to an enabled Doubao video plugin")
				return
			}
			if intentErr := applyOriginTaskIntent(c, map[string]any{"originTaskIds": originIDs}, plugin.Meta); intentErr != nil {
				abortWithOpenAiMessage(c, intentErr.StatusCode, intentErr.Message)
				return
			}
			c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Generation: generation, Plugin: plugin})
			c.Set("expected_task_plugin_key", plugin.Meta.Key)
			c.Set("task_plugin_key", plugin.Meta.Key)
			service.AppendTaskPluginIdentityFilter(c, plugin.Meta.Key)
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
