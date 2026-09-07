package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminAuthSkipsAssetReadAuditsButPreservesWriteAudits(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	user := createMiddlewarePATUser(t, "asset-audit-admin", "asset-audit-pat")
	require.NoError(t, model.DB.Model(user).Update("role", common.RoleAdminUser).Error)
	for _, tc := range []struct {
		action, route, url string
		wantAudit          bool
	}{
		{"ListAssets", "/api/asset-library/admin/users/:user_id", "/api/asset-library/admin/users/1", false},
		{"ListAssetGroups", "/api/asset-library/admin/users/:user_id", "/api/asset-library/admin/users/1", false},
		{"GetAsset", "/api/asset-library/admin/users/:user_id", "/api/asset-library/admin/users/1", false},
		{"GetAssetGroup", "/api/asset-library/admin/users/:user_id", "/api/asset-library/admin/users/1", false},
		{"DeleteAsset", "/api/asset-library/admin/users/:user_id", "/api/asset-library/admin/users/1", true},
		{"UnknownAction", "/api/asset-library/admin/users/:user_id", "/api/asset-library/admin/users/1", true},
		{"GetAsset", "/api/asset-library/admin/assets/:id/sync", "/api/asset-library/admin/assets/1/sync", true},
	} {
		for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
			router := gin.New()
			called := false
			router.POST(tc.route, AdminAuth(), func(c *gin.Context) {
				called = true
				// This assertion runs inside the real auth lifecycle, before the async
				// fallback can be scheduled. No sleeps or races against the log worker.
				_, audited := c.Writer.(*auditResponseWriter)
				assert.Equal(t, tc.wantAudit, audited, tc.action)
				assert.Equal(t, user.Id, c.GetInt("id"))
				// The stub mutation handler represents an operation with its own audit.
				c.Set(string(constant.ContextKeyAuditLogged), true)
				c.Status(status)
			})
			request := httptest.NewRequest(http.MethodPost, tc.url+"?Action="+tc.action, nil)
			request.Header.Set("Authorization", "Bearer asset-audit-pat")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.True(t, called)
			assert.Equal(t, status, response.Code)
		}
	}
}
