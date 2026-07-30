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
	require.Empty(t, state.ProviderIDs)
	require.Empty(t, state.EnabledChannels)
	require.Empty(t, state.ExcludedUserIDs)
	require.Empty(t, state.ExcludedModels)
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
	secondProvider := &RiskProvider{Name: "validated second", ProviderType: RiskProviderCloudflare, AccountID: "fedcba9876543210fedcba9876543210", Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(secondProvider))
	require.NoError(t, MarkRiskProviderValidated(secondProvider.Id))
	firstChannel := &Channel{Name: "Primary", Key: "secret", Models: "gpt-test"}
	secondChannel := &Channel{Name: "Backup", Key: "secret", Models: "gpt-test"}
	require.NoError(t, DB.Create(firstChannel).Error)
	require.NoError(t, DB.Create(secondChannel).Error)
	firstUser := &User{Username: "alice", AffCode: "risk-alice"}
	secondUser := &User{Username: "bob", AffCode: "risk-bob"}
	require.NoError(t, DB.Create(firstUser).Error)
	require.NoError(t, DB.Create(secondUser).Error)
	_, err := SaveRiskPolicy(RiskPolicyInput{
		ProviderIDs:     []int{secondProvider.Id, provider.Id, secondProvider.Id},
		EnabledChannels: []int{secondChannel.Id, firstChannel.Id, secondChannel.Id},
		ExcludedUserIDs: []int{secondUser.Id, firstUser.Id, secondUser.Id},
		ExcludedModels:  []string{" codex-auto-review ", "", "gpt-test", "codex-auto-review"},
	})
	require.NoError(t, err)

	// When
	state, err := GetRiskPolicyState()

	// Then
	require.NoError(t, err)
	require.True(t, state.Configured)
	require.True(t, state.Enabled)
	require.Equal(t, []int{secondProvider.Id, provider.Id}, state.ProviderIDs)
	require.Equal(t, []int{secondChannel.Id, firstChannel.Id}, state.EnabledChannels)
	require.Equal(t, []int{secondUser.Id, firstUser.Id}, state.ExcludedUserIDs)
	require.Equal(t, []string{"codex-auto-review", "gpt-test"}, state.ExcludedModels)
	require.Equal(t, RiskReviewSelective, state.ReviewMode)
	require.Equal(t, RiskActionObserve, state.ActionMode)
}

func TestRiskPolicyForRelay_readsProviderPoolWithoutProviderTableLookup(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	require.NoError(t, DB.Create(&RiskPolicy{
		Id:              riskPolicySingletonID,
		ProviderIDs:     `[7]`,
		EnabledChannels: `[24]`,
		ExcludedUserIDs: `[42]`,
		ExcludedModels:  `["codex-auto-review"]`,
		ReviewMode:      RiskReviewFull,
		ActionMode:      RiskActionBlock,
	}).Error)
	require.NoError(t, DB.Migrator().DropTable(&RiskProvider{}))

	for _, test := range []struct {
		name   string
		userID int
		model  string
	}{
		{name: "excluded user", userID: 42, model: "gpt-test"},
		{name: "excluded model", userID: 7, model: "codex-auto-review"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// When
			state, err := GetRiskPolicyStateForRelay(test.userID, test.model)

			// Then
			require.NoError(t, err)
			require.True(t, state.Configured)
			require.Equal(t, []int{42}, state.ExcludedUserIDs)
			require.Equal(t, []string{"codex-auto-review"}, state.ExcludedModels)
			require.Equal(t, []int{7}, state.ProviderIDs)
			require.False(t, state.Enabled)
		})
	}

	state, err := GetRiskPolicyStateForRelay(7, "codex-auto-review-upstream")
	require.NoError(t, err)
	require.Equal(t, []int{7}, state.ProviderIDs)
}

func TestRiskPolicy_rejects_unknown_excluded_user_id(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)

	// When
	_, err := SaveRiskPolicy(RiskPolicyInput{ExcludedUserIDs: []int{999}})

	// Then
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidRiskPolicy))
}

func TestRiskPolicy_rejects_unvalidated_provider(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "draft", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	channel := &Channel{Name: "Selected", Key: "secret", Models: "gpt-test"}
	require.NoError(t, DB.Create(channel).Error)

	// When
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderIDs: []int{provider.Id}, EnabledChannels: []int{channel.Id}})

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
	// When
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderIDs: []int{provider.Id}, EnabledChannels: []int{999}})

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

