package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskModerationExecutor_Execute_doesNotApplyProviderDeadlineToCache(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	cacheHasDeadline := make(chan bool, 1)
	store.get = func(ctx context.Context, _ string) (RiskReviewResult, bool, error) {
		_, hasDeadline := ctx.Deadline()
		cacheHasDeadline <- hasDeadline
		return RiskReviewResult{}, false, nil
	}
	provider := riskModerationProviderForTest()
	providerDeadline := make(chan bool, 1)
	reviewer := func(ctx context.Context, _ *model.RiskProvider, _ string) (RiskReviewResult, error) {
		_, hasDeadline := ctx.Deadline()
		providerDeadline <- hasDeadline
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(store, "provider-budget-test-secret"), Reviewer: reviewer, Now: time.Now,
	})
	// When
	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: provider, Content: "text", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.NoError(t, err)
	assert.False(t, <-cacheHasDeadline)
	assert.True(t, <-providerDeadline)
	assert.Equal(t, RiskReviewSourceProvider, outcome.Source)
	assert.True(t, outcome.ProviderCalled)
	assert.Equal(t, RiskReviewSafe, outcome.Result.Status)
}

func TestRiskModerationExecutor_Execute_classifiesCallerDeadline(t *testing.T) {
	// Given
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "caller-deadline-test-secret"),
		Reviewer: func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		},
		Now: time.Now,
	})

	// When
	outcome, err := executor.Execute(ctx, RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text", ReviewMode: model.RiskReviewSelective,
	})
	code, detail := RiskObservationErrorInfo(err)

	// Then
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, ErrRiskModerationCallerCanceled)
	assert.False(t, outcome.ProviderCalled)
	assert.Equal(t, riskObservationCallerCanceled, code)
	assert.Equal(t, "Risk moderation request was canceled", detail)
}

func TestRiskModerationExecutor_Execute_continuesFullReviewAfterOneChunkTimesOut(t *testing.T) {
	// Given
	provider := riskModerationProviderForTest()
	provider.TimeoutMs = 1
	reviewCalls := 0
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "deadline-test-secret"),
		Reviewer: func(ctx context.Context, _ *model.RiskProvider, _ string) (RiskReviewResult, error) {
			reviewCalls++
			if reviewCalls == 1 {
				<-ctx.Done()
				return RiskReviewResult{}, ctx.Err()
			}
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		},
	})

	// When
	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: provider, Content: "abcdef", ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2,
	})

	// Then
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Len(t, outcome.Chunks, 3)
	assert.Equal(t, 3, reviewCalls)
	assert.Equal(t, RiskReviewError, outcome.Chunks[0].Status)
	assert.Equal(t, RiskReviewSafe, outcome.Chunks[1].Status)
	assert.Equal(t, RiskReviewSafe, outcome.Chunks[2].Status)
}
