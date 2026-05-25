package controller

import (
	"bytes"
	"encoding/json"
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

type salesAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func setupSalesControllerTestDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.QuotaData{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func newSalesControllerContext(t *testing.T, method string, target string, body any, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var requestBody *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, requestBody)
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set("id", userID)
	return ctx, recorder
}

func decodeSalesAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder) salesAPIResponse {
	t.Helper()

	require.Equal(t, http.StatusOK, recorder.Code)
	var response salesAPIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestGetSalesUsersReturnsOnlyInvitedUsers(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "seller", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ctrl-sales-1"},
		{Id: 2, Username: "alice", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 1, AffCode: "ctrl-sales-2"},
		{Id: 3, Username: "mallory", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 99, AffCode: "ctrl-sales-3"},
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodGet, "/api/sales/users?p=1&page_size=20", nil, 1)

	GetSalesUsers(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success)
	var page common.PageInfo
	require.NoError(t, json.Unmarshal(response.Data, &page))
	require.Equal(t, 1, page.Total)
}

func TestUpdateSalesUserGroupRejectsUsersOutsideInvites(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "seller", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ctrl-group-1"},
		{Id: 2, Username: "mallory", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 99, AffCode: "ctrl-group-2"},
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodPatch, "/api/sales/users/2/group", gin.H{"group": "vip"}, 1)
	ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	UpdateSalesUserGroup(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.False(t, response.Success)

	var user model.User
	require.NoError(t, db.First(&user, 2).Error)
	require.Equal(t, "default", user.Group)
}

func TestManageUserCanPromoteCommonUserToSales(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       2,
		Username: "alice",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "ctrl-promote-2",
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodPost, "/api/user/manage", ManageRequest{Id: 2, Action: "promote_sales"}, common.RoleAdminUser)
	ctx.Set("role", common.RoleAdminUser)

	ManageUser(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var user model.User
	require.NoError(t, db.First(&user, 2).Error)
	require.Equal(t, common.RoleSalesUser, user.Role)
}

func TestManageUserRejectsUnknownAction(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       2,
		Username: "alice",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "ctrl-unknown-action-2",
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodPost, "/api/user/manage", ManageRequest{Id: 2, Action: "promote_sales_typo"}, common.RoleAdminUser)
	ctx.Set("role", common.RoleAdminUser)

	ManageUser(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.False(t, response.Success)

	var user model.User
	require.NoError(t, db.First(&user, 2).Error)
	require.Equal(t, common.RoleCommonUser, user.Role)
}
