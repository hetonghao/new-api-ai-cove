package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerRiskPolicyRoutes(apiRouter *gin.RouterGroup) {
	riskRoute := apiRouter.Group("/risk")
	riskRoute.Use(middleware.RootAuth())
	{
		riskRoute.GET("/policy", controller.GetRiskPolicy)
		riskRoute.PUT("/policy", controller.UpdateRiskPolicy)
		riskRoute.GET("/rules", controller.ListRiskRules)
		riskRoute.POST("/rules", controller.CreateRiskRule)
		riskRoute.POST("/rules/test", controller.TestRiskRule)
		riskRoute.PUT("/rules/:id", controller.UpdateRiskRule)
		riskRoute.DELETE("/rules/:id", controller.DeleteRiskRule)
		riskRoute.GET("/records", controller.ListRiskRecords)
		riskRoute.GET("/severe-records", controller.ListSevereRiskRecords)
		riskRoute.GET("/severe-records/:id", controller.GetSevereRiskRecord)
		riskRoute.GET("/records/settings", controller.GetRiskRecordGovernance)
		riskRoute.PUT("/records/settings", controller.UpdateRiskRecordGovernance)
		riskRoute.GET("/statistics", controller.GetRiskStatistics)
	}
}
