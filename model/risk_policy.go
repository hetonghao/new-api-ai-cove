package model

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type RiskReviewMode string
type RiskActionMode string

const (
	RiskReviewSelective   RiskReviewMode = "selective"
	RiskReviewFull        RiskReviewMode = "full"
	RiskActionObserve     RiskActionMode = "observe"
	RiskActionBlock       RiskActionMode = "block"
	riskPolicySingletonID                = 1
)

var ErrInvalidRiskPolicy = errors.New("invalid risk policy")

type RiskPolicy struct {
	Id              int            `json:"-" gorm:"primaryKey"`
	Enabled         bool           `json:"-" gorm:"not null;default:true"`
	ProviderIDs     string         `json:"-" gorm:"type:text;not null;default:'[]'"`
	EnabledChannels string         `json:"-" gorm:"type:text;not null"`
	ExcludedUserIDs string         `json:"-" gorm:"type:text;not null;default:'[]'"`
	ExcludedModels  string         `json:"-" gorm:"type:text;not null;default:'[]'"`
	ReviewMode      RiskReviewMode `json:"review_mode" gorm:"type:varchar(16);not null"`
	ActionMode      RiskActionMode `json:"action_mode" gorm:"type:varchar(16);not null"`
	CreatedAt       time.Time      `json:"-"`
	UpdatedAt       time.Time      `json:"-"`
}

type RiskPolicyInput struct {
	Enabled         *bool
	ProviderIDs     []int
	EnabledChannels []int
	ExcludedUserIDs []int
	ExcludedModels  []string
	ReviewMode      RiskReviewMode
	ActionMode      RiskActionMode
}

type RiskPolicyState struct {
	Configured      bool           `json:"configured"`
	Enabled         bool           `json:"enabled"`
	ProviderIDs     []int          `json:"provider_ids"`
	EnabledChannels []int          `json:"enabled_channels"`
	ExcludedUserIDs []int          `json:"excluded_user_ids"`
	ExcludedModels  []string       `json:"excluded_models"`
	ReviewMode      RiskReviewMode `json:"review_mode"`
	ActionMode      RiskActionMode `json:"action_mode"`
}

func (RiskPolicy) TableName() string {
	return "risk_policies"
}

func GetRiskPolicyState() (RiskPolicyState, error) {
	return getRiskPolicyState(0, "")
}

func GetRiskPolicyStateForRelay(userID int, modelName string) (RiskPolicyState, error) {
	return getRiskPolicyState(userID, modelName)
}

func getRiskPolicyState(relayUserID int, relayModel string) (RiskPolicyState, error) {
	state := RiskPolicyState{
		ProviderIDs:     []int{},
		EnabledChannels: []int{},
		ExcludedUserIDs: []int{},
		ExcludedModels:  []string{},
		ReviewMode:      RiskReviewSelective,
		ActionMode:      RiskActionObserve,
	}
	var policy RiskPolicy
	policyEnabled := false
	policyQuery := DB.Where("id = ?", riskPolicySingletonID).Limit(1).Find(&policy)
	if policyQuery.Error != nil {
		return RiskPolicyState{}, fmt.Errorf("get risk policy: %w", policyQuery.Error)
	}
	if policyQuery.RowsAffected > 0 {
		providerIDs, err := decodeRiskPolicyList[int](policy.ProviderIDs)
		if err != nil {
			return RiskPolicyState{}, fmt.Errorf("decode risk policy providers: %w", err)
		}
		enabledChannels, err := decodeRiskChannelIDs(policy.EnabledChannels)
		if err != nil {
			return RiskPolicyState{}, fmt.Errorf("decode risk policy channels: %w", err)
		}
		excludedUserIDs, err := decodeRiskPolicyList[int](policy.ExcludedUserIDs)
		if err != nil {
			return RiskPolicyState{}, fmt.Errorf("decode risk policy excluded users: %w", err)
		}
		excludedModels, err := decodeRiskPolicyList[string](policy.ExcludedModels)
		if err != nil {
			return RiskPolicyState{}, fmt.Errorf("decode risk policy excluded models: %w", err)
		}
		state.Configured = true
		policyEnabled = policy.Enabled
		state.ProviderIDs = providerIDs
		state.EnabledChannels = enabledChannels
		state.ExcludedUserIDs = excludedUserIDs
		state.ExcludedModels = excludedModels
		state.ReviewMode = policy.ReviewMode
		state.ActionMode = policy.ActionMode
	}
	if relayUserID > 0 && slices.Contains(state.ExcludedUserIDs, relayUserID) {
		return state, nil
	}
	if relayModel != "" && slices.Contains(state.ExcludedModels, relayModel) {
		return state, nil
	}

	state.Enabled = policyEnabled && len(state.ProviderIDs) > 0 && len(state.EnabledChannels) > 0
	return state, nil
}

