package service

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskProviderNeuronsBudget_reservesAtomicallyAndWaitsFiveMinutesAfterReset(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	now := time.Date(2026, 7, 31, 23, 59, 0, 0, time.UTC)
	provider := &model.RiskProvider{Id: 42, ProviderType: model.RiskProviderCloudflare, DailyNeuronsLimit: 10, DailyResetTime: "08:00"}
	budget := newRiskProviderNeuronsBudget(func() time.Time { return now })

	// When
	first, firstErr := budget.Reserve(context.Background(), provider, 6)
	second, secondErr := budget.Reserve(context.Background(), provider, 5)
	now = now.Add(2 * time.Minute)
	third, thirdErr := budget.Reserve(context.Background(), provider, 1)
	now = now.Add(4 * time.Minute)
	fourth, fourthErr := budget.Reserve(context.Background(), provider, 1)
	_ = first
	_ = second
	_ = third
	_ = fourth

	// Then
	require.NoError(t, firstErr)
	require.ErrorIs(t, secondErr, ErrRiskProviderDailyNeuronsExhausted)
	require.ErrorIs(t, thirdErr, ErrRiskProviderDailyNeuronsExhausted)
	require.NoError(t, fourthErr)
	assert.NotEmpty(t, first.Window)
	assert.NotNil(t, common.RDB)
}

func TestRiskProviderNeuronsBudget_requiresSharedRedis(t *testing.T) {
	// Given
	originalEnabled, originalClient := common.RedisEnabled, common.RDB
	common.RedisEnabled = false
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = originalEnabled
		common.RDB = originalClient
	})
	provider := &model.RiskProvider{Id: 43, ProviderType: model.RiskProviderCloudflare, DailyNeuronsLimit: 10, DailyResetTime: "08:00"}
	budget := newRiskProviderNeuronsBudget(time.Now)

	// When
	_, reserveErr := budget.Reserve(context.Background(), provider, 1)
	settleErr := budget.Settle(context.Background(), provider, riskProviderNeuronsReservation{Key: "budget", Window: "window", Estimated: 1}, 1)
	state, stateErr := budget.State(context.Background(), provider)

	// Then
	require.ErrorIs(t, reserveErr, ErrRiskProviderBudgetUnavailable)
	require.ErrorIs(t, settleErr, ErrRiskProviderBudgetUnavailable)
	require.ErrorIs(t, stateErr, ErrRiskProviderBudgetUnavailable)
	assert.Zero(t, state.Used)
}

func TestRiskProviderNeuronsBudget_redisExhaustionStaysUnavailable(t *testing.T) {
	// Given
	useRiskModerationMiniRedis(t)
	provider := &model.RiskProvider{Id: 44, ProviderType: model.RiskProviderCloudflare, DailyNeuronsLimit: 10, DailyResetTime: "08:00"}
	budget := newRiskProviderNeuronsBudget(func() time.Time {
		return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	})
	reservation, err := budget.Reserve(context.Background(), provider, 1)
	require.NoError(t, err)
	require.NoError(t, budget.Exhaust(context.Background(), provider, reservation))

	// When
	_, err = budget.Reserve(context.Background(), provider, 1)

	// Then
	require.ErrorIs(t, err, ErrRiskProviderDailyNeuronsExhausted)
}

func TestRiskProviderNeuronsBudget_settlementCannotExceedDailyLimit(t *testing.T) {
	useRiskModerationMiniRedis(t)
	provider := &model.RiskProvider{Id: 45, ProviderType: model.RiskProviderCloudflare, DailyNeuronsLimit: 10, DailyResetTime: "08:00"}
	budget := newRiskProviderNeuronsBudget(func() time.Time {
		return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	})
	reservation, err := budget.Reserve(context.Background(), provider, 1)
	require.NoError(t, err)

	require.NoError(t, budget.Settle(context.Background(), provider, reservation, 12))
	state, err := budget.State(context.Background(), provider)
	require.NoError(t, err)

	assert.Equal(t, int64(10), state.Used)
	assert.Zero(t, state.Reserved)
	assert.True(t, state.Exhausted)
	_, err = budget.Reserve(context.Background(), provider, 1)
	assert.ErrorIs(t, err, ErrRiskProviderDailyNeuronsExhausted)
}

