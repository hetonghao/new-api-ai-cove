package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenQueriesIncludeLegacyRowsWithNullSystemManaged(t *testing.T) {
	// Given
	originalDB := DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.LogDatabaseType())
	t.Cleanup(func() {
		DB = originalDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Token{}))
	token := &Token{UserId: 42, Key: "legacy-token", Name: "legacy", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Update("system_managed", nil).Error)

	// When
	tokens, err := GetAllUserTokens(token.UserId, 0, 10)

	// Then
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, token.Id, tokens[0].Id)
}