func TestRiskPolicy_keeps_provider_and_channels_when_disabled(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "validated", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", BaseURL: "https://example.com", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	require.NoError(t, MarkRiskProviderValidated(provider.Id))
	channel := &Channel{Name: "Selected", Key: "secret", Models: "gpt-test"}
	require.NoError(t, DB.Create(channel).Error)
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderIDs: []int{provider.Id}, EnabledChannels: []int{channel.Id}})
	require.NoError(t, err)
	disabled := false
	_, err = SaveRiskPolicy(RiskPolicyInput{Enabled: &disabled, ProviderIDs: []int{provider.Id}, EnabledChannels: []int{channel.Id}})
	require.NoError(t, err)

	// When
	state, err := GetRiskPolicyState()

	// Then
	require.NoError(t, err)
	require.False(t, state.Enabled)
	require.Equal(t, []int{provider.Id}, state.ProviderIDs)
	require.Equal(t, []int{channel.Id}, state.EnabledChannels)
}

func TestActivateRiskProvider_replacesPoolAndPreservesPolicy(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	first := &RiskProvider{Name: "first", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", CredentialEncrypted: "ciphertext"}
	second := &RiskProvider{Name: "second", ProviderType: RiskProviderCloudflare, AccountID: "fedcba9876543210fedcba9876543210", Model: "guard", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(first))
	require.NoError(t, CreateRiskProvider(second))
	require.NoError(t, MarkRiskProviderValidated(first.Id))
	require.NoError(t, MarkRiskProviderValidated(second.Id))
	channel := &Channel{Name: "Selected", Key: "secret", Models: "gpt-test"}
	require.NoError(t, DB.Create(channel).Error)
	_, err := SaveRiskPolicy(RiskPolicyInput{ProviderIDs: []int{first.Id, second.Id}, EnabledChannels: []int{channel.Id}})
	require.NoError(t, err)

	// When
	require.NoError(t, ActivateRiskProvider(second.Id))

	// Then
	state, err := GetRiskPolicyState()
	require.NoError(t, err)
	require.Equal(t, []int{second.Id}, state.ProviderIDs)
	require.Equal(t, []int{channel.Id}, state.EnabledChannels)
}

func TestDeleteRiskProvider_removesItFromProviderPool(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	first := &RiskProvider{Name: "first", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", CredentialEncrypted: "ciphertext"}
	second := &RiskProvider{Name: "second", ProviderType: RiskProviderCloudflare, AccountID: "fedcba9876543210fedcba9876543210", Model: "guard", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(first))
	require.NoError(t, CreateRiskProvider(second))
	require.NoError(t, MarkRiskProviderValidated(first.Id))
	require.NoError(t, MarkRiskProviderValidated(second.Id))
	disabled := false
	_, err := SaveRiskPolicy(RiskPolicyInput{Enabled: &disabled, ProviderIDs: []int{first.Id, second.Id}})
	require.NoError(t, err)

	// When
	require.NoError(t, DeleteRiskProvider(first.Id))

	// Then
	state, err := GetRiskPolicyState()
	require.NoError(t, err)
	require.Equal(t, []int{second.Id}, state.ProviderIDs)
}

func TestUpdateRiskProvider_removesInvalidatedProviderFromPool(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	provider := &RiskProvider{Name: "provider", ProviderType: RiskProviderCloudflare, AccountID: "0123456789abcdef0123456789abcdef", Model: "guard", CredentialEncrypted: "ciphertext"}
	require.NoError(t, CreateRiskProvider(provider))
	require.NoError(t, MarkRiskProviderValidated(provider.Id))
	disabled := false
	_, err := SaveRiskPolicy(RiskPolicyInput{Enabled: &disabled, ProviderIDs: []int{provider.Id}})
	require.NoError(t, err)
	require.NoError(t, DB.First(provider, provider.Id).Error)
	provider.Model = "guard-v2"
	provider.ValidatedAt = nil

	// When
	require.NoError(t, UpdateRiskProvider(provider))

	// Then
	state, err := GetRiskPolicyState()
	require.NoError(t, err)
	require.Empty(t, state.ProviderIDs)
}
