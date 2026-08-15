package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
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

func TestValidateRiskProviderRecordsOpenAIModerationResultWithoutUsage(t *testing.T) {
	db := setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("openai-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "OpenAI moderation", ProviderType: model.RiskProviderOpenAI,
		Model: "omni-moderation-latest", BaseURL: "https://api.openai.com/v1",
		CredentialEncrypted: ciphertext, TimeoutMs: 800,
	}
	require.NoError(t, model.CreateRiskProvider(provider))
	client := service.GetHttpClient()
	require.NotNil(t, client)
	originalTransport := client.Transport
	client.Transport = riskProviderControllerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://api.openai.com/v1/moderations", request.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"results":[{"flagged":true,"categories":{"violence":true,"sexual":false}}]}`,
			)),
		}, nil
	})
	t.Cleanup(func() { client.Transport = originalTransport })

	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/validate", Id: provider.Id, Handler: ValidateRiskProvider,
	})

	require.True(t, response.Success, response.Message)
	var result service.RiskReviewResult
	require.NoError(t, common.Unmarshal(response.Data, &result))
	assert.Equal(t, service.RiskReviewUnsafe, result.Status)
	assert.Equal(t, []string{"violence"}, result.Categories)
	assert.Equal(t, service.RiskReviewUsage{}, result.Usage)
	var stored model.RiskProvider
	require.NoError(t, db.First(&stored, provider.Id).Error)
	assert.NotNil(t, stored.ValidatedAt)
	var record model.RiskRecord
	require.NoError(t, db.Take(&record).Error)
	assert.Equal(t, provider.Id, record.ProviderID)
	assert.Equal(t, model.RiskProviderOpenAI, record.ProviderType)
	assert.Equal(t, model.RiskRecordResultUnsafe, record.Result)
	assert.Equal(t, []string{"violence"}, record.Categories)
	assert.True(t, record.ProviderCalled)
	assert.Zero(t, record.PromptTokens)
	assert.Zero(t, record.CompletionTokens)
	assert.Zero(t, record.TotalTokens)
	assert.Zero(t, record.Neurons)
}

func TestOpenAIRiskProviderCanActivateOnlyAfterSafeValidation(t *testing.T) {
	db := setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("openai-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "OpenAI moderation", ProviderType: model.RiskProviderOpenAI,
		Model: "omni-moderation-latest", BaseURL: "https://api.openai.com/v1",
		CredentialEncrypted: ciphertext, TimeoutMs: 800,
	}
	require.NoError(t, model.CreateRiskProvider(provider))

	beforeValidation := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/active", Id: provider.Id, Handler: ActivateRiskProvider,
	})
	assert.False(t, beforeValidation.Success)

	client := service.GetHttpClient()
	require.NotNil(t, client)
	originalTransport := client.Transport
	client.Transport = riskProviderControllerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"results":[{"flagged":false,"categories":{}}]}`)),
		}, nil
	})
	t.Cleanup(func() { client.Transport = originalTransport })

	validation := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/validate", Id: provider.Id, Handler: ValidateRiskProvider,
	})
	require.True(t, validation.Success, validation.Message)
	var result service.RiskReviewResult
	require.NoError(t, common.Unmarshal(validation.Data, &result))
	assert.Equal(t, service.RiskReviewSafe, result.Status)

	afterValidation := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/active", Id: provider.Id, Handler: ActivateRiskProvider,
	})
	require.True(t, afterValidation.Success, afterValidation.Message)
	var stored model.RiskProvider
	require.NoError(t, db.First(&stored, provider.Id).Error)
	assert.True(t, stored.Active)
	assert.NotNil(t, stored.ValidatedAt)
}

func TestRiskProviderValidationRecordInputKeepsOpenAIErrorAndLatency(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Set("id", 1)
	provider := &model.RiskProvider{
		Id: 77, Name: "OpenAI moderation", ProviderType: model.RiskProviderOpenAI,
		Model: "omni-moderation-latest",
	}

	input := riskProviderValidationRecordInput(
		context,
		provider,
		service.RiskReviewResult{},
		service.ErrRiskProviderRateLimited,
		time.Now().Add(-1500*time.Millisecond),
	)

	assert.Equal(t, provider.Id, input.ProviderID)
	assert.Equal(t, provider.Name, input.ProviderName)
	assert.Equal(t, model.RiskProviderOpenAI, input.ProviderType)
	assert.Equal(t, model.RiskRecordResultError, input.Result)
	assert.Equal(t, "provider_error", input.ErrorCode)
	assert.GreaterOrEqual(t, input.LatencyMS, int64(1500))
	assert.True(t, input.ProviderCalled)
	assert.Equal(t, model.RiskRecordSourceProvider, input.Source)
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
