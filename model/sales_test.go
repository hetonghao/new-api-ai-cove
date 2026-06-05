package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSalesModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	initCol()

	db := DB
	require.NoError(t, db.AutoMigrate(&User{}, &QuotaData{}, &TopUp{}, &Log{}, &SalesCommissionSettlement{}))
	require.NoError(t, db.Exec("DELETE FROM quota_data").Error)
	require.NoError(t, db.Exec("DELETE FROM top_ups").Error)
	require.NoError(t, db.Exec("DELETE FROM logs").Error)
	require.NoError(t, db.Exec("DELETE FROM users").Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM quota_data").Error)
		require.NoError(t, db.Exec("DELETE FROM top_ups").Error)
		require.NoError(t, db.Exec("DELETE FROM logs").Error)
		require.NoError(t, db.Exec("DELETE FROM users").Error)
	})

	return db
}

func setupSplitSalesModelTestDB(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	oldDB := DB
	oldLogDB := LOG_DB
	name := strings.ReplaceAll(t.Name(), "/", "_")
	appDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s_app?mode=memory&cache=shared", name)), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s_log?mode=memory&cache=shared", name)), &gorm.Config{})
	require.NoError(t, err)

	DB = appDB
	LOG_DB = logDB
	initCol()

	require.NoError(t, appDB.AutoMigrate(&User{}, &QuotaData{}, &TopUp{}, &Channel{}, &SalesCommissionSettlement{}))
	require.NoError(t, logDB.AutoMigrate(&Log{}))

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		if sqlDB, err := appDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return appDB, logDB
}

func seedSalesModelUsers(t *testing.T, db *gorm.DB) {
	t.Helper()

	require.NoError(t, db.Create(&[]User{
		{Id: 1, Username: "seller", Role: common.RoleSalesUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "sales-1"},
		{Id: 2, Username: "alice", Email: "alice@example.com", DisplayName: "Alice", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 1, AffCode: "sales-2"},
		{Id: 3, Username: "bob", Email: "bob@example.com", DisplayName: "Bob", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "vip", InviterId: 1, AffCode: "sales-3"},
		{Id: 4, Username: "mallory", Email: "mallory@example.com", DisplayName: "Mallory", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", InviterId: 99, AffCode: "sales-4"},
	}).Error)
}

func TestListSalesInvitedUsersScopesToInviterAndFilters(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)

	users, total, err := ListSalesInvitedUsers(1, "bob", "vip", 0, 20)

	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, users, 1)
	require.Equal(t, "bob", users[0].Username)
	require.Equal(t, "vip", users[0].Group)
}

func TestListSalesInvitedUsersIncludesSuccessfulTopUpAmountPerUser(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	require.NoError(t, db.Create(&[]TopUp{
		{UserId: 2, Amount: 10000, Money: 10.5, Status: common.TopUpStatusSuccess, TradeNo: "sales-user-topup-1"},
		{UserId: 2, Amount: 20000, Money: 20, Status: common.TopUpStatusPending, TradeNo: "sales-user-topup-2"},
		{UserId: 3, Amount: 5000, Money: 5.25, Status: common.TopUpStatusSuccess, TradeNo: "sales-user-topup-3"},
		{UserId: 1, Amount: 30000, Money: 30, Status: common.TopUpStatusSuccess, TradeNo: "sales-user-topup-4"},
		{UserId: 4, Amount: 40000, Money: 40, Status: common.TopUpStatusSuccess, TradeNo: "sales-user-topup-5"},
	}).Error)

	users, total, err := ListSalesInvitedUsers(1, "", "", 0, 20)

	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, users, 2)
	amountByUsername := map[string]float64{}
	for _, user := range users {
		amountByUsername[user.Username] = user.TopUpAmount
	}
	require.InDelta(t, 10.5, amountByUsername["alice"], 0.0001)
	require.InDelta(t, 5.25, amountByUsername["bob"], 0.0001)
}

