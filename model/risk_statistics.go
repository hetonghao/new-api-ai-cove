package model

import (
	"context"
	"errors"
	"time"
)

type RiskStatisticsGranularity string

const (
	RiskStatisticsGranularityHour RiskStatisticsGranularity = "hour"
	RiskStatisticsGranularityDay  RiskStatisticsGranularity = "day"
	RiskStatisticsGranularityWeek RiskStatisticsGranularity = "week"
	riskStatisticsMaxRange                                  = 29 * 24 * time.Hour
)

var ErrInvalidRiskStatisticsQuery = errors.New("invalid risk statistics query")

type RiskStatisticsQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	Granularity    RiskStatisticsGranularity
}

type RiskStatisticsSummary struct {
	Records       int64   `json:"records"`
	AffectedUsers int64   `json:"affected_users"`
	Unsafe        int64   `json:"unsafe"`
	UnsafeRate    float64 `json:"unsafe_rate"`
	Blocked       int64   `json:"blocked"`
	BlockedRate   float64 `json:"blocked_rate"`
	Errors        int64   `json:"errors"`
	ErrorRate     float64 `json:"error_rate"`
	CacheHits     int64   `json:"cache_hits"`
	CacheHitRate  float64 `json:"cache_hit_rate"`
	ProviderCalls int64   `json:"provider_calls"`
	Neurons       int64   `json:"neurons"`
	P95LatencyMS  int64   `json:"p95_latency_ms"`
}

type RiskStatisticsUser struct {
	UserID      int    `json:"user_id"`
	Username    string `json:"username"`
	Safe        int64  `json:"safe"`
	Unsafe      int64  `json:"unsafe"`
	Errors      int64  `json:"errors"`
	NotReviewed int64  `json:"not_reviewed"`
	Total       int64  `json:"total"`
}

type RiskStatisticsChannel struct {
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Safe        int64  `json:"safe"`
	Unsafe      int64  `json:"unsafe"`
	Errors      int64  `json:"errors"`
	Total       int64  `json:"total"`
}

type RiskStatisticsSourceBucket struct {
	BucketStart int64 `json:"bucket_start"`
	Provider    int64 `json:"provider"`
	Cache       int64 `json:"cache"`
	Inflight    int64 `json:"inflight"`
	Local       int64 `json:"local"`
	Total       int64 `json:"total"`
}

type RiskStatistics struct {
	StartTimestamp int64                        `json:"start_timestamp"`
	EndTimestamp   int64                        `json:"end_timestamp"`
	Granularity    RiskStatisticsGranularity    `json:"granularity"`
	Summary        RiskStatisticsSummary        `json:"summary"`
	Users          []RiskStatisticsUser         `json:"users"`
	Channels       []RiskStatisticsChannel      `json:"channels"`
	SourceTrend    []RiskStatisticsSourceBucket `json:"source_trend"`
}

func QueryRiskStatistics(ctx context.Context, input RiskStatisticsQuery) (RiskStatistics, error) {
	query, err := normalizeRiskStatisticsQuery(input)
	if err != nil {
		return RiskStatistics{}, err
	}
	summary, err := queryRiskStatisticsSummary(ctx, query)
	if err != nil {
		return RiskStatistics{}, err
	}
	summary.P95LatencyMS, err = queryRiskStatisticsP95(ctx, query)
	if err != nil {
		return RiskStatistics{}, err
	}
	summary.UnsafeRate = percentage(summary.Unsafe, summary.Records)
	summary.BlockedRate = percentage(summary.Blocked, summary.Records)
	summary.ErrorRate = percentage(summary.Errors, summary.Records)
	summary.CacheHitRate = percentage(summary.CacheHits, summary.Records)
	userList, err := queryRiskStatisticsUsers(ctx, query)
	if err != nil {
		return RiskStatistics{}, err
	}
	channelList, err := queryRiskStatisticsChannels(ctx, query)
	if err != nil {
		return RiskStatistics{}, err
	}
	sourceTrend, err := queryRiskStatisticsSourceTrend(ctx, query)
	if err != nil {
		return RiskStatistics{}, err
	}

	return RiskStatistics{
		StartTimestamp: query.StartTimestamp,
		EndTimestamp:   query.EndTimestamp,
		Granularity:    query.Granularity,
		Summary:        summary,
		Users:          userList,
		Channels:       channelList,
		SourceTrend:    sourceTrend,
	}, nil
}

func normalizeRiskStatisticsQuery(input RiskStatisticsQuery) (RiskStatisticsQuery, error) {
	granularity := input.Granularity
	if granularity == "" {
		granularity = RiskStatisticsGranularityHour
	}
	switch granularity {
	case RiskStatisticsGranularityHour, RiskStatisticsGranularityDay, RiskStatisticsGranularityWeek:
	default:
		return RiskStatisticsQuery{}, ErrInvalidRiskStatisticsQuery
	}
	end := input.EndTimestamp
	if end == 0 {
		end = time.Now().UTC().Unix()
	}
	start := input.StartTimestamp
	if start == 0 {
		start = end - int64(24*time.Hour/time.Second)
	}
	maxRangeSeconds := int64(riskStatisticsMaxRange / time.Second)
	if start < 0 || end < 0 || start > end || end-start > maxRangeSeconds {
		return RiskStatisticsQuery{}, ErrInvalidRiskStatisticsQuery
	}
	return RiskStatisticsQuery{StartTimestamp: start, EndTimestamp: end, Granularity: granularity}, nil
}

func riskStatisticsBucketStart(value time.Time, granularity RiskStatisticsGranularity) time.Time {
	local := value.UTC().In(time.FixedZone("UTC+8", 8*60*60))
	switch granularity {
	case RiskStatisticsGranularityDay:
		local = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	case RiskStatisticsGranularityWeek:
		local = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
		daysSinceMonday := (int(local.Weekday()) + 6) % 7
		local = local.AddDate(0, 0, -daysSinceMonday)
	default:
		local = time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, local.Location())
	}
	return local.UTC()
}

func riskStatisticsBucketStep(granularity RiskStatisticsGranularity) time.Duration {
	switch granularity {
	case RiskStatisticsGranularityDay:
		return 24 * time.Hour
	case RiskStatisticsGranularityWeek:
		return 7 * 24 * time.Hour
	default:
		return time.Hour
	}
}

func percentage(value, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}
