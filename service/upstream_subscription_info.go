package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func fetchUpstreamSubscriptionInfo(ctx context.Context, subscription *model.UpstreamSubscription, credential string) (model.UpstreamSubscriptionInfo, error) {
	var info model.UpstreamSubscriptionInfo
	if subscription.Platform == model.SubscriptionPlatformCommandCode {
		return fetchCommandCodeSubscriptionInfo(ctx, subscription, credential)
	}
	if subscription.Platform == model.SubscriptionPlatformOpenCode {
		var response struct {
			Access json.RawMessage `json:"access"`
		}
		if err := subscriptionJSONRequest(ctx, http.MethodGet, "https://opencode.ai/console/api/go/status", credential, nil, &response); err != nil {
			return info, err
		}
		if len(response.Access) == 0 {
			return info, invalidSubscriptionQuota()
		}
		if strings.TrimSpace(string(response.Access)) == "null" {
			return info, nil
		}
		var access struct {
			EndsAt *time.Time `json:"endsAt"`
		}
		if common.Unmarshal(response.Access, &access) != nil || access.EndsAt == nil || access.EndsAt.IsZero() {
			return info, invalidSubscriptionQuota()
		}
		info.PeriodEnd = access.EndsAt
		return info, nil
	}
	if subscription.Platform != model.SubscriptionPlatformOpenDesign {
		return info, invalidSubscriptionQuota()
	}
	var workspace struct {
		ID   string `json:"workspaceId"`
		Type string `json:"workspaceType"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, openDesignControlOrigin+"/api/v1/workspaces/current", credential, nil, &workspace); err != nil {
		return info, err
	}
	var quota model.SubscriptionQuota
	if !validSubscriptionDeviceID(workspace.ID) || (subscription.QuotaJSON != "" && common.UnmarshalJsonStr(subscription.QuotaJSON, &quota) != nil) {
		return info, invalidSubscriptionQuota()
	}
	// Quota pulls may be paused; discard the old workspace's expiry independently.
	if quota.WorkspaceID != "" && quota.WorkspaceID != workspace.ID {
		return info, nil
	}
	if workspace.Type == "team" {
		return info, nil
	}
	if workspace.Type != "personal" {
		return info, invalidSubscriptionQuota()
	}
	var summary struct {
		PeriodEnd json.RawMessage `json:"subscriptionCurrentPeriodEnd"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, openDesignControlOrigin+"/api/v1/billing/summary", credential, nil, &summary); err != nil {
		return info, err
	}
	if len(summary.PeriodEnd) == 0 {
		return info, invalidSubscriptionQuota()
	}
	var periodEnd *time.Time
	if common.Unmarshal(summary.PeriodEnd, &periodEnd) != nil || (periodEnd != nil && periodEnd.IsZero()) {
		return info, invalidSubscriptionQuota()
	}
	info.PeriodEnd = periodEnd
	return info, nil
}

func refreshUpstreamSubscriptionInfo(ctx context.Context, id int) error {
	now := time.Now()
	subscription, claimed, err := model.ClaimUpstreamSubscriptionInfoRefresh(id, now.Unix())
	if err != nil || !claimed {
		return err
	}
	credential, err := common.DecryptCredential(subscription.CredentialEncrypted)
	var info model.UpstreamSubscriptionInfo
	if err == nil {
		infoContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		info, err = fetchUpstreamSubscriptionInfo(infoContext, subscription, credential)
	}
	next := now.Add(6 * time.Hour)
	if err != nil {
		next = now.Add(15 * time.Minute)
		var failure *subscriptionQuotaError
		if errors.As(err, &failure) {
			next = now.Add(max(15*time.Minute, failure.RetryAfter))
		}
	} else if periodEnd := info.PeriodEnd; periodEnd != nil {
		if periodEnd.After(now.Add(time.Hour)) {
			if approaching := periodEnd.Add(-time.Hour); approaching.Before(next) {
				next = approaching
			}
		} else if periodEnd.After(now.Add(-time.Hour)) {
			next = now.Add(5 * time.Minute)
		}
	}
	return model.FinishUpstreamSubscriptionInfoRefresh(subscription, info, err == nil, next.Unix())
}
