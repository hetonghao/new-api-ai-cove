package model

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const deviMonResetAttemptGapSeconds = 120

type DeviMonResetPolicy struct {
	AccountKey             string `gorm:"type:varchar(64);primaryKey"`
	Enabled                bool   `gorm:"not null;default:false"`
	KeepDisabledAfterReset bool   `gorm:"not null;default:false"`
	PausedCredential       string `gorm:"type:varchar(64);not null;default:''"`
	PausedWindowEnd        int64  `gorm:"not null;default:0"`
	AttemptedWindowEnd     int64  `gorm:"not null;default:0"`
	AttemptedAt            int64  `gorm:"not null;default:0"`
	HoldExhaustedUsage     bool   `gorm:"not null;default:false"`
}

func SetDeviMonResetPolicy(ctx context.Context, accountKey string, enabled bool) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled"}),
	}).Create(&DeviMonResetPolicy{AccountKey: accountKey, Enabled: enabled}).Error
}

func GetDeviMonResetPolicies(ctx context.Context) (map[string]DeviMonResetPolicy, error) {
	var rows []DeviMonResetPolicy
	if err := DB.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	policies := make(map[string]DeviMonResetPolicy, len(rows))
	for _, row := range rows {
		policies[row.AccountKey] = row
	}
	return policies, nil
}

func SetDeviMonResumeAfterReset(ctx context.Context, accountKey string, enable bool) error {
	fields := []string{"keep_disabled_after_reset"}
	if !enable {
		fields = append(fields, "paused_credential", "paused_window_end")
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_key"}}, DoUpdates: clause.AssignmentColumns(fields),
	}).Create(&DeviMonResetPolicy{AccountKey: accountKey, KeepDisabledAfterReset: !enable}).Error
}

// Serialize the decision with policy edits on either gateway. No quota or
// subscription probe runs under this lock; the CPA management call is bounded.
func UpdateDeviMonQuotaPause(ctx context.Context, key string, task *SystemTask, apply func(*DeviMonResetPolicy) error) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&DeviMonResetPolicy{AccountKey: key}).Error; err != nil {
			return err
		}
		var p DeviMonResetPolicy
		if err := lockForUpdate(tx).First(&p, "account_key = ?", key).Error; err != nil {
			return err
		}
		if task != nil {
			var count int64
			if err := tx.Model(&SystemTaskLock{}).Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until >= ?", task.Type, task.TaskID, task.LockedBy, time.Now().Unix()).Count(&count).Error; err != nil || count == 0 {
				return err
			}
		}
		name, end := p.PausedCredential, p.PausedWindowEnd
		if err := apply(&p); err != nil {
			return err
		}
		if name == p.PausedCredential && end == p.PausedWindowEnd {
			return nil
		}
		return tx.Model(&p).Updates(map[string]any{"paused_credential": p.PausedCredential, "paused_window_end": p.PausedWindowEnd}).Error
	})
}

func ForgetDeviMonQuotaPause(ctx context.Context, credential string) error {
	return DB.WithContext(ctx).Model(&DeviMonResetPolicy{}).Where("paused_credential = ?", credential).Updates(map[string]any{"paused_credential": "", "paused_window_end": 0}).Error
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
