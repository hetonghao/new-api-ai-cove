package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

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
