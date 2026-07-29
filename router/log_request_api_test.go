package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogRequestAPI_returnsExactLogForAdmin(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	accessToken := "log-request-admin-token"
	admin := &model.User{
		Username: "admin", Password: "password", Role: common.RoleAdminUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "log-request-admin",
	}
	admin.SetAccessToken(accessToken)
	require.NoError(t, db.Create(admin).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{RequestId: "target-request-extra", ModelName: "wrong-prefix-match"},
		{RequestId: "target-request", ModelName: "gpt-target"},
	}).Error)
	engine := gin.New()
	SetApiRouter(engine)
	server := httptest.NewServer(engine)
	t.Cleanup(func() {
		server.Close()
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/log/request/target-request", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// When
	response, err := server.Client().Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var payload struct {
		Success bool       `json:"success"`
		Data    *model.Log `json:"data"`
	}
	require.NoError(t, common.DecodeJson(response.Body, &payload))
	require.True(t, payload.Success)
	require.NotNil(t, payload.Data)
	require.Equal(t, "target-request", payload.Data.RequestId)
	require.Equal(t, "gpt-target", payload.Data.ModelName)
}
