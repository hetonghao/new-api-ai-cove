package cachex

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHybridCache_GetContext_returnsCallerCancellation(t *testing.T) {
	// Given
	cache := NewHybridCache[int](HybridCacheConfig[int]{Namespace: Namespace("test")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	_, _, err := cache.GetContext(ctx, "key")

	// Then
	require.ErrorIs(t, err, context.Canceled)
}

func TestHybridCache_SetWithTTLContext_returnsCallerCancellation(t *testing.T) {
	// Given
	cache := NewHybridCache[int](HybridCacheConfig[int]{Namespace: Namespace("test")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	err := cache.SetWithTTLContext(ctx, "key", 1, time.Minute)

	// Then
	require.ErrorIs(t, err, context.Canceled)
}
