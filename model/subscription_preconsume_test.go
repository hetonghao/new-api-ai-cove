package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertSubscriptionPlanForPreConsumeTest(t *testing.T, id int, resetPeriod string, resetSeconds int64) *SubscriptionPlan {
	t.Helper()
	InvalidateSubscriptionPlanCache(id)
	plan := &SubscriptionPlan{
		Id:                      id,
		Title:                   "PreConsume Test Plan",
		PriceAmount:             9.99,
		Currency:                "USD",
		DurationUnit:            SubscriptionDurationMonth,
		DurationValue:           1,
		Enabled:                 true,
		TotalAmount:             100,
		QuotaResetPeriod:        resetPeriod,
		QuotaResetCustomSeconds: resetSeconds,
	}
	require.NoError(t, DB.Create(plan).Error)
	InvalidateSubscriptionPlanCache(id)
	return plan
}

func insertUserSubscriptionForPreConsumeTest(t *testing.T, sub *UserSubscription) {
	t.Helper()
	require.NoError(t, DB.Create(sub).Error)
}

func loadUserSubscriptionForPreConsumeTest(t *testing.T, subID int) UserSubscription {
	t.Helper()
	var sub UserSubscription
	require.NoError(t, DB.Where("id = ?", subID).First(&sub).Error)
	return sub
}

func TestPreConsumeUserSubscriptionPrefersSubscriptionsWithReset(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	noResetPlan := insertSubscriptionPlanForPreConsumeTest(t, 9101, SubscriptionResetNever, 0)
	resetPlan := insertSubscriptionPlanForPreConsumeTest(t, 9102, SubscriptionResetDaily, 0)

	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9201,
		UserId:        9301,
		PlanId:        noResetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 600,
		Status:        "active",
		LastResetTime: now - 3600,
	})
	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9202,
		UserId:        9301,
		PlanId:        resetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 3600,
		Status:        "active",
		LastResetTime: now - 3600,
		NextResetTime: now + 300,
	})

	result, err := PreConsumeUserSubscription("preconsume-prefers-reset", 9301, "test-model", 0, 10)
	require.NoError(t, err)

	assert.Equal(t, 9202, result.UserSubscriptionId)
	assert.EqualValues(t, 10, loadUserSubscriptionForPreConsumeTest(t, 9202).AmountUsed)
	assert.EqualValues(t, 0, loadUserSubscriptionForPreConsumeTest(t, 9201).AmountUsed)
}

func TestPreConsumeUserSubscriptionPrefersEarlierNextResetTime(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	resetPlan := insertSubscriptionPlanForPreConsumeTest(t, 9103, SubscriptionResetDaily, 0)
	laterResetPlan := insertSubscriptionPlanForPreConsumeTest(t, 9104, SubscriptionResetWeekly, 0)

	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9203,
		UserId:        9302,
		PlanId:        resetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 600,
		Status:        "active",
		LastResetTime: now - 3600,
		NextResetTime: now + 900,
	})
	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9204,
		UserId:        9302,
		PlanId:        laterResetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 3600,
		Status:        "active",
		LastResetTime: now - 3600,
		NextResetTime: now + 300,
	})

	result, err := PreConsumeUserSubscription("preconsume-prefers-earlier-reset", 9302, "test-model", 0, 10)
	require.NoError(t, err)

	assert.Equal(t, 9204, result.UserSubscriptionId)
	assert.EqualValues(t, 10, loadUserSubscriptionForPreConsumeTest(t, 9204).AmountUsed)
	assert.EqualValues(t, 0, loadUserSubscriptionForPreConsumeTest(t, 9203).AmountUsed)
}

func TestPreConsumeUserSubscriptionSortsAfterApplyingDueReset(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	dueResetPlan := insertSubscriptionPlanForPreConsumeTest(t, 9105, SubscriptionResetCustom, 1000)
	soonResetPlan := insertSubscriptionPlanForPreConsumeTest(t, 9106, SubscriptionResetCustom, 1000)

	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9205,
		UserId:        9303,
		PlanId:        dueResetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    20,
		StartTime:     now - 5000,
		EndTime:       now + 5000,
		Status:        "active",
		LastResetTime: now - 1000,
		NextResetTime: now - 1,
	})
	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9206,
		UserId:        9303,
		PlanId:        soonResetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 5000,
		EndTime:       now + 5000,
		Status:        "active",
		LastResetTime: now - 500,
		NextResetTime: now + 60,
	})

	result, err := PreConsumeUserSubscription("preconsume-sorts-after-due-reset", 9303, "test-model", 0, 10)
	require.NoError(t, err)

	assert.Equal(t, 9206, result.UserSubscriptionId)

	dueResetSub := loadUserSubscriptionForPreConsumeTest(t, 9205)
	assert.EqualValues(t, 0, dueResetSub.AmountUsed)
	assert.Greater(t, dueResetSub.NextResetTime, now+60)
}

func TestPreConsumeUserSubscriptionFallsBackToEndTimeThenID(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	noResetPlan := insertSubscriptionPlanForPreConsumeTest(t, 9107, SubscriptionResetNever, 0)

	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9209,
		UserId:        9304,
		PlanId:        noResetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 900,
		Status:        "active",
		LastResetTime: now - 3600,
	})
	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9208,
		UserId:        9304,
		PlanId:        noResetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 300,
		Status:        "active",
		LastResetTime: now - 3600,
	})
	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9207,
		UserId:        9304,
		PlanId:        noResetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 300,
		Status:        "active",
		LastResetTime: now - 3600,
	})

	result, err := PreConsumeUserSubscription("preconsume-fallback-order", 9304, "test-model", 0, 10)
	require.NoError(t, err)

	assert.Equal(t, 9207, result.UserSubscriptionId)
	assert.EqualValues(t, 10, loadUserSubscriptionForPreConsumeTest(t, 9207).AmountUsed)
	assert.EqualValues(t, 0, loadUserSubscriptionForPreConsumeTest(t, 9208).AmountUsed)
	assert.EqualValues(t, 0, loadUserSubscriptionForPreConsumeTest(t, 9209).AmountUsed)
}

func TestPreConsumeUserSubscriptionFallsBackToEndTimeThenIDWhenNextResetTimeMatches(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	resetPlan := insertSubscriptionPlanForPreConsumeTest(t, 9108, SubscriptionResetDaily, 0)

	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9212,
		UserId:        9305,
		PlanId:        resetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 900,
		Status:        "active",
		LastResetTime: now - 3600,
		NextResetTime: now + 300,
	})
	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9211,
		UserId:        9305,
		PlanId:        resetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 600,
		Status:        "active",
		LastResetTime: now - 3600,
		NextResetTime: now + 300,
	})
	insertUserSubscriptionForPreConsumeTest(t, &UserSubscription{
		Id:            9210,
		UserId:        9305,
		PlanId:        resetPlan.Id,
		AmountTotal:   100,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 600,
		Status:        "active",
		LastResetTime: now - 3600,
		NextResetTime: now + 300,
	})

	result, err := PreConsumeUserSubscription("preconsume-fallback-same-reset-time", 9305, "test-model", 0, 10)
	require.NoError(t, err)

	assert.Equal(t, 9210, result.UserSubscriptionId)
	assert.EqualValues(t, 10, loadUserSubscriptionForPreConsumeTest(t, 9210).AmountUsed)
	assert.EqualValues(t, 0, loadUserSubscriptionForPreConsumeTest(t, 9211).AmountUsed)
	assert.EqualValues(t, 0, loadUserSubscriptionForPreConsumeTest(t, 9212).AmountUsed)
}
