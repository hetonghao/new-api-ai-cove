package controller

import (
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
	err := applyRelayRiskGate(ctx, relayRiskContext{
		request: &dto.GeneralOpenAIRequest{},
		info:    &relaycommon.RelayInfo{OriginModelName: "gpt-test"},
		meta:    &types.TokenCountMeta{CombineText: "test_sensitive"},
	}, func(_ *gin.Context, _ service.RiskObservationJob) bool {
		processorCalls++
		return false
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
	err := applyRelayRiskGate(ctx, relayRiskContext{
		request: request,
		info:    &relaycommon.RelayInfo{OriginModelName: "gpt-test"},
		meta:    &types.TokenCountMeta{},
	}, func(_ *gin.Context, got service.RiskObservationJob) bool {
		job = got
		return true
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
