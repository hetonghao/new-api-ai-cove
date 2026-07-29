package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type riskPolicyRequest struct {
	Enabled         *bool                `json:"enabled"`
	ProviderID      *int                 `json:"provider_id"`
	EnabledChannels []int                `json:"enabled_channels"`
	ExcludedUserIDs []int                `json:"excluded_user_ids"`
	ExcludedModels  []string             `json:"excluded_models"`
	ReviewMode      model.RiskReviewMode `json:"review_mode"`
	ActionMode      model.RiskActionMode `json:"action_mode"`
}

func GetRiskPolicy(c *gin.Context) {
	state, err := model.GetRiskPolicyState()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}

func UpdateRiskPolicy(c *gin.Context) {
	var request riskPolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "无效的风控策略")
		return
	}
	state, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		Enabled:         request.Enabled,
		ProviderID:      request.ProviderID,
		EnabledChannels: request.EnabledChannels,
		ExcludedUserIDs: request.ExcludedUserIDs,
		ExcludedModels:  request.ExcludedModels,
		ReviewMode:      request.ReviewMode,
		ActionMode:      request.ActionMode,
	})
	if err != nil {
		if errors.Is(err, model.ErrRiskProviderNotValidated) {
			common.ApiErrorMsg(c, "供应商连接尚未验证")
			return
		}
		if errors.Is(err, model.ErrInvalidRiskPolicy) {
			common.ApiErrorMsg(c, "无效的风控策略")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}
