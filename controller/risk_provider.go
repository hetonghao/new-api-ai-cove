// allow: SIZE_OK -- existing controller endpoint group; this task extends response normalization and derived runtime status mapping.
package controller

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type RiskProviderResponse struct {
	Id                    int                    `json:"id"`
	Name                  string                 `json:"name"`
	ProviderType          model.RiskProviderType `json:"provider_type"`
	AccountID             string                 `json:"account_id"`
	ChannelID             int                    `json:"channel_id"`
	Model                 string                 `json:"model"`
	HasCredential         bool                   `json:"has_credential"`
	SystemManaged         bool                   `json:"system_managed"`
	TimeoutMs             int                    `json:"timeout_ms"`
	FailureThreshold      int                    `json:"failure_threshold"`
	CooldownSeconds       int                    `json:"cooldown_seconds"`
	Priority              int                    `json:"priority"`
	DailyNeuronsLimit     int64                  `json:"daily_neurons_limit"`
	DailyResetTime        string                 `json:"daily_reset_time"`
	CurrentStatus         string                 `json:"current_status"`
	DailyNeuronsUsed      int64                  `json:"daily_neurons_used"`
	DailyNeuronsReserved  int64                  `json:"daily_neurons_reserved"`
	DailyNeuronsRemaining int64                  `json:"daily_neurons_remaining"`
	DailyNeuronsResetAt   *time.Time             `json:"daily_neurons_reset_at,omitempty"`
	ValidatedAt           *time.Time             `json:"validated_at"`
	Active                bool                   `json:"active"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

type riskProviderRequest struct {
	Name              string                 `json:"name" binding:"required"`
	ProviderType      model.RiskProviderType `json:"provider_type" binding:"required,oneof=cloudflare platform_internal"`
	AccountID         string                 `json:"account_id"`
	ChannelID         int                    `json:"channel_id"`
	Model             string                 `json:"model" binding:"required"`
	Credential        string                 `json:"credential"`
	TimeoutMs         int                    `json:"timeout_ms" binding:"omitempty,gte=1,lte=60000"`
	FailureThreshold  int                    `json:"failure_threshold" binding:"omitempty,gte=1,lte=100"`
	CooldownSeconds   int                    `json:"cooldown_seconds" binding:"omitempty,gte=1,lte=86400"`
	Priority          int                    `json:"priority" binding:"omitempty,gte=0,lte=1000000000"`
	DailyNeuronsLimit int64                  `json:"daily_neurons_limit" binding:"omitempty,gte=1,lte=1000000000000"`
	DailyResetTime    string                 `json:"daily_reset_time"`
}

type riskProviderValidationRequest struct {
	Text string `json:"text"`
}

const riskProviderValidationDefaultText = "AI Cove provider connection test"

func ListRiskProviders(c *gin.Context) {
	providers, err := model.GetRiskProviders()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := make([]RiskProviderResponse, 0, len(providers))
	for _, provider := range providers {
		response = append(response, toRiskProviderResponse(c.Request.Context(), provider))
	}
	common.ApiSuccess(c, response)
}

func CreateRiskProvider(c *gin.Context) {
	var request riskProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil ||
		(request.ProviderType == model.RiskProviderCloudflare && request.Credential == "") {
		common.ApiErrorMsg(c, "无效的供应商配置")
		return
	}
	ciphertext := ""
	if request.ProviderType == model.RiskProviderCloudflare {
		var err error
		ciphertext, err = common.EncryptCredential(request.Credential)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	provider := &model.RiskProvider{
		Name: request.Name, ProviderType: request.ProviderType, AccountID: request.AccountID,
		ChannelID: request.ChannelID, Model: request.Model,
		CredentialEncrypted: ciphertext, TimeoutMs: request.TimeoutMs, FailureThreshold: request.FailureThreshold,
		CooldownSeconds: request.CooldownSeconds,
		Priority:        request.Priority, DailyNeuronsLimit: request.DailyNeuronsLimit, DailyResetTime: request.DailyResetTime,
	}
	if err := model.CreateRiskProvider(provider); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, toRiskProviderResponse(c.Request.Context(), provider))
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
	connectionChanged := provider.ProviderType != request.ProviderType || provider.Model != strings.TrimSpace(request.Model)
	if provider.ProviderType == model.RiskProviderCloudflare && request.ProviderType == model.RiskProviderCloudflare {
		currentAccountID, _ := provider.CloudflareAccountID()
		connectionChanged = connectionChanged ||
			currentAccountID != strings.ToLower(strings.TrimSpace(request.AccountID)) || request.Credential != ""
	}
	if provider.ProviderType == model.RiskProviderPlatformInternal && request.ProviderType == model.RiskProviderPlatformInternal {
		connectionChanged = connectionChanged || provider.ChannelID != request.ChannelID
	}
	provider.Name = request.Name
	provider.ProviderType = request.ProviderType
	provider.AccountID = request.AccountID
	provider.ChannelID = request.ChannelID
	provider.Model = request.Model
	provider.TimeoutMs = request.TimeoutMs
	provider.FailureThreshold = request.FailureThreshold
	provider.CooldownSeconds = request.CooldownSeconds
	provider.Priority = request.Priority
	provider.DailyNeuronsLimit = request.DailyNeuronsLimit
	provider.DailyResetTime = request.DailyResetTime
	if request.ProviderType == model.RiskProviderCloudflare && request.Credential != "" {
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
	common.ApiSuccess(c, toRiskProviderResponse(c.Request.Context(), provider))
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
	validationText := riskProviderValidationDefaultText
	if c.Request.ContentLength != 0 {
		var request riskProviderValidationRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			common.ApiErrorMsg(c, "无效的连接测试内容")
			return
		}
		validationText = strings.TrimSpace(request.Text)
		if validationText == "" || len([]rune(validationText)) > 4000 {
			common.ApiErrorMsg(c, "连接测试内容必须为 1 至 4000 个字符")
			return
		}
	}
	startedAt := time.Now()
	result, err := service.ReviewRiskContentWithBudget(c.Request.Context(), provider, validationText)
	recordInput := riskProviderValidationRecordInput(c, provider, result, err, startedAt)
	metadata := service.BuildRiskRecordContentMetadata(validationText)
	recordInput.Preview = metadata.Preview
	recordInput.ContentHash = metadata.ContentHash
	if err != nil {
		if recordErr := model.RecordRiskProviderValidation(c.Request.Context(), recordInput); recordErr != nil {
			logger.LogError(c.Request.Context(), "record risk provider validation: "+recordErr.Error())
		}
		if errors.Is(err, context.DeadlineExceeded) {
			common.ApiErrorI18n(c, i18n.MsgRiskProviderConnectionTimeout, map[string]any{"TimeoutMs": provider.TimeoutMs})
			return
		}
		common.ApiError(c, err)
		return
	}
	if err := model.RecordRiskProviderValidation(c.Request.Context(), recordInput); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.MarkRiskProviderValidated(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func riskProviderValidationRecordInput(
	c *gin.Context,
	provider *model.RiskProvider,
	result service.RiskReviewResult,
	reviewErr error,
	startedAt time.Time,
) model.RiskRecordInput {
	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		requestID = "risk-provider-validation-" + common.GetUUID()
	}
	recordResult := model.RiskRecordResult(result.Status)
	errorCode := ""
	errorDetail := ""
	if reviewErr != nil {
		recordResult = model.RiskRecordResultError
		errorCode, errorDetail = service.RiskObservationErrorInfo(reviewErr)
	}
	neurons := service.NormalizeRiskProviderNeurons(result.Usage.Neurons)
	providerCalled := reviewErr == nil || !service.IsRiskProviderLocalBudgetUnavailable(reviewErr)
	providerID := 0
	providerName := ""
	providerType := model.RiskProviderType("")
	source := model.RiskRecordSourceLocal
	if providerCalled {
		providerID = provider.Id
		providerName = provider.Name
		providerType = provider.ProviderType
		source = model.RiskRecordSourceProvider
	}
	return model.RiskRecordInput{
		RequestID: requestID, ChannelID: provider.ChannelID, UserID: c.GetInt("id"), Model: provider.Model,
		ProviderID: providerID, ProviderName: providerName, ProviderType: providerType,
		Result: recordResult, Categories: append([]string(nil), result.Categories...),
		LatencyMS:    time.Since(startedAt).Milliseconds(),
		PromptTokens: result.Usage.PromptTokens, CompletionTokens: result.Usage.CompletionTokens,
		TotalTokens: result.Usage.TotalTokens, Neurons: neurons,
		ErrorCode: errorCode, ErrorDetail: errorDetail, Source: source,
		ProviderCalled: providerCalled, ObservedAt: time.Now(),
	}
}

func ActivateRiskProvider(c *gin.Context) {
	id, ok := parseRiskProviderID(c)
	if !ok {
		return
	}
	active := true
	if c.Request.ContentLength != 0 {
		var request struct {
			Active *bool `json:"active"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			common.ApiErrorMsg(c, "无效的供应商启用状态")
			return
		}
		if request.Active != nil {
			active = *request.Active
		}
	}
	if err := model.SetRiskProviderActive(id, active); err != nil {
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
	common.ApiSuccess(c, toRiskProviderResponse(c.Request.Context(), provider))
}

