package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
			requestID := fmt.Sprintf("asset-audit-%s-%s-%d", tc.action, tc.url, status)
			router := gin.New()
			called := false
			router.Use(func(c *gin.Context) { c.Set(common.RequestIdKey, requestID) })
			router.POST(tc.route, AdminAuth(), func(c *gin.Context) {
				called = true
				assert.Equal(t, user.Id, c.GetInt("id"))
				actor := model.AuditActorFromContext(c.Request.Context())
				assert.Equal(t, model.AuditActor{UserID: user.Id, Role: common.RoleAdminUser, Username: user.Username}, actor)
				c.Status(status)
			})
			request := httptest.NewRequest(http.MethodPost, tc.url+"?Action="+tc.action, nil)
			request.Header.Set("Authorization", "Bearer asset-audit-pat")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.True(t, called)
			assert.Equal(t, status, response.Code)
			var entries []model.AuditLog
			require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Find(&entries).Error)
			categories := make(map[string]int)
			for _, entry := range entries {
				categories[entry.Category]++
				assert.Equal(t, status < 400, entry.Success)
				assert.Equal(t, common.RoleAdminUser, entry.ActorRole)
			}
			assert.Equal(t, 1, categories[model.AuditCategoryAccessToken])
			wantOperations := 0
			if tc.wantAudit {
				wantOperations = 1
			}
			assert.Equal(t, wantOperations, categories[model.AuditCategoryOperation], tc.action)
		}
	}
}