func TestUpdateSalesInvitedUserGroupOnlyUpdatesOwnedUsers(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)

	require.NoError(t, UpdateSalesInvitedUserGroup(1, 2, "vip"))
	require.Error(t, UpdateSalesInvitedUserGroup(1, 4, "vip"))

	var owned User
	require.NoError(t, db.First(&owned, 2).Error)
	require.Equal(t, "vip", owned.Group)

	var notOwned User
	require.NoError(t, db.First(&notOwned, 4).Error)
	require.Equal(t, "default", notOwned.Group)
}

func TestGetSalesQuotaDataOnlyAggregatesInvitedUsers(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	require.NoError(t, db.Create(&[]QuotaData{
		{UserID: 2, Username: "alice", ModelName: "gpt-a", CreatedAt: 100, Count: 1, Quota: 10, TokenUsed: 20},
		{UserID: 3, Username: "bob", ModelName: "gpt-a", CreatedAt: 100, Count: 2, Quota: 30, TokenUsed: 40},
		{UserID: 4, Username: "mallory", ModelName: "gpt-a", CreatedAt: 100, Count: 5, Quota: 500, TokenUsed: 600},
		{UserID: 2, Username: "alice", ModelName: "gpt-b", CreatedAt: 200, Count: 3, Quota: 50, TokenUsed: 60},
	}).Error)

	byModel, err := GetSalesQuotaData(1, 0, 300)
	require.NoError(t, err)
	require.Len(t, byModel, 2)
	require.Equal(t, "gpt-a", byModel[0].ModelName)
	require.Equal(t, 3, byModel[0].Count)
	require.Equal(t, 40, byModel[0].Quota)
	require.Equal(t, 60, byModel[0].TokenUsed)

	byUser, err := GetSalesQuotaDataGroupByUser(1, 0, 300)
	require.NoError(t, err)
	require.Len(t, byUser, 3)
	require.Equal(t, "alice", byUser[0].Username)
	require.EqualValues(t, 100, byUser[0].CreatedAt)
	require.Equal(t, 1, byUser[0].Count)
	require.Equal(t, 10, byUser[0].Quota)
	require.Equal(t, 20, byUser[0].TokenUsed)
}

func TestGetSalesQuotaDataGroupByUserIncludesSalesSelfAndInvitedUsers(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	require.NoError(t, db.Create(&QuotaData{UserID: 2, Username: "alice", ModelName: "gpt-a", CreatedAt: 100, Count: 1, Quota: 10, TokenUsed: 20}).Error)
	require.NoError(t, db.Create(&QuotaData{UserID: 3, Username: "bob", ModelName: "gpt-b", CreatedAt: 100, Count: 1, Quota: 12, TokenUsed: 22}).Error)
	require.NoError(t, db.Create(&QuotaData{UserID: 1, Username: "seller", ModelName: "gpt-self", CreatedAt: 100, Count: 1, Quota: 15, TokenUsed: 25}).Error)
	require.NoError(t, db.Create(&QuotaData{UserID: 4, Username: "mallory", ModelName: "gpt-other", CreatedAt: 100, Count: 1, Quota: 500, TokenUsed: 600}).Error)

	byUser, err := GetSalesQuotaDataGroupByUser(1, 0, 300)

	require.NoError(t, err)
	require.Len(t, byUser, 3)
	require.Equal(t, "alice", byUser[0].Username)
	require.Equal(t, "bob", byUser[1].Username)
	require.Equal(t, "seller", byUser[2].Username)
}

