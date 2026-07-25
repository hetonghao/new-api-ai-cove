package model

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordRiskObservation_persistsGovernedMetadataWithinPrivacyBounds(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultUnsafe)
	input.TokenID = 55
	input.Model = "gpt-5.6"
	input.Path = "/v1/responses"
	input.Preview = strings.Repeat("隐", 205)
	input.ContentHash = strings.Repeat("a", 64)
	input.Source = RiskRecordSourceCache
	input.CacheHit = true
	input.ProviderCalled = false
	input.Blocked = false

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, total, err := ListRiskRecords(context.Background(), 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	record := records[0]
	assert.Equal(t, 55, record.TokenID)
	assert.Equal(t, "gpt-5.6", record.Model)
	assert.Equal(t, "/v1/responses", record.Path)
	assert.Len(t, []rune(record.Preview), 200)
	assert.Equal(t, strings.Repeat("a", 64), record.ContentHash)
	assert.Equal(t, RiskRecordSourceCache, record.Source)
	assert.True(t, record.CacheHit)
	assert.False(t, record.ProviderCalled)
	assert.False(t, record.Blocked)
}

func TestRecordRiskObservation_persistsActualProviderCallAudit(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultSafe)
	input.Source = RiskRecordSourceProvider
	input.ProviderCalled = true

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, _, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.True(t, records[0].ProviderCalled)
}

func TestRecordRiskObservation_rejectsInconsistentGovernanceMetadata(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	tests := []struct {
		name   string
		mutate func(*RiskRecordInput)
	}{
		{name: "cache source without hit", mutate: func(input *RiskRecordInput) { input.Source = RiskRecordSourceCache }},
		{name: "provider source marked cache hit", mutate: func(input *RiskRecordInput) {
			input.Source = RiskRecordSourceProvider
			input.CacheHit = true
		}},
		{name: "inflight source marked cache hit", mutate: func(input *RiskRecordInput) {
			input.Source = RiskRecordSourceInflight
			input.CacheHit = true
		}},
		{name: "local source marked provider called", mutate: func(input *RiskRecordInput) {
			input.ProviderID = 0
			input.ProviderName = ""
			input.Result = RiskRecordResultNotReviewed
			input.Source = RiskRecordSourceLocal
			input.ProviderCalled = true
		}},
		{name: "cache source marked provider called", mutate: func(input *RiskRecordInput) {
			input.Source = RiskRecordSourceCache
			input.CacheHit = true
			input.ProviderCalled = true
		}},
		{name: "inflight source marked provider called", mutate: func(input *RiskRecordInput) {
			input.Source = RiskRecordSourceInflight
			input.ProviderCalled = true
		}},
		{name: "unknown source", mutate: func(input *RiskRecordInput) { input.Source = "remote" }},
		{name: "safe result marked blocked", mutate: func(input *RiskRecordInput) { input.Blocked = true }},
		{name: "path includes query", mutate: func(input *RiskRecordInput) { input.Path = "/v1/responses?token=secret" }},
		{name: "invalid content hash", mutate: func(input *RiskRecordInput) { input.ContentHash = "plaintext" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRiskRecordInput(RiskRecordResultSafe)
			test.mutate(&input)

			// When
			err := RecordRiskObservation(context.Background(), input)

			// Then
			require.ErrorIs(t, err, ErrInvalidRiskRecord)
		})
	}
}

func TestRecordRiskObservation_derivesProviderSourceForLegacyProviderEvent(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultSafe)
	require.Empty(t, input.Source)

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, _, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, RiskRecordSourceProvider, records[0].Source)
	assert.False(t, records[0].CacheHit)
}

func TestRecordRiskObservation_derivesLocalSourceForLegacyPreProviderError(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultError)
	input.ProviderID = 0
	input.ProviderName = ""
	input.ErrorCode = "queue_full"

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, _, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, RiskRecordSourceLocal, records[0].Source)
}

func TestRecordRiskObservation_recordsProviderConfigFailureAsLocalWithoutProvider(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultError)
	input.ProviderID = 0
	input.ProviderName = ""
	input.ErrorCode = "provider_config_error"
	input.Source = RiskRecordSourceLocal

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, _, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, RiskRecordSourceLocal, records[0].Source)
	assert.Zero(t, records[0].ProviderID)
	assert.Empty(t, records[0].ProviderName)
	assert.False(t, records[0].ProviderCalled)
}
