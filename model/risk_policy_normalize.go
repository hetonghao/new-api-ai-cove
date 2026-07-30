package model

import (
	"fmt"
	"strings"
)

func normalizeRiskPolicyInput(input RiskPolicyInput) (RiskPolicyInput, error) {
	if len(input.ProviderIDs) == 0 && input.ProviderID != nil {
		input.ProviderIDs = []int{*input.ProviderID}
	}
	if input.Enabled == nil {
		enabled := len(input.ProviderIDs) > 0 && len(input.EnabledChannels) > 0
		input.Enabled = &enabled
	}
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
	if *input.Enabled && (len(input.ProviderIDs) == 0 || len(input.EnabledChannels) == 0) {
		return RiskPolicyInput{}, fmt.Errorf("%w: enabled policy requires provider and channels", ErrInvalidRiskPolicy)
	}

	seenProviders := make(map[int]struct{}, len(input.ProviderIDs))
	providerIDs := make([]int, 0, len(input.ProviderIDs))
	for _, providerID := range input.ProviderIDs {
		if providerID < 1 {
			return RiskPolicyInput{}, fmt.Errorf("%w: provider id must be positive", ErrInvalidRiskPolicy)
		}
		if _, exists := seenProviders[providerID]; !exists {
			seenProviders[providerID] = struct{}{}
			providerIDs = append(providerIDs, providerID)
		}
	}
	input.ProviderIDs = providerIDs
	input.ProviderID = nil
	if len(providerIDs) > 0 {
		input.ProviderID = &input.ProviderIDs[0]
	}

	seenChannels := make(map[int]struct{}, len(input.EnabledChannels))
	channels := make([]int, 0, len(input.EnabledChannels))
	for _, channel := range input.EnabledChannels {
		if channel < 1 {
			return RiskPolicyInput{}, fmt.Errorf("%w: channel id must be positive", ErrInvalidRiskPolicy)
		}
		if _, exists := seenChannels[channel]; !exists {
			seenChannels[channel] = struct{}{}
			channels = append(channels, channel)
		}
	}
	input.EnabledChannels = channels

	seenUsers := make(map[int]struct{}, len(input.ExcludedUserIDs))
	excludedUserIDs := make([]int, 0, len(input.ExcludedUserIDs))
	for _, userID := range input.ExcludedUserIDs {
		if userID < 1 {
			return RiskPolicyInput{}, fmt.Errorf("%w: user id must be positive", ErrInvalidRiskPolicy)
		}
		if _, exists := seenUsers[userID]; !exists {
			seenUsers[userID] = struct{}{}
			excludedUserIDs = append(excludedUserIDs, userID)
		}
	}
	input.ExcludedUserIDs = excludedUserIDs

	seenModels := make(map[string]struct{}, len(input.ExcludedModels))
	excludedModels := make([]string, 0, len(input.ExcludedModels))
	for _, modelName := range input.ExcludedModels {
		modelName = strings.TrimSpace(modelName)
		if _, exists := seenModels[modelName]; modelName != "" && !exists {
			seenModels[modelName] = struct{}{}
			excludedModels = append(excludedModels, modelName)
		}
	}
	input.ExcludedModels = excludedModels

	return input, nil
}
