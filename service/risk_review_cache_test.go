package service

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRiskReviewCacheStore struct {
	mu       sync.Mutex
	values   map[string]RiskReviewResult
	getErr   error
	setErr   error
	getCalls int
	setCalls int
	lastKey  string
	lastTTL  time.Duration
	onGet    func(int)
	get      func(context.Context, string) (RiskReviewResult, bool, error)
}

func newFakeRiskReviewCacheStore() *fakeRiskReviewCacheStore {
	return &fakeRiskReviewCacheStore{values: make(map[string]RiskReviewResult)}
}

func (s *fakeRiskReviewCacheStore) Get(ctx context.Context, key string) (RiskReviewResult, bool, error) {
	if s.get != nil {
		return s.get(ctx, key)
	}
	s.mu.Lock()
	s.getCalls++
	call := s.getCalls
	result, found := s.values[key]
	err := s.getErr
	onGet := s.onGet
	s.mu.Unlock()
	if onGet != nil {
		onGet(call)
	}
	return result, found, err
}

func (s *fakeRiskReviewCacheStore) Set(_ context.Context, key string, result RiskReviewResult, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls++
	s.lastKey = key
	s.lastTTL = ttl
	if s.setErr != nil {
		return s.setErr
	}
	s.values[key] = result
	return nil
}

func (s *fakeRiskReviewCacheStore) snapshot() (getCalls, setCalls int, key string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getCalls, s.setCalls, s.lastKey, s.lastTTL
}

func TestRiskReviewCacheService_CacheKey_normalizesContentAndVersionsPolicy(t *testing.T) {
	// Given
	service := newRiskReviewCacheService(newFakeRiskReviewCacheStore(), "fixed-secret")

	// When
	first, err := service.CacheKey(RiskReviewCacheInput{Content: "  HELLO\u3000World  ", PolicyVersion: "policy-7"})
	require.NoError(t, err)
	second, err := service.CacheKey(RiskReviewCacheInput{Content: "hello world", PolicyVersion: "policy-7"})
	require.NoError(t, err)
	changedPolicy, err := service.CacheKey(RiskReviewCacheInput{Content: "hello world", PolicyVersion: "policy-8"})
	require.NoError(t, err)

	// Then
	assert.Equal(t, first, second)
	assert.NotEqual(t, first, changedPolicy)
	assert.Equal(t, "new-api:risk-review:v1:cG9saWN5LTc:4af4d02f6f1f217d4c35ee0a2a94be69f4d0c19b5e5c412d72058ce7c9118be5", first)
	assert.NotContains(t, first, "hello")
	assert.NotContains(t, first, "provider")
	assert.NotContains(t, first, "model")
}

