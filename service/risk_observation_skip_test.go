package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestProcessRiskObservationForRelay_skips_cloud_review_for_enabled_skip_rule(t *testing.T) {
	tests := []struct {
		name       string
		reviewMode model.RiskReviewMode
		actionMode model.RiskActionMode
	}{
		{name: "selective observe", reviewMode: model.RiskReviewSelective, actionMode: model.RiskActionObserve},
		{name: "selective block", reviewMode: model.RiskReviewSelective, actionMode: model.RiskActionBlock},
		{name: "full observe", reviewMode: model.RiskReviewFull, actionMode: model.RiskActionObserve},
		{name: "full block", reviewMode: model.RiskReviewFull, actionMode: model.RiskActionBlock},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			providerID := 17
			executorCalls := 0
			queueCalls := 0
			var completed RiskObservationEvent
			deps := riskObservationRelayDeps{
				loadPolicy: func() (model.RiskPolicyState, error) {
					return model.RiskPolicyState{
						Enabled: true, ProviderIDs: []int{providerID}, EnabledChannels: []int{24},
						ReviewMode: test.reviewMode, ActionMode: test.actionMode,
					}, nil
				},
				loadRules: func() ([]*model.RiskRule, error) {
					return []*model.RiskRule{
						{Id: 7, RuleType: model.RiskRuleRegex, Pattern: `^\s*HEARTBEAT`, Enabled: true, Action: model.RiskRuleActionSkip},
						{Id: 8, RuleType: model.RiskRuleKeyword, Pattern: "heartbeat", Enabled: true, Action: model.RiskRuleActionReview},
					}, nil
				},
				executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
					executorCalls++
					return RiskModerationOutcome{}, nil
				}),
				enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
					queueCalls++
					return queuedRiskObservationResult()
				},
				enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
					completed = event
					return queuedRiskObservationResult()
				},
			}

			// When
			decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
				RequestID: "skip", ChannelID: 24, UserID: 42, Model: "gpt-test", Text: "  HEARTBEAT   ready  ",
			}, deps)

			// Then
			require.False(t, decision.Blocked)
			require.Nil(t, decision.DirectRecord)
			require.Zero(t, executorCalls)
			require.Zero(t, queueCalls)
			require.Equal(t, RiskObservationNotReviewed, completed.Result)
			require.Equal(t, RiskObservationSourceLocal, completed.Source)
			require.Equal(t, []int{7}, completed.RuleIDs)
			require.False(t, completed.ProviderCalled)
			require.Zero(t, completed.ProviderID)
		})
	}
}

func TestProcessRiskObservationForRelay_ignores_disabled_skip_rule(t *testing.T) {
	// Given
	providerID := 17
	queueCalls := 0
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled: true, ProviderIDs: []int{providerID}, EnabledChannels: []int{24},
				ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionObserve,
			}, nil
		},
		loadRules: func() ([]*model.RiskRule, error) {
			return []*model.RiskRule{{
				Id: 7, RuleType: model.RiskRuleRegex, Pattern: `^heartbeat`, Enabled: false, Action: model.RiskRuleActionSkip,
			}}, nil
		},
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			queueCalls++
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "review", ChannelID: 24, UserID: 42, Model: "gpt-test", Text: "heartbeat ready",
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Equal(t, 1, queueCalls)
}

func TestProcessRiskObservationForRelay_does_not_skip_similar_non_prefix_text(t *testing.T) {
	// Given
	providerID := 17
	queueCalls := 0
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled: true, ProviderIDs: []int{providerID}, EnabledChannels: []int{24},
				ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionObserve,
			}, nil
		},
		loadRules: func() ([]*model.RiskRule, error) {
			return []*model.RiskRule{{
				Id: 7, RuleType: model.RiskRuleRegex, Pattern: `^heartbeat`, Enabled: true, Action: model.RiskRuleActionSkip,
			}}, nil
		},
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			queueCalls++
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "review", ChannelID: 24, UserID: 42, Model: "gpt-test", Text: "status heartbeat ready",
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Equal(t, 1, queueCalls)
}

func TestProcessRiskObservationForRelay_skips_excluded_original_model(t *testing.T) {
	// Given
	providerID := 17
	ruleCalls := 0
	executorCalls := 0
	queueCalls := 0
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled: true, ProviderIDs: []int{providerID}, EnabledChannels: []int{24}, ExcludedModels: []string{"codex-auto-review"},
				ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionBlock,
			}, nil
		},
		loadRules: func() ([]*model.RiskRule, error) {
			ruleCalls++
			return nil, nil
		},
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			return RiskModerationOutcome{}, nil
		}),
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			queueCalls++
			return queuedRiskObservationResult()
		},
		enqueueEvent: func(RiskObservationEvent) RiskObservationEnqueueResult {
			queueCalls++
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "excluded-model", ChannelID: 24, UserID: 42, Model: "codex-auto-review", Text: "current",
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Zero(t, ruleCalls)
	require.Zero(t, executorCalls)
	require.Zero(t, queueCalls)
}

func TestProcessRiskObservationForRelay_does_not_skip_different_original_model(t *testing.T) {
	// Given
	providerID := 17
	queueCalls := 0
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled: true, ProviderIDs: []int{providerID}, EnabledChannels: []int{24}, ExcludedModels: []string{"codex-auto-review"},
				ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionObserve,
			}, nil
		},
		loadRules: func() ([]*model.RiskRule, error) { return nil, nil },
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			queueCalls++
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "other-model", ChannelID: 24, UserID: 42, Model: "codex-auto-review-upstream", Text: "current",
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Equal(t, 1, queueCalls)
}
