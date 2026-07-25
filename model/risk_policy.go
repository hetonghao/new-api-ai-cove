package model

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type RiskChannel string
type RiskReviewMode string
type RiskActionMode string

const (
	RiskChannelCPAPro     RiskChannel    = "cpa-pro"
	RiskReviewSelective   RiskReviewMode = "selective"
	RiskReviewFull        RiskReviewMode = "full"
	RiskActionObserve     RiskActionMode = "observe"
	RiskActionBlock       RiskActionMode = "block"
	riskPolicySingletonID                = 1
)

var ErrInvalidRiskPolicy = errors.New("invalid risk policy")

type RiskPolicy struct {
	Id              int            `json:"-" gorm:"primaryKey"`
	EnabledChannels []RiskChannel  `json:"enabled_channels" gorm:"serializer:json;type:text;not null"`
	ReviewMode      RiskReviewMode `json:"review_mode" gorm:"type:varchar(16);not null"`
	ActionMode      RiskActionMode `json:"action_mode" gorm:"type:varchar(16);not null"`
	CreatedAt       time.Time      `json:"-"`
	UpdatedAt       time.Time      `json:"-"`
}

type RiskPolicyInput struct {
	ProviderID      *int
	EnabledChannels []RiskChannel
	ReviewMode      RiskReviewMode
	ActionMode      RiskActionMode
}

type RiskPolicyState struct {
	Configured      bool           `json:"configured"`
	Enabled         bool           `json:"enabled"`
	ProviderID      *int           `json:"provider_id"`
	EnabledChannels []RiskChannel  `json:"enabled_channels"`
	ReviewMode      RiskReviewMode `json:"review_mode"`
	ActionMode      RiskActionMode `json:"action_mode"`
}

func (RiskPolicy) TableName() string {
	return "risk_policies"
}

func GetRiskPolicyState() (RiskPolicyState, error) {
	state := RiskPolicyState{
		EnabledChannels: []RiskChannel{},
		ReviewMode:      RiskReviewSelective,
		ActionMode:      RiskActionObserve,
	}
	var policy RiskPolicy
	policyQuery := DB.Where("id = ?", riskPolicySingletonID).Limit(1).Find(&policy)
	if policyQuery.Error != nil {
		return RiskPolicyState{}, fmt.Errorf("get risk policy: %w", policyQuery.Error)
	}
	if policyQuery.RowsAffected > 0 {
		state.Configured = true
		state.EnabledChannels = policy.EnabledChannels
		state.ReviewMode = policy.ReviewMode
		state.ActionMode = policy.ActionMode
	}

	var provider RiskProvider
	providerQuery := DB.Where("active = ?", true).Limit(1).Find(&provider)
	if providerQuery.Error != nil {
		return RiskPolicyState{}, fmt.Errorf("get active risk provider: %w", providerQuery.Error)
	}
	if providerQuery.RowsAffected > 0 {
		state.ProviderID = &provider.Id
	}
	state.Enabled = state.Configured && state.ProviderID != nil && len(state.EnabledChannels) > 0
	return state, nil
}

func SaveRiskPolicy(input RiskPolicyInput) (RiskPolicyState, error) {
	input, err := normalizeRiskPolicyInput(input)
	if err != nil {
		return RiskPolicyState{}, err
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if input.ProviderID == nil {
			if err := tx.Model(&RiskProvider{}).Where("active = ?", true).Update("active", false).Error; err != nil {
				return fmt.Errorf("clear active risk provider: %w", err)
			}
		} else {
			var provider RiskProvider
			if err := lockForUpdate(tx).First(&provider, *input.ProviderID).Error; err != nil {
				return fmt.Errorf("get risk provider %d: %w", *input.ProviderID, err)
			}
			if provider.ValidatedAt == nil {
				return ErrRiskProviderNotValidated
			}
			if err := tx.Model(&RiskProvider{}).Where("active = ?", true).Update("active", false).Error; err != nil {
				return fmt.Errorf("deactivate risk providers: %w", err)
			}
			if err := tx.Model(&provider).Update("active", true).Error; err != nil {
				return fmt.Errorf("activate risk provider %d: %w", provider.Id, err)
			}
		}

		policy := RiskPolicy{Id: riskPolicySingletonID}
		values := RiskPolicy{EnabledChannels: input.EnabledChannels, ReviewMode: input.ReviewMode, ActionMode: input.ActionMode}
		if err := tx.Where("id = ?", riskPolicySingletonID).Assign(values).FirstOrCreate(&policy).Error; err != nil {
			return fmt.Errorf("save risk policy: %w", err)
		}
		return nil
	})
	if err != nil {
		return RiskPolicyState{}, err
	}
	return RiskPolicyState{
		Configured:      true,
		Enabled:         input.ProviderID != nil && len(input.EnabledChannels) > 0,
		ProviderID:      input.ProviderID,
		EnabledChannels: input.EnabledChannels,
		ReviewMode:      input.ReviewMode,
		ActionMode:      input.ActionMode,
	}, nil
}

func normalizeRiskPolicyInput(input RiskPolicyInput) (RiskPolicyInput, error) {
	if input.ReviewMode == "" {
		input.ReviewMode = RiskReviewSelective
	}
	if input.ActionMode == "" {
		input.ActionMode = RiskActionObserve
	}
	switch input.ReviewMode {
	case RiskReviewSelective, RiskReviewFull:
	default:
		return RiskPolicyInput{}, fmt.Errorf("%w: unsupported review mode", ErrInvalidRiskPolicy)
	}
	switch input.ActionMode {
	case RiskActionObserve, RiskActionBlock:
	default:
		return RiskPolicyInput{}, fmt.Errorf("%w: unsupported action mode", ErrInvalidRiskPolicy)
	}
	if input.ProviderID == nil && len(input.EnabledChannels) > 0 {
		return RiskPolicyInput{}, fmt.Errorf("%w: provider is required for enabled channels", ErrInvalidRiskPolicy)
	}
	seen := make(map[RiskChannel]struct{}, len(input.EnabledChannels))
	channels := make([]RiskChannel, 0, len(input.EnabledChannels))
	for _, channel := range input.EnabledChannels {
		if channel != RiskChannelCPAPro {
			return RiskPolicyInput{}, fmt.Errorf("%w: unsupported channel %q", ErrInvalidRiskPolicy, channel)
		}
		if _, exists := seen[channel]; exists {
			continue
		}
		seen[channel] = struct{}{}
		channels = append(channels, channel)
	}
	input.EnabledChannels = channels
	if input.ProviderID != nil && *input.ProviderID < 1 {
		return RiskPolicyInput{}, fmt.Errorf("%w: provider id must be positive", ErrInvalidRiskPolicy)
	}
	return input, nil
}
