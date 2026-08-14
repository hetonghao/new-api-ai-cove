package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	openai "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type responsesWebSocketRequestState struct {
	ctx           *gin.Context
	info          *relaycommon.RelayInfo
	pricingMeta   *types.TokenCountMeta
	tracker       *openai.ResponsesWebSocketUsageTracker
	rateLimit     *middleware.ModelRequestRateLimitTicket
	observability *responsesWebSocketObservability
}

func prepareResponsesWebSocketRequest(baseCtx *gin.Context, payload []byte, inheritedModel string) (*responsesWebSocketRequestState, *dto.OpenAIResponsesRequest, error) {
	validationPayload, modelName, validationErr := responsesWebSocketValidationPayload(payload, inheritedModel)
	requestPayload := validationPayload
	if validationErr != nil {
		requestPayload = payload
		modelName = inheritedModel
		if inheritedModel == "" {
			modelResult := gjson.GetBytes(payload, "model")
			if modelResult.Type == gjson.String {
				modelName = strings.TrimSpace(modelResult.String())
			}
		}
	}
	requestCtx := newResponsesWebSocketRequestContext(baseCtx, requestPayload, modelName)
	observabilityValue, _ := baseCtx.Get(responsesWebSocketObservabilityKey)
	observability, _ := observabilityValue.(*responsesWebSocketObservability)
	state := &responsesWebSocketRequestState{
		ctx:           requestCtx,
		observability: observability,
		info: &relaycommon.RelayInfo{
			OriginModelName: modelName,
			UsingGroup:      common.GetContextKeyString(requestCtx, constant.ContextKeyUsingGroup),
			StartTime:       common.GetContextKeyTime(requestCtx, constant.ContextKeyRequestStartTime),
			IsStream:        true,
		},
	}
	rateLimit, rateLimitErr := middleware.TakeModelRequestRateLimit(requestCtx)
	state.rateLimit = rateLimit
	if rateLimitErr != nil {
		return state, nil, rateLimitErr
	}
	if validationErr != nil {
		return state, nil, validationErr
	}
	if accessErr := middleware.CheckTokenModelAccess(requestCtx, modelName); accessErr != nil {
		common.CleanupBodyStorage(requestCtx)
		return state, nil, types.NewErrorWithStatusCode(accessErr, types.ErrorCodeAccessDenied, http.StatusForbidden, types.ErrOptionWithSkipRetry())
	}

	requestValue, err := helper.GetAndValidateRequest(requestCtx, types.RelayFormatOpenAIResponses)
	if err != nil {
		common.CleanupBodyStorage(requestCtx)
		return state, nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	request := requestValue.(*dto.OpenAIResponsesRequest)
	info, err := relaycommon.GenRelayInfo(requestCtx, types.RelayFormatOpenAIResponses, request, nil)
	if err != nil {
		common.CleanupBodyStorage(requestCtx)
		return state, nil, types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
	}
	info.IsResponsesWebSocket = true
	info.IsStream = true
	state.info = info

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}
	if apiErr := applyRelaySensitiveWordGate(requestCtx, meta); apiErr != nil {
		common.CleanupBodyStorage(requestCtx)
		return state, nil, apiErr
	}
	tokens, err := service.EstimateRequestToken(requestCtx, meta, info)
	if err != nil {
		common.CleanupBodyStorage(requestCtx)
		return state, nil, types.NewError(err, types.ErrorCodeCountTokenFailed)
	}
	info.SetEstimatePromptTokens(tokens)
	state.pricingMeta = meta
	return state, request, nil
}

func prepareResponsesWebSocketBilling(state *responsesWebSocketRequestState) *types.NewAPIError {
	priceData, err := helper.ModelPriceHelper(state.ctx, state.info, state.info.GetEstimatePromptTokens(), state.pricingMeta)
	if err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
	}
	if !priceData.FreeModel {
		if state.info.Billing == nil {
			if apiErr := service.PreConsumeBilling(state.ctx, priceData.QuotaToPreConsume, state.info); apiErr != nil {
				return apiErr
			}
		} else {
			if err := state.info.Billing.Reserve(priceData.QuotaToPreConsume); err != nil {
				return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
			}
			state.info.FinalPreConsumedQuota = state.info.Billing.GetPreConsumedQuota()
		}
	}
	if state.tracker == nil {
		state.tracker = openai.NewResponsesWebSocketUsageTracker(state.info)
	}
	return nil
}

