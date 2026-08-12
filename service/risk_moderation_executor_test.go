// allow: SIZE_OK -- executor scenarios share one cache/provider fixture boundary.
package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskModerationExecutor_Execute_doesNotRetryOrCacheSelectiveProviderErrors(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	cache := newRiskReviewCacheService(store, "selective-error-test-secret")
	providerErr := errors.New("provider unavailable")
	partial := RiskReviewResult{Status: RiskReviewError, Categories: []string{"partial"}, Usage: RiskReviewUsage{PromptTokens: 2, TotalTokens: 2}}
	calls := 0
	reviewer := func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
		calls++
		return partial, providerErr
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	provider := riskModerationProviderForTest()
	input := RiskModerationInput{Provider: provider, Content: "text", ReviewMode: model.RiskReviewSelective}
	expected := partial
	expected.ProviderID = provider.Id
	expected.ProviderName = provider.Name
	expected.ProviderType = provider.ProviderType

	// When
	first, firstErr := executor.Execute(context.Background(), input)
	second, secondErr := executor.Execute(context.Background(), input)

	// Then
	require.ErrorIs(t, firstErr, providerErr)
	require.ErrorIs(t, firstErr, ErrRiskModerationProvider)
	assert.Equal(t, expected, first.Result)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	assert.True(t, first.ProviderCalled)
	require.ErrorIs(t, secondErr, providerErr)
	assert.Equal(t, expected, second.Result)
	assert.Equal(t, 2, calls)
	_, setCalls, _, _ := store.snapshot()
	assert.Zero(t, setCalls)
}

func TestRiskModerationExecutor_Execute_doesNotCacheFullReviewErrors(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	cache := newRiskReviewCacheService(store, "full-error-test-secret")
	providerErr := errors.New("provider unavailable")
	calls := 0
	reviewer := func(_ context.Context, _ *model.RiskProvider, chunk string) (RiskReviewResult, error) {
		calls++
		if chunk == "cd" {
			return RiskReviewResult{Categories: []string{"partial"}, Usage: RiskReviewUsage{PromptTokens: 2}}, providerErr
		}
		return RiskReviewResult{Status: RiskReviewSafe, Usage: RiskReviewUsage{PromptTokens: 2, TotalTokens: 2}}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	input := RiskModerationInput{Provider: riskModerationProviderForTest(), Content: "abcdef", ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2}

	// When
	first, firstErr := executor.Execute(context.Background(), input)
	second, secondErr := executor.Execute(context.Background(), input)

	// Then
	require.ErrorIs(t, firstErr, providerErr)
	assert.Equal(t, RiskReviewError, first.Result.Status)
	assert.Equal(t, []string{"partial"}, first.Result.Categories)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	require.ErrorIs(t, secondErr, providerErr)
	assert.Equal(t, RiskReviewError, second.Result.Status)
	assert.Equal(t, 6, calls)
	_, setCalls, _, _ := store.snapshot()
	assert.Zero(t, setCalls)
}

func TestRiskModerationExecutor_Execute_keepsProviderCallAfterLaterBudgetFailure(t *testing.T) {
	cache := newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "full-budget-call-test-secret")
	reviewer := func(_ context.Context, _ *model.RiskProvider, chunk string) (RiskReviewResult, error) {
		if chunk == "cd" {
			return RiskReviewResult{}, ErrRiskProviderBudgetUnavailable
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})

	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "abcdef", ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2,
	})

	require.ErrorIs(t, err, ErrRiskProviderBudgetUnavailable)
	assert.Equal(t, RiskReviewSourceProvider, outcome.Source)
	assert.True(t, outcome.ProviderCalled)
}

func TestRiskModerationExecutor_Execute_keepsLocalSourceWhenBudgetPreventsProviderCall(t *testing.T) {
	provider := riskModerationProviderForTest()
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "budget-source-test-secret"),
		Reviewer: func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
			return RiskReviewResult{}, ErrRiskProviderBudgetUnavailable
		},
		Now: time.Now,
	})

	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: provider, Content: "budget exhausted", ReviewMode: model.RiskReviewSelective,
	})

	require.ErrorIs(t, err, ErrRiskModerationNoAvailableProvider)
	assert.Empty(t, outcome.Source)
	assert.Zero(t, outcome.Result.ProviderID)
	assert.False(t, outcome.ProviderCalled)
}

func TestRiskModerationExecutor_Execute_keepsLocalSourceWhenCircuitPreventsProviderCall(t *testing.T) {
	provider := riskModerationProviderForTest()
	provider.FailureThreshold = 1
	circuit := newRiskModerationCircuit(time.Now)
	permit, err := circuit.Allow(context.Background(), riskModerationProviderCircuitKey(provider), provider.FailureThreshold, time.Minute)
	require.NoError(t, err)
	circuit.Failure(context.Background(), permit)
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "circuit-source-test-secret"),
		Reviewer: func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
			require.Fail(t, "circuit-open provider must not be called")
			return RiskReviewResult{}, nil
		},
		Now: time.Now,
	})
	executor.circuit = circuit

	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: provider, Content: "circuit open", ReviewMode: model.RiskReviewSelective,
	})

	require.ErrorIs(t, err, ErrRiskModerationCircuitOpen)
	assert.Empty(t, outcome.Source)
	assert.Zero(t, outcome.Result.ProviderID)
	assert.False(t, outcome.ProviderCalled)
}

