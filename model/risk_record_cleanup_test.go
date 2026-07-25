package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteExpiredRiskRecordsBatch_deletesOnlyRowsStrictlyBeforeCutoffWithinLimit(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	cutoff := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	for index, observedAt := range []time.Time{
		cutoff.Add(-2 * time.Second),
		cutoff.Add(-time.Second),
		cutoff,
		cutoff.Add(time.Second),
	} {
		require.NoError(t, db.Create(&RiskRecord{RequestID: string(rune('a' + index)), ObservedAt: observedAt}).Error)
	}

	// When
	deleted, err := DeleteExpiredRiskRecordsBatch(context.Background(), cutoff, 1)

	// Then
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	var remaining []RiskRecord
	require.NoError(t, db.Order("observed_at asc").Find(&remaining).Error)
	require.Len(t, remaining, 3)
	assert.Equal(t, cutoff.Add(-time.Second), remaining[0].ObservedAt)
	assert.Equal(t, cutoff, remaining[1].ObservedAt)
	assert.Equal(t, cutoff.Add(time.Second), remaining[2].ObservedAt)
}

func TestDeleteExpiredRiskRecordsBatch_honorsCancellationAndInputBounds(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	_, canceledErr := DeleteExpiredRiskRecordsBatch(ctx, time.Now(), 500)
	_, cutoffErr := DeleteExpiredRiskRecordsBatch(context.Background(), time.Time{}, 500)
	_, limitErr := DeleteExpiredRiskRecordsBatch(context.Background(), time.Now(), 0)

	// Then
	require.ErrorIs(t, canceledErr, context.Canceled)
	require.ErrorIs(t, cutoffErr, ErrInvalidRiskRecordCleanup)
	require.ErrorIs(t, limitErr, ErrInvalidRiskRecordCleanup)
}
