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

func TestDeviMonAccountActionsRequireRootAndValidTarget(t *testing.T) {
	db := setupTransportAckRouteTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.UserSession{}, &model.AuditLog{}))
	engine := gin.New()
	registerDeviMonRoutes(engine.Group("/api"))
	request := func(method, endpoint, token, body string) int {
		t.Helper()
		req := httptest.NewRequest(method, "/api/devin-monitor/accounts/"+endpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}
	t.Setenv("CPA_MANAGEMENT_KEY", "")
	routes := map[string]string{"refresh": http.MethodPost, "reset": http.MethodPost, "auto-reset": http.MethodPut, "devin-routing": http.MethodPut}
	require.Equal(t, http.StatusUnauthorized, request(http.MethodGet, "devin", "", ""))
	for endpoint, method := range routes {
		require.Equal(t, http.StatusUnauthorized, request(method, endpoint, "", `{"email":"ops@example.test"}`))
	}
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
			require.Equal(t, http.StatusForbidden, request(http.MethodGet, "devin", session.AccessToken, ""))
			for endpoint, method := range routes {
				require.Equal(t, http.StatusForbidden, request(method, endpoint, session.AccessToken, `{"email":"ops@example.test"}`))
			}
			continue
		}
		prefix := `{"email":"ops@example.test","reset_key":"` + strings.Repeat("a", 64) + `",`
		for _, fields := range []string{`"exhaustion_action":"typo"`, `"exhaustion_action":"silent","enabled":true`, `"exhaustion_action":"silent","enable_after_reset":true`} {
			require.Equal(t, http.StatusBadRequest, request(http.MethodPut, "auto-reset", session.AccessToken, prefix+fields+"}"))
		}
		for _, body := range []string{`{"email":"  "}`, "{}{}", strings.Repeat(" ", 4097)} {
			for endpoint, method := range routes {
				require.Equal(t, http.StatusBadRequest, request(method, endpoint, session.AccessToken, body))
			}
		}
	}
}
