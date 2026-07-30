package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRiskRecordContentMetadata_usesDomainSeparatedHMACOfNormalizedContent(t *testing.T) {
	// Given
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "fixed-secret"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })

	// When
	first := BuildRiskRecordContentMetadata("  HELLO\u3000World  ")
	second := BuildRiskRecordContentMetadata("hello world")

	// Then
	assert.Equal(t, "6574319f4c83e98e86b710e7ea7c3ea6438e3849ffa56533f2f62d164a2c164b", first.ContentHash)
	assert.Equal(t, first.ContentHash, second.ContentHash)
	assert.NotContains(t, first.ContentHash, "hello")
}

func TestBuildRiskRecordContentMetadata_masksSensitivePreviewWithoutTruncating(t *testing.T) {
	// Given
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "fixed-secret"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })
	content := "https://api.example.com/v1/users?key=secret 192.168.1.1 api_key:abc " + strings.Repeat("隐", 220)

	// When
	metadata := BuildRiskRecordContentMetadata(content)

	// Then
	require.Greater(t, len([]rune(metadata.Preview)), 200)
	assert.Contains(t, metadata.Preview, strings.Repeat("隐", 220))
	assert.NotContains(t, metadata.Preview, "api.example.com")
	assert.NotContains(t, metadata.Preview, "192.168.1.1")
	assert.NotContains(t, metadata.Preview, "api_key:abc")
	assert.Len(t, metadata.ContentHash, 64)
}
