package model

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type RiskRecordQuery struct {
	Offset         int
	Limit          int
	StartTimestamp int64
	EndTimestamp   int64
	ChannelID      int
	Username       string
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
		query = query.Where("risk_records.observed_at >= ?", time.Unix(filter.StartTimestamp, 0).UTC())
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("risk_records.observed_at <= ?", time.Unix(filter.EndTimestamp, 0).UTC())
	}
	if filter.ChannelID > 0 {
		query = query.Where("risk_records.channel_id = ?", filter.ChannelID)
	}
	if filter.Username != "" {
		userQuery := DB.WithContext(ctx).Model(&User{})
		if strings.Contains(filter.Username, "%") {
			pattern, err := sanitizeLikePattern(filter.Username)
			if err != nil {
				return nil, 0, ErrInvalidRiskRecordPage
			}
			userQuery = userQuery.Where("username LIKE ? ESCAPE '!'", pattern)
		} else {
			userQuery = userQuery.Where("username = ?", filter.Username)
		}
		userIDs := make([]int, 0)
		if err := userQuery.Pluck("id", &userIDs).Error; err != nil {
			return nil, 0, fmt.Errorf("query risk record users: %w", err)
		}
		if len(userIDs) == 0 {
			return []*RiskRecord{}, 0, nil
		}
		query = query.Where("risk_records.user_id IN ?", userIDs)
	}
	if filter.ProviderID != nil {
		query = query.Where("risk_records.provider_id = ?", *filter.ProviderID)
	}
	if filter.Result != "" {
		query = query.Where("risk_records.result = ?", filter.Result)
	}
	if filter.Source != "" {
		query = query.Where("risk_records.source = ?", filter.Source)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count risk records: %w", err)
	}
	records := make([]*RiskRecord, 0)
	query = query.Select(
		"risk_records.*, channels.name AS channel_name, users.username AS username, tokens.name AS token_name",
	).Joins(
		"LEFT JOIN channels ON channels.id = risk_records.channel_id",
	).Joins(
		"LEFT JOIN users ON users.id = risk_records.user_id",
	).Joins(
		"LEFT JOIN tokens ON tokens.id = risk_records.token_id",
	)
	if err := query.Order("risk_records.observed_at desc, risk_records.id desc").Offset(filter.Offset).Limit(filter.Limit).Find(&records).Error; err != nil {
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
	usernameText := strings.ReplaceAll(filter.Username, "%", "")
	if filter.ChannelID < 0 || utf8.RuneCountInString(usernameText) > UserNameMaxLength ||
		(filter.ProviderID != nil && *filter.ProviderID < 0) {
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
