package service

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const commandCodeAPIOrigin = "https://api.commandcode.ai"

func commandCodeBillingAccount(ctx context.Context, credential string) (scope, query string, err error) {
	var response struct {
		Success bool `json:"success"`
		User    struct {
			ID string `json:"id"`
		} `json:"user"`
		Org *struct {
			ID string `json:"id"`
		} `json:"org"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, commandCodeAPIOrigin+"/alpha/whoami?limits=1", credential, nil, &response); err != nil {
		return "", "", err
	}
	if !response.Success || response.User.ID == "" || len(response.User.ID) > 256 {
		return "", "", invalidSubscriptionQuota()
	}
	if response.Org == nil {
		return "user:" + response.User.ID, "", nil
	}
	if response.Org.ID == "" || len(response.Org.ID) > 256 {
		return "", "", invalidSubscriptionQuota()
	}
	return "org:" + response.Org.ID, "?orgId=" + url.QueryEscape(response.Org.ID), nil
}

func fetchCommandCodeSubscriptionQuota(ctx context.Context, credential string) (*model.SubscriptionQuota, error) {
	scope, query, err := commandCodeBillingAccount(ctx, credential)
	if err != nil {
		return nil, err
	}
	type window struct {
		Used     *float64 `json:"used"`
		Cap      *float64 `json:"cap"`
		Exceeded bool     `json:"exceeded"`
		ResetAt  *int64   `json:"resetAt"`
	}
	var response struct {
		Credits *struct {
			Remaining *float64 `json:"monthlyCredits"`
			Granted   *float64 `json:"monthlyCreditsGranted"`
		} `json:"credits"`
		WindowLimits *struct {
			FiveHour *window `json:"fiveHour"`
			Weekly   *window `json:"weekly"`
		} `json:"windowLimits"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, commandCodeAPIOrigin+"/alpha/billing/credits"+query, credential, nil, &response); err != nil {
		return nil, err
	}
	if response.WindowLimits == nil {
		return nil, invalidSubscriptionQuota()
	}
	monthly := &model.SubscriptionMonthlyQuota{}
	if response.Credits != nil {
		for _, amount := range []*float64{response.Credits.Remaining, response.Credits.Granted} {
			if amount != nil && (math.IsNaN(*amount) || math.IsInf(*amount, 0)) {
				return nil, invalidSubscriptionQuota()
			}
		}
		monthly.Remaining, monthly.Granted = response.Credits.Remaining, response.Credits.Granted
	}
	quota := &model.SubscriptionQuota{WorkspaceID: scope, Monthly: monthly, Windows: make([]model.SubscriptionQuotaWindow, 0, 2)}
	for _, item := range []struct {
		key      string
		duration int64
		value    *window
	}{{"rolling", 18000, response.WindowLimits.FiveHour}, {"weekly", 604800, response.WindowLimits.Weekly}} {
		value := item.value
		if value == nil {
			continue
		}
		if value.Used == nil || value.Cap == nil || *value.Used < 0 || *value.Cap < 0 || math.IsNaN(*value.Used) || math.IsInf(*value.Used, 0) || math.IsNaN(*value.Cap) || math.IsInf(*value.Cap, 0) {
			return nil, invalidSubscriptionQuota()
		}
		result := model.SubscriptionQuotaWindow{Key: item.key, DurationSeconds: item.duration, Status: "unavailable"}
		if *value.Cap > 0 {
			percent := *value.Used / *value.Cap * 100
			if math.IsNaN(percent) || math.IsInf(percent, 0) {
				return nil, invalidSubscriptionQuota()
			}
			if value.Exceeded {
				percent = max(100, percent)
			}
			result.UsedPercent, result.Status = &percent, "ok"
		}
		if value.ResetAt != nil {
			if *value.ResetAt < 0 || *value.ResetAt > 253402300799999 {
				return nil, invalidSubscriptionQuota()
			}
			if *value.ResetAt > 0 {
				at := time.UnixMilli(*value.ResetAt).UTC()
				result.ResetsAt = &at
			}
		}
		quota.Windows = append(quota.Windows, result)
	}
	return quota, nil
}

func fetchCommandCodeSubscriptionInfo(ctx context.Context, subscription *model.UpstreamSubscription, credential string) (model.UpstreamSubscriptionInfo, error) {
	var info model.UpstreamSubscriptionInfo
	scope, query, err := commandCodeBillingAccount(ctx, credential)
	if err != nil {
		return info, err
	}
	var quota model.SubscriptionQuota
	if subscription.QuotaJSON != "" && common.UnmarshalJsonStr(subscription.QuotaJSON, &quota) != nil {
		return info, invalidSubscriptionQuota()
	}
	if quota.WorkspaceID != "" && quota.WorkspaceID != scope {
		return info, nil
	}
	var response struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, commandCodeAPIOrigin+"/alpha/billing/subscriptions"+query, credential, nil, &response); err != nil {
		return info, err
	}
	if !response.Success || len(response.Data) == 0 {
		return info, invalidSubscriptionQuota()
	}
	if strings.TrimSpace(string(response.Data)) == "null" {
		return info, nil
	}
	var subscriptionInfo struct {
		CurrentPeriodEnd json.RawMessage `json:"currentPeriodEnd"`
		PlanID           string          `json:"planId"`
		Status           string          `json:"status"`
		Quantity         int64           `json:"quantity"`
	}
	if common.Unmarshal(response.Data, &subscriptionInfo) != nil || len(subscriptionInfo.CurrentPeriodEnd) == 0 || len(subscriptionInfo.PlanID) > 64 || len(subscriptionInfo.Status) > 64 {
		return info, invalidSubscriptionQuota()
	}
	if common.Unmarshal(subscriptionInfo.CurrentPeriodEnd, &info.PeriodEnd) != nil || (info.PeriodEnd != nil && info.PeriodEnd.IsZero()) {
		return info, invalidSubscriptionQuota()
	}
	info.ScopeID, info.Tier = scope, subscriptionInfo.PlanID
	// shortcut: Studio base limits as of 2026-11-15; refresh when plan-tiers changes. Live grants take precedence.
	// https://commandcode.ai/assets/plan-tiers-1RNg_E6d.js
	baseLimits := map[string]float64{
		"individual-go": 10, "individual-go-v1": 10, "individual-goat": 70,
		"individual-pro": 30, "individual-pro-v1": 80, "individual-provider": 15,
		"individual-max": 150, "individual-ultra": 300, "teams-pro": 40,
	}
	if limit, known := baseLimits[info.Tier]; known && subscriptionInfo.Status != "past_due" {
		info.Monthly = &model.SubscriptionMonthlyQuota{BaseLimit: limit, Seats: 1}
		if info.Tier == "teams-pro" {
			info.Monthly.Seats = max(1, subscriptionInfo.Quantity)
		}
	}
	return info, nil
}
