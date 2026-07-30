package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func UpdateRiskProvider(provider *RiskProvider) error {
	if err := normalizeRiskProvider(provider); err != nil {
		return err
	}
	if provider.ValidatedAt == nil {
		provider.Active = false
	}
	var invalidatedTokens []Token
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing RiskProvider
		if err := lockForUpdate(tx).First(&existing, provider.Id).Error; err != nil {
			return fmt.Errorf("get risk provider %d for update: %w", provider.Id, err)
		}
		if provider.ProviderType == RiskProviderPlatformInternal {
			if err := validatePlatformInternalRiskChannel(tx, provider); err != nil {
				return err
			}
			if existing.ProviderType == RiskProviderPlatformInternal && existing.InternalTokenID > 0 {
				var token Token
				if err := lockForUpdate(tx).First(&token, existing.InternalTokenID).Error; err != nil {
					return fmt.Errorf("get internal risk token: %w", err)
				}
				if !token.SystemManaged {
					return errors.New("internal risk token is not system managed")
				}
				token.Status = common.TokenStatusEnabled
				token.ModelLimitsEnabled = true
				token.ModelLimits = provider.Model
				if err := tx.Model(&token).Select("status", "model_limits_enabled", "model_limits").Updates(&token).Error; err != nil {
					return fmt.Errorf("update internal risk token: %w", err)
				}
				provider.InternalTokenID = token.Id
				invalidatedTokens = append(invalidatedTokens, token)
			} else {
				token, err := createPlatformInternalRiskToken(tx, provider)
				if err != nil {
					return err
				}
				provider.InternalTokenID = token.Id
				invalidatedTokens = append(invalidatedTokens, *token)
			}
		} else {
			provider.InternalTokenID = 0
			if existing.ProviderType == RiskProviderPlatformInternal && existing.InternalTokenID > 0 {
				var token Token
				if err := lockForUpdate(tx).First(&token, existing.InternalTokenID).Error; err != nil {
					return fmt.Errorf("get internal risk token: %w", err)
				}
				if err := tx.Model(&token).Update("status", common.TokenStatusDisabled).Error; err != nil {
					return fmt.Errorf("disable internal risk token: %w", err)
				}
				invalidatedTokens = append(invalidatedTokens, token)
			}
		}
		if err := tx.Save(provider).Error; err != nil {
			return fmt.Errorf("update risk provider %d: %w", provider.Id, err)
		}
		if provider.ValidatedAt == nil {
			if err := removeRiskProviderFromPolicy(tx, provider.Id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := invalidateTokensCache(invalidatedTokens); err != nil {
		common.SysLog("failed to invalidate internal risk token cache: " + err.Error())
	}
	return nil
}

func DeleteRiskProvider(id int) error {
	var invalidatedTokens []Token
	err := DB.Transaction(func(tx *gorm.DB) error {
		var provider RiskProvider
		if err := lockForUpdate(tx).First(&provider, id).Error; err != nil {
			return fmt.Errorf("get risk provider %d for deletion: %w", id, err)
		}
		if provider.ProviderType == RiskProviderPlatformInternal && provider.InternalTokenID > 0 {
			var token Token
			if err := lockForUpdate(tx).First(&token, provider.InternalTokenID).Error; err != nil {
				return fmt.Errorf("get internal risk token: %w", err)
			}
			if err := tx.Model(&token).Update("status", common.TokenStatusDisabled).Error; err != nil {
				return fmt.Errorf("disable internal risk token: %w", err)
			}
			invalidatedTokens = append(invalidatedTokens, token)
		}
		if err := removeRiskProviderFromPolicy(tx, id); err != nil {
			return err
		}
		if err := tx.Delete(&RiskProvider{}, id).Error; err != nil {
			return fmt.Errorf("delete risk provider %d: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := invalidateTokensCache(invalidatedTokens); err != nil {
		common.SysLog("failed to invalidate internal risk token cache: " + err.Error())
	}
	return nil
}

func ActivateRiskProvider(id int) error {
	providerIDs, err := common.Marshal([]int{id})
	if err != nil {
		return fmt.Errorf("encode active risk provider: %w", err)
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var provider RiskProvider
		if err := lockForUpdate(tx).First(&provider, id).Error; err != nil {
			return fmt.Errorf("get risk provider %d for activation: %w", id, err)
		}
		if provider.ValidatedAt == nil {
			return ErrRiskProviderNotValidated
		}
		var policy RiskPolicy
		policyQuery := lockForUpdate(tx).Where("id = ?", riskPolicySingletonID).Limit(1).Find(&policy)
		if policyQuery.Error != nil {
			return fmt.Errorf("get risk policy for activation: %w", policyQuery.Error)
		}
		if policyQuery.RowsAffected == 0 {
			policy = RiskPolicy{
				Id: riskPolicySingletonID, ProviderIDs: string(providerIDs), EnabledChannels: "[]",
				ExcludedUserIDs: "[]", ExcludedModels: "[]",
				ReviewMode: RiskReviewSelective, ActionMode: RiskActionObserve,
			}
			if err := tx.Create(&policy).Error; err != nil {
				return fmt.Errorf("create risk policy for activation: %w", err)
			}
		} else if err := tx.Model(&policy).Update("provider_ids", string(providerIDs)).Error; err != nil {
			return fmt.Errorf("replace risk provider pool: %w", err)
		}
		if err := tx.Model(&RiskProvider{}).Where("active = ?", true).Update("active", false).Error; err != nil {
			return fmt.Errorf("deactivate risk providers: %w", err)
		}
		if err := tx.Model(&provider).Update("active", true).Error; err != nil {
			return fmt.Errorf("activate risk provider %d: %w", id, err)
		}
		return nil
	})
}
