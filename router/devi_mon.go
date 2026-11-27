package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerDeviMonRoutes(apiRouter *gin.RouterGroup) {
	routes := apiRouter.Group("/devin-monitor", middleware.DisableCache(), middleware.RootAuth())
	routes.GET("", controller.GetDeviMonSnapshot)
	routes.GET("/accounts", controller.GetDeviMonAccounts)
	routes.PUT("/accounts/channel", controller.SetDeviMonAccountChannel)
	routes.POST("/accounts/chatgpt/session", middleware.UserCriticalRateLimit("devin-chatgpt-session"), controller.SaveDeviMonChatGPTSession)
	routes.DELETE("/accounts/chatgpt", controller.ForgetDeviMonChatGPTAccount)
	routes.POST("/collect", controller.RunDeviMonCollection)
	routes.POST("/accounts/refresh", middleware.UserCriticalRateLimit("devin-account-refresh"), controller.RefreshDeviMonAccount)
	routes.PUT("/accounts/auto-reset", controller.SetDeviMonAccountAutoReset)
	routes.POST("/accounts/reset", middleware.UserCriticalRateLimit("devin-account-reset"), controller.ConfirmDeviMonAccountReset)
	routes.GET("/config", controller.GetDeviMonSettingsHandler)
	routes.PUT("/config", controller.SetDeviMonSettingsHandler)
}
