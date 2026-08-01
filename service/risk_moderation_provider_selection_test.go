package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskModerationExecutor_Execute_keepsProviderCircuitsIndependent(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()[:2]
	providers[0].FailureThreshold = 1
	providers[1].FailureThreshold = 1
	selected := make([]int, 0, 2)
	providerErr := errors.New("provider one failed")
	reviewer := func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
		selected = append(selected, provider.Id)
		if provider.Id == providers[0].Id {
			return RiskReviewResult{}, providerErr
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-circuit"), Reviewer: reviewer, Now: time.Now,
	})

	// When
	_, firstErr := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "first", ReviewMode: model.RiskReviewSelective,
	})
	second, secondErr := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "second", ReviewMode: model.RiskReviewSelective,
	})
	third, thirdErr := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "third", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.ErrorIs(t, firstErr, providerErr)
	require.NoError(t, secondErr)
	require.NoError(t, thirdErr)
	assert.Equal(t, []int{providers[0].Id, providers[1].Id, providers[1].Id}, selected)
	assert.Equal(t, providers[1].Id, second.Result.ProviderID)
	assert.Equal(t, providers[1].Id, third.Result.ProviderID)
}

func TestRiskModerationExecutor_Execute_usesSelectedProviderTimeout(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()[:2]
	providers[0].TimeoutMs = 80
	providers[1].TimeoutMs = 800
	var remaining time.Duration
	reviewer := func(ctx context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		remaining = time.Until(deadline)
		assert.Equal(t, providers[0].Id, provider.Id)
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-timeout"), Reviewer: reviewer, Now: time.Now,
	})

	// When
	_, err := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "text", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.NoError(t, err)
	assert.Positive(t, remaining)
	assert.LessOrEqual(t, remaining, 80*time.Millisecond)
}

func TestRiskModerationExecutor_Execute_circuitOpenDoesNotAdvanceProviderPool(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()[:2]
	fixedNow := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	blockedExecutor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-circuit-open"),
		Reviewer: func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		},
		Now: func() time.Time { return fixedNow },
	})
	providerKey := riskModerationProviderCircuitKey(providers[0])
	blockedExecutor.circuit.Failure(context.Background(), riskModerationCircuitPermit{
		key: providerKey, threshold: 1, cooldown: time.Hour,
	})
	selected := 0
	freshExecutor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-circuit-fresh"),
		Reviewer: func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
			selected = provider.Id
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		},
		Now: time.Now,
	})

	// When
	_, blockedErr := blockedExecutor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "blocked", ReviewMode: model.RiskReviewSelective,
	})
	_, freshErr := freshExecutor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "fresh", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.NoError(t, blockedErr)
	require.NoError(t, freshErr)
	assert.Equal(t, providers[1].Id, selected)
}

func TestRiskModerationExecutor_Execute_preCallCanceledContextDoesNotAdvanceProviderPool(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()[:2]
	ctx, cancel := context.WithCancel(context.Background())
	baseNow := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	nowCalls := 0
	preCallExecutor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-pre-call-cancel"),
		Reviewer: func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		},
		Now: func() time.Time {
			nowCalls++
			if nowCalls == 2 {
				cancel()
				return baseNow.Add(2 * time.Hour)
			}
			return baseNow
		},
	})
	providerKey := riskModerationProviderCircuitKey(providers[0])
	preCallExecutor.circuit.Failure(context.Background(), riskModerationCircuitPermit{
		key: providerKey, threshold: 1, cooldown: time.Hour,
	})
	selected := 0
	freshExecutor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-pre-call-fresh"),
		Reviewer: func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
			selected = provider.Id
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		},
		Now: time.Now,
	})

	// When
	_, canceledErr := preCallExecutor.Execute(ctx, RiskModerationInput{
		Providers: providers, Content: "canceled", ReviewMode: model.RiskReviewSelective,
	})
	_, freshErr := freshExecutor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "fresh", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.ErrorIs(t, canceledErr, context.Canceled)
	require.NoError(t, freshErr)
	assert.Equal(t, providers[0].Id, selected)
}

func TestRiskModerationExecutor_Execute_degradesOnlyAfterHighestPriorityIsUnavailable(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()
	providers[0].Priority = 20
	providers[1].Priority = 20
	providers[2].Priority = 10
	for _, provider := range providers {
		provider.FailureThreshold = 1
	}
	selected := make([]int, 0, 3)
	reviewer := func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
		selected = append(selected, provider.Id)
		if provider.Id < 3 {
			return RiskReviewResult{}, errors.New("provider unavailable")
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "priority-degrade"), Reviewer: reviewer, Now: time.Now,
	})

	// When
	_, firstErr := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "first", ReviewMode: model.RiskReviewSelective,
	})
	_, secondErr := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "second", ReviewMode: model.RiskReviewSelective,
	})
	third, thirdErr := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "third", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.Error(t, firstErr)
	require.Error(t, secondErr)
	require.NoError(t, thirdErr)
	assert.Equal(t, []int{1, 2, 3}, selected)
	assert.Equal(t, 3, third.Result.ProviderID)
}

func TestRiskModerationExecutor_Execute_reusesCacheAcrossProvidersWithoutProviderMetadata(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()[:2]
	store := newFakeRiskReviewCacheStore()
	calls := 0
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(store, "provider-independent-cache"),
		Reviewer: func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
			calls++
			return RiskReviewResult{
				Status: RiskReviewUnsafe, Categories: []string{"S1"},
				ProviderID: provider.Id, ProviderName: provider.Name, ProviderType: provider.ProviderType,
				Usage: RiskReviewUsage{PromptTokens: 3, TotalTokens: 3, Neurons: 7},
			}, nil
		},
		Now: time.Now,
	})

	// When
	first, firstErr := executor.Execute(context.Background(), RiskModerationInput{
		Provider: providers[0], Content: "same content", ReviewMode: model.RiskReviewSelective,
	})
	second, secondErr := executor.Execute(context.Background(), RiskModerationInput{
		Provider: providers[1], Content: "same content", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, 1, calls)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	assert.Equal(t, RiskReviewSourceCache, second.Source)
	assert.Equal(t, providers[0].Id, first.Result.ProviderID)
	assert.Zero(t, second.Result.ProviderID)
	assert.Zero(t, second.Result.Usage.Neurons)
	assert.False(t, second.ProviderCalled)
}
