package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskModerationExecutor_Execute_usesOneTotalBudgetAcrossCacheAndProvider(t *testing.T) {
	// Given
	const totalBudget = 100 * time.Millisecond
	store := newFakeRiskReviewCacheStore()
	var cacheDeadline time.Time
	store.get = func(ctx context.Context, _ string) (RiskReviewResult, bool, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		cacheDeadline = deadline
		timer := time.NewTimer(70 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return RiskReviewResult{}, false, ctx.Err()
		case <-timer.C:
			return RiskReviewResult{}, false, nil
		}
	}
	provider := riskModerationProviderForTest()
	provider.TimeoutMs = int(totalBudget / time.Millisecond)
	providerDeadline := make(chan time.Time, 1)
	reviewer := func(ctx context.Context, _ *model.RiskProvider, _ string) (RiskReviewResult, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		providerDeadline <- deadline
		<-ctx.Done()
		return RiskReviewResult{}, ctx.Err()
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(store, "total-budget-test-secret"), Reviewer: reviewer, Now: time.Now,
	})
	started := time.Now()

	// When
	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: provider, Content: "text", ReviewMode: model.RiskReviewSelective,
	})
	elapsed := time.Since(started)

	// Then
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, cacheDeadline, <-providerDeadline)
	assert.Equal(t, RiskReviewSourceProvider, outcome.Source)
	assert.True(t, outcome.ProviderCalled)
	assert.Less(t, elapsed, totalBudget+50*time.Millisecond)
}