func TestGetSalesTopUpAmountOnlyCountsSuccessfulInvitedUsers(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	require.NoError(t, db.Create(&[]TopUp{
		{UserId: 2, Amount: 10000, Money: 10.5, Status: common.TopUpStatusSuccess, TradeNo: "sales-topup-1"},
		{UserId: 2, Amount: 20000, Money: 20, Status: common.TopUpStatusPending, TradeNo: "sales-topup-2"},
		{UserId: 1, Amount: 30000, Money: 30, Status: common.TopUpStatusSuccess, TradeNo: "sales-topup-3"},
		{UserId: 4, Amount: 40000, Money: 40, Status: common.TopUpStatusSuccess, TradeNo: "sales-topup-4"},
	}).Error)

	amount, err := GetSalesTopUpAmount(1)

	require.NoError(t, err)
	require.InDelta(t, 10.5, amount, 0.0001)
}

func TestGetSalesCommissionSummaryArchivesSettledAmountAndRepricesPendingRevenue(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	require.NoError(t, db.Model(&User{}).Where("id = ?", 1).Update("commission_ratio", 10).Error)
	require.NoError(t, db.Create(&[]TopUp{
		{UserId: 2, Amount: 10000, Money: 600, Status: common.TopUpStatusSuccess, TradeNo: "commission-topup-1"},
		{UserId: 3, Amount: 10000, Money: 400, Status: common.TopUpStatusSuccess, TradeNo: "commission-topup-2"},
	}).Error)

	settlement, err := CreateSalesCommissionSettlement(1, 99, 30, "first settlement")
	require.NoError(t, err)
	require.InDelta(t, 300, settlement.CoveredRevenue, 0.0001)

	require.NoError(t, db.Model(&User{}).Where("id = ?", 1).Update("commission_ratio", 15).Error)

	summary, err := GetSalesCommissionSummary(1)

	require.NoError(t, err)
	require.InDelta(t, 1000, summary.TotalRevenue, 0.0001)
	require.InDelta(t, 15, summary.CommissionRatio, 0.0001)
	require.InDelta(t, 30, summary.SettledCommissionAmount, 0.0001)
	require.InDelta(t, 700, summary.PendingCommissionRevenue, 0.0001)
	require.InDelta(t, 105, summary.PendingCommissionAmount, 0.0001)
	require.InDelta(t, 135, summary.TotalCommissionAmount, 0.0001)
}

func TestCreateSalesCommissionSettlementRejectsAmountAbovePending(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	require.NoError(t, db.Model(&User{}).Where("id = ?", 1).Update("commission_ratio", 10).Error)
	require.NoError(t, db.Create(&TopUp{UserId: 2, Amount: 10000, Money: 100, Status: common.TopUpStatusSuccess, TradeNo: "commission-topup-limit"}).Error)

	_, err := CreateSalesCommissionSettlement(1, 99, 11, "too much")

	require.Error(t, err)
}

func TestGetSalesLogsOnlyReturnsInvitedUsers(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, Username: "seller", Type: LogTypeConsume, CreatedAt: 100, ModelName: "gpt-sales-self", TokenName: "self-token", Group: "default", Quota: 10},
		{UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: 101, ModelName: "gpt-sales-invite", TokenName: "invite-token", Group: "default", Quota: 20, ChannelId: 1, ChannelName: "secret-channel", Other: `{"admin_info":{"use_channel":[1]},"stream_status":{"status":"error"},"group":"default"}`},
		{UserId: 4, Username: "mallory", Type: LogTypeConsume, CreatedAt: 102, ModelName: "gpt-other", TokenName: "other-token", Group: "default", Quota: 30},
	}).Error)

	logs, total, err := GetSalesLogs(1, LogTypeConsume, 0, 0, "", "", "", 0, 20, 0, "", "", "")

	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, 2, logs[0].UserId)
	require.Equal(t, "alice", logs[0].Username)
	require.Empty(t, logs[0].ChannelName)
	require.NotContains(t, logs[0].Other, "admin_info")
	require.NotContains(t, logs[0].Other, "stream_status")
	serialized, err := json.Marshal(logs[0])
	require.NoError(t, err)
	require.NotContains(t, string(serialized), "channel_name")
}

