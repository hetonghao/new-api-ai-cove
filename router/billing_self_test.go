package router

import (
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

type billingSelfAPIResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		BillingPreference     string `json:"billing_preference"`
		HasActiveSubscription bool   `json:"has_active_subscription"`
		Wallet                struct {
			RemainingQuota int `json:"remaining_quota"`
			UsedQuota      int `json:"used_quota"`
		} `json:"wallet"`
		Subscriptions []struct {
			Id              int    `json:"id"`
			PlanId          int    `json:"plan_id"`
			Status          string `json:"status"`
			Source          string `json:"source"`
			StartTime       int64  `json:"start_time"`
			EndTime         int64  `json:"end_time"`
			NextResetTime   int64  `json:"next_reset_time"`
			AmountTotal     int64  `json:"amount_total"`
			AmountUsed      int64  `json:"amount_used"`
			AmountRemaining *int64 `json:"amount_remaining"`
			Unlimited       bool   `json:"unlimited"`
		} `json:"subscriptions"`
	} `json:"data"`
}

type billingSelfV2APIResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Currency              string `json:"currency"`
		BillingPreference     string `json:"billing_preference"`
		HasActiveSubscription bool   `json:"has_active_subscription"`
		Wallet                struct {
			RemainingAmount float64 `json:"remaining_amount"`
			UsedAmount      float64 `json:"used_amount"`
		} `json:"wallet"`
		Subscriptions []struct {
			Id              int      `json:"id"`
			PlanId          int      `json:"plan_id"`
			Status          string   `json:"status"`
			Source          string   `json:"source"`
			StartTime       int64    `json:"start_time"`
			EndTime         int64    `json:"end_time"`
			NextResetTime   int64    `json:"next_reset_time"`
			AmountTotal     float64  `json:"amount_total"`
			AmountUsed      float64  `json:"amount_used"`
			AmountRemaining *float64 `json:"amount_remaining"`
			Unlimited       bool     `json:"unlimited"`
		} `json:"subscriptions"`
	} `json:"data"`
}

func setupBillingSelfRouteTestDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.UserSubscription{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func newBillingSelfTestEngine() *gin.Engine {
	engine := gin.New()
	SetApiRouter(engine)
	return engine
}

func decodeBillingSelfResponse(t *testing.T, recorder *httptest.ResponseRecorder) billingSelfAPIResponse {
	t.Helper()

	var response billingSelfAPIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func decodeBillingSelfV2Response(t *testing.T, recorder *httptest.ResponseRecorder) billingSelfV2APIResponse {
	t.Helper()

	var response billingSelfV2APIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestBillingSelfReturnsWalletAndActiveSubscriptionsForToken(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.User{
		Id:        5001,
		Username:  "billing-user",
		Password:  "hashed-password",
		Role:      common.RoleCommonUser,
		Status:    common.UserStatusEnabled,
		Group:     "default",
		Quota:     1200,
		UsedQuota: 340,
		Setting:   `{"billing_preference":"wallet_first"}`,
		AffCode:   "B5001",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5001,
		Key:            "billingkeyactive",
		Name:           "primary-key",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    -1,
		RemainQuota:    200,
		UsedQuota:      50,
		UnlimitedQuota: false,
		Group:          "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:            7001,
		UserId:        5001,
		PlanId:        9001,
		AmountTotal:   1000,
		AmountUsed:    240,
		StartTime:     now - 7200,
		EndTime:       now + 7200,
		Status:        "active",
		Source:        "order",
		NextResetTime: now + 3600,
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:          7002,
		UserId:      5001,
		PlanId:      9002,
		AmountTotal: 500,
		AmountUsed:  500,
		StartTime:   now - 14400,
		EndTime:     now - 60,
		Status:      "expired",
		Source:      "order",
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeyactive")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeBillingSelfResponse(t, recorder)
	require.True(t, response.Success)
	require.Equal(t, "wallet_first", response.Data.BillingPreference)
	require.True(t, response.Data.HasActiveSubscription)
	require.Equal(t, 1200, response.Data.Wallet.RemainingQuota)
	require.Equal(t, 340, response.Data.Wallet.UsedQuota)
	require.Len(t, response.Data.Subscriptions, 1)
	require.Equal(t, 7001, response.Data.Subscriptions[0].Id)
	require.Equal(t, int64(1000), response.Data.Subscriptions[0].AmountTotal)
	require.Equal(t, int64(240), response.Data.Subscriptions[0].AmountUsed)
	require.NotNil(t, response.Data.Subscriptions[0].AmountRemaining)
	require.Equal(t, int64(760), *response.Data.Subscriptions[0].AmountRemaining)
	require.False(t, response.Data.Subscriptions[0].Unlimited)
}

func TestBillingSelfAllowsExhaustedTokenToQuery(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.User{
		Id:        5002,
		Username:  "exhausted-user",
		Password:  "hashed-password",
		Role:      common.RoleCommonUser,
		Status:    common.UserStatusEnabled,
		Group:     "default",
		Quota:     88,
		UsedQuota: 912,
		AffCode:   "B5002",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5002,
		Key:            "billingkeyexhausted",
		Name:           "exhausted-key",
		Status:         common.TokenStatusExhausted,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    -1,
		RemainQuota:    0,
		UsedQuota:      999,
		UnlimitedQuota: false,
		Group:          "default",
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeyexhausted")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeBillingSelfResponse(t, recorder)
	require.True(t, response.Success)
	require.Equal(t, 88, response.Data.Wallet.RemainingQuota)
	require.Equal(t, 912, response.Data.Wallet.UsedQuota)
}

func TestBillingSelfAllowsExpiredTokenToQuery(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.User{
		Id:        5003,
		Username:  "expired-user",
		Password:  "hashed-password",
		Role:      common.RoleCommonUser,
		Status:    common.UserStatusEnabled,
		Group:     "default",
		Quota:     321,
		UsedQuota: 654,
		AffCode:   "B5003",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5003,
		Key:            "billingkeyexpired",
		Name:           "expired-key",
		Status:         common.TokenStatusExpired,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    now - 10,
		RemainQuota:    0,
		UsedQuota:      20,
		UnlimitedQuota: false,
		Group:          "default",
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeyexpired")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeBillingSelfResponse(t, recorder)
	require.True(t, response.Success)
	require.Equal(t, 321, response.Data.Wallet.RemainingQuota)
	require.Equal(t, 654, response.Data.Wallet.UsedQuota)
}

func TestBillingSelfRejectsDisabledToken(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.User{
		Id:       5004,
		Username: "disabled-user",
		Password: "hashed-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
		AffCode:  "B5004",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5004,
		Key:            "billingkeydisabled",
		Name:           "disabled-key",
		Status:         common.TokenStatusDisabled,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    -1,
		RemainQuota:    10,
		UsedQuota:      20,
		UnlimitedQuota: false,
		Group:          "default",
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeydisabled")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	response := decodeBillingSelfResponse(t, recorder)
	require.False(t, response.Success)
	require.Equal(t, "token_disabled", response.Code)
	require.NotEmpty(t, response.Message)
}

func TestBillingSelfRejectsTokenWhenClientIPOutsideAllowList(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()
	allowIps := "127.0.0.1/32"

	require.NoError(t, db.Create(&model.User{
		Id:       5005,
		Username: "ip-restricted-user",
		Password: "hashed-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
		AffCode:  "B5005",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5005,
		Key:            "billingkeyiplocked",
		Name:           "ip-locked-key",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    -1,
		RemainQuota:    10,
		UsedQuota:      20,
		UnlimitedQuota: false,
		Group:          "default",
		AllowIps:       &allowIps,
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeyiplocked")
	request.RemoteAddr = "198.51.100.10:34567"

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	response := decodeBillingSelfResponse(t, recorder)
	require.False(t, response.Success)
	require.Equal(t, "您的 IP 不在令牌允许访问的列表中", response.Message)
}

func TestBillingSelfMarksUnlimitedSubscriptions(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.User{
		Id:       5006,
		Username: "unlimited-user",
		Password: "hashed-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    50,
		AffCode:  "B5006",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5006,
		Key:            "billingkeyunlimited",
		Name:           "unlimited-key",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    -1,
		RemainQuota:    10,
		UsedQuota:      20,
		UnlimitedQuota: false,
		Group:          "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:            7006,
		UserId:        5006,
		PlanId:        9006,
		AmountTotal:   0,
		AmountUsed:    240,
		StartTime:     now - 7200,
		EndTime:       now + 7200,
		Status:        "active",
		Source:        "order",
		NextResetTime: now + 3600,
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeyunlimited")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeBillingSelfResponse(t, recorder)
	require.True(t, response.Success)
	require.Len(t, response.Data.Subscriptions, 1)
	require.True(t, response.Data.Subscriptions[0].Unlimited)
	require.Nil(t, response.Data.Subscriptions[0].AmountRemaining)
	require.Equal(t, int64(0), response.Data.Subscriptions[0].AmountTotal)
}

func TestBillingSelfV2ReturnsWalletAndActiveSubscriptionsForToken(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.User{
		Id:        5101,
		Username:  "billing-user-v2",
		Password:  "hashed-password",
		Role:      common.RoleCommonUser,
		Status:    common.UserStatusEnabled,
		Group:     "default",
		Quota:     1200,
		UsedQuota: 340,
		Setting:   `{"billing_preference":"wallet_first"}`,
		AffCode:   "B5101",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5101,
		Key:            "billingkeyactivev2",
		Name:           "primary-key-v2",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    -1,
		RemainQuota:    200,
		UsedQuota:      50,
		UnlimitedQuota: false,
		Group:          "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:            7101,
		UserId:        5101,
		PlanId:        9101,
		AmountTotal:   1000,
		AmountUsed:    240,
		StartTime:     now - 7200,
		EndTime:       now + 7200,
		Status:        "active",
		Source:        "order",
		NextResetTime: now + 3600,
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self/v2", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeyactivev2")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeBillingSelfV2Response(t, recorder)
	require.True(t, response.Success)
	require.Equal(t, "USD", response.Data.Currency)
	require.Equal(t, "wallet_first", response.Data.BillingPreference)
	require.True(t, response.Data.HasActiveSubscription)
	require.InDelta(t, 0.0024, response.Data.Wallet.RemainingAmount, 0.000001)
	require.InDelta(t, 0.00068, response.Data.Wallet.UsedAmount, 0.000001)
	require.Len(t, response.Data.Subscriptions, 1)
	require.Equal(t, 7101, response.Data.Subscriptions[0].Id)
	require.InDelta(t, 0.002, response.Data.Subscriptions[0].AmountTotal, 0.000001)
	require.InDelta(t, 0.00048, response.Data.Subscriptions[0].AmountUsed, 0.000001)
	require.NotNil(t, response.Data.Subscriptions[0].AmountRemaining)
	require.InDelta(t, 0.00152, *response.Data.Subscriptions[0].AmountRemaining, 0.000001)
	require.False(t, response.Data.Subscriptions[0].Unlimited)
}

func TestBillingSelfV2KeepsUnlimitedSubscriptionsInUSDContract(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.User{
		Id:       5106,
		Username: "unlimited-user-v2",
		Password: "hashed-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    50,
		AffCode:  "B5106",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5106,
		Key:            "billingkeyunlimitedv2",
		Name:           "unlimited-key-v2",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    -1,
		RemainQuota:    10,
		UsedQuota:      20,
		UnlimitedQuota: false,
		Group:          "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserSubscription{
		Id:            7106,
		UserId:        5106,
		PlanId:        9106,
		AmountTotal:   0,
		AmountUsed:    240,
		StartTime:     now - 7200,
		EndTime:       now + 7200,
		Status:        "active",
		Source:        "order",
		NextResetTime: now + 3600,
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self/v2", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeyunlimitedv2")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeBillingSelfV2Response(t, recorder)
	require.True(t, response.Success)
	require.Len(t, response.Data.Subscriptions, 1)
	require.True(t, response.Data.Subscriptions[0].Unlimited)
	require.Nil(t, response.Data.Subscriptions[0].AmountRemaining)
	require.Equal(t, 0.0, response.Data.Subscriptions[0].AmountTotal)
}

func TestBillingSelfV2RejectsDisabledToken(t *testing.T) {
	db := setupBillingSelfRouteTestDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&model.User{
		Id:       5104,
		Username: "disabled-user-v2",
		Password: "hashed-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
		AffCode:  "B5104",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId:         5104,
		Key:            "billingkeydisabledv2",
		Name:           "disabled-key-v2",
		Status:         common.TokenStatusDisabled,
		CreatedTime:    now - 3600,
		AccessedTime:   now - 60,
		ExpiredTime:    -1,
		RemainQuota:    10,
		UsedQuota:      20,
		UnlimitedQuota: false,
		Group:          "default",
	}).Error)

	engine := newBillingSelfTestEngine()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/billing/self/v2", nil)
	request.Header.Set("Authorization", "Bearer sk-billingkeydisabledv2")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	response := decodeBillingSelfV2Response(t, recorder)
	require.False(t, response.Success)
	require.Equal(t, "token_disabled", response.Code)
	require.NotEmpty(t, response.Message)
}
