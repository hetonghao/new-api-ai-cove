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
	return SetRiskProviderActive(id, true)
}

func SetRiskProviderActive(id int, active bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var provider RiskProvider
		if err := lockForUpdate(tx).First(&provider, id).Error; err != nil {
			return fmt.Errorf("get risk provider %d for activation: %w", id, err)
		}
		if active && provider.ValidatedAt == nil {
			return ErrRiskProviderNotValidated
		}
		if err := tx.Model(&provider).Update("active", active).Error; err != nil {
			return fmt.Errorf("set risk provider %d active=%t: %w", id, active, err)
		}
		return nil
	})
}
