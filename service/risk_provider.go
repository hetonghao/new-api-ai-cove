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

const (
	RiskReviewSafe   RiskReviewStatus = "safe"
	RiskReviewUnsafe RiskReviewStatus = "unsafe"
)

type RiskReviewUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type RiskReviewResult struct {
	Status     RiskReviewStatus `json:"status"`
	Categories []string         `json:"categories"`
	Usage      RiskReviewUsage  `json:"usage"`
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

func ReviewRiskContent(ctx context.Context, provider *model.RiskProvider, content string) (RiskReviewResult, error) {
	if provider == nil || provider.ProviderType != model.RiskProviderCloudflare {
		return RiskReviewResult{}, errors.New("unsupported risk provider")
	}

	credential, err := common.DecryptCredential(provider.CredentialEncrypted)
	if err != nil {
		return RiskReviewResult{}, fmt.Errorf("decrypt risk provider credential: %w", err)
	}
	body, err := common.Marshal(cloudflareRiskRequest{
		Messages:    []cloudflareRiskMessage{{Role: "user", Content: content}},
		MaxTokens:   16,
		Temperature: 0,
	})
	if err != nil {
		return RiskReviewResult{}, fmt.Errorf("encode Cloudflare risk request: %w", err)
	}

	requestURL, err := url.JoinPath(strings.TrimRight(provider.BaseURL, "/"), provider.Model)
	if err != nil {
		return RiskReviewResult{}, fmt.Errorf("build Cloudflare risk URL: %w", err)
	}
	timeout := time.Duration(provider.TimeoutMs) * time.Millisecond
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return RiskReviewResult{}, fmt.Errorf("create Cloudflare risk request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")

	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return RiskReviewResult{}, fmt.Errorf("call Cloudflare risk provider: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return RiskReviewResult{}, fmt.Errorf("Cloudflare risk provider returned HTTP %d", response.StatusCode)
	}

	var decoded cloudflareRiskResponse
	if err := common.DecodeJson(io.LimitReader(response.Body, 1<<20), &decoded); err != nil {
		return RiskReviewResult{}, fmt.Errorf("decode Cloudflare risk response: %w", err)
	}
	if !decoded.Success {
		return RiskReviewResult{}, errors.New("Cloudflare risk provider rejected the request")
	}

	result, err := parseCloudflareRiskResult(decoded.Result.Response)
	if err != nil {
		return RiskReviewResult{}, err
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
