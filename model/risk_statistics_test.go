package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueryRiskStatisticsRanksAbsoluteCountsAndTracksSourceTrend(t *testing.T) {
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.Create(&User{Id: 34, Username: "alice", Password: "password", AffCode: "alice"}).Error)
	require.NoError(t, db.Create(&User{Id: 35, Username: "bob", Password: "password", AffCode: "bob"}).Error)
	require.NoError(t, db.Create(&Channel{Id: 12, Name: "Primary", Key: "primary"}).Error)
	require.NoError(t, db.Create(&Channel{Id: 13, Name: "Fallback", Key: "fallback"}).Error)

	base := time.Date(2026, time.July, 25, 0, 30, 0, 0, time.UTC)
	providerRecord := func(requestID string, userID, channelID int, result RiskRecordResult, observedAt time.Time) RiskRecordInput {
		return RiskRecordInput{
			RequestID: requestID, ChannelID: channelID, UserID: userID,
			ProviderID: 21, ProviderName: "Cloudflare", ProviderType: RiskProviderCloudflare,
			Result: result, Source: RiskRecordSourceProvider, ProviderCalled: true,
			Neurons: 4, LatencyMS: 100, ObservedAt: observedAt,
		}
	}
	inputs := []RiskRecordInput{
		providerRecord("alice-unsafe", 34, 12, RiskRecordResultUnsafe, base),
		providerRecord("alice-error", 34, 12, RiskRecordResultError, base.Add(10*time.Minute)),
		providerRecord("bob-safe", 35, 13, RiskRecordResultSafe, base.Add(time.Hour)),
		{
			RequestID: "bob-not-reviewed", ChannelID: 13, UserID: 35,
			Result: RiskRecordResultNotReviewed, Source: RiskRecordSourceLocal,
			ObservedAt: base.Add(2 * time.Hour),
		},
		{
			RequestID: "bob-cache-unsafe", ChannelID: 13, UserID: 35,
			Result: RiskRecordResultUnsafe, Source: RiskRecordSourceCache, CacheHit: true,
			LatencyMS: 20, ObservedAt: base.Add(3 * time.Hour),
		},
	}
	inputs[0].Blocked = true
	inputs[1].ErrorCode = "provider_error"
	inputs[1].ErrorDetail = "provider unavailable"
	inputs[1].Neurons = 0
	for _, input := range inputs {
		require.NoError(t, RecordRiskObservation(context.Background(), input))
	}

	statistics, err := QueryRiskStatistics(context.Background(), RiskStatisticsQuery{
		StartTimestamp: base.Add(-time.Hour).Unix(), EndTimestamp: base.Add(3*time.Hour + time.Minute).Unix(),
		Granularity: RiskStatisticsGranularityHour,
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), statistics.Summary.Records)
	require.Equal(t, int64(2), statistics.Summary.AffectedUsers)
	require.Equal(t, int64(2), statistics.Summary.Unsafe)
	require.Equal(t, int64(1), statistics.Summary.Errors)
	require.Equal(t, int64(1), statistics.Summary.Blocked)
	require.Equal(t, int64(1), statistics.Summary.CacheHits)
	require.Equal(t, int64(3), statistics.Summary.ProviderCalls)
	require.Equal(t, int64(8), statistics.Summary.Neurons)
	require.Equal(t, int64(100), statistics.Summary.P95LatencyMS)
	require.Len(t, statistics.Users, 2)
	require.Equal(t, 34, statistics.Users[0].UserID)
	require.Equal(t, int64(1), statistics.Users[0].Unsafe)
	require.Equal(t, int64(1), statistics.Users[0].Errors)
	require.Equal(t, 35, statistics.Users[1].UserID)
	require.Equal(t, int64(3), statistics.Users[1].Total)
	require.Len(t, statistics.Channels, 2)
	require.Equal(t, 12, statistics.Channels[0].ChannelID)
	require.Equal(t, int64(2), statistics.Channels[0].Total)
	require.Equal(t, int64(2), statistics.Channels[1].Total)
	require.Len(t, statistics.SourceTrend, 5)
	require.Equal(t, int64(2), statistics.SourceTrend[1].Provider)
	require.Equal(t, int64(1), statistics.SourceTrend[3].Local)
	require.Equal(t, int64(1), statistics.SourceTrend[4].Cache)
}

func TestQueryRiskStatisticsRejectsRangesLongerThanTwentyNineDays(t *testing.T) {
	setupRiskRecordModelTest(t)
	now := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	_, err := QueryRiskStatistics(context.Background(), RiskStatisticsQuery{
		StartTimestamp: now.Add(-30 * 24 * time.Hour).Unix(), EndTimestamp: now.Unix(),
		Granularity: RiskStatisticsGranularityDay,
	})
	require.ErrorIs(t, err, ErrInvalidRiskStatisticsQuery)
}

func TestQueryRiskStatisticsRejectsOverflowingTimestampRange(t *testing.T) {
	setupRiskRecordModelTest(t)
	maxTimestamp := int64(^uint64(0) >> 1)

	_, err := QueryRiskStatistics(context.Background(), RiskStatisticsQuery{
		StartTimestamp: 1, EndTimestamp: maxTimestamp, Granularity: RiskStatisticsGranularityHour,
	})

	require.ErrorIs(t, err, ErrInvalidRiskStatisticsQuery)
}
