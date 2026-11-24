package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestImageCompressionCacheAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB, oldRedis := model.DB, model.LOG_DB, common.RedisEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldNode, oldOptionMap := common.NodeName, common.OptionMap
	oldSettings := operation_setting.GetImageCompressionCacheSettings()
	databaseType := common.DatabaseTypeSQLite
	fixture := sqlite.Open(t.TempDir() + "/cache.db")
	if dsn := os.Getenv("IMAGE_CACHE_TEST_MYSQL_DSN"); dsn != "" {
		databaseType, fixture = common.DatabaseTypeMySQL, mysql.Open(dsn)
	}
	if dsn := os.Getenv("IMAGE_CACHE_TEST_POSTGRES_DSN"); dsn != "" {
		databaseType, fixture = common.DatabaseTypePostgreSQL, postgres.Open(dsn)
	}
	db, err := gorm.Open(fixture, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.NodeName, common.OptionMap = oldNode, oldOptionMap
		require.NoError(t, operation_setting.SetImageCompressionCacheSettings(oldSettings))
	})
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	var version string
	versionQuery := "SELECT version()"
	if databaseType == common.DatabaseTypeSQLite {
		versionQuery = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("fixture database: %s %s", databaseType, version)
	common.SetDatabaseTypes(databaseType, databaseType)
	require.NoError(t, operation_setting.SetImageCompressionCacheSettings(operation_setting.ImageCompressionCacheSettings{MaxBytes: 64 << 20, TTLSeconds: 900}))
	common.NodeName, common.OptionMap = "cache-test-node", make(map[string]string)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.SystemTask{}, &model.User{}, &model.UserSession{}, &model.AuditLog{}, &model.SystemInstance{}))
	fixtureID := common.GetUUID()[:6]
	root := model.User{Username: "root-" + fixtureID, Password: "password", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "r-" + fixtureID}
	admin := model.User{Username: "admin-" + fixtureID, Password: "password", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "a-" + fixtureID}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&admin).Error)
	rootSession, err := service.CreateLoginSession(root.Id, "password", "127.0.0.1", "cache-route-test")
	require.NoError(t, err)
	adminSession, err := service.CreateLoginSession(admin.Id, "password", "127.0.0.1", "cache-route-test")
	require.NoError(t, err)
	engine := gin.New()
	SetApiRouter(engine)

	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/config"}, {http.MethodPut, "/config"},
		{http.MethodGet, "/stats"}, {http.MethodPost, "/clear"}, {http.MethodGet, "/nodes"},
	} {
		t.Run("RootOnly"+endpoint.method+endpoint.path, func(t *testing.T) {
			require.Equal(t, http.StatusUnauthorized, imageCacheAPIRequest(engine, endpoint.method, endpoint.path, "", nil).Code)
			require.Equal(t, http.StatusForbidden, imageCacheAPIRequest(engine, endpoint.method, endpoint.path, adminSession.AccessToken, nil).Code)
		})
	}
	const changed = `{"enabled":true,"max_bytes":2097152,"ttl_seconds":120}`
	t.Run("Configuration", func(t *testing.T) {
		response := imageCacheAPIRequest(engine, http.MethodGet, "/config", rootSession.AccessToken, nil)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.JSONEq(t, `{"success":true,"message":"","data":{"enabled":false,"max_bytes":67108864,"ttl_seconds":900}}`, response.Body.String())

		response = imageCacheAPIRequest(engine, http.MethodPut, "/config", rootSession.AccessToken, strings.NewReader(changed))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.JSONEq(t, `{"success":true,"message":"","data":`+changed+`}`, response.Body.String())

		for _, invalid := range []string{
			`{"max_bytes":2097152,"ttl_seconds":120}`,
			`{"enabled":true,"max_bytes":1048575,"ttl_seconds":120}`,
			`{"enabled":true,"max_bytes":268435457,"ttl_seconds":120}`,
			`{"enabled":true,"max_bytes":2097152,"ttl_seconds":59}`,
			`{"enabled":true,"max_bytes":2097152,"ttl_seconds":3601}`,
		} {
			response = imageCacheAPIRequest(engine, http.MethodPut, "/config", rootSession.AccessToken, strings.NewReader(invalid))
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		}
		require.NoError(t, operation_setting.SetImageCompressionCacheSettings(operation_setting.ImageCompressionCacheSettings{MaxBytes: 64 << 20, TTLSeconds: 900}))
		require.NoError(t, model.LoadOptionsFromDatabase())
		response = imageCacheAPIRequest(engine, http.MethodGet, "/config", rootSession.AccessToken, nil)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.JSONEq(t, `{"success":true,"message":"","data":`+changed+`}`, response.Body.String())
	})
	t.Run("DedicatedConfigurationEndpoint", func(t *testing.T) {
		body, err := common.Marshal(model.Option{Key: operation_setting.ImageCompressionCacheOptionKey, Value: changed})
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(string(body)))
		request.Header.Set("Authorization", "Bearer "+rootSession.AccessToken)
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	})

	t.Run("NodeAndProcessScope", func(t *testing.T) {
		require.NoError(t, model.UpsertSystemInstance("cache-peer", nil, 1, common.GetTimestamp()))
		response := imageCacheAPIRequest(engine, http.MethodGet, "/nodes", rootSession.AccessToken, nil)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.JSONEq(t, `{"success":true,"message":"","data":{"local_node_name":"cache-test-node","nodes":[{"node_name":"cache-test-node","status":"online"},{"node_name":"cache-peer","status":"unavailable"}]}}`, response.Body.String())

		response = imageCacheAPIRequest(engine, http.MethodGet, "/stats?page=2&page_size=7&node_name=cache-test-node", rootSession.AccessToken, nil)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Contains(t, response.Header().Get("Cache-Control"), "no-store")
		var stats struct {
			Data struct {
				InstanceID string   `json:"instance_id"`
				NodeName   string   `json:"node_name"`
				Page       int      `json:"page"`
				PageSize   int      `json:"page_size"`
				Entries    []string `json:"entries"`
			} `json:"data"`
		}
		require.NoError(t, common.DecodeJson(response.Body, &stats))
		require.Equal(t, "cache-test-node", stats.Data.NodeName)
		require.NotEmpty(t, stats.Data.InstanceID)
		require.Equal(t, 2, stats.Data.Page)
		require.Equal(t, 7, stats.Data.PageSize)
		require.NotNil(t, stats.Data.Entries)
		require.Equal(t, http.StatusBadRequest, imageCacheAPIRequest(engine, http.MethodGet, "/stats?page_size=101", rootSession.AccessToken, nil).Code)
		require.Equal(t, http.StatusNotFound, imageCacheAPIRequest(engine, http.MethodGet, "/stats?node_name=unknown-node", rootSession.AccessToken, nil).Code)
		require.Equal(t, http.StatusServiceUnavailable, imageCacheAPIRequest(engine, http.MethodGet, "/stats?node_name=cache-peer", rootSession.AccessToken, nil).Code)

		for _, removal := range []struct {
			body   string
			status int
		}{
			{`{"node_name":"cache-test-node","instance_id":"stale-process","all":true}`, http.StatusConflict},
			{`{"node_name":"cache-test-node","instance_id":"` + stats.Data.InstanceID + `","ids":["a"],"all":true}`, http.StatusBadRequest},
			{`{"node_name":"cache-test-node","instance_id":"` + stats.Data.InstanceID + `","ids":["a","a"]}`, http.StatusBadRequest},
			{`{"node_name":"cache-test-node","instance_id":"` + stats.Data.InstanceID + `","ids":["already-gone"]}`, http.StatusOK},
			{`{"node_name":"cache-test-node","instance_id":"` + stats.Data.InstanceID + `","all":true}`, http.StatusOK},
		} {
			response := imageCacheAPIRequest(engine, http.MethodPost, "/clear", rootSession.AccessToken, strings.NewReader(removal.body))
			require.Equal(t, removal.status, response.Code, response.Body.String())
			if removal.status == http.StatusOK {
				require.JSONEq(t, `{"success":true,"message":"","data":{"deleted":0}}`, response.Body.String())
			}
		}
	})
}

func imageCacheAPIRequest(engine http.Handler, method, path, bearer string, body io.Reader) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/api/image-compression-cache"+path, body)
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	engine.ServeHTTP(recorder, request)
	return recorder
}
