package model

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordRiskObservation_persistsPreProviderErrorsWithoutProvider(t *testing.T) {
	errorCodes := []string{"queue_full", "service_shutdown", "policy_error", "rules_error"}
	for _, errorCode := range errorCodes {
		t.Run(errorCode, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			input := validRiskRecordInput(RiskRecordResultError)
			input.ProviderID = 0
			input.ProviderName = ""
			input.ProviderType = ""
			input.ErrorCode = errorCode

			// When
			err := RecordRiskObservation(context.Background(), input)

			// Then
			require.NoError(t, err)
			records, total, err := ListRiskRecords(context.Background(), 0, 1)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, records, 1)
			assert.Zero(t, records[0].ProviderID)
			assert.Empty(t, records[0].ProviderName)
			assert.Equal(t, errorCode, records[0].ErrorCode)
		})
	}
}

func TestRecordRiskObservation_truncatesErrorDetail(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultError)
	input.ErrorDetail = strings.Repeat("诊", riskRecordErrorDetailMaxRunes+50)

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, _, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Len(t, []rune(records[0].ErrorDetail), riskRecordErrorDetailMaxRunes)
}

func TestRecordRiskObservation_rejectsMissingProviderOutsidePreProviderErrors(t *testing.T) {
	tests := []struct {
		name      string
		result    RiskRecordResult
		errorCode string
	}{
		{name: "safe", result: RiskRecordResultSafe},
		{name: "unsafe", result: RiskRecordResultUnsafe},
		{name: "provider error", result: RiskRecordResultError, errorCode: "provider_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			input := validRiskRecordInput(test.result)
			input.ProviderID = 0
			input.ProviderName = ""
			input.ProviderType = ""
			input.ErrorCode = test.errorCode

			// When
			err := RecordRiskObservation(context.Background(), input)

			// Then
			require.ErrorIs(t, err, ErrInvalidRiskRecord)
		})
	}
}
