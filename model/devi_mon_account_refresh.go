package model

import (
	"context"

	"gorm.io/gorm/clause"
)

// One manual reading per account, shared by both gateways. A full collection
// starting after the reading supersedes it without moving the global schedule.
type DeviMonAccountRefresh struct {
	Email       string `gorm:"type:varchar(254);primaryKey"`
	StartedAtUS int64  `gorm:"index"`
	Data        string `gorm:"type:text"`
}

func SaveDeviMonAccountRefresh(ctx context.Context, reading *DeviMonAccountRefresh) error {
	db := DB.WithContext(ctx)
	// Ignore a parallel insert, then conditionally update. This also works on
	// MySQL, whose ON DUPLICATE KEY UPDATE has no WHERE clause.
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(reading)
	if result.Error != nil {
		return result.Error
	}
	// MySQL can report a matched row for a no-op conflict, so don't infer an
	// insert from RowsAffected. The timestamp check keeps newer readings intact.
	return db.Model(&DeviMonAccountRefresh{}).Where("email = ? AND started_at_us < ?", reading.Email, reading.StartedAtUS).Updates(map[string]any{"started_at_us": reading.StartedAtUS, "data": reading.Data}).Error
}

func GetDeviMonRefreshesSince(start int64) ([]DeviMonAccountRefresh, error) {
	var rows []DeviMonAccountRefresh
	err := DB.Where("started_at_us >= ?", start).Find(&rows).Error
	return rows, err
}

func DeleteDeviMonRefreshesBefore(start int64) error {
	return DB.Where("started_at_us < ?", start).Delete(&DeviMonAccountRefresh{}).Error
}
