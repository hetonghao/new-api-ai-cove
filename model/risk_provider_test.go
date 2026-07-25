package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRiskProviderPersistenceEnforcesValidationBeforeActivation(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&RiskProvider{}))

	first := &RiskProvider{Name: "primary", ProviderType: RiskProviderCloudflare, Model: "@cf/meta/llama-guard-3-8b", BaseURL: "https://api.cloudflare.com/client/v4/accounts/a/ai/run", CredentialEncrypted: "ciphertext-1"}
	require.NoError(t, CreateRiskProvider(first))
	assert.Equal(t, DefaultRiskProviderTimeoutMs, first.TimeoutMs)
	assert.Equal(t, DefaultRiskProviderFailureThreshold, first.FailureThreshold)
	assert.Equal(t, DefaultRiskProviderCooldownSeconds, first.CooldownSeconds)

	err = ActivateRiskProvider(first.Id)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRiskProviderNotValidated))

	require.NoError(t, MarkRiskProviderValidated(first.Id))
	require.NoError(t, ActivateRiskProvider(first.Id))

	second := &RiskProvider{Name: "secondary", ProviderType: RiskProviderCloudflare, Model: "@cf/meta/llama-guard-3-8b", BaseURL: "https://api.cloudflare.com/client/v4/accounts/b/ai/run", CredentialEncrypted: "ciphertext-2"}
	require.NoError(t, CreateRiskProvider(second))
	require.NoError(t, MarkRiskProviderValidated(second.Id))
	require.NoError(t, ActivateRiskProvider(second.Id))

	providers, err := GetRiskProviders()
	require.NoError(t, err)
	require.Len(t, providers, 2)
	assert.False(t, providers[0].Active)
	assert.True(t, providers[1].Active)
}
