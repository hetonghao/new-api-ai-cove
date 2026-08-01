package model

import (
	"fmt"

	"gorm.io/gorm"
)

type riskPolicyProviderIDsMigration struct {
	ProviderIDs *string `gorm:"column:provider_ids;type:text"`
}

func (riskPolicyProviderIDsMigration) TableName() string {
	return "risk_policies"
}

func prepareRiskSchemaMigration(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable(&RiskPolicy{}) {
		return nil
	}
	if !migrator.HasColumn(&riskPolicyProviderIDsMigration{}, "ProviderIDs") {
		if err := migrator.AddColumn(&riskPolicyProviderIDsMigration{}, "ProviderIDs"); err != nil {
			return fmt.Errorf("add risk policy provider pool column: %w", err)
		}
	}
	if err := db.Table("risk_policies").
		Where("provider_ids IS NULL OR provider_ids = ?", "").
		Update("provider_ids", "[]").Error; err != nil {
		return fmt.Errorf("initialize risk policy provider pool column: %w", err)
	}
	return nil
}

func migrateRiskData(db *gorm.DB) error {
	if err := backfillLegacyRiskPolicyProviderIDs(db); err != nil {
		return err
	}
	if err := backfillRiskRecordProviderTypes(db); err != nil {
		return err
	}
	if err := backfillRiskRecordGovernancePreviewChars(db); err != nil {
		return err
	}
	return nil
}

func backfillRiskRecordGovernancePreviewChars(db *gorm.DB) error {
	if !db.Migrator().HasTable(&RiskRecordGovernance{}) {
		return nil
	}
	var governance struct {
		PreviewChars        int `gorm:"column:preview_chars"`
		SafePreviewChars    int `gorm:"column:safe_preview_chars"`
		NonSafePreviewChars int `gorm:"column:non_safe_preview_chars"`
	}
	query := db.Table("risk_record_governance").Where("id = ?", riskRecordGovernanceID).Limit(1).Find(&governance)
	if query.Error != nil {
		return fmt.Errorf("load risk record preview settings for migration: %w", query.Error)
	}
	if query.RowsAffected == 0 || (governance.SafePreviewChars != 0 && governance.NonSafePreviewChars != 0) {
		return nil
	}
	legacyPreviewChars := governance.PreviewChars
	if legacyPreviewChars < RiskRecordPreviewCharsMin {
		legacyPreviewChars = RiskRecordPreviewCharsDefault
	}
	updates := map[string]interface{}{}
	if governance.SafePreviewChars == 0 {
		updates["safe_preview_chars"] = legacyPreviewChars
	}
	if governance.NonSafePreviewChars == 0 {
		updates["non_safe_preview_chars"] = legacyPreviewChars
	}
	if err := db.Table("risk_record_governance").Where("id = ?", riskRecordGovernanceID).Updates(updates).Error; err != nil {
		return fmt.Errorf("backfill risk record preview settings: %w", err)
	}
	return nil
}

func backfillLegacyRiskPolicyProviderIDs(db *gorm.DB) error {
	var policy RiskPolicy
	query := db.Where("id = ?", riskPolicySingletonID).Limit(1).Find(&policy)
	if query.Error != nil {
		return fmt.Errorf("load risk policy for provider migration: %w", query.Error)
	}
	if query.RowsAffected == 0 {
		return nil
	}
	providerIDs, err := decodeRiskPolicyList[int](policy.ProviderIDs)
	if err != nil {
		return fmt.Errorf("decode risk policy providers for migration: %w", err)
	}
	if len(providerIDs) == 0 {
		return nil
	}
	if err := db.Model(&RiskProvider{}).Where("1 = 1").Update("active", false).Error; err != nil {
		return fmt.Errorf("disable legacy risk provider pool: %w", err)
	}
	if err := db.Model(&RiskProvider{}).Where("id IN ?", providerIDs).
		Where("validated_at IS NOT NULL").Update("active", true).Error; err != nil {
		return fmt.Errorf("sync migrated risk provider mirror: %w", err)
	}
	if err := db.Model(&RiskPolicy{}).Where("id = ?", riskPolicySingletonID).Update("provider_ids", "[]").Error; err != nil {
		return fmt.Errorf("clear migrated risk provider pool: %w", err)
	}
	return nil
}

func backfillRiskRecordProviderTypes(db *gorm.DB) error {
	for _, providerType := range []RiskProviderType{RiskProviderCloudflare, RiskProviderPlatformInternal} {
		providerIDs := make([]int, 0)
		if err := db.Model(&RiskProvider{}).Where("provider_type = ?", providerType).Pluck("id", &providerIDs).Error; err != nil {
			return fmt.Errorf("list %s risk providers for record backfill: %w", providerType, err)
		}
		if len(providerIDs) == 0 {
			continue
		}
		if err := db.Model(&RiskRecord{}).
			Where("provider_type = ? AND provider_id IN ?", "", providerIDs).
			Update("provider_type", providerType).Error; err != nil {
			return fmt.Errorf("backfill %s risk record providers: %w", providerType, err)
		}
	}
	return nil
}
