package model

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type riskStatisticsSummaryRow struct {
	Records       int64 `gorm:"column:records"`
	AffectedUsers int64 `gorm:"column:affected_users"`
	Unsafe        int64 `gorm:"column:unsafe_count"`
	Blocked       int64 `gorm:"column:blocked_count"`
	Errors        int64 `gorm:"column:error_count"`
	CacheHits     int64 `gorm:"column:cache_hits"`
	ProviderCalls int64 `gorm:"column:provider_calls"`
	Neurons       int64 `gorm:"column:neurons"`
}

type riskStatisticsUserRow struct {
	UserID      int    `gorm:"column:user_id"`
	Username    string `gorm:"column:username"`
	Safe        int64  `gorm:"column:safe_count"`
	Unsafe      int64  `gorm:"column:unsafe_count"`
	Errors      int64  `gorm:"column:error_count"`
	NotReviewed int64  `gorm:"column:not_reviewed_count"`
	Total       int64  `gorm:"column:total"`
}

type riskStatisticsChannelRow struct {
	ChannelID   int    `gorm:"column:channel_id"`
	ChannelName string `gorm:"column:channel_name"`
	Safe        int64  `gorm:"column:safe_count"`
	Unsafe      int64  `gorm:"column:unsafe_count"`
	Errors      int64  `gorm:"column:error_count"`
	Total       int64  `gorm:"column:total"`
}

type riskStatisticsSourceRow struct {
	BucketStart int64 `gorm:"column:bucket_start"`
	Provider    int64 `gorm:"column:provider_count"`
	Cache       int64 `gorm:"column:cache_count"`
	Inflight    int64 `gorm:"column:inflight_count"`
	Local       int64 `gorm:"column:local_count"`
	Total       int64 `gorm:"column:total"`
}

func riskStatisticsBaseQuery(ctx context.Context, query RiskStatisticsQuery) *gorm.DB {
	return DB.WithContext(ctx).
		Table("risk_records").
		Where(
			"risk_records.observed_at >= ? AND risk_records.observed_at <= ?",
			time.Unix(query.StartTimestamp, 0).UTC(),
			time.Unix(query.EndTimestamp, 0).UTC(),
		).
		Where("risk_records.path <> ?", riskProviderValidationPath)
}

func queryRiskStatisticsSummary(ctx context.Context, query RiskStatisticsQuery) (RiskStatisticsSummary, error) {
	var row riskStatisticsSummaryRow
	err := riskStatisticsBaseQuery(ctx, query).Select(`
		COUNT(*) AS records,
		COUNT(DISTINCT risk_records.user_id) AS affected_users,
		COALESCE(SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END), 0) AS unsafe_count,
		COALESCE(SUM(CASE WHEN risk_records.blocked = ? THEN 1 ELSE 0 END), 0) AS blocked_count,
		COALESCE(SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END), 0) AS error_count,
		COALESCE(SUM(CASE WHEN risk_records.cache_hit = ? THEN 1 ELSE 0 END), 0) AS cache_hits,
		COALESCE(SUM(CASE WHEN risk_records.provider_called = ? THEN 1 ELSE 0 END), 0) AS provider_calls,
		COALESCE(SUM(risk_records.neurons), 0) AS neurons
	`, RiskRecordResultUnsafe, true, RiskRecordResultError, true, true).Scan(&row).Error
	if err != nil {
		return RiskStatisticsSummary{}, fmt.Errorf("query risk statistics summary: %w", err)
	}
	return RiskStatisticsSummary{
		Records: row.Records, AffectedUsers: row.AffectedUsers, Unsafe: row.Unsafe,
		Blocked: row.Blocked, Errors: row.Errors, CacheHits: row.CacheHits,
		ProviderCalls: row.ProviderCalls, Neurons: row.Neurons,
	}, nil
}

