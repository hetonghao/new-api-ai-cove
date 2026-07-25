package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestEvaluateRiskObservation_maps_source_accounting_and_blocking(t *testing.T) {
	tests := []struct {
		name               string
		actionMode         model.RiskActionMode
		outcome            RiskModerationOutcome
		executeErr         error
		wantSource         RiskObservationSource
		wantCacheHit       bool
		wantProviderCalled bool
		wantBlocked        bool
		wantErrorCode      string
	}{
		{
			name: "provider unsafe block", actionMode: model.RiskActionBlock,
			outcome: RiskModerationOutcome{
				Result: RiskReviewResult{Status: RiskReviewUnsafe}, Source: RiskReviewSourceProvider, ProviderCalled: true,
			},
			wantSource: RiskObservationSourceProvider, wantProviderCalled: true, wantBlocked: true,
		},
		{
			name: "cache unsafe block", actionMode: model.RiskActionBlock,
			outcome: RiskModerationOutcome{
				Result: RiskReviewResult{Status: RiskReviewUnsafe}, Source: RiskReviewSourceCache, CacheHit: true,
			},
			wantSource: RiskObservationSourceCache, wantCacheHit: true, wantBlocked: true,
		},
		{
			name: "inflight unsafe observe", actionMode: model.RiskActionObserve,
			outcome: RiskModerationOutcome{
				Result: RiskReviewResult{Status: RiskReviewUnsafe}, Source: RiskReviewSourceInflight,
			},
			wantSource: RiskObservationSourceInflight,
		},
		{
			name: "circuit open fail open", actionMode: model.RiskActionBlock,
			outcome: RiskModerationOutcome{Source: RiskReviewSourceProvider}, executeErr: ErrRiskModerationCircuitOpen,
			wantSource: RiskObservationSourceProvider, wantErrorCode: riskObservationCircuitOpen,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskObservationTest(t)
			provider := createActiveRiskProvider(t, "https://example.com")
			executor := riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
				return test.outcome, test.executeErr
			})

			// When
			event, ok := evaluateRiskObservation(context.Background(), RiskObservationJob{
				RequestID: "mapping", ChannelID: 24, ChannelName: "cpa-pro", UserID: 42,
				Text: "current", ProviderID: provider.Id, ReviewMode: model.RiskReviewFull, ActionMode: test.actionMode,
			}, executor)

			// Then
			require.True(t, ok)
			require.Equal(t, test.wantSource, event.Source)
			require.Equal(t, test.wantCacheHit, event.CacheHit)
			require.Equal(t, test.wantProviderCalled, event.ProviderCalled)
			require.Equal(t, test.wantBlocked, event.Blocked)
			require.Equal(t, test.wantErrorCode, event.ErrorCode)
		})
	}
}

func TestEvaluateRiskObservation_records_provider_config_error_as_local_without_provider(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	executorCalls := 0
	executor := riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
		executorCalls++
		return RiskModerationOutcome{}, nil
	})

	// When
	event, ok := evaluateRiskObservation(context.Background(), RiskObservationJob{
		RequestID: "missing-provider", ChannelID: 24, UserID: 42, Text: "current",
		ProviderID: 999, ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionBlock,
	}, executor)

	// Then
	require.True(t, ok)
	require.Zero(t, executorCalls)
	require.Equal(t, RiskObservationError, event.Result)
	require.Equal(t, riskObservationProviderConfigError, event.ErrorCode)
	require.Equal(t, RiskObservationSourceLocal, event.Source)
	require.Zero(t, event.ProviderID)
	require.Empty(t, event.ProviderName)
	require.False(t, event.CacheHit)
	require.False(t, event.ProviderCalled)
	require.False(t, event.Blocked)
}
