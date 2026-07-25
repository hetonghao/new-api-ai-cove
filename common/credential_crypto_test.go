package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentialEncryptionRoundTripAndKeyIsolation(t *testing.T) {
	originalSecret := CryptoSecret
	t.Cleanup(func() { CryptoSecret = originalSecret })

	CryptoSecret = "test-credential-key"
	ciphertext, err := EncryptCredential("cf-api-token")
	require.NoError(t, err)
	require.False(t, strings.Contains(ciphertext, "cf-api-token"))

	plaintext, err := DecryptCredential(ciphertext)
	require.NoError(t, err)
	require.Equal(t, "cf-api-token", plaintext)

	CryptoSecret = "different-key"
	_, err = DecryptCredential(ciphertext)
	require.Error(t, err)
}
