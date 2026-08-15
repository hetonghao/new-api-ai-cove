package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type openAIRiskRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openAIRiskResponse struct {
	Results []struct {
		Flagged    *bool            `json:"flagged"`
		Categories map[string]*bool `json:"categories"`
	} `json:"results"`
}

func reviewOpenAIRiskContent(ctx context.Context, provider *model.RiskProvider, content string) (RiskReviewResult, error) {
	credential, err := common.DecryptCredential(provider.CredentialEncrypted)
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Risk provider credential could not be decrypted")
	}
	body, err := common.Marshal(openAIRiskRequest{Model: provider.Model, Input: content})
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "OpenAI request could not be encoded")
	}
	requestURL, err := url.JoinPath(provider.BaseURL, "moderations")
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "OpenAI request URL could not be built")
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(provider.TimeoutMs)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "OpenAI request could not be created")
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")

	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		detail := "OpenAI network request failed"
		if errors.Is(err, context.DeadlineExceeded) {
			detail = "OpenAI request timed out"
		}
		return RiskReviewResult{}, newRiskProviderError(err, detail)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		cause := fmt.Errorf("OpenAI risk provider returned HTTP %d", response.StatusCode)
		if response.StatusCode == http.StatusTooManyRequests {
			cause = fmt.Errorf("%w: %w", ErrRiskProviderRateLimited, cause)
		}
		return RiskReviewResult{}, newRiskProviderError(cause, fmt.Sprintf("OpenAI returned HTTP %d", response.StatusCode))
	}

	var decoded openAIRiskResponse
	if err := common.DecodeJson(io.LimitReader(response.Body, 1<<20), &decoded); err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "OpenAI response could not be decoded")
	}
	if len(decoded.Results) == 0 || decoded.Results[0].Flagged == nil {
		return RiskReviewResult{}, newRiskProviderError(errors.New("invalid OpenAI moderation verdict"), "OpenAI returned an invalid moderation verdict")
	}
	status := RiskReviewSafe
	if *decoded.Results[0].Flagged {
		status = RiskReviewUnsafe
	}
	var categories []string
	if status == RiskReviewUnsafe {
		categories = make([]string, 0, len(decoded.Results[0].Categories))
		for category, applied := range decoded.Results[0].Categories {
			if applied != nil && *applied {
				categories = append(categories, category)
			}
		}
		sort.Strings(categories)
	}
	return RiskReviewResult{Status: status, Categories: categories}, nil
}
