package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestNewSensitiveWordsDetectedErrorIsNonRetryableBadRequest(t *testing.T) {
	err := newSensitiveWordsDetectedError()

	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, types.ErrorCodeSensitiveWordsDetected, err.GetErrorCode())
	require.True(t, types.IsSkipRetryError(err))
	require.Equal(t, "sensitive words detected", err.ToOpenAIError().Message)
	require.Equal(t, types.ErrorCodeSensitiveWordsDetected, err.ToOpenAIError().Code)
}
