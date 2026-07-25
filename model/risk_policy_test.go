package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRiskPolicy_returns_disabled_defaults_when_missing(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)

	// When
	state, err := GetRiskPolicyState()

	// Then
	require.NoError(t, err)
	require.False(t, state.Configured)
	require.False(t, state.Enabled)
	require.Nil(t, state.ProviderID)
	require.Empty(t, state.EnabledChannels)
	require.Equal(t, RiskReviewSelective, state.ReviewMode)
	require.Equal(t, RiskActionObserve, state.ActionMode)
}

func TestRiskPolicy_persists_first_enable_defaults(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "validated", ProviderType: RiskProviderCloudflare, Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	require.NoError(t, MarkRiskProviderValidated(provider.Id))
	providerID := provider.Id
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderID: &providerID, EnabledChannels: []RiskChannel{RiskChannelCPAPro}})
	require.NoError(t, err)

	// When
	state, err := GetRiskPolicyState()

	// Then
	require.NoError(t, err)
	require.True(t, state.Configured)
	require.True(t, state.Enabled)
	require.Equal(t, &providerID, state.ProviderID)
	require.Equal(t, []RiskChannel{RiskChannelCPAPro}, state.EnabledChannels)
	require.Equal(t, RiskReviewSelective, state.ReviewMode)
	require.Equal(t, RiskActionObserve, state.ActionMode)
}

func TestRiskPolicy_rejects_unvalidated_provider(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "draft", ProviderType: RiskProviderCloudflare, Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	providerID := provider.Id

	// When
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderID: &providerID, EnabledChannels: []RiskChannel{RiskChannelCPAPro}})

	// Then
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrRiskProviderNotValidated))
}

func TestRiskPolicy_rejects_unknown_channel(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)

	// When
	_, err := SaveRiskPolicy(RiskPolicyInput{EnabledChannels: []RiskChannel{"cpa-core"}})

	// Then
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidRiskPolicy))
}

func TestRiskPolicy_rejects_unknown_modes(t *testing.T) {
	tests := []struct {
		name  string
		input RiskPolicyInput
	}{
		{name: "review mode", input: RiskPolicyInput{ReviewMode: "sometimes"}},
		{name: "action mode", input: RiskPolicyInput{ActionMode: "warn"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskPolicyModelTest(t)

			// When
			_, err := SaveRiskPolicy(test.input)

			// Then
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrInvalidRiskPolicy))
		})
	}
}

func TestRiskPolicy_clears_provider_and_channels_when_disabled(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "validated", ProviderType: RiskProviderCloudflare, Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	require.NoError(t, MarkRiskProviderValidated(provider.Id))
	providerID := provider.Id
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderID: &providerID, EnabledChannels: []RiskChannel{RiskChannelCPAPro}})
	require.NoError(t, err)
	_, err = SaveRiskPolicy(RiskPolicyInput{})
	require.NoError(t, err)

	// When
	state, err := GetRiskPolicyState()

	// Then
	require.NoError(t, err)
	require.False(t, state.Enabled)
	require.Nil(t, state.ProviderID)
	require.Empty(t, state.EnabledChannels)
}
