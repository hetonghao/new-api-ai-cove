package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	require.NoError(t, db.AutoMigrate(&User{}, &QuotaData{}))
	require.NoError(t, db.Exec("DELETE FROM quota_data").Error)
	require.NoError(t, db.Exec("DELETE FROM users").Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DELETE FROM quota_data").Error)
		require.NoError(t, db.Exec("DELETE FROM users").Error)
	})

	return db
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
