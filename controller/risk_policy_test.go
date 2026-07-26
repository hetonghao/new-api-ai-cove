package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func setupRiskPolicyControllerTest(t *testing.T) {
	t.Helper()
	db := setupRiskProviderControllerTest(t)
	require.NoError(t, db.AutoMigrate(&model.RiskPolicy{}, &model.RiskRule{}, &model.Channel{}))
}

func TestRiskPolicyAPI_returns_disabled_defaults_when_missing(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodGet, Target: "/api/risk/policy", Handler: GetRiskPolicy})

	// Then
	require.True(t, response.Success, response.Message)
	var state model.RiskPolicyState
	require.NoError(t, common.Unmarshal(response.Data, &state))
	require.False(t, state.Configured)
	require.False(t, state.Enabled)
	require.Equal(t, model.RiskReviewSelective, state.ReviewMode)
	require.Equal(t, model.RiskActionObserve, state.ActionMode)
}

func TestRiskPolicyAPI_saves_first_enable_defaults(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)
	provider := &model.RiskProvider{Name: "validated", ProviderType: model.RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, model.CreateRiskProvider(provider))
	require.NoError(t, model.MarkRiskProviderValidated(provider.Id))
	channel := &model.Channel{Name: "CPA Pro", Key: "secret", Models: "gpt-test"}
	require.NoError(t, model.DB.Create(channel).Error)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPut, Target: "/api/risk/policy", Body: map[string]any{
		"provider_id": provider.Id, "enabled_channels": []int{channel.Id},
	}, Handler: UpdateRiskPolicy})

	// Then
	require.True(t, response.Success, response.Message)
	var state model.RiskPolicyState
	require.NoError(t, common.Unmarshal(response.Data, &state))
	require.True(t, state.Enabled)
	require.Equal(t, &provider.Id, state.ProviderID)
	require.Equal(t, []int{channel.Id}, state.EnabledChannels)
	require.Equal(t, model.RiskReviewSelective, state.ReviewMode)
	require.Equal(t, model.RiskActionObserve, state.ActionMode)
}

func TestRiskPolicyAPI_accepts_disabled_payload(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPut, Target: "/api/risk/policy", Body: map[string]any{
		"provider_id": nil, "enabled_channels": []string{},
	}, Handler: UpdateRiskPolicy})

	// Then
	require.True(t, response.Success, response.Message)
	var state model.RiskPolicyState
	require.NoError(t, common.Unmarshal(response.Data, &state))
	require.True(t, state.Configured)
	require.False(t, state.Enabled)
	require.Nil(t, state.ProviderID)
}