func SaveRiskPolicy(input RiskPolicyInput) (RiskPolicyState, error) {
	input, err := normalizeRiskPolicyInput(input)
	if err != nil {
		return RiskPolicyState{}, err
	}
	providerIDs, err := common.Marshal(input.ProviderIDs)
	if err != nil {
		return RiskPolicyState{}, fmt.Errorf("encode risk policy providers: %w", err)
	}
	enabledChannels, err := common.Marshal(input.EnabledChannels)
	if err != nil {
		return RiskPolicyState{}, fmt.Errorf("encode risk policy channels: %w", err)
	}
	excludedUserIDs, err := common.Marshal(input.ExcludedUserIDs)
	if err != nil {
		return RiskPolicyState{}, fmt.Errorf("encode risk policy excluded users: %w", err)
	}
	excludedModels, err := common.Marshal(input.ExcludedModels)
	if err != nil {
		return RiskPolicyState{}, fmt.Errorf("encode risk policy excluded models: %w", err)
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if len(input.EnabledChannels) > 0 {
			var channelCount int64
			if err := tx.Model(&Channel{}).Where("id IN ?", input.EnabledChannels).Count(&channelCount).Error; err != nil {
				return fmt.Errorf("validate risk policy channels: %w", err)
			}
			if channelCount != int64(len(input.EnabledChannels)) {
				return fmt.Errorf("%w: channel does not exist", ErrInvalidRiskPolicy)
			}
		}
		if len(input.ExcludedUserIDs) > 0 {
			var userCount int64
			if err := tx.Model(&User{}).Where("id IN ?", input.ExcludedUserIDs).Count(&userCount).Error; err != nil {
				return fmt.Errorf("validate risk policy excluded users: %w", err)
			}
			if userCount != int64(len(input.ExcludedUserIDs)) {
				return fmt.Errorf("%w: user does not exist", ErrInvalidRiskPolicy)
			}
		}
		if len(input.ProviderIDs) > 0 {
			providers := make([]RiskProvider, 0, len(input.ProviderIDs))
			if err := lockForUpdate(tx).Where("id IN ?", input.ProviderIDs).Find(&providers).Error; err != nil {
				return fmt.Errorf("get risk providers: %w", err)
			}
			if len(providers) != len(input.ProviderIDs) {
				return fmt.Errorf("%w: provider does not exist", ErrInvalidRiskPolicy)
			}
			for _, provider := range providers {
				if provider.ValidatedAt == nil {
					return ErrRiskProviderNotValidated
				}
			}
		}
		if err := tx.Model(&RiskProvider{}).Where("active = ?", true).Update("active", false).Error; err != nil {
			return fmt.Errorf("clear risk provider pool mirror: %w", err)
		}
		if len(input.ProviderIDs) > 0 {
			if err := tx.Model(&RiskProvider{}).Where("id IN ?", input.ProviderIDs).Update("active", true).Error; err != nil {
				return fmt.Errorf("update risk provider pool mirror: %w", err)
			}
		}

		policy := RiskPolicy{Id: riskPolicySingletonID}
		values := map[string]any{
			"enabled":           *input.Enabled,
			"provider_ids":      string(providerIDs),
			"enabled_channels":  string(enabledChannels),
			"excluded_user_ids": string(excludedUserIDs),
			"excluded_models":   string(excludedModels),
			"review_mode":       input.ReviewMode,
			"action_mode":       input.ActionMode,
		}
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
		Enabled:         *input.Enabled && len(input.ProviderIDs) > 0 && len(input.EnabledChannels) > 0,
		ProviderIDs:     input.ProviderIDs,
		EnabledChannels: input.EnabledChannels,
		ExcludedUserIDs: input.ExcludedUserIDs,
		ExcludedModels:  input.ExcludedModels,
		ReviewMode:      input.ReviewMode,
		ActionMode:      input.ActionMode,
	}, nil
}

func decodeRiskPolicyList[T any](value string) ([]T, error) {
	if value == "" {
		return []T{}, nil
	}
	var values []T
	if err := common.UnmarshalJsonStr(value, &values); err != nil {
		return nil, ErrInvalidRiskPolicy
	}
	return values, nil
}

func decodeRiskChannelIDs(value string) ([]int, error) {
	var channelIDs []int
	if err := common.UnmarshalJsonStr(value, &channelIDs); err == nil {
		return channelIDs, nil
	}
	var legacyChannels []string
	if err := common.UnmarshalJsonStr(value, &legacyChannels); err == nil {
		return []int{}, nil
	}
	return nil, ErrInvalidRiskPolicy
}
