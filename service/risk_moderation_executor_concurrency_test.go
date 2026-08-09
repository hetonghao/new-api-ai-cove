package service

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskModerationExecutor_Execute_coalescesSameKeyFullReview(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	followerCacheChecked := make(chan struct{})
	store.onGet = func(call int) {
		if call == 2 {
			close(followerCacheChecked)
		}
	}
	cache := newRiskReviewCacheService(store, "executor-concurrency-test-secret")
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	var calls atomic.Int32
	reviewer := func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
		if calls.Add(1) == 1 {
			close(providerStarted)
			<-releaseProvider
		}
		return RiskReviewResult{Status: RiskReviewSafe, Usage: RiskReviewUsage{PromptTokens: 1, TotalTokens: 1}}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	input := RiskModerationInput{Provider: riskModerationProviderForTest(), Content: "abcdef", ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2}
	type response struct {
		outcome RiskModerationOutcome
		err     error
	}
	leaderResult := make(chan response, 1)
	followerResult := make(chan response, 1)

	// When
	go func() {
		outcome, err := executor.Execute(context.Background(), input)
		leaderResult <- response{outcome: outcome, err: err}
	}()
	<-providerStarted
	go func() {
		outcome, err := executor.Execute(context.Background(), input)
		followerResult <- response{outcome: outcome, err: err}
	}()
	<-followerCacheChecked
	for range 8 {
		runtime.Gosched()
	}
	close(releaseProvider)
	leader := <-leaderResult
	follower := <-followerResult

	// Then
	require.NoError(t, leader.err)
	require.NoError(t, follower.err)
	assert.Equal(t, RiskReviewSourceProvider, leader.outcome.Source)
	assert.True(t, leader.outcome.ProviderCalled)
	assert.Equal(t, RiskReviewSourceInflight, follower.outcome.Source)
	assert.False(t, follower.outcome.ProviderCalled)
	assert.Equal(t, leader.outcome.Result.Status, follower.outcome.Result.Status)
	assert.Equal(t, leader.outcome.Result.Categories, follower.outcome.Result.Categories)
	assert.Zero(t, follower.outcome.Result.ProviderID)
	assert.Zero(t, follower.outcome.Result.Usage)
	assert.EqualValues(t, 3, calls.Load())
}

func TestRiskModerationExecutor_Execute_inflightFollowerUsesCallerDeadline(t *testing.T) {
	// Given
	cache := newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "follower-budget-test-secret")
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	var calls atomic.Int32
	reviewer := func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
		calls.Add(1)
		close(providerStarted)
		<-releaseProvider
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	leaderProvider := riskModerationProviderForTest()
	leaderProvider.TimeoutMs = 500
	followerProvider := riskModerationProviderForTest()
	followerProvider.TimeoutMs = 500
	followerCtx, cancelFollower := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancelFollower()
	leaderResult := make(chan error, 1)
	go func() {
		_, err := executor.Execute(context.Background(), RiskModerationInput{
			Provider: leaderProvider, Content: "same", ReviewMode: model.RiskReviewSelective,
		})
		leaderResult <- err
	}()
	<-providerStarted
	type response struct {
		outcome RiskModerationOutcome
		err     error
	}
	followerResult := make(chan response, 1)
	go func() {
		outcome, err := executor.Execute(followerCtx, RiskModerationInput{
			Provider: followerProvider, Content: "same", ReviewMode: model.RiskReviewSelective,
		})
		followerResult <- response{outcome: outcome, err: err}
	}()

	// When
	var follower response
	select {
	case follower = <-followerResult:
	case <-time.After(150 * time.Millisecond):
		close(releaseProvider)
		require.FailNow(t, "inflight follower ignored its caller deadline")
	}
	close(releaseProvider)
	leaderErr := <-leaderResult

	// Then
	require.ErrorIs(t, follower.err, context.DeadlineExceeded)
	assert.False(t, follower.outcome.ProviderCalled)
	require.NoError(t, leaderErr)
	assert.EqualValues(t, 1, calls.Load())
}
