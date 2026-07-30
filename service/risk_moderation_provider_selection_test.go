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
	require.ErrorIs(t, thirdErr, ErrRiskModerationCircuitOpen)
	assert.Equal(t, []int{providers[0].Id, providers[1].Id}, selected)
	assert.Equal(t, providers[1].Id, second.Result.ProviderID)
	assert.Equal(t, providers[0].Id, third.Result.ProviderID)
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
