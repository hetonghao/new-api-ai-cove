package model

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type RiskProviderType string

const (
	RiskProviderCloudflare       RiskProviderType = "cloudflare"
	RiskProviderPlatformInternal RiskProviderType = "platform_internal"
)

const (
	DefaultRiskProviderTimeoutMs               = 800
	DefaultRiskProviderFailureThreshold        = 5
	DefaultRiskProviderCooldownSeconds         = 30
	DefaultRiskProviderPriority                = 0
	DefaultRiskProviderDailyNeuronsLimit int64 = 10000
	DefaultRiskProviderDailyResetTime          = "08:00"
)

var ErrRiskProviderNotValidated = errors.New("risk provider is not validated")

var cloudflareAccountIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type RiskProvider struct {
	Id                  int              `json:"id" gorm:"primaryKey"`
	Name                string           `json:"name" gorm:"type:varchar(128);not null"`
	ProviderType        RiskProviderType `json:"provider_type" gorm:"type:varchar(32);not null"`
	AccountID           string           `json:"account_id" gorm:"type:varchar(64);not null;default:''"`
	ChannelID           int              `json:"channel_id" gorm:"index"`
	Model               string           `json:"model" gorm:"type:varchar(256);not null"`
	BaseURL             string           `json:"base_url" gorm:"type:varchar(1024);not null"`
	CredentialEncrypted string           `json:"-" gorm:"type:text;not null"`
	InternalTokenID     int              `json:"-" gorm:"index"`
	TimeoutMs           int              `json:"timeout_ms" gorm:"not null"`
	FailureThreshold    int              `json:"failure_threshold" gorm:"not null"`
	CooldownSeconds     int              `json:"cooldown_seconds" gorm:"not null"`
	Priority            int              `json:"priority" gorm:"not null;default:0;index"`
	DailyNeuronsLimit   int64            `json:"daily_neurons_limit" gorm:"not null;default:10000"`
	DailyResetTime      string           `json:"daily_reset_time" gorm:"type:char(5);not null;default:'08:00'"`
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
	if err := DB.Order("priority desc, id asc").Find(&providers).Error; err != nil {
		return nil, fmt.Errorf("list risk providers: %w", err)
	}
	return providers, nil
}

func GetEnabledRiskProviders() ([]*RiskProvider, error) {
	var providers []*RiskProvider
	if err := DB.Where("active = ? AND validated_at IS NOT NULL", true).
		Order("priority desc, id asc").Find(&providers).Error; err != nil {
		return nil, fmt.Errorf("list enabled risk providers: %w", err)
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
	if provider.ProviderType != RiskProviderPlatformInternal {
		if err := DB.Create(provider).Error; err != nil {
			return fmt.Errorf("create risk provider: %w", err)
		}
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := validatePlatformInternalRiskChannel(tx, provider); err != nil {
			return err
		}
		if err := tx.Create(provider).Error; err != nil {
			return fmt.Errorf("create risk provider: %w", err)
		}
		token, err := createPlatformInternalRiskToken(tx, provider)
		if err != nil {
			return err
		}
		provider.InternalTokenID = token.Id
		if err := tx.Model(provider).Update("internal_token_id", token.Id).Error; err != nil {
			return fmt.Errorf("attach internal risk token: %w", err)
		}
		return nil
	})
}

func MarkRiskProviderValidated(id int) error {
	validatedAt := time.Now().UTC()
	if err := DB.Model(&RiskProvider{}).Where("id = ?", id).Update("validated_at", validatedAt).Error; err != nil {
		return fmt.Errorf("mark risk provider %d validated: %w", id, err)
	}
	return nil
}

