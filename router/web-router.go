package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

var webGzipExcludedExtensions = []string{
	".avif",
	".css",
	".gif",
	".ico",
	".jpeg",
	".jpg",
	".js",
	".map",
	".mjs",
	".otf",
	".png",
	".svg",
	".ttf",
	".webp",
	".woff",
	".woff2",
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatchers ...gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	handlers := []gin.HandlerFunc{}
	if len(pluginDispatchers) > 0 && pluginDispatchers[0] != nil {
		handlers = append(handlers, pluginDispatchers[0])
	}
	handlers = append(handlers,
		middleware.RouteTag("web"),
		gzip.Gzip(
			gzip.DefaultCompression,
			gzip.WithExcludedPaths([]string{"/downloads/"}),
			gzip.WithExcludedExtensions(webGzipExcludedExtensions),
		),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
		static.Serve("/", frontendFS),
		func(c *gin.Context) {
			if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
				controller.RelayNotFound(c)
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.IndexPage)
		},
	)
	router.NoRoute(handlers...)
}
