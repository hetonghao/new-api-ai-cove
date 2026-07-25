package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskRecordGovernance_defaultsToAllForThirtyDays(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)

	// When
	governance, err := GetRiskRecordGovernance(context.Background())

	// Then
	require.NoError(t, err)
	assert.Equal(t, RiskRecordSaveAll, governance.SaveScope)
	assert.Equal(t, 30, governance.RetentionDays)
}

func TestSaveRiskRecordGovernance_rejectsInvalidScopeAndRetention(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	tests := []RiskRecordGovernanceInput{
		{SaveScope: "sometimes", RetentionDays: 30},
		{SaveScope: RiskRecordSaveAll, RetentionDays: 0},
		{SaveScope: RiskRecordSaveAll, RetentionDays: 181},
	}

	for _, input := range tests {
		// When
		_, err := SaveRiskRecordGovernance(context.Background(), input)

		// Then
		require.ErrorIs(t, err, ErrInvalidRiskRecordGovernance)
	}
}

func TestRecordRiskObservation_appliesConfiguredSaveScopeAtPersistenceBoundary(t *testing.T) {
	tests := []struct {
		name      string
		scope     RiskRecordSaveScope
		input     RiskRecordInput
		wantSaved bool
	}{
		{name: "all saves local not reviewed", scope: RiskRecordSaveAll, input: func() RiskRecordInput {
			input := validRiskRecordInput(RiskRecordResultSafe)
			input.Result = RiskRecordResultNotReviewed
			input.ProviderID = 0
			input.ProviderName = ""
			input.Source = RiskRecordSourceLocal
			return input
		}(), wantSaved: true},
		{name: "all saves observed safe", scope: RiskRecordSaveAll, input: validRiskRecordInput(RiskRecordResultSafe), wantSaved: true},
		{name: "suspicious saves provider review", scope: RiskRecordSaveSuspicious, input: validRiskRecordInput(RiskRecordResultSafe), wantSaved: true},
		{name: "suspicious omits local not reviewed", scope: RiskRecordSaveSuspicious, input: func() RiskRecordInput {
			input := validRiskRecordInput(RiskRecordResultSafe)
			input.Result = RiskRecordResultNotReviewed
			input.ProviderID = 0
			input.ProviderName = ""
			input.Source = RiskRecordSourceLocal
			return input
		}(), wantSaved: false},
		{name: "unsafe omits observation-only unsafe", scope: RiskRecordSaveUnsafe, input: validRiskRecordInput(RiskRecordResultUnsafe), wantSaved: false},
		{name: "unsafe saves actually blocked request", scope: RiskRecordSaveUnsafe, input: func() RiskRecordInput {
			input := validRiskRecordInput(RiskRecordResultUnsafe)
			input.Blocked = true
			return input
		}(), wantSaved: true},
		{name: "unsafe always saves provider error", scope: RiskRecordSaveUnsafe, input: validRiskRecordInput(RiskRecordResultError), wantSaved: true},
		{name: "unsafe always saves pre-provider degradation", scope: RiskRecordSaveUnsafe, input: func() RiskRecordInput {
			input := validRiskRecordInput(RiskRecordResultError)
			input.ProviderID = 0
			input.ProviderName = ""
			input.ErrorCode = "queue_full"
			return input
		}(), wantSaved: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			_, err := SaveRiskRecordGovernance(context.Background(), RiskRecordGovernanceInput{
				SaveScope: test.scope, RetentionDays: 30,
			})
			require.NoError(t, err)

			// When
			err = RecordRiskObservation(context.Background(), test.input)

			// Then
			require.NoError(t, err)
			var count int64
			require.NoError(t, DB.Model(&RiskRecord{}).Count(&count).Error)
			if test.wantSaved {
				assert.EqualValues(t, 1, count)
			} else {
				assert.Zero(t, count)
			}
		})
	}
}