func TestGetSalesLogsStillFiltersByRequestedTypeWithinSalesScope(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: 100, ModelName: "gpt-consume", TokenName: "consume-token", Group: "default", Quota: 10},
		{UserId: 2, Username: "alice", Type: LogTypeError, CreatedAt: 101, ModelName: "gpt-error", TokenName: "error-token", Group: "default", Quota: 20},
		{UserId: 2, Username: "alice", Type: LogTypeManage, CreatedAt: 102, ModelName: "gpt-manage", TokenName: "manage-token", Group: "default", Quota: 30},
		{UserId: 4, Username: "mallory", Type: LogTypeError, CreatedAt: 103, ModelName: "gpt-other-error", TokenName: "other-error-token", Group: "default", Quota: 40},
	}).Error)

	logs, total, err := GetSalesLogs(1, LogTypeError, 0, 0, "", "", "", 0, 20, 0, "", "", "")

	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, 2, logs[0].UserId)
	require.Equal(t, "alice", logs[0].Username)
	require.Equal(t, LogTypeError, logs[0].Type)
	require.Equal(t, "gpt-error", logs[0].ModelName)
}

func TestGetSalesLogsStatOnlyCountsInvitedUsers(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, Username: "seller", Type: LogTypeConsume, CreatedAt: now - 3, ModelName: "gpt-sales-self", TokenName: "self-token", Group: "default", Quota: 10, PromptTokens: 10, CompletionTokens: 20},
		{UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: now - 2, ModelName: "gpt-sales-invite", TokenName: "invite-token", Group: "default", Quota: 20, PromptTokens: 30, CompletionTokens: 40},
		{UserId: 4, Username: "mallory", Type: LogTypeConsume, CreatedAt: now - 1, ModelName: "gpt-other", TokenName: "other-token", Group: "default", Quota: 30, PromptTokens: 50, CompletionTokens: 60},
	}).Error)

	stat, err := GetSalesLogsStat(1, LogTypeUnknown, 0, 0, "", "", "", 0, "", "", "")

	require.NoError(t, err)
	require.Equal(t, 20, stat.Quota)
	require.Equal(t, 1, stat.Rpm)
	require.Equal(t, 70, stat.Tpm)
}

func TestGetSalesLogsStatWithUnknownTypeOnlyCountsInvitedConsumeLogs(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]Log{
		{UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: now - 4, ModelName: "gpt-sales-invite", TokenName: "invite-token", Group: "default", Quota: 20, PromptTokens: 30, CompletionTokens: 40},
		{UserId: 2, Username: "alice", Type: LogTypeManage, CreatedAt: now - 3, ModelName: "gpt-sales-manage", TokenName: "manage-token", Group: "default", Quota: 200, PromptTokens: 300, CompletionTokens: 400},
		{UserId: 2, Username: "alice", Type: LogTypeTopup, CreatedAt: now - 2, ModelName: "gpt-sales-topup", TokenName: "topup-token", Group: "default", Quota: 500, PromptTokens: 600, CompletionTokens: 700},
		{UserId: 4, Username: "mallory", Type: LogTypeConsume, CreatedAt: now - 1, ModelName: "gpt-other", TokenName: "other-token", Group: "default", Quota: 30, PromptTokens: 50, CompletionTokens: 60},
	}).Error)

	stat, err := GetSalesLogsStat(1, LogTypeUnknown, 0, 0, "", "", "", 0, "", "", "")

	require.NoError(t, err)
	require.Equal(t, 20, stat.Quota)
	require.Equal(t, 1, stat.Rpm)
	require.Equal(t, 70, stat.Tpm)
}

