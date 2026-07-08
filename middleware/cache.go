package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	immutableStaticCacheControl = "public, max-age=31536000, immutable"
	defaultWebCacheControl      = "max-age=604800"
)

func Cache() func(c *gin.Context) {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" {
			c.Header("Cache-Control", "no-cache")
		} else if strings.HasPrefix(path, "/static/") {
			c.Header("Cache-Control", immutableStaticCacheControl)
		} else {
			c.Header("Cache-Control", defaultWebCacheControl)
		}
		c.Header("Cache-Version", "b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14")
		c.Next()
	}
}
