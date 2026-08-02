package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type relayRiskContext struct {
	request       dto.Request
	info          *relaycommon.RelayInfo
	meta          *types.TokenCountMeta
	originalModel string
}

type relayRiskProcessor func(*gin.Context, service.RiskObservationJob) service.RiskObservationRelayDecision

type relayUpstreamAttempt func() *types.NewAPIError

type relayAttemptRiskGate struct {
	process      relayRiskProcessor
	recordDirect func(context.Context, service.RiskObservationDirectRecord)
}

func applyRelaySensitiveWordGate(c *gin.Context, meta *types.TokenCountMeta) *types.NewAPIError {
	if !setting.ShouldCheckPromptSensitive() || meta == nil {
		return nil
	}
	contains, words := service.CheckSensitiveText(meta.CombineText)
	if contains {
		logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
		return newSensitiveWordsDetectedError()
	}
	return nil
}

func applyRelayRiskGate(c *gin.Context, risk relayRiskContext, process relayRiskProcessor) (*types.NewAPIError, *service.RiskObservationDirectRecord) {
	if err := applyRelaySensitiveWordGate(c, risk.meta); err != nil {
		return err, nil
	}
	if common.GetContextKeyBool(c, constant.ContextKeyRiskInternalReview) {
		return nil, nil
	}
	text := service.ExtractRiskObservationText(risk.request)
	if risk.info == nil || process == nil {
		return nil, nil
	}
	modelName := risk.originalModel
	if modelName == "" {
		modelName = risk.info.OriginModelName
	}
	decision := process(c, service.RiskObservationJob{
		RequestID:   c.GetString(common.RequestIdKey),
		ChannelID:   common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		ChannelName: common.GetContextKeyString(c, constant.ContextKeyChannelName),
		UserID:      c.GetInt("id"),
		TokenID:     c.GetInt("token_id"),
		Model:       modelName,
		Path:        c.Request.URL.Path,
		Text:        text,
	})
	if !decision.Blocked {
		return nil, decision.DirectRecord
	}
	return types.NewErrorWithStatusCode(
		errors.New("request rejected by content policy"),
		types.ErrorCodeContentPolicyViolation,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	), decision.DirectRecord
}

func executeRelayAttempt(c *gin.Context, risk relayRiskContext, riskGate relayAttemptRiskGate, upstream relayUpstreamAttempt) *types.NewAPIError {
	err, directRecord := applyRelayRiskGate(c, risk, riskGate.process)
	if directRecord != nil && riskGate.recordDirect != nil {
		defer riskGate.recordDirect(c.Request.Context(), *directRecord)
	}
	if err != nil {
		return err
	}
	return upstream()
}
