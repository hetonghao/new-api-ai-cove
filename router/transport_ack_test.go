package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTransportAckRouteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainType, previousLogType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestTransportAckRouteRequiresTokenAuthAndSkipsPersistence(t *testing.T) {
	db := setupTransportAckRouteTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id: 5001, Username: "transport-ack-user", Password: "password",
		Status: common.UserStatusEnabled, Group: "default", Quota: 1000,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId: 5001, Key: "transportackkey", Status: common.TokenStatusEnabled,
		RemainQuota: 1000, Group: "", ExpiredTime: -1,
	}).Error)
	engine := gin.New()
	SetRelayRouter(engine)
	payload := `{"kind":"transport-ack","secret":"do-not-persist"}`

	unauthenticated := httptest.NewRecorder()
	unauthenticatedRequest := httptest.NewRequest(http.MethodPost, constant.TransportAckPath, strings.NewReader(payload))
	engine.ServeHTTP(unauthenticated, unauthenticatedRequest)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, constant.TransportAckPath, strings.NewReader(payload))
	request.Header.Set("Authorization", "Bearer sk-transportackkey")
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "do-not-persist")
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Count(&logCount).Error)
	require.Zero(t, logCount)
}

func TestTransportAckWebSocketRouteRequiresTokenAuth(t *testing.T) {
	setupTransportAckRouteTestDB(t)
	server := httptest.NewServer(func() http.Handler {
		engine := gin.New()
		SetRelayRouter(engine)
		return engine
	}())
	defer server.Close()

	_, response, err := (&websocket.Dialer{}).Dial("ws"+strings.TrimPrefix(server.URL, "http")+constant.TransportAckPath, nil)
	require.Error(t, err)
	require.NotNil(t, response)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
}
