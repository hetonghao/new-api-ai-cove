package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type RiskReviewStatus string

const cloudflareWorkersAIBaseURL = "https://api.cloudflare.com/client/v4/accounts"
const riskProviderErrorDetailMaxRunes = 512

const (
	RiskReviewSafe   RiskReviewStatus = "safe"
	RiskReviewUnsafe RiskReviewStatus = "unsafe"
)

type RiskReviewUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Neurons          float64 `json:"neurons"`
}

type RiskReviewResult struct {
	Status       RiskReviewStatus       `json:"status"`
	Categories   []string               `json:"categories"`
	Usage        RiskReviewUsage        `json:"usage"`
	ProviderID   int                    `json:"provider_id,omitempty"`
	ProviderName string                 `json:"provider_name,omitempty"`
	ProviderType model.RiskProviderType `json:"provider_type,omitempty"`
}

type cloudflareRiskResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Response json.RawMessage `json:"response"`
		Usage    RiskReviewUsage `json:"usage"`
	} `json:"result"`
}

type cloudflareRiskRequest struct {
	Messages    []cloudflareRiskMessage `json:"messages"`
	MaxTokens   int                     `json:"max_tokens"`
	Temperature int                     `json:"temperature"`
}

type cloudflareRiskMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type riskProviderError struct {
	cause  error
	detail string
}

func (e *riskProviderError) Error() string {
	return e.detail
}

func (e *riskProviderError) Unwrap() error {
	return e.cause
}

func newRiskProviderError(cause error, detail string) error {
	detail = strings.Join(strings.Fields(detail), " ")
	runes := []rune(detail)
	if len(runes) > riskProviderErrorDetailMaxRunes {
		detail = string(runes[:riskProviderErrorDetailMaxRunes])
	}
	return &riskProviderError{cause: cause, detail: detail}
}

func RiskObservationErrorInfo(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	code := riskObservationProviderError
	detail := "Risk provider request failed"
	if errors.Is(err, context.DeadlineExceeded) {
		code = riskObservationTimeout
		detail = "Cloudflare request timed out"
	} else if errors.Is(err, ErrRiskModerationCircuitOpen) {
		code = riskObservationCircuitOpen
		detail = "Risk moderation circuit is open; provider was not called"
	}
	var providerErr *riskProviderError
	if errors.As(err, &providerErr) && providerErr.detail != "" {
		detail = providerErr.detail
	}
	return code, detail
}

func ReviewRiskContent(ctx context.Context, provider *model.RiskProvider, content string) (RiskReviewResult, error) {
	if provider == nil {
		cause := errors.New("unsupported risk provider")
		return RiskReviewResult{}, newRiskProviderError(cause, "Risk provider configuration is unsupported")
	}
	switch provider.ProviderType {
	case model.RiskProviderCloudflare:
		return reviewCloudflareRiskContent(ctx, provider, content)
	case model.RiskProviderPlatformInternal:
		return reviewPlatformInternalRiskContent(ctx, provider, content)
	default:
		cause := errors.New("unsupported risk provider")
		return RiskReviewResult{}, newRiskProviderError(cause, "Risk provider configuration is unsupported")
	}
}

func reviewCloudflareRiskContent(ctx context.Context, provider *model.RiskProvider, content string) (RiskReviewResult, error) {
	credential, err := common.DecryptCredential(provider.CredentialEncrypted)
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Risk provider credential could not be decrypted")
	}
	body, err := common.Marshal(cloudflareRiskRequest{
		Messages:    []cloudflareRiskMessage{{Role: "user", Content: content}},
		MaxTokens:   16,
		Temperature: 0,
	})
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Cloudflare request could not be encoded")
	}

	accountID, err := provider.CloudflareAccountID()
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Cloudflare account configuration is invalid")
	}
	requestURL, err := url.JoinPath(cloudflareWorkersAIBaseURL, accountID, "ai", "run", provider.Model)
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Cloudflare request URL could not be built")
	}
	timeout := time.Duration(provider.TimeoutMs) * time.Millisecond
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Cloudflare request could not be created")
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")

	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		detail := "Cloudflare network request failed"
		if errors.Is(err, context.DeadlineExceeded) {
			detail = "Cloudflare request timed out"
		}
		return RiskReviewResult{}, newRiskProviderError(err, detail)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		cause := fmt.Errorf("Cloudflare risk provider returned HTTP %d", response.StatusCode)
		return RiskReviewResult{}, newRiskProviderError(cause, fmt.Sprintf("Cloudflare returned HTTP %d", response.StatusCode))
	}

	var decoded cloudflareRiskResponse
	if err := common.DecodeJson(io.LimitReader(response.Body, 1<<20), &decoded); err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Cloudflare response could not be decoded")
	}
	if !decoded.Success {
		cause := errors.New("Cloudflare risk provider rejected the request")
		return RiskReviewResult{}, newRiskProviderError(cause, "Cloudflare rejected the moderation request")
	}

	result, err := parseCloudflareRiskResult(decoded.Result.Response)
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Cloudflare returned an invalid moderation verdict")
	}
	result.Usage = decoded.Result.Usage
	return result, nil
}

func parseCloudflareRiskResult(raw json.RawMessage) (RiskReviewResult, error) {
	if common.GetJsonType(raw) == "object" {
		var response struct {
			Safe       *bool    `json:"safe"`
			Categories []string `json:"categories"`
		}
		if err := common.Unmarshal(raw, &response); err != nil {
			return RiskReviewResult{}, fmt.Errorf("decode Cloudflare risk verdict: %w", err)
		}
		if response.Safe == nil {
			return RiskReviewResult{}, errors.New("invalid Cloudflare risk verdict")
		}
		status := RiskReviewUnsafe
		if *response.Safe {
			status = RiskReviewSafe
		}
		return RiskReviewResult{Status: status, Categories: response.Categories}, nil
	}

	var text string
	if err := common.Unmarshal(raw, &text); err != nil {
		return RiskReviewResult{}, errors.New("invalid Cloudflare risk verdict")
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	switch strings.ToLower(strings.TrimSpace(lines[0])) {
	case string(RiskReviewSafe):
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	case string(RiskReviewUnsafe):
		var categories []string
		if len(lines) > 1 {
			for _, category := range strings.Split(lines[1], ",") {
				if category = strings.TrimSpace(category); category != "" {
					categories = append(categories, category)
				}
			}
		}
		return RiskReviewResult{Status: RiskReviewUnsafe, Categories: categories}, nil
	default:
		return RiskReviewResult{}, errors.New("invalid Cloudflare risk verdict")
	}
}
