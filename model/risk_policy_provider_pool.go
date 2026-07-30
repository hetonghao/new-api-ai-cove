package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func removeRiskProviderFromPolicy(tx *gorm.DB, providerID int) error {
	var policy RiskPolicy
	query := lockForUpdate(tx).Where("id = ?", riskPolicySingletonID).Limit(1).Find(&policy)
	if query.Error != nil {
		return fmt.Errorf("get risk policy for provider removal: %w", query.Error)
	}
	if query.RowsAffected == 0 {
		return nil
	}
	providerIDs, err := decodeRiskPolicyList[int](policy.ProviderIDs)
	if err != nil {
		return fmt.Errorf("decode risk policy providers for removal: %w", err)
	}
	remaining := make([]int, 0, len(providerIDs))
	for _, id := range providerIDs {
		if id != providerID {
			remaining = append(remaining, id)
		}
	}
	if len(remaining) == len(providerIDs) {
		return nil
	}
	encoded, err := common.Marshal(remaining)
	if err != nil {
		return fmt.Errorf("encode risk policy providers after removal: %w", err)
	}
	if err := tx.Model(&policy).Update("provider_ids", string(encoded)).Error; err != nil {
		return fmt.Errorf("remove risk provider from policy: %w", err)
	}
	return nil
}
