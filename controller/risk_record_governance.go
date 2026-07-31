package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type riskRecordGovernanceRequest struct {
	SaveScope           model.RiskRecordSaveScope  `json:"save_scope" binding:"required,oneof=all suspicious unsafe"`
	ContentSaveScope    model.RiskContentSaveScope `json:"content_save_scope" binding:"required,oneof=all unsafe none"`
	RetentionDays       int                        `json:"retention_days" binding:"required,gte=1,lte=180"`
	SafePreviewChars    int                        `json:"safe_preview_chars"`
	NonSafePreviewChars int                        `json:"non_safe_preview_chars"`
	PreviewChars        *int                       `json:"preview_chars"`
}

func GetRiskRecordGovernance(c *gin.Context) {
	governance, err := model.GetRiskRecordGovernance(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, governance)
}

func UpdateRiskRecordGovernance(c *gin.Context) {
	var request riskRecordGovernanceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "无效的风控记录治理设置")
		return
	}
	if request.PreviewChars == nil && (request.SafePreviewChars < model.RiskRecordPreviewCharsMin || request.NonSafePreviewChars < model.RiskRecordPreviewCharsMin) {
		common.ApiErrorMsg(c, "无效的风控记录治理设置")
		return
	}
	if request.PreviewChars != nil && (*request.PreviewChars < model.RiskRecordPreviewCharsMin ||
		(request.SafePreviewChars != 0 && request.SafePreviewChars < model.RiskRecordPreviewCharsMin) ||
		(request.NonSafePreviewChars != 0 && request.NonSafePreviewChars < model.RiskRecordPreviewCharsMin)) {
		common.ApiErrorMsg(c, "无效的风控记录治理设置")
		return
	}
	if request.PreviewChars != nil {
		if request.SafePreviewChars == 0 {
			request.SafePreviewChars = *request.PreviewChars
		}
		if request.NonSafePreviewChars == 0 {
			request.NonSafePreviewChars = *request.PreviewChars
		}
	}
	governance, err := model.SaveRiskRecordGovernance(c.Request.Context(), model.RiskRecordGovernanceInput{
		SaveScope: request.SaveScope, ContentSaveScope: request.ContentSaveScope,
		RetentionDays: request.RetentionDays, SafePreviewChars: request.SafePreviewChars,
		NonSafePreviewChars: request.NonSafePreviewChars,
	})
	if err != nil {
		if errors.Is(err, model.ErrInvalidRiskRecordGovernance) {
			common.ApiErrorMsg(c, "无效的风控记录治理设置")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, governance)
}
