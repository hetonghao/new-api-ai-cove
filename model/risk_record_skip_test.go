package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordRiskObservation_applies_governance_to_local_skip_result(t *testing.T) {
	tests := []struct {
		name      string
		saveScope RiskRecordSaveScope
		wantCount int64
	}{
		{name: "all saves", saveScope: RiskRecordSaveAll, wantCount: 1},
		{name: "suspicious omits", saveScope: RiskRecordSaveSuspicious},
		{name: "unsafe omits", saveScope: RiskRecordSaveUnsafe},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			_, err := SaveRiskRecordGovernance(context.Background(), RiskRecordGovernanceInput{
				SaveScope: test.saveScope, ContentSaveScope: RiskContentSaveAll, RetentionDays: 30,
			})
			require.NoError(t, err)
			input := validRiskRecordInput(RiskRecordResultNotReviewed)
			input.ProviderID = 0
			input.ProviderName = ""
			input.ProviderType = ""
			input.Categories = nil

			// When
			err = RecordRiskObservation(context.Background(), input)

			// Then
			require.NoError(t, err)
			records, total, err := ListRiskRecords(context.Background(), 0, 10)
			require.NoError(t, err)
			require.Equal(t, test.wantCount, total)
			if test.wantCount == 1 {
				require.Equal(t, []int{5, 8}, records[0].RuleIDs)
				require.Equal(t, RiskRecordSourceLocal, records[0].Source)
			}
		})
	}
}
