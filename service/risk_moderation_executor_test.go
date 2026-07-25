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

func riskModerationProviderForTest() *model.RiskProvider {
	return &model.RiskProvider{
		Id: 7, ProviderType: model.RiskProviderCloudflare,
		Model: "guard-v1", BaseURL: "https://risk.example/v1",
		TimeoutMs: 800, FailureThreshold: 2, CooldownSeconds: 30,
		UpdatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
}

func TestRiskModerationPolicyVersion_changesWithVerdictSemanticsOnly(t *testing.T) {
	// Given
	base := RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text",
		ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2048,
	}

	// When
	version, err := RiskModerationPolicyVersion(base)
	require.NoError(t, err)
	changedModel := base
	changedModel.Provider = riskModerationProviderForTest()
	changedModel.Provider.Model = "guard-v2"
	modelVersion, err := RiskModerationPolicyVersion(changedModel)
	require.NoError(t, err)
	changedEndpoint := base
	changedEndpoint.Provider = riskModerationProviderForTest()
	changedEndpoint.Provider.BaseURL = "https://other-risk.example/v1"
	endpointVersion, err := RiskModerationPolicyVersion(changedEndpoint)
	require.NoError(t, err)
	changedIdentity := base
	changedIdentity.Provider = riskModerationProviderForTest()
	changedIdentity.Provider.Id = 8
	identityVersion, err := RiskModerationPolicyVersion(changedIdentity)
	require.NoError(t, err)
	changedChunks := base
	changedChunks.FullReviewChunkRunes = 1024
	chunkVersion, err := RiskModerationPolicyVersion(changedChunks)
	require.NoError(t, err)
	changedMode := base
	changedMode.ReviewMode = model.RiskReviewSelective
	modeVersion, err := RiskModerationPolicyVersion(changedMode)
	require.NoError(t, err)
	operationalChange := base
	operationalChange.Provider = riskModerationProviderForTest()
	operationalChange.Provider.TimeoutMs = 900
	operationalChange.Provider.FailureThreshold = 9
	operationalChange.Provider.CooldownSeconds = 90
	operationalChange.Provider.CredentialEncrypted = "rotated-secret"
	validatedAt := operationalChange.Provider.UpdatedAt.Add(-time.Hour)
	operationalChange.Provider.ValidatedAt = &validatedAt
	operationalChange.Provider.Active = true
	operationalChange.Provider.UpdatedAt = operationalChange.Provider.UpdatedAt.Add(time.Hour)
	operationalVersion, err := RiskModerationPolicyVersion(operationalChange)
	require.NoError(t, err)

	// Then
	assert.Equal(t, "b0c834dddc29cf2ab839a5ae1381d12f2d559278b98bfec15945c0503beeda52", version)
	assert.NotEqual(t, version, modelVersion)
	assert.NotEqual(t, version, endpointVersion)
	assert.NotEqual(t, version, identityVersion)
	assert.NotEqual(t, version, chunkVersion)
	assert.NotEqual(t, version, modeVersion)
	assert.Equal(t, version, operationalVersion)
	assert.NotContains(t, version, base.Provider.Model)
	assert.NotContains(t, version, string(base.Provider.ProviderType))
}

func TestRiskModerationPolicyVersion_usesCloudflareFullReviewChunkDefault(t *testing.T) {
	// Given
	defaultInput := RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text", ReviewMode: model.RiskReviewFull,
	}
	explicitInput := defaultInput
	explicitInput.FullReviewChunkRunes = RiskModerationCloudflareFullReviewChunkRunes

	// When
	defaultVersion, defaultErr := RiskModerationPolicyVersion(defaultInput)
	explicitVersion, explicitErr := RiskModerationPolicyVersion(explicitInput)

	// Then
	require.NoError(t, defaultErr)
	require.NoError(t, explicitErr)
	assert.Equal(t, 16000, RiskModerationCloudflareFullReviewChunkRunes)
	assert.Equal(t, explicitVersion, defaultVersion)
}

func TestRiskModerationPolicyVersion_rejectsNegativeFullReviewChunkOverride(t *testing.T) {
	// Given
	input := RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text",
		ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: -1,
	}

	// When
	_, err := RiskModerationPolicyVersion(input)

	// Then
	require.ErrorIs(t, err, ErrInvalidRiskModerationInput)
}

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
	input := RiskModerationInput{Provider: riskModerationProviderForTest(), Content: "text", ReviewMode: model.RiskReviewSelective}

	// When
	first, firstErr := executor.Execute(context.Background(), input)
	second, secondErr := executor.Execute(context.Background(), input)

	// Then
	require.ErrorIs(t, firstErr, providerErr)
	require.ErrorIs(t, firstErr, ErrRiskModerationProvider)
	assert.Equal(t, partial, first.Result)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	assert.True(t, first.ProviderCalled)
	require.ErrorIs(t, secondErr, providerErr)
	assert.Equal(t, partial, second.Result)
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
	assert.Equal(t, first.Result, second.Result)
	assert.Equal(t, RiskReviewSourceCache, second.Source)
	assert.True(t, second.CacheHit)
	assert.False(t, second.ProviderCalled)
	assert.Equal(t, 3, calls)
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

func TestRiskModerationExecutor_Execute_reusesOneProviderDeadlineAcrossChunks(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	cache := newRiskReviewCacheService(store, "deadline-test-secret")
	provider := riskModerationProviderForTest()
	var deadlines []time.Time
	reviewer := func(ctx context.Context, _ *model.RiskProvider, _ string) (RiskReviewResult, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		deadlines = append(deadlines, deadline)
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})

	// When
	_, err := executor.Execute(context.Background(), RiskModerationInput{
		Provider: provider, Content: "abcdef", ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2,
	})

	// Then
	require.NoError(t, err)
	require.Len(t, deadlines, 3)
	assert.Equal(t, deadlines[0], deadlines[1])
	assert.Equal(t, deadlines[0], deadlines[2])
}
