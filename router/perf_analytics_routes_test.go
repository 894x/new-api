package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPerformanceAnalyticsRoutesExposeSeparateSelfAndAdminScopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	_, hasSelfRoute := routes[http.MethodGet+" /api/perf-analytics/self"]
	_, hasAdminRoute := routes[http.MethodGet+" /api/perf-analytics/admin"]
	_, hasSelfOptionsRoute := routes[http.MethodGet+" /api/perf-analytics/self/options"]
	_, hasAdminOptionsRoute := routes[http.MethodGet+" /api/perf-analytics/admin/options"]
	assert.True(t, hasSelfRoute)
	assert.True(t, hasAdminRoute)
	assert.True(t, hasSelfOptionsRoute)
	assert.True(t, hasAdminOptionsRoute)
}
