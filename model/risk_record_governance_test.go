package model

import (
	"context"
	"strings"
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
	assert.Equal(t, RiskContentSaveAll, governance.ContentSaveScope)
	assert.Equal(t, 30, governance.RetentionDays)
	assert.Equal(t, 200, governance.PreviewChars)
	assert.Equal(t, 200, governance.SafePreviewChars)
	assert.Equal(t, 200, governance.NonSafePreviewChars)
}

func TestRecordRiskObservation_truncatesPreviewByReviewResult(t *testing.T) {
	for _, test := range []struct {
		name   string
		result RiskRecordResult
		want   int
	}{
		{name: "safe", result: RiskRecordResultSafe, want: 50},
		{name: "unsafe", result: RiskRecordResultUnsafe, want: 80},
		{name: "error", result: RiskRecordResultError, want: 80},
		{name: "not reviewed", result: RiskRecordResultNotReviewed, want: 80},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			_, err := SaveRiskRecordGovernance(context.Background(), RiskRecordGovernanceInput{
				SaveScope: RiskRecordSaveAll, ContentSaveScope: RiskContentSaveAll, RetentionDays: 30,
				SafePreviewChars: 50, NonSafePreviewChars: 80,
			})
			require.NoError(t, err)
			input := validRiskRecordInput(test.result)
			if test.result == RiskRecordResultNotReviewed {
				input.ProviderID = 0
				input.ProviderName = ""
				input.ProviderType = ""
				input.Source = RiskRecordSourceLocal
			}
			input.Preview = "中文🙂" + strings.Repeat("a", 100)

			// When
			require.NoError(t, RecordRiskObservation(context.Background(), input))

			// Then
			var record RiskRecord
			require.NoError(t, DB.Take(&record).Error)
			assert.Len(t, []rune(record.Preview), test.want)
		})
	}
}

func TestSaveRiskRecordGovernance_rejectsInvalidScopeAndRetention(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	tests := []RiskRecordGovernanceInput{
		{SaveScope: "sometimes", ContentSaveScope: RiskContentSaveAll, RetentionDays: 30},
		{SaveScope: RiskRecordSaveAll, ContentSaveScope: "sometimes", RetentionDays: 30},
		{SaveScope: RiskRecordSaveAll, ContentSaveScope: RiskContentSaveAll, RetentionDays: 0},
		{SaveScope: RiskRecordSaveAll, ContentSaveScope: RiskContentSaveAll, RetentionDays: 181},
		{SaveScope: RiskRecordSaveAll, ContentSaveScope: RiskContentSaveAll, RetentionDays: 30, SafePreviewChars: 49, NonSafePreviewChars: 50},
		{SaveScope: RiskRecordSaveAll, ContentSaveScope: RiskContentSaveAll, RetentionDays: 30, SafePreviewChars: 50, NonSafePreviewChars: 49},
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
			input.ProviderType = ""
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
			input.ProviderType = ""
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
			input.ProviderType = ""
			input.ErrorCode = "queue_full"
			return input
		}(), wantSaved: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			_, err := SaveRiskRecordGovernance(context.Background(), RiskRecordGovernanceInput{
				SaveScope: test.scope, ContentSaveScope: RiskContentSaveAll, RetentionDays: 30,
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

func TestRecordRiskObservation_appliesConfiguredContentSaveScope(t *testing.T) {
	tests := []struct {
		name        string
		scope       RiskContentSaveScope
		result      RiskRecordResult
		wantContent bool
	}{
		{name: "all keeps safe content", scope: RiskContentSaveAll, result: RiskRecordResultSafe, wantContent: true},
		{name: "unsafe omits safe content", scope: RiskContentSaveUnsafe, result: RiskRecordResultSafe, wantContent: false},
		{name: "unsafe keeps unsafe content", scope: RiskContentSaveUnsafe, result: RiskRecordResultUnsafe, wantContent: true},
		{name: "none omits unsafe content", scope: RiskContentSaveNone, result: RiskRecordResultUnsafe, wantContent: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			_, err := SaveRiskRecordGovernance(context.Background(), RiskRecordGovernanceInput{
				SaveScope: RiskRecordSaveAll, ContentSaveScope: test.scope, RetentionDays: 30,
			})
			require.NoError(t, err)
			input := validRiskRecordInput(test.result)
			input.Preview = "masked preview"
			input.ContentHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			if test.result == RiskRecordResultUnsafe {
				input.Source = RiskRecordSourceProvider
				input.ProviderCalled = true
				input.Chunks = []RiskRecordChunk{{
					Index: 0, Result: RiskRecordResultUnsafe, Summary: "masked chunk summary",
				}}
			}

			// When
			require.NoError(t, RecordRiskObservation(context.Background(), input))

			// Then
			var record RiskRecord
			require.NoError(t, DB.Take(&record).Error)
			if test.wantContent {
				assert.Equal(t, input.Preview, record.Preview)
				assert.Equal(t, input.ContentHash, record.ContentHash)
				if test.result == RiskRecordResultUnsafe {
					require.Len(t, record.Chunks, 1)
					assert.Equal(t, "masked chunk summary", record.Chunks[0].Summary)
				}
			} else {
				assert.Empty(t, record.Preview)
				assert.Empty(t, record.ContentHash)
				for _, chunk := range record.Chunks {
					assert.Empty(t, chunk.Summary)
				}
			}
		})
	}
}

func TestRecordRiskProviderValidation_appliesConfiguredContentSaveScope(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	_, err := SaveRiskRecordGovernance(context.Background(), RiskRecordGovernanceInput{
		SaveScope: RiskRecordSaveAll, ContentSaveScope: RiskContentSaveNone, RetentionDays: 30,
	})
	require.NoError(t, err)
	input := validRiskRecordInput(RiskRecordResultSafe)
	input.Preview = "masked preview"
	input.ContentHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// When
	require.NoError(t, RecordRiskProviderValidation(context.Background(), input))

	// Then
	var record RiskRecord
	require.NoError(t, DB.Take(&record).Error)
	assert.Empty(t, record.Preview)
	assert.Empty(t, record.ContentHash)
}
