package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyRelayRiskGate_keeps_sensitive_word_error_before_ai_and_upstream(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	originalCheckEnabled := setting.CheckSensitiveEnabled
	originalPromptCheck := setting.CheckSensitiveOnPromptEnabled
	setting.CheckSensitiveEnabled = true
	setting.CheckSensitiveOnPromptEnabled = true
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = originalCheckEnabled
		setting.CheckSensitiveOnPromptEnabled = originalPromptCheck
	})
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	processorCalls := 0
	upstreamCalls := 0

	// When
	err, _ := applyRelayRiskGate(ctx, relayRiskContext{
		request: &dto.GeneralOpenAIRequest{},
		info:    &relaycommon.RelayInfo{OriginModelName: "gpt-test"},
		meta:    &types.TokenCountMeta{CombineText: "test_sensitive"},
	}, func(_ *gin.Context, _ service.RiskObservationJob) service.RiskObservationRelayDecision {
		processorCalls++
		return service.RiskObservationRelayDecision{}
	})
	if err == nil {
		upstreamCalls++
	}

	// Then
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, types.ErrorCodeSensitiveWordsDetected, err.GetErrorCode())
	require.Zero(t, processorCalls)
	require.Zero(t, upstreamCalls)
}

func TestApplyRelayRiskGate_maps_block_and_builds_governed_job(t *testing.T) {
	// Given
	originalCheckEnabled := setting.CheckSensitiveEnabled
	setting.CheckSensitiveEnabled = false
	t.Cleanup(func() { setting.CheckSensitiveEnabled = originalCheckEnabled })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses?secret=hidden", nil)
	ctx.Set(common.RequestIdKey, "req-1")
	ctx.Set("id", 42)
	ctx.Set("token_id", 9)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 24)
	common.SetContextKey(ctx, constant.ContextKeyChannelName, " CPA-Pro ")
	request := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{
			{Role: "user", Content: "old"},
			{Role: "assistant", Content: "answer"},
			{Role: "user", Content: "current"},
		},
	}
	var job service.RiskObservationJob

	// When
	err, _ := applyRelayRiskGate(ctx, relayRiskContext{
		request: request,
		info:    &relaycommon.RelayInfo{OriginModelName: "gpt-test"},
		meta:    &types.TokenCountMeta{},
	}, func(_ *gin.Context, got service.RiskObservationJob) service.RiskObservationRelayDecision {
		job = got
		return service.RiskObservationRelayDecision{Blocked: true}
	})

	// Then
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, types.ErrorCodeContentPolicyViolation, err.GetErrorCode())
	require.Equal(t, "request rejected by content policy", err.ToOpenAIError().Message)
	require.Equal(t, "req-1", job.RequestID)
	require.Equal(t, 24, job.ChannelID)
	require.Equal(t, " CPA-Pro ", job.ChannelName)
	require.Equal(t, 42, job.UserID)
	require.Equal(t, 9, job.TokenID)
	require.Equal(t, "gpt-test", job.Model)
	require.Equal(t, "/v1/responses", job.Path)
	require.Equal(t, "current", job.Text)
}

func TestApplyRelayRiskGate_routes_empty_current_turn_to_processor(t *testing.T) {
	// Given
	originalCheckEnabled := setting.CheckSensitiveEnabled
	setting.CheckSensitiveEnabled = false
	t.Cleanup(func() { setting.CheckSensitiveEnabled = originalCheckEnabled })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	processorCalls := 0

	// When
	err, _ := applyRelayRiskGate(ctx, relayRiskContext{
		request: &dto.GeneralOpenAIRequest{},
		info:    &relaycommon.RelayInfo{OriginModelName: "gpt-test"},
	}, func(_ *gin.Context, job service.RiskObservationJob) service.RiskObservationRelayDecision {
		processorCalls++
		require.Empty(t, job.Text)
		return service.RiskObservationRelayDecision{}
	})

	// Then
	require.Nil(t, err)
	require.Equal(t, 1, processorCalls)
}

func TestApplyRelayRiskGate_skips_ai_risk_for_authenticated_internal_review_token(t *testing.T) {
	originalCheckEnabled := setting.CheckSensitiveEnabled
	setting.CheckSensitiveEnabled = false
	t.Cleanup(func() { setting.CheckSensitiveEnabled = originalCheckEnabled })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyRiskInternalReview, true)
	processorCalls := 0

	err, directRecord := applyRelayRiskGate(ctx, relayRiskContext{
		request: &dto.GeneralOpenAIRequest{Messages: []dto.Message{{Role: "user", Content: "review me"}}},
		info:    &relaycommon.RelayInfo{OriginModelName: "guard-model"},
	}, func(_ *gin.Context, _ service.RiskObservationJob) service.RiskObservationRelayDecision {
		processorCalls++
		return service.RiskObservationRelayDecision{}
	})

	require.Nil(t, err)
	require.Nil(t, directRecord)
	require.Zero(t, processorCalls)
}

