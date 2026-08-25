package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func registerAssetLibraryAdminRoutes(apiRouter *gin.RouterGroup) {
	assetLibraryRoute := apiRouter.Group("/asset-library/admin")
	assetLibraryRoute.Use(middleware.AdminAuth())
	for _, route := range assetLibraryAdminPermissionRoutes {
		assetLibraryRoute.Handle(
			route.method,
			route.path,
			middleware.RequirePermission(route.permission),
			route.handler,
		)
	}
}

var assetLibraryAdminPermissionRoutes = []permissionRoute{
	{method: http.MethodGet, path: "/assets/:id/replicas", permission: authz.ChannelRead, handler: controller.GetAdminAssetReplicaDetails},
	{method: http.MethodPost, path: "/assets/:id/sync", permission: authz.ChannelSensitiveWrite, handler: controller.SyncAdminAssetReplicas},
	{method: http.MethodGet, path: "/groups/:id/replicas", permission: authz.ChannelRead, handler: controller.GetAdminAssetGroupReplicaDetails},
	{method: http.MethodPost, path: "/groups/:id/sync", permission: authz.ChannelSensitiveWrite, handler: controller.SyncAdminAssetGroupReplicas},
}
