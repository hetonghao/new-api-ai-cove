package model

import (
	"fmt"
	"strings"
)

func normalizeRiskPolicyInput(input RiskPolicyInput) (RiskPolicyInput, error) {
	if input.Enabled == nil {
		enabled := false
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
	if *input.Enabled && len(input.EnabledChannels) == 0 {
		return RiskPolicyInput{}, fmt.Errorf("%w: enabled policy requires channels", ErrInvalidRiskPolicy)
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

	seenCategories := make(map[string]struct{}, len(input.NonBlockingCategories))
	nonBlockingCategories := make([]string, 0, len(input.NonBlockingCategories))
	for _, category := range input.NonBlockingCategories {
		category = strings.ToLower(strings.TrimSpace(category))
		if category == "" {
			continue
		}
		if len(category) > 128 {
			return RiskPolicyInput{}, fmt.Errorf("%w: non-blocking category is too long", ErrInvalidRiskPolicy)
		}
		if _, exists := seenCategories[category]; exists {
			continue
		}
		seenCategories[category] = struct{}{}
		nonBlockingCategories = append(nonBlockingCategories, category)
	}
	input.NonBlockingCategories = nonBlockingCategories

	return input, nil
}
