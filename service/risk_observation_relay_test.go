package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

type riskModerationExecutorFunc func(context.Context, RiskModerationInput) (RiskModerationOutcome, error)

func (execute riskModerationExecutorFunc) Execute(ctx context.Context, input RiskModerationInput) (RiskModerationOutcome, error) {
	return execute(ctx, input)
}

func TestProcessRiskObservationForRelay_enqueues_observe_job_without_executor_call(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	providerID := provider.Id
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderID:      &providerID,
		EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
		ReviewMode:      model.RiskReviewFull,
		ActionMode:      model.RiskActionObserve,
	})
	require.NoError(t, err)
	executorCalls := 0
	var queuedJob RiskObservationJob
	completedEvents := 0
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			return RiskModerationOutcome{}, nil
		}),
		enqueueJob: func(job RiskObservationJob) bool {
			queuedJob = job
			return true
		},
		enqueueEvent: func(RiskObservationEvent) bool {
			completedEvents++
			return true
		},
	}
	job := RiskObservationJob{RequestID: "observe", ChannelName: " CPA-Pro ", Text: "current"}

	// When
	blocked := processRiskObservationForRelay(context.Background(), job, deps)

	// Then
	require.False(t, blocked)
	require.Zero(t, executorCalls)
	require.Equal(t, job.RequestID, queuedJob.RequestID)
	require.Equal(t, providerID, queuedJob.ProviderID)
	require.Equal(t, model.RiskReviewFull, queuedJob.ReviewMode)
	require.Equal(t, model.RiskActionObserve, queuedJob.ActionMode)
	require.Zero(t, completedEvents)
}

func TestProcessRiskObservationForRelay_blocks_unsafe_result_and_enqueues_completed_event(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	providerID := provider.Id
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderID:      &providerID,
		EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
		ReviewMode:      model.RiskReviewFull,
		ActionMode:      model.RiskActionBlock,
	})
	require.NoError(t, err)
	executorCalls := 0
	var completed RiskObservationEvent
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(_ context.Context, input RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			require.Equal(t, providerID, input.Provider.Id)
			require.Equal(t, model.RiskReviewFull, input.ReviewMode)
			require.Zero(t, input.FullReviewChunkRunes)
			return RiskModerationOutcome{
				Result:         RiskReviewResult{Status: RiskReviewUnsafe, Categories: []string{"S1"}},
				Source:         RiskReviewSourceProvider,
				ProviderCalled: true,
			}, nil
		}),
		enqueueJob: func(RiskObservationJob) bool {
			t.Fatal("block mode must not enqueue a pending review job")
			return false
		},
		enqueueEvent: func(event RiskObservationEvent) bool {
			completed = event
			return true
		},
	}

	// When
	blocked := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "block", ChannelName: "cpa-pro", Text: "current",
	}, deps)

	// Then
	require.True(t, blocked)
	require.Equal(t, 1, executorCalls)
	require.Equal(t, RiskObservationUnsafe, completed.Result)
	require.Equal(t, providerID, completed.ProviderID)
	require.Equal(t, []string{"S1"}, completed.Categories)
}

func TestProcessRiskObservationForRelay_skips_non_cpa_pro_channel(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	executorCalls := 0
	queueCalls := 0
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			return RiskModerationOutcome{}, nil
		}),
		enqueueJob: func(RiskObservationJob) bool {
			queueCalls++
			return true
		},
		enqueueEvent: func(RiskObservationEvent) bool {
			queueCalls++
			return true
		},
	}

	// When
	blocked := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "core", ChannelName: "CPA-core", Text: "current",
	}, deps)

	// Then
	require.False(t, blocked)
	require.Zero(t, executorCalls)
	require.Zero(t, queueCalls)
}

func TestProcessRiskObservationForRelay_fails_open_for_safe_provider_error_and_open_circuit(t *testing.T) {
	tests := []struct {
		name          string
		outcome       RiskModerationOutcome
		executeErr    error
		wantResult    RiskObservationResult
		wantErrorCode string
	}{
		{
			name:       "safe",
			outcome:    RiskModerationOutcome{Result: RiskReviewResult{Status: RiskReviewSafe}, Source: RiskReviewSourceProvider, ProviderCalled: true},
			wantResult: RiskObservationSafe,
		},
		{
			name:          "provider error",
			outcome:       RiskModerationOutcome{Source: RiskReviewSourceProvider, ProviderCalled: true},
			executeErr:    ErrRiskModerationProvider,
			wantResult:    RiskObservationError,
			wantErrorCode: riskObservationProviderError,
		},
		{
			name:          "open circuit",
			outcome:       RiskModerationOutcome{Source: RiskReviewSourceProvider},
			executeErr:    ErrRiskModerationCircuitOpen,
			wantResult:    RiskObservationError,
			wantErrorCode: riskObservationCircuitOpen,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskObservationTest(t)
			provider := createActiveRiskProvider(t, "https://example.com")
			providerID := provider.Id
			_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
				ProviderID:      &providerID,
				EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
				ReviewMode:      model.RiskReviewFull,
				ActionMode:      model.RiskActionBlock,
			})
			require.NoError(t, err)
			var completed RiskObservationEvent
			deps := riskObservationRelayDeps{
				executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
					return test.outcome, test.executeErr
				}),
				enqueueEvent: func(event RiskObservationEvent) bool {
					completed = event
					return true
				},
			}

			// When
			blocked := processRiskObservationForRelay(context.Background(), RiskObservationJob{
				RequestID: "fail-open", ChannelName: "CPA-pro", Text: "current",
			}, deps)

			// Then
			require.False(t, blocked)
			require.Equal(t, test.wantResult, completed.Result)
			require.Equal(t, test.wantErrorCode, completed.ErrorCode)
		})
	}
}

func TestProcessRiskObservationForRelay_keeps_unsafe_block_when_completed_event_queue_is_full(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	providerID := provider.Id
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderID:      &providerID,
		EnabledChannels: []model.RiskChannel{model.RiskChannelCPAPro},
		ReviewMode:      model.RiskReviewFull,
		ActionMode:      model.RiskActionBlock,
	})
	require.NoError(t, err)
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			return RiskModerationOutcome{Result: RiskReviewResult{Status: RiskReviewUnsafe}}, nil
		}),
		enqueueEvent: func(RiskObservationEvent) bool { return false },
	}

	// When
	blocked := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "overflow", ChannelName: "CPA-pro", Text: "current",
	}, deps)

	// Then
	require.True(t, blocked)
}

func TestRiskObservationErrorMapping_matches_typed_executor_errors(t *testing.T) {
	// Given
	tests := []struct {
		err  error
		want string
	}{
		{err: ErrRiskModerationCircuitOpen, want: riskObservationCircuitOpen},
		{err: errors.Join(errors.New("wrapped"), ErrRiskModerationProvider), want: riskObservationProviderError},
	}

	for _, test := range tests {
		// When
		got := riskObservationErrorCode(test.err)

		// Then
		require.Equal(t, test.want, got)
	}
}