func TestGetSalesLogsStatWithNonConsumeTypeReturnsEmptyStats(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]Log{
		{UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: now - 3, ModelName: "gpt-sales-invite", TokenName: "invite-token", Group: "default", Quota: 20, PromptTokens: 30, CompletionTokens: 40},
		{UserId: 2, Username: "alice", Type: LogTypeError, CreatedAt: now - 2, ModelName: "gpt-sales-error", TokenName: "error-token", Group: "default", Quota: 900, PromptTokens: 901, CompletionTokens: 902},
		{UserId: 2, Username: "alice", Type: LogTypeManage, CreatedAt: now - 1, ModelName: "gpt-sales-manage", TokenName: "manage-token", Group: "default", Quota: 800, PromptTokens: 801, CompletionTokens: 802},
	}).Error)

	stat, err := GetSalesLogsStat(1, LogTypeError, 0, 0, "", "", "", 0, "", "", "")

	require.NoError(t, err)
	require.Equal(t, 0, stat.Quota)
	require.Equal(t, 0, stat.Rpm)
	require.Equal(t, 0, stat.Tpm)
}

func TestGetSalesLogsStatSupportsRequestFilters(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]Log{
		{UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: now - 3, ModelName: "gpt-sales-invite", TokenName: "invite-token", Group: "default", Quota: 20, PromptTokens: 30, CompletionTokens: 40, RequestId: "req-1", UpstreamRequestId: "up-1"},
		{UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: now - 2, ModelName: "gpt-sales-invite", TokenName: "invite-token", Group: "default", Quota: 50, PromptTokens: 60, CompletionTokens: 70, RequestId: "req-2", UpstreamRequestId: "up-2"},
	}).Error)

	stat, err := GetSalesLogsStat(1, LogTypeUnknown, 0, 0, "", "", "", 0, "", "req-1", "up-1")

	require.NoError(t, err)
	require.Equal(t, 20, stat.Quota)
	require.Equal(t, 1, stat.Rpm)
	require.Equal(t, 70, stat.Tpm)
}

func TestSalesLogsWorkWhenDBAndLogDBAreSeparate(t *testing.T) {
	userDB, logDB := setupSplitSalesModelTestDB(t)
	seedSalesModelUsers(t, userDB)
	now := time.Now().Unix()
	require.NoError(t, logDB.Create(&[]Log{
		{UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: now - 3, ModelName: "gpt-sales-invite", TokenName: "invite-token", Group: "default", Quota: 20, PromptTokens: 30, CompletionTokens: 40},
		{UserId: 2, Username: "alice", Type: LogTypeError, CreatedAt: now - 2, ModelName: "gpt-sales-error", TokenName: "error-token", Group: "default", Quota: 200, PromptTokens: 300, CompletionTokens: 400},
		{UserId: 4, Username: "mallory", Type: LogTypeConsume, CreatedAt: now - 1, ModelName: "gpt-other", TokenName: "other-token", Group: "default", Quota: 30, PromptTokens: 50, CompletionTokens: 60},
	}).Error)

	logs, total, err := GetSalesLogs(1, LogTypeConsume, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, 2, logs[0].UserId)

	stat, err := GetSalesLogsStat(1, LogTypeUnknown, 0, 0, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, 20, stat.Quota)
	require.Equal(t, 1, stat.Rpm)
	require.Equal(t, 70, stat.Tpm)
}

func TestSalesLogsReturnEmptyWhenSalesHasNoInvitedUsers(t *testing.T) {
	userDB, logDB := setupSplitSalesModelTestDB(t)
	require.NoError(t, userDB.Create(&User{
		Id:       1,
		Username: "seller",
		Role:     common.RoleSalesUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "sales-empty-1",
	}).Error)
	require.NoError(t, logDB.Create(&Log{UserId: 99, Username: "other", Type: LogTypeConsume, CreatedAt: time.Now().Unix(), Quota: 100}).Error)

	logs, total, err := GetSalesLogs(1, LogTypeUnknown, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
	require.Empty(t, logs)

	stat, err := GetSalesLogsStat(1, LogTypeUnknown, 0, 0, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, 0, stat.Quota)
	require.Equal(t, 0, stat.Rpm)
	require.Equal(t, 0, stat.Tpm)
}
