package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerImageCompressionCacheRoutes(apiRouter *gin.RouterGroup) {
	routes := apiRouter.Group("/image-compression-cache", middleware.DisableCache(), middleware.RootAuth())
	routes.GET("/config", controller.GetImageCompressionCacheConfig)
	routes.PUT("/config", controller.SetImageCompressionCacheConfig)
	routes.GET("/stats", controller.GetImageCompressionCacheStats)
	routes.POST("/clear", controller.ClearImageCompressionCache)
}
