package model

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidRiskRecordCleanup = errors.New("invalid risk record cleanup")

const SystemTaskTypeRiskRecordCleanup = "risk_record_cleanup"

func DeleteExpiredRiskRecordsBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if cutoff.IsZero() || limit < 1 {
		return 0, ErrInvalidRiskRecordCleanup
	}

	expiredIDs := make([]int, 0, limit)
	if err := DB.WithContext(ctx).Model(&RiskRecord{}).
		Where("observed_at < ?", cutoff.UTC()).
		Order("observed_at asc, id asc").
		Limit(limit).
		Pluck("id", &expiredIDs).Error; err != nil {
		return 0, fmt.Errorf("select expired risk records: %w", err)
	}
	if len(expiredIDs) == 0 {
		return 0, nil
	}
	result := DB.WithContext(ctx).Where("id IN ?", expiredIDs).Delete(&RiskRecord{})
	if result.Error != nil {
		return 0, fmt.Errorf("delete expired risk records: %w", result.Error)
	}
	return result.RowsAffected, nil
}