func TestExecuteRelayAttempt_blocks_retry_to_cpa_pro_before_upstream(t *testing.T) {
	// Given
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	risk := relayRiskContext{
		request: &dto.GeneralOpenAIRequest{},
		info:    &relaycommon.RelayInfo{OriginModelName: "gpt-test"},
	}
	events := make([]string, 0, 2)
	process := func(c *gin.Context, _ service.RiskObservationJob) service.RiskObservationRelayDecision {
		if common.GetContextKeyString(c, constant.ContextKeyChannelName) != "CPA Pro" {
			return service.RiskObservationRelayDecision{}
		}
		events = append(events, "review:CPA Pro")
		return service.RiskObservationRelayDecision{Blocked: true}
	}
	upstream := func() *types.NewAPIError {
		channelName := common.GetContextKeyString(ctx, constant.ContextKeyChannelName)
		events = append(events, "upstream:"+channelName)
		return types.NewError(errors.New("retryable upstream failure"), types.ErrorCodeBadResponse)
	}

	// When: the first attempt uses a non-protected channel and fails upstream.
	common.SetContextKey(ctx, constant.ContextKeyChannelName, "Standard")
	firstErr := executeRelayAttempt(ctx, risk, relayAttemptRiskGate{process: process}, upstream)

	// And: retry channel selection switches the same request to CPA Pro.
	common.SetContextKey(ctx, constant.ContextKeyChannelName, "CPA Pro")
	secondErr := executeRelayAttempt(ctx, risk, relayAttemptRiskGate{process: process}, upstream)

	// Then: the unsafe CPA Pro retry is reviewed and blocked before its upstream call.
	require.NotNil(t, firstErr)
	require.NotNil(t, secondErr)
	require.Equal(t, types.ErrorCodeContentPolicyViolation, secondErr.GetErrorCode())
	require.Equal(t, []string{"upstream:Standard", "review:CPA Pro"}, events)
}

func TestExecuteRelayAttempt_keeps_original_gemini_model_across_mutating_retry(t *testing.T) {
	// Given
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", nil)
	originalModel := "gemini-2.5-flash"
	adaptedModel := originalModel + "-nothinking"
	info := &relaycommon.RelayInfo{OriginModelName: originalModel}
	risk := relayRiskContext{request: &dto.GeminiChatRequest{}, info: info, originalModel: originalModel}
	originalExclusionMatches := 0
	adaptedExclusionMatches := 0
	process := func(_ *gin.Context, job service.RiskObservationJob) service.RiskObservationRelayDecision {
		if job.Model == originalModel {
			originalExclusionMatches++
		}
		if job.Model == adaptedModel {
			adaptedExclusionMatches++
		}
		return service.RiskObservationRelayDecision{}
	}

	// When
	firstErr := executeRelayAttempt(ctx, risk, relayAttemptRiskGate{process: process}, func() *types.NewAPIError {
		info.OriginModelName = adaptedModel
		return types.NewError(errors.New("retryable upstream failure"), types.ErrorCodeBadResponse)
	})
	secondErr := executeRelayAttempt(ctx, risk, relayAttemptRiskGate{process: process}, func() *types.NewAPIError {
		return nil
	})

	// Then
	require.NotNil(t, firstErr)
	require.Nil(t, secondErr)
	require.Equal(t, []int{2, 0}, []int{originalExclusionMatches, adaptedExclusionMatches})
}

func TestExecuteRelayAttempt_records_direct_fallback_after_upstream(t *testing.T) {
	// Given
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	events := make([]string, 0, 2)
	job := service.RiskObservationJob{RequestID: "observe"}
	riskGate := relayAttemptRiskGate{
		process: func(_ *gin.Context, _ service.RiskObservationJob) service.RiskObservationRelayDecision {
			return service.RiskObservationRelayDecision{
				DirectRecord: &service.RiskObservationDirectRecord{
					Job:       &job,
					ErrorCode: service.RiskObservationErrorQueueFull,
				},
			}
		},
		recordDirect: func(_ context.Context, _ service.RiskObservationDirectRecord) {
			events = append(events, "direct")
		},
	}

	// When
	err := executeRelayAttempt(ctx, relayRiskContext{
		request: &dto.GeneralOpenAIRequest{},
		info:    &relaycommon.RelayInfo{OriginModelName: "gpt-test"},
	}, riskGate, func() *types.NewAPIError {
		events = append(events, "upstream")
		return nil
	})

	// Then
	require.Nil(t, err)
	require.Equal(t, []string{"upstream", "direct"}, events)
}