func responsesWebSocketValidationPayload(payload []byte, inheritedModel string) ([]byte, string, error) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := common.Unmarshal(payload, &envelope); err != nil {
		return nil, "", err
	}
	if envelope.Type != "response.create" {
		return nil, "", errors.New("expected response.create event")
	}
	modelResult := gjson.GetBytes(payload, "model")
	if modelResult.Exists() && modelResult.Type != gjson.String {
		return nil, "", errors.New("field model must be a string")
	}
	explicitModel := strings.TrimSpace(modelResult.String())
	if inheritedModel != "" && explicitModel != "" && explicitModel != inheritedModel {
		return nil, "", fmt.Errorf("WebSocket 会话已固定模型 %s，不能切换到 %s", inheritedModel, explicitModel)
	}
	modelName := explicitModel
	validationPayload := payload
	if modelName == "" && inheritedModel != "" {
		modelName = inheritedModel
		var err error
		validationPayload, err = sjson.SetBytes(payload, "model", inheritedModel)
		if err != nil {
			return nil, "", err
		}
	}
	if modelName == "" {
		return nil, "", errors.New("field model is required")
	}
	return validationPayload, modelName, nil
}

func newResponsesWebSocketRequestContext(baseCtx *gin.Context, payload []byte, modelName string) *gin.Context {
	requestCtx := baseCtx.Copy()
	for _, key := range []constant.ContextKey{
		constant.ContextKeyWebSocketUpstreamConnectMs,
		constant.ContextKeyWebSocketFirstEventMs,
		constant.ContextKeyWebSocketFirstOutputMs,
		constant.ContextKeyWebSocketCompleteMs,
		constant.ContextKeyWebSocketCloseReason,
	} {
		delete(requestCtx.Keys, string(key))
	}
	request := baseCtx.Request.Clone(baseCtx.Request.Context())
	request.Method = http.MethodPost
	request.Body = io.NopCloser(bytes.NewReader(payload))
	request.ContentLength = int64(len(payload))
	request.Header = baseCtx.Request.Header.Clone()
	request.Header.Set("Content-Type", "application/json")
	requestCtx.Request = request
	requestCtx.Set(common.RequestIdKey, common.NewRequestId())
	requestCtx.Set("use_channel", []string{})
	requestCtx.Set(common.KeyBodyStorage, nil)
	common.SetContextKey(requestCtx, constant.ContextKeyOriginalModel, modelName)
	common.SetContextKey(requestCtx, constant.ContextKeyRequestStartTime, time.Now())
	common.SetContextKey(requestCtx, constant.ContextKeyIsStream, true)
	common.SetContextKey(requestCtx, constant.ContextKeyRelayTransport, "websocket")
	return requestCtx
}

func responsesWebSocketEventType(payload []byte) (string, error) {
	var event struct {
		Type string `json:"type"`
	}
	if err := common.Unmarshal(payload, &event); err != nil {
		return "", err
	}
	if strings.TrimSpace(event.Type) == "" {
		return "", errors.New("websocket event type is required")
	}
	return event.Type, nil
}

func defaultResponsesWebSocketRiskGate() relayAttemptRiskGate {
	return relayAttemptRiskGate{
		process: func(c *gin.Context, job service.RiskObservationJob) service.RiskObservationRelayDecision {
			return service.ProcessRiskObservationForRelay(c.Request.Context(), job)
		},
		recordDirect: func(ctx context.Context, direct service.RiskObservationDirectRecord) {
			if direct.Job != nil {
				service.RecordRiskObservationDegradationDirect(ctx, *direct.Job, direct.ErrorCode)
			}
			if direct.Event != nil {
				service.RecordRiskObservationEventDirect(ctx, *direct.Event)
			}
		},
	}
}
