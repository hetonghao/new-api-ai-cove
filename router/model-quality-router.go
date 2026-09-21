package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

// requireQualityPanelAccess allows operators (ChannelRead permission) through,
// and any authenticated user when the public test panel is enabled.
func requireQualityPanelAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetInt("role")
		userID := c.GetInt("id")
		if authz.Can(userID, role, authz.ChannelRead) {
			c.Next()
			return
		}
		settings, _, err := model.GetQualitySettings()
		if err == nil && settings.PublicPanel {
			c.Next()
			return
		}
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
		})
		c.Abort()
	}
}

func setModelQualityRouter(api *gin.RouterGroup) {
	group := api.Group("/model-quality", middleware.UserAuth(), middleware.DisableCache())
	// Capabilities stay available to every authenticated user so the sidebar
	// can decide whether the entry is visible.
	group.GET("/capabilities", controller.GetQualityCapabilities)
	read := group.Group("", requireQualityPanelAccess())
	read.GET("/cases", controller.GetQualityCases)
	read.GET("/cases/:id/dashboard", controller.GetQualityDashboard)
	read.GET("/cases/:id/samples", controller.GetQualitySamples)
	read.GET("/samples/:id/artifact", controller.GetQualityArtifact)
	admin := group.Group("", middleware.AdminAuth(), middleware.RequirePermission(authz.ChannelRead))
	admin.GET("/cases/:id", controller.GetQualityCase)
	admin.GET("/cases/:id/revisions", controller.GetQualityCaseRevisions)
	admin.GET("/cases/:id/export", controller.GetQualityExport)
	admin.GET("/runs", controller.GetQualityRuns)
	admin.GET("/runs/:id", controller.GetQualityRun)
	operate := group.Group("", middleware.AdminAuth(), middleware.RequirePermission(authz.ChannelOperate))
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
