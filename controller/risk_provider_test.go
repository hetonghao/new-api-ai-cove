package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	require.NoError(t, db.AutoMigrate(&model.RiskProvider{}))
	service.InitHttpClient()
	t.Cleanup(func() {
		model.DB = originalDB
		common.CryptoSecret = originalSecret
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})
	return db
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
