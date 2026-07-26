package model

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

type RiskProviderType string

const RiskProviderCloudflare RiskProviderType = "cloudflare"

const (
	DefaultRiskProviderTimeoutMs        = 800
	DefaultRiskProviderFailureThreshold = 5
	DefaultRiskProviderCooldownSeconds  = 30
)

var ErrRiskProviderNotValidated = errors.New("risk provider is not validated")

var cloudflareAccountIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type RiskProvider struct {
	Id                  int              `json:"id" gorm:"primaryKey"`
	Name                string           `json:"name" gorm:"type:varchar(128);not null"`
	ProviderType        RiskProviderType `json:"provider_type" gorm:"type:varchar(32);not null"`
	AccountID           string           `json:"account_id" gorm:"type:varchar(64);not null;default:''"`
	Model               string           `json:"model" gorm:"type:varchar(256);not null"`
	BaseURL             string           `json:"base_url" gorm:"type:varchar(1024);not null"`
	CredentialEncrypted string           `json:"-" gorm:"type:text;not null"`
	TimeoutMs           int              `json:"timeout_ms" gorm:"not null"`
	FailureThreshold    int              `json:"failure_threshold" gorm:"not null"`
	CooldownSeconds     int              `json:"cooldown_seconds" gorm:"not null"`
	ValidatedAt         *time.Time       `json:"validated_at"`
	Active              bool             `json:"active" gorm:"not null"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

func (RiskProvider) TableName() string {
	return "risk_providers"
}

func GetRiskProviders() ([]*RiskProvider, error) {
	var providers []*RiskProvider
	if err := DB.Order("id asc").Find(&providers).Error; err != nil {
		return nil, fmt.Errorf("list risk providers: %w", err)
	}
	return providers, nil
}

func GetRiskProviderByID(id int) (*RiskProvider, error) {
	var provider RiskProvider
	if err := DB.First(&provider, id).Error; err != nil {
		return nil, fmt.Errorf("get risk provider %d: %w", id, err)
	}
	return &provider, nil
}

func CreateRiskProvider(provider *RiskProvider) error {
	if err := normalizeRiskProvider(provider); err != nil {
		return err
	}
	provider.Active = false
	provider.ValidatedAt = nil
	if err := DB.Create(provider).Error; err != nil {
		return fmt.Errorf("create risk provider: %w", err)
	}
	return nil
}

func UpdateRiskProvider(provider *RiskProvider) error {
	if err := normalizeRiskProvider(provider); err != nil {
		return err
	}
	if err := DB.Save(provider).Error; err != nil {
		return fmt.Errorf("update risk provider %d: %w", provider.Id, err)
	}
	return nil
}

func DeleteRiskProvider(id int) error {
	if err := DB.Delete(&RiskProvider{}, id).Error; err != nil {
		return fmt.Errorf("delete risk provider %d: %w", id, err)
	}
	return nil
}

func MarkRiskProviderValidated(id int) error {
	validatedAt := time.Now().UTC()
	if err := DB.Model(&RiskProvider{}).Where("id = ?", id).Update("validated_at", validatedAt).Error; err != nil {
		return fmt.Errorf("mark risk provider %d validated: %w", id, err)
	}
	return nil
}

func ActivateRiskProvider(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var provider RiskProvider
		if err := lockForUpdate(tx).First(&provider, id).Error; err != nil {
			return fmt.Errorf("get risk provider %d for activation: %w", id, err)
		}
		if provider.ValidatedAt == nil {
			return ErrRiskProviderNotValidated
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

func normalizeRiskProvider(provider *RiskProvider) error {
	if provider == nil {
		return errors.New("risk provider is required")
	}
	provider.Name = strings.TrimSpace(provider.Name)
	provider.Model = strings.TrimSpace(provider.Model)
	if provider.ProviderType != RiskProviderCloudflare {
		return errors.New("unsupported risk provider type")
	}
	accountID, err := provider.CloudflareAccountID()
	if err != nil {
		return err
	}
	provider.AccountID = accountID
	if provider.Name == "" || provider.Model == "" || provider.CredentialEncrypted == "" {
		return errors.New("risk provider name, account ID, model, and credential are required")
	}
	if provider.TimeoutMs == 0 {
		provider.TimeoutMs = DefaultRiskProviderTimeoutMs
	}
	if provider.FailureThreshold == 0 {
		provider.FailureThreshold = DefaultRiskProviderFailureThreshold
	}
	if provider.CooldownSeconds == 0 {
		provider.CooldownSeconds = DefaultRiskProviderCooldownSeconds
	}
	if provider.TimeoutMs < 1 || provider.TimeoutMs > 60000 || provider.FailureThreshold < 1 || provider.FailureThreshold > 100 || provider.CooldownSeconds < 1 || provider.CooldownSeconds > 86400 {
		return errors.New("risk provider timeout or circuit breaker value is out of range")
	}
	return nil
}

func (provider *RiskProvider) CloudflareAccountID() (string, error) {
	if provider == nil {
		return "", errors.New("risk provider is required")
	}
	accountID := strings.ToLower(strings.TrimSpace(provider.AccountID))
	if accountID != "" {
		if !cloudflareAccountIDPattern.MatchString(accountID) {
			return "", errors.New("invalid Cloudflare account ID")
		}
		return accountID, nil
	}

	parsedURL, err := url.Parse(strings.TrimSpace(provider.BaseURL))
	if err != nil || parsedURL.Host == "" {
		return "", errors.New("Cloudflare account ID is required")
	}
	parts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	for index := 0; index+3 < len(parts); index++ {
		if parts[index] != "accounts" || parts[index+2] != "ai" || parts[index+3] != "run" {
			continue
		}
		accountID = strings.ToLower(parts[index+1])
		if cloudflareAccountIDPattern.MatchString(accountID) {
			return accountID, nil
		}
	}
	return "", errors.New("Cloudflare account ID is required")
}
