package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRiskModerationClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeRiskModerationClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeRiskModerationClock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

func TestRiskModerationExecutor_Execute_circuitAllowsSingleRecoveryProbe(t *testing.T) {
	// Given
	clock := &fakeRiskModerationClock{now: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)}
	provider := riskModerationProviderForTest()
	provider.FailureThreshold = 2
	provider.CooldownSeconds = 30
	providerErr := errors.New("provider unavailable")
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	var calls atomic.Int32
	reviewer := func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
		switch calls.Add(1) {
		case 1, 2:
			return RiskReviewResult{}, providerErr
		case 3:
			close(probeStarted)
			<-releaseProbe
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		default:
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		}
	}
	cache := newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "circuit-test-secret")
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: clock.Now})
	input := func(content string) RiskModerationInput {
		return RiskModerationInput{Provider: provider, Content: content, ReviewMode: model.RiskReviewSelective}
	}

	// When
	first, firstErr := executor.Execute(context.Background(), input("first"))
	second, secondErr := executor.Execute(context.Background(), input("second"))
	blocked, blockedErr := executor.Execute(context.Background(), input("blocked"))
	clock.Advance(30 * time.Second)
	type response struct {
		outcome RiskModerationOutcome
		err     error
	}
	probeResult := make(chan response, 1)
	go func() {
		outcome, err := executor.Execute(context.Background(), input("probe"))
		probeResult <- response{outcome: outcome, err: err}
	}()
	<-probeStarted
	parallel, parallelErr := executor.Execute(context.Background(), input("parallel"))
	close(releaseProbe)
	probe := <-probeResult
	recovered, recoveredErr := executor.Execute(context.Background(), input("recovered"))

	// Then
	require.ErrorIs(t, firstErr, providerErr)
	require.ErrorIs(t, secondErr, providerErr)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	assert.Equal(t, RiskReviewSourceProvider, second.Source)
	assert.True(t, first.ProviderCalled)
	assert.True(t, second.ProviderCalled)
	require.ErrorIs(t, blockedErr, ErrRiskModerationCircuitOpen)
	assert.Equal(t, RiskReviewSourceProvider, blocked.Source)
	assert.False(t, blocked.ProviderCalled)
	require.ErrorIs(t, parallelErr, ErrRiskModerationCircuitOpen)
	assert.Equal(t, RiskReviewSourceProvider, parallel.Source)
	assert.False(t, parallel.ProviderCalled)
	require.NoError(t, probe.err)
	assert.True(t, probe.outcome.ProviderCalled)
	require.NoError(t, recoveredErr)
	assert.True(t, recovered.ProviderCalled)
	assert.EqualValues(t, 4, calls.Load())
}

func TestRiskModerationExecutor_Execute_failedProbeRestartsCooldown(t *testing.T) {
	// Given
	clock := &fakeRiskModerationClock{now: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)}
	provider := riskModerationProviderForTest()
	provider.FailureThreshold = 1
	provider.CooldownSeconds = 30
	providerErr := errors.New("provider unavailable")
	var calls atomic.Int32
	reviewer := func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
		if calls.Add(1) <= 2 {
			return RiskReviewResult{}, providerErr
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	cache := newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "failed-probe-test-secret")
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: clock.Now})
	input := func(content string) RiskModerationInput {
		return RiskModerationInput{Provider: provider, Content: content, ReviewMode: model.RiskReviewSelective}
	}

	// When
	_, firstErr := executor.Execute(context.Background(), input("first"))
	clock.Advance(30 * time.Second)
	_, probeErr := executor.Execute(context.Background(), input("probe"))
	_, blockedErr := executor.Execute(context.Background(), input("blocked"))
	clock.Advance(30 * time.Second)
	recovered, recoveredErr := executor.Execute(context.Background(), input("recovered"))

	// Then
	require.ErrorIs(t, firstErr, providerErr)
	require.ErrorIs(t, probeErr, providerErr)
	require.ErrorIs(t, blockedErr, ErrRiskModerationCircuitOpen)
	require.NoError(t, recoveredErr)
	assert.True(t, recovered.ProviderCalled)
	assert.EqualValues(t, 3, calls.Load())
}

