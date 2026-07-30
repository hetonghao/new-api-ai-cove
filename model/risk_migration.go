package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
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
		var provider RiskProvider
		err := db.Where("active = ?", true).Order("id asc").First(&provider).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load legacy active risk provider: %w", err)
		}
		if err == nil {
			providerIDs = []int{provider.Id}
			encoded, marshalErr := common.Marshal(providerIDs)
			if marshalErr != nil {
				return fmt.Errorf("encode migrated risk policy providers: %w", marshalErr)
			}
			if updateErr := db.Model(&policy).Update("provider_ids", string(encoded)).Error; updateErr != nil {
				return fmt.Errorf("migrate risk policy providers: %w", updateErr)
			}
		}
	}
	if err := db.Model(&RiskProvider{}).Where("active = ?", true).Update("active", false).Error; err != nil {
		return fmt.Errorf("clear legacy risk provider mirror: %w", err)
	}
	if len(providerIDs) > 0 {
		if err := db.Model(&RiskProvider{}).Where("id IN ?", providerIDs).Update("active", true).Error; err != nil {
			return fmt.Errorf("sync migrated risk provider mirror: %w", err)
		}
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
