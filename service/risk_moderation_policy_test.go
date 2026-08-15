package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func riskModerationProviderForTest() *model.RiskProvider {
	return &model.RiskProvider{
		Id: 7, ProviderType: model.RiskProviderCloudflare,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "guard-v1",
		TimeoutMs: 800, FailureThreshold: 2, CooldownSeconds: 30,
		UpdatedAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	}
}

func TestRiskModerationPolicyVersion_changesWithVerdictSemanticsOnly(t *testing.T) {
	// Given
	base := RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text",
		ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2048,
	}

	// When
	version, err := RiskModerationPolicyVersion(base)
	require.NoError(t, err)
	changedModel := base
	changedModel.Provider = riskModerationProviderForTest()
	changedModel.Provider.Model = "guard-v2"
	modelVersion, err := RiskModerationPolicyVersion(changedModel)
	require.NoError(t, err)
	changedAccount := base
	changedAccount.Provider = riskModerationProviderForTest()
	changedAccount.Provider.AccountID = "fedcba9876543210fedcba9876543210"
	accountVersion, err := RiskModerationPolicyVersion(changedAccount)
	require.NoError(t, err)
	changedIdentity := base
	changedIdentity.Provider = riskModerationProviderForTest()
	changedIdentity.Provider.Id = 8
	identityVersion, err := RiskModerationPolicyVersion(changedIdentity)
	require.NoError(t, err)
	changedChunks := base
	changedChunks.FullReviewChunkRunes = 1024
	chunkVersion, err := RiskModerationPolicyVersion(changedChunks)
	require.NoError(t, err)
	changedMode := base
	changedMode.ReviewMode = model.RiskReviewSelective
	modeVersion, err := RiskModerationPolicyVersion(changedMode)
	require.NoError(t, err)
	operationalChange := base
	operationalChange.Provider = riskModerationProviderForTest()
	operationalChange.Provider.TimeoutMs = 900
	operationalChange.Provider.FailureThreshold = 9
	operationalChange.Provider.CooldownSeconds = 90
	operationalChange.Provider.CredentialEncrypted = "rotated-secret"
	validatedAt := operationalChange.Provider.UpdatedAt.Add(-time.Hour)
	operationalChange.Provider.ValidatedAt = &validatedAt
	operationalChange.Provider.Active = true
	operationalChange.Provider.UpdatedAt = operationalChange.Provider.UpdatedAt.Add(time.Hour)
	operationalVersion, err := RiskModerationPolicyVersion(operationalChange)
	require.NoError(t, err)

	// Then
	assert.Equal(t, version, modelVersion)
	assert.Equal(t, version, accountVersion)
	assert.Equal(t, version, identityVersion)
	assert.NotEqual(t, version, chunkVersion)
	assert.NotEqual(t, version, modeVersion)
	assert.Equal(t, version, operationalVersion)
	assert.NotContains(t, version, base.Provider.Model)
	assert.NotContains(t, version, string(base.Provider.ProviderType))
}

func TestRiskModerationPolicyVersion_keepsOpenAICacheProviderIndependent(t *testing.T) {
	cloudflare := RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text",
		ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: 2048,
	}
	openAI := cloudflare
	openAI.Provider = riskModerationProviderForTest()
	openAI.Provider.ProviderType = model.RiskProviderOpenAI
	openAI.Provider.AccountID = ""
	openAI.Provider.Model = "omni-moderation-latest"
	openAI.Provider.BaseURL = "https://api.openai.com/v1"

	cloudflareVersion, cloudflareErr := RiskModerationPolicyVersion(cloudflare)
	openAIVersion, openAIErr := RiskModerationPolicyVersion(openAI)

	require.NoError(t, cloudflareErr)
	require.NoError(t, openAIErr)
	assert.Equal(t, cloudflareVersion, openAIVersion)
}

func TestRiskModerationPolicyVersion_usesCloudflareFullReviewChunkDefault(t *testing.T) {
	// Given
	defaultInput := RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text", ReviewMode: model.RiskReviewFull,
	}
	explicitInput := defaultInput
	explicitInput.FullReviewChunkRunes = RiskModerationCloudflareFullReviewChunkRunes

	// When
	defaultVersion, defaultErr := RiskModerationPolicyVersion(defaultInput)
	explicitVersion, explicitErr := RiskModerationPolicyVersion(explicitInput)

	// Then
	require.NoError(t, defaultErr)
	require.NoError(t, explicitErr)
	assert.Equal(t, explicitVersion, defaultVersion)
}

func TestRiskModerationPolicyVersion_rejectsNegativeFullReviewChunkOverride(t *testing.T) {
	// Given
	input := RiskModerationInput{
		Provider: riskModerationProviderForTest(), Content: "text",
		ReviewMode: model.RiskReviewFull, FullReviewChunkRunes: -1,
	}

	// When
	_, err := RiskModerationPolicyVersion(input)

	// Then
	require.ErrorIs(t, err, ErrInvalidRiskModerationInput)
}
