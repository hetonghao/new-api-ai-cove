package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

type riskModerationExecutorFunc func(context.Context, RiskModerationInput) (RiskModerationOutcome, error)

func (execute riskModerationExecutorFunc) Execute(ctx context.Context, input RiskModerationInput) (RiskModerationOutcome, error) {
	return execute(ctx, input)
}

func queuedRiskObservationResult() RiskObservationEnqueueResult {
	return RiskObservationEnqueueResult{Outcome: RiskObservationEnqueueQueued}
}

func TestProcessRiskObservationForRelay_does_not_direct_record_retained_observe_job(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	secondProvider := createActiveRiskProvider(t, "https://second.example.com")
	channelID := createRiskPolicyChannel(t)
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderIDs:     []int{provider.Id, secondProvider.Id},
		EnabledChannels: []int{channelID},
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
		enqueueJob: func(job RiskObservationJob) RiskObservationEnqueueResult {
			queuedJob = job
			return RiskObservationEnqueueResult{Outcome: RiskObservationEnqueueFallbackRetained}
		},
		enqueueEvent: func(RiskObservationEvent) RiskObservationEnqueueResult {
			completedEvents++
			return queuedRiskObservationResult()
		},
	}
	job := RiskObservationJob{RequestID: "observe", ChannelID: channelID, ChannelName: "renamed", Text: "current"}

	// When
	decision := processRiskObservationForRelay(context.Background(), job, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Zero(t, executorCalls)
	require.Equal(t, job.RequestID, queuedJob.RequestID)
	require.Equal(t, []int{provider.Id, secondProvider.Id}, queuedJob.ProviderIDs)
	require.Equal(t, model.RiskReviewFull, queuedJob.ReviewMode)
	require.Equal(t, model.RiskActionObserve, queuedJob.ActionMode)
	require.Zero(t, completedEvents)
}

func TestProcessRiskObservationForRelay_returns_direct_record_for_unretained_observe_job(t *testing.T) {
	// Given
	providerID := 17
	job := RiskObservationJob{RequestID: "overflow", ChannelID: 24, ChannelName: "anything", Text: "current"}
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled:         true,
				ProviderIDs:     []int{providerID},
				EnabledChannels: []int{24},
				ReviewMode:      model.RiskReviewFull,
				ActionMode:      model.RiskActionObserve,
			}, nil
		},
		loadRules: func() ([]*model.RiskRule, error) { return nil, nil },
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			return RiskObservationEnqueueResult{
				Outcome:   RiskObservationEnqueueDirectRecordRequired,
				ErrorCode: RiskObservationErrorQueueFull,
			}
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), job, deps)

	// Then
	require.False(t, decision.Blocked)
	require.NotNil(t, decision.DirectRecord)
	require.NotNil(t, decision.DirectRecord.Job)
	require.Nil(t, decision.DirectRecord.Event)
	require.Equal(t, RiskObservationErrorQueueFull, decision.DirectRecord.ErrorCode)
	require.Equal(t, []int{providerID}, decision.DirectRecord.Job.ProviderIDs)
	require.Zero(t, decision.DirectRecord.Job.ProviderID)
	require.Equal(t, model.RiskActionObserve, decision.DirectRecord.Job.ActionMode)
}

func TestProcessRiskObservationForRelay_blocks_unsafe_result_and_enqueues_completed_event(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	secondProvider := createActiveRiskProvider(t, "https://second.example.com")
	channelID := createRiskPolicyChannel(t)
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderIDs:     []int{provider.Id, secondProvider.Id},
		EnabledChannels: []int{channelID},
		ReviewMode:      model.RiskReviewFull,
		ActionMode:      model.RiskActionBlock,
	})
	require.NoError(t, err)
	executorCalls := 0
	var completed RiskObservationEvent
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(_ context.Context, input RiskModerationInput) (RiskModerationOutcome, error) {
			executorCalls++
			require.Nil(t, input.Provider)
			require.Len(t, input.Providers, 2)
			require.Equal(t, []int{provider.Id, secondProvider.Id}, []int{input.Providers[0].Id, input.Providers[1].Id})
			require.Equal(t, model.RiskReviewFull, input.ReviewMode)
			require.Zero(t, input.FullReviewChunkRunes)
			return RiskModerationOutcome{
				Result: RiskReviewResult{
					Status: RiskReviewUnsafe, Categories: []string{"S1"}, ProviderID: secondProvider.Id,
					ProviderName: secondProvider.Name, ProviderType: secondProvider.ProviderType,
				},
				Chunks: []RiskReviewChunkAudit{{
					Index: 0, Status: RiskReviewUnsafe, Categories: []string{"S1"}, LatencyMS: 41,
					Usage: RiskReviewUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6, Neurons: 9},
				}},
				Source:         RiskReviewSourceProvider,
				ProviderCalled: true,
			}, nil
		}),
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			t.Fatal("block mode must not enqueue a pending review job")
			return RiskObservationEnqueueResult{}
		},
		enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
			completed = event
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "block", ChannelID: channelID, ChannelName: "renamed", Text: "current",
	}, deps)

	// Then
	require.True(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Equal(t, 1, executorCalls)
	require.Equal(t, RiskObservationUnsafe, completed.Result)
	require.Equal(t, secondProvider.Id, completed.ProviderID)
	require.Equal(t, secondProvider.Name, completed.ProviderName)
	require.Equal(t, secondProvider.ProviderType, completed.ProviderType)
	require.Equal(t, []string{"S1"}, completed.Categories)
	require.Equal(t, []RiskReviewChunkAudit{{
		Index: 0, Status: RiskReviewUnsafe, Categories: []string{"S1"}, LatencyMS: 41,
		Usage: RiskReviewUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6, Neurons: 9},
	}}, completed.Chunks)
}

