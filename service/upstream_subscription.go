package service

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

func FetchSubscriptionQuota(ctx context.Context, platform, credential string) (*model.SubscriptionQuota, error) {
	switch platform {
	case model.SubscriptionPlatformOpenCode:
		return fetchOpenCodeSubscriptionQuota(ctx, credential)
	case model.SubscriptionPlatformOpenDesign:
		return fetchOpenDesignSubscriptionQuota(ctx, credential)
	case model.SubscriptionPlatformCommandCode:
		return fetchCommandCodeSubscriptionQuota(ctx, credential)
	default:
		return nil, invalidSubscriptionQuota()
	}
}

func fetchOpenCodeSubscriptionQuota(ctx context.Context, credential string) (*model.SubscriptionQuota, error) {
	var response struct {
		Usage map[string]struct {
			Status   string     `json:"status"`
			Percent  *float64   `json:"percent"`
			ResetsAt *time.Time `json:"resetsAt"`
		} `json:"usage"`
	}
	err := subscriptionJSONRequest(ctx, http.MethodGet, "https://opencode.ai/inference/go/v1/usage", credential, nil, &response)
	var failure *subscriptionQuotaError
	if errors.As(err, &failure) && (failure.HTTPStatus == 404 || failure.HTTPStatus == 405) {
		err = subscriptionJSONRequest(ctx, http.MethodGet, "https://opencode.ai/zen/go/v1/usage", credential, nil, &response)
	}
	if err != nil {
		return nil, err
	}
	if len(response.Usage) != 3 {
		return nil, invalidSubscriptionQuota()
	}
	quota := &model.SubscriptionQuota{Windows: make([]model.SubscriptionQuotaWindow, 0, 3)}
	for _, window := range []struct {
		key      string
		duration int64
	}{{"rolling", 18000}, {"weekly", 604800}, {"monthly", 0}} {
		value, exists := response.Usage[window.key]
		if !exists || value.Status == "" || len(value.Status) > 64 || value.ResetsAt == nil || (value.Percent == nil && value.Status == "ok") {
			return nil, invalidSubscriptionQuota()
		}
		if value.Percent != nil && (math.IsNaN(*value.Percent) || math.IsInf(*value.Percent, 0) || *value.Percent < 0) {
			return nil, invalidSubscriptionQuota()
		}
		quota.Windows = append(quota.Windows, model.SubscriptionQuotaWindow{Key: window.key, DurationSeconds: window.duration, UsedPercent: value.Percent, Status: value.Status, ResetsAt: value.ResetsAt})
	}
	return quota, nil
}

func subscriptionCredit(value string) (*big.Int, bool) {
	if value == "" || len(value) > 256 || strings.ContainsFunc(value, func(r rune) bool { return r < '0' || r > '9' }) {
		return nil, false
	}
	return new(big.Int).SetString(value, 10)
}

