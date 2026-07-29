package model

import (
	"fmt"
	"strings"
)

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

	if input.ProviderID != nil && *input.ProviderID < 1 {
		return RiskPolicyInput{}, fmt.Errorf("%w: provider id must be positive", ErrInvalidRiskPolicy)
	}
	return input, nil
}
