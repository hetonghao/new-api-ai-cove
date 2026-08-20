package model

import (
	"context"
	"fmt"
	"time"
)

func CountExpiredSevereRiskRecords(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, ErrInvalidSevereRiskRecord
	}
	var count int64
	if err := DB.WithContext(ctx).Model(&SevereRiskRecord{}).Where("triggered_at < ?", cutoff.UTC()).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count expired severe risk records: %w", err)
	}
	return count, nil
}

func DeleteExpiredSevereRiskRecordsBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if cutoff.IsZero() || limit < 1 {
		return 0, ErrInvalidSevereRiskRecord
	}
	var ids []int
	if err := DB.WithContext(ctx).Model(&SevereRiskRecord{}).Where("triggered_at < ?", cutoff.UTC()).Order("triggered_at asc, id asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("select expired severe risk records: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.WithContext(ctx).Where("id IN ?", ids).Delete(&SevereRiskRecord{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete expired severe risk records: %w", result.Error)
	}
	return result.RowsAffected, nil
}