func parseRiskProviderID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		common.ApiErrorMsg(c, "无效的供应商 ID")
		return 0, false
	}
	return id, true
}

func toRiskProviderResponse(ctx context.Context, provider *model.RiskProvider) RiskProviderResponse {
	accountID := ""
	if provider.ProviderType == model.RiskProviderCloudflare {
		accountID, _ = provider.CloudflareAccountID()
	}
	dailyLimit := provider.DailyNeuronsLimit
	if dailyLimit < 1 {
		dailyLimit = model.DefaultRiskProviderDailyNeuronsLimit
	}
	dailyResetTime := provider.DailyResetTime
	if dailyResetTime == "" {
		dailyResetTime = model.DefaultRiskProviderDailyResetTime
	}
	response := RiskProviderResponse{
		Id: provider.Id, Name: provider.Name, ProviderType: provider.ProviderType, Model: provider.Model,
		AccountID: accountID, ChannelID: provider.ChannelID, HasCredential: provider.CredentialEncrypted != "",
		SystemManaged:    provider.ProviderType == model.RiskProviderPlatformInternal && provider.InternalTokenID > 0,
		TimeoutMs:        provider.TimeoutMs,
		FailureThreshold: provider.FailureThreshold, CooldownSeconds: provider.CooldownSeconds,
		Priority: provider.Priority, DailyNeuronsLimit: dailyLimit, DailyResetTime: dailyResetTime,
		CurrentStatus: service.RiskProviderStatusNormal,
		ValidatedAt:   provider.ValidatedAt, Active: provider.Active, CreatedAt: provider.CreatedAt, UpdatedAt: provider.UpdatedAt,
	}
	if service.RiskProviderCircuitOpen(provider.Id) {
		response.CurrentStatus = service.RiskProviderStatusCircuitOpen
	}
	if provider.ProviderType == model.RiskProviderCloudflare {
		if snapshot, err := service.GetRiskProviderBudgetSnapshot(ctx, provider); err == nil {
			response.DailyNeuronsUsed = snapshot.Used
			response.DailyNeuronsReserved = snapshot.Reserved
			response.DailyNeuronsRemaining = maxInt64(dailyLimit-snapshot.Used-snapshot.Reserved, 0)
			response.DailyNeuronsResetAt = &snapshot.ReadyAt
			if snapshot.Exhausted {
				response.CurrentStatus = service.RiskProviderStatusDailyExhausted
			}
		}
	}
	return response
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}
