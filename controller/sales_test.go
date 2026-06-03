package controller

import (
	"bytes"
	"encoding/json"
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.QuotaData{}, &model.TopUp{}, &model.Log{}))

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
		{Id: 1, Username: "seller", Password: "seller-secret", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ctrl-sales-1"},
		{Id: 2, Username: "alice", Password: "alice-secret", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 1, AffCode: "ctrl-sales-2"},
		{Id: 3, Username: "mallory", Password: "mallory-secret", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 99, AffCode: "ctrl-sales-3"},
	}).Error)
	require.NoError(t, db.Create(&[]model.TopUp{
		{UserId: 2, Amount: 10000, Money: 10.5, Status: common.TopUpStatusSuccess, TradeNo: "ctrl-sales-topup-1"},
		{UserId: 2, Amount: 20000, Money: 20, Status: common.TopUpStatusPending, TradeNo: "ctrl-sales-topup-2"},
		{UserId: 1, Amount: 30000, Money: 30, Status: common.TopUpStatusSuccess, TradeNo: "ctrl-sales-topup-3"},
		{UserId: 3, Amount: 40000, Money: 40, Status: common.TopUpStatusSuccess, TradeNo: "ctrl-sales-topup-4"},
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodGet, "/api/sales/users?p=1&page_size=20", nil, 1)

	GetSalesUsers(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success)
	var page common.PageInfo
	require.NoError(t, json.Unmarshal(response.Data, &page))
	require.Equal(t, 1, page.Total)
	require.NotContains(t, recorder.Body.String(), "alice-secret")

	var data struct {
		Items []struct {
			Username    string  `json:"username"`
			TopUpAmount float64 `json:"topup_amount"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(response.Data, &data))
	require.Len(t, data.Items, 1)
	require.Equal(t, "alice", data.Items[0].Username)
	require.InDelta(t, 10.5, data.Items[0].TopUpAmount, 0.0001)
}

func TestGetSalesUserReturnsOnlyInvitedUser(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "seller", Password: "seller-secret", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ctrl-sales-user-1"},
		{Id: 2, Username: "alice", Password: "alice-secret", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 1, AffCode: "ctrl-sales-user-2", Remark: "admin-only note"},
		{Id: 3, Username: "mallory", Password: "mallory-secret", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 99, AffCode: "ctrl-sales-user-3"},
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodGet, "/api/sales/users/2", nil, 1)
	ctx.Params = gin.Params{{Key: "id", Value: "2"}}

	GetSalesUser(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success)
	require.NotContains(t, recorder.Body.String(), "alice-secret")
	require.NotContains(t, recorder.Body.String(), "remark")
	require.NotContains(t, recorder.Body.String(), "admin-only note")

	var user model.User
	require.NoError(t, json.Unmarshal(response.Data, &user))
	require.Equal(t, "alice", user.Username)

	ctx, recorder = newSalesControllerContext(t, http.MethodGet, "/api/sales/users/3", nil, 1)
	ctx.Params = gin.Params{{Key: "id", Value: "3"}}

	GetSalesUser(ctx)

	response = decodeSalesAPIResponse(t, recorder)
	require.False(t, response.Success)
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

func TestGetSalesStatsReturnsInvitedTopUpAmountOnly(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "seller", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ctrl-stats-1"},
		{Id: 2, Username: "alice", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 1, AffCode: "ctrl-stats-2"},
		{Id: 3, Username: "mallory", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 99, AffCode: "ctrl-stats-3"},
	}).Error)
	require.NoError(t, db.Create(&[]model.TopUp{
		{UserId: 2, Amount: 10000, Money: 10.5, Status: common.TopUpStatusSuccess, TradeNo: "ctrl-topup-1"},
		{UserId: 2, Amount: 20000, Money: 20, Status: common.TopUpStatusPending, TradeNo: "ctrl-topup-2"},
		{UserId: 1, Amount: 30000, Money: 30, Status: common.TopUpStatusSuccess, TradeNo: "ctrl-topup-3"},
		{UserId: 3, Amount: 40000, Money: 40, Status: common.TopUpStatusSuccess, TradeNo: "ctrl-topup-4"},
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodGet, "/api/sales/stats", nil, 1)

	GetSalesStats(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success)

	var data struct {
		TopUpAmount float64 `json:"topup_amount"`
	}
	require.NoError(t, json.Unmarshal(response.Data, &data))
	require.InDelta(t, 10.5, data.TopUpAmount, 0.0001)
}

func TestGetSalesLogsStatPassesThroughRequestFiltersAndOnlyCountsConsumeLogs(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "seller", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ctrl-logstat-1"},
		{Id: 2, Username: "alice", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 1, AffCode: "ctrl-logstat-2"},
	}).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 2, Username: "alice", Type: model.LogTypeConsume, CreatedAt: now - 3, ModelName: "gpt-5.4", TokenName: "invite-token", Group: "default", Quota: 20, PromptTokens: 30, CompletionTokens: 40, RequestId: "req-1", UpstreamRequestId: "up-1"},
		{UserId: 2, Username: "alice", Type: model.LogTypeConsume, CreatedAt: now - 2, ModelName: "gpt-5.4", TokenName: "invite-token", Group: "default", Quota: 50, PromptTokens: 60, CompletionTokens: 70, RequestId: "req-2", UpstreamRequestId: "up-2"},
		{UserId: 2, Username: "alice", Type: model.LogTypeManage, CreatedAt: now - 1, ModelName: "gpt-5.4", TokenName: "invite-token", Group: "default", Quota: 500, PromptTokens: 600, CompletionTokens: 700, RequestId: "req-1", UpstreamRequestId: "up-1"},
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodGet, "/api/sales/logs/stat?type=2&request_id=req-1&upstream_request_id=up-1", nil, 1)

	GetSalesLogsStat(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success)

	var data struct {
		Quota int `json:"quota"`
		Rpm   int `json:"rpm"`
		Tpm   int `json:"tpm"`
	}
	require.NoError(t, json.Unmarshal(response.Data, &data))
	require.Equal(t, 20, data.Quota)
	require.Equal(t, 1, data.Rpm)
	require.Equal(t, 70, data.Tpm)
}

func TestGetSalesLogsStatWithNonConsumeTypeReturnsEmptyStats(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "seller", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ctrl-logstat-type-1"},
		{Id: 2, Username: "alice", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 1, AffCode: "ctrl-logstat-type-2"},
	}).Error)
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 2, Username: "alice", Type: model.LogTypeConsume, CreatedAt: now - 3, ModelName: "gpt-5.4", TokenName: "invite-token", Group: "default", Quota: 20, PromptTokens: 30, CompletionTokens: 40},
		{UserId: 2, Username: "alice", Type: model.LogTypeError, CreatedAt: now - 2, ModelName: "gpt-5.4", TokenName: "invite-token", Group: "default", Quota: 900, PromptTokens: 901, CompletionTokens: 902},
	}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodGet, "/api/sales/logs/stat?type=1", nil, 1)

	GetSalesLogsStat(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success)

	var data struct {
		Quota int `json:"quota"`
		Rpm   int `json:"rpm"`
		Tpm   int `json:"tpm"`
	}
	require.NoError(t, json.Unmarshal(response.Data, &data))
	require.Equal(t, 0, data.Quota)
	require.Equal(t, 0, data.Rpm)
	require.Equal(t, 0, data.Tpm)
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
