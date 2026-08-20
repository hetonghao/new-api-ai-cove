package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAPIErrorIsUpstreamInvalidPrompt_requiresStructured400(t *testing.T) {
	// Given
	upstream := WithOpenAIError(OpenAIError{Message: "Invalid prompt", Code: "invalid_prompt"}, http.StatusBadRequest)
	nonBadRequest := WithOpenAIError(OpenAIError{Message: "Invalid prompt", Code: "invalid_prompt"}, http.StatusInternalServerError)
	local := NewErrorWithStatusCode(errors.New("invalid prompt"), ErrorCode("invalid_prompt"), http.StatusBadRequest)
	remapped := WithOpenAIError(OpenAIError{Message: "Invalid prompt", Code: "invalid_prompt"}, http.StatusBadRequest)
	remapped.StatusCode = http.StatusInternalServerError
	upstream500Remapped := WithOpenAIError(OpenAIError{Message: "Invalid prompt", Code: "invalid_prompt"}, http.StatusInternalServerError)
	upstream500Remapped.StatusCode = http.StatusBadRequest

	// When / Then
	require.True(t, upstream.IsUpstreamInvalidPrompt())
	require.False(t, nonBadRequest.IsUpstreamInvalidPrompt())
	require.False(t, local.IsUpstreamInvalidPrompt())
	require.True(t, remapped.IsUpstreamInvalidPrompt())
	require.False(t, upstream500Remapped.IsUpstreamInvalidPrompt())
}
