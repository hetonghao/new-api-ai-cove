package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelQualityConfigPreservesPromptAndExplicitZero(t *testing.T) {
	zero := 0.0
	cfg := QualityConfig{Model: "gpt-6-astra", OutputType: "svg", Mode: "channel", Protocol: "responses", TokenID: 1, Group: "default", ChannelIDs: []int{1}, Prompt: " Generate an SVG image of a pelican riding a bicycle by the seaside.\n", MaxOutputTokens: 16384, SamplesPerTarget: 3, TimeoutSeconds: 180, DailyLimit: 200}
	body, first, err := QualityConfigFingerprint(cfg)
	require.NoError(t, err)
	assert.Contains(t, body, `"prompt":" Generate an SVG image of a pelican riding a bicycle by the seaside.\n"`)
	assert.NotContains(t, body, `"temperature"`)
	cfg.Temperature = &zero
	body, second, err := QualityConfigFingerprint(cfg)
	require.NoError(t, err)
	assert.Contains(t, body, `"temperature":0`)
	assert.NotEqual(t, first, second)
	cfg.SamplesPerTarget = 11
	assert.Error(t, ValidateQualityConfig(cfg))
	cfg.SamplesPerTarget = 3
	cfg.Mode = "route"
	assert.Error(t, ValidateQualityConfig(cfg))
	cfg.ChannelIDs = nil
	assert.NoError(t, ValidateQualityConfig(cfg))
}

func TestModelQualityScheduleBoundaries(t *testing.T) {
	utc := func(value string) time.Time {
		result, err := time.Parse(time.RFC3339, value)
		require.NoError(t, err)
		return result
	}
	for _, tc := range []struct {
		name                string
		schedule            QualitySchedule
		after, anchor, want string
	}{
		{"interval retains anchor", QualitySchedule{Enabled: true, Kind: "interval", IntervalMinutes: 10, Timezone: "UTC"}, "2026-09-20T00:23:00Z", "2026-09-20T00:00:00Z", "2026-09-20T00:30:00Z"},
		{"daily Shanghai", QualitySchedule{Enabled: true, Kind: "daily", Time: "09:00", Timezone: "Asia/Shanghai"}, "2026-09-20T01:00:00Z", "2026-09-20T00:00:00Z", "2026-09-21T01:00:00Z"},
		{"spring missing time skipped", QualitySchedule{Enabled: true, Kind: "daily", Time: "02:30", Timezone: "America/New_York"}, "2026-03-08T05:00:00Z", "2026-03-08T05:00:00Z", "2026-03-09T06:30:00Z"},
		{"fall earliest time", QualitySchedule{Enabled: true, Kind: "daily", Time: "01:30", Timezone: "America/New_York"}, "2026-11-01T04:00:00Z", "2026-11-01T04:00:00Z", "2026-11-01T05:30:00Z"},
		{"fall duplicate time skipped", QualitySchedule{Enabled: true, Kind: "daily", Time: "01:30", Timezone: "America/New_York"}, "2026-11-01T05:45:00Z", "2026-11-01T05:30:00Z", "2026-11-02T06:30:00Z"},
		{"weekly Monday", QualitySchedule{Enabled: true, Kind: "weekly", Time: "09:00", Weekdays: []int{1}, Timezone: "UTC"}, "2026-09-20T12:00:00Z", "2026-09-20T12:00:00Z", "2026-09-21T09:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NextQualitySchedule(tc.schedule, utc(tc.after), utc(tc.anchor))
			require.NoError(t, err)
			assert.Equal(t, utc(tc.want), got.UTC())
		})
	}
	_, err := NextQualitySchedule(QualitySchedule{Enabled: true, Kind: "interval", IntervalMinutes: 0, Timezone: "UTC"}, time.Now(), time.Time{})
	assert.Error(t, err)
	got, err := NextQualitySchedule(QualitySchedule{}, time.Now(), time.Time{})
	require.NoError(t, err)
	assert.True(t, got.IsZero())
}

func TestModelQualityTimelineCountsSamplesNotBuckets(t *testing.T) {
	now := time.UnixMilli(1_000_000_000)
	end := now.UnixMilli() + 1
	start := end - 144*QualityBucketMillis
	fast, slow := int64(1000), int64(3000)
	summary, buckets := BuildQualityTimeline([]ModelQualitySample{
		{Status: "succeeded", StartedAt: start, DurationMs: &fast},
		{Status: "failed", StartedAt: start + 5},
		{Status: "succeeded", StartedAt: end - 1, DurationMs: &slow},
		{Status: "succeeded", StartedAt: start - 1, DurationMs: &slow},
		{Status: "failed", StartedAt: end},
		{Status: "pending", CreatedAt: start + 10},
		{Status: "cancelled", CreatedAt: start + 12},
		{Status: "failed", CreatedAt: start + 20},
	}, now)
	require.Len(t, buckets, 144)
	assert.Equal(t, QualityBucket{Start: start, End: start + QualityBucketMillis, Success: 1, Failure: 1, LatestAt: start + 5}, buckets[0])
	assert.Equal(t, 1, buckets[143].Success)
	assert.Equal(t, 0, buckets[1].Success+buckets[1].Failure)
	assert.Equal(t, 2, summary.Success)
	assert.Equal(t, 1, summary.Failure)
	assert.Equal(t, 3, summary.Total)
	assert.Equal(t, 1, summary.Pending)
	assert.Equal(t, 1, summary.Cancelled)
	require.NotNil(t, summary.SuccessRate)
	assert.InDelta(t, 2.0/3, *summary.SuccessRate, 1e-12)
	require.NotNil(t, summary.P50Ms)
	assert.Equal(t, int64(1000), *summary.P50Ms)
	assert.Nil(t, summary.P95Ms)
}

func TestModelQualityTimelineNearestRankAndEmpty(t *testing.T) {
	now := time.UnixMilli(1_000_000_000)
	samples := []ModelQualitySample{}
	for i := range 20 {
		duration := int64((i + 1) * 100)
		samples = append(samples, ModelQualitySample{Status: "succeeded", StartedAt: now.UnixMilli() - int64(i), DurationMs: &duration})
	}
	summary, _ := BuildQualityTimeline(samples, now)
	require.NotNil(t, summary.P50Ms)
	require.NotNil(t, summary.P95Ms)
	assert.Equal(t, int64(1000), *summary.P50Ms)
	assert.Equal(t, int64(1900), *summary.P95Ms)
	empty, buckets := BuildQualityTimeline(nil, now)
	assert.Zero(t, empty.Total)
	assert.Nil(t, empty.SuccessRate)
	assert.Nil(t, empty.P50Ms)
	require.Len(t, buckets, 144)
}