func normalizeRiskProvider(provider *RiskProvider) error {
	if provider == nil {
		return errors.New("risk provider is required")
	}
	provider.Name = strings.TrimSpace(provider.Name)
	provider.Model = strings.TrimSpace(provider.Model)
	switch provider.ProviderType {
	case RiskProviderCloudflare:
		accountID, err := provider.CloudflareAccountID()
		if err != nil {
			return err
		}
		provider.AccountID = accountID
		provider.ChannelID = 0
		if provider.Name == "" || provider.Model == "" || provider.CredentialEncrypted == "" {
			return errors.New("risk provider name, account ID, model, and credential are required")
		}
	case RiskProviderPlatformInternal:
		provider.AccountID = ""
		provider.BaseURL = ""
		provider.CredentialEncrypted = ""
		if provider.Name == "" || provider.Model == "" || provider.ChannelID < 1 {
			return errors.New("risk provider name, channel, and model are required")
		}
	default:
		return errors.New("unsupported risk provider type")
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
	if provider.DailyNeuronsLimit == 0 {
		provider.DailyNeuronsLimit = DefaultRiskProviderDailyNeuronsLimit
	}
	if provider.DailyResetTime == "" {
		provider.DailyResetTime = DefaultRiskProviderDailyResetTime
	}
	if provider.TimeoutMs < 1 || provider.TimeoutMs > 60000 || provider.FailureThreshold < 1 || provider.FailureThreshold > 100 || provider.CooldownSeconds < 1 || provider.CooldownSeconds > 86400 {
		return errors.New("risk provider timeout or circuit breaker value is out of range")
	}
	if provider.DailyNeuronsLimit < 1 || provider.DailyNeuronsLimit > 1_000_000_000_000 {
		return errors.New("risk provider daily Neurons limit is out of range")
	}
	if _, err := ParseRiskProviderDailyResetTime(provider.DailyResetTime); err != nil {
		return err
	}
	return nil
}

func ParseRiskProviderDailyResetTime(value string) (int, error) {
	if len(value) != 5 || value[2] != ':' {
		return 0, errors.New("risk provider daily reset time must use HH:mm")
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	if value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' || value[3] < '0' || value[3] > '9' || hour > 23 || minute > 59 {
		return 0, errors.New("risk provider daily reset time must use HH:mm")
	}
	return hour*60 + minute, nil
}

func validatePlatformInternalRiskChannel(tx *gorm.DB, provider *RiskProvider) error {
	var channel Channel
	if err := tx.First(&channel, provider.ChannelID).Error; err != nil {
		return fmt.Errorf("get internal risk channel %d: %w", provider.ChannelID, err)
	}
	if channel.Status != common.ChannelStatusEnabled {
		return errors.New("internal risk channel is disabled")
	}
	for _, modelName := range channel.GetModels() {
		if strings.TrimSpace(modelName) == provider.Model {
			return nil
		}
	}
	return fmt.Errorf("internal risk channel does not support model %s", provider.Model)
}

func createPlatformInternalRiskToken(tx *gorm.DB, provider *RiskProvider) (*Token, error) {
	var root User
	if err := tx.Select("id").Where("role = ?", common.RoleRootUser).Order("id asc").First(&root).Error; err != nil {
		return nil, fmt.Errorf("resolve root user: %w", err)
	}
	if root.Id < 1 {
		return nil, errors.New("root user is required for internal risk provider")
	}
	key, err := common.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate internal risk token: %w", err)
	}
	allowIPs := "127.0.0.1/32\n::1/128"
	now := common.GetTimestamp()
	token := &Token{
		UserId: root.Id, Key: key, Name: fmt.Sprintf("AI 风控内部审核 #%d", provider.Id),
		Status: common.TokenStatusEnabled, CreatedTime: now, AccessedTime: now, ExpiredTime: -1,
		UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: provider.Model,
		AllowIps: &allowIPs, SystemManaged: true,
	}
	if err := tx.Create(token).Error; err != nil {
		return nil, fmt.Errorf("create internal risk token: %w", err)
	}
	return token, nil
}

func IsPlatformInternalRiskTokenID(tokenID int) (bool, error) {
	_, linked, err := GetPlatformInternalRiskTokenChannelID(tokenID)
	return linked, err
}

func GetPlatformInternalRiskTokenChannelID(tokenID int) (int, bool, error) {
	if tokenID < 1 {
		return 0, false, nil
	}
	var provider RiskProvider
	if err := DB.Model(&RiskProvider{}).Select("channel_id").
		Where("provider_type = ? AND internal_token_id = ?", RiskProviderPlatformInternal, tokenID).
		First(&provider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("check internal risk token: %w", err)
	}
	return provider.ChannelID, true, nil
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