func TestNormalizeRiskProviderNeuronsSaturatesInvalidAndOverflowingUsage(t *testing.T) {
	maxNeurons := int64(^uint64(0) >> 1)

	assert.Equal(t, int64(43), NormalizeRiskProviderNeurons(42.5))
	assert.Zero(t, NormalizeRiskProviderNeurons(-1))
	assert.Zero(t, NormalizeRiskProviderNeurons(math.NaN()))
	assert.Zero(t, NormalizeRiskProviderNeurons(math.Inf(-1)))
	assert.Equal(t, maxNeurons, NormalizeRiskProviderNeurons(math.Inf(1)))
}

func TestEstimateCloudflareNeuronsUsesConservativeUTF8Size(t *testing.T) {
	ascii := EstimateCloudflareNeurons(strings.Repeat("a", 4000))
	unicode := EstimateCloudflareNeurons(strings.Repeat("风", 4000))

	assert.Greater(t, unicode, ascii)
}

func TestReviewRiskContentWithBudgetChargesEstimateWhenUsageIsMissing(t *testing.T) {
	useRiskModerationMiniRedis(t)
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "risk-provider-budget-test-secret"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })
	originalHTTPClient := httpClient
	httpClient = &http.Client{Transport: riskProviderRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}}`)),
		}, nil
	})}
	t.Cleanup(func() { httpClient = originalHTTPClient })

	provider := riskProviderTestProvider(t)
	provider.Id = 46
	provider.DailyNeuronsLimit = 6
	provider.DailyResetTime = "08:00"

	result, err := ReviewRiskContentWithBudget(context.Background(), provider, "x")
	require.NoError(t, err)
	assert.Equal(t, RiskReviewSafe, result.Status)
	snapshot, err := GetRiskProviderBudgetSnapshot(context.Background(), provider)
	require.NoError(t, err)
	assert.Equal(t, int64(6), snapshot.Used)
	assert.True(t, snapshot.Exhausted)
}

func TestReviewRiskContentWithBudgetAccountsForProviderFailure(t *testing.T) {
	useRiskModerationMiniRedis(t)
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "risk-provider-budget-failure-test-secret"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })
	originalHTTPClient := httpClient
	t.Cleanup(func() { httpClient = originalHTTPClient })

	tests := []struct {
		name        string
		transport   riskProviderRoundTripFunc
		wantUsed    int64
		rateLimited bool
	}{
		{
			name:     "network failure",
			wantUsed: EstimateCloudflareNeurons("x"),
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("network unavailable")
			},
		},
		{
			name:     "http failure",
			wantUsed: EstimateCloudflareNeurons("x"),
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Body:       io.NopCloser(strings.NewReader(`{"error":"upstream failed"}`)),
				}, nil
			},
		},
		{
			name:        "generic http 429",
			wantUsed:    0,
			rateLimited: true,
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Body:       io.NopCloser(strings.NewReader(`{"error":"rate limit"}`)),
				}, nil
			},
		},
		{
			name:     "request timeout",
			wantUsed: EstimateCloudflareNeurons("x"),
			transport: func(*http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			},
		},
		{
			name:     "invalid moderation response",
			wantUsed: EstimateCloudflareNeurons("x"),
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(
						`{"success":true,"result":{"response":{"unknown":true}}}`,
					)),
				}, nil
			},
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient = &http.Client{Transport: tt.transport}
			provider := riskProviderTestProvider(t)
			provider.Id = 50 + index
			provider.DailyNeuronsLimit = 100
			provider.DailyResetTime = "08:00"

			_, err := ReviewRiskContentWithBudget(context.Background(), provider, "x")
			require.Error(t, err)
			snapshot, snapshotErr := GetRiskProviderBudgetSnapshot(context.Background(), provider)
			require.NoError(t, snapshotErr)
			assert.Equal(t, tt.wantUsed, snapshot.Used)
			assert.Zero(t, snapshot.Reserved)
			assert.False(t, snapshot.Exhausted)
			if tt.rateLimited {
				require.ErrorIs(t, err, ErrRiskProviderRateLimited)
			}
		})
	}
}
