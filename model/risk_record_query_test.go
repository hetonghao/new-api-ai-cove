package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryRiskRecords_filtersByTimeChannelUserResultSourceAndProvider(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	baseTime := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	inputs := []RiskRecordInput{
		validRiskRecordInput(RiskRecordResultSafe),
		validRiskRecordInput(RiskRecordResultUnsafe),
		validRiskRecordInput(RiskRecordResultUnsafe),
	}
	inputs[0].RequestID = "req-before"
	inputs[0].ObservedAt = baseTime.Add(-time.Minute)
	inputs[1].RequestID = "req-match"
	inputs[1].ObservedAt = baseTime
	inputs[1].ChannelID = 88
	inputs[1].UserID = 99
	inputs[1].ProviderID = 77
	inputs[1].ProviderName = "Matched"
	inputs[1].Source = RiskRecordSourceInflight
	inputs[2].RequestID = "req-after"
	inputs[2].ObservedAt = baseTime.Add(time.Minute)
	for _, input := range inputs {
		require.NoError(t, RecordRiskObservation(context.Background(), input))
	}

	// When
	records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{
		Offset: 0, Limit: 20,
		StartTimestamp: baseTime.Unix(), EndTimestamp: baseTime.Unix(),
		ChannelID: 88, UserID: 99, ProviderID: 77,
		Result: RiskRecordResultUnsafe, Source: RiskRecordSourceInflight,
	})

	// Then
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "req-match", records[0].RequestID)
}

func TestQueryRiskRecords_rejectsInvalidFilters(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	tests := []RiskRecordQuery{
		{Offset: -1, Limit: 20},
		{Offset: 0, Limit: 101},
		{Offset: 0, Limit: 20, StartTimestamp: 20, EndTimestamp: 10},
		{Offset: 0, Limit: 20, ChannelID: -1},
		{Offset: 0, Limit: 20, Result: "maybe"},
		{Offset: 0, Limit: 20, Source: "remote"},
	}
	for _, query := range tests {
		// When
		_, _, err := QueryRiskRecords(context.Background(), query)

		// Then
		require.ErrorIs(t, err, ErrInvalidRiskRecordPage)
	}
}
