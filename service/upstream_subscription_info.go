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

func fetchSubscriptionPeriodEnd(ctx context.Context, subscription *model.UpstreamSubscription, credential string) (*time.Time, error) {
	if subscription.Platform == model.SubscriptionPlatformOpenCode {
		var response struct {
			Access json.RawMessage `json:"access"`
		}
		if err := subscriptionJSONRequest(ctx, http.MethodGet, "https://opencode.ai/console/api/go/status", credential, nil, &response); err != nil {
			return nil, err
		}
		if len(response.Access) == 0 {
			return nil, invalidSubscriptionQuota()
		}
		if strings.TrimSpace(string(response.Access)) == "null" {
			return nil, nil
		}
		var access struct {
			EndsAt *time.Time `json:"endsAt"`
		}
		if common.Unmarshal(response.Access, &access) != nil || access.EndsAt == nil || access.EndsAt.IsZero() {
			return nil, invalidSubscriptionQuota()
		}
		return access.EndsAt, nil
	}
	if subscription.Platform != model.SubscriptionPlatformOpenDesign {
		return nil, invalidSubscriptionQuota()
	}
	var workspace struct {
		ID   string `json:"workspaceId"`
		Type string `json:"workspaceType"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, openDesignControlOrigin+"/api/v1/workspaces/current", credential, nil, &workspace); err != nil {
		return nil, err
	}
	var quota model.SubscriptionQuota
	if !validSubscriptionDeviceID(workspace.ID) || (subscription.QuotaJSON != "" && (common.UnmarshalJsonStr(subscription.QuotaJSON, &quota) != nil || (quota.WorkspaceID != "" && quota.WorkspaceID != workspace.ID))) {
		return nil, invalidSubscriptionQuota()
	}
	if workspace.Type == "team" {
		return nil, nil
	}
	if workspace.Type != "personal" {
		return nil, invalidSubscriptionQuota()
	}
	var summary struct {
		PeriodEnd json.RawMessage `json:"subscriptionCurrentPeriodEnd"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodGet, openDesignControlOrigin+"/api/v1/billing/summary", credential, nil, &summary); err != nil {
		return nil, err
	}
	if len(summary.PeriodEnd) == 0 {
		return nil, invalidSubscriptionQuota()
	}
	var periodEnd *time.Time
	if common.Unmarshal(summary.PeriodEnd, &periodEnd) != nil || (periodEnd != nil && periodEnd.IsZero()) {
		return nil, invalidSubscriptionQuota()
	}
	return periodEnd, nil
}

func refreshUpstreamSubscriptionInfo(ctx context.Context, id int) error {
	now := time.Now()
	subscription, claimed, err := model.ClaimUpstreamSubscriptionInfoRefresh(id, now.Unix())
	if err != nil || !claimed {
		return err
	}
	credential, err := common.DecryptCredential(subscription.CredentialEncrypted)
	var periodEnd *time.Time
	if err == nil {
		infoContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		periodEnd, err = fetchSubscriptionPeriodEnd(infoContext, subscription, credential)
	}
	next := now.Add(6 * time.Hour)
	if err != nil {
		next = now.Add(15 * time.Minute)
		var failure *subscriptionQuotaError
		if errors.As(err, &failure) {
			next = now.Add(max(15*time.Minute, failure.RetryAfter))
		}
	} else if periodEnd != nil {
		if periodEnd.After(now.Add(time.Hour)) {
			if approaching := periodEnd.Add(-time.Hour); approaching.Before(next) {
				next = approaching
			}
		} else if periodEnd.After(now.Add(-time.Hour)) {
			next = now.Add(5 * time.Minute)
		}
	}
	return model.FinishUpstreamSubscriptionInfoRefresh(subscription, periodEnd, err == nil, next.Unix())
}
