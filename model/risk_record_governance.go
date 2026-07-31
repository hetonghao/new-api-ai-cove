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

	riskRecordGovernanceID        = 1
	RiskRecordPreviewCharsDefault = 200
	RiskRecordPreviewCharsMin     = 50
)

var ErrInvalidRiskRecordGovernance = errors.New("invalid risk record governance")

type RiskRecordGovernance struct {
	Id                  int                  `json:"-" gorm:"primaryKey"`
	SaveScope           RiskRecordSaveScope  `json:"save_scope" gorm:"type:varchar(16);not null"`
	ContentSaveScope    RiskContentSaveScope `json:"content_save_scope" gorm:"type:varchar(16);not null;default:all"`
	RetentionDays       int                  `json:"retention_days" gorm:"not null"`
	PreviewChars        int                  `json:"preview_chars" gorm:"not null;default:200"`
	SafePreviewChars    int                  `json:"safe_preview_chars" gorm:"not null;default:0"`
	NonSafePreviewChars int                  `json:"non_safe_preview_chars" gorm:"not null;default:0"`
}

type RiskRecordGovernanceInput struct {
	SaveScope           RiskRecordSaveScope
	ContentSaveScope    RiskContentSaveScope
	RetentionDays       int
	PreviewChars        int
	SafePreviewChars    int
	NonSafePreviewChars int
}

func (RiskRecordGovernance) TableName() string {
	return "risk_record_governance"
}

func GetRiskRecordGovernance(ctx context.Context) (RiskRecordGovernance, error) {
	governance := RiskRecordGovernance{
		Id: riskRecordGovernanceID, SaveScope: RiskRecordSaveAll,
		ContentSaveScope: RiskContentSaveAll, RetentionDays: 30,
		PreviewChars: RiskRecordPreviewCharsDefault, SafePreviewChars: RiskRecordPreviewCharsDefault,
		NonSafePreviewChars: RiskRecordPreviewCharsDefault,
	}
	query := DB.WithContext(ctx).Where("id = ?", riskRecordGovernanceID).Limit(1).Find(&governance)
	if query.Error != nil {
		return RiskRecordGovernance{}, fmt.Errorf("get risk record governance: %w", query.Error)
	}
	if query.RowsAffected == 0 {
		return governance, nil
	}
	normalizeRiskRecordGovernancePreviewChars(&governance)
	if err := validateRiskRecordGovernance(governance.SaveScope, governance.ContentSaveScope, governance.RetentionDays, governance.SafePreviewChars, governance.NonSafePreviewChars); err != nil {
		return RiskRecordGovernance{}, err
	}
	return governance, nil
}

func SaveRiskRecordGovernance(ctx context.Context, input RiskRecordGovernanceInput) (RiskRecordGovernance, error) {
	if input.SafePreviewChars == 0 && input.NonSafePreviewChars == 0 && input.PreviewChars != 0 {
		input.SafePreviewChars = input.PreviewChars
		input.NonSafePreviewChars = input.PreviewChars
	}
	if input.SafePreviewChars == 0 {
		input.SafePreviewChars = RiskRecordPreviewCharsDefault
	}
	if input.NonSafePreviewChars == 0 {
		input.NonSafePreviewChars = RiskRecordPreviewCharsDefault
	}
	if err := validateRiskRecordGovernance(input.SaveScope, input.ContentSaveScope, input.RetentionDays, input.SafePreviewChars, input.NonSafePreviewChars); err != nil {
		return RiskRecordGovernance{}, err
	}
	governance := RiskRecordGovernance{Id: riskRecordGovernanceID}
	values := RiskRecordGovernance{
		SaveScope: input.SaveScope, ContentSaveScope: input.ContentSaveScope,
		RetentionDays: input.RetentionDays, PreviewChars: input.SafePreviewChars,
		SafePreviewChars: input.SafePreviewChars, NonSafePreviewChars: input.NonSafePreviewChars,
	}
	if err := DB.WithContext(ctx).Where("id = ?", riskRecordGovernanceID).Assign(values).FirstOrCreate(&governance).Error; err != nil {
		return RiskRecordGovernance{}, fmt.Errorf("save risk record governance: %w", err)
	}
	return governance, nil
}

func (governance RiskRecordGovernance) PreviewCharsForResult(result RiskRecordResult) int {
	if result == RiskRecordResultSafe {
		return governance.SafePreviewChars
	}
	return governance.NonSafePreviewChars
}

func normalizeRiskRecordGovernancePreviewChars(governance *RiskRecordGovernance) {
	legacyPreviewChars := governance.PreviewChars
	if legacyPreviewChars < RiskRecordPreviewCharsMin {
		legacyPreviewChars = RiskRecordPreviewCharsDefault
	}
	if governance.SafePreviewChars == 0 {
		governance.SafePreviewChars = legacyPreviewChars
	}
	if governance.NonSafePreviewChars == 0 {
		governance.NonSafePreviewChars = legacyPreviewChars
	}
	governance.PreviewChars = governance.SafePreviewChars
}

func validateRiskRecordGovernance(scope RiskRecordSaveScope, contentScope RiskContentSaveScope, retentionDays int, safePreviewChars int, nonSafePreviewChars int) error {
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
	if safePreviewChars < RiskRecordPreviewCharsMin || nonSafePreviewChars < RiskRecordPreviewCharsMin {
		return ErrInvalidRiskRecordGovernance
	}
	return nil
}
