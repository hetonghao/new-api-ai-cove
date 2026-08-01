package controller

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRiskProvider_acceptsFractionalCloudflareNeurons(t *testing.T) {
	// Given
	db := setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("cf-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "Cloudflare", ProviderType: model.RiskProviderCloudflare,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext, TimeoutMs: 800,
	}
	require.NoError(t, model.CreateRiskProvider(provider))
	client := service.GetHttpClient()
	require.NotNil(t, client)
	originalTransport := client.Transport
	client.Transport = riskProviderControllerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/run/@cf/meta/llama-guard-3-8b", request.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":9.072817475858999}}}`,
			)),
		}, nil
	})
	t.Cleanup(func() { client.Transport = originalTransport })

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/validate", Id: provider.Id, Handler: ValidateRiskProvider,
	})

	// Then
	require.True(t, response.Success, response.Message)
	var result service.RiskReviewResult
	require.NoError(t, common.Unmarshal(response.Data, &result))
	assert.Equal(t, service.RiskReviewSafe, result.Status)
	assert.InDelta(t, 9.072817475858999, result.Usage.Neurons, 1e-12)
	var stored model.RiskProvider
	require.NoError(t, db.First(&stored, provider.Id).Error)
	assert.NotNil(t, stored.ValidatedAt)
	var record model.RiskRecord
	require.NoError(t, db.Take(&record).Error)
	assert.Equal(t, provider.Id, record.ProviderID)
	assert.Equal(t, provider.Name, record.ProviderName)
	assert.Equal(t, model.RiskRecordResultSafe, record.Result)
	assert.Equal(t, model.RiskRecordSourceProvider, record.Source)
	assert.True(t, record.ProviderCalled)
	assert.Empty(t, record.ErrorCode)
	assert.Equal(t, 3, record.PromptTokens)
	assert.Equal(t, 1, record.CompletionTokens)
	assert.Equal(t, 4, record.TotalTokens)
	assert.EqualValues(t, 9, record.Neurons)
}

func TestValidateRiskProvider_recordsExplicitDailyQuotaResponseAsProviderCall(t *testing.T) {
	// Given
	db := setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("cf-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Id: 901, Name: "Cloudflare quota", ProviderType: model.RiskProviderCloudflare,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext, TimeoutMs: 800,
	}
	require.NoError(t, model.CreateRiskProvider(provider))
	client := service.GetHttpClient()
	require.NotNil(t, client)
	originalTransport := client.Transport
	client.Transport = riskProviderControllerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":"daily neurons quota exceeded"}`)),
		}, nil
	})
	t.Cleanup(func() { client.Transport = originalTransport })

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/901/validate", Id: provider.Id, Handler: ValidateRiskProvider,
	})

	// Then
	assert.False(t, response.Success)
	var record model.RiskRecord
	require.NoError(t, db.Take(&record).Error)
	assert.Equal(t, provider.Id, record.ProviderID)
	assert.Equal(t, provider.Name, record.ProviderName)
	assert.Equal(t, provider.ProviderType, record.ProviderType)
	assert.Equal(t, model.RiskRecordResultError, record.Result)
	assert.Equal(t, model.RiskRecordSourceProvider, record.Source)
	assert.True(t, record.ProviderCalled)
	assert.Equal(t, "daily_neurons_exhausted", record.ErrorCode)
}

func TestValidateRiskProvider_usesCustomTestContentAndRecordsItsPreview(t *testing.T) {
	// Given
	db := setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("cf-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "Cloudflare", ProviderType: model.RiskProviderCloudflare,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext, TimeoutMs: 800,
	}
	require.NoError(t, model.CreateRiskProvider(provider))
	client := service.GetHttpClient()
	require.NotNil(t, client)
	originalTransport := client.Transport
	client.Transport = riskProviderControllerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		assert.Contains(t, string(body), "custom moderation content")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":9}}}`,
			)),
		}, nil
	})
	t.Cleanup(func() { client.Transport = originalTransport })

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/validate",
		Body: map[string]string{"text": "custom moderation content"}, Id: provider.Id, Handler: ValidateRiskProvider,
	})

	// Then
	require.True(t, response.Success, response.Message)
	var record model.RiskRecord
	require.NoError(t, db.Take(&record).Error)
	assert.Equal(t, "custom moderation content", record.Preview)
	assert.NotEmpty(t, record.ContentHash)
}

func TestValidateRiskProvider_rejectsBlankCustomTestContent(t *testing.T) {
	// Given
	setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("cf-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "Cloudflare", ProviderType: model.RiskProviderCloudflare,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext, TimeoutMs: 800,
	}
	require.NoError(t, model.CreateRiskProvider(provider))
	client := service.GetHttpClient()
	require.NotNil(t, client)
	originalTransport := client.Transport
	called := false
	client.Transport = riskProviderControllerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected upstream call")
	})
	t.Cleanup(func() { client.Transport = originalTransport })

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/validate",
		Body: map[string]string{"text": "   "}, Id: provider.Id, Handler: ValidateRiskProvider,
	})

	// Then
	assert.False(t, response.Success)
	assert.False(t, called)
}
