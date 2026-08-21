package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetRelayRetryCountHeader(t *testing.T) {
	for _, retryCount := range []int{0, 1, 3} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)

		setRelayRetryCountHeader(ctx, retryCount)

		require.Equal(t, strconv.Itoa(retryCount), recorder.Header().Get("X-AI-Cove-Retry-Count"))
		require.Empty(t, recorder.Header().Get("X-New-Api-Retry-Count"))
	}
}

func TestPreserveCapacityAttemptErrorWhenFinalAttemptLosesAuth(t *testing.T) {
	for _, code := range []types.ErrorCode{types.ErrorCodeServerIsOverloaded, types.ErrorCodeModelCapacity, types.ErrorCodeSlowDown} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		capacity := types.NewErrorWithStatusCode(errors.New(string(code)), code, 503)
		authUnavailable := types.NewErrorWithStatusCode(errors.New("auth unavailable"), types.ErrorCodeAuthUnavailable, 503)
		recordRelayAttemptError(ctx, capacity)
		recordRelayAttemptError(ctx, authUnavailable)
		require.Same(t, capacity, preserveCapacityAttemptError(ctx, types.RelayFormatOpenAI, authUnavailable))
	}
	realtimeCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	realtimeAuthUnavailable := types.NewErrorWithStatusCode(errors.New("auth unavailable"), types.ErrorCodeAuthUnavailable, 503)
	require.Same(t, realtimeAuthUnavailable, preserveCapacityAttemptError(realtimeCtx, types.RelayFormatOpenAIRealtime, realtimeAuthUnavailable))
}

func TestPreserveCapacityAttemptErrorKeepsEvidenceOverInternalChannelExhaustion(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	capacity := types.NewErrorWithStatusCode(errors.New("model capacity"), types.ErrorCodeModelCapacity, 503)
	channelExhausted := types.NewError(errors.New("no channel"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	recordRelayAttemptError(ctx, capacity)
	recordRelayAttemptError(ctx, channelExhausted)
	require.Same(t, capacity, preserveCapacityAttemptError(ctx, types.RelayFormatOpenAI, channelExhausted))
}

func TestPreserveCapacityAttemptErrorRecognizesSanitizedOverloadMessage(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	capacity := types.NewErrorWithStatusCode(errors.New("Our servers are currently overloaded."), types.ErrorCode("unknown_error"), 503)
	authUnavailable := types.NewErrorWithStatusCode(errors.New("auth unavailable"), types.ErrorCodeAuthUnavailable, 503)
	recordRelayAttemptError(ctx, capacity)
	recordRelayAttemptError(ctx, authUnavailable)
	require.Same(t, capacity, preserveCapacityAttemptError(ctx, types.RelayFormatOpenAI, authUnavailable))
}

func TestPreserveCapacityAttemptErrorRecognizesSelectedModelCapacityMessage(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	capacity := types.NewErrorWithStatusCode(
		errors.New("Selected model is at capacity. Please try a different model."),
		types.ErrorCode("unknown_error"),
		http.StatusTooManyRequests,
	)
	authUnavailable := types.NewErrorWithStatusCode(
		errors.New("auth unavailable"),
		types.ErrorCodeAuthUnavailable,
		http.StatusUnauthorized,
	)
	recordRelayAttemptError(ctx, capacity)
	recordRelayAttemptError(ctx, authUnavailable)

	retryCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, shouldRetry(retryCtx, capacity, 1))
	require.Same(t, capacity, preserveCapacityAttemptError(ctx, types.RelayFormatOpenAI, authUnavailable))
}

func TestPreserveCapacityAttemptErrorKeepsEvidenceOverUpstream429(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	capacity := types.NewErrorWithStatusCode(
		errors.New("Selected model is at capacity. Please try a different model."),
		types.ErrorCode("unknown_error"),
		http.StatusTooManyRequests,
	)
	rateLimited := types.NewErrorWithStatusCode(
		errors.New("rate limit exceeded"),
		types.ErrorCode("rate_limit_exceeded"),
		http.StatusTooManyRequests,
	)
	recordRelayAttemptError(ctx, capacity)
	recordRelayAttemptError(ctx, rateLimited)

	require.Same(t, capacity, preserveCapacityAttemptError(ctx, types.RelayFormatOpenAI, rateLimited))
}

func TestPreserveCapacityAttemptErrorDoesNotReplaceSkipRetry429(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	capacity := types.NewErrorWithStatusCode(
		errors.New("Selected model is at capacity"),
		types.ErrorCode("unknown_error"),
		http.StatusTooManyRequests,
	)
	skipRetry429 := types.NewErrorWithStatusCode(
		errors.New("local rate limit"),
		types.ErrorCode("rate_limit_exceeded"),
		http.StatusTooManyRequests,
		types.ErrOptionWithSkipRetry(),
	)
	recordRelayAttemptError(ctx, capacity)
	recordRelayAttemptError(ctx, skipRetry429)

	require.Same(t, skipRetry429, preserveCapacityAttemptError(ctx, types.RelayFormatOpenAI, skipRetry429))
}

func TestRelayCapacityMessageMatchIsBoundedAndCaseInsensitive(t *testing.T) {
	require.True(t, isRelayCapacityError(types.NewError(errors.New("selected model is at capacity"), types.ErrorCode("unknown_error"))))
	require.True(t, isRelayCapacityError(types.NewError(errors.New("SELECTED MODEL IS AT CAPACITY"), types.ErrorCode("unknown_error"))))
	require.False(t, isRelayCapacityError(types.NewError(
		errors.New(strings.Repeat("x", relayCapacityMessageMax)+"selected model is at capacity"),
		types.ErrorCode("unknown_error"),
	)))
}

func TestGetChannelSyntheticRetainsCachedRouteIdentity(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	baseURL := "https://cpa-route.example/"
	require.NoError(t, db.Create(&model.Channel{
		Id:      2401,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "route-key",
		Status:  common.ChannelStatusEnabled,
		Name:    "route-channel",
		BaseURL: &baseURL,
		Models:  "gpt-4o-mini",
		Group:   "default",
	}).Error)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("channel_id", 2401)
	ctx.Set("channel_type", constant.ChannelTypeOpenAI)
	ctx.Set("channel_name", "route-channel")
	ctx.Set("auto_ban", true)
	retry := 0
	param := &service.RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   "gpt-4o-mini",
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	channel, err := getChannel(ctx, &relaycommon.RelayInfo{}, param)
	require.Nil(t, err)
	require.NotNil(t, channel)
	require.NotNil(t, channel.BaseURL)
	require.Equal(t, baseURL, *channel.BaseURL)
	require.Equal(t, "https://cpa-route.example", param.LastChannelRoute)
}

func TestShouldRetryHonorsExhaustedBudgetForChannelError(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channelErr := types.NewError(errors.New("channel failure"), types.ErrorCodeChannelNoAvailableKey)
	require.False(t, shouldRetry(ctx, channelErr, 0))
}

func TestShouldRetryHonorsSkipRetryForChannelError(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channelErr := types.NewError(
		errors.New("channel failure"),
		types.ErrorCodeChannelNoAvailableKey,
		types.ErrOptionWithSkipRetry(),
	)
	require.False(t, shouldRetry(ctx, channelErr, 1))
}
