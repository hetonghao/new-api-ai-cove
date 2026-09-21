package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func setModelQualityRouter(api *gin.RouterGroup) {
	group := api.Group("/model-quality", middleware.AdminAuth(), middleware.DisableCache())
	read := group.Group("", middleware.RequirePermission(authz.ChannelRead))
	read.GET("/capabilities", controller.GetQualityCapabilities)
	read.GET("/cases", controller.GetQualityCases)
	read.GET("/cases/:id", controller.GetQualityCase)
	read.GET("/cases/:id/revisions", controller.GetQualityCaseRevisions)
	read.GET("/cases/:id/dashboard", controller.GetQualityDashboard)
	read.GET("/cases/:id/samples", controller.GetQualitySamples)
	read.GET("/cases/:id/export", controller.GetQualityExport)
	read.GET("/runs", controller.GetQualityRuns)
	read.GET("/runs/:id", controller.GetQualityRun)
	read.GET("/samples/:id/artifact", controller.GetQualityArtifact)
	operate := group.Group("", middleware.RequirePermission(authz.ChannelOperate))
	operate.POST("/cases", controller.CreateQualityCase)
	operate.PUT("/cases/:id", controller.UpdateQualityCase)
	operate.DELETE("/cases/:id", controller.DeleteQualityCase)
	operate.PUT("/cases/:id/state", controller.SetQualityCaseState)
	operate.POST("/cases/:id/runs", controller.CreateQualityRun)
	operate.POST("/runs/:id/cancel", controller.CancelQualityRun)
	operate.PUT("/samples/:id/annotation", controller.AnnotateQualitySample)
	root := group.Group("", middleware.RootAuth())
	root.GET("/settings", controller.GetQualitySettings)
	root.PUT("/settings", controller.UpdateQualitySettings)
}
