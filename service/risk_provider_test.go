package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewRiskContentMapsCloudflareResponses(t *testing.T) {
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "risk-provider-test-key"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })

	tests := []struct {
		name       string
		response   string
		wantStatus RiskReviewStatus
		categories []string
	}{
		{name: "safe object", response: `{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":12}}}`, wantStatus: RiskReviewSafe, categories: []string{}},
		{name: "unsafe text", response: `{"success":true,"result":{"response":"unsafe\nS1,S9","usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}}`, wantStatus: RiskReviewUnsafe, categories: []string{"S1", "S9"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "Bearer cf-token", r.Header.Get("Authorization"))
				assert.Equal(t, "/@cf/meta/llama-guard-3-8b", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			ciphertext, err := common.EncryptCredential("cf-token")
			require.NoError(t, err)
			provider := &model.RiskProvider{
				ProviderType:        model.RiskProviderCloudflare,
				Model:               "@cf/meta/llama-guard-3-8b",
				BaseURL:             server.URL,
				CredentialEncrypted: ciphertext,
				TimeoutMs:           800,
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