func TestRiskReviewCacheService_Review_cachesSafeAndUnsafeFor24Hours(t *testing.T) {
	tests := []struct {
		name   string
		result RiskReviewResult
	}{
		{name: "safe", result: RiskReviewResult{Status: RiskReviewSafe}},
		{name: "unsafe", result: RiskReviewResult{Status: RiskReviewUnsafe, Categories: []string{"S1"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			store := newFakeRiskReviewCacheStore()
			service := newRiskReviewCacheService(store, "fixed-secret")
			var calls atomic.Int32
			reviewer := func(context.Context) (RiskReviewResult, error) {
				calls.Add(1)
				return test.result, nil
			}

			// When
			providerOutcome, err := service.Review(context.Background(), RiskReviewCacheInput{Content: "private text", PolicyVersion: "v1"}, reviewer)
			require.NoError(t, err)
			cacheOutcome, err := service.Review(context.Background(), RiskReviewCacheInput{Content: " PRIVATE   TEXT ", PolicyVersion: "v1"}, reviewer)
			require.NoError(t, err)

			// Then
			assert.Equal(t, RiskReviewSourceProvider, providerOutcome.Source)
			assert.Equal(t, RiskReviewSourceCache, cacheOutcome.Source)
			assert.Equal(t, test.result, cacheOutcome.Result)
			assert.EqualValues(t, 1, calls.Load())
			_, setCalls, key, ttl := store.snapshot()
			assert.Equal(t, 1, setCalls)
			assert.Equal(t, 24*time.Hour, ttl)
			assert.False(t, strings.Contains(key, "private text"))
		})
	}
}

func TestRiskReviewCacheService_Review_preservesAndDoesNotCacheProviderErrors(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	service := newRiskReviewCacheService(store, "fixed-secret")
	providerErr := errors.New("provider unavailable")
	partial := RiskReviewResult{
		Status: RiskReviewError, Categories: []string{"partial"},
		Usage: RiskReviewUsage{PromptTokens: 3, TotalTokens: 3},
	}
	var calls atomic.Int32
	reviewer := func(context.Context) (RiskReviewResult, error) {
		if calls.Add(1) == 1 {
			return partial, providerErr
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}

	// When
	first, firstErr := service.Review(context.Background(), RiskReviewCacheInput{Content: "text", PolicyVersion: "v1"}, reviewer)
	second, secondErr := service.Review(context.Background(), RiskReviewCacheInput{Content: "text", PolicyVersion: "v1"}, reviewer)

	// Then
	require.ErrorIs(t, firstErr, providerErr)
	assert.Equal(t, partial, first.Result)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	require.NoError(t, secondErr)
	assert.Equal(t, RiskReviewSourceProvider, second.Source)
	assert.EqualValues(t, 2, calls.Load())
	_, setCalls, _, _ := store.snapshot()
	assert.Equal(t, 1, setCalls)
}

func TestRiskReviewCacheService_Review_degradesWhenCacheFails(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	store.getErr = errors.New("redis get failed")
	store.setErr = errors.New("redis set failed")
	service := newRiskReviewCacheService(store, "fixed-secret")
	var calls atomic.Int32
	reviewer := func(context.Context) (RiskReviewResult, error) {
		calls.Add(1)
		return RiskReviewResult{Status: RiskReviewUnsafe}, nil
	}

	// When
	first, firstErr := service.Review(context.Background(), RiskReviewCacheInput{Content: "text", PolicyVersion: "v1"}, reviewer)
	second, secondErr := service.Review(context.Background(), RiskReviewCacheInput{Content: "text", PolicyVersion: "v1"}, reviewer)

	// Then
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, RiskReviewSourceProvider, first.Source)
	assert.Equal(t, RiskReviewSourceProvider, second.Source)
	assert.EqualValues(t, 2, calls.Load())
}

func TestRiskReviewCacheService_Review_coalescesConcurrentSameKey(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	followerCacheChecked := make(chan struct{})
	store.onGet = func(call int) {
		if call == 2 {
			close(followerCacheChecked)
		}
	}
	service := newRiskReviewCacheService(store, "fixed-secret")
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	var calls atomic.Int32
	leaderReviewer := func(context.Context) (RiskReviewResult, error) {
		calls.Add(1)
		close(providerStarted)
		<-releaseProvider
		return RiskReviewResult{Status: RiskReviewUnsafe, Categories: []string{"S9"}}, nil
	}
	followerReviewer := func(context.Context) (RiskReviewResult, error) {
		calls.Add(1)
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}
	input := RiskReviewCacheInput{Content: "same text", PolicyVersion: "v1"}
	type response struct {
		outcome RiskReviewOutcome
		err     error
	}
	leaderResult := make(chan response, 1)
	followerResult := make(chan response, 1)

	// When
	go func() {
		outcome, err := service.Review(context.Background(), input, leaderReviewer)
		leaderResult <- response{outcome: outcome, err: err}
	}()
	<-providerStarted
	go func() {
		outcome, err := service.Review(context.Background(), input, followerReviewer)
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
	assert.Equal(t, RiskReviewSourceInflight, follower.outcome.Source)
	assert.Equal(t, leader.outcome.Result, follower.outcome.Result)
	assert.EqualValues(t, 1, calls.Load())
}

func TestRiskReviewCacheService_Review_sanitizesInflightProviderErrors(t *testing.T) {
	// Given
	store := newFakeRiskReviewCacheStore()
	followerCacheChecked := make(chan struct{})
	store.onGet = func(call int) {
		if call == 2 {
			close(followerCacheChecked)
		}
	}
	service := newRiskReviewCacheService(store, "fixed-secret")
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	providerErr := errors.New("provider unavailable")
	providerResult := RiskReviewResult{
		Status: RiskReviewError, ProviderID: 7, ProviderName: "secret-provider", ProviderType: "cloudflare",
		Usage: RiskReviewUsage{PromptTokens: 3, TotalTokens: 3},
	}
	var calls atomic.Int32
	input := RiskReviewCacheInput{Content: "same error text", PolicyVersion: "v1"}
	type response struct {
		outcome RiskReviewOutcome
		err     error
	}
	leaderResult := make(chan response, 1)
	followerResult := make(chan response, 1)

	// When
	go func() {
		outcome, err := service.Review(context.Background(), input, func(context.Context) (RiskReviewResult, error) {
			calls.Add(1)
			close(providerStarted)
			<-releaseProvider
			return providerResult, providerErr
		})
		leaderResult <- response{outcome: outcome, err: err}
	}()
	<-providerStarted
	go func() {
		outcome, err := service.Review(context.Background(), input, func(context.Context) (RiskReviewResult, error) {
			calls.Add(1)
			return RiskReviewResult{Status: RiskReviewSafe}, nil
		})
		followerResult <- response{outcome: outcome, err: err}
	}()
	<-followerCacheChecked
	close(releaseProvider)
	leader := <-leaderResult
	follower := <-followerResult

	// Then
	require.ErrorIs(t, leader.err, providerErr)
	assert.Equal(t, providerResult, leader.outcome.Result)
	assert.Equal(t, RiskReviewSourceProvider, leader.outcome.Source)
	require.ErrorIs(t, follower.err, providerErr)
	assert.Equal(t, RiskReviewSourceInflight, follower.outcome.Source)
	assert.Zero(t, follower.outcome.Result.ProviderID)
	assert.Empty(t, follower.outcome.Result.ProviderName)
	assert.Empty(t, follower.outcome.Result.ProviderType)
	assert.Zero(t, follower.outcome.Result.Usage)
	assert.EqualValues(t, 1, calls.Load())
}
