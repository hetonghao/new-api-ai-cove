package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestProcessRiskObservationForRelay_records_not_reviewed_when_selective_rules_do_not_match(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	providerID := provider.Id
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderID:      &providerID,
		EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
		ReviewMode:      model.RiskReviewSelective,
		ActionMode:      model.RiskActionBlock,
	})
	require.NoError(t, err)
	executorCalls := 0
	var completed RiskObservationEvent
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			return RiskModerationOutcome{}, nil
		}),
		enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
			completed = event
			return queuedRiskObservationResult()
		},
	}
	job := RiskObservationJob{
		RequestID: "no-hit", ChannelID: 24, ChannelName: "cpa-pro", UserID: 42, TokenID: 9,
		Model: "gpt-test", Path: "/v1/responses", Text: "ordinary current-turn text",
	}
	wantMetadata := BuildRiskRecordContentMetadata(job.Text)

	// When
	decision := processRiskObservationForRelay(context.Background(), job, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Zero(t, executorCalls)
	require.Equal(t, RiskObservationNotReviewed, completed.Result)
	require.Equal(t, RiskObservationSourceLocal, completed.Source)
	require.False(t, completed.Blocked)
	require.Zero(t, completed.ProviderID)
	require.Empty(t, completed.ProviderName)
	require.Equal(t, job.TokenID, completed.TokenID)
	require.Equal(t, job.Model, completed.Model)
	require.Equal(t, job.Path, completed.Path)
	require.Equal(t, wantMetadata.Preview, completed.Preview)
	require.Equal(t, wantMetadata.ContentHash, completed.ContentHash)
}

func TestProcessRiskObservationForRelay_sends_bounded_excerpt_when_selective_rule_matches(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	providerID := provider.Id
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderID:      &providerID,
		EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
		ReviewMode:      model.RiskReviewSelective,
		ActionMode:      model.RiskActionBlock,
	})
	require.NoError(t, err)
	rule, err := model.CreateRiskRule(model.RiskRuleInput{
		RuleType: model.RiskRuleKeyword, Pattern: "danger", Enabled: true,
	})
	require.NoError(t, err)
	text := strings.Repeat("safe ", 900) + "danger" + strings.Repeat(" tail", 900)
	var reviewed RiskModerationInput
	var completed RiskObservationEvent
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(_ context.Context, input RiskModerationInput) (RiskModerationOutcome, error) {
			reviewed = input
			return RiskModerationOutcome{
				Result:   RiskReviewResult{Status: RiskReviewSafe},
				Source:   RiskReviewSourceCache,
				CacheHit: true,
			}, nil
		}),
		enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
			completed = event
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "hit", ChannelID: 24, ChannelName: "cpa-pro", UserID: 42, Text: text,
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Equal(t, model.RiskReviewSelective, reviewed.ReviewMode)
	require.LessOrEqual(t, len([]rune(reviewed.Content)), riskExcerptLimit)
	require.Contains(t, reviewed.Content, "danger")
	require.NotEqual(t, text, reviewed.Content)
	require.Equal(t, []int{rule.Id}, completed.RuleIDs)
	require.Equal(t, RiskObservationSourceCache, completed.Source)
	require.True(t, completed.CacheHit)
	require.False(t, completed.ProviderCalled)
}

func TestEvaluateRiskObservation_passes_full_current_turn_without_truncation(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	text := strings.Repeat("完整当前轮", 5000)
	var reviewed RiskModerationInput
	executor := riskModerationExecutorFunc(func(_ context.Context, input RiskModerationInput) (RiskModerationOutcome, error) {
		reviewed = input
		return RiskModerationOutcome{Result: RiskReviewResult{Status: RiskReviewSafe}, Source: RiskReviewSourceProvider}, nil
	})

	// When
	_, ok := evaluateRiskObservation(context.Background(), RiskObservationJob{
		RequestID: "full", ChannelID: 24, ChannelName: "cpa-pro", UserID: 42,
		Text: text, ProviderID: provider.Id, ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionBlock,
	}, executor)

	// Then
	require.True(t, ok)
	require.Equal(t, text, reviewed.Content)
	require.Equal(t, model.RiskReviewFull, reviewed.ReviewMode)
	require.Zero(t, reviewed.FullReviewChunkRunes)
}
