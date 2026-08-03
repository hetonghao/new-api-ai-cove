package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInMemoryRateLimiterCanRequestDoesNotConsumeSlot(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)

	require.True(t, limiter.CanRequest("user", 1, 60))
	require.True(t, limiter.CanRequest("user", 1, 60))
	require.True(t, limiter.Request("user", 1, 60))
	require.False(t, limiter.CanRequest("user", 1, 60))
}
