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

func TestRiskPolicy_treats_legacy_named_channels_as_empty_selection(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	require.NoError(t, DB.Create(&RiskPolicy{
		Id: riskPolicySingletonID, EnabledChannels: `["cpa-pro"]`,
		ReviewMode: RiskReviewSelective, ActionMode: RiskActionObserve,
	}).Error)

	// When
	state, err := GetRiskPolicyState()

	// Then
	require.NoError(t, err)
	require.True(t, state.Configured)
	require.False(t, state.Enabled)
	require.Empty(t, state.EnabledChannels)
}

func TestRiskPolicy_persists_first_enable_defaults(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "validated", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	require.NoError(t, MarkRiskProviderValidated(provider.Id))
	providerID := provider.Id
	firstChannel := &Channel{Name: "Primary", Key: "secret", Models: "gpt-test"}
	secondChannel := &Channel{Name: "Backup", Key: "secret", Models: "gpt-test"}
	require.NoError(t, DB.Create(firstChannel).Error)
	require.NoError(t, DB.Create(secondChannel).Error)
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderID: &providerID, EnabledChannels: []int{secondChannel.Id, firstChannel.Id, secondChannel.Id}})
	require.NoError(t, err)

	// When
	state, err := GetRiskPolicyState()

	// Then
	require.NoError(t, err)
	require.True(t, state.Configured)
	require.True(t, state.Enabled)
	require.Equal(t, &providerID, state.ProviderID)
	require.Equal(t, []int{secondChannel.Id, firstChannel.Id}, state.EnabledChannels)
	require.Equal(t, RiskReviewSelective, state.ReviewMode)
	require.Equal(t, RiskActionObserve, state.ActionMode)
}

func TestRiskPolicy_rejects_unvalidated_provider(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "draft", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	providerID := provider.Id
	channel := &Channel{Name: "Selected", Key: "secret", Models: "gpt-test"}
	require.NoError(t, DB.Create(channel).Error)

	// When
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderID: &providerID, EnabledChannels: []int{channel.Id}})

	// Then
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrRiskProviderNotValidated))
}

func TestRiskPolicy_rejects_unknown_channel_id(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "validated", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	require.NoError(t, MarkRiskProviderValidated(provider.Id))
	providerID := provider.Id

	// When
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderID: &providerID, EnabledChannels: []int{999}})

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
	provider := &RiskProvider{Name: "validated", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	require.NoError(t, MarkRiskProviderValidated(provider.Id))
	providerID := provider.Id
	channel := &Channel{Name: "Selected", Key: "secret", Models: "gpt-test"}
	require.NoError(t, DB.Create(channel).Error)
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderID: &providerID, EnabledChannels: []int{channel.Id}})
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