func TestRiskModerationExecutor_Execute_doesNotSwitchProviderAfterPartialFullReview(t *testing.T) {
	providers := riskModerationProviderPoolForTest()[:2]
	selected := make([]int, 0, 2)
	reviewer := func(_ context.Context, provider *model.RiskProvider, chunk string) (RiskReviewResult, error) {
		selected = append(selected, provider.Id)
		if provider.Id == providers[0].Id && chunk == "cd" {
			return RiskReviewResult{}, ErrRiskProviderBudgetUnavailable
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache:    newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "partial-full-review-test-secret"),
		Reviewer: reviewer, Now: time.Now,
	})

	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "abcdef", ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2,
	})

	require.ErrorIs(t, err, ErrRiskProviderBudgetUnavailable)
	assert.Equal(t, RiskReviewSourceProvider, outcome.Source)
	assert.Equal(t, providers[0].Id, outcome.Result.ProviderID)
	assert.True(t, outcome.ProviderCalled)
	assert.NotContains(t, selected, providers[1].Id)
}

func TestRiskModerationExecutor_Execute_fallsThroughWhenCacheFails(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	store.getErr = errors.New("redis get failed")
	store.setErr = errors.New("redis set failed")
	cache := newRiskReviewCacheService(store, "cache-failure-test-secret")
	calls := 0
	reviewer := func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
		calls++
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	input := RiskModerationInput{Provider: riskModerationProviderForTest(), Content: "text", ReviewMode: model.RiskReviewSelective}

	// When
	first, firstErr := executor.Execute(context.Background(), input)
	second, secondErr := executor.Execute(context.Background(), input)

	// Then
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	assert.Equal(t, RiskReviewSourceProvider, second.Source)
	assert.True(t, first.ProviderCalled)
	assert.True(t, second.ProviderCalled)
	assert.Equal(t, 2, calls)
}

func TestRiskModerationExecutor_Execute_cachesFinalFullReviewAggregate(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	cache := newRiskReviewCacheService(store, "executor-test-secret")
	provider := riskModerationProviderForTest()
	calls := 0
	reviewer := func(_ context.Context, _ *model.RiskProvider, chunk string) (RiskReviewResult, error) {
		calls++
		if chunk == "cd" {
			return RiskReviewResult{Status: RiskReviewUnsafe, Categories: []string{"S1"}, Usage: RiskReviewUsage{PromptTokens: 2, TotalTokens: 2}}, nil
		}
		return RiskReviewResult{Status: RiskReviewSafe, Usage: RiskReviewUsage{PromptTokens: 2, TotalTokens: 2}}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	input := RiskModerationInput{Provider: provider, Content: "abcdef", ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2}

	// When
	first, err := executor.Execute(context.Background(), input)
	require.NoError(t, err)
	second, err := executor.Execute(context.Background(), input)
	require.NoError(t, err)

	// Then
	assert.Equal(t, RiskReviewUnsafe, first.Result.Status)
	assert.Equal(t, []string{"S1"}, first.Result.Categories)
	assert.Equal(t, RiskReviewUsage{PromptTokens: 6, TotalTokens: 6}, first.Result.Usage)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	assert.False(t, first.CacheHit)
	assert.True(t, first.ProviderCalled)
	require.Len(t, first.Chunks, 3)
	assert.Equal(t, 0, first.Chunks[0].Index)
	assert.Equal(t, RiskReviewSafe, first.Chunks[0].Status)
	assert.Equal(t, 1, first.Chunks[1].Index)
	assert.Equal(t, RiskReviewUnsafe, first.Chunks[1].Status)
	assert.Equal(t, []string{"S1"}, first.Chunks[1].Categories)
	assert.Equal(t, RiskReviewUsage{PromptTokens: 2, TotalTokens: 2}, first.Chunks[1].Usage)
	assert.Equal(t, first.Result.Status, second.Result.Status)
	assert.Equal(t, first.Result.Categories, second.Result.Categories)
	assert.Zero(t, second.Result.ProviderID)
	assert.Zero(t, second.Result.Usage)
	assert.Equal(t, RiskReviewSourceCache, second.Source)
	assert.True(t, second.CacheHit)
	assert.False(t, second.ProviderCalled)
	assert.Empty(t, second.Chunks)
	assert.Equal(t, 3, calls)
}

func TestRiskModerationExecutor_Execute_exposesOnlyUnsafeChunkRedactedSummary(t *testing.T) {
	// Given
	reviewer := func(_ context.Context, _ *model.RiskProvider, chunk string) (RiskReviewResult, error) {
		if chunk == "api_key:secret" {
			return RiskReviewResult{Status: RiskReviewUnsafe, Categories: []string{"S3"}}, nil
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache:    newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "chunk-privacy-test-secret"),
		Reviewer: reviewer,
		Now:      time.Now,
	})

	// When
	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "safe..........api_key:secret",
		ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 14,
	})

	// Then
	require.NoError(t, err)
	require.Len(t, outcome.Chunks, 2)
	assert.Empty(t, outcome.Chunks[0].Summary)
	assert.NotEmpty(t, outcome.Chunks[1].Summary)
	assert.NotContains(t, outcome.Chunks[1].Summary, "secret")
	encoded, err := common.Marshal(outcome.Chunks)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "api_key:secret")
}

func TestRiskModerationExecutor_Execute_usesCloudflareFullReviewChunkDefault(t *testing.T) {
	// Given
	calls := 0
	reviewer := func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
		calls++
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache:    newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "chunk-default-test-secret"),
		Reviewer: reviewer, Now: time.Now,
	})

	// When
	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text", ReviewMode: model.RiskReviewFull,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, RiskReviewSafe, outcome.Result.Status)
	assert.Equal(t, 1, calls)
}