func queryRiskStatisticsP95(ctx context.Context, query RiskStatisticsQuery) (int64, error) {
	base := riskStatisticsBaseQuery(ctx, query).Where("risk_records.provider_called = ? AND risk_records.latency_ms >= ?", true, 0)
	var count int64
	if err := base.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count risk statistics latencies: %w", err)
	}
	if count == 0 {
		return 0, nil
	}
	var row struct {
		LatencyMS int64 `gorm:"column:latency_ms"`
	}
	offset := count - count/20 - 1
	if err := base.Select("risk_records.latency_ms").Order("risk_records.latency_ms ASC").Offset(int(offset)).Limit(1).Scan(&row).Error; err != nil {
		return 0, fmt.Errorf("query risk statistics P95 latency: %w", err)
	}
	return row.LatencyMS, nil
}

func queryRiskStatisticsUsers(ctx context.Context, query RiskStatisticsQuery) ([]RiskStatisticsUser, error) {
	rows := make([]riskStatisticsUserRow, 0, 10)
	err := riskStatisticsBaseQuery(ctx, query).
		Select(`
			risk_records.user_id AS user_id,
			COALESCE(users.username, '') AS username,
			SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END) AS safe_count,
			SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END) AS unsafe_count,
			SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END) AS error_count,
			SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END) AS not_reviewed_count,
			COUNT(*) AS total
		`, RiskRecordResultSafe, RiskRecordResultUnsafe, RiskRecordResultError, RiskRecordResultNotReviewed).
		Joins("LEFT JOIN users ON users.id = risk_records.user_id").
		Group("risk_records.user_id, users.username").
		Order("unsafe_count DESC, error_count DESC, total DESC, risk_records.user_id ASC").
		Limit(10).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query risk statistics users: %w", err)
	}
	users := make([]RiskStatisticsUser, 0, len(rows))
	for _, row := range rows {
		username := row.Username
		if username == "" {
			username = fmt.Sprintf("#%d", row.UserID)
		}
		users = append(users, RiskStatisticsUser{
			UserID: row.UserID, Username: username, Safe: row.Safe, Unsafe: row.Unsafe,
			Errors: row.Errors, NotReviewed: row.NotReviewed, Total: row.Total,
		})
	}
	return users, nil
}

func queryRiskStatisticsChannels(ctx context.Context, query RiskStatisticsQuery) ([]RiskStatisticsChannel, error) {
	rows := make([]riskStatisticsChannelRow, 0)
	err := riskStatisticsBaseQuery(ctx, query).
		Where("risk_records.result IN (?, ?, ?)", RiskRecordResultSafe, RiskRecordResultUnsafe, RiskRecordResultError).
		Select(`
			risk_records.channel_id AS channel_id,
			COALESCE(channels.name, '') AS channel_name,
			SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END) AS safe_count,
			SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END) AS unsafe_count,
			SUM(CASE WHEN risk_records.result = ? THEN 1 ELSE 0 END) AS error_count,
			SUM(CASE WHEN risk_records.result IN (?, ?, ?) THEN 1 ELSE 0 END) AS total
		`, RiskRecordResultSafe, RiskRecordResultUnsafe, RiskRecordResultError,
			RiskRecordResultSafe, RiskRecordResultUnsafe, RiskRecordResultError).
		Joins("LEFT JOIN channels ON channels.id = risk_records.channel_id").
		Group("risk_records.channel_id, channels.name").
		Order("unsafe_count DESC, error_count DESC, total DESC, risk_records.channel_id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query risk statistics channels: %w", err)
	}
	channels := make([]RiskStatisticsChannel, 0, len(rows))
	for _, row := range rows {
		channelName := row.ChannelName
		if channelName == "" {
			channelName = fmt.Sprintf("#%d", row.ChannelID)
		}
		channels = append(channels, RiskStatisticsChannel{
			ChannelID: row.ChannelID, ChannelName: channelName, Safe: row.Safe,
			Unsafe: row.Unsafe, Errors: row.Errors, Total: row.Total,
		})
	}
	return channels, nil
}

