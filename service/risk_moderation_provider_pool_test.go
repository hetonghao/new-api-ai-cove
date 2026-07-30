package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func riskModerationProviderPoolForTest() []*model.RiskProvider {
	providers := make([]*model.RiskProvider, 3)
	for index := range providers {
		provider := riskModerationProviderForTest()
		provider.Id = index + 1
		provider.Name = "provider-" + string(rune('a'+index))
		provider.Model = "guard-" + string(rune('a'+index))
		providers[index] = provider
	}
	return providers
}

func useRiskModerationMiniRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	originalEnabled, originalClient := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = originalEnabled
		common.RDB = originalClient
	})
	return server
}

func TestRiskModerationExecutor_Execute_roundRobinsProviderPoolAcrossInstances(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()
	selected := make([]int, 0, 4)
	reviewer := func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
		selected = append(selected, provider.Id)
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	newExecutor := func(secret string) *RiskModerationExecutor {
		return newRiskModerationExecutor(riskModerationExecutorDeps{
			Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), secret), Reviewer: reviewer, Now: time.Now,
		})
	}
	firstInstance := newExecutor("pool-instance-a")
	secondInstance := newExecutor("pool-instance-b")

	// When
	for index, executor := range []*RiskModerationExecutor{firstInstance, secondInstance, firstInstance, secondInstance} {
		outcome, err := executor.Execute(context.Background(), RiskModerationInput{
			Providers: providers, Content: "content-" + string(rune('a'+index)), ReviewMode: model.RiskReviewSelective,
		})
		require.NoError(t, err)
		assert.True(t, outcome.ProviderCalled)
	}

	// Then
	assert.Equal(t, []int{1, 2, 3, 1}, selected)
}

func TestRiskModerationPolicyVersion_changesWithProviderPoolOrder(t *testing.T) {
	// Given
	providers := riskModerationProviderPoolForTest()
	input := RiskModerationInput{Providers: providers, ReviewMode: model.RiskReviewSelective}

	// When
	version, err := RiskModerationPolicyVersion(input)
	require.NoError(t, err)
	reordered := input
	reordered.Providers = []*model.RiskProvider{providers[1], providers[0], providers[2]}
	reorderedVersion, err := RiskModerationPolicyVersion(reordered)

	// Then
	require.NoError(t, err)
	assert.NotEqual(t, version, reorderedVersion)
}

func TestRiskModerationExecutor_Execute_cacheHitDoesNotAdvanceProviderPool(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()
	selected := make([]int, 0, 2)
	reviewer := func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
		selected = append(selected, provider.Id)
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-cache"), Reviewer: reviewer, Now: time.Now,
	})
	input := RiskModerationInput{Providers: providers, Content: "same", ReviewMode: model.RiskReviewSelective}

	// When
	first, firstErr := executor.Execute(context.Background(), input)
	second, secondErr := executor.Execute(context.Background(), input)
	input.Content = "different"
	third, thirdErr := executor.Execute(context.Background(), input)

	// Then
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.NoError(t, thirdErr)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	assert.Equal(t, RiskReviewSourceCache, second.Source)
	assert.Equal(t, RiskReviewSourceProvider, third.Source)
	assert.Equal(t, []int{1, 2}, selected)
	assert.Equal(t, 1, first.Result.ProviderID)
	assert.Equal(t, 1, second.Result.ProviderID)
	assert.Equal(t, 2, third.Result.ProviderID)
}

func TestRiskModerationExecutor_Execute_redisFailureSelectsFirstProvider(t *testing.T) {
	// Given
	server := useRiskModerationMiniRedis(t)
	server.Close()
	providers := riskModerationProviderPoolForTest()
	selected := 0
	reviewer := func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
		selected = provider.Id
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-redis-failure"), Reviewer: reviewer, Now: time.Now,
	})

	// When
	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "text", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, 1, selected)
	assert.Equal(t, 1, outcome.Result.ProviderID)
}

func TestRiskModerationExecutor_Execute_doesNotFailOverProviderPool(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	providers := riskModerationProviderPoolForTest()
	providerErr := errors.New("selected provider failed")
	selected := make([]int, 0, 1)
	reviewer := func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
		selected = append(selected, provider.Id)
		return RiskReviewResult{}, providerErr
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "pool-no-failover"), Reviewer: reviewer, Now: time.Now,
	})

	// When
	outcome, err := executor.Execute(context.Background(), RiskModerationInput{
		Providers: providers, Content: "text", ReviewMode: model.RiskReviewSelective,
	})

	// Then
	require.ErrorIs(t, err, providerErr)
	assert.Equal(t, []int{1}, selected)
	assert.Equal(t, 1, outcome.Result.ProviderID)
}

func TestRiskModerationExecutor_Execute_inflightFollowerDoesNotAdvanceProviderPool(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	store := newFakeRiskReviewCacheStore()
	followerCacheChecked := make(chan struct{})
	store.onGet = func(call int) {
		if call == 2 {
			close(followerCacheChecked)
		}
	}
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	providers := riskModerationProviderPoolForTest()
	selected := make([]int, 0, 2)
	var selectedMu sync.Mutex
	reviewer := func(_ context.Context, provider *model.RiskProvider, _ string) (RiskReviewResult, error) {
		selectedMu.Lock()
		selected = append(selected, provider.Id)
		call := len(selected)
		selectedMu.Unlock()
		if call == 1 {
			close(providerStarted)
			<-releaseProvider
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: newRiskReviewCacheService(store, "pool-singleflight"), Reviewer: reviewer, Now: time.Now,
	})
	input := RiskModerationInput{Providers: providers, Content: "same", ReviewMode: model.RiskReviewSelective}
	type response struct {
		outcome RiskModerationOutcome
		err     error
	}
	leaderResult := make(chan response, 1)
	followerResult := make(chan response, 1)
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

	// When
	close(releaseProvider)
	leader := <-leaderResult
	follower := <-followerResult
	input.Content = "different"
	third, thirdErr := executor.Execute(context.Background(), input)

	// Then
	require.NoError(t, leader.err)
	require.NoError(t, follower.err)
	require.NoError(t, thirdErr)
	assert.Equal(t, RiskReviewSourceProvider, leader.outcome.Source)
	assert.Equal(t, RiskReviewSourceInflight, follower.outcome.Source)
	assert.Equal(t, 1, follower.outcome.Result.ProviderID)
	assert.Equal(t, 2, third.Result.ProviderID)
	assert.Equal(t, []int{1, 2}, selected)
}