func fetchOpenDesignSubscriptionQuota(ctx context.Context, credential string) (*model.SubscriptionQuota, error) {
	var workspace struct {
		ID   string `json:"workspaceId"`
		Name string `json:"workspaceName"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, openDesignControlOrigin+"/api/v1/workspaces/current", credential, nil, &workspace); err != nil {
		return nil, err
	}
	if !validSubscriptionDeviceID(workspace.ID) || len(workspace.Name) > 1024 {
		return nil, invalidSubscriptionQuota()
	}
	var response struct {
		WorkspaceID string  `json:"workspaceId"`
		BalanceUSD  *string `json:"balanceUsd"`
		CodingPlan  *struct {
			WorkspaceID string `json:"workspaceId"`
			Eligible    *bool  `json:"eligible"`
			Tier        string `json:"tier"`
			Windows     []struct {
				Duration  int64      `json:"durationSeconds"`
				Used      string     `json:"usedCredits"`
				Limit     string     `json:"limitCredits"`
				Remaining string     `json:"remainingCredits"`
				ResetsAt  *time.Time `json:"resetsAt"`
			} `json:"windows"`
		} `json:"codingPlan"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, openDesignControlOrigin+"/api/v1/workspaces/"+url.PathEscape(workspace.ID)+"/billing/preflight", credential, nil, &response); err != nil {
		return nil, err
	}
	plan := response.CodingPlan
	if response.WorkspaceID != workspace.ID || plan == nil || plan.Eligible == nil || len(plan.Tier) > 64 || (plan.WorkspaceID != "" && plan.WorkspaceID != workspace.ID) {
		return nil, invalidSubscriptionQuota()
	}
	if response.BalanceUSD != nil {
		if len(*response.BalanceUSD) > 64 {
			return nil, invalidSubscriptionQuota()
		}
		if _, ok := new(big.Rat).SetString(*response.BalanceUSD); !ok || strings.ContainsAny(*response.BalanceUSD, "/eE") {
			return nil, invalidSubscriptionQuota()
		}
	}
	quota := &model.SubscriptionQuota{WorkspaceID: workspace.ID, WorkspaceName: workspace.Name, Tier: plan.Tier, Eligible: plan.Eligible, BalanceUSD: response.BalanceUSD, Windows: make([]model.SubscriptionQuotaWindow, 0, len(plan.Windows))}
	seen := map[int64]bool{}
	for _, value := range plan.Windows {
		if !slices.Contains([]int64{18000, 604800}, value.Duration) || seen[value.Duration] || value.ResetsAt == nil {
			return nil, invalidSubscriptionQuota()
		}
		seen[value.Duration] = true
		used, usedOK := subscriptionCredit(value.Used)
		limit, limitOK := subscriptionCredit(value.Limit)
		_, remainingOK := subscriptionCredit(value.Remaining)
		if !usedOK || !limitOK || !remainingOK {
			return nil, invalidSubscriptionQuota()
		}
		window := model.SubscriptionQuotaWindow{Key: "rolling", DurationSeconds: value.Duration, Status: "ok", UsedCredits: value.Used, LimitCredits: value.Limit, RemainingCredits: value.Remaining, ResetsAt: value.ResetsAt}
		if value.Duration == 604800 {
			window.Key = "weekly"
		}
		if limit.Sign() > 0 {
			percentage, _ := new(big.Rat).Mul(new(big.Rat).SetFrac(used, limit), big.NewRat(100, 1)).Float64()
			window.UsedPercent = &percentage
		} else {
			window.Status = "unavailable"
		}
		quota.Windows = append(quota.Windows, window)
	}
	slices.SortFunc(quota.Windows, func(a, b model.SubscriptionQuotaWindow) int { return int(a.DurationSeconds - b.DurationSeconds) })
	return quota, nil
}

func RefreshUpstreamSubscription(ctx context.Context, id int, manual bool) (*model.UpstreamSubscription, error) {
	now := time.Now()
	subscription, claimed, err := model.ClaimUpstreamSubscriptionRefresh(id, now.Unix(), manual)
	if err != nil {
		return subscription, err
	}
	if claimed {
		credential, credentialErr := common.DecryptCredential(subscription.CredentialEncrypted)
		var quota *model.SubscriptionQuota
		code := "credential_error"
		retryAfter := min(subscription.RefreshInterval()*time.Duration(1<<min(subscription.FailureCount, 3)), 24*time.Hour)
		if credentialErr == nil {
			quota, credentialErr = FetchSubscriptionQuota(ctx, subscription.Platform, credential)
			code = ""
			if credentialErr != nil {
				code = "upstream_error"
				var failure *subscriptionQuotaError
				if errors.As(credentialErr, &failure) {
					code = failure.Code
					retryAfter = max(retryAfter, failure.RetryAfter)
				}
			} else {
				retryAfter = subscription.RefreshInterval()
			}
		}
		if err := model.FinishUpstreamSubscriptionRefresh(subscription, quota, code, now.Add(retryAfter).Unix()); err != nil {
			return nil, err
		}
	}
	if ctx.Err() == nil && subscription.SubscriptionInfoNextRefreshAt <= now.Unix() {
		if err := refreshUpstreamSubscriptionInfo(ctx, id); err != nil {
			return nil, err
		}
	}
	return model.GetUpstreamSubscription(id)
}
