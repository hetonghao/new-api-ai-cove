package controller

import (
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

type getSelfAPIResponse struct {
	Success bool `json:"success"`
	Data    struct {
		HasPassword bool `json:"has_password"`
	} `json:"data"`
}

func setupGetSelfControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestGetSelfIncludesHasPassword(t *testing.T) {
	db := setupGetSelfControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       1001,
		Username: "with-password",
		Password: "hashed-value",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "A1001",
	}).Error)
	require.NoError(t, db.Create(&model.User{
		Id:       1002,
		Username: "without-password",
		Password: "",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "A1002",
	}).Error)

	t.Run("returns true for user with password", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
		ctx.Set("id", 1001)
		ctx.Set("role", common.RoleCommonUser)

		GetSelf(ctx)

		require.Equal(t, http.StatusOK, recorder.Code)
		var payload getSelfAPIResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.True(t, payload.Success)
		require.True(t, payload.Data.HasPassword)
	})

	t.Run("returns false for user without password", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
		ctx.Set("id", 1002)
		ctx.Set("role", common.RoleCommonUser)

		GetSelf(ctx)

		require.Equal(t, http.StatusOK, recorder.Code)
		var payload getSelfAPIResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.True(t, payload.Success)
		require.False(t, payload.Data.HasPassword)
	})
}

func TestCheckUpdatePasswordAllowsFirstPasswordSetup(t *testing.T) {
	db := setupGetSelfControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       2001,
		Username: "oauth-user",
		Password: "",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "B2001",
	}).Error)

	updatePassword, err := checkUpdatePassword("", "new-pass-123", 2001)
	require.NoError(t, err)
	require.True(t, updatePassword)
}

func TestCheckUpdatePasswordRequiresCurrentPasswordWhenPresent(t *testing.T) {
	db := setupGetSelfControllerTestDB(t)
	hashedPassword, err := common.Password2Hash("old-pass-123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.User{
		Id:       2002,
		Username: "password-user",
		Password: hashedPassword,
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "B2002",
	}).Error)

	updatePassword, err := checkUpdatePassword("", "new-pass-123", 2002)
	require.Error(t, err)
	require.False(t, updatePassword)
}
