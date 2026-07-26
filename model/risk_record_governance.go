package model

import (
	"context"
	"errors"
	"fmt"
)

type RiskRecordSaveScope string
type RiskContentSaveScope string

const (
	RiskRecordSaveAll        RiskRecordSaveScope = "all"
	RiskRecordSaveSuspicious RiskRecordSaveScope = "suspicious"
	RiskRecordSaveUnsafe     RiskRecordSaveScope = "unsafe"

	RiskContentSaveAll    RiskContentSaveScope = "all"
	RiskContentSaveUnsafe RiskContentSaveScope = "unsafe"
	RiskContentSaveNone   RiskContentSaveScope = "none"

	riskRecordGovernanceID = 1
)

var ErrInvalidRiskRecordGovernance = errors.New("invalid risk record governance")

type RiskRecordGovernance struct {
	Id               int                  `json:"-" gorm:"primaryKey"`
	SaveScope        RiskRecordSaveScope  `json:"save_scope" gorm:"type:varchar(16);not null"`
	ContentSaveScope RiskContentSaveScope `json:"content_save_scope" gorm:"type:varchar(16);not null;default:all"`
	RetentionDays    int                  `json:"retention_days" gorm:"not null"`
}

type RiskRecordGovernanceInput struct {
	SaveScope        RiskRecordSaveScope
	ContentSaveScope RiskContentSaveScope
	RetentionDays    int
}

func (RiskRecordGovernance) TableName() string {
	return "risk_record_governance"
}

func GetRiskRecordGovernance(ctx context.Context) (RiskRecordGovernance, error) {
	governance := RiskRecordGovernance{
		Id: riskRecordGovernanceID, SaveScope: RiskRecordSaveAll,
		ContentSaveScope: RiskContentSaveAll, RetentionDays: 30,
	}
	query := DB.WithContext(ctx).Where("id = ?", riskRecordGovernanceID).Limit(1).Find(&governance)
	if query.Error != nil {
		return RiskRecordGovernance{}, fmt.Errorf("get risk record governance: %w", query.Error)
	}
	if query.RowsAffected == 0 {
		return governance, nil
	}
	if err := validateRiskRecordGovernance(governance.SaveScope, governance.ContentSaveScope, governance.RetentionDays); err != nil {
		return RiskRecordGovernance{}, err
	}
	return governance, nil
}

func SaveRiskRecordGovernance(ctx context.Context, input RiskRecordGovernanceInput) (RiskRecordGovernance, error) {
	if err := validateRiskRecordGovernance(input.SaveScope, input.ContentSaveScope, input.RetentionDays); err != nil {
		return RiskRecordGovernance{}, err
	}
	governance := RiskRecordGovernance{Id: riskRecordGovernanceID}
	values := RiskRecordGovernance{
		SaveScope: input.SaveScope, ContentSaveScope: input.ContentSaveScope, RetentionDays: input.RetentionDays,
	}
	if err := DB.WithContext(ctx).Where("id = ?", riskRecordGovernanceID).Assign(values).FirstOrCreate(&governance).Error; err != nil {
		return RiskRecordGovernance{}, fmt.Errorf("save risk record governance: %w", err)
	}
	return governance, nil
}

func validateRiskRecordGovernance(scope RiskRecordSaveScope, contentScope RiskContentSaveScope, retentionDays int) error {
	switch scope {
	case RiskRecordSaveAll, RiskRecordSaveSuspicious, RiskRecordSaveUnsafe:
	default:
		return ErrInvalidRiskRecordGovernance
	}
	switch contentScope {
	case RiskContentSaveAll, RiskContentSaveUnsafe, RiskContentSaveNone:
	default:
		return ErrInvalidRiskRecordGovernance
	}
	if retentionDays < 1 || retentionDays > 180 {
		return ErrInvalidRiskRecordGovernance
	}
	return nil
}
