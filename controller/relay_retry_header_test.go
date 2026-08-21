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
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	capacity := types.NewErrorWithStatusCode(errors.New("server overloaded"), types.ErrorCodeServerIsOverloaded, 503)
	authUnavailable := types.NewErrorWithStatusCode(errors.New("auth unavailable"), types.ErrorCode("auth_unavailable"), 503)
	recordRelayAttemptError(ctx, capacity)
	recordRelayAttemptError(ctx, authUnavailable)
	require.Same(t, capacity, preserveCapacityAttemptError(ctx, authUnavailable))
}
