package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type BillingSelfWallet struct {
	RemainingAmount float64 `json:"remaining_amount"`
	UsedAmount      float64 `json:"used_amount"`
}

type BillingSelfSubscription struct {
	Id              int      `json:"id"`
	PlanId          int      `json:"plan_id"`
	Status          string   `json:"status"`
	Source          string   `json:"source"`
	StartTime       int64    `json:"start_time"`
	EndTime         int64    `json:"end_time"`
	NextResetTime   int64    `json:"next_reset_time"`
	AmountTotal     float64  `json:"amount_total"`
	AmountUsed      float64  `json:"amount_used"`
	AmountRemaining *float64 `json:"amount_remaining,omitempty"`
	Unlimited       bool     `json:"unlimited"`
}

type billingSelfSnapshot struct {
	RemainingQuota        int
	UsedQuota             int
	BillingPreference     string
	SubscriptionSummaries []model.SubscriptionSummary
}

func loadBillingSelfSnapshot(c *gin.Context) (*billingSelfSnapshot, bool) {
	userId := c.GetInt("id")
	remainQuota, err := model.GetUserQuota(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}

	usedQuota, err := model.GetUserUsedQuota(userId)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}

	settingMap, err := model.GetUserSetting(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	subscriptions, err := model.GetAllActiveUserSubscriptions(userId)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}

	return &billingSelfSnapshot{
		RemainingQuota:        remainQuota,
		UsedQuota:             usedQuota,
		BillingPreference:     common.NormalizeBillingPreference(settingMap.BillingPreference),
		SubscriptionSummaries: subscriptions,
	}, true
}

func GetBillingSelf(c *gin.Context) {
	snapshot, ok := loadBillingSelfSnapshot(c)
	if !ok {
		return
	}

	items := make([]BillingSelfSubscription, 0, len(snapshot.SubscriptionSummaries))
	for _, summary := range snapshot.SubscriptionSummaries {
		if summary.Subscription == nil {
			continue
		}
		sub := summary.Subscription
		unlimited := sub.AmountTotal == 0
		amountTotalUSD := float64(sub.AmountTotal) / common.QuotaPerUnit
		amountUsedUSD := float64(sub.AmountUsed) / common.QuotaPerUnit
		var amountRemaining *float64
		if !unlimited {
			remainingQuota := sub.AmountTotal - sub.AmountUsed
			if remainingQuota < 0 {
				remainingQuota = 0
			}
			remainingUSD := float64(remainingQuota) / common.QuotaPerUnit
			amountRemaining = &remainingUSD
		}
		items = append(items, BillingSelfSubscription{
			Id:              sub.Id,
			PlanId:          sub.PlanId,
			Status:          sub.Status,
			Source:          sub.Source,
			StartTime:       sub.StartTime,
			EndTime:         sub.EndTime,
			NextResetTime:   sub.NextResetTime,
			AmountTotal:     amountTotalUSD,
			AmountUsed:      amountUsedUSD,
			AmountRemaining: amountRemaining,
			Unlimited:       unlimited,
		})
	}

	common.ApiSuccess(c, gin.H{
		"currency":                "USD",
		"billing_preference":      snapshot.BillingPreference,
		"has_active_subscription": len(items) > 0,
		"wallet": BillingSelfWallet{
			RemainingAmount: float64(snapshot.RemainingQuota) / common.QuotaPerUnit,
			UsedAmount:      float64(snapshot.UsedQuota) / common.QuotaPerUnit,
		},
		"subscriptions": items,
	})
}
