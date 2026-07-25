package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type relayRiskContext struct {
	request dto.Request
	info    *relaycommon.RelayInfo
	meta    *types.TokenCountMeta
}

type relayRiskProcessor func(*gin.Context, service.RiskObservationJob) bool

type relayUpstreamAttempt func() *types.NewAPIError

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

func applyRelayRiskGate(c *gin.Context, risk relayRiskContext, process relayRiskProcessor) *types.NewAPIError {
	if err := applyRelaySensitiveWordGate(c, risk.meta); err != nil {
		return err
	}
	text := service.ExtractRiskObservationText(risk.request)
	if risk.info == nil || process == nil {
		return nil
	}
	blocked := process(c, service.RiskObservationJob{
		RequestID:   c.GetString(common.RequestIdKey),
		ChannelID:   common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		ChannelName: common.GetContextKeyString(c, constant.ContextKeyChannelName),
		UserID:      c.GetInt("id"),
		TokenID:     c.GetInt("token_id"),
		Model:       risk.info.OriginModelName,
		Path:        c.Request.URL.Path,
		Text:        text,
	})
	if !blocked {
		return nil
	}
	return types.NewErrorWithStatusCode(
		errors.New("request rejected by content policy"),
		types.ErrorCodeContentPolicyViolation,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}

func executeRelayAttempt(c *gin.Context, risk relayRiskContext, process relayRiskProcessor, upstream relayUpstreamAttempt) *types.NewAPIError {
	if err := applyRelayRiskGate(c, risk, process); err != nil {
		return err
	}
	return upstream()
}