func queryRiskStatisticsSourceTrend(ctx context.Context, query RiskStatisticsQuery) ([]RiskStatisticsSourceBucket, error) {
	bucketExpression, err := riskStatisticsBucketExpression(DB.Dialector.Name(), query.Granularity)
	if err != nil {
		return nil, err
	}
	rows := make([]riskStatisticsSourceRow, 0)
	err = riskStatisticsBaseQuery(ctx, query).Select(fmt.Sprintf(`
		%s AS bucket_start,
		SUM(CASE WHEN risk_records.source = ? THEN 1 ELSE 0 END) AS provider_count,
		SUM(CASE WHEN risk_records.source = ? THEN 1 ELSE 0 END) AS cache_count,
		SUM(CASE WHEN risk_records.source = ? THEN 1 ELSE 0 END) AS inflight_count,
		SUM(CASE WHEN risk_records.source = ? OR risk_records.source = ? OR risk_records.source = ? THEN 0 ELSE 1 END) AS local_count,
		COUNT(*) AS total
	`, bucketExpression), RiskRecordSourceProvider, RiskRecordSourceCache, RiskRecordSourceInflight,
		RiskRecordSourceProvider, RiskRecordSourceCache, RiskRecordSourceInflight).
		Group("bucket_start").
		Order("bucket_start ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query risk statistics source trend: %w", err)
	}
	byBucket := make(map[int64]RiskStatisticsSourceBucket, len(rows))
	for _, row := range rows {
		byBucket[row.BucketStart] = RiskStatisticsSourceBucket{
			BucketStart: row.BucketStart, Provider: row.Provider, Cache: row.Cache,
			Inflight: row.Inflight, Local: row.Local, Total: row.Total,
		}
	}
	trend := make([]RiskStatisticsSourceBucket, 0)
	for cursor := riskStatisticsBucketStart(time.Unix(query.StartTimestamp, 0).UTC(), query.Granularity); !cursor.After(time.Unix(query.EndTimestamp, 0).UTC()); cursor = cursor.Add(riskStatisticsBucketStep(query.Granularity)) {
		bucketStart := cursor.Unix()
		if bucket, ok := byBucket[bucketStart]; ok {
			trend = append(trend, bucket)
		} else {
			trend = append(trend, RiskStatisticsSourceBucket{BucketStart: bucketStart})
		}
	}
	return trend, nil
}

func riskStatisticsBucketExpression(dialect string, granularity RiskStatisticsGranularity) (string, error) {
	switch dialect {
	case "mysql", "postgres", "sqlite":
	default:
		return "", fmt.Errorf("unsupported database dialect for risk statistics: %s", dialect)
	}
	step := int64(riskStatisticsBucketStep(granularity) / time.Second)
	if granularity == RiskStatisticsGranularityWeek {
		return riskStatisticsBucketExpressionForDialect(dialect, step, 288000), nil
	}
	return riskStatisticsBucketExpressionForDialect(dialect, step, 28800), nil
}

func riskStatisticsBucketExpressionForDialect(dialect string, step, offset int64) string {
	switch dialect {
	case "mysql":
		epoch := "UNIX_TIMESTAMP(risk_records.observed_at)"
		return fmt.Sprintf("((%s + %d) DIV %d * %d - %d)", epoch, offset, step, step, offset)
	case "postgres":
		epoch := "EXTRACT(EPOCH FROM risk_records.observed_at)"
		return fmt.Sprintf("CAST(FLOOR((%s + %d) / %d) * %d - %d AS BIGINT)", epoch, offset, step, step, offset)
	default:
		epoch := "CAST(strftime('%s', risk_records.observed_at) AS INTEGER)"
		return fmt.Sprintf("((%s + %d) / %d * %d - %d)", epoch, offset, step, step, offset)
	}
}
