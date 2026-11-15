package service

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type OpenDesignSubscriptionChallenge struct {
	DeviceID     string    `json:"deviceId"`
	DeviceSecret string    `json:"deviceSecret"`
	UserCode     string    `json:"userCode"`
	Activation   string    `json:"activationUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
	PollInterval int       `json:"pollIntervalSeconds"`
	Status       string    `json:"status"`
}

func validSubscriptionDeviceID(value string) bool {
	return value != "" && len(value) <= 128 && !strings.ContainsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	})
}

func StartOpenDesignSubscriptionLogin(ctx context.Context) (*OpenDesignSubscriptionChallenge, error) {
	var challenge OpenDesignSubscriptionChallenge
	if err := subscriptionJSONRequest(ctx, http.MethodPost, openDesignControlOrigin+"/api/v1/cli/device-authorizations", "", map[string]string{"cliVersion": common.Version, "host": "AI Cove", "profile": "prod"}, &challenge); err != nil {
		return nil, err
	}
	activation, err := url.Parse(challenge.Activation)
	if err != nil || activation.Scheme != "https" || activation.Host != "open-design.ai" || activation.User != nil || activation.Fragment != "" || activation.Path != "/cloud/cli/activate" {
		return nil, invalidSubscriptionQuota()
	}
	query := activation.Query()
	if !validSubscriptionDeviceID(challenge.DeviceID) || challenge.DeviceSecret == "" || len(challenge.DeviceSecret) > 4096 || challenge.UserCode == "" || len(challenge.UserCode) > 64 || challenge.Status != "pending" || challenge.PollInterval < 1 || challenge.PollInterval > 60 || !challenge.ExpiresAt.After(time.Now()) || len(query) != 2 || len(query["deviceId"]) != 1 || len(query["userCode"]) != 1 || query.Get("deviceId") != challenge.DeviceID || query.Get("userCode") != challenge.UserCode {
		return nil, invalidSubscriptionQuota()
	}
	if bound := time.Now().Add(15 * time.Minute); challenge.ExpiresAt.After(bound) {
		challenge.ExpiresAt = bound
	}
	return &challenge, nil
}

func PollOpenDesignSubscriptionLogin(ctx context.Context, deviceID, deviceSecret string) (string, string, error) {
	if !validSubscriptionDeviceID(deviceID) {
		return "", "", invalidSubscriptionQuota()
	}
	var response struct {
		Status     string `json:"status"`
		ControlKey string `json:"controlKey"`
	}
	if err := subscriptionJSONRequest(ctx, http.MethodPost, openDesignControlOrigin+"/api/v1/cli/device-authorizations/"+url.PathEscape(deviceID)+"/token", "", map[string]string{"deviceSecret": deviceSecret}, &response); err != nil {
		return "", "", err
	}
	switch response.Status {
	case "pending", "expired", "denied", "rejected", "cancelled", "canceled":
		return response.Status, "", nil
	case "approved":
		if ValidateSubscriptionCredential(model.SubscriptionPlatformOpenDesign, response.ControlKey) == nil {
			return response.Status, response.ControlKey, nil
		}
	}
	return "", "", invalidSubscriptionQuota()
}
