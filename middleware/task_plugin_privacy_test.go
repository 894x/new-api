package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPluginErrorsRespectRoleAndReplacementPolicy(t *testing.T) {
	previous := operation_setting.GetErrorSetting()
	t.Cleanup(func() {
		operation_setting.UpdateHideErrorDetails(previous.HideErrorDetails)
		require.NoError(t, operation_setting.UpdateErrorResponseReplacementRules(previous.ResponseReplacementRules))
	})
	publicMessage := strings.TrimSuffix(service.PublicErrorMessage(""), " (request id: )")
	for _, tc := range []struct {
		name                  string
		role, status          int
		hide, replace         bool
		wantCode, wantMessage string
	}{
		{"customer validation", common.RoleCommonUser, 400, true, false, "request_failed", publicMessage},
		{"admin validation", common.RoleAdminUser, 400, true, false, "invalid_request", "provider rejection request id: local-request"},
		{"visible validation", common.RoleCommonUser, 400, false, false, "invalid_request", "provider rejection request id: local-request"},
		{"customer server failure", common.RoleCommonUser, 502, true, false, "request_failed", publicMessage},
		{"admin server failure", common.RoleAdminUser, 502, true, false, "server_error", "Task request failed"},
		{"visible server failure", common.RoleCommonUser, 502, false, false, "server_error", "Task request failed"},
		{"customer replacement", common.RoleCommonUser, 502, true, true, "request_failed", "retry later"},
		{"visible replacement", common.RoleCommonUser, 502, false, true, "server_error", "retry later"},
		{"admin ignores replacement", common.RoleAdminUser, 502, true, true, "server_error", "Task request failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.UpdateHideErrorDetails(tc.hide)
			var rules []operation_setting.ErrorResponseReplacementRule
			if tc.replace {
				rules = []operation_setting.ErrorResponseReplacementRule{{StatusCode: 502, Match: "provider rejection.*", Replacement: "retry later"}}
			}
			require.NoError(t, operation_setting.UpdateErrorResponseReplacementRules(rules))
			for _, renderer := range []string{"native", "missing", "throwing", "unbound"} {
				t.Run(renderer, func(t *testing.T) {
					source := genericTaskPluginSource
					switch renderer {
					case "native":
						source += `
export const native = {error: function(ctx, error) { return {code:error.code,message:error.message,request_id:error.requestId,retryable:error.retryable}; }};
`
					case "throwing":
						source += `export const native = {error: function() { throw new Error("renderer-private-data"); }};`
					}
					plugin := compileTaskRoutePlugin(t, source)
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/vendor/tasks", nil)
					c.Set("role", tc.role)
					c.Set(common.RequestIdKey, "local-request")
					if renderer != "unbound" {
						c.Set(jsplugin.ContextKeyPinnedRoute, jsplugin.PinnedRoute{Plugin: plugin})
						c.Set(jsplugin.ContextKeyRouteRequest, jsplugin.RouteRequestContext{Method: http.MethodPost, Path: "/vendor/tasks"})
					}
					internal := &dto.TaskError{Code: "private_provider_code", Message: "provider rejection request id: upstream-private-id", StatusCode: tc.status, Data: map[string]any{"credential": "private-key"}}
					if renderer == "unbound" {
						abortTaskPluginRouteErrorDetail(c, tc.status, internal.Message)
					} else {
						require.True(t, RespondTaskPluginError(c, internal))
					}
					require.Equal(t, tc.status, recorder.Code)
					var payload map[string]any
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
					assert.Equal(t, tc.wantCode, payload["code"])
					if renderer == "native" {
						assert.Equal(t, tc.wantMessage, payload["message"])
						assert.Equal(t, "local-request", payload["request_id"])
						assert.Equal(t, tc.status >= 500, payload["retryable"])
					} else {
						assert.Equal(t, fmt.Sprintf("%s (request id: local-request)", tc.wantMessage), payload["message"])
					}
					assert.NotContains(t, recorder.Body.String(), "upstream-private-id")
					assert.NotContains(t, recorder.Body.String(), "private-key")
					assert.NotContains(t, recorder.Body.String(), "renderer-private-data")
					assert.Equal(t, "provider rejection request id: upstream-private-id", internal.Message)
					assert.Equal(t, "private_provider_code", internal.Code)
				})
			}
		})
	}
}
