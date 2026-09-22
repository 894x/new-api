package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	miniMaxVideoV2Router := router.Group("/v2")
	miniMaxVideoV2Router.Use(middleware.RouteTag("relay"))
	miniMaxVideoV2Router.Use(middleware.MiniMaxVideoV2RequestConvert(), middleware.TokenAuth(), middleware.AssetLibraryRouting(), middleware.Distribute())
	{
		miniMaxVideoV2Router.POST("/video_generation", controller.RelayTask)
		miniMaxVideoV2Router.GET("/query/video_generation/:task_id", controller.RelayTaskFetch)
	}

	aliVideoRouter := router.Group("/api/v1")
	aliVideoRouter.Use(middleware.RouteTag("relay"))
	aliVideoRouter.Use(middleware.AliVideoRequestConvert(), middleware.TokenAuth(), middleware.AssetLibraryRouting(), middleware.Distribute())
	{
		aliVideoRouter.POST("/services/aigc/video-generation/video-synthesis", controller.RelayTask)
		aliVideoRouter.GET("/tasks/:task_id", controller.RelayTaskFetch)
	}

	doubaoVideoRouter := router.Group("/api/v3/contents/generations")
	doubaoVideoRouter.Use(middleware.RouteTag("relay"))
	doubaoVideoRouter.Use(middleware.TokenAuth(), middleware.DoubaoVideoRequestConvert(), middleware.AssetLibraryRouting(), middleware.Distribute())
	{
		doubaoVideoRouter.POST("/tasks", controller.RelayTask)
		doubaoVideoRouter.GET("/tasks/:task_id", controller.RelayTaskFetch)
	}

	videoSharedRouter := router.Group("/v1")
	videoSharedRouter.Use(middleware.RouteTag("relay"))
	videoSharedRouter.Use(middleware.TokenAuth())
	videoSharedRouter.Use(middleware.SystemPerformanceCheck())
	videoSharedRouter.POST(
		"/video/generations",
		middleware.PinTaskPluginEndpoint(),
		middleware.TaskPluginEndpointOnly(middleware.ModelRequestRateLimit()),
		middleware.AssetLibraryRouting(),
		middleware.PrepareTaskPluginEndpoint(),
		middleware.Distribute(),
		func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, controller.RelayTask)
		},
	)

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.AssetLibraryRouting(), middleware.Distribute())
	{
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
}
