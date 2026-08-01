package service

import (
	"context"
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
