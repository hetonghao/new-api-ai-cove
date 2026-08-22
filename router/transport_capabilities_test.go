package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTransportCapabilitiesRouteRequiresTokenAuth(t *testing.T) {
	setupTransportAckRouteTestDB(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, constant.TransportCapabilitiesPath+"?models=gpt-5", nil)
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}
