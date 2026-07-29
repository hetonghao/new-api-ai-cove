package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type RiskReviewStatus string

const cloudflareWorkersAIBaseURL = "https://api.cloudflare.com/client/v4/accounts"
const riskProviderErrorDetailMaxRunes = 512
const platformInternalRiskPromptSemantics = "platform-internal-json-verdict-v1"

var platformInternalRiskHTTPClient = func() *http.Client {
	transport := newRelayHTTPTransport()
	transport.Proxy = nil
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}()

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

type platformInternalRiskRequest struct {
	Model       string                  `json:"model"`
	Messages    []cloudflareRiskMessage `json:"messages"`
	MaxTokens   int                     `json:"max_tokens"`
	Temperature int                     `json:"temperature"`
	Stream      bool                    `json:"stream"`
}

type platformInternalRiskResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage RiskReviewUsage `json:"usage"`
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

func reviewPlatformInternalRiskContent(ctx context.Context, provider *model.RiskProvider, content string) (RiskReviewResult, error) {
	token, err := validatePlatformInternalRiskRuntime(provider)
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Platform internal model configuration is invalid")
	}
	body, err := common.Marshal(platformInternalRiskRequest{
		Model: provider.Model,
		Messages: []cloudflareRiskMessage{
			{
				Role:    "system",
				Content: "Classify the user content for safety. Return only a JSON object with verdict set to safe or unsafe and categories set to a JSON string array. Do not explain the result. Policy semantics: " + platformInternalRiskPromptSemantics,
			},
			{Role: "user", Content: content},
		},
		MaxTokens: 64, Temperature: 0, Stream: false,
	})
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Platform internal model request could not be encoded")
	}
	requestURL, err := platformInternalRiskURL()
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Platform internal model loopback URL is invalid")
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(provider.TimeoutMs)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Platform internal model request could not be created")
	}
	request.Header.Set("Authorization", "Bearer sk-"+token.Key+"-"+strconv.Itoa(provider.ChannelID))
	request.Header.Set("Content-Type", "application/json")
	response, err := platformInternalRiskHTTPClient.Do(request)
	if err != nil {
		detail := "Platform internal model network request failed"
		if errors.Is(err, context.DeadlineExceeded) {
			detail = "Platform internal model request timed out"
		}
		return RiskReviewResult{}, newRiskProviderError(err, detail)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		cause := fmt.Errorf("platform internal risk provider returned HTTP %d", response.StatusCode)
		return RiskReviewResult{}, newRiskProviderError(cause, fmt.Sprintf("Platform internal model returned HTTP %d", response.StatusCode))
	}
	var decoded platformInternalRiskResponse
	if err := common.DecodeJson(io.LimitReader(response.Body, 1<<20), &decoded); err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Platform internal model response could not be decoded")
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return RiskReviewResult{}, newRiskProviderError(errors.New("empty internal moderation response"), "Platform internal model returned an invalid moderation verdict")
	}
	result, err := parsePlatformInternalRiskResult(decoded.Choices[0].Message.Content)
	if err != nil {
		return RiskReviewResult{}, newRiskProviderError(err, "Platform internal model returned an invalid moderation verdict")
	}
	result.Usage = decoded.Usage
	return result, nil
}

func validatePlatformInternalRiskRuntime(provider *model.RiskProvider) (*model.Token, error) {
	if provider.InternalTokenID < 1 || provider.ChannelID < 1 || strings.TrimSpace(provider.Model) == "" {
		return nil, errors.New("incomplete platform internal risk provider")
	}
	linked, err := model.IsPlatformInternalRiskTokenID(provider.InternalTokenID)
	if err != nil || !linked {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("internal risk token is not linked to a provider")
	}
	token, err := model.GetTokenById(provider.InternalTokenID)
	if err != nil {
		return nil, fmt.Errorf("get internal risk token: %w", err)
	}
	if !token.SystemManaged || token.Status != common.TokenStatusEnabled || !token.UnlimitedQuota ||
		!token.ModelLimitsEnabled || len(token.GetModelLimits()) != 1 || token.GetModelLimits()[0] != provider.Model {
		return nil, errors.New("internal risk token permissions are invalid")
	}
	allowedIPs := token.GetIpLimits()
	if len(allowedIPs) != 2 || !containsString(allowedIPs, "127.0.0.1/32") || !containsString(allowedIPs, "::1/128") {
		return nil, errors.New("internal risk token loopback restriction is invalid")
	}
	var root model.User
	if err := model.DB.Select("id").Where("role = ?", common.RoleRootUser).Order("id asc").First(&root).Error; err != nil {
		return nil, fmt.Errorf("resolve root user: %w", err)
	}
	if token.UserId != root.Id {
		return nil, errors.New("internal risk token does not belong to the root user")
	}
	channel, err := model.GetChannelById(provider.ChannelID, false)
	if err != nil {
		return nil, fmt.Errorf("get internal risk channel: %w", err)
	}
	if channel.Status != common.ChannelStatusEnabled {
		return nil, errors.New("internal risk channel is disabled")
	}
	return token, nil
}

func platformInternalRiskURL() (string, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("invalid server port")
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port) + "/v1/chat/completions", nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func parsePlatformInternalRiskResult(content string) (RiskReviewResult, error) {
	var fields map[string]json.RawMessage
	if err := common.Unmarshal([]byte(strings.TrimSpace(content)), &fields); err != nil || len(fields) != 2 {
		return RiskReviewResult{}, errors.New("invalid platform internal risk verdict")
	}
	verdictRaw, verdictOK := fields["verdict"]
	categoriesRaw, categoriesOK := fields["categories"]
	if !verdictOK || !categoriesOK {
		return RiskReviewResult{}, errors.New("invalid platform internal risk verdict")
	}
	var verdict string
	var categories []string
	if err := common.Unmarshal(verdictRaw, &verdict); err != nil {
		return RiskReviewResult{}, errors.New("invalid platform internal risk verdict")
	}
	if err := common.Unmarshal(categoriesRaw, &categories); err != nil || categories == nil {
		return RiskReviewResult{}, errors.New("invalid platform internal risk verdict")
	}
	for index, category := range categories {
		category = strings.TrimSpace(category)
		if category == "" {
			return RiskReviewResult{}, errors.New("invalid platform internal risk category")
		}
		categories[index] = category
	}
	switch verdict {
	case string(RiskReviewSafe):
		return RiskReviewResult{Status: RiskReviewSafe, Categories: categories}, nil
	case string(RiskReviewUnsafe):
		return RiskReviewResult{Status: RiskReviewUnsafe, Categories: categories}, nil
	default:
		return RiskReviewResult{}, errors.New("invalid platform internal risk verdict")
	}
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
