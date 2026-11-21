package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
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
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.QuotaData{}, &model.TopUp{}, &model.Log{}, &model.SalesCommissionSettlement{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
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

func TestGetSalesStatsIncludesCommissionSummary(t *testing.T) {
	db := setupSalesControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "seller", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "ctrl-commission-1", CommissionRatio: 10},
		{Id: 2, Username: "alice", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 1, AffCode: "ctrl-commission-2"},
	}).Error)
	require.NoError(t, db.Create(&model.TopUp{UserId: 2, Amount: 10000, Money: 100, Status: common.TopUpStatusSuccess, TradeNo: "ctrl-commission-topup"}).Error)

	ctx, recorder := newSalesControllerContext(t, http.MethodGet, "/api/sales/stats", nil, 1)

	GetSalesStats(ctx)

	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success)
	var data struct {
		TopUpAmount              float64 `json:"topup_amount"`
		CommissionRatio          float64 `json:"commission_ratio"`
		PendingCommissionAmount  float64 `json:"pending_commission_amount"`
		TotalCommissionAmount    float64 `json:"total_commission_amount"`
		SettledCommissionAmount  float64 `json:"settled_commission_amount"`
		PendingCommissionRevenue float64 `json:"pending_commission_revenue"`
	}
	require.NoError(t, json.Unmarshal(response.Data, &data))
	require.InDelta(t, 100, data.TopUpAmount, 0.0001)
	require.InDelta(t, 10, data.CommissionRatio, 0.0001)
	require.InDelta(t, 10, data.PendingCommissionAmount, 0.0001)
	require.InDelta(t, 10, data.TotalCommissionAmount, 0.0001)
	require.InDelta(t, 0, data.SettledCommissionAmount, 0.0001)
	require.InDelta(t, 100, data.PendingCommissionRevenue, 0.0001)
}

func TestCreateSalesCommissionSettlementByRootRecordsSettlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldQuotaPerUnit, oldRate := common.RedisEnabled, common.QuotaPerUnit, operation_setting.USDExchangeRate
	common.RedisEnabled, common.QuotaPerUnit, operation_setting.USDExchangeRate = false, 500_000, 7.3
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled, common.QuotaPerUnit, operation_setting.USDExchangeRate = oldRedis, oldQuotaPerUnit, oldRate
	})

	for _, dialect := range []common.DatabaseType{common.DatabaseTypeSQLite, common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(dialect), func(t *testing.T) {
			dsn := os.Getenv("TEST_" + strings.ToUpper(string(dialect)) + "_DSN")
			if dialect != common.DatabaseTypeSQLite && dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			db, _ := newAuditTestDatabase(t, string(dialect), dsn)
			model.DB, model.LOG_DB = db, db
			common.SetDatabaseTypes(dialect, dialect)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.SalesCommissionSettlement{}, &model.Log{}))
			versionQuery := "SELECT version()"
			if dialect == common.DatabaseTypeSQLite {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)

			cases := []struct {
				name       string
				payload    gin.H
				before     int
				wantQuota  int
				wantCredit bool
				wantError  error
			}{
				{"offline", gin.H{"amount": 7.31, "note": "offline paid"}, 1234, 1234, false, nil},
				{"wallet", gin.H{"amount": 7.31, "note": "wallet payment", "credit_to_balance": true}, 1234, 501919, true, nil},
				{"wallet_limit", gin.H{"amount": 7.31, "credit_to_balance": true}, common.MaxWalletQuota - 500684, common.MaxWalletQuota - 500684, false, model.ErrWalletQuotaLimitExceeded},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					seller := model.User{Username: tc.name, AffCode: tc.name, Role: common.RoleSalesUser, CommissionRatio: 10, Quota: tc.before}
					require.NoError(t, db.Create(&seller).Error)
					invitee := model.User{Username: tc.name + "_buyer", AffCode: tc.name + "_buyer", InviterId: seller.Id}
					require.NoError(t, db.Create(&invitee).Error)
					require.NoError(t, db.Create(&model.TopUp{UserId: invitee.Id, Money: 100, Status: common.TopUpStatusSuccess, TradeNo: tc.name}).Error)
					if dialect == common.DatabaseTypeSQLite {
						server := miniredis.RunT(t)
						cache := redis.NewClient(&redis.Options{Addr: server.Addr()})
						previousCache := common.RDB
						common.RDB, common.RedisEnabled = cache, true
						t.Cleanup(func() {
							common.RDB, common.RedisEnabled = previousCache, false
							assert.NoError(t, cache.Close())
						})
						_, err := model.GetUserCache(seller.Id)
						require.NoError(t, err)
					}

					id := fmt.Sprint(seller.Id)
					ctx, recorder := newSalesControllerContext(t, http.MethodPost, "/api/sales/admin/commissions/"+id+"/settlements", tc.payload, 99)
					ctx.Set("role", common.RoleRootUser)
					ctx.Params = gin.Params{{Key: "sales_user_id", Value: id}}
					CreateSalesCommissionSettlementByRoot(ctx)

					response := decodeSalesAPIResponse(t, recorder)
					quota, err := model.GetUserQuota(seller.Id, true)
					require.NoError(t, err)
					assert.Equal(t, tc.wantQuota, quota)
					if common.RedisEnabled {
						cache, err := model.GetUserCache(seller.Id)
						require.NoError(t, err)
						assert.Equal(t, tc.wantQuota, cache.Quota)
					}
					var credits int64
					require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", seller.Id, model.LogTypeTopup).Count(&credits).Error)
					if tc.wantCredit {
						assert.EqualValues(t, 1, credits)
					} else {
						assert.Zero(t, credits)
					}
					var settlements []model.SalesCommissionSettlement
					require.NoError(t, db.Where("sales_user_id = ?", seller.Id).Find(&settlements).Error)
					if tc.wantError != nil {
						assert.False(t, response.Success)
						assert.Equal(t, tc.wantError.Error(), response.Message)
						assert.Empty(t, settlements)
						return
					}
					require.True(t, response.Success, response.Message)
					require.Len(t, settlements, 1)
					assert.Equal(t, 7.31, settlements[0].Amount)
					assert.Equal(t, tc.payload["note"], settlements[0].Note)
				})
			}
		})
	}
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
	_, identity, target := setupAdminUserTest(t)
	body := fmt.Sprintf(`{"id":%d,"action":"promote_sales"}`, target.Id)
	withoutProof := adminUserRequest(http.MethodPost, "/api/user/manage", body, "", identity, common.RoleRootUser, nil, ManageUser)
	require.Contains(t, withoutProof.Body.String(), "SECURITY_PROOF_REQUIRED")
	var unchanged model.User
	require.NoError(t, model.DB.First(&unchanged, target.Id).Error)
	require.Equal(t, common.RoleCommonUser, unchanged.Role)
	proof := issueSecurityEnrollmentProof(t, identity, service.VerificationOperation{
		Scope:   service.VerificationScopeAdminUserManage,
		Context: []byte(fmt.Sprintf(`{"user_id":%d,"action":"promote_sales"}`, target.Id)),
	}, service.VerificationMethodPassword)
	recorder := adminUserRequest(http.MethodPost, "/api/user/manage", body, proof, identity, common.RoleRootUser, nil, ManageUser)
	response := decodeSalesAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var user model.User
	require.NoError(t, model.DB.First(&user, target.Id).Error)
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