func TestProcessRiskObservationForRelay_allows_configured_non_blocking_category(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	channelID := createRiskPolicyChannel(t)
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderIDs: []int{provider.Id}, EnabledChannels: []int{channelID},
		ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionBlock,
		NonBlockingCategories: []string{"S14"},
	})
	require.NoError(t, err)
	var completed RiskObservationEvent
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			return RiskModerationOutcome{
				Result: RiskReviewResult{Status: RiskReviewUnsafe, Categories: []string{"s14"}},
				Source: RiskReviewSourceCache, CacheHit: true,
			}, nil
		}),
		enqueueJob: func(RiskObservationJob) RiskObservationEnqueueResult {
			t.Fatal("block mode must not enqueue a pending review job")
			return RiskObservationEnqueueResult{}
		},
		enqueueEvent: func(event RiskObservationEvent) RiskObservationEnqueueResult {
			completed = event
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "non-blocking", ChannelID: channelID, UserID: 42, Text: "current",
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Equal(t, RiskObservationUnsafe, completed.Result)
	require.False(t, completed.Blocked)
	require.True(t, completed.NonBlockingMatched)
	require.True(t, completed.CacheHit)
}

func TestProcessRiskObservationForRelay_skips_unselected_channel_id(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	executorCalls := 0
	queueCalls := 0
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			providerID := 17
			return model.RiskPolicyState{Enabled: true, ProviderIDs: []int{providerID}, EnabledChannels: []int{24}}, nil
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
		RequestID: "other", ChannelID: 25, ChannelName: "CPA Pro", Text: "current",
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Zero(t, executorCalls)
	require.Zero(t, queueCalls)
}

func TestProcessRiskObservationForRelay_skips_excluded_user(t *testing.T) {
	// Given
	providerID := 17
	executorCalls := 0
	queueCalls := 0
	deps := riskObservationRelayDeps{
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.RiskPolicyState{
				Enabled:         true,
				ProviderIDs:     []int{providerID},
				EnabledChannels: []int{24},
				ExcludedUserIDs: []int{42},
				ReviewMode:      model.RiskReviewFull,
				ActionMode:      model.RiskActionBlock,
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
		enqueueEvent: func(RiskObservationEvent) RiskObservationEnqueueResult {
			queueCalls++
			return queuedRiskObservationResult()
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "excluded", ChannelID: 24, UserID: 42, Text: "current",
	}, deps)

	// Then
	require.False(t, decision.Blocked)
	require.Nil(t, decision.DirectRecord)
	require.Zero(t, executorCalls)
	require.Zero(t, queueCalls)
}

func TestProcessRiskObservationForRelay_keeps_unsafe_block_when_completed_event_queue_is_full(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	provider := createActiveRiskProvider(t, "https://example.com")
	providerID := provider.Id
	channelID := createRiskPolicyChannel(t)
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderIDs:     []int{providerID},
		EnabledChannels: []int{channelID},
		ReviewMode:      model.RiskReviewFull,
		ActionMode:      model.RiskActionBlock,
	})
	require.NoError(t, err)
	deps := riskObservationRelayDeps{
		executor: riskModerationExecutorFunc(func(context.Context, RiskModerationInput) (RiskModerationOutcome, error) {
			return RiskModerationOutcome{Result: RiskReviewResult{Status: RiskReviewUnsafe}}, nil
		}),
		enqueueEvent: func(RiskObservationEvent) RiskObservationEnqueueResult {
			return RiskObservationEnqueueResult{Outcome: RiskObservationEnqueueDirectRecordRequired}
		},
	}

	// When
	decision := processRiskObservationForRelay(context.Background(), RiskObservationJob{
		RequestID: "overflow", ChannelID: channelID, ChannelName: "renamed", Text: "current",
	}, deps)

	// Then
	require.True(t, decision.Blocked)
	require.NotNil(t, decision.DirectRecord)
	require.Nil(t, decision.DirectRecord.Job)
	require.NotNil(t, decision.DirectRecord.Event)
	require.True(t, decision.DirectRecord.Event.Blocked)
}
