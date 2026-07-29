package service

import (
	"context"
	"errors"
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

func riskProviderTestProvider(t *testing.T) *model.RiskProvider {
	t.Helper()
	ciphertext, err := common.EncryptCredential("cf-token")
	require.NoError(t, err)
	return &model.RiskProvider{
		ProviderType:        model.RiskProviderCloudflare,
		AccountID:           "0123456789abcdef0123456789abcdef",
		Model:               "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext,
		TimeoutMs:           800,
	}
}

func TestReviewRiskContentMapsCloudflareResponses(t *testing.T) {
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "risk-provider-test-key"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })

	tests := []struct {
		name        string
		response    string
		legacy      bool
		wantStatus  RiskReviewStatus
		categories  []string
		wantNeurons float64
	}{
		{name: "safe object", response: `{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":12}}}`, wantStatus: RiskReviewSafe, categories: []string{}, wantNeurons: 12},
		{name: "safe object with fractional neurons", response: `{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":9.072817475858999}}}`, wantStatus: RiskReviewSafe, categories: []string{}, wantNeurons: 9.072817475858999},
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

			provider := riskProviderTestProvider(t)
			if tt.legacy {
				provider.AccountID = ""
				provider.BaseURL = "https://legacy.example/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/run"
			}

			result, err := ReviewRiskContent(context.Background(), provider, "connection test")
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, result.Status)
			assert.Equal(t, tt.categories, result.Categories)
			if tt.wantNeurons != 0 {
				assert.InDelta(t, tt.wantNeurons, result.Usage.Neurons, 1e-12)
			}
		})
	}
}

func TestReviewRiskContent_returnsSafeProviderErrorDetails(t *testing.T) {
	// Given
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "risk-provider-error-test-key"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })
	provider := riskProviderTestProvider(t)
	privateText := strings.Repeat("private user text ", 80)
	unsafeValues := []string{provider.AccountID, "cf-token", privateText, "https://api.cloudflare.com/client/v4/accounts/" + provider.AccountID}
	transportCause := errors.New("dial " + unsafeValues[3] + " Authorization Bearer cf-token body=" + privateText)
	tests := []struct {
		name       string
		transport  riskProviderRoundTripFunc
		wantCode   string
		wantDetail string
		wantCause  error
	}{
		{
			name: "network failure",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, transportCause
			},
			wantCode: riskObservationProviderError, wantDetail: "Cloudflare network request failed", wantCause: transportCause,
		},
		{
			name: "HTTP failure",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(strings.Join(unsafeValues, " ")))}, nil
			},
			wantCode: riskObservationProviderError, wantDetail: "Cloudflare returned HTTP 429",
		},
		{
			name: "response parse failure",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"success":true,"result":` + privateText))}, nil
			},
			wantCode: riskObservationProviderError, wantDetail: "Cloudflare response could not be decoded",
		},
		{
			name: "deadline",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			},
			wantCode: riskObservationTimeout, wantDetail: "Cloudflare request timed out", wantCause: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalHTTPClient := httpClient
			httpClient = &http.Client{Transport: test.transport}
			t.Cleanup(func() { httpClient = originalHTTPClient })

			// When
			_, err := ReviewRiskContent(context.Background(), provider, privateText)
			code, detail := RiskObservationErrorInfo(err)

			// Then
			require.Error(t, err)
			if test.wantCause != nil {
				require.ErrorIs(t, err, test.wantCause)
			}
			assert.Equal(t, test.wantCode, code)
			assert.Equal(t, test.wantDetail, detail)
			assert.LessOrEqual(t, len([]rune(detail)), riskProviderErrorDetailMaxRunes)
			for _, unsafeValue := range unsafeValues {
				assert.NotContains(t, detail, unsafeValue)
			}
		})
	}
}
