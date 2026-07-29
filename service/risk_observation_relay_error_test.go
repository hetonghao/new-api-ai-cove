package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestProcessRiskObservationForRelay_fails_open_for_safe_provider_error_and_open_circuit(t *testing.T) {
	tests := []struct {
		name            string
		outcome         RiskModerationOutcome
		executeErr      error
		wantResult      RiskObservationResult
		wantErrorCode   string
		wantErrorDetail string
	}{
		{name: "safe", outcome: RiskModerationOutcome{Result: RiskReviewResult{Status: RiskReviewSafe}, Source: RiskReviewSourceProvider, ProviderCalled: true}, wantResult: RiskObservationSafe},
		{name: "timeout", outcome: RiskModerationOutcome{Source: RiskReviewSourceProvider, ProviderCalled: true}, executeErr: context.DeadlineExceeded, wantResult: RiskObservationError, wantErrorCode: riskObservationTimeout, wantErrorDetail: "Cloudflare request timed out"},
		{name: "provider error", outcome: RiskModerationOutcome{Source: RiskReviewSourceProvider, ProviderCalled: true}, executeErr: ErrRiskModerationProvider, wantResult: RiskObservationError, wantErrorCode: riskObservationProviderError, wantErrorDetail: "Risk provider request failed"},
		{name: "open circuit", outcome: RiskModerationOutcome{Source: RiskReviewSourceProvider}, executeErr: ErrRiskModerationCircuitOpen, wantResult: RiskObservationError, wantErrorCode: riskObservationCircuitOpen, wantErrorDetail: "Risk moderation circuit is open; provider was not called"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskObservationTest(t)
			provider := createActiveRiskProvider(t, "https://example.com")
			providerID := provider.Id
			channelID := createRiskPolicyChannel(t)
			_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
				ProviderID: &providerID, EnabledChannels: []int{channelID}, ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionBlock,
			})
			require.NoError(t, err)
			var completed RiskObservationEvent
			deps := riskObservationRelayDeps{
				executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
					return test.outcome, test.executeErr
				}),
				enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
					completed = event
					return queuedRiskObservationResult()
				},
			}

			// When
			decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
				RequestID: "fail-open", ChannelID: channelID, ChannelName: "renamed", Text: "current",
			}, deps)

			// Then
			require.False(t, decision.Blocked)
			require.Nil(t, decision.DirectRecord)
			require.Equal(t, test.wantResult, completed.Result)
			require.Equal(t, test.wantErrorCode, completed.ErrorCode)
			require.Equal(t, test.wantErrorDetail, completed.ErrorDetail)
		})
	}
}

func TestRiskObservationErrorMapping_matches_typed_executor_errors(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: ErrRiskModerationCircuitOpen, want: riskObservationCircuitOpen},
		{err: context.DeadlineExceeded, want: riskObservationTimeout},
		{err: errors.Join(errors.New("wrapped"), ErrRiskModerationProvider), want: riskObservationProviderError},
	}
	for _, test := range tests {
		// When
		got, _ := RiskObservationErrorInfo(test.err)

		// Then
		require.Equal(t, test.want, got)
	}
}
