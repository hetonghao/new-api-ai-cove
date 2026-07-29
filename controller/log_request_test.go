package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetLogByRequestID_returns_exact_log_without_pagination(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Channel{}))
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})
	logs := make([]model.Log, 0, 101)
	for index := 0; index < 100; index++ {
		logs = append(logs, model.Log{RequestId: fmt.Sprintf("other-%d", index), CreatedAt: time.Now().Unix()})
	}
	logs = append(logs, model.Log{RequestId: "target-request", CreatedAt: time.Now().Unix(), ModelName: "gpt-target"})
	logs = append(logs, model.Log{RequestId: "target-request-extra", CreatedAt: time.Now().Add(time.Hour).Unix(), ModelName: "wrong-prefix-match"})
	require.NoError(t, db.Create(&logs).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/request/target-request", nil)
	ctx.Params = gin.Params{{Key: "request_id", Value: "target-request"}}

	// When
	GetLogByRequestID(ctx)

	// Then
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool       `json:"success"`
		Data    *model.Log `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotNil(t, response.Data)
	require.Equal(t, "target-request", response.Data.RequestId)
	require.Equal(t, "gpt-target", response.Data.ModelName)
}

func TestGetLogByRequestID_returns_null_when_missing(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	originalLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open("file:missing_request_log?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	t.Cleanup(func() { model.LOG_DB = originalLogDB })
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/request/missing", nil)
	ctx.Params = gin.Params{{Key: "request_id", Value: "missing"}}

	// When
	GetLogByRequestID(ctx)

	// Then
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool       `json:"success"`
		Data    *model.Log `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Nil(t, response.Data)
}
