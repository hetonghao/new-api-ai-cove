package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRiskPolicyRoutes_require_root_auth(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("risk-route-test"))))
	registerRiskPolicyRoutes(engine.Group("/api"))
	request := httptest.NewRequest(http.MethodGet, "/api/risk/policy", nil)
	recorder := httptest.NewRecorder()

	// When
	engine.ServeHTTP(recorder, request)

	// Then
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}
