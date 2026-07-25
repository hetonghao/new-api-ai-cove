package model

import (
	"context"
	"fmt"
	"time"
)

type RiskRecordQuery struct {
	Offset         int
	Limit          int
	StartTimestamp int64
	EndTimestamp   int64
	ChannelID      int
	UserID         int
	ProviderID     *int
	Result         RiskRecordResult
	Source         RiskRecordSource
}

func QueryRiskRecords(ctx context.Context, filter RiskRecordQuery) ([]*RiskRecord, int64, error) {
	if err := validateRiskRecordQuery(filter); err != nil {
		return nil, 0, err
	}

	query := DB.WithContext(ctx).Model(&RiskRecord{})
	if filter.StartTimestamp > 0 {
		query = query.Where("observed_at >= ?", time.Unix(filter.StartTimestamp, 0).UTC())
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("observed_at <= ?", time.Unix(filter.EndTimestamp, 0).UTC())
	}
	if filter.ChannelID > 0 {
		query = query.Where("channel_id = ?", filter.ChannelID)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.ProviderID != nil {
		query = query.Where("provider_id = ?", *filter.ProviderID)
	}
	if filter.Result != "" {
		query = query.Where("result = ?", filter.Result)
	}
	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count risk records: %w", err)
	}
	records := make([]*RiskRecord, 0)
	if err := query.Order("observed_at desc, id desc").Offset(filter.Offset).Limit(filter.Limit).Find(&records).Error; err != nil {
		return nil, 0, fmt.Errorf("query risk records: %w", err)
	}
	return records, total, nil
}

func validateRiskRecordQuery(filter RiskRecordQuery) error {
	if filter.Offset < 0 || filter.Limit < 1 || filter.Limit > 100 {
		return ErrInvalidRiskRecordPage
	}
	if filter.StartTimestamp < 0 || filter.EndTimestamp < 0 ||
		(filter.StartTimestamp > 0 && filter.EndTimestamp > 0 && filter.StartTimestamp > filter.EndTimestamp) {
		return ErrInvalidRiskRecordPage
	}
	if filter.ChannelID < 0 || filter.UserID < 0 || (filter.ProviderID != nil && *filter.ProviderID < 0) {
		return ErrInvalidRiskRecordPage
	}
	switch filter.Result {
	case "", RiskRecordResultNotReviewed, RiskRecordResultSafe, RiskRecordResultUnsafe, RiskRecordResultError:
	default:
		return ErrInvalidRiskRecordPage
	}
	switch filter.Source {
	case "", RiskRecordSourceLocal, RiskRecordSourceProvider, RiskRecordSourceCache, RiskRecordSourceInflight:
	default:
		return ErrInvalidRiskRecordPage
	}
	return nil
}
