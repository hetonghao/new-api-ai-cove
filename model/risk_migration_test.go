package model

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfillRiskRecordProviderTypes_updatesOnlyMatchableHistory(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.AutoMigrate(&RiskProvider{}))
	require.NoError(t, db.Create(&RiskProvider{Id: 21, Name: "known", ProviderType: RiskProviderCloudflare}).Error)
	known := validRiskRecordInput(RiskRecordResultSafe)
	known.ProviderType = ""
	orphaned := validRiskRecordInput(RiskRecordResultSafe)
	orphaned.RequestID = "req-orphaned"
	orphaned.ProviderID = 22
	orphaned.ProviderName = "deleted"
	orphaned.ProviderType = ""
	require.NoError(t, RecordRiskObservation(context.Background(), known))
	require.NoError(t, RecordRiskObservation(context.Background(), orphaned))

	// When
	require.NoError(t, backfillRiskRecordProviderTypes(db))

	// Then
	var records []RiskRecord
	require.NoError(t, db.Order("id asc").Find(&records).Error)
	require.Len(t, records, 2)
	assert.Equal(t, RiskProviderCloudflare, records[0].ProviderType)
	assert.Empty(t, records[1].ProviderType)
}

func TestRiskRecordPreviewMigration_preservesHistoryAndSupportsLargeText(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.Create(&RiskRecord{
		RequestID: "legacy", ChannelID: 1, UserID: 1, Preview: "legacy preview",
		RuleIDs: []int{}, ProviderID: 1, ProviderName: "provider", Result: RiskRecordResultSafe,
		Categories: []string{}, Chunks: []RiskRecordChunk{}, Source: RiskRecordSourceProvider,
	}).Error)

	// When
	require.NoError(t, db.AutoMigrate(&RiskRecord{}))
	largePreview := strings.Repeat("审", 1200)
	require.NoError(t, db.Model(&RiskRecord{}).Where("request_id = ?", "legacy").Update("preview", largePreview).Error)

	// Then
	var record RiskRecord
	require.NoError(t, db.Where("request_id = ?", "legacy").Take(&record).Error)
	assert.Equal(t, largePreview, record.Preview)
}

func TestBackfillLegacyRiskPolicyProviderIDs_migratesActiveProvider(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{
		Name: "legacy active", ProviderType: RiskProviderCloudflare, Active: true,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", CredentialEncrypted: "ciphertext",
	}
	require.NoError(t, CreateRiskProvider(provider))
	require.NoError(t, MarkRiskProviderValidated(provider.Id))
	require.NoError(t, DB.Model(provider).Update("active", true).Error)
	require.NoError(t, DB.Create(&RiskPolicy{
		Id: riskPolicySingletonID, Enabled: true, ProviderIDs: "[]", EnabledChannels: "[24]",
		ExcludedUserIDs: "[]", ExcludedModels: "[]", ReviewMode: RiskReviewSelective, ActionMode: RiskActionObserve,
	}).Error)

	// When
	require.NoError(t, backfillLegacyRiskPolicyProviderIDs(DB))

	// Then
	state, err := GetRiskPolicyState()
	require.NoError(t, err)
	assert.Equal(t, []int{provider.Id}, state.ProviderIDs)
}
