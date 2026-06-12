package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type BillingSelfWallet struct {
	RemainingQuota int `json:"remaining_quota"`
	UsedQuota      int `json:"used_quota"`
}

type BillingSelfSubscription struct {
	Id              int    `json:"id"`
	PlanId          int    `json:"plan_id"`
	Status          string `json:"status"`
	Source          string `json:"source"`
	StartTime       int64  `json:"start_time"`
	EndTime         int64  `json:"end_time"`
	NextResetTime   int64  `json:"next_reset_time"`
	AmountTotal     int64  `json:"amount_total"`
	AmountUsed      int64  `json:"amount_used"`
	AmountRemaining *int64 `json:"amount_remaining,omitempty"`
	Unlimited       bool   `json:"unlimited"`
}

func GetBillingSelf(c *gin.Context) {
	userId := c.GetInt("id")
	remainQuota, err := model.GetUserQuota(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	usedQuota, err := model.GetUserUsedQuota(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	settingMap, err := model.GetUserSetting(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	subscriptions, err := model.GetAllActiveUserSubscriptions(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	items := make([]BillingSelfSubscription, 0, len(subscriptions))
	for _, summary := range subscriptions {
		if summary.Subscription == nil {
			continue
		}
		sub := summary.Subscription
		unlimited := sub.AmountTotal == 0
		var amountRemaining *int64
		if !unlimited {
			remaining := sub.AmountTotal - sub.AmountUsed
			if remaining < 0 {
				remaining = 0
			}
			amountRemaining = &remaining
		}
		items = append(items, BillingSelfSubscription{
			Id:              sub.Id,
			PlanId:          sub.PlanId,
			Status:          sub.Status,
			Source:          sub.Source,
			StartTime:       sub.StartTime,
			EndTime:         sub.EndTime,
			NextResetTime:   sub.NextResetTime,
			AmountTotal:     sub.AmountTotal,
			AmountUsed:      sub.AmountUsed,
			AmountRemaining: amountRemaining,
			Unlimited:       unlimited,
		})
	}

	common.ApiSuccess(c, gin.H{
		"billing_preference":      common.NormalizeBillingPreference(settingMap.BillingPreference),
		"has_active_subscription": len(items) > 0,
		"wallet": BillingSelfWallet{
			RemainingQuota: remainQuota,
			UsedQuota:      usedQuota,
		},
		"subscriptions": items,
	})
}
