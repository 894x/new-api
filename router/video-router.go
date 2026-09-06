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
	doubaoVideoRouter.Use(middleware.DoubaoVideoRequestConvert(), middleware.TokenAuth(), middleware.AssetLibraryRouting(), middleware.Distribute())
	{
		doubaoVideoRouter.POST("/tasks", controller.RelayTask)
		doubaoVideoRouter.GET("/tasks/:task_id", controller.RelayTaskFetch)
	}

	// Video proxy: accepts either session auth (dashboard) or token auth (API clients)
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(middleware.TokenOrUserAuth())
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy)
	}

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.AssetLibraryRouting(), middleware.Distribute())
	{
		videoV1Router.POST("/video/generations", controller.RelayTask)
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
	// openai compatible API video routes
	// docs: https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", controller.RelayTask)
		videoV1Router.GET("/videos/:task_id", controller.RelayTaskFetch)
	}

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		klingV1Router.POST("/videos/text2video", controller.RelayTask)
		klingV1Router.POST("/videos/image2video", controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTaskFetch)
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.RelayTask)
	}
}
