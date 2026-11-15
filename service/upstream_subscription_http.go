package service

import (
	"bytes"
	"context"
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const openDesignControlOrigin = "https://amr-api.open-design.ai"

type subscriptionQuotaError struct {
	Code       string
	HTTPStatus int
	RetryAfter time.Duration
}

func (e *subscriptionQuotaError) Error() string {
	return "Subscription quota request failed: " + e.Code
}

func invalidSubscriptionQuota() error { return &subscriptionQuotaError{Code: "invalid_response"} }

func ValidateSubscriptionCredential(platform, credential string) error {
	if platform == model.SubscriptionPlatformCommandCode {
		if credential == "" || len(credential) > 4096 || strings.ContainsFunc(credential, func(r rune) bool { return r < '!' || r > '~' }) {
			return errors.New("Use a valid CommandCode API key")
		}
		return nil
	}
	prefix := "oc_sk_"
	if platform == model.SubscriptionPlatformOpenDesign {
		prefix = "vela_ctrl_"
	} else if platform != model.SubscriptionPlatformOpenCode {
		return errors.New("Unsupported subscription platform")
	}
	if !strings.HasPrefix(credential, prefix) || len(credential) <= len(prefix) || len(credential) > 4096 || strings.ContainsAny(credential, " \t\r\n") {
		return errors.New("Use the platform subscription credential, not an inference key")
	}
	return nil
}

// Requests never follow redirects or expose upstream bodies, which may contain credentials.
func subscriptionJSONRequest(ctx context.Context, method, target, credential string, payload, output any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := common.Marshal(payload)
		if err != nil {
			return invalidSubscriptionQuota()
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return invalidSubscriptionQuota()
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "AI-Cove/1.0")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	client := http.Client{}
	if shared := GetHttpClient(); shared != nil {
		client = *shared
	}
	client.Timeout = 15 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		code := "network_error"
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
			code = "timeout"
		}
		return &subscriptionQuotaError{Code: code}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		failure := &subscriptionQuotaError{Code: "upstream_error", HTTPStatus: response.StatusCode}
		switch response.StatusCode {
		case http.StatusUnauthorized:
			failure.Code = "unauthorized"
		case http.StatusForbidden:
			failure.Code = "forbidden"
		case http.StatusNotFound, http.StatusMethodNotAllowed:
			failure.Code = "endpoint_changed"
		case http.StatusTooManyRequests:
			failure.Code = "rate_limited"
			if seconds, parseErr := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 32); parseErr == nil && seconds > 0 {
				failure.RetryAfter = time.Duration(min(seconds, 86400)) * time.Second
			} else if at, parseErr := http.ParseTime(response.Header.Get("Retry-After")); parseErr == nil {
				failure.RetryAfter = min(max(time.Until(at), 0), 24*time.Hour)
			}
		}
		return failure
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(encoded) > 1<<20 || common.Unmarshal(encoded, output) != nil {
		return invalidSubscriptionQuota()
	}
	return nil
}
