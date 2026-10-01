package jsplugin

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func (a *TaskAdaptor) nativeCompatibilityRoute(format, method string) (pluginruntime.Route, bool) {
	// Shared task protocols are declared by the plugin and their paths and
	// required hooks are defined and validated by the host.
	for _, claim := range a.plugin.Meta.Protocols {
		definition, ok := pluginruntime.HostProtocol(claim.Name)
		if !ok || definition.TaskResponseFormat == "" || definition.TaskResponseFormat != format {
			continue
		}
		for _, operation := range definition.Operations {
			if !slices.Contains(operation.Methods, method) {
				continue
			}
			route := pluginruntime.Route{Protocol: claim.Name, Method: method, Path: operation.Path}
			if operation.ModelField != "" {
				route.Type, route.Decode, route.Render = pluginruntime.RouteTypeSubmit, "decodeRequest", "renderSubmitted"
			} else {
				route.Type, route.Render, route.TaskIDParam = pluginruntime.RouteTypeQuery, "render", "task_id"
			}
			return route, true
		}
	}
	// Legacy compatibility bridges predate meta.protocols. Keep their native
	// route lookup until those plugins declare a shared host protocol too.
	var path string
	switch {
	case format == constant.TaskResponseFormatDoubaoVideo && (a.plugin.Meta.Key == "doubao" || a.plugin.Meta.Key == "seedance-sls"):
		path = "/" + a.plugin.Meta.Key + "/api/v3/contents/generations/tasks"
		if method == http.MethodGet {
			path += "/:task_id"
		}
	case format == constant.TaskResponseFormatMiniMaxVideoV2 && a.plugin.Meta.Key == "hailuo":
		path = "/hailuo/v2/video_generation"
		if method == http.MethodGet {
			path = "/hailuo/v2/query/video_generation/:task_id"
		}
	case format == constant.TaskResponseFormatAliVideo && a.plugin.Meta.Key == "alibaba":
		path = "/ali/api/v1/services/aigc/video-generation/video-synthesis"
		if method == http.MethodGet {
			path = "/ali/api/v1/tasks/:task_id"
		}
	default:
		return pluginruntime.Route{}, false
	}
	for _, route := range a.plugin.Meta.Routes {
		if route.Path == path && route.Method == method {
			return route, true
		}
	}
	return pluginruntime.Route{}, false
}

func (a *TaskAdaptor) SupportsNativeTaskFormat(format string) bool {
	_, submit := a.nativeCompatibilityRoute(format, http.MethodPost)
	_, query := a.nativeCompatibilityRoute(format, http.MethodGet)
	return submit && query
}

func (a *TaskAdaptor) prepareNativeCompatibilityRequest(c *gin.Context, info *relaycommon.RelayInfo, format string) *dto.TaskError {
	route, supported := a.nativeCompatibilityRoute(format, http.MethodPost)
	if !supported {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported native task protocol"), "invalid_api_platform", http.StatusBadRequest)
	}
	pinnedValue, _ := c.Get(pluginruntime.ContextKeyPinnedPlugin)
	pinned, ok := pinnedValue.(pluginruntime.PinnedPlugin)
	if !ok || pinned.Plugin != a.plugin {
		return service.TaskErrorWrapperLocal(fmt.Errorf("native task plugin is not pinned"), "plugin_request_invalid", http.StatusBadRequest)
	}
	if route.Protocol != "" {
		modelName, declared := pinned.Generation.CanonicalModel(info.OriginModelName)
		if !declared {
			modelName, _ = pinned.Generation.CanonicalModel(info.GetUpstreamModelName())
		}
		bindings := pinned.Generation.LookupEndpointCandidates(route.Method, route.Path, modelName)
		if !slices.ContainsFunc(bindings, func(binding pluginruntime.ProtocolBinding) bool {
			return binding.Plugin == a.plugin && binding.Protocol == route.Protocol
		}) {
			return service.TaskErrorWrapperLocal(fmt.Errorf("selected plugin does not claim the model on this task protocol"), "invalid_api_platform", http.StatusBadRequest)
		}
	}
	var envelope map[string]any
	if err := common.UnmarshalBodyReusable(c, &envelope); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	// The compatibility middleware retains the complete provider body here;
	// managed custody has already rewritten its references before this point.
	body, ok := envelope["metadata"].(map[string]any)
	if !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("missing native video payload"), "invalid_request", http.StatusBadRequest)
	}
	request := pluginruntime.RouteRequestContext{
		Method: c.Request.Method, Path: c.Request.URL.Path,
		Body: map[string]any{"kind": "json", "value": body}, RequestBody: body,
	}
	decodeContext := request.JSValue()
	if route.Protocol != "" {
		decodeContext["protocol"], decodeContext["operation"] = route.Protocol, "create"
		decodeContext["model"], decodeContext["stream"] = info.OriginModelName, false
		decodeContext["upstreamModel"] = info.GetUpstreamModelName()
	}
	value, err := route.CallHook(c.Request.Context(), a.plugin.Engine, route.Decode, decodeContext)
	resolved, valid := value.(map[string]any)
	if err != nil || !valid || resolved["kind"] != "submit" || resolved["model"] != info.OriginModelName {
		return service.TaskErrorWrapperLocal(fmt.Errorf("native task decoder rejected the request: %v", err), "plugin_request_invalid", http.StatusBadRequest)
	}
	if _, exists := resolved["renderer"]; exists {
		return service.TaskErrorWrapperLocal(fmt.Errorf("native task decoder cannot override the renderer"), "plugin_request_invalid", http.StatusBadRequest)
	}
	if replacement, exists := resolved["requestBody"]; exists {
		request.RequestBody = replacement
	}
	if action, ok := resolved["action"].(string); ok {
		info.Action = action
		c.Set("task_action", action)
	}
	c.Set(pluginruntime.ContextKeyPinnedRoute, pluginruntime.PinnedRoute{Generation: pinned.Generation, Plugin: a.plugin, Route: route})
	c.Set(pluginruntime.ContextKeyRouteRequest, request)
	c.Set("task_request", request.RequestBody)
	return nil
}

func (a *TaskAdaptor) RenderNativeTask(c *gin.Context, format string, task *model.Task) ([]byte, error) {
	route, supported := a.nativeCompatibilityRoute(format, http.MethodGet)
	if !supported {
		return nil, fmt.Errorf("unsupported native task protocol")
	}
	view, err := service.BuildTaskPluginViewForClient(c, task)
	if err != nil {
		return nil, err
	}
	request := pluginruntime.RouteRequestContext{Method: c.Request.Method, Path: c.Request.URL.Path, Params: map[string]string{"task_id": task.TaskID}}
	value, err := route.CallHook(c.Request.Context(), a.plugin.Engine, route.Render, request.JSValue(), jsonValue(view))
	if err != nil {
		return nil, err
	}
	body, err := common.Marshal(value)
	if err != nil {
		return nil, err
	}
	return service.TaskResponseDataForClient(c, body), nil
}
