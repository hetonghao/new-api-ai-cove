package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDeviMonAccountRefreshRequiresRootAndValidTarget(t *testing.T) {
	db := setupTransportAckRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.UserSession{}))
	engine := gin.New()
	registerDeviMonRoutes(engine.Group("/api"))
	request := func(token, body string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/devin-monitor/accounts/refresh", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}
	t.Setenv("CPA_MANAGEMENT_KEY", "")
	require.Equal(t, http.StatusUnauthorized, request("", `{"email":"ops@example.test"}`))
	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		name := "admin"
		if role == common.RoleRootUser {
			name = "root"
		}
		u := model.User{Username: name, AffCode: name, Password: "fixture", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
		require.NoError(t, db.Create(&u).Error)
		session, err := service.CreateLoginSession(u.Id, "password", "127.0.0.1", "monitor-test")
		require.NoError(t, err)
		if role != common.RoleRootUser {
			require.Equal(t, http.StatusForbidden, request(session.AccessToken, `{"email":"ops@example.test"}`))
			continue
		}
		for _, body := range []string{`{"email":"  "}`, "{}{}", strings.Repeat(" ", 4097)} {
			require.Equal(t, http.StatusBadRequest, request(session.AccessToken, body))
		}
	}
}
