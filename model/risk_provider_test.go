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
	require.NoError(t, db.AutoMigrate(&RiskProvider{}, &RiskPolicy{}))

	first := &RiskProvider{Name: "primary", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b", CredentialEncrypted: "ciphertext-1"}
	require.NoError(t, CreateRiskProvider(first))
	assert.Equal(t, DefaultRiskProviderTimeoutMs, first.TimeoutMs)
	assert.Equal(t, DefaultRiskProviderFailureThreshold, first.FailureThreshold)
	assert.Equal(t, DefaultRiskProviderCooldownSeconds, first.CooldownSeconds)

	err = ActivateRiskProvider(first.Id)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRiskProviderNotValidated))

	require.NoError(t, MarkRiskProviderValidated(first.Id))
	require.NoError(t, ActivateRiskProvider(first.Id))

	second := &RiskProvider{Name: "secondary", ProviderType: RiskProviderCloudflare, AccountID: "fedcba9876543210fedcba9876543210", Model: "@cf/meta/llama-guard-3-8b", CredentialEncrypted: "ciphertext-2"}
	require.NoError(t, CreateRiskProvider(second))
	require.NoError(t, MarkRiskProviderValidated(second.Id))
	require.NoError(t, ActivateRiskProvider(second.Id))

	providers, err := GetRiskProviders()
	require.NoError(t, err)
	require.Len(t, providers, 2)
	assert.False(t, providers[0].Active)
	assert.True(t, providers[1].Active)
}

func TestRiskProviderAccountIDRejectsInvalidInputAndReadsLegacyBaseURL(t *testing.T) {
	// Given a legacy row and invalid new account IDs
	legacy := &RiskProvider{BaseURL: "https://legacy.example/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/run"}

	// When the effective account ID is resolved
	accountID, err := legacy.CloudflareAccountID()

	// Then the legacy path remains readable without trusting its host
	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", accountID)

	for _, accountID := range []string{"", "not-an-account-id", "0123456789abcdef0123456789abcdeg"} {
		provider := &RiskProvider{
			Name: "invalid", ProviderType: RiskProviderCloudflare, AccountID: accountID,
			Model: "@cf/meta/llama-guard-3-8b", CredentialEncrypted: "ciphertext",
		}
		require.Error(t, normalizeRiskProvider(provider))
	}
}
