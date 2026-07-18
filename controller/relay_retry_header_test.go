package controller

import (
	"net/http/httptest"
	"strconv"
	"testing"

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