func TestRiskModerationExecutor_Execute_usesCacheWhileCircuitOpen(t *testing.T) {
	// Given
	provider := riskModerationProviderForTest()
	provider.FailureThreshold = 1
	providerErr := errors.New("provider unavailable")
	var calls atomic.Int32
	reviewer := func(_ context.Context, _ *model.RiskProvider, content string) (RiskReviewResult, error) {
		calls.Add(1)
		if content == "failure" {
			return RiskReviewResult{}, providerErr
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	cache := newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "open-cache-test-secret")
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	input := func(content string) RiskModerationInput {
		return RiskModerationInput{Provider: provider, Content: content, ReviewMode: model.RiskReviewSelective}
	}

	// When
	warmed, warmErr := executor.Execute(context.Background(), input("cached"))
	_, failureErr := executor.Execute(context.Background(), input("failure"))
	cached, cachedErr := executor.Execute(context.Background(), input("cached"))

	// Then
	require.NoError(t, warmErr)
	assert.Equal(t, RiskReviewSourceProvider, warmed.Source)
	require.ErrorIs(t, failureErr, providerErr)
	require.NoError(t, cachedErr)
	assert.Equal(t, RiskReviewSourceCache, cached.Source)
	assert.True(t, cached.CacheHit)
	assert.False(t, cached.ProviderCalled)
	assert.EqualValues(t, 2, calls.Load())
}

func TestRiskModerationCircuit_Abandon_allowsNextRecoveryProbe(t *testing.T) {
	// Given
	clock := &fakeRiskModerationClock{now: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)}
	circuit := newRiskModerationCircuit(clock.Now)
	permit, err := circuit.Allow(context.Background(), "policy", 1, 30*time.Second)
	require.NoError(t, err)
	circuit.Failure(context.Background(), permit)
	clock.Advance(30 * time.Second)
	probe, err := circuit.Allow(context.Background(), "policy", 1, 30*time.Second)
	require.NoError(t, err)

	// When
	circuit.Abandon(context.Background(), probe)
	nextProbe, nextErr := circuit.Allow(context.Background(), "policy", 1, 30*time.Second)

	// Then
	require.NoError(t, nextErr)
	assert.True(t, nextProbe.probe)
}

func TestRiskModerationCircuit_sharesOpenStateThroughRedis(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	first := newRiskModerationCircuit(func() time.Time { return now })
	second := newRiskModerationCircuit(func() time.Time { return now })
	permit, err := first.Allow(context.Background(), "shared", 1, 30*time.Second)
	require.NoError(t, err)

	// When
	first.Failure(context.Background(), permit)
	_, blockedErr := second.Allow(context.Background(), "shared", 1, 30*time.Second)

	// Then
	require.ErrorIs(t, blockedErr, ErrRiskModerationCircuitOpen)
}

func TestRiskModerationExecutor_Execute_countsProviderDeadlineFailure(t *testing.T) {
	// Given
	provider := riskModerationProviderForTest()
	provider.FailureThreshold = 1
	var calls atomic.Int32
	reviewer := func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error) {
		calls.Add(1)
		return RiskReviewResult{}, context.DeadlineExceeded
	}
	cache := newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "provider-timeout-test-secret")
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	input := func(content string) RiskModerationInput {
		return RiskModerationInput{Provider: provider, Content: content, ReviewMode: model.RiskReviewSelective}
	}

	// When
	_, firstErr := executor.Execute(context.Background(), input("first"))
	blocked, blockedErr := executor.Execute(context.Background(), input("blocked"))

	// Then
	require.ErrorIs(t, firstErr, context.DeadlineExceeded)
	require.ErrorIs(t, blockedErr, ErrRiskModerationCircuitOpen)
	assert.False(t, blocked.ProviderCalled)
	assert.EqualValues(t, 1, calls.Load())
}

func TestRiskModerationExecutor_Execute_doesNotCountCallerCancellation(t *testing.T) {
	// Given
	provider := riskModerationProviderForTest()
	provider.FailureThreshold = 1
	providerStarted := make(chan struct{})
	var calls atomic.Int32
	reviewer := func(ctx context.Context, _ *model.RiskProvider, _ string) (RiskReviewResult, error) {
		if calls.Add(1) == 1 {
			close(providerStarted)
			<-ctx.Done()
			return RiskReviewResult{}, ctx.Err()
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	cache := newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "caller-cancel-test-secret")
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{Cache: cache, Reviewer: reviewer, Now: time.Now})
	input := func(content string) RiskModerationInput {
		return RiskModerationInput{Provider: provider, Content: content, ReviewMode: model.RiskReviewSelective}
	}
	ctx, cancel := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := executor.Execute(ctx, input("first"))
		firstResult <- err
	}()
	<-providerStarted

	// When
	cancel()
	firstErr := <-firstResult
	_, _ = executor.Execute(context.Background(), input("first"))
	next, nextErr := executor.Execute(context.Background(), input("next"))

	// Then
	require.ErrorIs(t, firstErr, context.Canceled)
	require.NoError(t, nextErr)
	assert.True(t, next.ProviderCalled)
	assert.GreaterOrEqual(t, calls.Load(), int32(2))
}
