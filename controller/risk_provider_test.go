package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type riskProviderAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type riskProviderControllerRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn riskProviderControllerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type riskProviderTestCall struct {
	Method  string
	Target  string
	Body    any
	Id      int
	Handler gin.HandlerFunc
}

func setupRiskProviderControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	originalDB := model.DB
	originalSecret := common.CryptoSecret
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	common.CryptoSecret = "risk-provider-controller-key"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.RiskProvider{}, &model.RiskRecord{}, &model.RiskRecordGovernance{}))
	service.InitHttpClient()
	t.Cleanup(func() {
		model.DB = originalDB
		common.CryptoSecret = originalSecret
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})
	return db
}

func TestValidateRiskProvider_mapsDeadlineExceededToActionableMessage(t *testing.T) {
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
	client.Transport = riskProviderControllerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})
	t.Cleanup(func() { client.Transport = originalTransport })

	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/validate", Id: provider.Id, Handler: ValidateRiskProvider,
	})

	assert.False(t, response.Success)
	assert.Equal(t, "Risk provider connection timed out (current timeout: 800 ms). Increase the timeout and try again.", response.Message)
	assert.NotContains(t, response.Message, "api.cloudflare.com")
	assert.NotContains(t, response.Message, provider.AccountID)
	assert.NotContains(t, response.Message, "context deadline exceeded")
	var record model.RiskRecord
	require.NoError(t, db.Take(&record).Error)
	assert.Equal(t, provider.Id, record.ProviderID)
	assert.Equal(t, provider.Name, record.ProviderName)
	assert.Equal(t, model.RiskRecordResultError, record.Result)
	assert.Equal(t, model.RiskRecordSourceProvider, record.Source)
	assert.True(t, record.ProviderCalled)
	assert.Equal(t, "timeout", record.ErrorCode)
	assert.Equal(t, 0, record.ChannelID)
	assert.Equal(t, 1, record.UserID)
}

func callRiskProviderHandler(t *testing.T, call riskProviderTestCall) riskProviderAPIResponse {
	t.Helper()
	var payload []byte
	if call.Body != nil {
		var err error
		payload, err = common.Marshal(call.Body)
		require.NoError(t, err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(call.Method, call.Target, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	if call.Id > 0 {
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(call.Id)}}
	}
	call.Handler(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response riskProviderAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestRiskProviderAPIWorkflowKeepsCredentialsMasked(t *testing.T) {
	db := setupRiskProviderControllerTest(t)

	create := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPost, Target: "/api/risk/providers", Body: map[string]any{
		"name": "Cloudflare primary", "provider_type": "cloudflare", "model": "@cf/meta/llama-guard-3-8b",
		"account_id": "0123456789abcdef0123456789abcdef", "credential": "cf-secret-token",
	}, Handler: CreateRiskProvider})
	require.True(t, create.Success, create.Message)
	var created RiskProviderResponse
	require.NoError(t, common.Unmarshal(create.Data, &created))
	assert.True(t, created.HasCredential)
	assert.Nil(t, created.ValidatedAt)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", created.AccountID)
	assert.NotContains(t, string(create.Data), "cf-secret-token")
	assert.NotContains(t, string(create.Data), "base_url")

	var stored model.RiskProvider
	require.NoError(t, db.First(&stored, created.Id).Error)
	assert.NotEqual(t, "cf-secret-token", stored.CredentialEncrypted)
	assert.NotContains(t, stored.CredentialEncrypted, "cf-secret-token")

	update := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPut, Target: "/api/risk/providers/1", Body: map[string]any{
		"name": "Cloudflare edited", "provider_type": "cloudflare", "model": "@cf/meta/llama-guard-3-8b",
		"account_id": "0123456789abcdef0123456789abcdef", "credential": "", "timeout_ms": 900, "failure_threshold": 6, "cooldown_seconds": 31,
	}, Id: created.Id, Handler: UpdateRiskProvider})
	require.True(t, update.Success, update.Message)
	var updated RiskProviderResponse
	require.NoError(t, common.Unmarshal(update.Data, &updated))
	assert.Equal(t, "Cloudflare edited", updated.Name)
	assert.False(t, updated.Active)
	assert.Nil(t, updated.ValidatedAt)
	assert.True(t, updated.HasCredential)
	assert.NotContains(t, string(update.Data), "cf-secret-token")

	list := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodGet, Target: "/api/risk/providers/", Handler: ListRiskProviders})
	require.True(t, list.Success, list.Message)
	var providers []RiskProviderResponse
	require.NoError(t, common.Unmarshal(list.Data, &providers))
	require.Len(t, providers, 1)
	assert.Equal(t, updated, providers[0])
	assert.NotContains(t, string(list.Data), "cf-secret-token")

	deleted := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodDelete, Target: "/api/risk/providers/1", Id: created.Id, Handler: DeleteRiskProvider})
	require.True(t, deleted.Success, deleted.Message)
}

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

