package controller

import (
	"errors"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
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

func TestShouldRetryHonorsExhaustedBudgetForChannelError(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channelErr := types.NewError(errors.New("channel failure"), types.ErrorCodeChannelNoAvailableKey)
	require.False(t, shouldRetry(ctx, channelErr, 0))
}
