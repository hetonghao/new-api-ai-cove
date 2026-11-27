package model

import (
	"context"

	"gorm.io/gorm/clause"
)

const deviMonResetAttemptGapSeconds = 120

type DeviMonResetPolicy struct {
	AccountKey         string `gorm:"type:varchar(64);primaryKey"`
	Enabled            bool   `gorm:"not null;default:false"`
	AttemptedWindowEnd int64  `gorm:"not null;default:0"`
	AttemptedAt        int64  `gorm:"not null;default:0"`
	HoldExhaustedUsage bool   `gorm:"not null;default:false"`
}

func SetDeviMonResetPolicy(ctx context.Context, accountKey string, enabled bool) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled"}),
	}).Create(&DeviMonResetPolicy{AccountKey: accountKey, Enabled: enabled}).Error
}

func GetDeviMonResetPolicies(ctx context.Context) (map[string]DeviMonResetPolicy, error) {
	var rows []DeviMonResetPolicy
	if err := DB.WithContext(ctx).Where("enabled = ? OR hold_exhausted_usage = ?", true, true).Find(&rows).Error; err != nil {
		return nil, err
	}
	policies := make(map[string]DeviMonResetPolicy, len(rows))
	for _, row := range rows {
		policies[row.AccountKey] = row
	}
	return policies, nil
}

// The claim is durable before the POST: another node must not spend a second
// card from the same reading even if the first response was lost.
func ClaimDeviMonReset(ctx context.Context, accountKey string, windowEnd, now int64, task *SystemTask) (bool, error) {
	db := DB.WithContext(ctx)
	claim := db.Model(&DeviMonResetPolicy{}).
		Where("account_key = ? AND enabled = ? AND attempted_window_end < ? AND attempted_at <= ?", accountKey, true, windowEnd, now-deviMonResetAttemptGapSeconds)
	if task != nil {
		lease := db.Model(&SystemTaskLock{}).Select("1").Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until >= ?", task.Type, task.TaskID, task.LockedBy, now)
		claim = claim.Where("EXISTS (?)", lease)
	}
	result := claim.Updates(map[string]any{"attempted_window_end": windowEnd, "attempted_at": now, "hold_exhausted_usage": true})
	return result.RowsAffected == 1, result.Error
}

func ReleaseDeviMonUsageHold(ctx context.Context, accountKey string, windowEnd int64) error {
	return DB.WithContext(ctx).Model(&DeviMonResetPolicy{}).
		Where("account_key = ? AND attempted_window_end = ?", accountKey, windowEnd).
		Update("hold_exhausted_usage", false).Error
}
