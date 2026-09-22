package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestModelQualityRoutesRejectUnauthenticatedAndCookieOnlyRequests(t *testing.T) {
	engine := gin.New()
	setModelQualityRouter(engine.Group("/api"))
	for _, route := range []struct{ method, path string }{
		{"GET", "/capabilities"},
		{"GET", "/cases"},
		{"GET", "/cases/1"},
		{"GET", "/cases/1/revisions"},
		{"GET", "/cases/1/dashboard"},
		{"GET", "/cases/1/samples?channel_id=7"},
		{"GET", "/cases/1/export?channel_id=7"},
		{"GET", "/runs"},
		{"GET", "/runs/fixture"},
		{"GET", "/samples/1/artifact"},
		{"POST", "/cases"},
		{"PUT", "/cases/order"},
		{"PUT", "/cases/1"},
		{"DELETE", "/cases/1"},
		{"PUT", "/cases/1/state"},
		{"POST", "/cases/1/runs"},
		{"POST", "/runs/fixture/cancel"},
		{"PUT", "/samples/1/annotation"},
		{"GET", "/settings"},
		{"PUT", "/settings"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			for _, cookie := range []string{"", "session=fixture"} {
				request := httptest.NewRequest(route.method, "/api/model-quality"+route.path, strings.NewReader(`{}`))
				request.Header.Set("Cookie", cookie)
				request.Header.Set("Origin", "https://untrusted.example")
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				assert.Equal(t, http.StatusUnauthorized, recorder.Code)
				assert.Contains(t, recorder.Body.String(), `"success":false`)
			}
		})
	}
}
