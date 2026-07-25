package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestProcessRiskObservationForRelay_fails_open_when_enabled_policy_has_no_provider(t *testing.T) {
	// Given
	executorCalls := 0
	queuedJobs := 0
	var completed RiskObservationEvent
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled:         true,
				ProviderID:      nil,
				EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
				ReviewMode:      model.RiskReviewFull,
				ActionMode:      model.RiskActionBlock,
			}, nil
		},
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			return RiskModerationOutcome{}, nil
		}),
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			queuedJobs++
			return queuedRiskObservationResult()
		},
		enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
			completed = event
			return queuedRiskObservationResult()
		},
	}
	var decision RiskObservationRelayDecision

	// When
	require.NotPanics(t, func() {
		decision = processRiskObservationForRelay(context.Background(), RiskObservationJob{
			RequestID: "missing-provider", ChannelName: "cpa-pro", Text: "current",
		}, deps)
	})

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Zero(t, executorCalls)
	require.Zero(t, queuedJobs)
	require.Equal(t, RiskObservationError, completed.Result)
	require.Equal(t, riskObservationProviderConfigError, completed.ErrorCode)
	require.Equal(t, RiskObservationSourceLocal, completed.Source)
	require.Zero(t, completed.ProviderID)
	require.False(t, completed.ProviderCalled)
	require.False(t, completed.Blocked)
}

func TestProcessRiskObservationForRelay_records_empty_text_as_local_not_reviewed(t *testing.T) {
	// Given
	providerID := 17
	executorCalls := 0
	var completed RiskObservationEvent
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled:         true,
				ProviderID:      &providerID,
				EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
				ReviewMode:      model.RiskReviewFull,
				ActionMode:      model.RiskActionBlock,
			}, nil
		},
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			return RiskModerationOutcome{}, nil
		}),
		enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
			completed = event
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "attachment-only", ChannelName: "cpa-pro",
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Zero(t, executorCalls)
	require.Equal(t, RiskObservationNotReviewed, completed.Result)
	require.Equal(t, RiskObservationSourceLocal, completed.Source)
	require.Empty(t, completed.Preview)
	require.Empty(t, completed.ContentHash)
	require.False(t, completed.ProviderCalled)
	require.False(t, completed.Blocked)
}
