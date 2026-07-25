package model

import (
	"context"
	"errors"
	"fmt"
)

type RiskRecordSaveScope string

const (
	RiskRecordSaveAll        RiskRecordSaveScope = "all"
	RiskRecordSaveSuspicious RiskRecordSaveScope = "suspicious"
	RiskRecordSaveUnsafe     RiskRecordSaveScope = "unsafe"

	riskRecordGovernanceID = 1
)

var ErrInvalidRiskRecordGovernance = errors.New("invalid risk record governance")

type RiskRecordGovernance struct {
	Id            int                 `json:"-" gorm:"primaryKey"`
	SaveScope     RiskRecordSaveScope `json:"save_scope" gorm:"type:varchar(16);not null"`
	RetentionDays int                 `json:"retention_days" gorm:"not null"`
}

type RiskRecordGovernanceInput struct {
	SaveScope     RiskRecordSaveScope
	RetentionDays int
}

func (RiskRecordGovernance) TableName() string {
	return "risk_record_governance"
}

func GetRiskRecordGovernance(ctx context.Context) (RiskRecordGovernance, error) {
	governance := RiskRecordGovernance{
		Id: riskRecordGovernanceID, SaveScope: RiskRecordSaveAll, RetentionDays: 30,
	}
	query := DB.WithContext(ctx).Where("id = ?", riskRecordGovernanceID).Limit(1).Find(&governance)
	if query.Error != nil {
		return RiskRecordGovernance{}, fmt.Errorf("get risk record governance: %w", query.Error)
	}
	if query.RowsAffected == 0 {
		return governance, nil
	}
	if err := validateRiskRecordGovernance(governance.SaveScope, governance.RetentionDays); err != nil {
		return RiskRecordGovernance{}, err
	}
	return governance, nil
}

func SaveRiskRecordGovernance(ctx context.Context, input RiskRecordGovernanceInput) (RiskRecordGovernance, error) {
	if err := validateRiskRecordGovernance(input.SaveScope, input.RetentionDays); err != nil {
		return RiskRecordGovernance{}, err
	}
	governance := RiskRecordGovernance{Id: riskRecordGovernanceID}
	values := RiskRecordGovernance{SaveScope: input.SaveScope, RetentionDays: input.RetentionDays}
	if err := DB.WithContext(ctx).Where("id = ?", riskRecordGovernanceID).Assign(values).FirstOrCreate(&governance).Error; err != nil {
		return RiskRecordGovernance{}, fmt.Errorf("save risk record governance: %w", err)
	}
	return governance, nil
}

func validateRiskRecordGovernance(scope RiskRecordSaveScope, retentionDays int) error {
	switch scope {
	case RiskRecordSaveAll, RiskRecordSaveSuspicious, RiskRecordSaveUnsafe:
	default:
		return ErrInvalidRiskRecordGovernance
	}
	if retentionDays < 1 || retentionDays > 180 {
		return ErrInvalidRiskRecordGovernance
	}
	return nil
}
