package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyRiskPolicyMigrationFixture struct {
	Id              int            `gorm:"primaryKey"`
	Enabled         bool           `gorm:"not null;default:true"`
	EnabledChannels string         `gorm:"type:text;not null"`
	ExcludedUserIDs string         `gorm:"type:text;not null;default:'[]'"`
	ExcludedModels  string         `gorm:"type:text;not null;default:'[]'"`
	ReviewMode      RiskReviewMode `gorm:"type:varchar(16);not null"`
	ActionMode      RiskActionMode `gorm:"type:varchar(16);not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (legacyRiskPolicyMigrationFixture) TableName() string {
	return "risk_policies"
}

type legacyRiskRecordMigrationFixture struct {
	Id               int               `gorm:"primaryKey"`
	RequestID        string            `gorm:"type:varchar(256);not null;index"`
	ChannelID        int               `gorm:"not null;index"`
	UserID           int               `gorm:"not null;index"`
	TokenID          int               `gorm:"not null"`
	Model            string            `gorm:"type:varchar(256);not null"`
	Path             string            `gorm:"type:varchar(512);not null"`
	Preview          string            `gorm:"type:varchar(800);not null"`
	ContentHash      string            `gorm:"type:varchar(64);not null"`
	RuleIDs          []int             `gorm:"serializer:json;type:text;not null"`
	ProviderID       int               `gorm:"not null;index"`
	ProviderName     string            `gorm:"type:varchar(128);not null"`
	Result           RiskRecordResult  `gorm:"type:varchar(16);not null;index"`
	Categories       []string          `gorm:"serializer:json;type:text;not null"`
	LatencyMS        int64             `gorm:"not null"`
	PromptTokens     int               `gorm:"not null"`
	CompletionTokens int               `gorm:"not null"`
	TotalTokens      int               `gorm:"not null"`
	Neurons          int64             `gorm:"not null"`
	Chunks           []RiskRecordChunk `gorm:"serializer:json;type:text"`
	ErrorCode        string            `gorm:"type:varchar(128);not null"`
	ErrorDetail      string            `gorm:"type:varchar(512)"`
	Source           RiskRecordSource  `gorm:"type:varchar(16);not null;index"`
	CacheHit         bool              `gorm:"not null"`
	ProviderCalled   bool              `gorm:"not null"`
	Blocked          bool              `gorm:"not null"`
	ObservedAt       time.Time         `gorm:"not null;index"`
}

func (legacyRiskRecordMigrationFixture) TableName() string {
	return "risk_records"
}

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

func TestBackfillRiskRecordGovernancePreviewChars_copiesLegacySettingOnce(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.Create(&RiskRecordGovernance{
		Id: riskRecordGovernanceID, SaveScope: RiskRecordSaveAll, ContentSaveScope: RiskContentSaveAll,
		RetentionDays: 30, PreviewChars: 1200,
	}).Error)

	// When
	require.NoError(t, backfillRiskRecordGovernancePreviewChars(db))
	require.NoError(t, db.Model(&RiskRecordGovernance{}).Where("id = ?", riskRecordGovernanceID).
		Update("safe_preview_chars", 50).Error)
	require.NoError(t, backfillRiskRecordGovernancePreviewChars(db))

	// Then
	var governance RiskRecordGovernance
	require.NoError(t, db.First(&governance, riskRecordGovernanceID).Error)
	assert.Equal(t, 50, governance.SafePreviewChars)
	assert.Equal(t, 1200, governance.NonSafePreviewChars)
}

func TestRiskSchemaMigration_preservesLegacyDataAndExpandsTextColumns(t *testing.T) {
	// Given
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&RiskProvider{}, &legacyRiskPolicyMigrationFixture{}, &legacyRiskRecordMigrationFixture{},
	))
	provider := &RiskProvider{
		Id: 21, Name: "legacy active", ProviderType: RiskProviderCloudflare, Active: true,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", CredentialEncrypted: "ciphertext",
	}
	require.NoError(t, db.Create(provider).Error)
	require.NoError(t, db.Create(&legacyRiskPolicyMigrationFixture{
		Id: riskPolicySingletonID, Enabled: true, EnabledChannels: "[24]",
		ExcludedUserIDs: "[]", ExcludedModels: "[]", ReviewMode: RiskReviewFull, ActionMode: RiskActionBlock,
	}).Error)
	require.NoError(t, db.Create(&legacyRiskRecordMigrationFixture{
		RequestID: "legacy", ChannelID: 1, UserID: 1, Preview: "legacy preview",
		RuleIDs: []int{}, ProviderID: provider.Id, ProviderName: provider.Name, Result: RiskRecordResultSafe,
		Categories: []string{}, Chunks: []RiskRecordChunk{}, Source: RiskRecordSourceProvider,
	}).Error)

	// When
	require.NoError(t, prepareRiskSchemaMigration(db))
	require.NoError(t, db.AutoMigrate(&RiskPolicy{}, &RiskRecord{}))
	require.NoError(t, migrateRiskData(db))
	var migratedRecord RiskRecord
	require.NoError(t, db.Where("request_id = ?", "legacy").Take(&migratedRecord).Error)
	require.Equal(t, "legacy preview", migratedRecord.Preview)
	require.Equal(t, RiskProviderCloudflare, migratedRecord.ProviderType)
	var migratedPolicy RiskPolicy
	require.NoError(t, db.First(&migratedPolicy, riskPolicySingletonID).Error)
	migratedProviderIDs, err := decodeRiskPolicyList[int](migratedPolicy.ProviderIDs)
	require.NoError(t, err)
	require.Equal(t, []int{provider.Id}, migratedProviderIDs)
	require.Equal(t, "[24]", migratedPolicy.EnabledChannels)
	require.Equal(t, RiskReviewFull, migratedPolicy.ReviewMode)
	require.Equal(t, RiskActionBlock, migratedPolicy.ActionMode)
	largePreview := strings.Repeat("审", 1200)
	require.NoError(t, db.Model(&RiskRecord{}).Where("request_id = ?", "legacy").Update("preview", largePreview).Error)
	longProviderIDs := make([]int, 0, 600)
	for id := 1; id <= 600; id++ {
		longProviderIDs = append(longProviderIDs, id)
	}
	encodedProviderIDs, err := common.Marshal(longProviderIDs)
	require.NoError(t, err)
	require.NoError(t, db.Model(&RiskPolicy{}).Where("id = ?", riskPolicySingletonID).
		Update("provider_ids", string(encodedProviderIDs)).Error)

	// Then
	var record RiskRecord
	require.NoError(t, db.Where("request_id = ?", "legacy").Take(&record).Error)
	require.Equal(t, largePreview, record.Preview)
	require.Equal(t, RiskProviderCloudflare, record.ProviderType)
	var policy RiskPolicy
	require.NoError(t, db.First(&policy, riskPolicySingletonID).Error)
	providerIDs, err := decodeRiskPolicyList[int](policy.ProviderIDs)
	require.NoError(t, err)
	require.Equal(t, longProviderIDs, providerIDs)
	policyColumns, err := db.Migrator().ColumnTypes(&RiskPolicy{})
	require.NoError(t, err)
	providerIDsType := ""
	providerIDsNullable := true
	providerIDsHasDefault := true
	for _, column := range policyColumns {
		if column.Name() == "provider_ids" {
			providerIDsType = strings.ToLower(column.DatabaseTypeName())
			providerIDsNullable, _ = column.Nullable()
			_, providerIDsHasDefault = column.DefaultValue()
		}
	}
	require.Equal(t, "text", providerIDsType)
	require.False(t, providerIDsNullable)
	require.False(t, providerIDsHasDefault)
	recordColumns, err := db.Migrator().ColumnTypes(&RiskRecord{})
	require.NoError(t, err)
	previewType := ""
	for _, column := range recordColumns {
		if column.Name() == "preview" {
			previewType = strings.ToLower(column.DatabaseTypeName())
		}
	}
	require.Equal(t, "text", previewType)
}