func TestUpdateRiskProviderRevokesValidationWhenConnectionChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(map[string]any)
	}{
		{
			name: "model changes",
			change: func(body map[string]any) {
				body["model"] = "@cf/meta/llama-guard-4-12b"
			},
		},
		{
			name: "account ID changes",
			change: func(body map[string]any) {
				body["account_id"] = "fedcba9876543210fedcba9876543210"
			},
		},
		{
			name: "new credential is supplied",
			change: func(body map[string]any) {
				body["credential"] = "replacement-secret"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupRiskProviderControllerTest(t)
			ciphertext, err := common.EncryptCredential("original-secret")
			require.NoError(t, err)
			provider := &model.RiskProvider{
				Name: "Cloudflare primary", ProviderType: model.RiskProviderCloudflare,
				AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b",
				CredentialEncrypted: ciphertext,
			}
			require.NoError(t, model.CreateRiskProvider(provider))
			require.NoError(t, model.MarkRiskProviderValidated(provider.Id))
			require.NoError(t, model.ActivateRiskProvider(provider.Id))

			body := map[string]any{
				"name": "Cloudflare primary", "provider_type": "cloudflare",
				"model": "@cf/meta/llama-guard-3-8b", "account_id": "0123456789abcdef0123456789abcdef",
				"credential": "", "timeout_ms": 800, "failure_threshold": 5, "cooldown_seconds": 30,
			}
			test.change(body)

			response := callRiskProviderHandler(t, riskProviderTestCall{
				Method: http.MethodPut, Target: "/api/risk/providers/1", Body: body,
				Id: provider.Id, Handler: UpdateRiskProvider,
			})
			require.True(t, response.Success, response.Message)
			var updated RiskProviderResponse
			require.NoError(t, common.Unmarshal(response.Data, &updated))
			assert.Nil(t, updated.ValidatedAt)
			assert.False(t, updated.Active)

			listed := callRiskProviderHandler(t, riskProviderTestCall{
				Method: http.MethodGet, Target: "/api/risk/providers/", Handler: ListRiskProviders,
			})
			require.True(t, listed.Success, listed.Message)
			var providers []RiskProviderResponse
			require.NoError(t, common.Unmarshal(listed.Data, &providers))
			require.Len(t, providers, 1)
			assert.Nil(t, providers[0].ValidatedAt)
			assert.False(t, providers[0].Active)
		})
	}
}

func TestCreateRiskProviderRejectsMissingOrInvalidAccountID(t *testing.T) {
	setupRiskProviderControllerTest(t)
	for _, accountID := range []string{"", "not-an-account-id"} {
		response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPost, Target: "/api/risk/providers", Body: map[string]any{
			"name": "Cloudflare", "provider_type": "cloudflare", "model": "@cf/meta/llama-guard-3-8b",
			"account_id": accountID, "credential": "cf-secret-token",
		}, Handler: CreateRiskProvider})
		assert.False(t, response.Success)
	}
}
