package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGlobalAPIRateLimitTestEngine(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 1
	common.GlobalApiRateLimitDuration = 180

	engine := gin.New()
	engine.Use(GlobalAPIRateLimit())
	engine.GET("/api/user/self", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.GET("/api/user/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.GET("/api/log/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.GET("/api/log/self", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.GET("/api/log/stat", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.GET("/api/data/self", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.GET("/api/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.POST("/api/user/checkin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	return engine
}

func TestGlobalAPIRateLimitSkipsUserSelfAndLogRoutes(t *testing.T) {
	engine := newGlobalAPIRateLimitTestEngine(t)

	for _, path := range []string{"/api/user/self", "/api/user/models", "/api/data/self", "/api/log/", "/api/log/stat", "/api/log/self"} {
		first := httptest.NewRecorder()
		firstRequest := httptest.NewRequest(http.MethodGet, path, nil)
		firstRequest.RemoteAddr = "203.0.113.9:3456"
		engine.ServeHTTP(first, firstRequest)
		require.Equal(t, http.StatusOK, first.Code)

		second := httptest.NewRecorder()
		secondRequest := httptest.NewRequest(http.MethodGet, path, nil)
		secondRequest.RemoteAddr = "203.0.113.9:3456"
		engine.ServeHTTP(second, secondRequest)
		require.Equal(t, http.StatusOK, second.Code)
	}
}

func TestGlobalAPIRateLimitStillAppliesToRegularRoutes(t *testing.T) {
	engine := newGlobalAPIRateLimitTestEngine(t)

	first := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	firstRequest.RemoteAddr = "198.51.100.7:7890"
	engine.ServeHTTP(first, firstRequest)
	require.Equal(t, http.StatusOK, first.Code)

	second := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	secondRequest.RemoteAddr = "198.51.100.7:7890"
	engine.ServeHTTP(second, secondRequest)
	require.Equal(t, http.StatusTooManyRequests, second.Code)
}

func TestGlobalAPIRateLimitStillAppliesToNonGetCheckinRequests(t *testing.T) {
	engine := newGlobalAPIRateLimitTestEngine(t)

	first := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, "/api/user/checkin", nil)
	firstRequest.RemoteAddr = "198.51.100.17:9000"
	engine.ServeHTTP(first, firstRequest)
	require.Equal(t, http.StatusOK, first.Code)

	second := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, "/api/user/checkin", nil)
	secondRequest.RemoteAddr = "198.51.100.17:9000"
	engine.ServeHTTP(second, secondRequest)
	require.Equal(t, http.StatusTooManyRequests, second.Code)
}
