package controller

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestListRiskProvidersReportsUnusedQuotaWhenProviderIsDisabled(t *testing.T) {
	setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("cf-secret-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "Cloudflare disabled", ProviderType: model.RiskProviderCloudflare,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext, DailyNeuronsLimit: 10000,
	}
	require.NoError(t, model.CreateRiskProvider(provider))

	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodGet, Target: "/api/risk/providers/", Handler: ListRiskProviders,
	})
	require.True(t, response.Success, response.Message)
	var providers []RiskProviderResponse
	require.NoError(t, common.Unmarshal(response.Data, &providers))
	require.Len(t, providers, 1)
	assert.False(t, providers[0].Active)
	assert.Equal(t, int64(10000), providers[0].DailyNeuronsRemaining)
	assert.Equal(t, service.RiskProviderStatusNormal, providers[0].CurrentStatus)
}

func TestListRiskProvidersReportsDailyExhaustionWhenProviderIsDisabled(t *testing.T) {
	setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("cf-secret-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "Cloudflare exhausted", ProviderType: model.RiskProviderCloudflare,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext, DailyNeuronsLimit: 10000, DailyResetTime: "08:00",
	}
	require.NoError(t, model.CreateRiskProvider(provider))

	location := time.FixedZone("UTC+8", 8*60*60)
	now := time.Now().UTC().In(location)
	reset := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, location)
	if now.Before(reset) {
		reset = reset.Add(-24 * time.Hour)
	}
	key := fmt.Sprintf("new-api:risk-neurons-budget:v1:%d", provider.Id)
	require.NoError(t, common.RDB.HSet(context.Background(), key, map[string]any{
		"window":    reset.UTC().Format("20060102T150405Z"),
		"used":      "10000",
		"reserved":  "0",
		"exhausted": "1",
	}).Err())

	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodGet, Target: "/api/risk/providers/", Handler: ListRiskProviders,
	})
	require.True(t, response.Success, response.Message)
	var providers []RiskProviderResponse
	require.NoError(t, common.Unmarshal(response.Data, &providers))
	require.Len(t, providers, 1)
	assert.False(t, providers[0].Active)
	assert.Equal(t, service.RiskProviderStatusDailyExhausted, providers[0].CurrentStatus)
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
