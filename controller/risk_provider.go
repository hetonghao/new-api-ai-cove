package controller

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type RiskProviderResponse struct {
	Id               int                    `json:"id"`
	Name             string                 `json:"name"`
	ProviderType     model.RiskProviderType `json:"provider_type"`
	Model            string                 `json:"model"`
	BaseURL          string                 `json:"base_url"`
	HasCredential    bool                   `json:"has_credential"`
	TimeoutMs        int                    `json:"timeout_ms"`
	FailureThreshold int                    `json:"failure_threshold"`
	CooldownSeconds  int                    `json:"cooldown_seconds"`
	ValidatedAt      *time.Time             `json:"validated_at"`
	Active           bool                   `json:"active"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type riskProviderRequest struct {
	Name             string                 `json:"name" binding:"required"`
	ProviderType     model.RiskProviderType `json:"provider_type" binding:"required,oneof=cloudflare"`
	Model            string                 `json:"model" binding:"required"`
	BaseURL          string                 `json:"base_url" binding:"required,url"`
	Credential       string                 `json:"credential"`
	TimeoutMs        int                    `json:"timeout_ms" binding:"omitempty,gte=1,lte=60000"`
	FailureThreshold int                    `json:"failure_threshold" binding:"omitempty,gte=1,lte=100"`
	CooldownSeconds  int                    `json:"cooldown_seconds" binding:"omitempty,gte=1,lte=86400"`
}

func ListRiskProviders(c *gin.Context) {
	providers, err := model.GetRiskProviders()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := make([]RiskProviderResponse, 0, len(providers))
	for _, provider := range providers {
		response = append(response, toRiskProviderResponse(provider))
	}
	common.ApiSuccess(c, response)
}

func CreateRiskProvider(c *gin.Context) {
	var request riskProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Credential == "" {
		common.ApiErrorMsg(c, "无效的供应商配置")
		return
	}
	ciphertext, err := common.EncryptCredential(request.Credential)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	provider := &model.RiskProvider{
		Name: request.Name, ProviderType: request.ProviderType, Model: request.Model, BaseURL: request.BaseURL,
		CredentialEncrypted: ciphertext, TimeoutMs: request.TimeoutMs, FailureThreshold: request.FailureThreshold,
		CooldownSeconds: request.CooldownSeconds,
	}
	if err := model.CreateRiskProvider(provider); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, toRiskProviderResponse(provider))
}

func UpdateRiskProvider(c *gin.Context) {
	id, ok := parseRiskProviderID(c)
	if !ok {
		return
	}
	var request riskProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "无效的供应商配置")
		return
	}
	provider, err := model.GetRiskProviderByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	connectionChanged := provider.ProviderType != request.ProviderType ||
		provider.Model != strings.TrimSpace(request.Model) ||
		provider.BaseURL != strings.TrimRight(strings.TrimSpace(request.BaseURL), "/") ||
		request.Credential != ""
	provider.Name = request.Name
	provider.ProviderType = request.ProviderType
	provider.Model = request.Model
	provider.BaseURL = request.BaseURL
	provider.TimeoutMs = request.TimeoutMs
	provider.FailureThreshold = request.FailureThreshold
	provider.CooldownSeconds = request.CooldownSeconds
	if request.Credential != "" {
		provider.CredentialEncrypted, err = common.EncryptCredential(request.Credential)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if connectionChanged {
		provider.ValidatedAt = nil
		provider.Active = false
	}
	if err := model.UpdateRiskProvider(provider); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, toRiskProviderResponse(provider))
}

func DeleteRiskProvider(c *gin.Context) {
	id, ok := parseRiskProviderID(c)
	if !ok {
		return
	}
	if err := model.DeleteRiskProvider(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ValidateRiskProvider(c *gin.Context) {
	id, ok := parseRiskProviderID(c)
	if !ok {
		return
	}
	provider, err := model.GetRiskProviderByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := service.ReviewRiskContent(c.Request.Context(), provider, "AI Cove provider connection test")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.MarkRiskProviderValidated(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func ActivateRiskProvider(c *gin.Context) {
	id, ok := parseRiskProviderID(c)
	if !ok {
		return
	}
	if err := model.ActivateRiskProvider(id); err != nil {
		if errors.Is(err, model.ErrRiskProviderNotValidated) {
			common.ApiErrorMsg(c, "供应商连接尚未验证")
			return
		}
		common.ApiError(c, err)
		return
	}
	provider, err := model.GetRiskProviderByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, toRiskProviderResponse(provider))
}

func parseRiskProviderID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		common.ApiErrorMsg(c, "无效的供应商 ID")
		return 0, false
	}
	return id, true
}

func toRiskProviderResponse(provider *model.RiskProvider) RiskProviderResponse {
	return RiskProviderResponse{
		Id: provider.Id, Name: provider.Name, ProviderType: provider.ProviderType, Model: provider.Model,
		BaseURL: provider.BaseURL, HasCredential: provider.CredentialEncrypted != "", TimeoutMs: provider.TimeoutMs,
		FailureThreshold: provider.FailureThreshold, CooldownSeconds: provider.CooldownSeconds,
		ValidatedAt: provider.ValidatedAt, Active: provider.Active, CreatedAt: provider.CreatedAt, UpdatedAt: provider.UpdatedAt,
	}
}
