package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type riskProviderRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn riskProviderRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestReviewRiskContentMapsCloudflareResponses(t *testing.T) {
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "risk-provider-test-key"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })

	tests := []struct {
		name       string
		response   string
		legacy     bool
		wantStatus RiskReviewStatus
		categories []string
	}{
		{name: "safe object", response: `{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":12}}}`, wantStatus: RiskReviewSafe, categories: []string{}},
		{name: "unsafe text from legacy row", response: `{"success":true,"result":{"response":"unsafe\nS1,S9","usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}}`, legacy: true, wantStatus: RiskReviewUnsafe, categories: []string{"S1", "S9"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalHTTPClient := httpClient
			httpClient = &http.Client{Transport: riskProviderRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodPost, request.Method)
				assert.Equal(t, "Bearer cf-token", request.Header.Get("Authorization"))
				assert.Equal(t, "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/run/@cf/meta/llama-guard-3-8b", request.URL.String())
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tt.response)),
				}, nil
			})}
			t.Cleanup(func() { httpClient = originalHTTPClient })

			ciphertext, err := common.EncryptCredential("cf-token")
			require.NoError(t, err)
			provider := &model.RiskProvider{
				ProviderType:        model.RiskProviderCloudflare,
				AccountID:           "0123456789abcdef0123456789abcdef",
				Model:               "@cf/meta/llama-guard-3-8b",
				CredentialEncrypted: ciphertext,
				TimeoutMs:           800,
			}
			if tt.legacy {
				provider.AccountID = ""
				provider.BaseURL = "https://legacy.example/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/run"
			}

			result, err := ReviewRiskContent(context.Background(), provider, "connection test")
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, result.Status)
			assert.Equal(t, tt.categories, result.Categories)
			if tt.name == "safe object" {
				assert.Equal(t, int64(12), result.Usage.Neurons)
			}
		})
	}
}
